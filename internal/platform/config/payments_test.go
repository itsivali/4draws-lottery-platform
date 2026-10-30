package config_test

import (
	"strings"
	"testing"

	"github.com/itsivali/lottery-platform/internal/platform/config"
)

var darajaVars = []string{"DARAJA_CONSUMER_KEY", "DARAJA_CONSUMER_SECRET", "DARAJA_SHORTCODE", "DARAJA_PASSKEY", "DARAJA_CALLBACK_TOKEN"}

func fullPaymentsEnv(t *testing.T) {
	t.Helper()
	fullAPIEnv(t)
	t.Setenv("DARAJA_CONSUMER_KEY", "ck")
	t.Setenv("DARAJA_CONSUMER_SECRET", "cs")
	t.Setenv("DARAJA_SHORTCODE", "174379")
	t.Setenv("DARAJA_PASSKEY", "pk")
	t.Setenv("DARAJA_CALLBACK_TOKEN", "a/b+c=0123456789abcdef0123456789") // Render-generated values are base64
	t.Setenv("DARAJA_BASE_URL", "")
	t.Setenv("DARAJA_ALLOW_PRODUCTION", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	t.Setenv("RENDER_EXTERNAL_URL", "https://lottery-api.onrender.com")
	t.Setenv("TOP_UP_MIN_KES", "")
	t.Setenv("DARAJA_SANDBOX_SETTLE", "")
	for _, k := range allInitiatorVars {
		t.Setenv(k, "")
	}
}

var c2bVars = []string{"DARAJA_C2B_SHORTCODE", "DARAJA_INITIATOR_NAME", "DARAJA_SECURITY_CREDENTIAL"}

var allInitiatorVars = []string{"DARAJA_C2B_SHORTCODE", "DARAJA_B2C_SHORTCODE", "DARAJA_INITIATOR_NAME", "DARAJA_SECURITY_CREDENTIAL"}

func fullC2BEnv(t *testing.T) {
	t.Helper()
	fullPaymentsEnv(t)
	t.Setenv("DARAJA_C2B_SHORTCODE", "600982")
	t.Setenv("DARAJA_INITIATOR_NAME", "testapi")
	t.Setenv("DARAJA_SECURITY_CREDENTIAL", "bG9jYWwtZGV2LW9ubHk=")
}

func TestPayments_BuyGoodsIsOnlyUsedWithATillNumber(t *testing.T) {
	fullPaymentsEnv(t)
	t.Setenv("DARAJA_TILL_NUMBER", "")
	if cfg, err := config.Load(); err != nil || cfg.Payments.TillNumber != "" {
		t.Fatalf("till=%q err=%v", cfg.Payments.TillNumber, err)
	}
	t.Setenv("DARAJA_TILL_NUMBER", "5432101")
	if cfg, err := config.Load(); err != nil || cfg.Payments.TillNumber != "5432101" {
		t.Fatalf("till=%q err=%v", cfg.Payments.TillNumber, err)
	}
}

func TestPayments_C2BIsOffUnlessConfigured(t *testing.T) {
	fullPaymentsEnv(t)
	cfg, err := config.Load()
	if err != nil || !cfg.Payments.Enabled || cfg.Payments.C2BEnabled() {
		t.Fatalf("payments=%v c2b=%v err=%v", cfg.Payments.Enabled, cfg.Payments.C2BEnabled(), err)
	}
}

func TestPayments_C2BConfiguredGivesHookURLsWithoutForbiddenWords(t *testing.T) {
	fullC2BEnv(t)
	cfg, err := config.Load()
	if err != nil || !cfg.Payments.C2BEnabled() {
		t.Fatalf("c2b=%v err=%v", cfg.Payments.C2BEnabled(), err)
	}
	p := cfg.Payments
	if p.C2BShortCode != "600982" || p.Initiator != "testapi" || p.SecurityCredential != "bG9jYWwtZGV2LW9ubHk=" {
		t.Fatalf("payments = %+v", p)
	}
	for _, action := range []string{"validate", "confirm", "status-result", "status-timeout"} {
		u := p.C2BURL(action)
		if u != "https://lottery-api.onrender.com/hooks/c2b/"+p.CallbackPath+"/"+action {
			t.Errorf("url = %q", u)
		}
		// Daraja refuses C2B URLs containing these words
		for _, w := range []string{"mpesa", "safaricom", "exec", "cmd", "sql", "query"} {
			if strings.Contains(strings.ToLower(u), w) {
				t.Errorf("%s contains %q", u, w)
			}
		}
	}
}

func TestPayments_PartialC2BConfigurationIsRefused(t *testing.T) {
	for _, missing := range c2bVars {
		fullC2BEnv(t)
		t.Setenv(missing, "")
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("missing %s: err = %v", missing, err)
		}
	}
	fullAPIEnv(t)
	for _, k := range darajaVars {
		t.Setenv(k, "")
	}
	t.Setenv("DARAJA_C2B_SHORTCODE", "600982")
	t.Setenv("DARAJA_INITIATOR_NAME", "")
	t.Setenv("DARAJA_SECURITY_CREDENTIAL", "")
	if _, err := config.Load(); err == nil {
		t.Error("c2b without the daraja app: accepted")
	}
}

