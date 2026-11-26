//go:build integration

package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/payments"
	"github.com/itsivali/4draws-lottery-platform/internal/payouts"
	"github.com/itsivali/4draws-lottery-platform/internal/staff"
)

// sameInstant compares an RFC 3339 value from a response with want, whatever offset it was written in.
func sameInstant(got any, want string) bool {
	s, _ := got.(string)
	g, err1 := time.Parse(time.RFC3339, s)
	w, err2 := time.Parse(time.RFC3339, want)
	return err1 == nil && err2 == nil && g.Equal(w)
}

func TestAPI_WalletShowsTheConfirmedBalanceDebtAndHistoryNewestFirst(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	if r := e.do(t, http.MethodGet, "/v1/wallet", s.access, nil); r.Code != http.StatusOK || r.Body["balance_kes"] != float64(0) ||
		r.Body["debt_kes"] != float64(0) || len(r.Body["transactions"].([]any)) != 0 {
		t.Fatalf("new wallet: %d %s", r.Code, r.Raw)
	}
	// the apps take the top-up and withdrawal bounds from here, so a minimum changes without a store release
	if r := e.do(t, http.MethodGet, "/v1/wallet", s.access, nil); r.Body["top_up"] == nil ||
		r.Body["top_up"].(map[string]any)["min_kes"] != float64(payments.DefaultMinTopUpKES) || r.Body["top_up"].(map[string]any)["max_kes"] != float64(250000) ||
		r.Body["withdraw"] == nil || r.Body["withdraw"].(map[string]any)["min_kes"] != float64(payouts.DefaultMinWithdrawalKES) ||
		r.Body["withdraw"].(map[string]any)["max_kes"] != float64(payouts.DefaultMaxWithdrawalKES) {
		t.Fatalf("bounds: %s", r.Raw)
	}
	e.canPlay(t, s, 1000) // 950 after excise
	if r := e.doKey(t, http.MethodPost, "/v1/plays", s.access, "play-1", nil); r.Code != http.StatusCreated {
		t.Fatalf("play: %d %s", r.Code, r.Raw)
	}
	r := e.do(t, http.MethodGet, "/v1/wallet", s.access, nil)
	if r.Code != http.StatusOK || r.Body["balance_kes"] != float64(850) {
		t.Fatalf("wallet: %d %s", r.Code, r.Raw)
	}
	txs := r.Body["transactions"].([]any)
	var sum float64
	for _, x := range txs {
		sum += x.(map[string]any)["amount_kes"].(float64)
	}
	if sum != 850 {
		t.Fatalf("transactions sum to %v, want the balance: %s", sum, r.Raw)
	}
	newest := txs[0].(map[string]any)
	if newest["kind"] != "stake" || newest["amount_kes"] != float64(-100) || newest["reference_type"] != "ticket" {
		t.Fatalf("newest transaction: %v", newest)
	}

	// pages: limit and a cursor (the last id seen)
	first := e.do(t, http.MethodGet, "/v1/wallet?limit=1", s.access, nil)
	page := first.Body["transactions"].([]any)
	if len(page) != 1 || first.Body["next_before"] == nil {
		t.Fatalf("first page: %s", first.Raw)
	}
	second := e.do(t, http.MethodGet, "/v1/wallet?limit=100&before="+first.Body["next_before"].(string), s.access, nil)
	if got := len(second.Body["transactions"].([]any)); got != len(txs)-1 || second.Body["next_before"] != nil {
		t.Fatalf("second page has %d, want %d: %s", got, len(txs)-1, second.Raw)
	}
	for _, q := range []string{"?limit=0", "?limit=101", "?before=nope"} {
		if r := e.do(t, http.MethodGet, "/v1/wallet"+q, s.access, nil); r.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", q, r.Code, r.Raw)
		}
	}

	other := e.signIn(t)
	if r := e.do(t, http.MethodGet, "/v1/wallet", other.access, nil); r.Body["balance_kes"] != float64(0) || len(r.Body["transactions"].([]any)) != 0 {
		t.Fatalf("another player's wallet: %s", r.Raw)
	}
}

