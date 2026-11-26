//go:build integration

package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/mail"
	"github.com/itsivali/4draws-lottery-platform/internal/winnotice"
)

// refusing is a mail server that refuses every message.
type refusing struct{ tried int }

func (r *refusing) Send(context.Context, mail.Message) error {
	r.tried++
	return errors.New("mail: refused")
}

func TestWinNotice_AWinnerWithAConfirmedAddressIsEmailedOnce(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a, b := e.signIn(t), e.signIn(t)
	for _, s := range []session{a, b} {
		e.canPlay(t, s, 1000)
		for range 3 {
			if r := e.do(t, http.MethodPost, "/v1/plays", s.access, nil); r.Code != http.StatusCreated {
				t.Fatalf("play: %d %s", r.Code, r.Raw)
			}
		}
	}
	// A confirmed their address; B only typed one
	e.exec(t, `UPDATE identity.users SET email_verified_at = now() WHERE id = $1`, e.userID(t, a))
	draw := e.conductDaily(t)

	// the worker's rights on the same database, as conductDaily uses
	cfg := e.pool.Config().Copy()
	cfg.AfterConnect = nil
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var prizes, forA int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE en.user_id = $2)
		FROM draws.draw_results r JOIN draws.draw_entries en ON en.id = r.entry_id WHERE r.draw_id = $1`, draw, e.userID(t, a)).Scan(&prizes, &forA); err != nil || prizes == 0 {
		t.Fatalf("%d prizes (%v)", prizes, err)
	}

	// a conducted draw is not announced until two officials have approved it
	if n, err := winnotice.NewService(pool, e.clk, audit.NewWriter(), &outbox{}, "").Notify(ctx); err != nil || n != 0 {
		t.Fatalf("before approval: sent %d (%v)", n, err)
	}
	for _, official := range []string{"operator:carol", "operator:dan"} {
		if _, err := e.deps.StaffServices.Draws.Approve(ctx, draw, official); err != nil {
			t.Fatalf("approve: %v", err)
		}
	}

	// while the mail server refuses, nothing is recorded for A's prizes: they are tried again
	down := &refusing{}
	if n, err := winnotice.NewService(pool, e.clk, audit.NewWriter(), down, "https://app.example.com").Notify(ctx); err != nil || n != 0 || down.tried != forA {
		t.Fatalf("mail server down: sent %d, tried %d of %d (%v)", n, down.tried, forA, err)
	}
	sent := &outbox{}
	svc := winnotice.NewService(pool, e.clk, audit.NewWriter(), sent, "https://app.example.com/")
	n, err := svc.Notify(ctx)
	if err != nil || n != forA || len(sent.msgs) != forA {
		t.Fatalf("sent %d, want %d (%v)", n, forA, err)
	}
	for _, m := range sent.msgs {
		if m.To[0] != a.email || !strings.Contains(m.Subject, "you won KES ") || !strings.Contains(m.Text, "https://app.example.com/draws/"+draw) ||
			!strings.Contains(m.Text, "daily draw") || !strings.Contains(m.HTML, "cid:logo.png") {
			t.Fatalf("message to %v: %s\n%s", m.To, m.Subject, m.Text)
		}
		for _, never := range []string{"play again", "Play again", "try your luck"} {
			if strings.Contains(m.Text, never) {
				t.Fatalf("the notice invites more play: %q", never)
			}
		}
	}
	// every prize has one record of what was done: A's emailed, B's skipped with the reason, each audited
	var emailed, skipped, audited int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE n.outcome = 'emailed'),
		count(*) FILTER (WHERE n.outcome = 'skipped' AND n.reason = 'the email address is not confirmed'),
		(SELECT count(*) FROM audit.audit_log WHERE action = 'draws.prize_notice')
		FROM draws.prize_notices n JOIN draws.draw_results r ON r.id = n.result_id WHERE r.draw_id = $1`, draw).Scan(&emailed, &skipped, &audited); err != nil ||
		emailed != forA || skipped != prizes-forA || audited != prizes {
		t.Fatalf("emailed %d of %d, skipped %d of %d, audited %d (%v)", emailed, forA, skipped, prizes-forA, audited, err)
	}
	// nobody is told twice
	if n, err := svc.Notify(ctx); err != nil || n != 0 || len(sent.msgs) != forA {
		t.Fatalf("a second run sent %d (%v)", n, err)
	}
}
