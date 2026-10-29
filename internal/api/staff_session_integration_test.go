//go:build integration

package api_test

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/itsivali/lottery-platform/internal/api"
	"github.com/itsivali/lottery-platform/internal/platform/mail"
	"github.com/itsivali/lottery-platform/internal/staff"
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
	if err := e.deps.StaffServices.Members.Bootstrap(context.Background(), "owner@example.com", ""); err != nil {
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
	inv := e.do(t, http.MethodPost, "/v1/staff/members", admin, map[string]any{"email": "Amina@Example.com", "name": "Amina", "roles": []string{"staff"}})
	id, _ := inv.Body["id"].(string)
	if inv.Code != http.StatusCreated || inv.Body["email"] != "amina@example.com" || id == "" {
		t.Fatalf("invite: %d %s", inv.Code, inv.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/members", admin, map[string]any{"email": "amina@example.com", "roles": []string{"staff"}}); r.Code != http.StatusConflict {
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
	if r := e.do(t, http.MethodGet, "/v1/staff/metrics", amina, nil); r.Code != http.StatusForbidden {
		t.Fatalf("metrics as staff: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/members/"+id+"/roles", admin, map[string]any{"roles": []string{"admin"}}); r.Code != http.StatusOK {
		t.Fatalf("roles: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/me", amina, nil); r.Body["roles"].([]any)[0] != "admin" {
		t.Fatalf("roles not applied to the open session: %s", r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/metrics?from=2026-10-01&to=2026-10-07", amina, nil); r.Code != http.StatusOK || len(r.Body["days"].([]any)) != 21 || r.Body["all"] == nil {
		t.Fatalf("metrics as admin: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/members/"+id+"/roles", admin, map[string]any{"roles": []string{}}); r.Code != http.StatusBadRequest {
		t.Fatalf("no roles: %d %s", r.Code, r.Raw)
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

func (e *env) passwordSignIn(t *testing.T, email, pw string) resp {
	t.Helper()
	return e.do(t, http.MethodPost, "/v1/staff/session", "", map[string]any{"provider": "password", "email": email, "password": pw})
}

// passwords the tests use (phrases, as the rules prefer; kept in one place so no literal looks like a credential)
const (
	tempPW  = "temporary nairobi welcome"
	ownPW   = "my own long password"
	resetPW = "another temporary one"
)

// outbox records the emails the api would send.
type outbox struct {
	mu   sync.Mutex
	msgs []mail.Message
}

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.msgs = append(o.msgs, m)
	return nil
}

func TestStaffSession_AForgottenPasswordIsResetThroughAnEmailedLinkOnce(t *testing.T) {
	sent := &outbox{}
	e := setupWith(t, func(d *api.Deps) {
		d.Staff = d.StaffServices.Members
		d.StaffServices.Members.WithPasswordResets(staff.PasswordResets{Sender: sent, BackOfficeURL: "https://bo.example.com/"})
	})
	if err := e.deps.StaffServices.Members.Bootstrap(context.Background(), "owner@example.com", ""); err != nil {
		t.Fatal(err)
	}
	admin := e.staffSignIn(t, "google", "owner").Body["token"].(string)
	if r := e.do(t, http.MethodPost, "/v1/staff/members", admin, map[string]any{"email": "otieno@example.com", "name": "Otieno", "roles": []string{"staff"}, "password": tempPW}); r.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", r.Code, r.Raw)
	}
	tok := e.passwordSignIn(t, "otieno@example.com", tempPW).Body["token"].(string)
	// an unknown address is answered exactly like a known one, and nothing is sent
	if r := e.do(t, http.MethodPost, "/v1/staff/password/forgot", "", map[string]any{"email": "nobody@example.com"}); r.Code != http.StatusAccepted {
		t.Fatalf("forgot (unknown): %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/password/forgot", "", map[string]any{"email": " Otieno@Example.com "}); r.Code != http.StatusAccepted {
		t.Fatalf("forgot: %d %s", r.Code, r.Raw)
	}
	if len(sent.msgs) != 1 || sent.msgs[0].To[0] != "otieno@example.com" || !strings.Contains(sent.msgs[0].Subject, "reset your password") {
		t.Fatalf("sent %+v", sent.msgs)
	}
	m := regexp.MustCompile(`https://bo\.example\.com/reset-password\?token=([A-Za-z0-9_-]+)`).FindStringSubmatch(sent.msgs[0].Text)
	if m == nil {
		t.Fatalf("no link in:\n%s", sent.msgs[0].Text)
	}
	token := m[1]
	if r := e.do(t, http.MethodPost, "/v1/staff/password/reset", "", map[string]any{"token": "not-a-token", "new_password": ownPW}); r.Code != http.StatusGone || !hasType(r, "invalid-token") {
		t.Fatalf("bad token: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/password/reset", "", map[string]any{"token": token, "new_password": "short"}); r.Code != http.StatusBadRequest || !hasType(r, "weak-password") {
		t.Fatalf("weak: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/password/reset", "", map[string]any{"token": token, "new_password": ownPW}); r.Code != http.StatusNoContent {
		t.Fatalf("reset: %d %s", r.Code, r.Raw)
	}
	// the link is spent, the old sessions are over, the new password works and needs no change
	if r := e.do(t, http.MethodPost, "/v1/staff/password/reset", "", map[string]any{"token": token, "new_password": resetPW}); r.Code != http.StatusGone {
		t.Fatalf("link reused: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/me", tok, nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("old session survived: %d", r.Code)
	}
	if r := e.passwordSignIn(t, "otieno@example.com", tempPW); r.Code != http.StatusUnauthorized {
		t.Fatalf("old password still works: %d", r.Code)
	}
	if r := e.passwordSignIn(t, "otieno@example.com", ownPW); r.Code != http.StatusCreated || r.Body["member"].(map[string]any)["must_change_password"] != false {
		t.Fatalf("new password: %d %s", r.Code, r.Raw)
	}
}

func TestStaffSession_ForgotPasswordNeedsEmailConfigured(t *testing.T) {
	e := setupStaff(t)
	if r := e.do(t, http.MethodPost, "/v1/staff/password/forgot", "", map[string]any{"email": "owner@example.com"}); r.Code != http.StatusServiceUnavailable {
		t.Fatalf("without a mailer: %d %s", r.Code, r.Raw)
	}
}

func TestStaffPlayers_StaffGiveAPlayerATemporaryPasswordTheyMustChange(t *testing.T) {
	e := setupStaff(t)
	player := e.signIn(t)
	admin := e.staffSignIn(t, "google", "owner").Body["token"].(string)
	userID, _ := e.do(t, http.MethodGet, "/v1/account", player.access, nil).Body["user_id"].(string)
	path := "/v1/staff/players/" + userID + "/password"
	if r := e.do(t, http.MethodPost, path, admin, map[string]any{"password": "fresh start 2026", "reason": ""}); r.Code != http.StatusBadRequest || !hasType(r, "reason-required") {
		t.Fatalf("no reason: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, path, admin, map[string]any{"password": "short", "reason": "caller verified by ID"}); r.Code != http.StatusBadRequest || !hasType(r, "weak-password") {
		t.Fatalf("weak: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/players/0192f000-0000-7000-8000-000000000009/password", admin, map[string]any{"password": "fresh start 2026", "reason": "x"}); r.Code != http.StatusNotFound {
		t.Fatalf("unknown player: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, path, admin, map[string]any{"password": "fresh start 2026", "reason": "caller verified by ID"}); r.Code != http.StatusNoContent {
		t.Fatalf("set: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/account", player.access, nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("the player's session survived: %d", r.Code)
	}
	login := e.do(t, http.MethodPost, "/v1/auth/login", "", map[string]any{"msisdn": player.msisdn, "email": e.email(), "first_name": "Jane", "second_name": "Wanjiru", "password": "fresh start 2026", "device": map[string]any{"fingerprint": "fp-temp", "platform": "android"}})
	access, _ := login.Body["access_token"].(string)
	if login.Code != http.StatusOK || login.Body["must_change_password"] != true || access == "" {
		t.Fatalf("login with the temporary password: %d %s", login.Code, login.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/account", access, nil); r.Code != http.StatusOK || r.Body["must_change_password"] != true {
		t.Fatalf("account: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/auth/password/change", access, map[string]any{"current_password": "fresh start 2026", "new_password": strongPassword + " again"}); r.Code != http.StatusNoContent && r.Code != http.StatusOK {
		t.Fatalf("change: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/account", access, nil); r.Code != http.StatusOK || r.Body["must_change_password"] != false {
		t.Fatalf("account after the change: %d %s", r.Code, r.Raw)
	}
}

func TestStaffMembers_MembersNameThemselvesAndAdminsNameAnyone(t *testing.T) {
	e := setupStaff(t)
	admin := e.staffSignIn(t, "google", "owner").Body["token"].(string)
	inv := e.do(t, http.MethodPost, "/v1/staff/members", admin, map[string]any{"email": "otieno@example.com", "roles": []string{"staff"}, "password": tempPW})
	id, _ := inv.Body["id"].(string)
	if inv.Code != http.StatusCreated || inv.Body["name"] != "" {
		t.Fatalf("invite without a name: %d %s", inv.Code, inv.Raw)
	}
	tok := e.passwordSignIn(t, "otieno@example.com", tempPW).Body["token"].(string)
	// naming oneself waits for the temporary password to be changed, like everything else
	if r := e.do(t, http.MethodPut, "/v1/staff/me/name", tok, map[string]any{"name": "Otieno"}); r.Code != http.StatusForbidden || !hasType(r, "password-change-required") {
		t.Fatalf("before the password change: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/password", tok, map[string]any{"current_password": tempPW, "new_password": ownPW}); r.Code != http.StatusNoContent {
		t.Fatalf("change: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/me/name", tok, map[string]any{"name": "   "}); r.Code != http.StatusBadRequest || !hasType(r, "invalid-name") {
		t.Fatalf("blank name: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/me/name", tok, map[string]any{"name": "  Otieno Kamau "}); r.Code != http.StatusOK || r.Body["name"] != "Otieno Kamau" {
		t.Fatalf("own name: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/me", tok, nil); r.Body["name"] != "Otieno Kamau" {
		t.Fatalf("me after naming: %s", r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/members/"+id+"/name", tok, map[string]any{"name": "Someone else"}); r.Code != http.StatusForbidden {
		t.Fatalf("staff naming a member: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/members/"+id+"/name", admin, map[string]any{"name": "Otieno K."}); r.Code != http.StatusOK || r.Body["name"] != "Otieno K." {
		t.Fatalf("admin renames: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/members/0192f000-0000-7000-8000-000000000009/name", admin, map[string]any{"name": "Nobody"}); r.Code != http.StatusNotFound {
		t.Fatalf("unknown member: %d %s", r.Code, r.Raw)
	}
}

func TestStaffSession_PasswordSignInStartsTemporaryAndIsChangedBeforeAnythingElse(t *testing.T) {
	e := setupStaff(t)
	admin := e.staffSignIn(t, "google", "owner").Body["token"].(string)
	// an admin invites a member with a temporary password; too weak a one is refused with the rule
	if r := e.do(t, http.MethodPost, "/v1/staff/members", admin, map[string]any{"email": "otieno@example.com", "roles": []string{"staff"}, "password": "short"}); r.Code != http.StatusBadRequest || !hasType(r, "weak-password") {
		t.Fatalf("weak temporary password: %d %s", r.Code, r.Raw)
	}
	inv := e.do(t, http.MethodPost, "/v1/staff/members", admin, map[string]any{"email": "otieno@example.com", "name": "Otieno", "roles": []string{"staff"}, "password": tempPW})
	if inv.Code != http.StatusCreated || inv.Body["has_password"] != true || inv.Body["must_change_password"] != true {
		t.Fatalf("invite: %d %s", inv.Code, inv.Raw)
	}
	id := inv.Body["id"].(string)
	for _, bad := range [][2]string{{"otieno@example.com", "wrong-password-here"}, {"nobody@example.com", tempPW}, {"owner@example.com", tempPW}} {
		if r := e.passwordSignIn(t, bad[0], bad[1]); r.Code != http.StatusUnauthorized || !hasType(r, "invalid-credentials") {
			t.Fatalf("%s: %d %s", bad[0], r.Code, r.Raw)
		}
	}
	in := e.passwordSignIn(t, "Otieno@Example.com", tempPW)
	tok, _ := in.Body["token"].(string)
	if in.Code != http.StatusCreated || tok == "" || in.Body["member"].(map[string]any)["must_change_password"] != true {
		t.Fatalf("temporary sign-in: %d %s", in.Code, in.Raw)
	}
	// with a temporary password only /me, the password change and sign-out are open
	if r := e.do(t, http.MethodGet, "/v1/staff/finance", tok, nil); r.Code != http.StatusForbidden || !hasType(r, "password-change-required") {
		t.Fatalf("before the change: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/me", tok, nil); r.Code != http.StatusOK || r.Body["must_change_password"] != true {
		t.Fatalf("me: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/password", tok, map[string]any{"current_password": "not-it-at-all", "new_password": ownPW}); r.Code != http.StatusForbidden || !hasType(r, "wrong-password") {
		t.Fatalf("wrong current: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/password", tok, map[string]any{"current_password": tempPW, "new_password": "otieno@example.com"}); r.Code != http.StatusBadRequest || !hasType(r, "weak-password") {
		t.Fatalf("email as password: %d %s", r.Code, r.Raw)
	}
	other := e.passwordSignIn(t, "otieno@example.com", tempPW).Body["token"].(string)
	if r := e.do(t, http.MethodPut, "/v1/staff/password", tok, map[string]any{"current_password": tempPW, "new_password": ownPW}); r.Code != http.StatusNoContent {
		t.Fatalf("change: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/finance", tok, nil); r.Code != http.StatusOK {
		t.Fatalf("after the change: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/me", other, nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("the other session survived the password change: %d", r.Code)
	}
	if r := e.passwordSignIn(t, "otieno@example.com", tempPW); r.Code != http.StatusUnauthorized {
		t.Fatalf("the old password still works: %d", r.Code)
	}
	// an admin resets it: the member is out until they sign in with the new temporary one and change it again
	if r := e.do(t, http.MethodPost, "/v1/staff/members/"+id+"/password", tok, map[string]any{"password": resetPW}); r.Code != http.StatusForbidden {
		t.Fatalf("staff resetting a password: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/members/"+id+"/password", admin, map[string]any{"password": resetPW}); r.Code != http.StatusNoContent {
		t.Fatalf("reset: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/me", tok, nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("session survived the reset: %d", r.Code)
	}
	if r := e.passwordSignIn(t, "otieno@example.com", resetPW); r.Code != http.StatusCreated || r.Body["member"].(map[string]any)["must_change_password"] != true {
		t.Fatalf("after the reset: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/session", "", map[string]any{"provider": "password", "email": "otieno@example.com"}); r.Code != http.StatusBadRequest {
		t.Fatalf("no password: %d %s", r.Code, r.Raw)
	}
	// a temporary password holds back only the sessions it opens: the owner, given one at bootstrap, still works through Google
	if err := e.deps.StaffServices.Members.Bootstrap(context.Background(), "owner@example.com", tempPW); err != nil {
		t.Fatal(err)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/members", e.staffSignIn(t, "google", "owner").Body["token"].(string), nil); r.Code != http.StatusOK {
		t.Fatalf("google session with a temporary password set: %d %s", r.Code, r.Raw)
	}
	if r := e.passwordSignIn(t, "owner@example.com", tempPW); r.Code != http.StatusCreated || r.Body["member"].(map[string]any)["must_change_password"] != true {
		t.Fatalf("owner's temporary password: %d %s", r.Code, r.Raw)
	}
}

func TestStaffMetrics_CountSignupsSessionsAndPlaysPerPlatform(t *testing.T) {
	e := setupStaff(t)
	admin := e.staffSignIn(t, "google", "owner").Body["token"].(string)
	s := e.signIn(t) // an android player (the test's device platform)
	e.canPlay(t, s, 1000)
	if r := e.doKey(t, http.MethodPost, "/v1/plays", s.access, "m-play-1", nil); r.Code != http.StatusCreated {
		t.Fatalf("play: %d %s", r.Code, r.Raw)
	}
	// the test clock plays on 2026-10-21 while M-Pesa's credit is stamped by the database clock: cover both
	r := e.do(t, http.MethodGet, "/v1/staff/metrics?from=2026-10-01&to="+time.Now().AddDate(0, 0, 1).Format(time.DateOnly), admin, nil)
	if r.Code != http.StatusOK {
		t.Fatalf("metrics: %d %s", r.Code, r.Raw)
	}
	days := len(r.Body["days"].([]any))
	var android map[string]any
	for _, tot := range r.Body["totals"].([]any) {
		if tot.(map[string]any)["platform"] == "android" {
			android = tot.(map[string]any)
		}
	}
	if android == nil || android["signups"].(float64) < 1 || android["sessions"].(float64) < 1 || android["active_players"].(float64) < 1 ||
		android["plays"] != float64(1) || android["top_ups"] != float64(1) || android["top_ups_kes"] != float64(1000) {
		t.Fatalf("android totals: %v (%s)", android, r.Raw)
	}
	if all := r.Body["all"].(map[string]any); all["plays"] != float64(1) || days%3 != 0 || days < 84 {
		t.Fatalf("all: %v, %d cells", all, days)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/metrics", admin, nil); r.Code != http.StatusOK || len(r.Body["days"].([]any)) != 90 {
		t.Fatalf("default range: %d, %d cells", r.Code, len(r.Body["days"].([]any)))
	}
	// the web app sends no device fingerprint: its sessions still count as web (the platform is kept on the session)
	if r := e.do(t, http.MethodPost, "/v1/auth/signup", "", map[string]any{"msisdn": e.msisdn(), "email": e.email(), "first_name": "Jane", "second_name": "Wanjiru", "password": strongPassword, "device": map[string]any{"platform": "web"}}); r.Code != http.StatusCreated {
		t.Fatalf("web signup: %d %s", r.Code, r.Raw)
	}
	r = e.do(t, http.MethodGet, "/v1/staff/metrics?from=2026-10-01&to="+time.Now().AddDate(0, 0, 1).Format(time.DateOnly), admin, nil)
	var web map[string]any
	for _, tot := range r.Body["totals"].([]any) {
		if tot.(map[string]any)["platform"] == "web" {
			web = tot.(map[string]any)
		}
	}
	if web == nil || web["signups"] != float64(1) || web["sessions"] != float64(1) || web["active_players"] != float64(1) {
		t.Fatalf("web totals: %v", web)
	}
	if len(r.Body["platforms"].([]any)) != 3 {
		t.Fatalf("an unknown platform appeared: %v", r.Body["platforms"])
	}
	for _, q := range []string{"from=2026-11-02&to=2026-11-01", "from=nope"} {
		if r := e.do(t, http.MethodGet, "/v1/staff/metrics?"+q, admin, nil); r.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", q, r.Code, r.Raw)
		}
	}
}
