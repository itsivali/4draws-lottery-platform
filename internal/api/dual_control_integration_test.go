//go:build integration

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/itsivali/4draws-lottery-platform/internal/staff"
)

// ADR 0032: staff record a self-exclusion a player asks for, with a reason; it then refuses top-ups like the player's
// own, and the audit names the staff member. Nothing lifts it.
func TestStaffAPI_StaffRecordAPlayersSelfExclusion(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	p := e.signIn(t)
	pr, err := e.deps.Identity.Authenticate(ctx, p.access)
	if err != nil {
		t.Fatal(err)
	}
	op := e.member(staff.Staff)
	path := "/v1/staff/players/" + pr.UserID + "/self-exclusion"
	if r := e.do(t, http.MethodPost, path, op, map[string]any{"term": "6m"}); r.Code != http.StatusBadRequest {
		t.Fatalf("without a reason: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, path, op, map[string]any{"term": "forever", "reason": "asked by phone"}); r.Code != http.StatusBadRequest {
		t.Fatalf("an unknown term: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/players/not-a-player/self-exclusion", op, map[string]any{"term": "6m", "reason": "asked by phone"}); r.Code != http.StatusNotFound {
		t.Fatalf("an unknown player: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, path, op, map[string]any{"term": "1y", "reason": "asked by phone, caller verified"}); r.Code != http.StatusOK {
		t.Fatalf("self-exclusion: %d %s", r.Code, r.Raw)
	}
	var actorType, actor, reason string
	if err := e.pool.QueryRow(ctx, `SELECT actor_type, actor_id, details->>'reason' FROM audit.audit_log WHERE action = 'rg.self_exclusion' AND entity_id = $1`,
		pr.UserID).Scan(&actorType, &actor, &reason); err != nil {
		t.Fatal(err)
	}
	if actorType != "staff" || actor == pr.UserID || reason != "asked by phone, caller verified" {
		t.Fatalf("audited as %s %s %q", actorType, actor, reason)
	}
	if r := e.do(t, http.MethodGet, "/v1/account", p.access, nil); r.Code == http.StatusOK {
		if rest, _ := r.Body["restriction"].(map[string]any); rest == nil || rest["kind"] != "self_exclusion" {
			t.Fatalf("account restriction: %s", r.Raw)
		}
	}
}
