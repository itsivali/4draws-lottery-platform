package payouts

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/aml"
	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/compliance"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/payouts/store"
)

// Refund is what makes a payout a refund to source (ADR 0031).
type Refund struct {
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	// MSISDN is where it goes: the number that paid in the account's latest credited top-up.
	MSISDN string `json:"msisdn"`
}

// Errors of refunds to source.
var (
	// ErrPayable: the account may be paid, so its balance leaves as a withdrawal the player makes, not a refund.
	ErrPayable = errors.New("payouts: the account can withdraw; a refund to source is only for balances it cannot")
	// ErrNothingToRefund: the wallet is empty.
	ErrNothingToRefund = errors.New("payouts: the wallet holds nothing to refund")
	// ErrNoSource: no credited top-up names a number to send the money back to.
	ErrNoSource = errors.New("payouts: no credited top-up to refund to")
	// ErrSameApprover: a refund is approved by someone other than the admin who asked for it.
	ErrSameApprover = errors.New("payouts: a refund needs a second person to approve it")
)

// RefundToSource sends the whole balance of an account that cannot withdraw it (suspended, identity not verified or
// refused, phone not proved) back to the number that paid in its latest credited top-up (ADR 0031). It is recorded as a
// held withdrawal: the wallet is debited at once, the withdrawal tax rule in force applies as to any withdrawal, and
// nothing is sent until a second staff member approves it (Approve); Reject gives the money back to the wallet. The
// reason and the requester stay on the payout. key makes the request idempotent.
func (s *Service) RefundToSource(ctx context.Context, userID, operator, reason, key string) (Payout, error) {
	reason = strings.TrimSpace(reason)
	if strings.TrimSpace(operator) == "" || reason == "" || len(reason) > 500 {
		return Payout{}, ErrOperator
	}
	if s.daraja == nil {
		return Payout{}, ErrUnavailable
	}
	var id string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		// the same lock withdrawals take: one decision about this wallet's money at a time
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('payouts.withdraw'), hashtext($1))", userID); err != nil {
			return err
		}
		key = "refund:" + key
		if existing, err := q.PayoutByKey(ctx, store.PayoutByKeyParams{UserID: userID, IdempotencyKey: key}); err == nil {
			id = existing
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := s.gate.Assert(ctx, tx, compliance.OpWithdrawal); err != nil {
			return err
		}
		// only for an account that cannot be paid: one that can withdraws its own money
		_, notPayable := s.ids.AssertCanBePaid(ctx, tx, userID)
		switch {
		case notPayable == nil:
			return ErrPayable
		case !payeeProblem(notPayable):
			return notPayable
		}
		if held, err := s.onHold(ctx, tx, userID); err != nil {
			return err
		} else if held {
			return aml.ErrUnderReview
		}
		if debt, err := s.wallet.Debt(ctx, tx, userID); err != nil {
			return err
		} else if debt > 0 {
			return ErrInDebt
		}
		amount, err := s.wallet.Balance(ctx, tx, userID)
		if err != nil {
			return err
		}
		if amount <= 0 {
			return ErrNothingToRefund
		}
		msisdn, err := q.RefundSource(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoSource
		} else if err != nil {
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		rule, err := s.ledger.ActiveRule(ctx, tx, WithdrawalTax, now)
		if errors.Is(err, ledger.ErrNoTaxRule) {
			return ErrUnavailable
		} else if err != nil {
			return err
		}
		tax, err := ledger.Apply(amount, rule.Rate, rule.Rounding)
		if err != nil {
			return err
		}
		if id, err = q.InsertPayout(ctx, store.InsertPayoutParams{UserID: userID, Kind: "withdrawal", Route: "b2c", Status: "held", GrossKes: amount,
			TaxKes: tax, NetKes: amount - tax, Msisdn: &msisdn, IdempotencyKey: key, TaxCode: strPtr(WithdrawalTax),
			TaxRate: rateText(rule.Rate), TaxRounding: strPtr(string(rule.Rounding)),
			RefundReason: &reason, RefundRequestedBy: &operator, At: now}); err != nil {
			return err
		}
		posting, err := ledger.Withdrawal(id, userID, amount, rule)
		if err != nil {
			return err
		}
		if _, err := s.wallet.Apply(ctx, tx, posting); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payout.refund.requested",
			EntityType: "payout", EntityID: id, Details: map[string]any{"user_id": userID, "amount_kes": amount, "tax_kes": tax,
				"msisdn": msisdn, "reason": reason, "why_not_payable": notPayable.Error()}})
	})
	if err != nil {
		if errors.Is(err, s.insufficient) {
			return Payout{}, ErrNothingToRefund
		}
		return Payout{}, err
	}
	p, err := store.New(s.pool).GetPayout(ctx, id)
	return view(p), err
}
