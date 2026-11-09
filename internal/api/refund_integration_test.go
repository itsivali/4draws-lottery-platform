//go:build integration

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/itsivali/lottery-platform/internal/staff"
)

// ADR 0031: an admin asks for a refund to source with a reason; the refusals come back as problems the back office
// can show.
func TestStaffAPI_RefundToSourceNeedsAReasonAndABalance(t *testing.T) {
	e := setup(t)
	p := e.signIn(t)
	pr, err := e.deps.Identity.Authenticate(context.Background(), p.access)
	if err != nil {
		t.Fatal(err)
	}
	admin := e.member(staff.Admin)
	path := "/v1/staff/players/" + pr.UserID + "/refund"
	if r := e.do(t, http.MethodPost, path, admin, map[string]any{"reason": ""}); r.Code != http.StatusBadRequest {
		t.Fatalf("without a reason: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, path, admin, map[string]any{"reason": "identity refused"}); r.Code != http.StatusConflict || r.Body["type"] == nil ||
		r.Body["title"] != "The wallet holds nothing to refund" {
		t.Fatalf("an empty wallet: %d %s", r.Code, r.Raw)
	}
}
