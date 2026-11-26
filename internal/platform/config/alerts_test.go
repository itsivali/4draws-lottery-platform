package config_test

import (
	"testing"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/platform/config"
)

func TestLoad_AlertsDefaultToLoggingWithPlaceholderDeadlines(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ALERT_WEBHOOK_URL", "")
	t.Setenv("ALERT_HEARTBEAT_URL", "")
	t.Setenv("KRA_REMIT_WARN_AFTER", "")
	t.Setenv("KRA_REMIT_DUE_AFTER", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if a := cfg.Alerts; a.WebhookURL != "" || a.HeartbeatURL != "" || a.KRARemitWarnAfter != 72*time.Hour || a.KRARemitDueAfter != 120*time.Hour {
		t.Fatalf("%+v", a)
	}
	t.Setenv("ALERT_WEBHOOK_URL", "https://hooks.example.com/T/B/x")
	t.Setenv("ALERT_HEARTBEAT_URL", "https://hc.example.com/ping/uuid")
	t.Setenv("KRA_REMIT_WARN_AFTER", "24h")
	t.Setenv("KRA_REMIT_DUE_AFTER", "48h")
	if cfg, err = config.Load(); err != nil || cfg.Alerts.KRARemitDueAfter != 48*time.Hour || cfg.Alerts.WebhookURL == "" {
		t.Fatalf("%+v %v", cfg.Alerts, err)
	}
}

func TestLoad_AlertsRefuseInsecureURLsAndBadDeadlines(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	for name, env := range map[string][2]string{
		"plain http webhook": {"ALERT_WEBHOOK_URL", "http://hooks.example.com/x"},
		"no host":            {"ALERT_HEARTBEAT_URL", "https:///ping"},
		"not a duration":     {"KRA_REMIT_DUE_AFTER", "five days"},
		"negative":           {"KRA_REMIT_WARN_AFTER", "-1h"},
		"warn after the due": {"KRA_REMIT_WARN_AFTER", "200h"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(env[0], env[1])
			if _, err := config.Load(); err == nil {
				t.Errorf("%s=%s accepted", env[0], env[1])
			}
		})
	}
}

func TestLoad_AlertEmailsAndThresholds(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ALERT_EMAILS", "ops@example.com, Owner <owner@example.com>")
	t.Setenv("TOPUP_UNCONFIRMED_PAGE_AFTER", "2h")
	t.Setenv("JOB_RETRY_WARN_ATTEMPTS", "3")
	t.Setenv("B2C_FLOAT_FLOOR_KES", "50000")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Alerts
	if len(a.Emails) != 2 || a.Emails[1] != "Owner <owner@example.com>" || a.Thresholds.TopUpUnconfirmedPage != 2*time.Hour ||
		a.Thresholds.TopUpUnconfirmedWarn != 30*time.Minute || a.Thresholds.JobRetryWarnAttempts != 3 || a.Thresholds.B2CFloatFloorKES != 50000 ||
		a.Thresholds.SuspensePage != 24*time.Hour || a.Thresholds.ClosureWaitWarn != 168*time.Hour {
		t.Fatalf("%+v", a)
	}
	for name, env := range map[string][2]string{
		"not an address":      {"ALERT_EMAILS", "ops"},
		"zero attempts":       {"JOB_RETRY_WARN_ATTEMPTS", "0"},
		"negative floor":      {"B2C_FLOAT_FLOOR_KES", "-1"},
		"warn after the page": {"TOPUP_UNCONFIRMED_WARN_AFTER", "3h"},
		"suspense not a time": {"SUSPENSE_PAGE_AFTER", "soon"},
		"throttle count":      {"DARAJA_THROTTLE_COUNT", "many"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(env[0], env[1])
			if _, err := config.Load(); err == nil {
				t.Errorf("%s=%s accepted", env[0], env[1])
			}
		})
	}
}
