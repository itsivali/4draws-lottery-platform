//go:build integration

package payouts_test

import (
	"context"
	"errors"
	"testing"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/compliance"
	"github.com/itsivali/4draws-lottery-platform/internal/payouts"
)

// ADR 0032: deciding that the money of a manual payout never left gives it back to the player, so it takes two staff
// members: the one who proposed it cannot confirm their own proposal, and only staff may propose. Until a second person
// confirms, the money stays reserved and the operator sees the open proposal.
func TestSettle_NotPaidNeedsASecondStaffMember(t *testing.T) {
	ctx := context.Background()
	d := &fakeDaraja{}
	e := setupFake(t, d)
	u := e.player(t, 1_000) // 950
	p := e.manualPayout(t, d, u, 300, "k")
	if _, err := e.svc.Settle(ctx, p.ID, "operator:carol", false, "", "not on the statement"); !errors.Is(err, payouts.ErrOperator) {
		t.Fatalf("proposed by someone who is not staff: %v", err)
	}
	got, err := e.svc.Settle(ctx, p.ID, "staff:alice", false, "", "not on the statement")
	if err != nil || got.State != "manual" || got.Proposal == nil || got.Proposal.By != "staff:alice" || got.Proposal.Paid {
		t.Fatalf("proposal: %+v %v", got, err)
	}
	if _, err := e.svc.Settle(ctx, p.ID, "staff:alice", false, "", "still not there"); !errors.Is(err, payouts.ErrSameApprover) {
		t.Fatalf("confirmed by the proposer: %v", err)
	}
	if st, _, _ := e.state(t, p.ID); st != "manual" || e.balance(t, u) != 650 {
		t.Fatalf("on one person's word: %s balance %d", st, e.balance(t, u))
	}
	if _, err := e.svc.Settle(ctx, p.ID, "staff:bob", false, "", "agreed"); err != nil {
		t.Fatal(err)
	}
	if st, _, reason := e.state(t, p.ID); st != "failed" || reason != "settled: not on the statement" || e.balance(t, u) != 950 {
		t.Fatalf("confirmed: %s %q balance %d", st, reason, e.balance(t, u))
	}
	var by string
	if err := e.pool.QueryRow(ctx, `SELECT settled_by FROM payouts.payouts WHERE id = $1`, p.ID).Scan(&by); err != nil || by != "staff:bob" {
		t.Fatalf("settled by %q %v", by, err)
	}
}

// Every operator action names who took it: a rejection without an operator is refused and the payout stays held.
func TestReject_NeedsAnOperator(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 250_000) // 237,500
	held := e.withdraw(t, u, 120_000, "big")
	if _, err := e.svc.Reject(ctx, held.ID, "  ", "unusual activity"); !errors.Is(err, payouts.ErrOperator) {
		t.Fatalf("rejected without an operator: %v", err)
	}
	if st, _, _ := e.state(t, held.ID); st != "held" || e.balance(t, u) != 117_500 {
		t.Fatalf("after a refused rejection: %s balance %d", st, e.balance(t, u))
	}
}

// ADR 0027: while operations are stopped no held payout is released: an approval is refused and not counted, so the
// payout still needs two approvals once operations resume.
func TestApprove_NothingIsReleasedWhileOperationsAreStopped(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	gate := compliance.NewGate(e.pool, e.clk, audit.NewWriter(), compliance.Config{Mode: compliance.Sandbox})
	e.svc.WithGate(gate)
	u := e.player(t, 250_000)
	held := e.withdraw(t, u, 120_000, "big")
	const actor = "staff:0192f000-0000-7000-8000-0000000000aa"
	if _, err := gate.Stop(ctx, actor, "drill"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Approve(ctx, held.ID, "staff:alice"); !errors.Is(err, compliance.ErrOperationsStopped) {
		t.Fatalf("approved while stopped: %v", err)
	}
	list, _, err := e.svc.InState(ctx, "held", 10, "")
	if err != nil || len(list) != 1 || len(list[0].Approvals) != 0 || len(e.q.take()) != 0 {
		t.Fatalf("after a refused approval: %+v %v", list, err)
	}
	if _, err := gate.Resume(ctx, compliance.Sandbox, actor, "drill over"); err != nil {
		t.Fatal(err)
	}
	one, err := e.svc.Approve(ctx, held.ID, "staff:alice")
	if err != nil || one.State != "held" || len(one.Approvals) != 1 {
		t.Fatalf("first approval after resume: %+v %v", one, err)
	}
	if _, err := e.svc.Approve(ctx, held.ID, "staff:bob"); err != nil {
		t.Fatal(err)
	}
	if st := e.run(t, held.ID); st != "succeeded" || len(e.mock.Payouts()) != 1 {
		t.Fatalf("after resume: %s, %d sent", st, len(e.mock.Payouts()))
	}
}
