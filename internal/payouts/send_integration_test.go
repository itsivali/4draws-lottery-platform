//go:build integration

package payouts_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/identity"
	"github.com/itsivali/4draws-lottery-platform/internal/ledger"
	"github.com/itsivali/4draws-lottery-platform/internal/payments/daraja"
	"github.com/itsivali/4draws-lottery-platform/internal/payouts"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
	"github.com/itsivali/4draws-lottery-platform/internal/wallet"
)

// service is another worker on the env's database and queue, with its own clock and Daraja (nil: no B2C).
func (e *env) service(clk clock.Clock, d payouts.Daraja) *payouts.Service {
	aw := audit.NewWriter()
	ls := ledger.NewStore()
	ids := identity.NewService(e.pool, e.clk, nil, aw, identity.Config{AuthKey: []byte("0123456789abcdef0123456789abcdef")})
	return payouts.NewService(e.pool, clk, d, e.q, ids, wallet.NewService(ls), ls, aw, payouts.Config{MaxStatusRequests: 3}, wallet.ErrInsufficientFunds)
}

// slowDaraja runs during while the B2C request is in flight, standing in for whatever happens meanwhile.
type slowDaraja struct {
	*fakeDaraja
	during func()
	sent   atomic.Int32
}

func (d *slowDaraja) B2C(ctx context.Context, r daraja.B2CRequest) (daraja.B2CAccepted, error) {
	d.sent.Add(1)
	d.during()
	return d.fakeDaraja.B2C(ctx, r)
}