func TestPayments_OffWhenNoDarajaCredentialsAreSet(t *testing.T) {
	fullAPIEnv(t)
	for _, k := range darajaVars {
		t.Setenv(k, "")
	}
	cfg, err := config.Load()
	if err != nil || cfg.Payments.Enabled {
		t.Fatalf("enabled=%v err=%v", cfg.Payments.Enabled, err)
	}
}

func TestPayments_AGeneratedCallbackTokenAloneLeavesPaymentsOff(t *testing.T) {
	fullAPIEnv(t)
	for _, k := range darajaVars {
		t.Setenv(k, "")
	}
	t.Setenv("DARAJA_CALLBACK_TOKEN", "generated-by-the-host-0123456789abcdef")
	cfg, err := config.Load()
	if err != nil || cfg.Payments.Enabled {
		t.Fatalf("enabled=%v err=%v; the host generates the token before anyone enters credentials", cfg.Payments.Enabled, err)
	}
}

func TestPayments_EnabledDefaultsToTheSandboxAndAnUnguessableCallbackPath(t *testing.T) {
	fullPaymentsEnv(t)
	cfg, err := config.Load()
	if err != nil || !cfg.Payments.Enabled {
		t.Fatalf("enabled=%v err=%v", cfg.Payments.Enabled, err)
	}
	p := cfg.Payments
	if p.BaseURL != "https://sandbox.safaricom.co.ke" || p.ConsumerKey != "ck" || p.ShortCode != "174379" || p.AllowProduction {
		t.Fatalf("payments = %+v", p)
	}
	if len(p.CallbackPath) != 64 || strings.ContainsAny(p.CallbackPath, "/+=") {
		t.Fatalf("callback path %q must be a hex digest, safe in a URL path", p.CallbackPath)
	}
	if p.CallbackURL() != "https://lottery-api.onrender.com/hooks/mpesa/stk/"+p.CallbackPath {
		t.Fatalf("callback url = %q", p.CallbackURL())
	}
	t.Setenv("PUBLIC_BASE_URL", "https://api.example.ke/")
	cfg, _ = config.Load()
	if !strings.HasPrefix(cfg.Payments.CallbackURL(), "https://api.example.ke/hooks/mpesa/stk/") {
		t.Fatalf("PUBLIC_BASE_URL not preferred: %q", cfg.Payments.CallbackURL())
	}
}

