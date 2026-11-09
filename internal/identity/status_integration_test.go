//go:build integration

package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/rg"
)

// ADR 0030: every status an account has had is kept, with who changed it and why, and the history cannot be rewritten.
// A change made outside the service (straight in the database) is still recorded, without a name.
func TestStatusHistory_KeepsEveryChangeWithWhoAndWhy(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tok := e.login(t, e.newMSISDN(), phone)
	p, err := e.svc.Authenticate(ctx, tok.Access)
	if err != nil {
		t.Fatal(err)
	}
	const staff = "staff:0192f000-0000-7000-8000-0000000000aa"
	e.clk.Advance(time.Hour)
	if err := e.svc.Suspend(ctx, p.UserID, staff, "chargeback inquiry", true); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(time.Hour)
	if err := e.svc.Suspend(ctx, p.UserID, staff, "inquiry closed", false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET status = 'suspended', updated_at = $2 WHERE id = $1`, p.UserID, start.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// a change that leaves the status alone adds nothing
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET language = language WHERE id = $1`, p.UserID); err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.StatusHistory(ctx, p.UserID)
	if err != nil {
		t.Fatal(err)
	}
	want := []identity.StatusChange{
		{From: "", To: "active", ActorType: "player", Actor: p.UserID, Reason: "signed up", At: start},
		{From: "active", To: "suspended", ActorType: "staff", Actor: staff, Reason: "chargeback inquiry", At: start.Add(time.Hour)},
		{From: "suspended", To: "active", ActorType: "staff", Actor: staff, Reason: "inquiry closed", At: start.Add(2 * time.Hour)},
		{From: "active", To: "suspended", At: start.Add(3 * time.Hour)},
	}
	if len(got) != len(want) {
		t.Fatalf("history %+v, want %d changes", got, len(want))
	}
	for i := range want {
		if g := got[i]; g.From != want[i].From || g.To != want[i].To || g.ActorType != want[i].ActorType || g.Actor != want[i].Actor ||
			g.Reason != want[i].Reason || !g.At.Equal(want[i].At) {
			t.Errorf("change %d = %+v, want %+v", i, g, want[i])
		}
	}
	for name, sql := range map[string]string{
		"rewrite": `UPDATE identity.user_status_history SET reason = 'nothing to see' WHERE user_id = $1`,
		"delete":  `DELETE FROM identity.user_status_history WHERE user_id = $1`,
	} {
		if _, err := e.pool.Exec(ctx, sql, p.UserID); err == nil {
			t.Errorf("%s the history: allowed", name)
		}
	}
	if _, err := e.svc.StatusHistory(ctx, "not-a-uuid"); !errors.Is(err, identity.ErrNoSuchPlayer) {
		t.Errorf("history of a malformed id: %v", err)
	}
}

// ADR 0030: excluding oneself signs one out everywhere, at once; signing in again is allowed (to withdraw), but the
// sessions that were open stay ended. A time-out is a pause and keeps the session.
func TestSelfExclusion_EndsEverySession(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	svc := rg.NewService(e.pool, e.clk, audit.NewWriter(), rg.Config{CoolingOff: 24 * time.Hour}).WithSessionEnder(e.svc)

	paused := e.newMSISDN()
	pt := e.login(t, paused, phone)
	pp, err := e.svc.Authenticate(ctx, pt.Access)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Timeout(ctx, pp.UserID, "24h"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Authenticate(ctx, pt.Access); err != nil {
		t.Fatalf("a time-out ended the session: %v", err)
	}

	tablet := identity.Device{Fingerprint: "fp-tablet", Platform: "android"}
	msisdn := e.newMSISDN()
	first := e.login(t, msisdn, phone)
	second := e.login(t, msisdn, tablet)
	p, err := e.svc.Authenticate(ctx, first.Access)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SelfExclude(ctx, p.UserID, "6m"); err != nil {
		t.Fatal(err)
	}
	for name, tok := range map[string]identity.Tokens{"phone": first, "tablet": second} {
		if _, err := e.svc.Authenticate(ctx, tok.Access); err == nil {
			t.Errorf("%s session still authenticates after self-exclusion", name)
		}
		if _, err := e.svc.Refresh(ctx, tok.Refresh, phone.Fingerprint); err == nil {
			t.Errorf("%s session still refreshes after self-exclusion", name)
		}
	}
	var reasons []string
	rows, err := e.pool.Query(ctx, `SELECT DISTINCT revoked_reason FROM identity.sessions WHERE user_id = $1`, p.UserID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var r *string
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		if r != nil {
			reasons = append(reasons, *r)
		}
	}
	if len(reasons) != 1 || reasons[0] != "self_exclusion" {
		t.Fatalf("revoked reasons %v, want self_exclusion", reasons)
	}
	if _, err := e.svc.Login(ctx, msisdn, goodPassword, phone); err != nil {
		t.Fatalf("signing in again after self-exclusion: %v", err)
	}
}
