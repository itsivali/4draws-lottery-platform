//go:build integration

package payouts_test

import (
	"context"
	"errors"
	"testing"

	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/payouts"
)

// ADR 0030: the send job checks the account again, as the money is about to leave. A payout whose account was
// suspended, lost its verified identity, or proved another number since the payout was made is not sent: it goes back
// to held, with why, and the money stays reserved until an operator approves (once the account may be paid) or rejects.
func TestSend_APayoutWhoseAccountCannotBePaidNowIsHeldNotSent(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	for _, tc := range []struct {
		name, change, reason string
	}{
		{"suspended", `UPDATE identity.users SET status = 'suspended' WHERE id = $1`, "payee_ineligible: account not active"},
		{"unverified", `UPDATE identity.users SET kyc_status = 'manual_review' WHERE id = $1`, "payee_ineligible: identity not verified"},
		{"number changed", `UPDATE identity.users SET msisdn = '25471' || lpad((abs(hashtext(id::text)) % 10000000)::text, 7, '0') WHERE id = $1`, "payee_ineligible: number changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := e.player(t, 1_000)
			p := e.withdraw(t, u, 500, "w-"+tc.name)
			if _, err := e.pool.Exec(ctx, tc.change, u); err != nil {
				t.Fatal(err)
			}
			if st := e.run(t, p.ID); st != "held" {
				t.Fatalf("state %s, want held", st)
			}
			if _, _, reason := e.state(t, p.ID); reason != tc.reason {
				t.Fatalf("reason %q, want %q", reason, tc.reason)
			}
			if n := e.balance(t, u); n != 950-500 {
				t.Fatalf("balance %d: the money stays reserved while held", n)
			}
		})
	}
	if n := len(e.mock.Payouts()); n != 0 {
		t.Fatalf("%d B2C requests went out", n)
	}
}

// ADR 0030: approving a held payout checks the account again. While the account cannot be paid the approval is refused
// (the payout stays held, and can be rejected); once it can, the approval sends it.
func TestApprove_IsRefusedWhileTheAccountCannotBePaid(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 250_000)
	held := e.withdraw(t, u, 120_000, "big")
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'suspended' WHERE id = $1`, u); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Approve(ctx, held.ID, "staff:alice")
	if !errors.Is(err, payouts.ErrPayeeIneligible) || !errors.Is(err, identity.ErrAccountSuspended) {
		t.Fatalf("approve for a suspended account: %v", err)
	}
	if st, _, _ := e.state(t, held.ID); st != "held" || len(e.q.take()) != 0 {
		t.Fatalf("after a refused approval: %s", st)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'active' WHERE id = $1`, u); err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"staff:alice", "staff:bob"} {
		if _, err := e.svc.Approve(ctx, held.ID, who); err != nil {
			t.Fatal(err)
		}
	}
	if st := e.run(t, held.ID); st != "succeeded" {
		t.Fatalf("approved once reinstated: %s", st)
	}

	// held by the send job, then released: the reason it was held for no longer shows on the payout
	again := e.withdraw(t, u, 500, "small")
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'suspended' WHERE id = $1`, u); err != nil {
		t.Fatal(err)
	}
	if st := e.run(t, again.ID); st != "held" {
		t.Fatalf("suspended at send: %s", st)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'active' WHERE id = $1`, u); err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"staff:alice", "staff:bob"} {
		if _, err := e.svc.Approve(ctx, again.ID, who); err != nil {
			t.Fatal(err)
		}
	}
	if st := e.run(t, again.ID); st != "succeeded" {
		t.Fatalf("released: %s", st)
	}
	if _, _, reason := e.state(t, again.ID); reason != "" {
		t.Fatalf("reason after release %q, want none", reason)
	}
}
