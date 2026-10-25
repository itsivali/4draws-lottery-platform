package payouts

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/payouts/store"
)

// Errors of prize payouts.
var (
	ErrNotRetryable = errors.New("payouts: only a prize whose payout failed can be paid again")
)

type prize struct {
	resultID, userID, drawType, drawID string
	amount                             int64
}

// PayPrizes creates a payout for every prize of an approved draw that has never had one: the winnings tax is withheld,
// the prize leaves its pool for B2C, and the payment is queued (or held above the manual threshold). A winner who
// cannot be paid now (suspended, or no longer verified) is skipped and reported; the prize stays owed. A prize whose
// payout failed is not paid again here: an operator retries it (RetryPrize), so a number that cannot receive M-Pesa
// does not loop. Without B2C nothing is created. It returns how many payouts it created.
func (s *Service) PayPrizes(ctx context.Context) (int, error) {
	if s.daraja == nil {
		return 0, nil
	}
	rows, err := store.New(s.pool).PrizesToPay(ctx)
	if err != nil {
		return 0, err
	}
	created := 0
	var errs []error
	for _, r := range rows {
		p := prize{resultID: r.ResultID, userID: r.UserID, drawType: r.DrawType, drawID: r.DrawID, amount: r.AmountKes}
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			// one decision per prize at a time (a second worker run, or an operator's retry)
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('payouts.prize'), hashtext($1))", p.resultID); err != nil {
				return err
			}
			n, err := store.New(tx).ResultPayouts(ctx, &p.resultID)
			if err != nil || n.Attempts > 0 {
				return err
			}
			return s.createPrize(ctx, tx, p, "prize:"+p.resultID, audit.Event{ActorType: audit.System})
		})
		switch {
		case errors.Is(err, errCannotPay):
		case err != nil:
			errs = append(errs, err)
		default:
			created++
		}
	}
	return created, errors.Join(errs...)
}

var errCannotPay = errors.New("payouts: the winner cannot be paid now")

// createPrize records one prize payout and its posting.
func (s *Service) createPrize(ctx context.Context, tx pgx.Tx, p prize, key string, actor audit.Event) error {
	q := store.New(tx)
	msisdn, err := s.ids.AssertCanBePaid(ctx, tx, p.userID)
	if err != nil {
		slog.WarnContext(ctx, "prize winner cannot be paid now; the prize stays owed", "result", p.resultID, "err", err)
		return errCannotPay
	}
	now, err := s.clk.Now(ctx, tx)
	if err != nil {
		return err
	}
	rule, err := s.ledger.ActiveRule(ctx, tx, WinningsTax, now)
	if errors.Is(err, ledger.ErrNoTaxRule) {
		return ErrUnavailable
	} else if err != nil {
		return err
	}
	tax, err := ledger.Apply(p.amount, rule.Rate, rule.Rounding)
	if err != nil {
		return err
	}
	status := "pending"
	if p.amount > s.cfg.ManualAboveKES {
		status = "held"
	}
	id, err := q.InsertPayout(ctx, store.InsertPayoutParams{UserID: p.userID, Kind: "prize", Status: status, GrossKes: p.amount, TaxKes: tax,
		NetKes: p.amount - tax, Msisdn: &msisdn, DrawResultID: &p.resultID, IdempotencyKey: key, TaxCode: strPtr(WinningsTax),
		TaxRate: rateText(rule.Rate), TaxRounding: strPtr(string(rule.Rounding)), At: now})
	if err != nil {
		return err
	}
	posting, err := ledger.PrizePayout(id, p.drawType, p.amount, rule)
	if err != nil {
		return err
	}
	if _, err := s.ledger.Post(ctx, tx, posting.Journal); err != nil {
		return err
	}
	if status == "pending" {
		if err := s.enq.EnqueueSend(ctx, tx, id); err != nil {
			return err
		}
	}
	actor.Action, actor.EntityType, actor.EntityID = "payout.prize.created", "payout", id
	actor.Details = map[string]any{"draw": p.drawID, "result": p.resultID, "gross_kes": p.amount, "tax_kes": tax, "held": status == "held"}
	return s.audit.Record(ctx, tx, actor)
}

