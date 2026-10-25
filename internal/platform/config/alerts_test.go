package config_test

import (
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/platform/config"
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
