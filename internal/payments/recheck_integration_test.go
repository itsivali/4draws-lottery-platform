//go:build integration

package payments_test

import (
	"context"
	"testing"
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
