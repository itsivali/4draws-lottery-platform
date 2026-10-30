package app

import (
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/platform/config"
)

// The back office's System page reads the settings the api runs with (owner, 2026-10-30).
func TestOversightSettings_SayWhatIsOnWithoutSecrets(t *testing.T) {
	cfg := config.Config{Payments: config.Payments{Enabled: true, BaseURL: config.SandboxURL, SandboxSettle: true,
		MinTopUpKES: 1, TopUpCooldown: 2 * time.Minute, ConsumerSecret: "secret"}}
	s := oversightSettings(cfg)
	if s.Daraja != "sandbox" || !s.SandboxSettlement || s.TopUpMinKES != 1 || s.TopUpCooldownS != 120 || s.AlertsDelivery != "log" {
		t.Fatalf("settings = %+v", s)
	}
	cfg.Payments.BaseURL, cfg.Payments.SandboxSettle = "https://api.safaricom.co.ke", false
	if s := oversightSettings(cfg); s.Daraja != "production" {
		t.Fatalf("production: %+v", s)
	}
	if s := oversightSettings(config.Config{}); s.Daraja != "" {
		t.Fatalf("top-ups off: %+v", s)
	}
}
