//go:build integration

package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/payments"
	"github.com/itsivali/lottery-platform/internal/payouts"
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
