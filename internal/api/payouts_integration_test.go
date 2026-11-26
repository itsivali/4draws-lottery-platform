//go:build integration

package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/payouts"
)

// payoutQueue records payout jobs alongside the payment ones; the API tests only check what was enqueued.
type payoutQueue struct{ q *payQueue }

func (p payoutQueue) EnqueueSend(_ context.Context, _ pgx.Tx, id string) error {
	return p.q.add("payout-send:" + id)
}
func (p payoutQueue) EnqueueVerify(_ context.Context, _ pgx.Tx, id string, src payouts.Source) error {
	return p.q.add("payout-verify:" + id + ":" + string(src))
}

func TestAPI_WithdrawalJourneyAndOwnership(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	e.canPlay(t, s, 1000) // 950
	r := e.doKey(t, http.MethodPost, "/v1/payouts/withdrawals", s.access, "w-1", map[string]any{"amount_kes": 400})
	if r.Code != http.StatusCreated || r.Body["status"] != "processing" || r.Body["amount_kes"] != float64(400) ||
		r.Body["net_kes"] != float64(400) || r.Body["receipt"] != nil {
		t.Fatalf("withdraw: %d %s", r.Code, r.Raw)
	}
	id := r.Body["id"].(string)
	if again := e.doKey(t, http.MethodPost, "/v1/payouts/withdrawals", s.access, "w-1", map[string]any{"amount_kes": 400}); again.Code != http.StatusCreated ||
		again.Body["id"] != id {
		t.Fatalf("replay: %d %s", again.Code, again.Raw)
	}
	e.pay.mu.Lock()
	jobs := strings.Join(e.pay.jobs, ",")
	e.pay.mu.Unlock()
	if !strings.Contains(jobs, "payout-send:"+id) {
		t.Fatalf("send not queued: %v", jobs)
	}
	if got := e.do(t, http.MethodGet, "/v1/payouts/"+id, s.access, nil); got.Code != http.StatusOK || got.Body["id"] != id {
		t.Fatalf("get: %d %s", got.Code, got.Raw)
	}
	if list := e.do(t, http.MethodGet, "/v1/payouts", s.access, nil); list.Code != http.StatusOK || len(list.Body["payouts"].([]any)) != 1 {
		t.Fatalf("list: %d %s", list.Code, list.Raw)
	}
	other := e.signIn(t)
	if got := e.do(t, http.MethodGet, "/v1/payouts/"+id, other.access, nil); got.Code != http.StatusNotFound {
		t.Fatalf("another player saw it: %d %s", got.Code, got.Raw)
	}
	if list := e.do(t, http.MethodGet, "/v1/payouts", other.access, nil); len(list.Body["payouts"].([]any)) != 0 {
		t.Fatalf("another player's list: %s", list.Raw)
	}
}

func TestAPI_WithdrawalErrorMapping(t *testing.T) {
	e := setup(t)
	noKYC := e.signIn(t)
	if r := e.do(t, http.MethodPost, "/v1/payouts/withdrawals", noKYC.access, map[string]any{"amount_kes": 100}); r.Code != http.StatusForbidden || !hasType(r, "kyc-required") {
		t.Fatalf("no kyc: %d %s", r.Code, r.Raw)
	}
	noPhone := e.signIn(t)
	e.verified(t, noPhone)
	if r := e.do(t, http.MethodPost, "/v1/payouts/withdrawals", noPhone.access, map[string]any{"amount_kes": 100}); r.Code != http.StatusForbidden || !hasType(r, "phone-not-verified") {
		t.Fatalf("no phone: %d %s", r.Code, r.Raw)
	}
	s := e.signIn(t)
	e.canPlay(t, s, 1000)
	for _, tc := range []struct {
		body any
		code int
		slug string
	}{
		{map[string]any{"amount_kes": 10}, http.StatusBadRequest, "invalid-amount"},
		{map[string]any{"amount_kes": 5000}, http.StatusConflict, "insufficient-funds"},
	} {
		if r := e.do(t, http.MethodPost, "/v1/payouts/withdrawals", s.access, tc.body); r.Code != tc.code || !hasType(r, tc.slug) {
			t.Errorf("%v: %d %s", tc.body, r.Code, r.Raw)
		}
	}
	for range 3 {
		if r := e.do(t, http.MethodPost, "/v1/payouts/withdrawals", s.access, map[string]any{"amount_kes": 50}); r.Code != http.StatusCreated {
			t.Fatalf("withdraw: %d %s", r.Code, r.Raw)
		}
	}
	if r := e.do(t, http.MethodPost, "/v1/payouts/withdrawals", s.access, map[string]any{"amount_kes": 50}); r.Code != http.StatusConflict || !hasType(r, "daily-withdrawal-limit") {
		t.Fatalf("fourth: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/payouts/not-a-uuid", s.access, nil); r.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d %s", r.Code, r.Raw)
	}
}

func TestAPI_B2CHooksNeedTheSecretPathAndAreStored(t *testing.T) {
	e := setup(t)
	result := `{"Result":{"ResultType":0,"ResultCode":2001,"ResultDesc":"x","OriginatorConversationID":"nobody-000001","ConversationID":"AG_1"}}`
	for _, seg := range []string{"result", "timeout", "status-result", "status-timeout"} {
		if code, _ := e.hook(t, "/hooks/b2c/wrong-path/"+seg, result); code != http.StatusNotFound {
			t.Errorf("%s with a wrong path: %d", seg, code)
		}
		if code, body := e.hook(t, "/hooks/b2c/"+callbackPath+"/"+seg, result); code != http.StatusOK || !strings.Contains(body, `"ResultCode":0`) {
			t.Errorf("%s: %d %s", seg, code, body)
		}
	}
	if code, _ := e.hook(t, "/hooks/b2c/"+callbackPath+"/result", "nope"); code != http.StatusBadRequest {
		t.Errorf("malformed: %d", code)
	}
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM payouts.b2c_callbacks`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("stored %d %v", n, err)
	}
}
