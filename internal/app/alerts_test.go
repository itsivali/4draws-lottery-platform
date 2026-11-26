package app

import (
	"net/http"
	"testing"

	"github.com/itsivali/4draws-lottery-platform/internal/alerts"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/config"
)

// The configuration's defaults are the alert package's (the two structs are kept field for field).
func TestAlertThresholds_ConfigDefaultsAreThePackageDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if alerts.Thresholds(cfg.Alerts.Thresholds) != alerts.DefaultThresholds() {
		t.Fatalf("config %+v, package %+v", cfg.Alerts.Thresholds, alerts.DefaultThresholds())
	}
}

func TestAlertNotifier_FansOutToEveryConfiguredChannelAndNamesThem(t *testing.T) {
	hc := &http.Client{}
	if _, ok := AlertNotifier(config.Config{}, hc).(alerts.LogNotifier); !ok || AlertChannels(config.Config{}) != "log" {
		t.Fatal("nothing configured: not the log")
	}
	cfg := config.Config{Alerts: config.Alerts{WebhookURL: "https://hooks.example.com/x", Emails: []string{"ops@example.com"}}}
	m, ok := AlertNotifier(cfg, hc).(alerts.Multi)
	if !ok || len(m) != 2 || AlertChannels(cfg) != "webhook, email" {
		t.Fatalf("%T %v %q", m, len(m), AlertChannels(cfg))
	}
	if _, ok := m[0].(alerts.Webhook); !ok {
		t.Fatalf("%T", m[0])
	}
	if e, ok := m[1].(alerts.Email); !ok || len(e.To) != 1 {
		t.Fatalf("%T %+v", m[1], m[1])
	}
	cfg.Alerts.WebhookURL = ""
	if AlertChannels(cfg) != "email" || oversightSettings(cfg).AlertsDelivery != "email" {
		t.Fatal(AlertChannels(cfg))
	}
}
