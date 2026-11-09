//go:build integration

package payouts_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/payouts"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

// batchEvents reads the events of a batch file after checking its header.
func batchEvents(t *testing.T, file []byte) []payouts.TaxEvent {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(file), []byte("\n"))
	var h struct {
		Format string `json:"format"`
		Events int    `json:"events"`
	}
	if err := json.Unmarshal(lines[0], &h); err != nil || h.Format != payouts.TaxFileFormat || h.Events != len(lines)-1 {
		t.Fatalf("header %s %v", lines[0], err)
	}
	out := make([]payouts.TaxEvent, 0, len(lines)-1)
	for _, l := range lines[1:] {
		var e payouts.TaxEvent
		if err := json.Unmarshal(l, &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

// tamper changes the database behind the application's back (no role, no triggers).
func (e *env) tamper(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
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

// paidPrizes pays one prize per amount, each to a new winner, through B2C.
func (e *env) paidPrizes(t *testing.T, amounts ...int64) {
	t.Helper()
	ctx := context.Background()
	var ws []winner
	for _, a := range amounts {
		ws = append(ws, winner{e.player(t, 0), a})
	}
	_, results := e.approvedDraw(t, ws...)
	if _, err := e.svc.PayPrizes(ctx); err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if st := e.run(t, e.prizePayout(t, r).ID); st != "succeeded" {
			t.Fatalf("prize %s", st)
		}
	}
}

func TestTaxBatches_PreparedByOnePersonRemittedByAnotherAndPostedOnce(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	if _, _, err := e.svc.PrepareBatch(ctx, "staff:alice", 100); !errors.Is(err, payouts.ErrNothingToBatch) {
		t.Fatalf("empty batch: %v", err)
	}
	e.paidPrizes(t, 105, 63) // tax 21 + 13
	if _, _, err := e.svc.PrepareBatch(ctx, " ", 100); !errors.Is(err, payouts.ErrOperator) {
		t.Fatalf("no operator: %v", err)
	}
	b, file, err := e.svc.PrepareBatch(ctx, "staff:alice", 100)
	if err != nil || b.No != 1 || b.Events != 2 || b.TaxKES != 34 || b.GrossKES != 168 || b.PreparedBy != "staff:alice" || b.PRN != "" {
		t.Fatalf("batch %+v %v", b, err)
	}
	if sum := sha256.Sum256(file); hex.EncodeToString(sum[:]) != b.SHA256 {
		t.Fatal("the file does not hash to the batch's SHA-256")
	}
	if len(batchEvents(t, file)) != 2 {
		t.Fatal("events")
	}
	// rebuilt byte for byte
	again, rebuilt, err := e.svc.BatchFile(ctx, 1)
	if err != nil || !bytes.Equal(rebuilt, file) || again.SHA256 != b.SHA256 {
		t.Fatalf("rebuilt %v\n%s\n%s", err, rebuilt, file)
	}
	if _, _, err := e.svc.BatchFile(ctx, 99); !errors.Is(err, payouts.ErrNotFound) {
		t.Fatalf("unknown batch: %v", err)
	}
	// the remittance: someone else, exactly the tax, a PRN, once
	for name, tc := range map[string]struct {
		no      int64
		prn, op string
		amount  int64
		want    error
	}{
		"the preparer":  {1, "PRN-1", "staff:alice", 34, payouts.ErrSamePerson},
		"short":         {1, "PRN-1", "staff:bob", 33, payouts.ErrRemittanceAmount},
		"no prn":        {1, " ", "staff:bob", 34, payouts.ErrOperator},
		"no operator":   {1, "PRN-1", "", 34, payouts.ErrOperator},
		"unknown batch": {7, "PRN-1", "staff:bob", 34, payouts.ErrNotFound},
	} {
		if _, err := e.svc.RecordRemittance(ctx, tc.no, tc.prn, tc.amount, tc.op); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
	}
	if e.account(t, "TAX_PAYABLE") != 34 {
		t.Fatalf("a refused remittance moved tax: %d", e.account(t, "TAX_PAYABLE"))
	}
	r, err := e.svc.RecordRemittance(ctx, 1, " PRN-1 ", 34, "staff:bob")
	if err != nil || r.PRN != "PRN-1" || r.RemittedBy != "staff:bob" || r.RemittedAt == nil {
		t.Fatalf("remit %+v %v", r, err)
	}
	if e.account(t, "TAX_PAYABLE") != 0 || e.account(t, "OPERATOR_BANK") != 34 {
		t.Fatalf("tax %d bank %d", e.account(t, "TAX_PAYABLE"), e.account(t, "OPERATOR_BANK"))
	}
	if _, err := e.svc.RecordRemittance(ctx, 1, "PRN-2", 34, "staff:bob"); !errors.Is(err, payouts.ErrRemitted) {
		t.Fatalf("remitted twice: %v", err)
	}
	rep := e.reconcileOK(t)
	if rep.Totals["tax_remitted_kes"] != 34 || rep.Totals["tax_payable_kes"] != 0 {
		t.Fatalf("totals %v", rep.Totals)
	}
	// the next batch is numbered 2 and cannot reuse a PRN
	e.paidPrizes(t, 50)
	b2, _, err := e.svc.PrepareBatch(ctx, "staff:alice", 100)
	if err != nil || b2.No != 2 || b2.TaxKES != 10 {
		t.Fatalf("second batch %+v %v", b2, err)
	}
	if _, err := e.svc.RecordRemittance(ctx, 2, "PRN-1", 10, "staff:bob"); !errors.Is(err, payouts.ErrPRNUsed) {
		t.Fatalf("PRN reused: %v", err)
	}
	if list, err := e.svc.Batches(ctx, 10); err != nil || len(list) != 2 || list[0].No != 2 {
		t.Fatalf("batches %+v %v", list, err)
	}
	e.reconcileOK(t)
	// the audit log records who prepared and who remitted
	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM audit.audit_log WHERE action IN ('kra.batch.prepared','kra.batch.remitted')`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("audit %d %v", n, err)
	}
}

func TestTaxBatches_TheDatabaseKeepsBatchesAndEventsAsTheyWere(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.paidPrizes(t, 105)
	b, _, err := e.svc.PrepareBatch(ctx, "staff:alice", 100)
	if err != nil {
		t.Fatal(err)
	}
	for name, sql := range map[string]string{
		"totals changed":           `UPDATE audit.kra_batches SET tax_kes = 1 WHERE id = $1`,
		"batch deleted":            `DELETE FROM audit.kra_batches WHERE id = $1`,
		"event moved":              `UPDATE audit.kra_outbox SET batch_id = NULL WHERE batch_id = $1`,
		"event rewritten":          `UPDATE audit.kra_outbox SET payload = '{}' WHERE batch_id = $1`,
		"event deleted":            `DELETE FROM audit.kra_outbox WHERE batch_id = $1`,
		"remitted by its preparer": `UPDATE audit.kra_batches SET prn = 'x', remitted_kes = tax_kes, remitted_by = prepared_by, remitted_at = now() WHERE id = $1`,
		"partly remitted":          `UPDATE audit.kra_batches SET prn = 'x', remitted_kes = 1, remitted_by = 'operator:z', remitted_at = now() WHERE id = $1`,
	} {
		if _, err := e.pool.Exec(ctx, sql, b.ID); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := e.svc.RecordRemittance(ctx, b.No, "PRN-9", b.TaxKES, "staff:bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE audit.kra_batches SET prn = 'other' WHERE id = $1`, b.ID); err == nil {
		t.Error("a remitted batch changed")
	}
}

func TestTaxBatches_AlteredEventsAreCaughtAndNeverRemitted(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.paidPrizes(t, 105, 63)
	b, _, err := e.svc.PrepareBatch(ctx, "staff:alice", 100)
	if err != nil {
		t.Fatal(err)
	}
	e.tamper(t, `UPDATE audit.kra_outbox SET payload = jsonb_set(payload, '{tax_kes}', '1') WHERE id = (SELECT min(id) FROM audit.kra_outbox WHERE batch_id = $1)`, b.ID)
	if _, _, err := e.svc.BatchFile(ctx, b.No); !errors.Is(err, payouts.ErrBatchAltered) {
		t.Fatalf("altered file: %v", err)
	}
	if _, err := e.svc.RecordRemittance(ctx, b.No, "PRN-1", b.TaxKES, "staff:bob"); !errors.Is(err, payouts.ErrBatchAltered) {
		t.Fatalf("remitted an altered batch: %v", err)
	}
	r, err := e.svc.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kra_batch_totals", "kra_event_mismatch"} {
		if len(r.Mismatches[want]) == 0 {
			t.Errorf("%s not caught: %v", want, r.Mismatches)
		}
	}
	// a remittance journal without its batch's remittance is caught too
	e.tamper(t, `UPDATE audit.kra_outbox SET payload = jsonb_set(payload, '{tax_kes}', '21') WHERE id = (SELECT min(id) FROM audit.kra_outbox WHERE batch_id = $1)`, b.ID)
	p, err := ledger.TaxRemittance(b.ID, b.TaxKES)
	if err != nil {
		t.Fatal(err)
	}
	if err := postJournal(e, p.Journal); err != nil {
		t.Fatal(err)
	}
	if r, err = e.svc.Reconcile(ctx); err != nil || len(r.Mismatches["remittance_journal"]) == 0 || len(r.Mismatches["tax_payable"]) == 0 {
		t.Fatalf("unrecorded remittance not caught: %v %v", r.Mismatches, err)
	}
}

