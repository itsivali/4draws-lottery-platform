package config_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/itsivali/lottery-platform/internal/platform/config"
)

func b64(n int) string { return base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", n))) }

// fullAPIEnv sets every variable the v1 API needs.
func fullAPIEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("AUTH_KEY", b64(32))
	t.Setenv("TOKEN_SEED", b64(32))
	t.Setenv("DATA_KEYS", "1:"+b64(32))
	t.Setenv("IDENTITY_KEY", b64(32))
	t.Setenv("VALKEY_ADDR", "valkey:6379")
	t.Setenv("TRUSTED_PROXY_HEADER", "X-Forwarded-For")
	for _, k := range []string{"TRUSTED_PROXY_HOPS", "DATA_KEY_ACTIVE", "KYC_API_URL", "KYC_API_KEY", "GOOGLE_CLIENT_ID", "APPLE_CLIENT_ID", "CORS_ALLOWED_ORIGINS", "STAFF_GOOGLE_CLIENT_ID", "STAFF_APPLE_CLIENT_ID", "STAFF_DEV_OIDC_ISSUER", "STAFF_DEV_OIDC_CLIENT_IDS", "STAFF_DEV_OIDC_JWKS_URL", "STAFF_BOOTSTRAP_ADMIN", "STAFF_DATABASE_URL", "ATTESTATION_MODE", "APP_ATTEST_APP_ID", "APP_ATTEST_ALLOW_DEVELOPMENT", "PLAY_INTEGRITY_PACKAGE", "PLAY_INTEGRITY_SERVICE_ACCOUNT", "PLAY_INTEGRITY_CERT_DIGESTS", "ATTESTATION_WEB"} {
		t.Setenv(k, "")
	}
}

func TestAPI_DisabledWhenNoSecretsAreSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	for _, k := range []string{"AUTH_KEY", "TOKEN_SEED", "DATA_KEYS", "IDENTITY_KEY", "VALKEY_ADDR", "TRUSTED_PROXY_HEADER"} {
		t.Setenv(k, "")
	}
	cfg, err := config.Load()
	if err != nil || cfg.API.Enabled {
		t.Fatalf("enabled=%v err=%v, want health-only mode", cfg.API.Enabled, err)
	}
}

func TestAPI_EnabledWithEverythingSet(t *testing.T) {
	fullAPIEnv(t)
	cfg, err := config.Load()
	if err != nil || !cfg.API.Enabled {
		t.Fatalf("enabled=%v err=%v", cfg.API.Enabled, err)
	}
	a := cfg.API
	if len(a.AuthKey) != 32 || len(a.TokenSeed) != 32 || len(a.IdentityKey) != 32 || len(a.DataKeys[1]) != 32 || a.DataKeyActive != 1 {
		t.Fatalf("keys not decoded: %+v", a)
	}
	if a.TrustedProxyHeader != "X-Forwarded-For" || a.TrustedProxyHops != 1 || a.ValkeyAddr != "valkey:6379" {
		t.Fatalf("settings: %+v", a)
	}
}

func TestAPI_PartialConfigurationIsRefusedAndNamesWhatIsMissing(t *testing.T) {
	for _, missing := range []string{"AUTH_KEY", "TOKEN_SEED", "DATA_KEYS", "IDENTITY_KEY", "VALKEY_ADDR"} {
		fullAPIEnv(t)
		t.Setenv(missing, "")
		_, err := config.Load()
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("missing %s: err = %v, want it to name the variable", missing, err)
		}
	}
}

func TestAPI_TrustedProxyHeaderMustBeChosenExplicitly(t *testing.T) {
	fullAPIEnv(t)
	t.Setenv("TRUSTED_PROXY_HEADER", "")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXY_HEADER") {
		t.Fatalf("API started without a proxy decision: %v", err)
	}
	t.Setenv("TRUSTED_PROXY_HEADER", "none")
	cfg, err := config.Load()
	if err != nil || cfg.API.TrustedProxyHeader != "" {
		t.Fatalf("none: header=%q err=%v (none means use the socket address)", cfg.API.TrustedProxyHeader, err)
	}
	t.Setenv("TRUSTED_PROXY_HEADER", "X-Forwarded-For")
	t.Setenv("TRUSTED_PROXY_HOPS", "3")
	if cfg, _ := config.Load(); cfg.API.TrustedProxyHops != 3 {
		t.Fatalf("hops = %d", cfg.API.TrustedProxyHops)
	}
	for _, bad := range []string{"0", "9", "x", "-1"} {
		t.Setenv("TRUSTED_PROXY_HOPS", bad)
		if _, err := config.Load(); err == nil {
			t.Errorf("hops %q accepted", bad)
		}
	}
}

