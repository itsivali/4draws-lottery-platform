// Package winnotice tells the winner of a draw prize by email (ADR 0042), so they need not come back and look. Each
// prize is announced at most once; a winner with no confirmed address is not written to, and that is recorded too.
package winnotice

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/mail"
)

// GiveUpAfter is how long a notice that the mail server keeps refusing is tried for. After it the prize is recorded
// as not announced: the winner still sees it in the app and is still paid.
const GiveUpAfter = 72 * time.Hour

// Service sends the notices.
type Service struct {
	pool   *pgxpool.Pool
	clk    clock.Clock
	audit  *audit.Writer
	sender mail.Sender
	site   string
}

// NewService returns a Service that sends through sender; site is the player web app's origin, for the link to the
// draw and the footer.
func NewService(pool *pgxpool.Pool, clk clock.Clock, a *audit.Writer, sender mail.Sender, site string) *Service {
	return &Service{pool: pool, clk: clk, audit: a, sender: sender, site: strings.TrimRight(site, "/")}
}

// prize is one prize whose winner has not been told.
type prize struct {
	resultID, drawID, drawType, userID, status string
	periodStart, periodEnd, wonAt              time.Time
	rank                                       int
	amount                                     int64
	email                                      *string
	confirmed                                  bool
}

// pending lists the prizes of approved draws that have no notice yet, oldest first.
const pending = `
SELECT r.id::text, d.id::text, d.draw_type, d.period_start, d.period_end, r.prize_rank, r.amount_kes, r.created_at,
       u.id::text, u.status, u.email, u.email_verified_at IS NOT NULL
FROM draws.draw_results r
JOIN draws.draws d ON d.id = r.draw_id
JOIN draws.draw_entries e ON e.id = r.entry_id
JOIN identity.users u ON u.id = e.user_id
WHERE d.status IN ('approved','paid')
  AND NOT EXISTS (SELECT 1 FROM draws.prize_notices n WHERE n.result_id = r.id)
ORDER BY r.created_at, r.id
LIMIT 200`

// Notify emails the winner of every approved prize that has no notice yet, and records what it did. A winner
// without a confirmed address, or whose account is closed, is skipped for good. A notice the mail server refuses is
// tried again on the next run, for GiveUpAfter. It returns how many emails it sent.
func (s *Service) Notify(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, pending)
	if err != nil {
		return 0, err
	}
	prizes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (prize, error) {
		var p prize
		err := row.Scan(&p.resultID, &p.drawID, &p.drawType, &p.periodStart, &p.periodEnd, &p.rank, &p.amount, &p.wonAt,
			&p.userID, &p.status, &p.email, &p.confirmed)
		return p, err
	})
	if err != nil {
		return 0, err
	}
	sent := 0
	var errs []error
	for _, p := range prizes {
		emailed, err := s.notify(ctx, p)
		if err != nil {
			errs = append(errs, err)
		}
		if emailed {
			sent++
		}
	}
	return sent, errors.Join(errs...)
}

func (s *Service) notify(ctx context.Context, p prize) (emailed bool, err error) {
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// one decision per prize at a time (a second worker run)
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('winnotice.prize'), hashtext($1))", p.resultID); err != nil {
			return err
		}
		var done bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM draws.prize_notices WHERE result_id = $1)`, p.resultID).Scan(&done); err != nil || done {
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		record := func(outcome string, reason *string) error {
			if _, err := tx.Exec(ctx, `INSERT INTO draws.prize_notices (result_id, outcome, reason, created_at) VALUES ($1, $2, $3, $4)`,
				p.resultID, outcome, reason, now); err != nil {
				return err
			}
			details := map[string]any{"outcome": outcome}
			if reason != nil {
				details["reason"] = *reason
			}
			return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "draws.prize_notice", EntityType: "draw_result",
				EntityID: p.resultID, Details: details})
		}
		skip := func(reason string) error { return record("skipped", &reason) }
		switch {
		case p.status == "closed":
			return skip("the account is closed")
		case p.email == nil:
			return skip("no email address")
		case !p.confirmed:
			return skip("the email address is not confirmed")
		}
		// sent before the record commits: if the mail server refuses it nothing is recorded and the next run tries again
		if err := s.sender.Send(ctx, s.notice(p).Message([]string{*p.email}, "4draws: you won "+kes(p.amount)+" in the "+p.drawType+" draw")); err != nil {
			if now.Sub(p.wonAt) > GiveUpAfter {
				slog.WarnContext(ctx, "a prize notice could not be sent and is given up", "result", p.resultID)
				return skip("the mail server kept refusing it")
			}
			slog.WarnContext(ctx, "a prize notice could not be sent; it is tried again", "result", p.resultID, "error", err)
			return nil
		}
		emailed = true
		return record("emailed", nil)
	})
	return emailed && err == nil, err
}

// notice is the email: what was won, in which draw, how it is paid, and where the result and its proof are. It
// states facts and asks for nothing: no invitation to play again.
func (s *Service) notice(p prize) mail.Notice {
	n := mail.Notice{
		Preheader: "Your entry won " + kes(p.amount) + " in the " + p.drawType + " draw.",
		Heading:   "You won " + kes(p.amount),
		Intro: []string{
			"One of your entries won a prize in the " + p.drawType + " draw.",
		},
		Details: []mail.Detail{
			{Label: "Draw", Value: strings.ToUpper(p.drawType[:1]) + p.drawType[1:]},
			{Label: "Period", Value: period(p.periodStart, p.periodEnd)},
			{Label: "Prize", Value: "No. " + strconv.Itoa(p.rank)},
			{Label: "Amount", Value: kes(p.amount)},
		},
		Outro: []string{
			"The prize is sent to the M-Pesa number on your account, after winnings tax is withheld. Large prizes are checked by our staff first, which can take a working day.",
			"If your identity is not verified yet, verify it in the app under Account: a prize is paid only to a verified player.",
		},
		Reason:  "You are receiving this email because an entry of your 4draws account won a prize.",
		SiteURL: s.site,
	}
	if s.site != "" {
		n.Action = &mail.Action{Label: "See the draw and its proof", URL: s.site + "/draws/" + p.drawID}
	}
	return n
}

// kes is an amount in whole shillings, as "KES 12,500".
func kes(amount int64) string {
	digits := strconv.FormatInt(amount, 10)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return "KES " + b.String()
}

// period is a draw's period as "21 Oct 2026" or "19 Oct 2026 to 25 Oct 2026".
func period(from, to time.Time) string {
	const day = "2 Jan 2006"
	if from.Equal(to) {
		return from.Format(day)
	}
	return from.Format(day) + " to " + to.Format(day)
}
