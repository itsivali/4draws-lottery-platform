package config_test

import (
	"strings"
	"testing"

	"github.com/itsivali/lottery-platform/internal/platform/config"
)

func TestOperating_DefaultsToDisabledAndRefusesUnknownModes(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("OPERATING_MODE", "")
	if cfg, err := config.Load(); err != nil || cfg.Operating.Mode != "disabled" {
		t.Fatalf("mode = %q err = %v, want disabled", cfg.Operating.Mode, err)
	}
	t.Setenv("OPERATING_MODE", "on")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "OPERATING_MODE") {
		t.Fatalf("unknown mode: err = %v, want a refusal", err)
	}
}

func TestOperating_SandboxNeverRunsOnProductionDaraja(t *testing.T) {
	fullPaymentsEnv(t)
	t.Setenv("OPERATING_MODE", "sandbox")
	if cfg, err := config.Load(); err != nil || cfg.Operating.Mode != "sandbox" {
		t.Fatalf("sandbox on the sandbox: mode = %q err = %v", cfg.Operating.Mode, err)
	}
	t.Setenv("DARAJA_BASE_URL", "https://api.safaricom.co.ke")
	t.Setenv("DARAJA_ALLOW_PRODUCTION", "true")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("sandbox on production: err = %v, want a refusal", err)
	}
	// stopped, production credentials are allowed: nothing moves
	t.Setenv("OPERATING_MODE", "disabled")
	if _, err := config.Load(); err != nil {
		t.Fatalf("disabled on production: %v", err)
	}
	// a sandbox without any Daraja (a demo of reads only) starts
	t.Setenv("DATABASE_URL", "postgres://x")
	for _, k := range darajaVars {
		t.Setenv(k, "")
	}
	t.Setenv("DARAJA_BASE_URL", "")
	t.Setenv("DARAJA_ALLOW_PRODUCTION", "")
	t.Setenv("OPERATING_MODE", "sandbox")
	if _, err := config.Load(); err != nil {
		t.Fatalf("sandbox without Daraja: %v", err)
	}
}

func TestOperating_LiveNeedsProductionDarajaAndALicenceReference(t *testing.T) {
	fullPaymentsEnv(t)
	t.Setenv("OPERATING_MODE", "live")
	t.Setenv("REGULATORY_LICENCE_REFERENCE", "GRA/OPL/0001")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "production Daraja") {
		t.Fatalf("live on the sandbox: err = %v, want a refusal", err)
	}
	t.Setenv("DARAJA_BASE_URL", "https://api.safaricom.co.ke")
	t.Setenv("DARAJA_ALLOW_PRODUCTION", "true")
	t.Setenv("REGULATORY_LICENCE_REFERENCE", "")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "REGULATORY_LICENCE_REFERENCE") {
		t.Fatalf("live without a licence: err = %v, want a refusal", err)
	}
	t.Setenv("REGULATORY_LICENCE_REFERENCE", " GRA/OPL/0001 ")
	// with real money the edge must say which country each request comes from (ADR 0031)
	t.Setenv("GEO_COUNTRY_HEADER", "")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "GEO_COUNTRY_HEADER") {
		t.Fatalf("live without a country signal: err = %v, want a refusal", err)
	}
	t.Setenv("GEO_COUNTRY_HEADER", "CF-IPCountry")
	cfg, err := config.Load()
	if err != nil || cfg.Operating.Mode != "live" || cfg.Operating.LicenceReference != "GRA/OPL/0001" {
		t.Fatalf("live: %+v err = %v", cfg.Operating, err)
	}
}

// ADR 0031: the country header is trusted only behind a proxy that sets it; a client could send it itself otherwise.
func TestGeoCountryHeader_NeedsATrustedProxyAndAHeaderName(t *testing.T) {
	fullPaymentsEnv(t)
	t.Setenv("GEO_COUNTRY_HEADER", "CF-IPCountry")
	if cfg, err := config.Load(); err != nil || cfg.API.GeoCountryHeader != "CF-IPCountry" {
		t.Fatalf("header %q err = %v", cfg.API.GeoCountryHeader, err)
	}
	t.Setenv("GEO_COUNTRY_HEADER", "CF IPCountry")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "GEO_COUNTRY_HEADER") {
		t.Fatalf("malformed header name: err = %v", err)
	}
	t.Setenv("GEO_COUNTRY_HEADER", "CF-IPCountry")
	t.Setenv("TRUSTED_PROXY_HEADER", "none")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "GEO_COUNTRY_HEADER") {
		t.Fatalf("country header without a proxy: err = %v", err)
	}
}
