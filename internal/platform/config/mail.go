package config

import (
	"errors"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Mail configures the email the platform sends to its own staff (internal/platform/mail). Without SMTP_HOST every
// notice is only logged. Each notice is switched on by its recipient: KYC_REVIEW_NOTIFY_TO receives an email, with a
// link into the back office, whenever a player's identity submission starts waiting for a decision.
type Mail struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string // a secret
	// From is the sender address, required with SMTP_HOST ("Tajirika <noreply@example.com>" or a bare address).
	From string
	// KYCReviewTo is who is emailed when an identity submission waits for review; empty means nobody.
	KYCReviewTo string
	// BackOfficeURL is the back office's origin, for the links in notices (https://backoffice.tajirika.co.ke).
	BackOfficeURL string
}

// Enabled reports whether real email is sent.
func (m Mail) Enabled() bool { return m.SMTPHost != "" }

func loadMail() (Mail, error) {
	m := Mail{SMTPHost: os.Getenv("SMTP_HOST"), SMTPPort: 587, SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"), From: os.Getenv("SMTP_FROM"),
		KYCReviewTo: strings.TrimSpace(os.Getenv("KYC_REVIEW_NOTIFY_TO")), BackOfficeURL: strings.TrimRight(os.Getenv("BACKOFFICE_URL"), "/")}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p <= 0 || p > 65535 {
			return Mail{}, errors.New("config: SMTP_PORT must be a port number (587 for STARTTLS, 465 for TLS)")
		}
		m.SMTPPort = p
	}
	if m.SMTPHost != "" {
		if _, err := mail.ParseAddress(m.From); err != nil {
			return Mail{}, errors.New("config: SMTP_FROM must be an email address when SMTP_HOST is set")
		}
	}
	if m.KYCReviewTo != "" {
		if _, err := mail.ParseAddress(m.KYCReviewTo); err != nil {
			return Mail{}, errors.New("config: KYC_REVIEW_NOTIFY_TO must be an email address")
		}
		u, err := url.Parse(m.BackOfficeURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Path != "" {
			return Mail{}, errors.New("config: BACKOFFICE_URL must be the back office's origin (https://host) when KYC_REVIEW_NOTIFY_TO is set")
		}
	}
	return m, nil
}
