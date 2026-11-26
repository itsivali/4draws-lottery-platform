//go:build integration

package payments_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/itsivali/4draws-lottery-platform/internal/aml"
	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/payments"
)

// ADR 0030: the prompt job decides again what StartTopUp decided. A player who excluded themself, or whose account was
// suspended, after the top-up was recorded and before its prompt went out gets no prompt: the top-up fails with why.
func TestPush_ATopUpWhosePlayerMayNoLongerTopUpSendsNoPrompt(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	excluded, _ := e.provedPlayer(t)
	suspended, _ := e.provedPlayer(t)
	fine, _ := e.provedPlayer(t)
	a := e.topUp(t, excluded, 100)
	b := e.topUp(t, suspended, 100)
	c := e.topUp(t, fine, 100)
	if err := e.rg.SelfExclude(ctx, excluded, "6m"); err != nil {
		t.Fatal(err)
	}
	if err := e.ids.Suspend(ctx, suspended, "staff:0192f000-0000-7000-8000-0000000000aa", "fraud check", true); err != nil {
		t.Fatal(err)
	}
	e.run(t)

	for _, tc := range []struct {
		id, want string
	}{{a.ID, "player_restricted"}, {b.ID, "account_ineligible"}} {
		if status, reason := e.status(t, tc.id); status != "failed" || reason == nil || *reason != tc.want {
			t.Errorf("top-up %s: %s %v, want failed %s", tc.id, status, reason, tc.want)
		}
	}
	if status, _ := e.status(t, c.ID); status != "stk_sent" {
		t.Errorf("eligible player's top-up: %s, want stk_sent", status)
	}
	if p := e.mock.Pushes(); len(p) != 1 {
		t.Fatalf("%d prompts went out, want 1 (only the eligible player's)", len(p))
	}
}

// ADR 0033: while a monitoring case holds a player's money, no top-up starts, a recorded one sends no prompt, and the
// paybill tells Safaricom to refuse their payments. The player is not told why.
func TestHold_AMonitoringHoldStopsMoneyComingIn(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	monitor := aml.NewService(e.pool, e.clk, audit.NewWriter())
	e.svc.WithHolds(monitor)
	held, msisdn := e.provedPlayer(t)
	queued := e.topUp(t, held, 100) // recorded before the hold
	c, err := monitor.OpenManual(ctx, held, "staff:alice", "bank enquiry")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := monitor.Hold(ctx, c.ID, "staff:alice", "until the bank answers"); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if status, reason := e.status(t, queued.ID); status != "failed" || reason == nil || *reason != "account_under_review" {
		t.Fatalf("queued top-up: %s %v", status, reason)
	}
	if n := len(e.mock.Pushes()); n != 0 {
		t.Fatalf("%d prompts went out", n)
	}
	e.clk.Advance(e.svc.TopUpCooldown() + 1)
	if _, err := e.svc.StartTopUp(ctx, held, 100, "while-held"); !errors.Is(err, aml.ErrUnderReview) {
		t.Fatalf("top-up while held: %v", err)
	}
	raw, _ := json.Marshal(confirmationBody("HOLD000001", 100, local(msisdn), payer))
	if code, err := e.svc.ValidateC2B(ctx, raw); err != nil || code != payments.C2BOtherError {
		t.Fatalf("paybill validation while held: %s %v", code, err)
	}
	if _, err := monitor.Close(ctx, c.ID, "staff:bob", "no concern"); err != nil {
		t.Fatal(err)
	}
	if code, err := e.svc.ValidateC2B(ctx, raw); err != nil || code != payments.C2BAccept {
		t.Fatalf("paybill validation after the case closed: %s %v", code, err)
	}
}

// ADR 0030: the prompt goes only to the number the account claims now. A top-up recorded before the account's number
// changed sends no prompt to the old one: it fails with why.
func TestPush_ATopUpRecordedBeforeTheNumberChangedSendsNoPrompt(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, _ := e.player(t)
	tu := e.topUp(t, user, 100)
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET msisdn = '254799000111' WHERE id = $1`, user); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if status, reason := e.status(t, tu.ID); status != "failed" || reason == nil || *reason != "number_changed" {
		t.Fatalf("top-up: %s %v, want failed number_changed", status, reason)
	}
	if p := e.mock.Pushes(); len(p) != 0 {
		t.Fatalf("prompts = %+v, want none", p)
	}
}
