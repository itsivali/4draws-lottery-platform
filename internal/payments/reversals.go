package payments

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/payments/store"
)

// Errors of the operator actions.
var (
	ErrNotReversible  = errors.New("payments: only a credited payment can be reversed")
	ErrNotResolvable  = errors.New("payments: only a settled, uncredited top-up can be resolved")
	ErrReceiptUsed    = errors.New("payments: this receipt already credited another payment")
	ErrReasonRequired = errors.New("payments: a reason is required")
)

// StatusReversed is shown to the player for a top-up M-Pesa took back.
const StatusReversed = "reversed"

// Reversal is the outcome of reversing a credited top-up.
type Reversal struct {
	PaymentID string
	UserID    string
	TakenKES  int64 // taken back from the wallet
	DebtKES   int64 // what the player now owes in total (recovered from later deposits)
}

// Reverse records that M-Pesa reversed a credited top-up (found on the statement or reported by Safaricom): the
// player gives back what they received, from the wallet as far as it goes and as debt beyond that, which blocks play
// and withdrawals and is recovered from later deposits. operator is who did it (audited as staff).
func (s *Service) Reverse(ctx context.Context, receipt, reason, operator string) (Reversal, error) {
	if strings.TrimSpace(reason) == "" {
		return Reversal{}, ErrReasonRequired
	}
	var out Reversal
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := q.LockPaymentByReceipt(ctx, &receipt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if p.Status != "credited" || p.AmountConfirmedKes == nil {
			return ErrNotReversible
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		excise, err := q.DepositExciseDeducted(ctx, p.ID)
		if err != nil {
			return err
		}
		posting, _, err := s.wallet.ReverseDeposit(ctx, tx, p.ID, p.UserID, *p.AmountConfirmedKes, excise)
		if err != nil {
			return err
		}
		if err := q.MarkReversed(ctx, store.MarkReversedParams{At: &now, Reason: &reason, ID: p.ID}); err != nil {
			return err
		}
		debt, err := s.wallet.Debt(ctx, tx, p.UserID)
		if err != nil {
			return err
		}
		out = Reversal{PaymentID: p.ID, UserID: p.UserID, TakenKES: -posting.WalletDelta(), DebtKES: debt}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payment.topup.reversed",
			EntityType: "payment", EntityID: p.ID, Details: map[string]any{"receipt": receipt, "reason": reason,
				"amount_kes": *p.AmountConfirmedKes, "taken_kes": out.TakenKES, "debt_kes": debt}})
	})
	return out, err
}

// ResolveUnconfirmed credits a settled, uncredited top-up from the receipt an operator found on the statement
// (Resolve, with no note).
func (s *Service) ResolveUnconfirmed(ctx context.Context, paymentID, receipt string, amount int64, operator string) error {
	return s.Resolve(ctx, Resolution{PaymentID: paymentID, Receipt: receipt, AmountKES: amount, Source: "statement"}, operator)
}
