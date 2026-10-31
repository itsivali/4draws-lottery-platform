//go:build integration

package api_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/itsivali/lottery-platform/internal/alerts"
	"github.com/itsivali/lottery-platform/internal/staff"
)

// The Alerts page filters, pages and opens alerts (owner, 2026-10-31: "expand the alerts system").
func TestStaffAPI_AlertsAreListedByStateKindAndSeverityAndOpenedWithTheirHistory(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tok := e.member(staff.Staff)
	// nothing yet: an empty page still names every kind the watch can raise
	r := e.do(t, http.MethodGet, "/v1/staff/alerts", tok, nil)
	if r.Code != http.StatusOK || len(r.Body["alerts"].([]any)) != 0 || r.Body["next_before"] != nil || len(r.Body["kinds"].([]any)) < 10 {
		t.Fatalf("empty: %d %s", r.Code, r.Raw)
	}
	// a stuck top-up and a day-old orphan callback: the watch raises a page and a warning
	player := e.signIn(t)
	at, _ := e.clk.Now(ctx, nil)
	if _, err := e.pool.Exec(ctx, `INSERT INTO payments.payments (user_id, channel, status, amount_requested_kes, msisdn, idempotency_key, created_at, updated_at)
		SELECT id, 'stk', 'stk_sent', 20, msisdn, 'stuck', $2::timestamptz - interval '1 hour', $2::timestamptz - interval '1 hour' FROM identity.users WHERE msisdn = $1`,
		player.msisdn, at); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO payments.stk_orphan_callbacks (checkout_request_id, raw_payload, received_at) VALUES ('ws_CO_9', '{}', $1)`, at); err != nil {
		t.Fatal(err)
	}
	if err := e.deps.StaffServices.Alerts.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	r = e.do(t, http.MethodGet, "/v1/staff/alerts", tok, nil)
	if r.Code != http.StatusOK || len(r.Body["alerts"].([]any)) != 2 {
		t.Fatalf("open: %d %s", r.Code, r.Raw)
	}
	open := map[string]int{}
	for _, k := range r.Body["kinds"].([]any) {
		kc := k.(map[string]any)
		open[kc["kind"].(string)] = int(kc["open"].(float64))
	}
	if open[alerts.KindTopUpOverduePending] != 1 || open[alerts.KindTopUpOrphanCallbacks] != 1 || open[alerts.KindSuspenseOpen] != 0 {
		t.Fatalf("kinds %v", open)
	}
	for name, tc := range map[string]struct {
		q    string
		want int
	}{
		"pages":            {"severity=page", 1},
		"kind":             {"kind=" + alerts.KindTopUpOrphanCallbacks, 1},
		"all legacy":       {"all=true", 2},
		"resolved":         {"state=resolved", 0},
		"acknowledged":     {"state=acknowledged", 0},
		"page of one":      {"limit=1", 1},
		"kind with nobody": {"kind=kyc.queue_stale", 0},
	} {
		r := e.do(t, http.MethodGet, "/v1/staff/alerts?"+tc.q, tok, nil)
		if r.Code != http.StatusOK || len(r.Body["alerts"].([]any)) != tc.want {
			t.Errorf("%s: %d %s", name, r.Code, r.Raw)
		}
		if name == "page of one" {
			next, _ := r.Body["next_before"].(string)
			if next == "" {
				t.Fatalf("no cursor: %s", r.Raw)
			}
			r2 := e.do(t, http.MethodGet, "/v1/staff/alerts?limit=1&before="+url.QueryEscape(next), tok, nil)
			if r2.Code != http.StatusOK || len(r2.Body["alerts"].([]any)) != 1 || r2.Body["next_before"] != nil {
				t.Errorf("second page: %d %s", r2.Code, r2.Raw)
			}
		}
	}
	for name, q := range map[string]string{"state": "state=closed", "severity": "severity=loud", "limit": "limit=201", "cursor": "before=yesterday"} {
		if r := e.do(t, http.MethodGet, "/v1/staff/alerts?"+q, tok, nil); r.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, r.Code, r.Raw)
		}
	}
	// open the page, acknowledge it, and its history says so
	r = e.do(t, http.MethodGet, "/v1/staff/alerts?severity=page", tok, nil)
	page := r.Body["alerts"].([]any)[0].(map[string]any)
	id := page["id"].(string)
	r = e.do(t, http.MethodGet, "/v1/staff/alerts/"+id, tok, nil)
	if r.Code != http.StatusOK || r.Body["check"] != "topups" || r.Body["kind"] != alerts.KindTopUpOverduePending || len(r.Body["history"].([]any)) != 1 {
		t.Fatalf("detail: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/alerts/"+id+"/acknowledge", tok, map[string]any{"note": "the worker was down; restarted"}); r.Code != http.StatusOK {
		t.Fatalf("ack: %d %s", r.Code, r.Raw)
	}
	r = e.do(t, http.MethodGet, "/v1/staff/alerts/"+id, tok, nil)
	history := r.Body["history"].([]any)
	if r.Code != http.StatusOK || len(history) != 2 || history[1].(map[string]any)["event"] != "alert.acknowledged" || history[1].(map[string]any)["note"] != "the worker was down; restarted" {
		t.Fatalf("history: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/alerts?state=acknowledged", tok, nil); r.Code != http.StatusOK || len(r.Body["alerts"].([]any)) != 1 {
		t.Fatalf("acknowledged: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/alerts/0192f000-0000-7000-8000-000000000001", tok, nil); r.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d %s", r.Code, r.Raw)
	}
}
