// Package mail sends plain-text email over SMTP, for the few notices the platform emails to its own staff. Messages
// never carry personal data of players: they say that something is waiting and where to look.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
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
	// From is the sender address ("Tajirika <noreply@example.com>" or a bare address).
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
	body := Render(from.String(), rcpts, m.Subject, m.Text, time.Now()) //nolint:forbidigo // the Date header is the wall clock, not business time

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
