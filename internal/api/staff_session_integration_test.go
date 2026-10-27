//go:build integration

package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/itsivali/lottery-platform/internal/api"
)

// staffIDTokens plays Google and Apple for the back office: the token names its claims.
type staffIDTokens struct{}

var staffClaims = map[string]jwt.MapClaims{
	"google:owner":    {"sub": "g-owner", "email": "owner@example.com", "email_verified": true},
	"apple:amina":     {"sub": "a-amina", "email": "amina@example.com", "email_verified": "true"},
	"google:stranger": {"sub": "g-x", "email": "stranger@example.com", "email_verified": true},
}

func (staffIDTokens) VerifyIDTokenClaims(_ context.Context, provider, token, nonce string) (jwt.MapClaims, error) {
	if c, ok := staffClaims[provider+":"+token]; ok && nonce == "n" {
		return c, nil
	}
	return nil, errors.New("invalid token")
}

// setupStaff uses real back-office sessions (the default env fakes them), with owner@example.com as the first admin.
func setupStaff(t *testing.T) *env {
	t.Helper()
	e := setupWith(t, func(d *api.Deps) { d.Staff = d.StaffServices.Members })
	if err := e.deps.StaffServices.Members.Bootstrap(context.Background(), "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) staffSignIn(t *testing.T, provider, token string) resp {
	t.Helper()
	return e.do(t, http.MethodPost, "/v1/staff/session", "", map[string]any{"provider": provider, "id_token": token, "nonce": "n"})
}

func TestStaffSession_GoogleOrAppleSignInOpensASessionThatSignOutEnds(t *testing.T) {
	e := setupStaff(t)
	r := e.staffSignIn(t, "google", "owner")
	tok, _ := r.Body["token"].(string)
	if r.Code != http.StatusCreated || tok == "" || r.Body["expires_at"] == nil {
		t.Fatalf("sign in: %d %s", r.Code, r.Raw)
	}
	if me := e.do(t, http.MethodGet, "/v1/staff/me", tok, nil); me.Code != http.StatusOK || me.Body["email"] != "owner@example.com" ||
		me.Body["roles"].([]any)[0] != "admin" {
		t.Fatalf("me: %d %s", me.Code, me.Raw)
	}
	if out := e.do(t, http.MethodDelete, "/v1/staff/session", tok, nil); out.Code != http.StatusNoContent {
		t.Fatalf("sign out: %d %s", out.Code, out.Raw)
	}
	if me := e.do(t, http.MethodGet, "/v1/staff/me", tok, nil); me.Code != http.StatusUnauthorized {
		t.Fatalf("after sign out: %d", me.Code)
	}
	if r := e.staffSignIn(t, "google", "stranger"); r.Code != http.StatusForbidden || !hasType(r, "not-invited") {
		t.Fatalf("not invited: %d %s", r.Code, r.Raw)
	}
	if r := e.staffSignIn(t, "google", "forged"); r.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/session", "", map[string]any{"provider": "facebook", "id_token": "x", "nonce": "n"}); r.Code != http.StatusUnauthorized {
		t.Fatalf("unknown provider: %d %s", r.Code, r.Raw)
	}
}

func TestStaffMembers_AnAdminInvitesAssignsRolesAndDisables(t *testing.T) {
	e := setupStaff(t)
	admin := e.staffSignIn(t, "google", "owner").Body["token"].(string)
	inv := e.do(t, http.MethodPost, "/v1/staff/members", admin, map[string]any{"email": "Amina@Example.com", "name": "Amina", "roles": []string{"finance"}})
	id, _ := inv.Body["id"].(string)
	if inv.Code != http.StatusCreated || inv.Body["email"] != "amina@example.com" || id == "" {
		t.Fatalf("invite: %d %s", inv.Code, inv.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/members", admin, map[string]any{"email": "amina@example.com", "roles": []string{}}); r.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/members", admin, map[string]any{"email": "x@example.com", "roles": []string{"god"}}); r.Code != http.StatusBadRequest {
		t.Fatalf("unknown role: %d %s", r.Code, r.Raw)
	}
	amina := e.staffSignIn(t, "apple", "amina").Body["token"].(string)
	if r := e.do(t, http.MethodGet, "/v1/staff/finance", amina, nil); r.Code != http.StatusOK || r.Body["totals"] == nil || len(r.Body["days"].([]any)) != 30 {
		t.Fatalf("finance: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/finance?from=2026-11-02&to=2026-11-01", amina, nil); r.Code != http.StatusBadRequest {
		t.Fatalf("reversed range: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/members/"+id+"/roles", admin, map[string]any{"roles": []string{"auditor"}}); r.Code != http.StatusOK {
		t.Fatalf("roles: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/me", amina, nil); r.Body["roles"].([]any)[0] != "auditor" {
		t.Fatalf("roles not applied to the open session: %s", r.Raw)
	}
	me := e.do(t, http.MethodGet, "/v1/staff/me", admin, nil).Body["id"].(string)
	if r := e.do(t, http.MethodPost, "/v1/staff/members/"+me+"/disable", admin, `{}`); r.Code != http.StatusConflict || !hasType(r, "self") {
		t.Fatalf("disable self: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/members/"+id+"/disable", admin, `{}`); r.Code != http.StatusOK || r.Body["active"] != false {
		t.Fatalf("disable: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/me", amina, nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("a disabled member's session: %d", r.Code)
	}
	list := e.do(t, http.MethodGet, "/v1/staff/members", admin, nil)
	if members, _ := list.Body["members"].([]any); list.Code != http.StatusOK || len(members) != 2 {
		t.Fatalf("members: %d %s", list.Code, list.Raw)
	}
}