func TestTaxBatches_AFailureMidBatchLeavesEverythingUnbatched(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.paidPrizes(t, 105)
	flaky := &flakyClock{inner: e.clk}
	ls := ledger.NewStore()
	broken := payouts.NewService(e.pool, flaky, nil, e.q, nil, wallet.NewService(ls), ls, audit.NewWriter(), payouts.Config{}, wallet.ErrInsufficientFunds)
	if _, _, err := broken.PrepareBatch(ctx, "staff:alice", 100); err == nil {
		t.Fatal("a batch without a clock")
	}
	if list, _ := e.svc.Batches(ctx, 10); len(list) != 0 {
		t.Fatalf("batches %+v", list)
	}
	b, _, err := e.svc.PrepareBatch(ctx, "staff:alice", 100)
	if err != nil || b.No != 1 || b.Events != 1 {
		t.Fatalf("batch after the failure %+v %v", b, err)
	}
	flaky.left.Store(0)
	if _, err := broken.RecordRemittance(ctx, 1, "PRN-1", b.TaxKES, "staff:bob"); err == nil {
		t.Fatal("remitted without a clock")
	}
	if e.account(t, "TAX_PAYABLE") != b.TaxKES {
		t.Fatal("a failed remittance moved tax")
	}
}

func postJournal(e *env, j ledger.Journal) error {
	ctx := context.Background()
	return pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error { _, err := ledger.NewStore().Post(ctx, tx, j); return err })
}

