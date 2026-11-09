package payouts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/compliance"
	"github.com/itsivali/lottery-platform/internal/identity"
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
	if err != nil {
		return Payout{}, err
	}
	v := view(p)
	return v, s.controls(ctx, &v)
}

// controls fills in who approved a held payout and the open settlement proposal of a manual one (ADR 0032).
func (s *Service) controls(ctx context.Context, p *Payout) error {
	q := store.New(s.pool)
	switch p.State {
	case "held":
		a, err := q.Approvals(ctx, p.ID)
		if err != nil {
			return err
		}
		p.Approvals = a
	case "manual":
		prop, err := q.LatestSettlementProposal(ctx, p.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		p.Proposal = &Proposal{Paid: prop.Paid, Receipt: deref(prop.Receipt), Reason: deref(prop.Reason), By: prop.Staff, At: prop.ProposedAt}
	}
	return nil
}

// Approvals is how many different staff members release a held payout (ADR 0032).
const Approvals = 2

// Approve records the operator's approval of a held payout. Once Approvals different staff members approved it (the
// admin who asked for a refund counting as the first) it is released to be sent, if the account may still be paid at
// its number (ErrPayeeIneligible). Approving twice is ErrSameApprover; the payout comes back with who approved so far.
func (s *Service) Approve(ctx context.Context, id, operator string) (Payout, error) {
	if s.daraja == nil {
		return Payout{}, ErrUnavailable
	}
	if !strings.HasPrefix(operator, "staff:") {
		return Payout{}, ErrOperator
	}
	return s.staffTx(ctx, id, operator, []string{"held"}, func(tx pgx.Tx, p store.PayoutsPayout) error {
		if err := s.gate.Assert(ctx, tx, compliance.OpPayout); err != nil {
			return err
		}
		if p.RefundRequestedBy != nil && *p.RefundRequestedBy == operator {
			return ErrSameApprover
		}
		// the account may have changed since the payout was held (ADR 0030): it stays held, for Reject
		if err := s.assertPayee(ctx, tx, p); err != nil {
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		q := store.New(tx)
		if n, err := q.InsertApproval(ctx, store.InsertApprovalParams{PayoutID: p.ID, Staff: operator, At: now}); err != nil {
			return err
		} else if n == 0 {
			return ErrSameApprover
		}
		approvers, err := q.Approvals(ctx, p.ID)
		if err != nil {
			return err
		}
		if p.RefundRequestedBy != nil {
			approvers = append(approvers, *p.RefundRequestedBy)
		}
		released := len(approvers) >= Approvals
		if err := s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payout.approved", EntityType: "payout",
			EntityID: p.ID, Details: map[string]any{"gross_kes": p.GrossKes, "approvals": len(approvers), "released": released}}); err != nil {
			return err
		}
		if !released {
			return nil
		}
		if err := q.ApprovePayout(ctx, store.ApprovePayoutParams{ApprovedBy: &operator, At: now, ID: p.ID}); err != nil {
			return err
		}
		return s.enq.EnqueueSend(ctx, tx, p.ID)
	})
}