func TestAPI_RejectsMalformedSecrets(t *testing.T) {
	cases := map[string]string{
		"AUTH_KEY":     b64(8),
		"TOKEN_SEED":   b64(16),
		"IDENTITY_KEY": "not base64!!",
		"DATA_KEYS":    "1:" + b64(16),
	}
	for k, v := range cases {
		fullAPIEnv(t)
		t.Setenv(k, v)
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), k) {
			t.Errorf("%s=%q: err = %v", k, v, err)
		}
	}
	fullAPIEnv(t)
	t.Setenv("DATA_KEYS", "1:"+b64(32)+",1:"+b64(32))
	if _, err := config.Load(); err == nil {
		t.Error("duplicate key id accepted")
	}
	fullAPIEnv(t)
	t.Setenv("DATA_KEYS", "1:"+b64(32)+",2:"+b64(32))
	t.Setenv("DATA_KEY_ACTIVE", "2")
	if cfg, err := config.Load(); err != nil || cfg.API.DataKeyActive != 2 {
		t.Errorf("rotation: %+v %v", cfg.API.DataKeyActive, err)
	}
	t.Setenv("DATA_KEY_ACTIVE", "7")
	if _, err := config.Load(); err == nil {
		t.Error("active key id not in the keyring accepted")
	}
}

func TestAPI_RegistryURLMustBeHTTPSExceptLocalOnes(t *testing.T) {
	fullAPIEnv(t)
	t.Setenv("KYC_API_URL", "http://registry.example.com/verify")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "KYC_API_URL") {
		t.Errorf("plain-http public gateway accepted: %v", err)
	}
	for _, ok := range []string{"https://registry.example.com/verify", "http://kyc-mock:8080/verify", "http://localhost:8091/verify"} {
		t.Setenv("KYC_API_URL", ok)
		if _, err := config.Load(); err != nil {
			t.Errorf("%s rejected: %v", ok, err)
		}
	}
}

func TestWorkersInProcess_IsOffUnlessExplicitlyEnabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	for _, v := range []string{"", "false", "yes", "1"} {
		t.Setenv("WORKERS_IN_PROCESS", v)
		if cfg, _ := config.Load(); cfg.WorkersInProcess {
			t.Errorf("WORKERS_IN_PROCESS=%q enabled workers", v)
		}
	}
	t.Setenv("WORKERS_IN_PROCESS", "true")
	if cfg, _ := config.Load(); !cfg.WorkersInProcess {
		t.Error("WORKERS_IN_PROCESS=true did not enable workers")
	}
}

func TestAPI_SingleDataKeyShorthand(t *testing.T) {
	fullAPIEnv(t)
	t.Setenv("DATA_KEYS", "")
	t.Setenv("DATA_KEY", b64(32))
	cfg, err := config.Load()
	if err != nil || len(cfg.API.DataKeys[1]) != 32 || cfg.API.DataKeyActive != 1 {
		t.Fatalf("DATA_KEY shorthand: %+v %v", cfg.API.DataKeys, err)
	}
	t.Setenv("DATA_KEY", b64(10))
	if _, err := config.Load(); err == nil {
		t.Error("short DATA_KEY accepted")
	}
}

func TestMigrateOnStart_IsOffUnlessExplicitlyEnabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("MIGRATE_ON_START", "")
	if cfg, _ := config.Load(); cfg.MigrateOnStart {
		t.Error("migrations ran by default")
	}
	t.Setenv("MIGRATE_ON_START", "true")
	if cfg, _ := config.Load(); !cfg.MigrateOnStart {
		t.Error("MIGRATE_ON_START=true ignored")
	}
}

func TestAPI_CORSOriginsAreExactHTTPSOriginsOrLocalhost(t *testing.T) {
	fullAPIEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://play.example.co.ke, http://localhost:5173,http://127.0.0.1:8088")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.API.CORSOrigins, " "); got != "https://play.example.co.ke http://localhost:5173 http://127.0.0.1:8088" {
		t.Fatalf("origins = %q", got)
	}
	for _, bad := range []string{"*", "https://*.example.co.ke", "http://play.example.co.ke", "https://play.example.co.ke/",
		"https://play.example.co.ke/app", "play.example.co.ke", "https://user@play.example.co.ke", "https://play.example.co.ke?x=1"} {
		t.Setenv("CORS_ALLOWED_ORIGINS", bad)
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
			t.Errorf("%q accepted (err %v)", bad, err)
		}
	}
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	if cfg, err := config.Load(); err != nil || len(cfg.API.CORSOrigins) != 0 {
		t.Fatalf("unset: %v %v", cfg.API.CORSOrigins, err)
	}
}