// RetryPrize pays a prize again after its payout failed (the prize went back to its pool).
func (s *Service) RetryPrize(ctx context.Context, resultID, operator string) (Payout, error) {
	if strings.TrimSpace(operator) == "" {
		return Payout{}, ErrOperator
	}
	if s.daraja == nil {
		return Payout{}, ErrUnavailable
	}
	var id string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('payouts.prize'), hashtext($1))", resultID); err != nil {
			return err
		}
		r, err := q.PrizeResult(ctx, resultID)
		if notFound(err) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		n, err := q.ResultPayouts(ctx, &resultID)
		if err != nil {
			return err
		}
		if n.Attempts == 0 || n.Live > 0 || (r.DrawStatus != "approved" && r.DrawStatus != "paid") {
			return ErrNotRetryable
		}
		key := "prize:" + resultID + ":" + strconv.Itoa(int(n.Attempts)+1)
		if err := s.createPrize(ctx, tx, prize{resultID: r.ResultID, userID: r.UserID, drawType: r.DrawType, drawID: r.DrawID, amount: r.AmountKes},
			key, audit.Event{ActorType: audit.Staff, ActorID: operator}); err != nil {
			return err
		}
		id, err = q.PayoutByKey(ctx, store.PayoutByKeyParams{UserID: r.UserID, IdempotencyKey: key})
		return err
	})
	if errors.Is(err, errCannotPay) {
		return Payout{}, errors.New("payouts: the winner cannot be paid now (suspended or not verified)")
	}
	if err != nil {
		return Payout{}, err
	}
	p, err := store.New(s.pool).GetPayout(ctx, id)
	return view(p), err
}

// recordWithholding appends the tax withheld on a paid payout to the KRA feed (once per payout).
func recordWithholding(ctx context.Context, tx pgx.Tx, p store.PayoutsPayout, receipt string, now time.Time) error {
	if p.TaxKes == 0 {
		return nil
	}
	rate, err := store.New(tx).TaxRateText(ctx, p.ID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"payout_id": p.ID, "kind": p.Kind, "payee": p.UserID, "gross_kes": p.GrossKes, "tax_kes": p.TaxKes,
		"net_kes": p.NetKes, "tax_code": deref(p.TaxCode), "tax_rate": rate, "receipt": receipt, "paid_at": now.UTC().Format(time.RFC3339)})
	if err != nil {
		return err
	}
	return store.New(tx).InsertKRAEvent(ctx, store.InsertKRAEventParams{EventType: "withholding_tax", Payload: payload, At: now})
}

// TaxEvent is one row of the KRA feed.
type TaxEvent struct {
	ID        int64           `json:"id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// ExportTax hands the events not yet exported to write, in order, and marks them exported only if write succeeds, in
// the same transaction: an export that fails half-way is repeated whole.
func (s *Service) ExportTax(ctx context.Context, limit int, write func([]TaxEvent) error) (int, error) {
	n := 0
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.PendingKRAEvents(ctx, int32(limit)) //nolint:gosec // small limits
		if err != nil || len(rows) == 0 {
			return err
		}
		events := make([]TaxEvent, 0, len(rows))
		ids := make([]int64, 0, len(rows))
		for _, r := range rows {
			events = append(events, TaxEvent{ID: r.ID, EventType: r.EventType, Payload: r.Payload, CreatedAt: r.CreatedAt})
			ids = append(ids, r.ID)
		}
		if err := write(events); err != nil {
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		n = len(events)
		return q.MarkKRAExported(ctx, store.MarkKRAExportedParams{At: &now, Ids: ids})
	})
	return n, err
}