// Reject refuses a held payout and gives the money back. Nothing was sent. One person may: it moves no money out.
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
// not paid (the money comes back). Only a payout in manual can be settled this way, and only by two people (ADR
// 0032): the first call proposes; a second staff member making exactly the same decision (paid with the same receipt,
// or not paid) applies it. A different decision replaces the proposal, and the person who proposed cannot confirm their
// own (ErrSameApprover). The payout comes back with the open proposal, if any.
func (s *Service) Settle(ctx context.Context, id, operator string, paid bool, receipt, reason string) (Payout, error) {
	receipt, reason = strings.TrimSpace(receipt), strings.TrimSpace(reason)
	if paid && receipt == "" {
		return Payout{}, ErrReceiptRequired
	}
	if !paid && reason == "" {
		return Payout{}, ErrOperator
	}
	if !strings.HasPrefix(operator, "staff:") {
		return Payout{}, ErrOperator
	}
	return s.staffTx(ctx, id, operator, []string{"manual"}, func(tx pgx.Tx, p store.PayoutsPayout) error {
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		q := store.New(tx)
		prev, err := q.LatestSettlementProposal(ctx, p.ID)
		switch {
		case err == nil && prev.Paid == paid && (!paid || deref(prev.Receipt) == receipt):
			if prev.Staff == operator {
				return ErrSameApprover
			}
			// confirmed by a second person
			if paid {
				return s.succeed(ctx, tx, p, receipt, staff(operator), now)
			}
			return s.fail(ctx, tx, p, "settled: "+deref(prev.Reason), staff(operator), now)
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		params := store.InsertSettlementProposalParams{PayoutID: p.ID, Paid: paid, Staff: operator, At: now}
		if paid {
			params.Receipt = &receipt
		} else {
			params.Reason = &reason
		}
		if err := q.InsertSettlementProposal(ctx, params); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payout.settlement_proposed", EntityType: "payout",
			EntityID: p.ID, Details: map[string]any{"paid": paid, "receipt": receipt, "reason": reason, "replaces": err == nil}})
	})
}

// InState returns up to limit payouts in a state, oldest first (the operator's queue). after is the id of the last
// payout of the previous page, or ""; next continues the list (pass it as after) and is empty on the last page.
func (s *Service) InState(ctx context.Context, state string, limit int, after string) (list []Payout, next string, err error) {
	var cursor *string
	if after != "" {
		cursor = &after
	}
	// one extra row tells whether another page exists
	rows, err := store.New(s.pool).PayoutsInStatus(ctx, store.PayoutsInStatusParams{Status: state, After: cursor, MaxRows: int32(limit) + 1}) //nolint:gosec // small limits
	if pe := (*pgconn.PgError)(nil); errors.As(err, &pe) && pe.Code == "22P02" {                                                              // not a uuid
		return nil, "", ErrBadCursor
	}
	if err != nil {
		return nil, "", err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		next = rows[len(rows)-1].ID
	}
	list = make([]Payout, 0, len(rows))
	for _, p := range rows {
		v := view(p)
		if err := s.controls(ctx, &v); err != nil {
			return nil, "", err
		}
		list = append(list, v)
	}
	return list, next, nil
}

// payeeProblems are the refusals of identity.AssertCanBePaid that mean the account, not the system, is the problem.
var payeeProblems = []error{identity.ErrAccountSuspended, identity.ErrKYCRequired, identity.ErrPhoneNotVerified}

func payeeProblem(err error) bool {
	for _, e := range payeeProblems {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// assertPayee checks again, as money is about to leave, that the account may be paid and that its proved number is
// the one the payout was made out to (ADR 0030). A refusal wraps ErrPayeeIneligible; other errors are returned as they
// are.
func (s *Service) assertPayee(ctx context.Context, tx pgx.Tx, p store.PayoutsPayout) error {
	if p.RefundReason != nil {
		// a refund to source goes to the number that paid in, to an account that by definition cannot be paid
		// (ADR 0031): the second person approving it is the check
		return nil
	}
	msisdn, err := s.ids.AssertCanBePaid(ctx, tx, p.UserID)
	if payeeProblem(err) {
		return fmt.Errorf("%w: %w", ErrPayeeIneligible, err)
	}
	if err != nil {
		return err
	}
	if msisdn != deref(p.Msisdn) {
		return fmt.Errorf("%w: the account's proved number is no longer the payout's", ErrPayeeIneligible)
	}
	return nil
}

// payeeReason is how a payee refusal is recorded on the payout.
func payeeReason(err error) string {
	switch {
	case errors.Is(err, identity.ErrAccountSuspended):
		return "payee_ineligible: account not active"
	case errors.Is(err, identity.ErrKYCRequired):
		return "payee_ineligible: identity not verified"
	case errors.Is(err, identity.ErrPhoneNotVerified):
		return "payee_ineligible: phone not proved"
	}
	return "payee_ineligible: number changed"
}
