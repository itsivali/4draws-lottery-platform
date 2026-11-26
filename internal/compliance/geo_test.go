package compliance_test

import (
	"testing"

	"github.com/itsivali/4draws-lottery-platform/internal/compliance"
)

func TestCountry_IsTwoCharactersOrNothing(t *testing.T) {
	for in, want := range map[string]string{"KE": "KE", " ke ": "KE", "T1": "T1", "XX": "XX", "": "", "KEN": "", "K": "", "K-": ""} {
		if got := compliance.Country(in); got != want {
			t.Errorf("Country(%q) = %q, want %q", in, got, want)
		}
	}
}

// ADR 0031: the sandbox only records; with real money only KE goes on, and a withdrawal always does.
func TestGeo_OnlyTheLivePolicyRefuses(t *testing.T) {
	sandbox, live := compliance.NewGeo(nil, nil, false), compliance.NewGeo(nil, nil, true)
	var none *compliance.Geo
	for _, action := range []string{compliance.GeoSignup, compliance.GeoTopUp, compliance.GeoPlay} {
		if !sandbox.Allowed(action, "UG") || !none.Allowed(action, "UG") {
			t.Errorf("%s from UG refused outside the live policy", action)
		}
		if live.Allowed(action, "UG") || live.Allowed(action, "") || live.Allowed(action, "T1") || !live.Allowed(action, "KE") {
			t.Errorf("%s: the live policy must serve KE only", action)
		}
	}
	if !live.Allowed(compliance.GeoWithdrawal, "UG") {
		t.Error("a withdrawal was refused for where the player is")
	}
}
