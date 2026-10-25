//go:build integration

package payouts_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/draws/drand"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/payments/darajamock"
	"github.com/itsivali/lottery-platform/internal/payouts"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

type winner struct {
	user   string
	amount int64
}

var periodSeq atomic.Int64

// approvedDraw records an approved daily draw that awarded the winners (in rank order) and funds its pool with their
// prizes, as approval does. It returns the draw and each winner's result id.
func (e *env) approvedDraw(t *testing.T, winners ...winner) (string, []string) {
	t.Helper()
	ctx := context.Background()
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(periodSeq.Add(1))).Format(time.DateOnly)
	var drawID string
	if err := e.pool.QueryRow(ctx, `INSERT INTO draws.draws (draw_type, period_start, period_end, status, seed_commit, seed_enc, revealed_seed,
		beacon_round, beacon_value, entry_list_hash, certificate_json, certificate_hash, frozen_at)
		VALUES ('daily', $1, $1, 'approved', repeat('a', 64), '\x00', 'seed', 1, 'beacon', repeat('b', 64), '{}', 'hash', now()) RETURNING id::text`, day).Scan(&drawID); err != nil {
		t.Fatal(err)
	}
	var results []string
	var pool int64
	for i, w := range winners {
		var ticket, entry, result string
		if err := e.pool.QueryRow(ctx, `INSERT INTO tickets.tickets (user_id, play_date, seq, price_kes, idempotency_key) VALUES ($1, $2, 1, 100, $3)
			RETURNING id::text`, w.user, day, "t-"+day).Scan(&ticket); err != nil {
			t.Fatal(err)
		}
		if err := e.pool.QueryRow(ctx, `INSERT INTO draws.draw_entries (draw_id, entry_no, user_id, play_date, ticket_id) VALUES ($1, $2, $3, $4, $5)
			RETURNING id::text`, drawID, i+1, w.user, day, ticket).Scan(&entry); err != nil {
			t.Fatal(err)
		}
		if err := e.pool.QueryRow(ctx, `INSERT INTO draws.draw_results (draw_id, entry_id, prize_rank, amount_kes) VALUES ($1, $2, $3, $4)
			RETURNING id::text`, drawID, entry, i+1, w.amount).Scan(&result); err != nil {
			t.Fatal(err)
		}
		results = append(results, result)
		pool += w.amount
	}
	if pool > 0 {
		p, err := ledger.PrizePoolFunding(drawID, "daily", pool)
		if err != nil {
			t.Fatal(err)
		}
		if err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error { _, err := ledger.NewStore().Post(ctx, tx, p.Journal); return err }); err != nil {
			t.Fatal(err)
		}
	}
	return drawID, results
}

