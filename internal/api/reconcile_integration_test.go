//go:build integration

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/itsivali/4draws-lottery-platform/internal/staff"
)

// Owner, 2026-10-31: a top-up M-Pesa completed but the platform never credited is reconciled from the receipt: the
// player gives it from their M-Pesa message, staff see everything about the top-up and credit, recheck or dismiss it,
// and the M-Pesa statement is matched and applied from the back office.
func TestAPI_APlayerGivesTheReceiptForAnUncreditedTopUpAndStaffReconcileIt(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	e.verified(t, s)
	started := e.do(t, http.MethodPost, "/v1/payments/topups", s.access, map[string]any{"amount_kes": 50})
	if started.Code != http.StatusAccepted {
		t.Fatalf("top-up: %d %s", started.Code, started.Raw)
	}
	id := started.Body["id"].(string)

	// while it is still settling the receipt is not taken
	if r := e.do(t, http.MethodPost, "/v1/payments/"+id+"/receipt", s.access, map[string]any{"receipt": "QJ7XK2M9PL"}); r.Code != http.StatusConflict || !hasType(r, "still-settling") {
		t.Fatalf("too early: %d %s", r.Code, r.Raw)
	}
	// the prompt was approved but the callback never came and the status query gave up: unconfirmed
	e.ownerExec(t, `UPDATE payments.payments SET status = 'unconfirmed', failure_reason = 'paid_without_callback', checkout_request_id = coalesce(checkout_request_id, 'ws_CO_test_'||id::text) WHERE id = $1`, id)
	if r := e.do(t, http.MethodGet, "/v1/payments/"+id, s.access, nil); r.Body["status"] != "unconfirmed" || r.Body["receipt_claim"] != nil {
		t.Fatalf("before the claim: %s", r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/payments/"+id+"/receipt", s.access, map[string]any{"receipt": "not-one"}); r.Code != http.StatusBadRequest || !hasType(r, "invalid-receipt") {
		t.Fatalf("malformed: %d %s", r.Code, r.Raw)
	}
	claim := e.do(t, http.MethodPost, "/v1/payments/"+id+"/receipt", s.access, map[string]any{"receipt": " qj7xk2m9pl "})
	if claim.Code != http.StatusAccepted || claim.Body["receipt"] != "QJ7XK2M9PL" || claim.Body["status"] != "claimed" {
		t.Fatalf("claim: %d %s", claim.Code, claim.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/payments/"+id+"/receipt", s.access, map[string]any{"receipt": "QJ7XK2M9PL"}); r.Code != http.StatusAccepted {
		t.Fatalf("the same receipt again is fine: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/payments/"+id+"/receipt", s.access, map[string]any{"receipt": "ZZ00000000"}); r.Code != http.StatusConflict || !hasType(r, "claim-open") {
		t.Fatalf("another receipt while one is open: %d %s", r.Code, r.Raw)
	}
	r := e.do(t, http.MethodGet, "/v1/payments/"+id, s.access, nil)
	rc, _ := r.Body["receipt_claim"].(map[string]any)
	if rc["receipt"] != "QJ7XK2M9PL" || (rc["status"] != "claimed" && rc["status"] != "checking") {
		t.Fatalf("player sees %s", r.Raw)
	}
	other := e.signIn(t)
	if r := e.do(t, http.MethodPost, "/v1/payments/"+id+"/receipt", other.access, map[string]any{"receipt": "QJ7XK2M9PL"}); r.Code != http.StatusNotFound {
		t.Fatalf("another player's top-up: %d", r.Code)
	}

	// staff: the list filters, the detail, then the credit from the receipt
	tok := e.member(staff.Staff)
	if r := e.do(t, http.MethodGet, "/v1/staff/topups?claimed=true", tok, nil); r.Code != http.StatusOK || len(r.Body["topups"].([]any)) != 1 ||
		r.Body["topups"].([]any)[0].(map[string]any)["claimed_receipt"] != "QJ7XK2M9PL" {
		t.Fatalf("claimed filter: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/topups?reason=paid_without_callback&channel=stk&from=2026-10-01&to=2026-12-31", tok, nil); r.Code != http.StatusOK || len(r.Body["topups"].([]any)) != 1 {
		t.Fatalf("reason/channel/date filters: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/topups?from=2026-12-31&to=2026-10-01", tok, nil); r.Code != http.StatusBadRequest || !hasType(r, "invalid-range") {
		t.Fatalf("reversed range: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/topups?channel=mpesa", tok, nil); r.Code != http.StatusBadRequest || !hasType(r, "invalid-channel") {
		t.Fatalf("bad channel: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/topups?q="+id[:8], tok, nil); r.Code != http.StatusOK || len(r.Body["topups"].([]any)) != 1 {
		t.Fatalf("search by payment id: %d %s", r.Code, r.Raw)
	}
	d := e.do(t, http.MethodGet, "/v1/staff/topups/"+id, tok, nil)
	dc, _ := d.Body["claim"].(map[string]any)
	player, _ := d.Body["player"].(map[string]any)
	if d.Code != http.StatusOK || d.Body["status"] != "unconfirmed" || d.Body["full_name"] != "Amina Otieno" || dc["receipt"] != "QJ7XK2M9PL" ||
		d.Body["checkout_request_id"] == nil || player["balance_kes"] != float64(0) || player["kyc_status"] != "verified" {
		t.Fatalf("detail: %d %s", d.Code, d.Raw)
	}
	var claimed bool
	for _, ev := range d.Body["events"].([]any) {
		if ev.(map[string]any)["action"] == "payment.topup.receipt_claimed" {
			claimed = true
		}
	}
	if !claimed {
		t.Fatalf("the detail's audit trail lacks the claim: %s", d.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/topups/nope", tok, nil); r.Code != http.StatusNotFound {
		t.Fatalf("unknown top-up: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/topups/"+id+"/resolve", tok, map[string]any{"receipt": "QJ7XK2M9PL", "amount_kes": 50, "note": "on the 31 Oct statement"}); r.Code != http.StatusNoContent {
		t.Fatalf("resolve: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/wallet", s.access, nil); r.Body["balance_kes"] != float64(50) {
		t.Fatalf("wallet after the credit: %s", r.Raw)
	}
	r = e.do(t, http.MethodGet, "/v1/payments/"+id, s.access, nil)
	rc, _ = r.Body["receipt_claim"].(map[string]any)
	if r.Body["status"] != "credited" || rc["status"] != "confirmed" {
		t.Fatalf("player after the credit: %s", r.Raw)
	}
	for p, body := range map[string]any{"/resolve": map[string]any{"receipt": "QJ7XK2M9PL", "amount_kes": 50}, "/recheck": nil, "/dismiss": map[string]any{"note": "again"}} {
		if r := e.do(t, http.MethodPost, "/v1/staff/topups/"+id+p, tok, body); r.Code != http.StatusConflict || !hasType(r, "state") {
			t.Fatalf("%s of a credited top-up: %d %s", p, r.Code, r.Raw)
		}
	}

	// a second unconfirmed top-up, dismissed as not on the statement
	second := e.do(t, http.MethodPost, "/v1/payments/topups", s.access, map[string]any{"amount_kes": 20})
	id2 := second.Body["id"].(string)
	e.ownerExec(t, `UPDATE payments.payments SET status = 'unconfirmed', failure_reason = 'still_pending_at_expiry' WHERE id = $1`, id2)
	if r := e.do(t, http.MethodPost, "/v1/staff/topups/"+id2+"/dismiss", tok, map[string]any{"note": "   "}); r.Code != http.StatusBadRequest || !hasType(r, "reason-required") {
		t.Fatalf("dismiss without a note: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/topups/"+id2+"/dismiss", tok, map[string]any{"note": "nothing on the statement"}); r.Code != http.StatusNoContent {
		t.Fatalf("dismiss: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/payments/"+id2, s.access, nil); r.Body["status"] != "failed" {
		t.Fatalf("dismissed top-up: %s", r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/topups?status=failed&reason=not_on_statement", tok, nil); len(r.Body["topups"].([]any)) != 1 {
		t.Fatalf("dismissed in the list: %s", r.Raw)
	}

	// the statement: the credited receipt is recognised, an unknown one is not; nothing is credited by matching
	csv := "Receipt No.,Completion Time,Details,Transaction Status,Paid In,Withdrawn,Other Party Info,A/C No.\n" +
		"QJ7XK2M9PL,2026-10-21 15:00:00,Pay Bill,Completed,50.00,,254712***001 - AMINA OTIENO,4draws\n" +
		"XY12345678,2026-10-21 15:05:00,Pay Bill,Completed,999.00,,254700***000 - NOBODY,4draws\n"
	m := e.do(t, http.MethodPost, "/v1/staff/payments/statement/match", tok, csv)
	rep, _ := m.Body["report"].(map[string]any)
	counts, _ := rep["counts"].(map[string]any)
	if m.Code != http.StatusOK || rep["rows"] != float64(2) || counts["credited"] != float64(1) || counts["unmatched"] != float64(1) || len(rep["to_credit"].([]any)) != 0 {
		t.Fatalf("match: %d %s", m.Code, m.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/payments/statement/match", tok, "no,header\n1,2\n"); r.Code != http.StatusBadRequest || !hasType(r, "statement-empty") {
		t.Fatalf("unreadable statement: %d %s", r.Code, r.Raw)
	}
	a := e.do(t, http.MethodPost, "/v1/staff/payments/statement/apply", tok, map[string]any{"filename": "oct.csv", "sha256": rep["sha256"], "rows": 2, "credits": []any{}, "report": rep})
	if a.Code != http.StatusOK || a.Body["credited"] != float64(0) || a.Body["import_id"] == nil {
		t.Fatalf("apply: %d %s", a.Code, a.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/payments/statements", tok, nil); r.Code != http.StatusOK || len(r.Body["imports"].([]any)) != 1 ||
		r.Body["imports"].([]any)[0].(map[string]any)["filename"] != "oct.csv" {
		t.Fatalf("imports: %d %s", r.Code, r.Raw)
	}
	if !strings.Contains(e.do(t, http.MethodGet, "/v1/staff/system", tok, nil).Raw, `"receipt_claims":0`) {
		t.Fatal("the system page counts open receipt claims")
	}
}
