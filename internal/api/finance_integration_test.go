//go:build integration

package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/itsivali/4draws-lottery-platform/internal/staff"
)

// seedJournal posts one balanced journal straight into the ledger at an instant of the test's choosing.
func seedJournal(t *testing.T, e *env, kind, debit, credit string, amount int64, at string) string {
	t.Helper()
	var id string
	err := e.pool.QueryRow(context.Background(), `
WITH j AS (INSERT INTO ledger.ledger_journals (kind, reference_type, reference_id, created_at) VALUES ($1, 'test', gen_random_uuid(), $5::timestamptz) RETURNING id),
     e AS (INSERT INTO ledger.ledger_entries (journal_id, account_id, direction, amount_kes)
           SELECT j.id, a.id, x.d, $4 FROM j, (VALUES ($2::text, 'D'), ($3::text, 'C')) AS x(code, d) JOIN ledger.ledger_accounts a ON a.code = x.code)
SELECT id::text FROM j`, kind, debit, credit, amount, at).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestStaffAPI_FinanceTabGroupsPeriodsAndCarriesTheAuditSections(t *testing.T) {
	e := setup(t) // the clock says 21 Oct 2026
	tok := e.member(staff.Staff)
	seedJournal(t, e, "deposit", "MPESA_CLEARING", "WALLET_LIABILITY", 500, "2026-10-20 10:00:00+03")
	seedJournal(t, e, "stake", "WALLET_LIABILITY", "STAKES_REVENUE", 1, "2026-10-02 09:00:00+03")

	// 1 Oct is a Thursday: four Monday-start weeks, the first and last clipped to the range
	r := e.do(t, http.MethodGet, "/v1/staff/finance?from=2026-10-01&to=2026-10-21&group=week", tok, nil)
	if r.Code != http.StatusOK || r.Body["group"] != "week" {
		t.Fatalf("finance by week: %d %s", r.Code, r.Raw)
	}
	periods := r.Body["periods"].([]any)
	first, last := periods[0].(map[string]any), periods[len(periods)-1].(map[string]any)
	if len(periods) != 4 || first["start"] != "2026-10-01" || first["end"] != "2026-10-04" || last["start"] != "2026-10-19" || last["end"] != "2026-10-21" ||
		first["stakes_kes"] != float64(1) || last["deposits_kes"] != float64(500) || r.Body["totals"].(map[string]any)["deposits_kes"] != float64(500) {
		t.Fatalf("weeks: %s", r.Raw)
	}
	kinds := map[string]map[string]any{}
	for _, k := range r.Body["by_kind"].([]any) {
		k := k.(map[string]any)
		kinds[k["kind"].(string)] = k
	}
	if kinds["deposit"]["journals"] != float64(1) || kinds["deposit"]["debit_kes"] != float64(500) || kinds["deposit"]["credit_kes"] != float64(500) || kinds["stake"] == nil {
		t.Fatalf("by kind: %v", kinds)
	}
	tb := r.Body["trial_balance"].(map[string]any)
	if tb["as_at"] != "2026-10-21" || tb["balanced"] != true || tb["debits_kes"] != float64(501) || len(tb["accounts"].([]any)) < 10 {
		t.Fatalf("trial balance: %v", tb)
	}
	for _, key := range []string{"top_ups", "payouts", "prize_pools"} {
		if _, ok := r.Body[key].([]any); !ok {
			t.Fatalf("%s missing: %s", key, r.Raw)
		}
	}
	if len(r.Body["top_ups"].([]any)) != 4 || r.Body["tax"].(map[string]any)["awaiting_remittance"] == nil {
		t.Fatalf("funnels and tax: %s", r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/finance?from=2026-10-01&to=2026-10-21&group=month", tok, nil); r.Code != http.StatusOK || len(r.Body["periods"].([]any)) != 1 {
		t.Fatalf("finance by month: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/finance?group=fortnight", tok, nil); r.Code != http.StatusBadRequest || !hasType(r, "invalid-group") {
		t.Fatalf("bad group: %d %s", r.Code, r.Raw)
	}
}

func TestStaffAPI_FinanceAndLedgerExportsAreHashedCappedAndAudited(t *testing.T) {
	e := setup(t)
	tok := e.member(staff.Staff)
	me := e.staff.members[tok].ID
	dep := seedJournal(t, e, "deposit", "MPESA_CLEARING", "WALLET_LIABILITY", 500, "2026-10-20 10:00:00+03")
	seedJournal(t, e, "stake", "WALLET_LIABILITY", "STAKES_REVENUE", 1, "2026-10-20 11:00:00+03")

	r := e.do(t, http.MethodGet, "/v1/staff/finance/export?from=2026-10-19&to=2026-10-21&group=day", tok, nil)
	sum := sha256.Sum256([]byte(r.Raw))
	if r.Code != http.StatusOK || r.Header.Get("Content-Type") != "text/csv" || r.Header.Get("Content-Disposition") != `attachment; filename="finance-2026-10-19-2026-10-21.csv"` ||
		r.Header.Get("X-File-SHA256") != hex.EncodeToString(sum[:]) || r.Header.Get("X-File-Rows") != "3" {
		t.Fatalf("finance export: %d %v %s", r.Code, r.Header, r.Raw)
	}
	if !strings.HasPrefix(r.Raw, "period_start,period_end,deposits_kes,") || !strings.Contains(r.Raw, "\n2026-10-20,2026-10-20,500,1,") ||
		!strings.Contains(r.Raw, "\ntrial_balance_as_at,2026-10-21\n") || !strings.HasSuffix(strings.TrimSpace(r.Raw), ",true") {
		t.Fatalf("finance csv:\n%s", r.Raw)
	}

	r = e.do(t, http.MethodGet, "/v1/staff/ledger/export?from=2026-10-20&to=2026-10-20", tok, nil)
	sum = sha256.Sum256([]byte(r.Raw))
	lines := strings.Split(strings.TrimSpace(r.Raw), "\n")
	if r.Code != http.StatusOK || r.Header.Get("Content-Disposition") != `attachment; filename="ledger-2026-10-20-2026-10-20.csv"` ||
		r.Header.Get("X-File-SHA256") != hex.EncodeToString(sum[:]) || r.Header.Get("X-File-Rows") != "4" || len(lines) != 5 {
		t.Fatalf("ledger export: %d %v %s", r.Code, r.Header, r.Raw)
	}
	if lines[0] != "journal_id,journal_kind,reference_type,reference_id,created_at_eat,account_code,direction,amount_kes,user_id" ||
		!strings.HasPrefix(lines[1], dep+",deposit,test,") || !strings.Contains(lines[1], ",2026-10-20 10:00:00,") || !strings.HasSuffix(lines[1], ",500,") {
		t.Fatalf("ledger csv:\n%s", r.Raw)
	}
	// nothing posted that day: a file of only the header
	if r := e.do(t, http.MethodGet, "/v1/staff/ledger/export?from=2026-10-01&to=2026-10-01", tok, nil); r.Code != http.StatusOK || r.Header.Get("X-File-Rows") != "0" || strings.Count(r.Raw, "\n") != 1 {
		t.Fatalf("empty ledger export: %d %s", r.Code, r.Raw)
	}
	// more entries than the cap: refused, and not audited
	e.deps.StaffServices.Finance.WithLedgerRowCap(3)
	if r := e.do(t, http.MethodGet, "/v1/staff/ledger/export?from=2026-10-20&to=2026-10-20", tok, nil); r.Code != http.StatusBadRequest || !hasType(r, "too-many-rows") {
		t.Fatalf("over the cap: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/ledger/export?from=2026-10-21&to=2026-10-20", tok, nil); r.Code != http.StatusBadRequest || !hasType(r, "invalid-range") {
		t.Fatalf("reversed range: %d %s", r.Code, r.Raw)
	}

	var finance, ledger int
	var actor, details string
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE action = 'finance.exported'), count(*) FILTER (WHERE action = 'ledger.exported') FROM audit.audit_log`).Scan(&finance, &ledger); err != nil || finance != 1 || ledger != 2 {
		t.Fatalf("audited exports: finance %d ledger %d (%v)", finance, ledger, err)
	}
	if err := e.pool.QueryRow(context.Background(), `SELECT actor_id, details::text FROM audit.audit_log WHERE action = 'ledger.exported' ORDER BY id LIMIT 1`).Scan(&actor, &details); err != nil ||
		actor != "staff:"+me || !strings.Contains(details, `"rows": 4`) || !strings.Contains(details, `"from": "2026-10-20"`) || !strings.Contains(details, `"sha256": "`) {
		t.Fatalf("ledger export audit: %s %s (%v)", actor, details, err)
	}
}

func TestStaffAPI_ReconciliationRunsAreListedNewestFirst(t *testing.T) {
	e := setup(t)
	tok := e.member(staff.Staff)
	if r := e.do(t, http.MethodGet, "/v1/staff/reconciliations?area=payments", tok, nil); r.Code != http.StatusOK || len(r.Body["runs"].([]any)) != 0 || r.Body["next_before"] != nil {
		t.Fatalf("no runs yet: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/payments/reconcile", tok, nil); r.Code != http.StatusOK {
		t.Fatalf("reconcile: %d %s", r.Code, r.Raw)
	}
	e.clk.Advance(1)
	if r := e.do(t, http.MethodPost, "/v1/staff/game/reconcile", tok, nil); r.Code != http.StatusOK {
		t.Fatalf("reconcile game: %d %s", r.Code, r.Raw)
	}
	r := e.do(t, http.MethodGet, "/v1/staff/reconciliations?area=payments", tok, nil)
	runs := r.Body["runs"].([]any)
	if r.Code != http.StatusOK || len(runs) != 1 {
		t.Fatalf("payments runs: %d %s", r.Code, r.Raw)
	}
	run := runs[0].(map[string]any)
	if run["area"] != "payments" || run["ok"] != true || run["id"] == "" || run["ran_at"] == nil || run["mismatches"] == nil || run["attention"] == nil || run["totals"] == nil {
		t.Fatalf("run: %v", run)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/reconciliations?area=game&limit=1", tok, nil); r.Code != http.StatusOK || len(r.Body["runs"].([]any)) != 1 {
		t.Fatalf("game runs: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/reconciliations?area=wallets", tok, nil); r.Code != http.StatusBadRequest || !hasType(r, "invalid-area") {
		t.Fatalf("bad area: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/reconciliations", tok, nil); r.Code != http.StatusBadRequest || !hasType(r, "invalid-area") {
		t.Fatalf("no area: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/reconciliations?area=payments&before=garbage", tok, nil); r.Code != http.StatusBadRequest || !hasType(r, "invalid-cursor") {
		t.Fatalf("bad cursor: %d %s", r.Code, r.Raw)
	}
}
