//go:build integration

package draws_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/itsivali/4draws-lottery-platform/internal/draws"
	"github.com/itsivali/4draws-lottery-platform/internal/ledger"
)

// approvedDraw plays a day for three players and takes its daily draw to approved; it returns the draw.
func (e *env) approvedDraw(t *testing.T) draws.Draw {
	t.Helper()
	ctx := context.Background()
	e.playerDay(t, "2026-10-24", 2, false)
	e.playerDay(t, "2026-10-24", 4, true)
	e.playerDay(t, "2026-10-24", 1, false)
	e.schedule(t)
	e.clk.Set(nairobi(2026, 10, 25, 0, 6))
	e.schedule(t)
	d := e.draw(t, "daily", "2026-10-24", "frozen")
	for _, official := range []string{"operator:alice", "operator:bob"} {
		if _, err := e.svc.Sign(ctx, d.ID, official); err != nil {
			t.Fatal(err)
		}
	}
	e.clk.Set(nairobi(2026, 10, 25, 9, 1))
	e.schedule(t)
	e.draw(t, "daily", "2026-10-24", "conducted")
	for _, official := range []string{"operator:carol", "operator:dave"} {
		if _, err := e.svc.Approve(ctx, d.ID, official); err != nil {
			t.Fatal(err)
		}
	}
	return e.draw(t, "daily", "2026-10-24", "approved")
}

// tamper changes rows as the database owner with triggers off, the way only an attacker with superuser could.
func (e *env) tamper(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	c, err := e.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Release()
	if _, err := c.Exec(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Exec(ctx, "SET session_replication_role = replica"); err != nil {
		t.Fatal(err)
	}
	_, err = c.Exec(ctx, sql, args...)
	_, _ = c.Exec(ctx, "SET session_replication_role = origin")
	_, _ = c.Exec(ctx, "SET ROLE app_worker")
	if err != nil {
		t.Fatal(err)
	}
}

func TestReconcile_CleanAtEveryStepOfTheLifecycle(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 24, 12, 0))
	if r, err := e.svc.Reconcile(ctx); err != nil || !r.OK || r.Totals["draws_checked"] != 0 {
		t.Fatalf("nothing fixed yet: %+v %v", r, err)
	}
	d := e.approvedDraw(t)
	r, err := e.svc.Reconcile(ctx)
	if err != nil || !r.OK {
		t.Fatalf("report %+v %v", r, err)
	}
	if r.Totals["draws_checked"] != 1 || r.Totals["entries_checked"] != 13 || r.Totals["prize_pool_daily_kes"] != 210 ||
		r.Totals["prize_pool_weekly_kes"] != 0 || d.Entries != 13 {
		t.Fatalf("totals %v", r.Totals)
	}
	var runs int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM draws.reconciliation_runs WHERE ok`).Scan(&runs); err != nil || runs != 2 {
		t.Fatalf("stored runs %d %v", runs, err)
	}
	if _, err := e.pool.Exec(ctx, `DELETE FROM draws.reconciliation_runs`); err == nil {
		t.Fatal("a reconciliation run was deleted")
	}
}

func TestReconcile_CatchesEveryKindOfTampering(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 24, 12, 0))
	d := e.approvedDraw(t)
	weekly := e.openWeekly(t)

	// money moved into a pool for a draw that has not been drawn
	p, err := ledger.PrizePoolFunding(weekly.ID, "weekly", 50)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.NewStore().Post(ctx, tx, p.Journal); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// an entry slipped in after the freeze
	e.tamper(t, `INSERT INTO draws.draw_entries (draw_id, entry_no, user_id, play_date, ticket_id)
		SELECT draw_id, 14, user_id, play_date, ticket_id FROM draws.draw_entries WHERE draw_id = $1 AND entry_no = 1`, d.ID)
	// the published certificate rewritten: a bigger first prize and another beacon signature
	e.tamper(t, `UPDATE draws.draws SET certificate_json = replace(replace(certificate_json::text, '"amount_kes":105', '"amount_kes":150'),
		'"signature":"00"', '"signature":"01"')::json WHERE id = $1`, d.ID)
	// a result row nobody drew
	e.tamper(t, `INSERT INTO draws.draw_results (draw_id, entry_id, prize_rank, amount_kes)
		SELECT draw_id, id, 9, 1000 FROM draws.draw_entries WHERE draw_id = $1 AND entry_no = 14`, d.ID)
	// the revealed seed swapped
	e.tamper(t, `UPDATE draws.draws SET revealed_seed = repeat('ab', 32) WHERE id = $1`, d.ID)

	r, err := e.svc.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"funding_mismatch", "pool_balance_mismatch", "entry_list_hash_mismatch", "certificate_hash_mismatch",
		"certificate_vs_draw", "beacon_invalid", "winners_not_reproducible", "results_vs_certificate", "prizes_exceed_pool"} {
		if len(r.Mismatches[want]) == 0 {
			t.Errorf("%s not caught: %v", want, r.Mismatches)
		}
	}
	if r.OK || !strings.HasPrefix(r.Mismatches["funding_mismatch"][0], weekly.ID) {
		t.Fatalf("report %+v", r)
	}
	var bad int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM draws.reconciliation_runs WHERE NOT ok`).Scan(&bad); err != nil || bad != 1 {
		t.Fatalf("stored failing runs %d %v", bad, err)
	}
}

