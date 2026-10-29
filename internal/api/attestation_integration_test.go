//go:build integration

package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// goodAttestor passes tokens "good" on phones; anything else fails. The challenge is checked by the identity service
// being handed one at all (the API only passes a challenge it issued and has just consumed).
type goodAttestor struct{}

func (goodAttestor) Verify(_ context.Context, platform, token, challenge string) (bool, error) {
	return token == "good" && challenge != "" && (platform == "android" || platform == "ios"), nil
}

func (e *env) challenge(t *testing.T) string {
	t.Helper()
	r := e.do(t, http.MethodPost, "/v1/auth/attestation/challenge", "", nil)
	if r.Code != http.StatusCreated || len(r.Body["challenge"].(string)) < 32 || r.Body["expires_at"] == nil {
		t.Fatalf("challenge: %d %s", r.Code, r.Raw)
	}
	return r.Body["challenge"].(string)
}

var attSeq int

// phone signs up on a phone with the given attestation evidence and returns the session.
func (e *env) phone(t *testing.T, token, challenge string) session {
	t.Helper()
	attSeq++
	m := e.msisdn()
	fp := fmt.Sprintf("phone-%d", attSeq)
	r := e.do(t, http.MethodPost, "/v1/auth/signup", "", map[string]any{"msisdn": m, "email": e.email(), "password": strongPassword,
		"device": map[string]any{"fingerprint": fp, "platform": "android", "attestation_token": token, "attestation_challenge": challenge}})
	if r.Code != http.StatusCreated {
		t.Fatalf("signup: %d %s", r.Code, r.Raw)
	}
	return session{access: r.Body["access_token"].(string), fp: fp, msisdn: "254" + strings.TrimPrefix(m, "0")}
}

func (e *env) verdict(t *testing.T, s session) string {
	t.Helper()
	var v string
	if err := e.pool.QueryRow(context.Background(), `SELECT d.attestation_verdict FROM identity.devices d JOIN identity.sessions s ON s.device_id = d.id
		JOIN identity.users u ON u.id = s.user_id WHERE u.msisdn = $1`, s.msisdn).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAPI_AnAttestationCountsOnlyWithAChallengeThisServerIssuedUsedOnce(t *testing.T) {
	e := setupWith(t, func(d *apiDeps) { d.Identity.WithAttestor(goodAttestor{}) })
	c := e.challenge(t)
	if v := e.verdict(t, e.phone(t, "good", c)); v != "passed" {
		t.Fatalf("first use: %s", v)
	}
	if v := e.verdict(t, e.phone(t, "good", c)); v != "none" {
		t.Fatalf("a replayed challenge counted: %s", v)
	}
	if v := e.verdict(t, e.phone(t, "good", "made-up-by-the-client")); v != "none" {
		t.Fatalf("an invented challenge counted: %s", v)
	}
	if v := e.verdict(t, e.phone(t, "bad", e.challenge(t))); v != "failed" {
		t.Fatalf("a failing token: %s", v)
	}
}

func TestAPI_RefuseFailedStopsDevicesThatFailedFromPlayingAndMovingMoney(t *testing.T) {
	e := setupWith(t, func(d *apiDeps) {
		d.Identity.WithAttestor(goodAttestor{})
		d.AttestationMode = "refuse_failed"
	})
	failed := e.phone(t, "bad", e.challenge(t))
	e.canPlay(t, failed, 1000)
	for _, rt := range []struct {
		path string
		body any
	}{{"/v1/plays", nil}, {"/v1/payments/topups", map[string]any{"amount_kes": 100}}, {"/v1/payouts/withdrawals", map[string]any{"amount_kes": 100}}} {
		if r := e.do(t, http.MethodPost, rt.path, failed.access, rt.body); r.Code != http.StatusForbidden || !hasType(r, "attestation-required") {
			t.Errorf("%s from a failed device: %d %s", rt.path, r.Code, r.Raw)
		}
	}
	unattested := e.phone(t, "", "")
	e.canPlay(t, unattested, 1000)
	if r := e.do(t, http.MethodPost, "/v1/plays", unattested.access, nil); r.Code != http.StatusCreated {
		t.Fatalf("refuse_failed blocked an unattested phone: %d %s", r.Code, r.Raw)
	}
}

func TestAPI_RequireLetsOnlyAttestedPhonesAndTheWebTransact(t *testing.T) {
	e := setupWith(t, func(d *apiDeps) {
		d.Identity.WithAttestor(goodAttestor{})
		d.AttestationMode = "require"
	})
	unattested := e.phone(t, "", "")
	e.canPlay(t, unattested, 1000)
	if r := e.do(t, http.MethodPost, "/v1/plays", unattested.access, nil); r.Code != http.StatusForbidden || !hasType(r, "attestation-required") {
		t.Fatalf("unattested phone: %d %s", r.Code, r.Raw)
	}
	passed := e.phone(t, "good", e.challenge(t))
	e.canPlay(t, passed, 1000)
	if r := e.do(t, http.MethodPost, "/v1/plays", passed.access, nil); r.Code != http.StatusCreated {
		t.Fatalf("attested phone: %d %s", r.Code, r.Raw)
	}
	web := e.signInWeb(t)
	e.canPlay(t, web, 1000)
	if r := e.do(t, http.MethodPost, "/v1/plays", web.access, nil); r.Code != http.StatusCreated {
		t.Fatalf("web (not attestable): %d %s", r.Code, r.Raw)
	}
}

func (e *env) signInWeb(t *testing.T) session {
	t.Helper()
	m := e.msisdn()
	r := e.do(t, http.MethodPost, "/v1/auth/signup", "", map[string]any{"msisdn": m, "email": e.email(), "password": strongPassword, "device": map[string]any{"platform": "web"}})
	if r.Code != http.StatusCreated {
		t.Fatalf("signup: %d %s", r.Code, r.Raw)
	}
	return session{access: r.Body["access_token"].(string), msisdn: "254" + strings.TrimPrefix(m, "0")}
}

func TestAPI_WhenTheWebIsRefusedASessionCallingItselfWebCannotTransactEither(t *testing.T) {
	e := setupWith(t, func(d *apiDeps) {
		d.Identity.WithAttestor(goodAttestor{})
		d.AttestationMode = "require"
		d.AttestationWeb = "refuse"
	})
	web := e.signInWeb(t) // a browser, or a script claiming to be one
	e.canPlay(t, web, 1000)
	if r := e.do(t, http.MethodPost, "/v1/plays", web.access, nil); r.Code != http.StatusForbidden || !hasType(r, "attestation-required") {
		t.Fatalf("web session with the web refused: %d %s", r.Code, r.Raw)
	}
	passed := e.phone(t, "good", e.challenge(t))
	e.canPlay(t, passed, 1000)
	if r := e.do(t, http.MethodPost, "/v1/plays", passed.access, nil); r.Code != http.StatusCreated {
		t.Fatalf("attested phone: %d %s", r.Code, r.Raw)
	}
}