func TestPayments_PartialOrUnsafeConfigurationIsRefused(t *testing.T) {
	for _, missing := range darajaVars {
		fullPaymentsEnv(t)
		t.Setenv(missing, "")
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("missing %s: err = %v", missing, err)
		}
	}
	for name, set := range map[string]func(){
		"short callback token": func() { t.Setenv("DARAJA_CALLBACK_TOKEN", "short") },
		"no public url":        func() { t.Setenv("RENDER_EXTERNAL_URL", "") },
		"production base":      func() { t.Setenv("DARAJA_BASE_URL", "https://api.safaricom.co.ke") },
		"payments without api": func() {
			t.Setenv("AUTH_KEY", "")
			t.Setenv("TOKEN_SEED", "")
			t.Setenv("DATA_KEYS", "")
			t.Setenv("IDENTITY_KEY", "")
			t.Setenv("VALKEY_ADDR", "")
		},
	} {
		fullPaymentsEnv(t)
		set()
		if _, err := config.Load(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	fullPaymentsEnv(t)
	t.Setenv("DARAJA_BASE_URL", "https://api.safaricom.co.ke")
	t.Setenv("DARAJA_ALLOW_PRODUCTION", "true")
	if cfg, err := config.Load(); err != nil || !cfg.Payments.AllowProduction {
		t.Fatalf("explicit production: %v", err)
	}
}

func TestPayments_B2CNeedsItsShortcodeAndTheInitiator(t *testing.T) {
	fullPaymentsEnv(t)
	if cfg, err := config.Load(); err != nil || cfg.Payments.B2CEnabled() {
		t.Fatalf("b2c=%v err=%v", cfg.Payments.B2CEnabled(), err)
	}
	t.Setenv("DARAJA_B2C_SHORTCODE", "600992")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DARAJA_INITIATOR_NAME") {
		t.Fatalf("b2c without initiator: %v", err)
	}
	t.Setenv("DARAJA_INITIATOR_NAME", "testapi")
	t.Setenv("DARAJA_SECURITY_CREDENTIAL", "bG9jYWwtZGV2LW9ubHk=")
	cfg, err := config.Load()
	if err != nil || !cfg.Payments.B2CEnabled() || cfg.Payments.C2BEnabled() {
		t.Fatalf("b2c=%v c2b=%v err=%v", cfg.Payments.B2CEnabled(), cfg.Payments.C2BEnabled(), err)
	}
	p := cfg.Payments
	for _, action := range []string{"result", "timeout", "status-result", "status-timeout"} {
		if u := p.B2CURL(action); !strings.HasPrefix(u, "https://lottery-api.onrender.com/hooks/b2c/"+p.CallbackPath+"/") || !strings.HasSuffix(u, action) {
			t.Errorf("%s: %s", action, u)
		}
	}
	// both sides on one shortcode, as in the sandbox
	t.Setenv("DARAJA_C2B_SHORTCODE", "600992")
	if cfg, err := config.Load(); err != nil || !cfg.Payments.B2CEnabled() || !cfg.Payments.C2BEnabled() {
		t.Fatalf("both: %v", err)
	}
}

func TestPayments_TheTopUpMinimumIsSetByTheServerWithinWhatMPesaAccepts(t *testing.T) {
	fullPaymentsEnv(t)
	cfg, err := config.Load()
	if err != nil || cfg.Payments.MinTopUpKES != 0 {
		t.Fatalf("unset: %d %v (0 leaves the payments default)", cfg.Payments.MinTopUpKES, err)
	}
	t.Setenv("TOP_UP_MIN_KES", "1") // testing with real money, a shilling at a time
	if cfg, err = config.Load(); err != nil || cfg.Payments.MinTopUpKES != 1 {
		t.Fatalf("1: %d %v", cfg.Payments.MinTopUpKES, err)
	}
	for _, bad := range []string{"0", "-5", "ten", "250001"} {
		t.Setenv("TOP_UP_MIN_KES", bad)
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "TOP_UP_MIN_KES") {
			t.Fatalf("%q accepted (%v)", bad, err)
		}
	}
}

func TestPayments_SandboxSettlementIsOptInAndRefusedWithProductionDaraja(t *testing.T) {
	fullPaymentsEnv(t)
	if cfg, err := config.Load(); err != nil || cfg.Payments.SandboxSettle {
		t.Fatalf("default: settle=%v err=%v", cfg.Payments.SandboxSettle, err)
	}
	t.Setenv("DARAJA_SANDBOX_SETTLE", "true")
	if cfg, err := config.Load(); err != nil || !cfg.Payments.SandboxSettle {
		t.Fatalf("sandbox: settle=%v err=%v", cfg.Payments.SandboxSettle, err)
	}
	t.Setenv("DARAJA_BASE_URL", "https://api.safaricom.co.ke")
	t.Setenv("DARAJA_ALLOW_PRODUCTION", "true")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DARAJA_SANDBOX_SETTLE") {
		t.Fatalf("production: err = %v, want a refusal", err)
	}
}