// The send job may be delivered twice: the second finds the payout no longer pending and sends nothing.
func TestSend_ADuplicateSendJobSendsNothing(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 1_000)
	p := e.withdraw(t, u, 300, "k")
	e.q.take()
	for range 2 {
		if err := e.svc.Send(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
	}
	if st := e.run(t, p.ID); st != "succeeded" {
		t.Fatalf("state %s", st)
	}
	if err := e.svc.Send(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(e.mock.Payouts()); n != 1 || e.balance(t, u) != 650 {
		t.Fatalf("%d B2C requests, balance %d", n, e.balance(t, u))
	}
}

// A request slow enough that the sweep took the payout as interrupted: the sweep's decision stands. The late answer to
// the request is not recorded over it, nothing is sent again, and the payout is settled by asking Daraja.
func TestSend_ASweepThatTookOverDuringTheRequestStands(t *testing.T) {
	ctx := context.Background()
	e := setupFake(t, &fakeDaraja{})
	d := &slowDaraja{fakeDaraja: &fakeDaraja{}}
	e.svc = e.service(e.clk, d)
	e.hooks = e.svc
	d.during = func() {
		e.clk.Advance(payouts.DefaultStaleAfter + time.Minute)
		if err := e.svc.Sweep(ctx); err != nil {
			t.Error(err)
		}
	}
	u := e.player(t, 1_000)
	msisdn := e.msisdn(t, u)
	p := e.withdraw(t, u, 300, "k")
	e.q.take()
	if err := e.svc.Send(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if st, _, reason := e.state(t, p.ID); st != "timed_out" || reason != "send_interrupted" {
		t.Fatalf("after the slow request: %s %s", st, reason)
	}
	var conv *string
	if err := e.pool.QueryRow(ctx, `SELECT conversation_id FROM payouts.payouts WHERE id = $1`, p.ID).Scan(&conv); err != nil || conv != nil {
		t.Fatalf("the late answer was recorded: %v %v", conv, err)
	}
	if jobs := e.q.take(); len(jobs) != 1 || jobs[0].kind != "verify" {
		t.Fatalf("jobs %+v: only the sweep's verification", jobs)
	}
	e.clk.Advance(payouts.DefaultResultWait + time.Second)
	if _, err := e.svc.Verify(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	var orig string
	if err := e.pool.QueryRow(ctx, `SELECT status_originator FROM payouts.payouts WHERE id = $1`, p.ID).Scan(&orig); err != nil {
		t.Fatal(err)
	}
	if err := e.hooks.HandleCallback(ctx, payouts.CallbackStatusResult, completed(orig, "MBCSLOW001", msisdn+" - Jane", 300)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Verify(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if st, receipt, _ := e.state(t, p.ID); st != "succeeded" || receipt != "MBCSLOW001" || d.sent.Load() != 1 || e.balance(t, u) != 650 {
		t.Fatalf("settled %s %s, sent %d, balance %d", st, receipt, d.sent.Load(), e.balance(t, u))
	}
}

// A worker without B2C neither queues a payout to be sent nor asks about one: the money stays reserved, the payout
// where it was, for a worker that can.
func TestSweepAndVerify_WithoutB2CNothingMoves(t *testing.T) {
	ctx := context.Background()
	d := &fakeDaraja{}
	e := setupFake(t, d)
	off := e.service(e.clk, nil)
	u := e.player(t, 1_000)
	pending := e.withdraw(t, u, 300, "a")
	sent := e.withdraw(t, u, 200, "b")
	e.q.take()
	if err := e.svc.Send(ctx, sent.ID); err != nil {
		t.Fatal(err)
	}
	e.q.take()
	e.clk.Advance(payouts.DefaultStaleAfter + time.Minute)
	if err := off.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	for _, j := range e.q.take() {
		if j.kind == "send" {
			t.Fatalf("queued to send without B2C: %+v", j)
		}
	}
	if _, err := off.Verify(ctx, sent.ID); !errors.Is(err, payouts.ErrUnavailable) {
		t.Fatalf("verify without B2C: %v", err)
	}
	if a, _, _ := e.state(t, pending.ID); a != "pending" {
		t.Fatalf("pending payout %s", a)
	}
	var st string
	var asked int
	if err := e.pool.QueryRow(ctx, `SELECT status, status_requests FROM payouts.payouts WHERE id = $1`, sent.ID).Scan(&st, &asked); err != nil ||
		st != "sent" || asked != 0 || e.balance(t, u) != 450 {
		t.Fatalf("sent payout %s asked %d balance %d %v", st, asked, e.balance(t, u), err)
	}
	// a worker with B2C picks the pending one up
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	queued := false
	for _, j := range e.q.take() {
		queued = queued || (j.kind == "send" && j.id == pending.ID)
	}
	if !queued {
		t.Fatal("the pending payout was not queued by a worker with B2C")
	}
}

// A failure in the middle of verification or settlement rolls that step back: no settlement, no status request
// recorded, no hand-off, no claim and no money moved. The next run of the step does it.
func TestVerify_AFailureMidStepLeavesNothingBehind(t *testing.T) {
	ctx := context.Background()
	d := &fakeDaraja{}
	e := setupFake(t, d)
	flaky := &flakyClock{inner: e.clk}
	broken := e.service(flaky, d)
	requests := func(id string) int {
		t.Helper()
		var n int
		if err := e.pool.QueryRow(ctx, `SELECT status_requests FROM payouts.payouts WHERE id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// settling from a Transaction Status answer
	u := e.player(t, 1_000)
	p := e.withdraw(t, u, 300, "k")
	orig := e.sendAndAsk(t, p.ID)
	if err := e.hooks.HandleCallback(ctx, payouts.CallbackStatusResult, completed(orig, "MBCFLAKY01", e.msisdn(t, u)+" - Jane", 300)); err != nil {
		t.Fatal(err)
	}
	flaky.left.Store(0)
	if _, err := broken.Verify(ctx, p.ID); err == nil {
		t.Fatal("settle: no error")
	}
	if st, receipt, _ := e.state(t, p.ID); st != "sent" || receipt != "" || e.audits(t, "payout.succeeded", p.ID) != 0 {
		t.Fatalf("after a failed settlement: %s %q", st, receipt)
	}
	if _, err := e.svc.Verify(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if st, receipt, _ := e.state(t, p.ID); st != "succeeded" || receipt != "MBCFLAKY01" || e.balance(t, u) != 650 {
		t.Fatalf("settled %s %q balance %d", st, receipt, e.balance(t, u))
	}

	// recording that Daraja was asked
	v := e.player(t, 1_000)
	q := e.withdraw(t, v, 300, "k")
	if err := e.svc.Send(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(payouts.DefaultResultWait + time.Second)
	flaky.left.Store(1)
	if _, err := broken.Verify(ctx, q.ID); err == nil {
		t.Fatal("ask: no error")
	}
	if st, _, _ := e.state(t, q.ID); st != "sent" || requests(q.ID) != 0 {
		t.Fatalf("after a failed ask: %s, %d requests recorded", st, requests(q.ID))
	}

	// handing an unconfirmed payout to an operator
	for range 3 {
		if _, err := e.svc.Verify(ctx, q.ID); err != nil {
			t.Fatal(err)
		}
		e.clk.Advance(payouts.DefaultStatusWait + time.Second)
	}
	flaky.left.Store(1)
	if _, err := broken.Verify(ctx, q.ID); err == nil {
		t.Fatal("hand-off: no error")
	}
	if st, _, _ := e.state(t, q.ID); st != "sent" || requests(q.ID) != 3 || e.audits(t, "payout.manual", q.ID) != 0 {
		t.Fatalf("after a failed hand-off: %s", st)
	}
	if _, err := e.svc.Verify(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	if st, _, reason := e.state(t, q.ID); st != "manual" || reason != "unconfirmed" {
		t.Fatalf("handed off: %s %s", st, reason)
	}

	// an operator's claim, and M-Pesa's answer to it
	flaky.left.Store(0)
	if _, err := broken.Settle(ctx, q.ID, "staff:alice", true, "MBCFLAKY02", ""); err == nil {
		t.Fatal("claim: no error")
	}
	if _, err := broken.Settle(ctx, q.ID, "staff:alice", false, "", "not on the statement"); err == nil {
		t.Fatal("proposal: no error")
	}
	list, _, err := e.svc.InState(ctx, "manual", 10, "")
	if err != nil || len(list) != 1 || list[0].Check != nil || list[0].Proposal != nil {
		t.Fatalf("after a failed claim and proposal: %+v %v", list, err)
	}
	orig = e.claim(t, q.ID, "staff:alice", "MBCFLAKY02")
	if err := e.hooks.HandleCallback(ctx, payouts.CallbackStatusResult, statusAnswer(orig, q.ID, 0, "Completed", "MBCFLAKY02", 300, e.msisdn(t, v)+" - Jo")); err != nil {
		t.Fatal(err)
	}
	flaky.left.Store(0)
	if _, err := broken.Verify(ctx, q.ID); err == nil {
		t.Fatal("check: no error")
	}
	if c := e.latestCheck(t, q.ID); c.Outcome != "" || e.balance(t, v) != 650 {
		t.Fatalf("after a failed check: %+v balance %d", c, e.balance(t, v))
	}
	if _, err := e.svc.Verify(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	if st, receipt, _ := e.state(t, q.ID); st != "succeeded" || receipt != "MBCFLAKY02" || e.balance(t, v) != 650 {
		t.Fatalf("confirmed %s %q balance %d", st, receipt, e.balance(t, v))
	}
}

// racingStatus runs during (once) while a Transaction Status request is in flight.
type racingStatus struct {
	*fakeDaraja
	during func()
}

func (d *racingStatus) B2CStatus(ctx context.Context, receipt, originator, occasion string) (daraja.StatusRequest, error) {
	if f := d.during; f != nil {
		d.during = nil
		f()
	}
	return d.fakeDaraja.B2CStatus(ctx, receipt, originator, occasion)
}

// The answer to an earlier status question can arrive, and settle the payout through another worker, while this worker
// is asking again: the settlement stands, and the new question is not recorded over it (its answer will match nothing).
func TestVerify_ASettlementWhileAskingAgainStands(t *testing.T) {
	ctx := context.Background()
	d := &fakeDaraja{}
	e := setupFake(t, d)
	racing := &racingStatus{fakeDaraja: d}
	asker := e.service(e.clk, racing)
	u := e.player(t, 1_000)
	p := e.withdraw(t, u, 300, "k")
	first := e.sendAndAsk(t, p.ID)
	e.clk.Advance(payouts.DefaultStatusWait + time.Second) // no answer yet: time to ask again
	racing.during = func() {
		if err := e.hooks.HandleCallback(ctx, payouts.CallbackStatusResult, completed(first, "MBCRACE001", e.msisdn(t, u)+" - Jane", 300)); err != nil {
			t.Error(err)
		}
		if _, err := e.svc.Verify(ctx, p.ID); err != nil {
			t.Error(err)
		}
	}
	if _, err := asker.Verify(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	var orig string
	var asked int
	if err := e.pool.QueryRow(ctx, `SELECT status_originator, status_requests FROM payouts.payouts WHERE id = $1`, p.ID).Scan(&orig, &asked); err != nil {
		t.Fatal(err)
	}
	if st, receipt, _ := e.state(t, p.ID); st != "succeeded" || receipt != "MBCRACE001" || orig != first || asked != 1 || e.balance(t, u) != 650 {
		t.Fatalf("%s %q originator %s (first %s) asked %d balance %d", st, receipt, orig, first, asked, e.balance(t, u))
	}
}
