package config_test

import (
	"testing"

	"github.com/itsivali/lottery-platform/internal/platform/config"
)

func TestLoad_MailIsOffByDefaultAndSwitchedOnByARecipient(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	for _, k := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "KYC_REVIEW_NOTIFY_TO", "BACKOFFICE_URL"} {
		t.Setenv(k, "")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if m := cfg.Mail; m.Enabled() || m.KYCReviewTo != "" || m.SMTPPort != 587 {
		t.Fatalf("%+v", m)
	}
	t.Setenv("SMTP_HOST", "smtp.resend.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_USERNAME", "resend")
	t.Setenv("SMTP_PASSWORD", "re_x")
	t.Setenv("SMTP_FROM", "Bahati <noreply@example.com>")
	t.Setenv("KYC_REVIEW_NOTIFY_TO", "ops@example.com")
	t.Setenv("BACKOFFICE_URL", "https://backoffice.example.com/")
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if m := cfg.Mail; !m.Enabled() || m.SMTPPort != 465 || m.KYCReviewTo != "ops@example.com" || m.BackOfficeURL != "https://backoffice.example.com" {
		t.Fatalf("%+v", m)
	}
}

func TestLoad_MailRefusesHalfConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	for name, env := range map[string]map[string]string{
		"smtp without a sender":         {"SMTP_HOST": "smtp.example.com"},
		"sender that is not an address": {"SMTP_HOST": "smtp.example.com", "SMTP_FROM": "bahati"},
		"bad port":                      {"SMTP_PORT": "mail"},
		"recipient without back office": {"KYC_REVIEW_NOTIFY_TO": "ops@example.com"},
		"back office with a path":       {"KYC_REVIEW_NOTIFY_TO": "ops@example.com", "BACKOFFICE_URL": "https://x.example.com/kyc"},
		"recipient not an address":      {"KYC_REVIEW_NOTIFY_TO": "ops", "BACKOFFICE_URL": "https://x.example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, k := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_FROM", "KYC_REVIEW_NOTIFY_TO", "BACKOFFICE_URL"} {
				t.Setenv(k, env[k])
			}
			if _, err := config.Load(); err == nil {
				t.Errorf("%v accepted", env)
			}
		})
	}
}