func TestReconcile_AConductedDrawIsNotFundedUntilApproved(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 24, 12, 0))
	d := e.approvedDraw(t)
	// a second funding of the same draw cannot even be posted (one journal per event)
	p, _ := ledger.PrizePoolFunding(d.ID, "daily", 210)
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ledger.NewStore().Post(ctx, tx, p.Journal)
	_ = tx.Rollback(ctx)
	if err == nil {
		t.Fatal("the pool was funded twice")
	}
	// the draw's state moved back to conducted behind the trigger's back: funded but not approved
	e.tamper(t, `UPDATE draws.draws SET status = 'conducted' WHERE id = $1`, d.ID)
	r, err := e.svc.Reconcile(ctx)
	if err != nil || r.OK || len(r.Mismatches["funding_mismatch"]) != 1 || len(r.Mismatches["pool_balance_mismatch"]) != 1 {
		t.Fatalf("report %+v %v", r, err)
	}
}

func TestOverdue_ReportsEachLateStep(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 24, 12, 0))
	e.playerDay(t, "2026-10-24", 1, false)
	e.playerDay(t, "2026-10-24", 1, false)
	e.schedule(t)
	late := func(want ...string) {
		t.Helper()
		got, err := e.svc.Overdue(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Fatalf("overdue %+v, want %v", got, want)
		}
		for i, w := range want {
			if got[i].Status != w || got[i].DrawType != "daily" || got[i].Problem == "" {
				t.Fatalf("overdue %+v, want %v", got, want)
			}
		}
	}
	late()
	e.clk.Set(nairobi(2026, 10, 25, 0, 34)) // within the grace after 00:05
	late()
	e.clk.Set(nairobi(2026, 10, 25, 0, 36)) // the worker has not frozen it
	late("open")
	e.schedule(t)
	late()
	d := e.draw(t, "daily", "2026-10-24", "frozen")
	e.clk.Set(nairobi(2026, 10, 25, 8, 1)) // under an hour of signing left
	if _, err := e.svc.Sign(ctx, d.ID, "operator:alice"); err != nil {
		t.Fatal(err)
	}
	late("frozen")
	if got, _ := e.svc.Overdue(ctx); !strings.HasPrefix(got[0].Problem, "1 of 2") {
		t.Fatalf("problem %q", got[0].Problem)
	}
	if _, err := e.svc.Sign(ctx, d.ID, "operator:bob"); err != nil {
		t.Fatal(err)
	}
	late()
	e.beacon.down.Store(true) // drand unreachable past the round
	e.clk.Set(nairobi(2026, 10, 25, 9, 40))
	e.schedule(t)
	late("signed")
	e.beacon.down.Store(false)
	e.schedule(t)
	late()
	e.clk.Set(nairobi(2026, 10, 26, 9, 1)) // a day after the round, still unapproved
	e.schedule(t)
	late("conducted")
}