func TestAPI_AccountShowsVerificationRestrictionAndClosureRequest(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	r := e.do(t, http.MethodGet, "/v1/account", s.access, nil)
	if r.Code != http.StatusOK || r.Body["msisdn"] != s.msisdn || r.Body["phone_verified"] != false || r.Body["kyc_status"] != "none" ||
		r.Body["status"] != "active" || r.Body["restriction"] != nil || r.Body["closure_requested_at"] != nil || r.Body["created_at"] == nil {
		t.Fatalf("new account: %d %s", r.Code, r.Raw)
	}

	e.do(t, http.MethodPost, "/v1/rg/timeout", s.access, map[string]any{"duration": "24h"})
	r = e.do(t, http.MethodGet, "/v1/account", s.access, nil)
	restriction, _ := r.Body["restriction"].(map[string]any)
	if restriction["kind"] != "timeout" || !sameInstant(restriction["until"], "2026-10-22T12:00:00Z") {
		t.Fatalf("timed out: %s", r.Raw)
	}
	e.do(t, http.MethodPost, "/v1/rg/self-exclusion", s.access, map[string]any{"term": "permanent"})
	r = e.do(t, http.MethodGet, "/v1/account", s.access, nil)
	restriction, _ = r.Body["restriction"].(map[string]any)
	if restriction["kind"] != "self_exclusion" || restriction["until"] != nil {
		t.Fatalf("permanent exclusion must win: %s", r.Raw)
	}

	c := e.do(t, http.MethodPost, "/v1/account/closure", s.access, nil)
	if c.Code != http.StatusAccepted || !sameInstant(c.Body["closure_requested_at"], "2026-10-21T12:00:00Z") {
		t.Fatalf("closure: %d %s", c.Code, c.Raw)
	}
	e.clk.Advance(time.Minute)
	if again := e.do(t, http.MethodPost, "/v1/account/closure", s.access, nil); again.Code != http.StatusAccepted ||
		again.Body["closure_requested_at"] != c.Body["closure_requested_at"] {
		t.Fatalf("a second request must keep the first: %d %s", again.Code, again.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/account", s.access, nil); r.Body["closure_requested_at"] != c.Body["closure_requested_at"] {
		t.Fatalf("account after closure request: %s", r.Raw)
	}
	var audited int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit.audit_log WHERE action = 'identity.closure_requested'`).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("closure audited %d times (%v), want once", audited, err)
	}

	other := e.signIn(t)
	if r := e.do(t, http.MethodGet, "/v1/account", other.access, nil); r.Body["restriction"] != nil || r.Body["closure_requested_at"] != nil {
		t.Fatalf("another player's account: %s", r.Raw)
	}
}

func TestAPI_TopUpHistoryListsOnlyTheOwnersTopUpsNewestFirst(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	e.verified(t, s)
	for i, amount := range []int{50, 70} {
		if r := e.do(t, http.MethodPost, "/v1/payments/topups", s.access, map[string]any{"amount_kes": amount}); r.Code != http.StatusAccepted {
			t.Fatalf("top-up %d: %d %s", i, r.Code, r.Raw)
		}
		e.clk.Advance(1e9)
	}
	r := e.do(t, http.MethodGet, "/v1/payments", s.access, nil)
	list, _ := r.Body["payments"].([]any)
	if r.Code != http.StatusOK || len(list) != 2 || list[0].(map[string]any)["amount_requested_kes"] != float64(70) ||
		list[0].(map[string]any)["status"] != "pending" {
		t.Fatalf("list: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/payments?limit=1", s.access, nil); len(r.Body["payments"].([]any)) != 1 {
		t.Fatalf("limit: %s", r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/payments?limit=0", s.access, nil); r.Code != http.StatusBadRequest {
		t.Fatalf("bad limit: %d", r.Code)
	}
	other := e.signIn(t)
	if r := e.do(t, http.MethodGet, "/v1/payments", other.access, nil); len(r.Body["payments"].([]any)) != 0 {
		t.Fatalf("another player's top-ups: %s", r.Raw)
	}
}

func TestAPI_ATopUpWithinTheCooldownIsRefusedWithHowLongToWait(t *testing.T) {
	e := setupWith(t, func(d *apiDeps) { d.Payments = d.Payments.WithTopUpCooldown(2 * time.Minute) })
	s := e.signIn(t)
	e.verified(t, s)
	if r := e.do(t, http.MethodGet, "/v1/wallet", s.access, nil); r.Body["top_up"].(map[string]any)["wait_s"] != float64(0) ||
		r.Body["top_up"].(map[string]any)["cooldown_s"] != float64(120) {
		t.Fatalf("wallet before: %s", r.Raw)
	}
	first := e.do(t, http.MethodPost, "/v1/payments/topups", s.access, map[string]any{"amount_kes": 50})
	if first.Code != http.StatusAccepted || first.Body["next_top_up_in_s"] != float64(120) {
		t.Fatalf("first: %d %s", first.Code, first.Raw)
	}
	e.clk.Advance(50 * time.Second)
	again := e.do(t, http.MethodPost, "/v1/payments/topups", s.access, map[string]any{"amount_kes": 50})
	if again.Code != http.StatusTooManyRequests || !hasType(again, "top-up-cooldown") || again.Header.Get("Retry-After") != "70" ||
		again.Body["detail"] != "you can top up again in 2 minutes" {
		t.Fatalf("too soon: %d %v %s", again.Code, again.Header.Get("Retry-After"), again.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/wallet", s.access, nil); r.Body["top_up"].(map[string]any)["wait_s"] != float64(70) {
		t.Fatalf("wallet during the cooldown: %s", r.Raw)
	}
	e.clk.Advance(70 * time.Second)
	if r := e.do(t, http.MethodPost, "/v1/payments/topups", s.access, map[string]any{"amount_kes": 50}); r.Code != http.StatusAccepted {
		t.Fatalf("after the cooldown: %d %s", r.Code, r.Raw)
	}
}

// A new account gives its first name, second name and ID number at sign-up (owner, 2026-10-30); its account shows the
// names, and verification before the first cash-out starts from them without a date of birth.
func TestAPI_SignUpTakesNamesAndIDNumberThatVerificationStartsFrom(t *testing.T) {
	e := setup(t)
	dev := map[string]any{"fingerprint": "fp-names", "platform": "android"}
	base := map[string]any{"msisdn": e.msisdn(), "email": e.email(), "password": strongPassword, "device": dev, "date_of_birth": "1990-05-17", "adult_declaration": true}
	for name, extra := range map[string]map[string]any{
		"no names":         {"national_id": "24681357"},
		"no ID number":     {"first_name": "Amina", "second_name": "Otieno"},
		"bad ID number":    {"first_name": "Amina", "second_name": "Otieno", "national_id": "12"},
		"a name of digits": {"first_name": "4mina", "second_name": "Otieno", "national_id": "24681357"},
	} {
		body := map[string]any{}
		for k, v := range base {
			body[k] = v
		}
		for k, v := range extra {
			body[k] = v
		}
		if r := e.do(t, http.MethodPost, "/v1/auth/signup", "", body); r.Code != http.StatusBadRequest || !hasType(r, "invalid-identity") {
			t.Fatalf("%s: %d %s", name, r.Code, r.Raw)
		}
	}
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.users WHERE msisdn = $1`, "254"+base["msisdn"].(string)[1:]).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a refused sign-up left %d accounts (%v)", n, err)
	}
	body := map[string]any{"first_name": " Amina ", "second_name": "Otieno", "national_id": "24681357"}
	for k, v := range base {
		body[k] = v
	}
	r := e.do(t, http.MethodPost, "/v1/auth/signup", "", body)
	if r.Code != http.StatusCreated {
		t.Fatalf("signup: %d %s", r.Code, r.Raw)
	}
	tok := r.Body["access_token"].(string)
	if a := e.do(t, http.MethodGet, "/v1/account", tok, nil); a.Body["first_name"] != "Amina" || a.Body["second_name"] != "Otieno" {
		t.Fatalf("account: %s", a.Raw)
	}
	k := e.do(t, http.MethodGet, "/v1/kyc", tok, nil)
	declared, _ := k.Body["declared"].(map[string]any)
	if k.Code != http.StatusOK || k.Body["status"] != "none" || declared["first_name"] != "Amina" || declared["national_id"] != "24681357" {
		t.Fatalf("kyc status: %s", k.Raw)
	}
	// the back office shows the names and ID number given at sign-up until the player submits
	pl := e.do(t, http.MethodGet, "/v1/staff/players", e.member(staff.Staff), nil)
	players, _ := pl.Body["players"].([]any)
	if len(players) != 1 || players[0].(map[string]any)["full_name"] != "Amina Otieno" || players[0].(map[string]any)["national_id"] != "24681357" {
		t.Fatalf("players: %s", pl.Raw)
	}
	// left out, the name and ID number are the ones from sign-up (and the date of birth too, ADR 0029; this one differs
	// from the declared date, so the given one counts)
	if s := e.do(t, http.MethodPost, "/v1/kyc", tok, map[string]any{"date_of_birth": "1992-03-04"}); s.Code != http.StatusAccepted {
		t.Fatalf("submit from sign-up: %d %s", s.Code, s.Raw)
	}
}

