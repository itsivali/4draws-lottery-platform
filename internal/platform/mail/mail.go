// Package mail sends email over SMTP: the notices the platform emails to its own staff, and the few it sends players
// (a link to confirm an address or reset a password, a copy of their data). A message is plain text, optionally with
// an HTML version of the same words (the branded layout of Notice) for mail programs that show it.
package mail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Message is one email. Subject and Text must be ASCII or UTF-8 without header injection (the sender refuses
// line breaks in the subject and in addresses).
type Message struct {
	To      []string
	Subject string
	Text    string
	// HTML is the same message as a web page, for mail programs that show it; Text stays the fallback. Empty: the
	// message is plain text only.
	HTML string
	// Inline are the images HTML shows, each referred to as cid:<its Name> (the logo). They are not listed as files.
	Inline []Attachment
	// Attachments ride with the text as files (a player's copy of their data, ADR 0038).
	Attachments []Attachment
}

// Attachment is one file sent with a message.
type Attachment struct {
	// Name is the file name shown to the recipient: letters, digits, dots, dashes and underscores only.
	Name string
	// ContentType is its media type, such as application/json.
	ContentType string
	Data        []byte
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Log only logs the message (no SMTP configured). It never logs the body, which may name a link.
type Log struct{}

// Send implements Sender.
func (Log) Send(ctx context.Context, m Message) error {
	slog.WarnContext(ctx, "email not sent (no SMTP_HOST: logged only)", "to", strings.Join(m.To, ","), "subject", m.Subject)
	return nil
}

// SMTP sends through one SMTP server. Port 465 uses TLS from the first byte; any other port starts in clear text and
// upgrades with STARTTLS, which it requires before sending credentials or the message.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	// From is the sender address ("4draws <noreply@example.com>" or a bare address).
	From string
	// Timeout bounds the whole exchange (default 20 s).
	Timeout time.Duration
}

var errBadHeader = errors.New("mail: header contains a line break")

// Send implements Sender.
func (s SMTP) Send(ctx context.Context, m Message) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return errors.New("mail: SMTP_FROM is not an address")
	}
	if len(m.To) == 0 {
		return errors.New("mail: no recipient")
	}
	if strings.ContainsAny(m.Subject, "\r\n") {
		return errBadHeader
	}
	rcpts := make([]string, 0, len(m.To))
	for _, to := range m.To {
		a, err := mail.ParseAddress(to)
		if err != nil {
			return errors.New("mail: bad recipient address")
		}
		rcpts = append(rcpts, a.Address)
	}
	body, err := RenderMessage(from.String(), rcpts, m, time.Now()) //nolint:forbidigo // the Date header is the wall clock, not business time
	if err != nil {
		return err
	}

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	tlsCfg := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	d := net.Dialer{}
	if s.Port == 465 {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: %s unreachable", s.Host) // never the error text: it could carry the address or credentials
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fmt.Errorf("mail: %s did not greet", s.Host)
	}
	defer func() { _ = c.Close() }()
	if s.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("mail: %s offers no STARTTLS", s.Host)
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("mail: STARTTLS with %s failed", s.Host)
		}
	}
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("mail: %s refused the credentials", s.Host)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("mail: %s refused the sender", s.Host)
	}
	for _, r := range rcpts {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("mail: %s refused a recipient", s.Host)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: %s refused the message", s.Host)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		return fmt.Errorf("mail: sending to %s failed", s.Host)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: %s did not accept the message", s.Host)
	}
	return c.Quit()
}

// Render builds the RFC 5322 message: a plain-text UTF-8 body with CRLF line endings.
func Render(from string, to []string, subject, text string, at time.Time) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("Date: " + at.UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, ".") { // dot-stuffing (RFC 5321 §4.5.2); smtp's Data writer also does it, harmlessly twice
			line = "." + line
		}
		b.WriteString(line + "\r\n")
	}
	return b.String()
}

var (
	attachmentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	attachmentType = regexp.MustCompile(`^[a-z0-9.+-]+/[a-z0-9.+-]+$`)
)

// RenderMessage renders m as Render does when it is plain text alone. Otherwise it nests the parts as mail programs
// expect: the text and its HTML version as alternatives, the images the HTML shows related to them, and files mixed
// in after. It refuses a file name or media type that could break out of its header.
func RenderMessage(from string, to []string, m Message, at time.Time) (string, error) {
	if len(m.Attachments) == 0 && m.HTML == "" {
		return Render(from, to, m.Subject, m.Text, at), nil
	}
	for _, a := range append(append([]Attachment{}, m.Attachments...), m.Inline...) {
		if !attachmentName.MatchString(a.Name) || !attachmentType.MatchString(a.ContentType) {
			return "", errors.New("mail: bad attachment name or type")
		}
	}
	var seed [16]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return "", errors.New("mail: no randomness for the boundary")
	}
	id := hex.EncodeToString(seed[:])

	// the innermost part: the text, or the text and the HTML as alternatives
	body := part{header: "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n", content: crlf(m.Text)}
	if m.HTML != "" {
		html := part{header: "Content-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n", content: wrap76(m.HTML)}
		body = multipart("alternative", "a"+id, body, html)
		if len(m.Inline) > 0 {
			parts := []part{body}
			for _, a := range m.Inline {
				parts = append(parts, part{
					header: "Content-Type: " + a.ContentType + "; name=\"" + a.Name + "\"\r\nContent-ID: <" + a.Name + ">\r\n" +
						"Content-Disposition: inline; filename=\"" + a.Name + "\"\r\nContent-Transfer-Encoding: base64\r\n",
					content: wrap76(string(a.Data)),
				})
			}
			body = multipart("related", "r"+id, parts...)
		}
	}
	if len(m.Attachments) > 0 {
		parts := []part{body}
		for _, a := range m.Attachments {
			parts = append(parts, part{
				header: "Content-Type: " + a.ContentType + "; name=\"" + a.Name + "\"\r\nContent-Disposition: attachment; filename=\"" + a.Name + "\"\r\n" +
					"Content-Transfer-Encoding: base64\r\n",
				content: wrap76(string(a.Data)),
			})
		}
		body = multipart("mixed", "b"+id, parts...)
	}
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + m.Subject + "\r\n")
	b.WriteString("Date: " + at.UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString(body.header + "\r\n" + body.content)
	return b.String(), nil
}

// part is one MIME part: its header lines (each ending in CRLF) and its content (ending in CRLF).
type part struct{ header, content string }

// multipart joins parts as one multipart/<kind> part.
func multipart(kind, boundary string, parts ...part) part {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("--" + boundary + "\r\n" + p.header + "\r\n" + p.content)
	}
	b.WriteString("--" + boundary + "--\r\n")
	return part{header: "Content-Type: multipart/" + kind + "; boundary=\"" + boundary + "\"\r\n", content: b.String()}
}

// crlf gives text CRLF line endings.
func crlf(text string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		b.WriteString(line + "\r\n")
	}
	return b.String()
}

// wrap76 is data in base64, 76 characters a line.
func wrap76(data string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(data))
	var b strings.Builder
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return b.String()
}
