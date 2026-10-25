package payouts

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/payouts/store"
)

// Operator actions (lotteryctl until the back office). Each is one transaction, audited as staff.

func staff(operator string) audit.Event {
	return audit.Event{ActorType: audit.Staff, ActorID: operator}
}

// staffTx locks a payout in one of the given states and runs f on it.
func (s *Service) staffTx(ctx context.Context, id, operator string, states []string, f func(tx pgx.Tx, p store.PayoutsPayout) error) (Payout, error) {
	if strings.TrimSpace(operator) == "" {
		return Payout{}, ErrOperator
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := store.New(tx).LockPayout(ctx, id)
		if notFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		for _, st := range states {
			if p.Status == st {
				return f(tx, p)
			}
		}
		return ErrState
	})
	if err != nil {
		return Payout{}, err
	}
	p, err := store.New(s.pool).GetPayout(ctx, id)
	return view(p), err
}

// Approve releases a held payout to be sent.
func (s *Service) Approve(ctx context.Context, id, operator string) (Payout, error) {
	if s.daraja == nil {
		return Payout{}, ErrUnavailable
	}
	return s.staffTx(ctx, id, operator, []string{"held"}, func(tx pgx.Tx, p store.PayoutsPayout) error {
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		if err := store.New(tx).ApprovePayout(ctx, store.ApprovePayoutParams{ApprovedBy: &operator, At: now, ID: p.ID}); err != nil {
			return err
		}
		if err := s.enq.EnqueueSend(ctx, tx, p.ID); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payout.approved", EntityType: "payout",
			EntityID: p.ID, Details: map[string]any{"gross_kes": p.GrossKes}})
	})
}

// Reject refuses a held payout and gives the money back. Nothing was sent.
func (s *Service) Reject(ctx context.Context, id, operator, reason string) (Payout, error) {
	if strings.TrimSpace(reason) == "" {
		return Payout{}, ErrOperator
	}
	return s.staffTx(ctx, id, operator, []string{"held"}, func(tx pgx.Tx, p store.PayoutsPayout) error {
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		return s.fail(ctx, tx, p, "rejected: "+reason, staff(operator), now)
	})
}

// Settle decides a payout whose outcome could not be confirmed, from the M-Pesa statement: paid (with its receipt) or
// not paid (the money comes back). Only a payout in manual can be settled this way.
func (s *Service) Settle(ctx context.Context, id, operator string, paid bool, receipt, reason string) (Payout, error) {
	receipt = strings.TrimSpace(receipt)
	if paid && receipt == "" {
		return Payout{}, ErrReceiptRequired
	}
	if !paid && strings.TrimSpace(reason) == "" {
		return Payout{}, ErrOperator
	}
	return s.staffTx(ctx, id, operator, []string{"manual"}, func(tx pgx.Tx, p store.PayoutsPayout) error {
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		if paid {
			return s.succeed(ctx, tx, p, receipt, staff(operator), now)
		}
		return s.fail(ctx, tx, p, "settled: "+reason, staff(operator), now)
	})
}

// InState lists payouts in an internal state, oldest first (for operators).
func (s *Service) InState(ctx context.Context, state string, limit int) ([]Payout, error) {
	rows, err := store.New(s.pool).PayoutsInStatus(ctx, store.PayoutsInStatusParams{Status: state, MaxRows: int32(limit)}) //nolint:gosec // small limits
	if err != nil {
		return nil, err
	}
	out := make([]Payout, 0, len(rows))
	for _, p := range rows {
		out = append(out, view(p))
	}
	return out, nil
}