// undeclare removes an account's sign-up declaration, standing in for an account from before 2026-10-30 (the table is
// append-only for the application; the database owner lifts the trigger for the test).
func (e *env) undeclare(t *testing.T, userID string) {
	t.Helper()
	e.ownerExec(t, `ALTER TABLE kyc.declared_identities DISABLE TRIGGER declared_identities_append_only`)
	e.ownerExec(t, `DELETE FROM kyc.declared_identities WHERE user_id = $1`, userID)
	e.ownerExec(t, `ALTER TABLE kyc.declared_identities ENABLE TRIGGER declared_identities_append_only`)
}

// Owner, 2026-10-31: a player whose account has no names fills them in from their account, once.
func TestAPI_AccountWithoutNamesGivesThemOnceFromTheAccount(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	id := e.userID(t, s)
	// a sign-up of today declares: the account says so and refuses a second declaration
	r := e.do(t, http.MethodGet, "/v1/account", s.access, nil)
	identity, _ := r.Body["identity"].(map[string]any)
	if r.Code != http.StatusOK || identity["status"] != "declared" || identity["national_id_hint"] != "357" || r.Body["first_name"] != "Amina" {
		t.Fatalf("declared account: %d %s", r.Code, r.Raw)
	}
	body := map[string]any{"first_name": "Grace", "second_name": "Wanjiru", "national_id": "13579246"}
	if r := e.do(t, http.MethodPost, "/v1/account/identity", s.access, body); r.Code != http.StatusConflict || !hasType(r, "identity-declared") {
		t.Fatalf("second declaration: %d %s", r.Code, r.Raw)
	}

	// an older account: nothing on it, so the app asks
	e.undeclare(t, id)
	r = e.do(t, http.MethodGet, "/v1/account", s.access, nil)
	identity, _ = r.Body["identity"].(map[string]any)
	if identity["status"] != "none" || identity["national_id_hint"] != nil || r.Body["first_name"] != nil || r.Body["second_name"] != nil {
		t.Fatalf("account without a declaration: %s", r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/account/identity", s.access, map[string]any{"first_name": "G", "second_name": "Wanjiru", "national_id": "13579246"}); r.Code != http.StatusBadRequest || !hasType(r, "invalid-identity") {
		t.Fatalf("invalid declaration: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/account/identity", s.access, body); r.Code != http.StatusNoContent {
		t.Fatalf("declare: %d %s", r.Code, r.Raw)
	}
	r = e.do(t, http.MethodGet, "/v1/account", s.access, nil)
	identity, _ = r.Body["identity"].(map[string]any)
	if r.Body["first_name"] != "Grace" || r.Body["second_name"] != "Wanjiru" || identity["status"] != "declared" || identity["national_id_hint"] != "246" {
		t.Fatalf("account once declared: %s", r.Raw)
	}
	k := e.do(t, http.MethodGet, "/v1/kyc", s.access, nil)
	declared, _ := k.Body["declared"].(map[string]any)
	if declared["first_name"] != "Grace" || declared["second_name"] != "Wanjiru" || declared["national_id"] != "13579246" {
		t.Fatalf("kyc declared: %s", k.Raw)
	}
	pl := e.do(t, http.MethodGet, "/v1/staff/players", e.member(staff.Staff), nil)
	players, _ := pl.Body["players"].([]any)
	if len(players) != 1 || players[0].(map[string]any)["full_name"] != "Grace Wanjiru" || players[0].(map[string]any)["national_id"] != "13579246" {
		t.Fatalf("players: %s", pl.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/account/identity", s.access, body); r.Code != http.StatusConflict || !hasType(r, "identity-declared") {
		t.Fatalf("declaring twice: %d %s", r.Code, r.Raw)
	}
	var audited int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit.audit_log WHERE action = 'identity.declared' AND actor_id = $1 AND details::text NOT LIKE '%Grace%'`, id).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("declaration audited %d times without the names (%v), want once", audited, err)
	}

	// once an identity submission exists the name comes from it; nothing can be declared any more
	o := e.signIn(t)
	e.undeclare(t, e.userID(t, o))
	if r := e.do(t, http.MethodPost, "/v1/kyc", o.access, map[string]any{"national_id": "11223344", "full_name": "Peter Kamau Mwangi", "date_of_birth": "1990-01-02"}); r.Code != http.StatusAccepted {
		t.Fatalf("submit: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/account/identity", o.access, body); r.Code != http.StatusConflict || !hasType(r, "kyc-state") {
		t.Fatalf("declaring after a submission: %d %s", r.Code, r.Raw)
	}
	r = e.do(t, http.MethodGet, "/v1/account", o.access, nil)
	identity, _ = r.Body["identity"].(map[string]any)
	if r.Body["first_name"] != "Peter" || r.Body["second_name"] != "Kamau" || identity["status"] != "submitted" || identity["national_id_hint"] != "344" {
		t.Fatalf("account once submitted: %s", r.Raw)
	}
}

// Owner, 2026-10-31: the account lists the devices signed in and signs the others out.
func TestAPI_AccountListsSessionsAndSignsTheOtherDevicesOut(t *testing.T) {
	e := setup(t)
	first := e.signIn(t)
	e.clk.Advance(time.Minute)
	// the same account on a second device
	r := e.do(t, http.MethodPost, "/v1/auth/login", "", map[string]any{"msisdn": first.msisdn, "password": strongPassword,
		"device": map[string]any{"fingerprint": "fp-second", "platform": "web"}})
	if r.Code != http.StatusOK {
		t.Fatalf("second login: %d %s", r.Code, r.Raw)
	}
	second := session{access: r.Body["access_token"].(string), refresh: r.Body["refresh_token"].(string), fp: "fp-second"}

	r = e.do(t, http.MethodGet, "/v1/account/sessions", second.access, nil)
	list, _ := r.Body["sessions"].([]any)
	if r.Code != http.StatusOK || len(list) != 2 {
		t.Fatalf("sessions: %d %s", r.Code, r.Raw)
	}
	newest, oldest := list[0].(map[string]any), list[1].(map[string]any)
	if newest["current"] != true || newest["platform"] != "web" || oldest["current"] != false || oldest["platform"] != "android" ||
		oldest["device_verdict"] != "none" || newest["id"] == oldest["id"] || newest["created_at"] == nil || newest["last_used_at"] == nil {
		t.Fatalf("sessions, newest first with the caller's marked: %s", r.Raw)
	}
	// the other device sees itself as current, and a refresh moves its last use on
	e.clk.Advance(time.Minute)
	if r := e.do(t, http.MethodPost, "/v1/auth/refresh", "", map[string]any{"refresh_token": first.refresh, "device_fingerprint": first.fp}); r.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", r.Code, r.Raw)
	} else {
		first.refresh = r.Body["refresh_token"].(string)
		first.access = r.Body["access_token"].(string)
	}
	r = e.do(t, http.MethodGet, "/v1/account/sessions", first.access, nil)
	list, _ = r.Body["sessions"].([]any)
	if list[1].(map[string]any)["current"] != true || !sameInstant(list[1].(map[string]any)["last_used_at"], "2026-10-21T12:02:00Z") ||
		!sameInstant(list[1].(map[string]any)["created_at"], "2026-10-21T12:00:00Z") {
		t.Fatalf("from the first device: %s", r.Raw)
	}

	if r := e.do(t, http.MethodPost, "/v1/account/sessions/revoke-others", second.access, nil); r.Code != http.StatusNoContent {
		t.Fatalf("revoke others: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/auth/refresh", "", map[string]any{"refresh_token": first.refresh, "device_fingerprint": first.fp}); r.Code != http.StatusUnauthorized {
		t.Fatalf("the other device's refresh after revocation: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/kyc", first.access, nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("the other device's token after revocation: %d", r.Code)
	}
	r = e.do(t, http.MethodGet, "/v1/account/sessions", second.access, nil)
	list, _ = r.Body["sessions"].([]any)
	if r.Code != http.StatusOK || len(list) != 1 || list[0].(map[string]any)["current"] != true {
		t.Fatalf("the caller's session must stay: %d %s", r.Code, r.Raw)
	}
	var count string
	if err := e.pool.QueryRow(context.Background(), `SELECT details->>'count' FROM audit.audit_log WHERE action = 'auth.sessions_revoked' AND actor_type = 'player'`).Scan(&count); err != nil || count != "1" {
		t.Fatalf("audited count = %q (%v), want 1", count, err)
	}
	if r := e.do(t, http.MethodPost, "/v1/account/sessions/revoke-others", second.access, nil); r.Code != http.StatusNoContent {
		t.Fatalf("nothing else to revoke is still fine: %d %s", r.Code, r.Raw)
	}
}

// Owner, 2026-10-31: the account shows how it can be signed in to.
func TestAPI_AccountShowsItsSignInMethods(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	r := e.do(t, http.MethodGet, "/v1/account/sign-ins", s.access, nil)
	providers, _ := r.Body["providers"].([]any)
	if r.Code != http.StatusOK || r.Body["password"] != true || len(providers) != 0 {
		t.Fatalf("password account: %d %s", r.Code, r.Raw)
	}
	e.exec(t, `INSERT INTO identity.auth_identities (user_id, provider, subject, created_at) VALUES ($1, 'google', 'sub-1', '2026-10-20T10:00:00Z')`, e.userID(t, s))
	r = e.do(t, http.MethodGet, "/v1/account/sign-ins", s.access, nil)
	providers, _ = r.Body["providers"].([]any)
	if len(providers) != 1 || providers[0].(map[string]any)["provider"] != "google" || !sameInstant(providers[0].(map[string]any)["linked_at"], "2026-10-20T10:00:00Z") {
		t.Fatalf("linked google: %s", r.Raw)
	}
	other := e.signIn(t)
	if r := e.do(t, http.MethodGet, "/v1/account/sign-ins", other.access, nil); len(r.Body["providers"].([]any)) != 0 {
		t.Fatalf("another account's sign-ins: %s", r.Raw)
	}
}

// ADR 0029: every new account gives a date of birth and declares it is 18 or older; the server checks both before the
// account exists, and identity verification later starts from the declared date.
func TestAPI_SignUpNeedsADateOfBirthAndAnAdultDeclaration(t *testing.T) {
	e := setup(t)
	var msisdn string
	var tried []string
	base := func(extra map[string]any) map[string]any {
		msisdn = e.msisdn() // a number of its own each time: sign-ups per number are rate limited
		tried = append(tried, "254"+msisdn[1:])
		b := map[string]any{"first_name": "Amina", "second_name": "Otieno", "national_id": "24681357", "msisdn": msisdn, "email": e.email(),
			"password": strongPassword, "device": map[string]any{"fingerprint": "fp-age", "platform": "android"}}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	now, _ := e.clk.Now(context.Background(), nil)
	adult := now.AddDate(-18, 0, -2).Format(time.DateOnly)
	minor := now.AddDate(-18, 0, 2).Format(time.DateOnly)
	for name, tc := range map[string]struct {
		extra   map[string]any
		code    int
		problem string
	}{
		"nothing":              {nil, http.StatusBadRequest, "age-required"},
		"no declaration":       {map[string]any{"date_of_birth": adult}, http.StatusBadRequest, "age-required"},
		"declared false":       {map[string]any{"date_of_birth": adult, "adult_declaration": false}, http.StatusBadRequest, "age-required"},
		"declaration, no date": {map[string]any{"adult_declaration": true}, http.StatusBadRequest, "age-required"},
		"not a date":           {map[string]any{"date_of_birth": "17/05/1990", "adult_declaration": true}, http.StatusBadRequest, "invalid-date-of-birth"},
		"born in the future":   {map[string]any{"date_of_birth": "2999-01-01", "adult_declaration": true}, http.StatusBadRequest, "invalid-date-of-birth"},
		"two days short of 18": {map[string]any{"date_of_birth": minor, "adult_declaration": true}, http.StatusForbidden, "underage"},
	} {
		if r := e.do(t, http.MethodPost, "/v1/auth/signup", "", base(tc.extra)); r.Code != tc.code || !hasType(r, tc.problem) {
			t.Errorf("%s: %d %s, want %d %s", name, r.Code, r.Raw, tc.code, tc.problem)
		}
	}
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.users WHERE msisdn = ANY($1)`, tried).Scan(&n); err != nil || n != 0 {
		t.Fatalf("refused sign-ups left %d accounts (%v)", n, err)
	}
	if r := e.do(t, http.MethodPost, "/v1/auth/sso/google/signup", "", map[string]any{"first_name": "Amina", "second_name": "Otieno", "national_id": "24681357",
		"id_token": "ok:g-age", "nonce": "n-age", "msisdn": e.msisdn(), "device": map[string]any{"fingerprint": "fp-age-sso", "platform": "android"}}); r.Code != http.StatusBadRequest || !hasType(r, "age-required") {
		t.Fatalf("sso sign-up without age: %d %s", r.Code, r.Raw)
	}

	r := e.do(t, http.MethodPost, "/v1/auth/signup", "", base(map[string]any{"date_of_birth": adult, "adult_declaration": true}))
	if r.Code != http.StatusCreated {
		t.Fatalf("adult sign-up: %d %s", r.Code, r.Raw)
	}
	tok := r.Body["access_token"].(string)
	var stored []byte
	if err := e.pool.QueryRow(context.Background(), `SELECT a.date_of_birth_enc FROM kyc.age_declarations a JOIN identity.users u ON u.id = a.user_id WHERE u.msisdn = $1`,
		"254"+msisdn[1:]).Scan(&stored); err != nil || len(stored) == 0 || strings.Contains(string(stored), adult) {
		t.Fatalf("declaration stored %q (%v): want it, encrypted", stored, err)
	}
	// identity verification starts from the declared date when the player leaves it out
	if k := e.do(t, http.MethodPost, "/v1/kyc", tok, map[string]any{"national_id": "24681357", "full_name": "Amina Otieno"}); k.Code != http.StatusAccepted {
		t.Fatalf("kyc without a date after declaring one: %d %s", k.Code, k.Raw)
	}
}
