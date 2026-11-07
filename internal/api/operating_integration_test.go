//go:build integration

package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/itsivali/lottery-platform/internal/staff"
)

// ADR 0027: GET /v1/status tells the apps whether money moves; a stop by any staff member refuses every money path
// with operations-stopped until an admin resumes; going live is not something a resume can do.
func TestAPI_AStopRefusesEveryMoneyPathUntilAnAdminResumes(t *testing.T) {
	e := setup(t)
	if r := e.do(t, http.MethodGet, "/v1/status", "", nil); r.Code != http.StatusOK || r.Body["operating_mode"] != "sandbox" ||
		r.Body["real_money"] != false || r.Body["money_moves"] != true {
		t.Fatalf("status: %d %s", r.Code, r.Raw)
	}
	p := e.signIn(t)
	e.canPlay(t, p, 500)
	// a withdrawal before the stop: its payout waits at Send while operations are stopped
	before := e.do(t, http.MethodPost, "/v1/payouts/withdrawals", p.access, map[string]any{"amount_kes": 100})
	if before.Code != http.StatusCreated {
		t.Fatalf("withdrawal before the stop: %d %s", before.Code, before.Raw)
	}
	payoutID, _ := before.Body["id"].(string)

	staffer, admin := e.member(staff.Staff), e.member(staff.Admin)
	if r := e.do(t, http.MethodPost, "/v1/staff/operating/stop", staffer, map[string]any{"reason": ""}); r.Code != http.StatusBadRequest {
		t.Fatalf("stop without a reason: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/operating/stop", staffer, map[string]any{"reason": "suspected fraud on top-ups"}); r.Code != http.StatusOK || r.Body["mode"] != "disabled" {
		t.Fatalf("stop: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/status", "", nil); r.Body["operating_mode"] != "disabled" || r.Body["money_moves"] != false {
		t.Fatalf("status after stop: %s", r.Raw)
	}
	for name, call := range map[string]func() resp{
		"top-up": func() resp {
			return e.do(t, http.MethodPost, "/v1/payments/topups", p.access, map[string]any{"amount_kes": 50})
		},
		"play": func() resp { return e.do(t, http.MethodPost, "/v1/plays", p.access, nil) },
		"withdrawal": func() resp {
			return e.do(t, http.MethodPost, "/v1/payouts/withdrawals", p.access, map[string]any{"amount_kes": 100})
		},
	} {
		if r := call(); r.Code != http.StatusServiceUnavailable || !hasType(r, "operations-stopped") {
			t.Errorf("%s while stopped: %d %s", name, r.Code, r.Raw)
		}
	}
	if err := e.deps.StaffServices.Payouts.Send(context.Background(), payoutID); err != nil { // the worker's role sends payouts
		t.Fatalf("send while stopped: %v", err)
	}
	var status string
	if err := e.pool.QueryRow(context.Background(), `SELECT status FROM payouts.payouts WHERE id = $1`, payoutID).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("a payout left while stopped: status %q %v", status, err)
	}
	// the back office shows the stop; staff cannot lift it, and nobody can resume straight to live
	if r := e.do(t, http.MethodGet, "/v1/staff/operating", staffer, nil); r.Code != http.StatusOK || !strings.Contains(r.Raw, "suspected fraud on top-ups") {
		t.Fatalf("operating page: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/operating/resume", staffer, map[string]any{"mode": "sandbox", "reason": "ok"}); r.Code != http.StatusForbidden {
		t.Fatalf("staff resume: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/operating/resume", admin, map[string]any{"mode": "live", "reason": "ok"}); r.Code != http.StatusBadRequest || !hasType(r, "live-needs-approval") {
		t.Fatalf("resume to live: %d %s", r.Code, r.Raw)
	}
	// this service is configured for sandbox: a live request is refused outright
	if r := e.do(t, http.MethodPost, "/v1/staff/operating/live-requests", admin, map[string]any{"licence_reference": "GRA/OPL/1",
		"approval_reference": "A1", "evidence_sha256": strings.Repeat("ab", 32), "evidence_location": "vault", "reason": "licensed"}); r.Code != http.StatusConflict || !hasType(r, "not-configured") {
		t.Fatalf("live request on a sandbox service: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/operating/resume", admin, map[string]any{"mode": "sandbox", "reason": "a test payment, not fraud"}); r.Code != http.StatusOK {
		t.Fatalf("admin resume: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/plays", p.access, nil); r.Code != http.StatusCreated {
		t.Fatalf("play after resume: %d %s", r.Code, r.Raw)
	}
}
