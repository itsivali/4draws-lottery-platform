package payouts

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/payments/daraja"
	"github.com/itsivali/4draws-lottery-platform/internal/payouts/store"
)

// Errors of receipt checks (ADR 0034).
var (
	ErrInvalidReceipt = errors.New("payouts: an M-Pesa receipt is 10 letters and digits")
	ErrReceiptUsed    = errors.New("payouts: this receipt already settled another payout")
	ErrCheckPending   = errors.New("payouts: M-Pesa has not answered about the last receipt yet")
	ErrNoAnswer       = errors.New("payouts: M-Pesa did not take the question; ask again in a few minutes")
)

// ReceiptCheckWait is how long a receipt check waits for M-Pesa's answer before another receipt may be checked.
const ReceiptCheckWait = 10 * time.Minute

var mpesaReceipt = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// ReceiptCheck is a receipt staff found on the statement for a manual payout, and what M-Pesa said about it.
type ReceiptCheck struct {
	Receipt string    `json:"receipt"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
	// Outcome is "" while M-Pesa has not answered, then confirmed, refuted or no_answer.
	Outcome string `json:"outcome"`
	Detail  string `json:"detail,omitempty"`
}

func checkView(c store.PayoutsReceiptCheck) *ReceiptCheck {
	return &ReceiptCheck{Receipt: c.Receipt, By: c.Staff, At: c.RequestedAt, Outcome: deref(c.Outcome), Detail: deref(c.Detail)}
}

// claimReceipt records the receipt an operator found on the statement for a manual payout and asks M-Pesa about it
// (Transaction Status, by receipt, from our B2C shortcode). Nothing is settled here: the payout is paid only when
// M-Pesa's answer confirms the receipt (checkReceipt).
func (s *Service) claimReceipt(ctx context.Context, id, operator, receipt string) (Payout, error) {
	if !mpesaReceipt.MatchString(receipt) {
		return Payout{}, ErrInvalidReceipt
	}
	if s.daraja == nil {
		return Payout{}, ErrUnavailable
	}
	var checkID int64
	if _, err := s.staffTx(ctx, id, operator, []string{"manual"}, func(tx pgx.Tx, p store.PayoutsPayout) error {
		q := store.New(tx)
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		prev, err := q.LatestReceiptCheck(ctx, p.ID)
		switch {
		case err == nil && prev.Outcome == nil && now.Sub(prev.RequestedAt) < ReceiptCheckWait:
			return ErrCheckPending
		case err == nil && prev.Outcome == nil:
			if err := q.DecideCheck(ctx, store.DecideCheckParams{Outcome: strPtr("no_answer"), Detail: strPtr("M-Pesa did not answer in time"), At: &now, ID: prev.ID}); err != nil {
				return err
			}
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if used, err := q.ReceiptPaid(ctx, &receipt); err != nil {
			return err
		} else if used {
			return ErrReceiptUsed
		}
		if checkID, err = q.InsertReceiptCheck(ctx, store.InsertReceiptCheckParams{PayoutID: p.ID, Receipt: receipt, Staff: operator, At: now}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payout.receipt_claimed", EntityType: "payout",
			EntityID: p.ID, Details: map[string]any{"receipt": receipt}})
	}); err != nil {
		return Payout{}, err
	}
	req, askErr := s.daraja.B2CStatus(ctx, receipt, "", id)
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if askErr == nil {
			return q.SetCheckOriginator(ctx, store.SetCheckOriginatorParams{Originator: &req.OriginatorConversationID, ID: checkID})
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		slog.WarnContext(ctx, "transaction status request about a claimed receipt failed", "payout", id, "err", askErr)
		return q.DecideCheck(ctx, store.DecideCheckParams{Outcome: strPtr("no_answer"), Detail: strPtr("M-Pesa did not take the question"), At: &now, ID: checkID})
	})
	if err != nil {
		return Payout{}, err
	}
	if askErr != nil {
		return Payout{}, ErrNoAnswer
	}
	return s.staffView(ctx, id)
}

// staffView is a payout with what staff see of it.
func (s *Service) staffView(ctx context.Context, id string) (Payout, error) {
	p, err := store.New(s.pool).GetPayout(ctx, id)
	if err != nil {
		return Payout{}, err
	}
	v := view(p)
	return v, s.controls(ctx, &v)
}

// checkReceipt decides a manual payout's pending receipt check from M-Pesa's answer to it: confirmed (completed, that
// very receipt, the payout's net amount, to the payout's number) settles the payout as paid under the receipt; anything
// else leaves it manual, with why. Called by the verify job when Daraja posts the answer.
func (s *Service) checkReceipt(ctx context.Context, p store.PayoutsPayout) error {
	c, err := store.New(s.pool).LatestReceiptCheck(ctx, p.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if c.Outcome != nil || c.StatusOriginator == nil {
		return nil
	}
	cbs, err := store.New(s.pool).Callbacks(ctx, &p.ID)
	if err != nil {
		return err
	}
	var answer *daraja.StatusResult
	timedOut := false
	for _, cb := range cbs {
		r, err := daraja.ParseStatusResult(cb.Raw)
		if err != nil || r.OriginatorConversationID != *c.StatusOriginator {
			continue
		}
		switch cb.Kind {
		case CallbackStatusResult:
			answer = &r
		case CallbackStatusTimeout:
			timedOut = true
		}
	}
	if answer == nil && !timedOut {
		return nil // not answered yet
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.LockPayout(ctx, p.ID)
		if err != nil || cur.Status != "manual" {
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		outcome, detail := "refuted", ""
		switch {
		case answer == nil:
			outcome, detail = "no_answer", "M-Pesa's answer timed out"
		case !answer.Completed():
			detail = "M-Pesa does not confirm a completed payment with this receipt: " + answer.ResultDesc
		case answer.Receipt != c.Receipt:
			detail = "M-Pesa reported receipt " + answer.Receipt
		case answer.AmountKES != cur.NetKes:
			detail = "M-Pesa reported KES " + strconv.FormatInt(answer.AmountKES, 10) + ", the payout is KES " + strconv.FormatInt(cur.NetKes, 10)
		case answer.CreditedMSISDN() != "" && answer.CreditedMSISDN() != deref(cur.Msisdn):
			detail = "M-Pesa paid another number"
		default:
			outcome = "confirmed"
		}
		if err := q.DecideCheck(ctx, store.DecideCheckParams{Outcome: &outcome, Detail: optional(detail), At: &now, ID: c.ID}); err != nil {
			return err
		}
		if outcome == "confirmed" {
			// the claimant settled it; M-Pesa proved it
			return s.succeed(ctx, tx, cur, c.Receipt, staff(c.Staff), now)
		}
		slog.WarnContext(ctx, "a claimed payout receipt was not confirmed by M-Pesa", "payout", p.ID, "outcome", outcome, "detail", detail)
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "payout.receipt_" + outcome, EntityType: "payout", EntityID: p.ID,
			Details: map[string]any{"receipt": c.Receipt, "claimed_by": c.Staff, "detail": detail, "check": strconv.FormatInt(c.ID, 10)}})
	})
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
