//go:build integration

package payments_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/compliance"
	"github.com/itsivali/lottery-platform/internal/payments"
)

// ADR 0027: once operations are stopped no prompt goes out (a top-up recorded before the stop fails without one),
// no new top-up starts and the paybill refuses payments; resuming lets them through again.
func TestOperatingMode_AStopSendsNoPromptAndRefusesNewMoney(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	gate := compliance.NewGate(e.pool, e.clk, audit.NewWriter(), compliance.Config{Mode: compliance.Sandbox})
	e.svc.WithGate(gate)
	user, msisdn := e.provedPlayer(t)

	queued := e.topUp(t, user, 100) // recorded before the stop; its push job has not run
	const actor = "staff:0192f000-0000-7000-8000-0000000000aa"
	if _, err := gate.Stop(ctx, actor, "drill"); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if status, reason := e.status(t, queued.ID); status != "failed" || reason == nil || *reason != "operations_stopped" {
		t.Fatalf("queued top-up: %s %v, want failed operations_stopped", status, reason)
	}
	if n := len(e.mock.Pushes()); n != 0 {
		t.Fatalf("%d prompts went out while stopped", n)
	}
	if _, err := e.svc.StartTopUp(ctx, user, 100, "after-stop"); !errors.Is(err, compliance.ErrOperationsStopped) {
		t.Fatalf("top-up while stopped: %v", err)
	}
	raw, _ := json.Marshal(confirmationBody("STOP000001", 100, local(msisdn), payer))
	if code, err := e.svc.ValidateC2B(ctx, raw); err != nil || code != payments.C2BOtherError {
		t.Fatalf("paybill validation while stopped: %s %v", code, err)
	}

	e.clk.Advance(e.svc.TopUpCooldown() + 1)
	if _, err := gate.Resume(ctx, compliance.Sandbox, actor, "drill over"); err != nil {
		t.Fatal(err)
	}
	if code, err := e.svc.ValidateC2B(ctx, raw); err != nil || code != payments.C2BAccept {
		t.Fatalf("paybill validation after resume: %s %v", code, err)
	}
	e.topUp(t, user, 100)
	e.run(t)
	if n := len(e.mock.Pushes()); n != 1 {
		t.Fatalf("after resume: %d prompts, want 1", n)
	}
}
