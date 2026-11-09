//go:build integration

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/itsivali/lottery-platform/internal/staff"
)

// ADR 0031: with real money only requests the edge places in Kenya sign up, top up or play (451 outside-kenya, an
// unknown country included); a withdrawal is never refused for where the player is. Every decision is recorded.
func TestAPI_WithRealMoneyOnlyKenyaSignsUpTopsUpAndPlays(t *testing.T) {
	e := setupWith(t, func(d *apiDeps) { d.GeoCountryHeader = "CF-IPCountry" })
	ctx := context.Background()

	e.header = map[string]string{"CF-IPCountry": "KE"}
	s := e.signIn(t)

	for _, country := range []string{"UG", "XX", ""} {
		e.header = map[string]string{"CF-IPCountry": country}
		for _, call := range []struct{ path string }{{"/v1/payments/topups"}, {"/v1/plays"}} {
			if r := e.do(t, http.MethodPost, call.path, s.access, map[string]any{"amount_kes": 100}); r.Code != http.StatusUnavailableForLegalReasons || r.Body["type"] == nil {
				t.Errorf("%s from %q: %d %s", call.path, country, r.Code, r.Raw)
			}
		}
		if r := e.do(t, http.MethodPost, "/v1/payouts/withdrawals", s.access, map[string]any{"amount_kes": 100}); r.Code == http.StatusUnavailableForLegalReasons {
			t.Errorf("withdrawal from %q refused for its country", country)
		}
	}
	e.header = map[string]string{"CF-IPCountry": "TZ"}
	r := e.do(t, http.MethodPost, "/v1/auth/signup", "", map[string]any{
		"first_name": "Amina", "second_name": "Otieno", "national_id": "24681357", "date_of_birth": "1990-05-17", "adult_declaration": true,
		"msisdn": e.msisdn(), "email": e.email(), "password": strongPassword, "device": map[string]any{"fingerprint": "fp-tz", "platform": "android"},
	})
	if r.Code != http.StatusUnavailableForLegalReasons {
		t.Fatalf("sign-up from TZ: %d %s", r.Code, r.Raw)
	}

	var signups, refused, withdrawals int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE action = 'signup' AND country = 'KE' AND user_id IS NOT NULL AND NOT refused),
		count(*) FILTER (WHERE refused), count(*) FILTER (WHERE action = 'withdrawal' AND NOT refused) FROM compliance.geo_signals`).Scan(&signups, &refused, &withdrawals); err != nil {
		t.Fatal(err)
	}
	if signups != 1 || refused != 3*2+1 || withdrawals != 3 {
		t.Fatalf("signals: %d accepted sign-ups, %d refused, %d withdrawals; want 1, 7, 3", signups, refused, withdrawals)
	}

	// the back office sees them on the player's timeline, newest first
	pr, err := e.deps.Identity.Authenticate(ctx, s.access)
	if err != nil {
		t.Fatal(err)
	}
	r = e.do(t, http.MethodGet, "/v1/staff/players/"+pr.UserID, e.member(staff.Staff), nil)
	locs, _ := r.Body["identity"].(map[string]any)["locations"].([]any)
	if r.Code != http.StatusOK || len(locs) != 1+3*3 || locs[len(locs)-1].(map[string]any)["country"] != "KE" || locs[0].(map[string]any)["action"] != "withdrawal" {
		t.Fatalf("timeline locations: %d %v", r.Code, locs)
	}
}
