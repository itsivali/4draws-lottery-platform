package payouts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/itsivali/lottery-platform/internal/aml"
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
		switch {
		case err == nil && !prop.Paid:
			p.Proposal = &Proposal{Paid: prop.Paid, Reason: deref(prop.Reason), By: prop.Staff, At: prop.ProposedAt}
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		}
	}
	if p.State == "manual" || p.State == "succeeded" {
		c, err := q.LatestReceiptCheck(ctx, p.ID)
		if err == nil {
			p.Check = checkView(c)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
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

// Settle decides a payout whose outcome could not be confirmed (manual), from the M-Pesa statement (ADR 0032, 0034).
//   - Paid: the receipt is proved by M-Pesa only. It is recorded as a claim and M-Pesa is asked about it (Transaction
//     Status); the payout is settled as paid when M-Pesa confirms that receipt as a completed payment of the payout's
//     amount to its number, and stays manual otherwise. The payout comes back with the check, waiting for M-Pesa.
//   - Not paid (the money comes back): two people. The first call proposes; a second staff member confirming it
//     applies it; the proposer cannot confirm their own. Refused while a receipt check waits for M-Pesa.
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
	if paid {
		return s.claimReceipt(ctx, id, operator, strings.ToUpper(receipt))
	}
	return s.staffTx(ctx, id, operator, []string{"manual"}, func(tx pgx.Tx, p store.PayoutsPayout) error {
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		q := store.New(tx)
		// M-Pesa may be about to confirm a receipt: wait for its answer before deciding the money never left
		if c, err := q.LatestReceiptCheck(ctx, p.ID); err == nil && c.Outcome == nil && now.Sub(c.RequestedAt) < ReceiptCheckWait {
			return ErrCheckPending
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		prev, err := q.LatestSettlementProposal(ctx, p.ID)
		switch {
		case err == nil && !prev.Paid:
			if prev.Staff == operator {
				return ErrSameApprover
			}
			// confirmed by a second person
			return s.fail(ctx, tx, p, "settled: "+deref(prev.Reason), staff(operator), now)
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if err := q.InsertSettlementProposal(ctx, store.InsertSettlementProposalParams{PayoutID: p.ID, Paid: false, Reason: &reason, Staff: operator, At: now}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payout.settlement_proposed", EntityType: "payout",
			EntityID: p.ID, Details: map[string]any{"paid": false, "reason": reason}})
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
	// a monitoring case holds the player's money (ADR 0033): refunds included
	if held, err := s.onHold(ctx, tx, p.UserID); err != nil {
		return err
	} else if held {
		return fmt.Errorf("%w: %w", ErrPayeeIneligible, aml.ErrUnderReview)
	}
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
	case errors.Is(err, aml.ErrUnderReview):
		return "payee_ineligible: under review"
	case errors.Is(err, identity.ErrAccountSuspended):
		return "payee_ineligible: account not active"
	case errors.Is(err, identity.ErrKYCRequired):
		return "payee_ineligible: identity not verified"
	case errors.Is(err, identity.ErrPhoneNotVerified):
		return "payee_ineligible: phone not proved"
	}
	return "payee_ineligible: number changed"
}