func TestReconcile_AnApprovedDrawFundedTheWrongAmount(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 24, 12, 0))
	d := e.approvedDraw(t)
	// both sides of the funding journal rewritten (still balanced): the pool got 200 of the certified 210
	e.tamper(t, `UPDATE ledger.ledger_entries SET amount_kes = 200 WHERE journal_id =
		(SELECT id FROM ledger.ledger_journals WHERE kind = 'prize_pool_funding' AND reference_id = $1)`, d.ID)
	r, err := e.svc.Reconcile(ctx)
	if err != nil || r.OK || len(r.Mismatches["funding_mismatch"]) != 1 || !strings.HasPrefix(r.Mismatches["funding_mismatch"][0], d.ID) {
		t.Fatalf("report %+v %v", r, err)
	}
}

func TestReconcile_WinnersMustFollowFromThePrizeStructureAndThePool(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 24, 12, 0))
	d := e.approvedDraw(t)
	// a prize structure slipped in behind the draw: the certified pool share is no longer the one in force
	e.tamper(t, `INSERT INTO draws.prize_structures (draw_type, effective_from, kind, rounding, pool_share, prizes, created_by)
		VALUES ('daily', '2026-10-01', 'pool_share', 'down', 0.40, '[{"rank":1,"share":"1"}]', 'test')`)
	r, err := e.svc.Reconcile(ctx)
	if err != nil || len(r.Mismatches["winners_not_reproducible"]) != 1 || !strings.Contains(r.Mismatches["winners_not_reproducible"][0], "prize structure") {
		t.Fatalf("report %+v %v", r, err)
	}
	e.tamper(t, `DELETE FROM draws.prize_structures WHERE created_by = 'test'`)
	// the certified pool is not the certified share of the certified stakes
	e.tamper(t, `UPDATE draws.draws SET certificate_json = replace(certificate_json::text, '"stakes_kes":700', '"stakes_kes":800')::json WHERE id = $1`, d.ID)
	r, err = e.svc.Reconcile(ctx)
	if err != nil || len(r.Mismatches["winners_not_reproducible"]) != 1 || !strings.Contains(r.Mismatches["winners_not_reproducible"][0], "pool 210") {
		t.Fatalf("report %+v %v", r, err)
	}
}

func TestViews_EntryListCertificateAndOwnEntries(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 24, 12, 0))
	d := e.approvedDraw(t)
	list, err := e.svc.EntryList(ctx, d.ID)
	if err != nil || len(list) != 13 || list[0].EntryNo != 1 {
		t.Fatalf("entry list %d %v", len(list), err)
	}
	h := sha256.New()
	for _, l := range list {
		_, _ = fmt.Fprintf(h, "%d|%s|%s|%s\n", l.EntryNo, l.UserID, l.PlayDate, l.TicketID)
	}
	if hex.EncodeToString(h.Sum(nil)) != d.EntryListHash {
		t.Fatal("the exported entry list does not hash to entry_list_hash")
	}
	raw, sum, err := e.svc.CertificateOf(ctx, d.ID)
	if got := sha256.Sum256(raw); err != nil || hex.EncodeToString(got[:]) != sum || sum != d.CertificateHash {
		t.Fatalf("certificate %s %v", sum, err)
	}
	mine, err := e.svc.EntriesOf(ctx, d.ID, list[0].UserID)
	if err != nil || len(mine.Entries) == 0 || mine.Entries[0].EntryNo != 1 {
		t.Fatalf("own entries %+v %v", mine, err)
	}
	weekly := e.openWeekly(t)
	if _, _, err := e.svc.CertificateOf(ctx, weekly.ID); !errors.Is(err, draws.ErrState) {
		t.Fatalf("certificate of an open draw: %v", err)
	}
	for _, id := range []string{"0192f000-0000-7000-8000-000000000000", "not-a-uuid"} {
		if _, err := e.svc.EntryList(ctx, id); !errors.Is(err, draws.ErrNotFound) {
			t.Errorf("entry list %s: %v", id, err)
		}
		if _, _, err := e.svc.CertificateOf(ctx, id); !errors.Is(err, draws.ErrNotFound) {
			t.Errorf("certificate %s: %v", id, err)
		}
		if _, err := e.svc.EntriesOf(ctx, id, list[0].UserID); !errors.Is(err, draws.ErrNotFound) {
			t.Errorf("own entries %s: %v", id, err)
		}
	}
	// the certificate names winning entries, never players; one issued before that (naming them) is not published
	if bytes.Contains(raw, []byte("user_id")) {
		t.Fatalf("the certificate names its winners: %s", raw)
	}
	e.tamper(t, `UPDATE draws.draws SET certificate_json = replace(certificate_json::text, '"entry_no":', '"user_id":"x","entry_no":')::json WHERE id = $1`, d.ID)
	if _, _, err := e.svc.CertificateOf(ctx, d.ID); !errors.Is(err, draws.ErrWithheld) {
		t.Fatalf("a certificate naming its winners: %v", err)
	}
}