func (e *env) account(t *testing.T, code string) int64 {
	t.Helper()
	var b int64
	if err := e.pool.QueryRow(context.Background(), `SELECT coalesce(sum(CASE e.direction WHEN 'C' THEN e.amount_kes ELSE -e.amount_kes END), 0)
		FROM ledger.ledger_entries e JOIN ledger.ledger_accounts a ON a.id = e.account_id WHERE a.code = $1`, code).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func (e *env) prizePayout(t *testing.T, result string) payouts.Payout {
	t.Helper()
	var id, user string
	if err := e.pool.QueryRow(context.Background(), `SELECT id::text, user_id::text FROM payouts.payouts WHERE draw_result_id = $1
		ORDER BY created_at DESC LIMIT 1`, result).Scan(&id, &user); err != nil {
		t.Fatal(err)
	}
	p, err := e.svc.Get(context.Background(), user, id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (e *env) reconcileOK(t *testing.T) payouts.Report {
	t.Helper()
	r, err := e.svc.Reconcile(context.Background())
	if err != nil || !r.OK {
		t.Fatalf("reconciliation %+v %v", r, err)
	}
	return r
}

type stubBeacon struct{}

func (stubBeacon) Chain() drand.Chain { return drand.Quicknet }
func (stubBeacon) Fetch(context.Context, time.Time, uint64) (drand.Beacon, error) {
	return drand.Beacon{}, drand.ErrNotYet
}
func (stubBeacon) Verify(drand.Beacon) error { return nil }

func TestPrizes_PaidWithTaxWithheldReportedAndTheDrawMarkedPaid(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	a, b := e.player(t, 0), e.player(t, 0)
	drawID, results := e.approvedDraw(t, winner{a, 105}, winner{b, 63})
	if e.account(t, "PRIZE_POOL_DAILY") != 168 {
		t.Fatalf("pool %d", e.account(t, "PRIZE_POOL_DAILY"))
	}
	if n, err := e.svc.PayPrizes(ctx); err != nil || n != 2 {
		t.Fatalf("created %d %v", n, err)
	}
	if n, err := e.svc.PayPrizes(ctx); err != nil || n != 0 {
		t.Fatalf("created again %d %v", n, err)
	}
	// 20% withheld (placeholder): 105 → 21, 63 → 12.6 rounded half up to 13
	for i, want := range []struct{ tax, net int64 }{{21, 84}, {13, 50}} {
		if p := e.prizePayout(t, results[i]); p.Kind != "prize" || p.TaxKES != want.tax || p.NetKES != want.net || p.State != "pending" {
			t.Fatalf("prize %d: %+v", i, p)
		}
	}
	for i := range results {
		if st := e.run(t, e.prizePayout(t, results[i]).ID); st != "succeeded" {
			t.Fatalf("prize %d: %s", i, st)
		}
	}
	if e.account(t, "PRIZE_POOL_DAILY") != 0 || e.account(t, "TAX_PAYABLE") != 34 || e.account(t, "B2C_FLOAT") != 134 {
		t.Fatalf("pool %d tax %d float %d", e.account(t, "PRIZE_POOL_DAILY"), e.account(t, "TAX_PAYABLE"), e.account(t, "B2C_FLOAT"))
	}
	for _, m := range e.mock.Payouts() {
		if m.CommandID != "PromotionPayment" {
			t.Fatalf("command %s", m.CommandID)
		}
	}
	r := e.reconcileOK(t)
	if r.Totals["tax_withheld_kes"] != 34 || r.Totals["net_kes"] != 134 {
		t.Fatalf("totals %v", r.Totals)
	}
	// the withheld tax goes to the KRA feed once per payout, exported once
	var exported []payouts.TaxEvent
	if n, err := e.svc.ExportTax(ctx, 100, func(ev []payouts.TaxEvent) error { exported = ev; return nil }); err != nil || n != 2 {
		t.Fatalf("export %d %v", n, err)
	}
	if n, err := e.svc.ExportTax(ctx, 100, func([]payouts.TaxEvent) error { return nil }); err != nil || n != 0 {
		t.Fatalf("second export %d %v", n, err)
	}
	if string(exported[0].Payload) == "" || exported[0].EventType != "withholding_tax" {
		t.Fatalf("event %+v", exported[0])
	}
	// the draw is paid once every prize is
	ds := draws.NewService(e.pool, e.clk, nil, stubBeacon{}, ledger.NewStore())
	if err := ds.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if d, err := ds.Get(ctx, drawID); err != nil || d.Status != "paid" {
		t.Fatalf("draw %s %v", d.Status, err)
	}
}

func TestPrizes_AFailedPrizeGoesBackToItsPoolAndAnOperatorPaysItAgain(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	a := e.player(t, 0)
	drawID, results := e.approvedDraw(t, winner{a, 500})
	e.mock.SetNextB2CFault(darajamock.B2CFault{ResultCode: 2001})
	if n, _ := e.svc.PayPrizes(ctx); n != 1 {
		t.Fatal("not created")
	}
	first := e.prizePayout(t, results[0])
	if st := e.run(t, first.ID); st != "failed" || e.account(t, "PRIZE_POOL_DAILY") != 500 || e.account(t, "TAX_PAYABLE") != 0 {
		t.Fatalf("state %s pool %d tax %d", st, e.account(t, "PRIZE_POOL_DAILY"), e.account(t, "TAX_PAYABLE"))
	}
	e.reconcileOK(t)
	// not paid again on its own
	if n, _ := e.svc.PayPrizes(ctx); n != 0 {
		t.Fatal("a failed prize was retried automatically")
	}
	ds := draws.NewService(e.pool, e.clk, nil, stubBeacon{}, ledger.NewStore())
	if err := ds.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if d, _ := ds.Get(ctx, drawID); d.Status != "approved" {
		t.Fatalf("draw %s with a prize unpaid", d.Status)
	}
	if _, err := e.svc.RetryPrize(ctx, results[0], ""); !errors.Is(err, payouts.ErrOperator) {
		t.Fatalf("no operator: %v", err)
	}
	again, err := e.svc.RetryPrize(ctx, results[0], "operator:alice")
	if err != nil || again.ID == first.ID {
		t.Fatalf("retry %+v %v", again, err)
	}
	if _, err := e.svc.RetryPrize(ctx, results[0], "operator:alice"); !errors.Is(err, payouts.ErrNotRetryable) {
		t.Fatalf("second retry while one is live: %v", err)
	}
	if st := e.run(t, again.ID); st != "succeeded" || e.account(t, "PRIZE_POOL_DAILY") != 0 {
		t.Fatalf("retry %s pool %d", st, e.account(t, "PRIZE_POOL_DAILY"))
	}
	e.reconcileOK(t)
	if _, err := e.svc.RetryPrize(ctx, "0192f000-0000-7000-8000-000000000000", "operator:alice"); !errors.Is(err, payouts.ErrNotFound) {
		t.Fatalf("unknown prize: %v", err)
	}
}

func TestPrizes_WinnersWhoCannotBePaidWaitAndLargePrizesAreHeld(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	suspended, big, ok := e.player(t, 0), e.player(t, 0), e.player(t, 0)
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'suspended' WHERE id = $1`, suspended); err != nil {
		t.Fatal(err)
	}
	_, results := e.approvedDraw(t, winner{big, 200_000}, winner{ok, 100}, winner{suspended, 50})
	if n, err := e.svc.PayPrizes(ctx); err != nil || n != 2 {
		t.Fatalf("created %d %v", n, err)
	}
	if held := e.prizePayout(t, results[0]); held.State != "held" {
		t.Fatalf("large prize %+v", held)
	}
	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM payouts.payouts WHERE draw_result_id = $1`, results[2]).Scan(&n); err != nil || n != 0 {
		t.Fatalf("suspended winner paid: %d %v", n, err)
	}
	if _, err := e.svc.RetryPrize(ctx, results[2], "operator:alice"); !errors.Is(err, payouts.ErrNotRetryable) {
		t.Fatalf("retry of a prize never attempted: %v", err)
	}
	e.reconcileOK(t)
	// without B2C no prize payout is created
	off := setupWith(t, false, payouts.Config{})
	w := off.player(t, 0)
	off.approvedDraw(t, winner{w, 100})
	if n, err := off.svc.PayPrizes(ctx); err != nil || n != 0 {
		t.Fatalf("without b2c: %d %v", n, err)
	}
}

func TestPayoutReconcile_CatchesTampering(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 1000)
	w := e.withdraw(t, u, 300, "w")
	e.run(t, w.ID)
	a := e.player(t, 0)
	_, results := e.approvedDraw(t, winner{a, 105})
	if _, err := e.svc.PayPrizes(ctx); err != nil {
		t.Fatal(err)
	}
	p := e.prizePayout(t, results[0])
	e.run(t, p.ID)
	e.reconcileOK(t)

	tamper := func(sql string, args ...any) {
		t.Helper()
		c, err := e.pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Release()
		for _, s := range []string{"RESET ROLE", "SET session_replication_role = replica"} {
			if _, err := c.Exec(ctx, s); err != nil {
				t.Fatal(err)
			}
		}
		_, err = c.Exec(ctx, sql, args...)
		_, _ = c.Exec(ctx, "SET session_replication_role = origin")
		_, _ = c.Exec(ctx, "SET ROLE app_worker")
		if err != nil {
			t.Fatal(err)
		}
	}
	// the prize's tax event disappears, the withdrawal claims to have failed without giving the money back, the prize's
	// journal is rewritten to send less to the float
	tamper(`DELETE FROM audit.kra_outbox WHERE payload->>'payout_id' = $1`, p.ID)
	tamper(`UPDATE payouts.payouts SET status = 'failed' WHERE id = $1`, w.ID)
	tamper(`UPDATE ledger.ledger_entries SET amount_kes = amount_kes - 1 WHERE journal_id = (SELECT id FROM ledger.ledger_journals
		WHERE kind = 'prize_payout' AND reference_id = $1) AND account_id = (SELECT id FROM ledger.ledger_accounts WHERE code = 'B2C_FLOAT')`, p.ID)
	r, err := e.svc.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"missing_kra_event", "failure_journal_mismatch", "journal_amount_mismatch", "b2c_float"} {
		if len(r.Mismatches[want]) == 0 {
			t.Errorf("%s not caught: %v", want, r.Mismatches)
		}
	}
	var bad int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM payouts.reconciliation_runs WHERE NOT ok`).Scan(&bad); err != nil || bad != 1 {
		t.Fatalf("stored failing runs %d %v", bad, err)
	}
}

func TestPrizes_RefusalsAndFailuresLeaveNothingBehind(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	a := e.player(t, 0)
	_, results := e.approvedDraw(t, winner{a, 500})
	count := func() int {
		t.Helper()
		var n int
		if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM payouts.payouts WHERE kind = 'prize'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	// a failing clock: no prize payout, no reconciliation run, no export
	flaky := &flakyClock{inner: e.clk}
	aw := audit.NewWriter()
	ls := ledger.NewStore()
	ids := identity.NewService(e.pool, e.clk, nil, aw, identity.Config{AuthKey: []byte("0123456789abcdef0123456789abcdef")})
	broken := payouts.NewService(e.pool, flaky, &fakeDaraja{}, e.q, ids, wallet.NewService(ls), ls, aw, payouts.Config{}, wallet.ErrInsufficientFunds)
	if _, err := broken.PayPrizes(ctx); err == nil || count() != 0 {
		t.Fatalf("pay prizes: %v, %d payouts", err, count())
	}
	if _, err := broken.Reconcile(ctx); err == nil {
		t.Fatal("reconcile with a failing clock")
	}
	// no winnings tax rule in force: nothing is paid
	c, err := e.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"RESET ROLE", "SET session_replication_role = replica", "DELETE FROM ledger.tax_rules WHERE code = 'winnings_wht'",
		"SET session_replication_role = origin", "SET ROLE app_worker"} {
		if _, err := c.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	c.Release()
	if _, err := e.svc.PayPrizes(ctx); !errors.Is(err, payouts.ErrUnavailable) || count() != 0 {
		t.Fatalf("no tax rule: %v, %d payouts", err, count())
	}
	// retries need B2C and a winner who can be paid
	off := setupWith(t, false, payouts.Config{})
	if _, err := off.svc.RetryPrize(ctx, results[0], "operator:a"); !errors.Is(err, payouts.ErrUnavailable) {
		t.Fatalf("retry without b2c: %v", err)
	}
}

func TestPrizes_RetryRefusedForAWinnerWhoCannotBePaid(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	a := e.player(t, 0)
	_, results := e.approvedDraw(t, winner{a, 500})
	e.mock.SetNextB2CFault(darajamock.B2CFault{ResultCode: 2001})
	if n, _ := e.svc.PayPrizes(ctx); n != 1 {
		t.Fatal("not created")
	}
	if st := e.run(t, e.prizePayout(t, results[0]).ID); st != "failed" {
		t.Fatalf("state %s", st)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'suspended' WHERE id = $1`, a); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RetryPrize(ctx, results[0], "operator:a"); err == nil {
		t.Fatal("retried a prize for a suspended winner")
	}
	e.reconcileOK(t)
}