func TestAPI_StaffSignInIsPasswordWithGoogleAppleOrTheLocalTestRealmOptional(t *testing.T) {
	fullAPIEnv(t)
	for _, k := range []string{"STAFF_GOOGLE_CLIENT_ID", "STAFF_APPLE_CLIENT_ID", "STAFF_DEV_OIDC_ISSUER", "STAFF_DEV_OIDC_CLIENT_IDS",
		"STAFF_DEV_OIDC_JWKS_URL", "STAFF_BOOTSTRAP_ADMIN", "STAFF_BOOTSTRAP_PASSWORD", "STAFF_DATABASE_URL"} {
		t.Setenv(k, "")
	}
	// password sign-in needs nothing configured (ADR 0019): the staff routes are always open
	cfg, err := config.Load()
	if err != nil || !cfg.API.Staff.Enabled || len(cfg.API.Staff.Providers()) != 1 || cfg.API.Staff.Providers()[0] != "password" {
		t.Fatalf("unset: %+v %v", cfg.API.Staff, err)
	}
	t.Setenv("STAFF_GOOGLE_CLIENT_ID", "123-abc.apps.googleusercontent.com")
	t.Setenv("STAFF_APPLE_CLIENT_ID", "ke.tajirika.backoffice")
	t.Setenv("STAFF_BOOTSTRAP_ADMIN", "Owner@Example.com")
	t.Setenv("STAFF_BOOTSTRAP_PASSWORD", "a long temporary password")
	t.Setenv("STAFF_DATABASE_URL", "postgres://worker@db/lottery")
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.API.Staff
	if !s.Enabled || s.GoogleClientID != "123-abc.apps.googleusercontent.com" || s.AppleClientID != "ke.tajirika.backoffice" ||
		s.BootstrapAdmin != "owner@example.com" || s.BootstrapPassword != "a long temporary password" || s.DatabaseURL != "postgres://worker@db/lottery" || s.Dev != nil {
		t.Fatalf("staff: %+v", s)
	}
	if got := s.Providers(); len(got) != 3 || got[0] != "password" || got[1] != "google" || got[2] != "apple" {
		t.Fatalf("providers %v", got)
	}
	t.Setenv("STAFF_BOOTSTRAP_ADMIN", "not-an-email")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "STAFF_BOOTSTRAP_ADMIN") {
		t.Fatalf("bad bootstrap admin accepted: %v", err)
	}
	t.Setenv("STAFF_BOOTSTRAP_ADMIN", "")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "STAFF_BOOTSTRAP_PASSWORD") {
		t.Fatalf("a bootstrap password without an admin accepted: %v", err)
	}
	t.Setenv("STAFF_BOOTSTRAP_PASSWORD", "")

	// the local staff realm, for local development and the e2e journey only: all three or none, https except in compose
	t.Setenv("STAFF_DEV_OIDC_ISSUER", "http://localhost:8081/realms/lottery-staff")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "STAFF_DEV_OIDC_CLIENT_IDS") {
		t.Fatalf("partial dev realm accepted: %v", err)
	}
	t.Setenv("STAFF_DEV_OIDC_CLIENT_IDS", "lottery-backoffice, lottery-e2e")
	t.Setenv("STAFF_DEV_OIDC_JWKS_URL", "http://sso.example.co.ke/certs")
	if _, err := config.Load(); err == nil {
		t.Fatal("plain http JWKS accepted")
	}
	t.Setenv("STAFF_DEV_OIDC_JWKS_URL", "http://keycloak:8080/realms/lottery-staff/protocol/openid-connect/certs")
	cfg, err = config.Load()
	if err != nil || cfg.API.Staff.Dev == nil || len(cfg.API.Staff.Dev.ClientIDs) != 2 || cfg.API.Staff.Dev.ClientIDs[1] != "lottery-e2e" {
		t.Fatalf("dev realm: %+v %v", cfg.API.Staff.Dev, err)
	}
	if got := cfg.API.Staff.Providers(); len(got) != 4 || got[3] != "dev" {
		t.Fatalf("providers %v", got)
	}
}

func TestAPI_AttestationModeNeedsItsVerifiers(t *testing.T) {
	fullAPIEnv(t)
	cfg, err := config.Load()
	if err != nil || cfg.API.Attestation.Mode != "record" || cfg.API.Attestation.Web != "allow" {
		t.Fatalf("default: %+v %v", cfg.API.Attestation, err)
	}
	t.Setenv("ATTESTATION_WEB", "sometimes")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ATTESTATION_WEB") {
		t.Fatalf("bad ATTESTATION_WEB accepted: %v", err)
	}
	t.Setenv("ATTESTATION_WEB", "refuse")
	t.Setenv("ATTESTATION_MODE", "lenient")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ATTESTATION_MODE") {
		t.Fatalf("unknown mode accepted: %v", err)
	}
	t.Setenv("ATTESTATION_MODE", "require")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "APP_ATTEST_APP_ID") {
		t.Fatalf("require without verifiers accepted: %v", err)
	}
	t.Setenv("APP_ATTEST_APP_ID", "ABCDE12345.ke.tajirika.lottery_mobile")
	t.Setenv("PLAY_INTEGRITY_PACKAGE", "ke.tajirika.lottery_mobile")
	t.Setenv("PLAY_INTEGRITY_SERVICE_ACCOUNT", base64.StdEncoding.EncodeToString([]byte(`{"client_email":"v@x.iam.gserviceaccount.com"}`)))
	t.Setenv("PLAY_INTEGRITY_CERT_DIGESTS", "abc,def")
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.API.Attestation
	if a.Mode != "require" || a.AppID == "" || a.PlayPackage == "" || len(a.PlayServiceAccount) == 0 || len(a.PlayCertDigests) != 2 || a.AllowDevelopment {
		t.Fatalf("attestation: %+v", a)
	}
	t.Setenv("PLAY_INTEGRITY_SERVICE_ACCOUNT", "not base64!")
	if _, err := config.Load(); err == nil {
		t.Fatal("bad service account encoding accepted")
	}
}
