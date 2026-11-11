//go:build integration

package compliance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/compliance"
)

func (e *env) assertOp(t *testing.T, g *compliance.Gate, op compliance.Op) error {
	t.Helper()
	return pgx.BeginFunc(context.Background(), e.pool, func(tx pgx.Tx) error { return g.Assert(context.Background(), tx, op) })
}

func TestGate_APausedAreaRefusesOnlyItsOwnOperations(t *testing.T) {
	e := setup(t)
	g := e.gate(compliance.Sandbox, "")
	ctx := context.Background()
	all := []compliance.Op{compliance.OpTopUp, compliance.OpPush, compliance.OpPaybill, compliance.OpPlay, compliance.OpWithdrawal, compliance.OpPayout}

	sws, err := g.Switches(ctx)
	if err != nil || len(sws) != 4 {
		t.Fatalf("switches = %+v %v", sws, err)
	}
	for _, sw := range sws {
		if sw.Paused || sw.At != nil {
			t.Fatalf("nothing switched yet: %+v", sw)
		}
	}
	if _, err := g.SetSwitch(ctx, compliance.AreaPlays, true, alice, " "); !errors.Is(err, compliance.ErrInvalid) {
		t.Fatalf("pause without a reason: %v", err)
	}
	if _, err := g.SetSwitch(ctx, "bets", true, alice, "why not"); !errors.Is(err, compliance.ErrInvalid) {
		t.Fatalf("unknown area: %v", err)
	}

	if _, err := g.SetSwitch(ctx, compliance.AreaTopUps, true, alice, "M-Pesa outage"); err != nil {
		t.Fatal(err)
	}
	for _, op := range all {
		err := e.assertOp(t, g, op)
		refused := op == compliance.OpTopUp || op == compliance.OpPush || op == compliance.OpPaybill
		if refused != errors.Is(err, compliance.ErrOperationsStopped) {
			t.Errorf("top-ups paused: %s = %v", op, err)
		}
	}

	e.clk.Advance(time.Minute)
	if _, err := g.SetSwitch(ctx, compliance.AreaTopUps, false, bob, "M-Pesa back"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.SetSwitch(ctx, compliance.AreaPayouts, true, alice, "B2C float low"); err != nil {
		t.Fatal(err)
	}
	for _, op := range all {
		if err := e.assertOp(t, g, op); (op == compliance.OpPayout) != errors.Is(err, compliance.ErrOperationsStopped) {
			t.Errorf("payouts paused: %s = %v", op, err)
		}
	}
	sws, err = g.Switches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sw := range sws {
		switch sw.Area {
		case compliance.AreaTopUps:
			if sw.Paused || sw.ChangedBy != bob || sw.Reason != "M-Pesa back" {
				t.Errorf("top-ups: %+v", sw)
			}
		case compliance.AreaPayouts:
			if !sw.Paused || sw.ChangedBy != alice || sw.At == nil {
				t.Errorf("payouts: %+v", sw)
			}
		}
	}

	// a stop still refuses everything, whatever the switches say
	if _, err := g.Stop(ctx, alice, "incident"); err != nil {
		t.Fatal(err)
	}
	if err := e.assertOp(t, g, compliance.OpPlay); !errors.Is(err, compliance.ErrOperationsStopped) {
		t.Errorf("stopped: play = %v", err)
	}

	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM audit.audit_log WHERE action IN ('compliance.area_paused','compliance.area_resumed')`).Scan(&n); err != nil || n != 3 {
		t.Errorf("audited %d %v, want 3", n, err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE compliance.area_switches SET paused = false`); err == nil {
		t.Error("switch records must be append-only")
	}
}