func TestExportTax_AFailedWriteExportsNothing(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	a := e.player(t, 0)
	_, results := e.approvedDraw(t, winner{a, 500})
	if _, err := e.svc.PayPrizes(ctx); err != nil {
		t.Fatal(err)
	}
	e.run(t, e.prizePayout(t, results[0]).ID)
	if _, err := e.svc.ExportTax(ctx, 10, func([]payouts.TaxEvent) error { return errors.New("disk full") }); err == nil {
		t.Fatal("a failed write was reported as exported")
	}
	if n, err := e.svc.ExportTax(ctx, 10, func(ev []payouts.TaxEvent) error { return nil }); err != nil || n != 1 {
		t.Fatalf("the event was lost: %d %v", n, err)
	}
	flaky := &flakyClock{inner: e.clk}
	aw := audit.NewWriter()
	ls := ledger.NewStore()
	broken := payouts.NewService(e.pool, flaky, nil, e.q, nil, wallet.NewService(ls), ls, aw, payouts.Config{}, wallet.ErrInsufficientFunds)
	e.player(t, 0)
	if _, err := e.svc.PayPrizes(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := broken.ExportTax(ctx, 10, func([]payouts.TaxEvent) error { return nil }); err != nil {
		t.Fatalf("an empty export failed: %v", err)
	}
}
