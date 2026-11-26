//go:build integration

package payouts_test

import (
	"context"
	"errors"
	"testing"

	"github.com/itsivali/lottery-platform/internal/aml"
	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/payouts"
)

// topUpFrom records a credited top-up paid from msisdn, as the payments module would.
func (e *env) topUpFrom(t *testing.T, user, msisdn string) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO payments.payments (user_id, channel, status, amount_requested_kes, amount_confirmed_kes,
		msisdn, mpesa_receipt, idempotency_key) VALUES ($1, 'stk', 'credited', 1000, 1000, $2, 'RF' || substr(md5(random()::text), 1, 8), md5(random()::text))`,
		user, msisdn); err != nil {
		t.Fatal(err)
	}
}

// ADR 0031: a balance the account cannot withdraw (its identity was refused) goes back to the number that paid it in.
// An admin asks with a reason; the wallet is debited and the payout held; a second staff member approves it and it is
// sent like any withdrawal. The admin who asked cannot approve it.
func TestRefundToSource_SendsAnUnpayableBalanceBackToThePayerAfterASecondPerson(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 1_000) // 950 in the wallet
	e.topUpFrom(t, u, "254708374149")
	if _, err := e.svc.RefundToSource(ctx, u, "staff:admin-a", "identity refused", "k0"); !errors.Is(err, payouts.ErrPayable) {
		t.Fatalf("refund for an account that can withdraw: %v", err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET kyc_status = 'rejected', national_id_hash = NULL WHERE id = $1`, u); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RefundToSource(ctx, u, "staff:admin-a", "  ", "k1"); !errors.Is(err, payouts.ErrOperator) {
		t.Fatalf("refund without a reason: %v", err)
	}
	p, err := e.svc.RefundToSource(ctx, u, "staff:admin-a", "identity refused", "k1")
	if err != nil {
		t.Fatal(err)
	}
	if again, err := e.svc.RefundToSource(ctx, u, "staff:admin-a", "identity refused", "k1"); err != nil || again.ID != p.ID {
		t.Fatalf("same key: %v %+v", err, again)
	}
	if p.State != "held" || p.GrossKES != 950 || p.Refund == nil || p.Refund.MSISDN != "254708374149" || p.Refund.RequestedBy != "staff:admin-a" {
		t.Fatalf("refund %+v %+v", p, p.Refund)
	}
	if b := e.balance(t, u); b != 0 {
		t.Fatalf("balance %d after the refund was asked for, want 0", b)
	}
	if _, err := e.svc.RefundToSource(ctx, u, "staff:admin-a", "again", "k2"); !errors.Is(err, payouts.ErrNothingToRefund) {
		t.Fatalf("a second refund of an empty wallet: %v", err)
	}
	if _, err := e.svc.Approve(ctx, p.ID, "staff:admin-a"); !errors.Is(err, payouts.ErrSameApprover) {
		t.Fatalf("approved by the admin who asked: %v", err)
	}
	if _, err := e.svc.Approve(ctx, p.ID, "staff:ops-b"); err != nil {
		t.Fatal(err)
	}
	if st := e.run(t, p.ID); st != "succeeded" {
		t.Fatalf("refund: %s", st)
	}
	sent := e.mock.Payouts()
	if len(sent) != 1 || sent[0].MSISDN != "254708374149" || sent[0].AmountKES != p.NetKES {
		t.Fatalf("B2C %+v, want %d to the payer", sent, p.NetKES)
	}
	// the refund's reason and requester are part of the payout
	if _, err := e.pool.Exec(ctx, `UPDATE payouts.payouts SET refund_reason = 'other' WHERE id = $1`, p.ID); err == nil {
		t.Fatal("a refund's reason was changed")
	}
}

