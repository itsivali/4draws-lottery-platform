//go:build integration

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/itsivali/lottery-platform/internal/staff"
)

// ADR 0033: staff open a case, hold the player's money, and someone else lifts it by closing the case; while held the
// player is refused top-ups without being told why.
func TestStaffAPI_MonitoringCasesHoldAndRelease(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	p := e.signIn(t)
	pr, err := e.deps.Identity.Authenticate(ctx, p.access)
	if err != nil {
		t.Fatal(err)
	}
	alice, bob := e.member(staff.Staff), e.member(staff.Staff)
	if r := e.do(t, http.MethodGet, "/v1/staff/aml/rules", alice, nil); r.Code != http.StatusOK || len(r.Body["rules"].([]any)) != 5 ||
		r.Body["rules"].([]any)[0].(map[string]any)["pending_approval"] != true {
		t.Fatalf("rules: %d %s", r.Code, r.Raw)
	}
	r := e.do(t, http.MethodPost, "/v1/staff/aml/cases", alice, map[string]any{"user_id": pr.UserID, "note": "the bank called"})
	if r.Code != http.StatusCreated || r.Body["rule"] != "manual" {
		t.Fatalf("open: %d %s", r.Code, r.Raw)
	}
	id := r.Body["id"].(string)
	if r := e.do(t, http.MethodPost, "/v1/staff/aml/cases/"+id+"/hold", alice, map[string]any{"note": ""}); r.Code != http.StatusBadRequest {
		t.Fatalf("hold without a note: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/aml/cases/"+id+"/hold", alice, map[string]any{"note": "until the bank answers"}); r.Code != http.StatusOK || r.Body["hold"] != true {
		t.Fatalf("hold: %d %s", r.Code, r.Raw)
	}
	top := e.do(t, http.MethodPost, "/v1/payments/topups", p.access, map[string]any{"amount_kes": 100})
	if top.Code != http.StatusForbidden || top.Body["title"] != "This isn't available on your account right now" {
		t.Fatalf("top-up while held: %d %s", top.Code, top.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/aml/cases?view=held", alice, nil); r.Code != http.StatusOK || len(r.Body["cases"].([]any)) != 1 {
		t.Fatalf("held view: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/aml/cases/"+id+"/close", alice, map[string]any{"note": "fine"}); r.Code != http.StatusConflict || r.Body["title"] == nil {
		t.Fatalf("closed by who held it: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/aml/cases/"+id+"/close", bob, map[string]any{"note": "the bank confirmed the source; not reported"}); r.Code != http.StatusOK || r.Body["status"] != "closed" {
		t.Fatalf("close: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/aml/cases/"+id, alice, nil); r.Code != http.StatusOK || len(r.Body["events"].([]any)) != 3 {
		t.Fatalf("case: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/aml/cases/not-a-case", alice, nil); r.Code != http.StatusNotFound {
		t.Fatalf("unknown case: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/aml/cases?view=everything", alice, nil); r.Code != http.StatusBadRequest {
		t.Fatalf("unknown view: %d %s", r.Code, r.Raw)
	}
}

// ADR 0036: an admin records a rule's setting from the back office: its thresholds checked, a note required, and the
// compliance owner's approval reference ending "pending approval".
func TestStaffAPI_AnAdminSetsARuleAndRecordsItsApproval(t *testing.T) {
	e := setup(t)
	admin := e.member(staff.Admin)
	set := func(body map[string]any) resp {
		return e.do(t, http.MethodPut, "/v1/staff/aml/rules/failed_topups", admin, body)
	}
	if r := set(map[string]any{"enabled": true, "params": map[string]any{"window_hours": 24, "min_failed": 0, "quiet_days": 7}, "note": "tighter"}); r.Code != http.StatusBadRequest || !hasType(r, "invalid-rule") {
		t.Fatalf("a zero threshold: %d %s", r.Code, r.Raw)
	}
	if r := set(map[string]any{"enabled": true, "params": map[string]any{"window_hours": 24, "min_failed": 3, "quiet_days": 7}, "note": ""}); r.Code != http.StatusBadRequest {
		t.Fatalf("no note: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/staff/aml/rules/no_such_rule", admin, map[string]any{"enabled": true, "params": map[string]any{"quiet_days": 7}, "note": "x"}); r.Code != http.StatusBadRequest {
		t.Fatalf("unknown rule: %d %s", r.Code, r.Raw)
	}
	r := set(map[string]any{"enabled": false, "params": map[string]any{"window_hours": 12, "min_failed": 3, "quiet_days": 7},
		"approval_reference": "CO-2026-11", "note": "compliance owner's memo of 11 November"})
	if r.Code != http.StatusOK {
		t.Fatalf("set: %d %s", r.Code, r.Raw)
	}
	var got map[string]any
	for _, x := range r.Body["rules"].([]any) {
		if x.(map[string]any)["rule"] == "failed_topups" {
			got = x.(map[string]any)
		}
	}
	params, _ := got["params"].(map[string]any)
	if got["enabled"] != false || got["pending_approval"] != false || got["approval_reference"] != "CO-2026-11" || params["window_hours"] != float64(12) {
		t.Fatalf("the rule after: %v", got)
	}
}