// A failed reconciliation's alert freezes payouts: nothing is sent while it is open, and the payout waits pending
// (never sent, so it can be sent later) until a person closes the alert.
func TestSend_NothingLeavesWhilePayoutsAreFrozen(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	var alert string
	if err := e.pool.QueryRow(ctx, `INSERT INTO audit.alerts (kind, subject, severity, freezes, summary, first_seen_at, last_seen_at)
		VALUES ('reconciliation.failed', 'payouts/x', 'page', 'payouts', 'books', now(), now()) RETURNING id::text`).Scan(&alert); err != nil {
		t.Fatal(err)
	}
	u := e.player(t, 1000)
	p := e.withdraw(t, u, 300, "frozen")
	e.q.take()
	if err := e.svc.Send(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if st, _, _ := e.state(t, p.ID); st != "pending" || len(e.mock.Payouts()) != 0 {
		t.Fatalf("sent while frozen: %s %+v", st, e.mock.Payouts())
	}
	if _, err := e.pool.Exec(ctx, `UPDATE audit.alerts SET resolved_at = now(), resolved_by = 'system' WHERE id = $1`, alert); err == nil {
		t.Fatal("a freeze was closed without a person acknowledging it")
	}
	if _, err := e.pool.Exec(ctx, `UPDATE audit.alerts SET acknowledged_by = 'operator:alice', acknowledged_at = now(), ack_note = 'fixed',
		resolved_by = 'operator:alice', resolved_at = now() WHERE id = $1`, alert); err != nil {
		t.Fatal(err)
	}
	// the sweep queues the waiting payout again
	e.clk.Advance(time.Hour)
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if st := e.run(t, p.ID); st != "succeeded" || len(e.mock.Payouts()) != 1 {
		t.Fatalf("after the freeze: %s %+v", st, e.mock.Payouts())
	}
}