// ADR 0031: a rejected refund gives the money back to the wallet; without a credited top-up there is nowhere to send it.
func TestRefundToSource_RejectedGivesTheMoneyBackAndNeedsASource(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	nowhere := e.player(t, 1_000)
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'suspended' WHERE id = $1`, nowhere); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RefundToSource(ctx, nowhere, "staff:admin-a", "fraud", "k1"); !errors.Is(err, payouts.ErrNoSource) {
		t.Fatalf("refund without a top-up: %v", err)
	}

	u := e.player(t, 1_000)
	e.topUpFrom(t, u, "254708374149")
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'suspended' WHERE id = $1`, u); err != nil {
		t.Fatal(err)
	}
	p, err := e.svc.RefundToSource(ctx, u, "staff:admin-a", "fraud check", "k1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Reject(ctx, p.ID, "staff:ops-b", "keep it while the inquiry runs"); err != nil {
		t.Fatal(err)
	}
	if b := e.balance(t, u); b != 950 {
		t.Fatalf("balance %d after the refund was rejected, want 950", b)
	}
}

// ADR 0031, 0033, 0007: a refund to source is refused, with nothing debited, while a monitoring case holds the
// player's money, while the player owes the platform (the debt is settled first), for an account that does not exist,
// and without B2C.
func TestRefundToSource_RefusedUnderReviewInDebtOrWithoutB2C(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	monitor := aml.NewService(e.pool, e.clk, audit.NewWriter())
	e.svc.WithHolds(monitor)
	suspend := func(u string) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'suspended' WHERE id = $1`, u); err != nil {
			t.Fatal(err)
		}
	}

	reviewed := e.player(t, 1_000)
	e.topUpFrom(t, reviewed, "254708374149")
	suspend(reviewed)
	c, err := monitor.OpenManual(ctx, reviewed, "staff:alice", "in and out")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := monitor.Hold(ctx, c.ID, "staff:alice", "pending review"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RefundToSource(ctx, reviewed, "staff:admin-a", "fraud check", "k1"); !errors.Is(err, aml.ErrUnderReview) {
		t.Fatalf("refund under review: %v", err)
	}

	debtor := e.player(t, 0)
	e.owe(t, debtor, 100)
	e.credit(t, debtor, 1_000) // 950, while 100 is still owed
	e.topUpFrom(t, debtor, "254708374149")
	suspend(debtor)
	if _, err := e.svc.RefundToSource(ctx, debtor, "staff:admin-a", "fraud check", "k1"); !errors.Is(err, payouts.ErrInDebt) {
		t.Fatalf("refund while in debt: %v", err)
	}
	if e.balance(t, reviewed) != 950 || e.balance(t, debtor) != 950 || e.debt(t, debtor) != 100 {
		t.Fatalf("balances %d %d debt %d", e.balance(t, reviewed), e.balance(t, debtor), e.debt(t, debtor))
	}

	nobody := "0192f000-0000-7000-8000-000000000000"
	if _, err := e.svc.RefundToSource(ctx, nobody, "staff:admin-a", "fraud check", "k1"); err == nil || errors.Is(err, payouts.ErrPayable) {
		t.Fatalf("refund for an unknown account: %v", err)
	}
	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM payouts.payouts`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d payouts made %v", n, err)
	}

	off := setupWith(t, false, payouts.Config{})
	if _, err := off.svc.RefundToSource(ctx, nobody, "staff:admin-a", "fraud check", "k1"); !errors.Is(err, payouts.ErrUnavailable) {
		t.Fatalf("refund without B2C: %v", err)
	}
}

// A failure in the middle of a refund request rolls it back: nothing is debited and no payout is made.
func TestRefundToSource_AFailureMidRequestDebitsNothing(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	broken := e.service(&flakyClock{inner: e.clk}, &fakeDaraja{})
	u := e.player(t, 1_000)
	e.topUpFrom(t, u, "254708374149")
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'suspended' WHERE id = $1`, u); err != nil {
		t.Fatal(err)
	}
	if _, err := broken.RefundToSource(ctx, u, "staff:admin-a", "fraud check", "k1"); err == nil {
		t.Fatal("refund: no error with a failing clock")
	}
	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM payouts.payouts WHERE user_id = $1`, u).Scan(&n); err != nil || n != 0 || e.balance(t, u) != 950 {
		t.Fatalf("%d payouts, balance %d %v", n, e.balance(t, u), err)
	}
	// the same request goes through once the step can complete
	if p, err := e.svc.RefundToSource(ctx, u, "staff:admin-a", "fraud check", "k1"); err != nil || p.GrossKES != 950 || e.balance(t, u) != 0 {
		t.Fatalf("retry: %+v %v balance %d", p, err, e.balance(t, u))
	}
}
