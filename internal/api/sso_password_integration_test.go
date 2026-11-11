//go:build integration

package api_test

import (
	"net/http"
	"testing"
)

// ADR 0035: an account made with Google sets a password right after signing in, then signs in with its phone number;
// it cannot set one twice, and the account reports a password and (with Google's photo) the photo.
func TestAPI_AGoogleAccountSetsAPasswordAndSignsInWithItsNumber(t *testing.T) {
	e := setup(t)
	m := e.msisdn()
	dev := map[string]any{"fingerprint": "fp-sso-pw", "platform": "android"}
	r := e.do(t, http.MethodPost, "/v1/auth/sso/google/signup", "", map[string]any{"first_name": "Amina", "second_name": "Otieno", "national_id": "24681357",
		"date_of_birth": "1990-05-17", "adult_declaration": true, "id_token": "ok:g-setpw", "nonce": "n1", "msisdn": m, "device": dev})
	if r.Code != http.StatusCreated {
		t.Fatalf("sso sign-up: %d %s", r.Code, r.Raw)
	}
	tok := r.Body["access_token"].(string)
	if a := e.do(t, http.MethodGet, "/v1/account", tok, nil); a.Body["has_password"] != false {
		t.Fatalf("account: %s", a.Raw)
	}
	if s := e.do(t, http.MethodPost, "/v1/auth/password/set", tok, map[string]any{"new_password": "password"}); s.Code != http.StatusBadRequest {
		t.Fatalf("a weak password: %d %s", s.Code, s.Raw)
	}
	if s := e.do(t, http.MethodPost, "/v1/auth/password/set", tok, map[string]any{"new_password": strongPassword}); s.Code != http.StatusNoContent {
		t.Fatalf("set: %d %s", s.Code, s.Raw)
	}
	if s := e.do(t, http.MethodPost, "/v1/auth/password/set", tok, map[string]any{"new_password": "another strong passphrase"}); s.Code != http.StatusConflict {
		t.Fatalf("set twice: %d %s", s.Code, s.Raw)
	}
	if a := e.do(t, http.MethodGet, "/v1/account", tok, nil); a.Body["has_password"] != true || a.Body["photo_url"] != nil {
		t.Fatalf("account after: %s", a.Raw)
	}
	if l := e.do(t, http.MethodPost, "/v1/auth/login", "", map[string]any{"msisdn": m, "password": strongPassword, "device": dev}); l.Code != http.StatusOK {
		t.Fatalf("login with the number: %d %s", l.Code, l.Raw)
	}
}