// Prize payouts (phase 5) take prizes out of the pools; the draw is paid once every prize is confirmed paid.
func TestReconcile_PrizePayoutsComeOutOfThePoolAndPayTheDraw(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 24, 12, 0))
	d := e.approvedDraw(t)
	e.schedule(t)
	if got := e.draw(t, "daily", "2026-10-24", "approved"); len(got.Results) != 3 {
		t.Fatalf("results %+v", got.Results)
	}
	none := ledger.TaxRule{Rate: ledger.RateFromE5(0), Rounding: ledger.HalfUp}
	for i, res := range d.Results {
		var resultID, payoutID string
		if err := e.pool.QueryRow(ctx, `SELECT r.id::text FROM draws.draw_results r WHERE r.draw_id = $1 AND r.prize_rank = $2`, d.ID, res.Rank).Scan(&resultID); err != nil {
			t.Fatal(err)
		}
		status := "succeeded"
		if i == 2 {
			status = "sent" // still being paid
		}
		if err := e.pool.QueryRow(ctx, `INSERT INTO payouts.payouts (user_id, kind, status, gross_kes, net_kes, msisdn, draw_result_id,
			originator_conversation_id, idempotency_key, mpesa_receipt, settled_at)
			VALUES ($1, 'prize', $2, $3, $3, '254700000001', $4, $5, $5, CASE WHEN $2 = 'succeeded' THEN $5 END, CASE WHEN $2 = 'succeeded' THEN now() END)
			RETURNING id::text`, res.UserID, status, res.AmountKES, resultID, fmt.Sprint("prize-", i)).Scan(&payoutID); err != nil {
			t.Fatal(err)
		}
		p, err := ledger.PrizePayout(payoutID, "daily", res.AmountKES, none)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := e.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.NewStore().Post(ctx, tx, p.Journal); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if r, err := e.svc.Reconcile(ctx); err != nil || !r.OK || r.Totals["prize_pool_daily_kes"] != 0 {
		t.Fatalf("report %+v %v", r, err)
	}
	e.schedule(t)
	e.draw(t, "daily", "2026-10-24", "approved") // one prize is still being paid
	if _, err := e.pool.Exec(ctx, `UPDATE payouts.payouts SET status = 'succeeded', mpesa_receipt = 'MBCLAST001', settled_at = now()
		WHERE idempotency_key = 'prize-2'`); err != nil {
		t.Fatal(err)
	}
	e.schedule(t)
	e.draw(t, "daily", "2026-10-24", "paid")
	if r, err := e.svc.Reconcile(ctx); err != nil || !r.OK {
		t.Fatalf("after paid %+v %v", r, err)
	}
}
