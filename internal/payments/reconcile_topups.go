package payments

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/payments/daraja"
	"github.com/itsivali/lottery-platform/internal/payments/store"
)

// Reconciliation of top-ups M-Pesa completed but the platform never credited (owner, 2026-10-31: a KES 1 sandbox
// payment got its M-Pesa message, Safaricom's callback never came, the status query was throttled, and the top-up sat
// "unconfirmed" until an operator typed the receipt in). Money is only ever credited against an M-Pesa receipt, and
// the receipt can come from four places, each handled here:
//
//   - a callback that arrives after the payment settled (HandleCallback): credited at once;
//   - the player's own M-Pesa message (ClaimReceipt): asked of M-Pesa through Transaction Status when the paybill
//     initiator is configured, and credited when M-Pesa confirms the receipt, the amount and the payer; otherwise, or
//     when M-Pesa does not confirm it, left for an operator with the receipt already filled in;
//   - the M-Pesa statement (statement.go): matched to the prompts by receipt, amount, number and time;
//   - an operator (Resolve, Recheck, Dismiss), from the back office.

// Reconciliation errors.
var (
	ErrInvalidReceipt = errors.New("payments: an M-Pesa receipt is 10 letters and digits")
	ErrStillSettling  = errors.New("payments: the top-up is still being settled")
	ErrNotClaimable   = errors.New("payments: a receipt is given for a top-up that settled without being credited")
	ErrClaimOpen      = errors.New("payments: this top-up already has a receipt being checked")
	ErrSettledAlready = errors.New("payments: a credited or reversed top-up is not checked again")
	ErrNotDismissable = errors.New("payments: only a settled, uncredited top-up can be dismissed")
	// ErrDarajaUnavailable wraps a Daraja outage or throttling met while an operator was waiting.
	ErrDarajaUnavailable = errors.New("payments: M-Pesa did not answer; ask again in a few minutes")
)

var receiptRE = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// NormalizeReceipt upper-cases and checks an M-Pesa receipt.
func NormalizeReceipt(s string) (string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if !receiptRE.MatchString(s) {
		return "", ErrInvalidReceipt
	}
	return s, nil
}

// uncredited reports a final status that holds no money: the top-up may still turn out to have been paid.
func uncredited(status string) bool {
	switch status {
	case "unconfirmed", "failed", "cancelled", "expired":
		return true
	}
	return false
}

// Claim statuses.
const (
	ClaimClaimed    = "claimed"    // the player gave a receipt; M-Pesa not asked yet (or cannot be asked)
	ClaimChecking   = "checking"   // Transaction Status asked; the answer is awaited
	ClaimConfirmed  = "confirmed"  // M-Pesa confirmed it and the top-up was credited
	ClaimUnverified = "unverified" // M-Pesa did not confirm it; an operator decides
	ClaimRejected   = "rejected"   // an operator found no payment, or credited the top-up another way
)

// Claim is what the player sees of their receipt.
type Claim struct {
	Receipt string `json:"receipt"`
	Status  string `json:"status"`
	Note    string `json:"note,omitempty"`
}

func notFound(err error) bool {
	var pe *pgconn.PgError
	return errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02")
}

// ClaimReceipt records the receipt the player read from their M-Pesa message for a top-up that settled uncredited,
// and asks M-Pesa about it when Transaction Status is configured. The receipt is only a claim until M-Pesa or an
// operator confirms it: nothing is credited here.
func (s *Service) ClaimReceipt(ctx context.Context, userID, paymentID, raw string) (Claim, error) {
	receipt, err := NormalizeReceipt(raw)
	if err != nil {
		return Claim{}, err
	}
	var out Claim
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := q.LockPayment(ctx, paymentID)
		if notFound(err) || (err == nil && p.UserID != userID) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		switch {
		case p.Status == "credited" || p.Status == "reversed":
			return ErrNotClaimable
		case !final(p.Status):
			return ErrStillSettling
		}
		d, err := q.GetPaymentDetail(ctx, paymentID)
		if err != nil {
			return err
		}
		if d.ClaimStatus != nil && (*d.ClaimStatus == ClaimClaimed || *d.ClaimStatus == ClaimChecking) {
			if d.ClaimedReceipt != nil && *d.ClaimedReceipt == receipt {
				out = Claim{Receipt: receipt, Status: *d.ClaimStatus}
				return nil
			}
			return ErrClaimOpen
		}
		used, err := q.ReceiptUsedElsewhere(ctx, store.ReceiptUsedElsewhereParams{MpesaReceipt: &receipt, ID: paymentID})
		if err != nil {
			return err
		}
		if used {
			return ErrReceiptUsed
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		if err := q.ClaimReceipt(ctx, store.ClaimReceiptParams{Receipt: &receipt, At: &now, ID: paymentID}); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: userID, Action: "payment.topup.receipt_claimed",
			EntityType: "payment", EntityID: paymentID, Details: map[string]any{"receipt": receipt, "status": p.Status}}); err != nil {
			return err
		}
		out = Claim{Receipt: receipt, Status: ClaimClaimed}
		if s.c2b != nil {
			return s.enq.EnqueueClaimCheck(ctx, tx, paymentID)
		}
		return nil
	})
	return out, err
}

// claimOf reads the claim on a payment, if any.
func claimOf(d store.GetPaymentDetailRow) *Claim {
	if d.ClaimStatus == nil || d.ClaimedReceipt == nil {
		return nil
	}
	c := &Claim{Receipt: *d.ClaimedReceipt, Status: *d.ClaimStatus}
	if d.ClaimNote != nil {
		c.Note = *d.ClaimNote
	}
	return c
}

// CheckClaim asks M-Pesa about a claimed receipt (Transaction Status; the answer comes back through the paybill
// status hook and settleClaim). A positive duration asks the job to try again later: M-Pesa was unavailable or
// throttling. Without the paybill initiator the claim waits for an operator.
func (s *Service) CheckClaim(ctx context.Context, paymentID string) (time.Duration, error) {
	d, err := store.New(s.pool).GetPaymentDetail(ctx, paymentID)
	if err != nil {
		return 0, err
	}
	if s.c2b == nil || d.ClaimStatus == nil || d.ClaimedReceipt == nil || (*d.ClaimStatus != ClaimClaimed && *d.ClaimStatus != ClaimChecking) {
		return 0, nil
	}
	req, err := s.c2b.daraja.TransactionStatus(ctx, *d.ClaimedReceipt)
	now, cerr := s.clk.Now(ctx, s.pool)
	if cerr != nil {
		return 0, cerr
	}
	switch {
	case errors.Is(err, daraja.ErrRejected) || errors.Is(err, daraja.ErrInvalidRequest):
		note := "M-Pesa refused to look the receipt up: " + err.Error()
		return 0, s.setClaim(ctx, paymentID, ClaimUnverified, nil, nil, &note, now)
	case err != nil:
		slog.WarnContext(ctx, "receipt claim: transaction status request failed; asking again later", "payment", paymentID, "err", err)
		return 2 * s.cfg.VerifyAfter, nil
	}
	return 0, store.New(s.pool).MarkClaimChecking(ctx, store.MarkClaimCheckingParams{Originator: req.OriginatorConversationID, At: now, ID: paymentID})
}

func (s *Service) setClaim(ctx context.Context, paymentID, status string, originator *string, result []byte, note *string, now time.Time) error {
	return store.New(s.pool).SetClaimStatus(ctx, store.SetClaimStatusParams{Status: &status, Originator: originator, Result: result, Note: note, At: now, ID: paymentID})
}

// settleClaim applies M-Pesa's Transaction Status answer about a claimed receipt, in the caller's transaction (the
// payment is locked). The top-up is credited only when M-Pesa confirms a completed payment of the amount asked for,
// into our business, from the player's number (when M-Pesa names it); anything else leaves the claim for an operator
// with M-Pesa's answer attached.
func (s *Service) settleClaim(ctx context.Context, tx pgx.Tx, paymentID string, raw []byte, r daraja.StatusResult, now time.Time) error {
	q := store.New(tx)
	d, err := q.GetPaymentDetail(ctx, paymentID)
	if err != nil {
		return err
	}
	if d.ClaimStatus == nil || d.ClaimedReceipt == nil || (*d.ClaimStatus != ClaimClaimed && *d.ClaimStatus != ClaimChecking) {
		return nil
	}
	receipt := *d.ClaimedReceipt
	unverified := func(note string) error {
		if err := q.SetClaimStatus(ctx, store.SetClaimStatusParams{Status: ptr(ClaimUnverified), Result: raw, Note: &note, At: now, ID: paymentID}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "payment.topup.receipt_unverified", EntityType: "payment",
			EntityID: paymentID, Details: map[string]any{"receipt": receipt, "note": note, "result_code": r.ResultCode}})
	}
	payer := r.Payer()
	switch {
	case !r.Completed():
		desc := r.ResultDesc
		if r.Status != "" && r.Status != "Completed" {
			desc = "M-Pesa shows the transaction as " + r.Status
		}
		return unverified("M-Pesa did not confirm the receipt: " + desc)
	case !r.CreditedTo(s.cfg.ShortCode) && !r.CreditedTo(s.c2bShortCode()):
		return unverified("the receipt was paid to another business: " + r.CreditParty)
	case r.Receipt != "" && r.Receipt != receipt:
		return unverified("M-Pesa answered about receipt " + r.Receipt + ", not " + receipt)
	case r.AmountKES != d.AmountRequestedKes:
		return unverified("M-Pesa says KES " + strconv.FormatInt(r.AmountKES, 10) + " was paid; the top-up asked for KES " + strconv.FormatInt(d.AmountRequestedKes, 10))
	case payer != "" && d.Msisdn != nil && payer != *d.Msisdn:
		return unverified("the receipt was paid from another number")
	}
	if d.Status == "credited" || d.Status == "reversed" { // credited meanwhile (a late callback, an operator)
		return q.SetClaimStatus(ctx, store.SetClaimStatusParams{Status: ptr(ClaimConfirmed), Result: raw, At: now, ID: paymentID})
	}
	used, err := q.ReceiptUsedElsewhere(ctx, store.ReceiptUsedElsewhereParams{MpesaReceipt: &receipt, ID: paymentID})
	if err != nil {
		return err
	}
	if used {
		return unverified("the receipt already credited another top-up")
	}
	if err := q.SetClaimStatus(ctx, store.SetClaimStatusParams{Status: ptr(ClaimConfirmed), Result: raw, At: now, ID: paymentID}); err != nil {
		return err
	}
	return s.creditPayment(ctx, tx, creditInput{paymentID: paymentID, user: d.UserID, target: deref(d.Msisdn), amount: r.AmountKES, receipt: receipt,
		payer: payer, now: now, actor: audit.Event{ActorType: audit.System}, source: "receipt_claim"})
}

func (s *Service) c2bShortCode() string {
	if s.c2b == nil {
		return ""
	}
	return s.c2b.cfg.ShortCode
}

func ptr[T any](v T) *T { return &v }

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// lateCallback credits a top-up that had settled uncredited when Safaricom's paid callback finally arrives (the
// callback is the receipt), in the caller's transaction with the payment locked. It returns false when the callback
// does not pay the top-up: a failure code, a receipt already used, or a payment already holding money.
func (s *Service) lateCallback(ctx context.Context, tx pgx.Tx, p settledTopUp, cb daraja.STKCallback, raw []byte, now time.Time) (bool, error) {
	if !uncredited(p.Status) || !cb.Paid() || p.Msisdn == nil {
		return false, nil
	}
	q := store.New(tx)
	used, err := q.ReceiptUsedElsewhere(ctx, store.ReceiptUsedElsewhereParams{MpesaReceipt: &cb.Receipt, ID: p.ID})
	if err != nil || used {
		return false, err
	}
	if err := q.RecordCallback(ctx, store.RecordCallbackParams{RawCallback: raw, At: now, ID: p.ID}); err != nil {
		return false, err
	}
	d, err := q.GetPaymentDetail(ctx, p.ID)
	if err != nil {
		return false, err
	}
	if c := claimOf(d); c != nil && (c.Status == ClaimClaimed || c.Status == ClaimChecking || c.Status == ClaimUnverified) {
		status, note := ClaimConfirmed, ""
		if c.Receipt != cb.Receipt {
			status, note = ClaimRejected, "M-Pesa's callback carried receipt "+cb.Receipt
		}
		if err := q.SetClaimStatus(ctx, store.SetClaimStatusParams{Status: &status, Note: nilIfEmpty(note), At: now, ID: p.ID}); err != nil {
			return false, err
		}
	}
	if err := s.creditPayment(ctx, tx, creditInput{paymentID: p.ID, user: p.UserID, target: *p.Msisdn, amount: cb.AmountKES, receipt: cb.Receipt,
		payer: cb.MSISDN, now: now, actor: audit.Event{ActorType: audit.System}, source: "late_callback"}); err != nil {
		return false, err
	}
	slog.InfoContext(ctx, "stk callback arrived after the top-up settled; credited", "payment", p.ID, "was", p.Status)
	return true, nil
}

// settledTopUp is what lateCallback needs of a locked payment.
type settledTopUp struct {
	ID, UserID, Status string
	Msisdn             *string
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Recheck is what M-Pesa says about a top-up now (an operator asked).
type Recheck struct {
	// Outcome: credited (a paid callback had arrived and is now credited), paid_without_receipt (M-Pesa says paid; the
	// receipt is needed), failed, cancelled, pending, no_prompt (the prompt was never accepted), asking (M-Pesa is being
	// asked about the player's receipt).
	Outcome    string `json:"outcome"`
	Status     string `json:"status"` // the payment's status afterwards
	ResultCode *int32 `json:"result_code"`
	ResultDesc string `json:"result_desc,omitempty"`
}

// Recheck asks M-Pesa about a top-up again, for an operator: a paid callback that arrived (as an orphan, or after
// the payment settled) is credited; otherwise the status query is asked and a failure recorded, a success reported
// as needing the receipt. A credited or reversed top-up is left alone. M-Pesa unavailable is ErrDarajaUnavailable.
func (s *Service) Recheck(ctx context.Context, paymentID, operator string) (Recheck, error) {
	var out Recheck
	var checkout *string
	var claimOpen bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := q.LockPayment(ctx, paymentID)
		if notFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if p.Status == "credited" || p.Status == "reversed" {
			return ErrSettledAlready
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		checkout = p.CheckoutRequestID
		d, err := q.GetPaymentDetail(ctx, paymentID)
		if err != nil {
			return err
		}
		if c := claimOf(d); c != nil && (c.Status == ClaimClaimed || c.Status == ClaimChecking) {
			claimOpen = true
		}
		raw := p.RawCallback
		if raw == nil && checkout != nil {
			raw, err = q.OrphanCallback(ctx, *checkout)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if raw != nil {
			if cb, err := daraja.ParseSTKCallback(raw); err == nil && cb.Paid() && uncredited(p.Status) {
				credited, err := s.lateCallback(ctx, tx, settledTopUp{ID: p.ID, UserID: p.UserID, Status: p.Status, Msisdn: p.Msisdn}, cb, raw, now)
				if err != nil {
					return err
				}
				if credited {
					out = Recheck{Outcome: "credited", Status: "credited", ResultCode: ptr(int32(0))}
					return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payment.topup.rechecked",
						EntityType: "payment", EntityID: paymentID, Details: map[string]any{"outcome": out.Outcome}})
				}
			}
		}
		return nil
	})
	if err != nil || out.Outcome != "" {
		return out, err
	}
	if checkout == nil {
		return s.rechecked(ctx, paymentID, operator, Recheck{Outcome: "no_prompt"})
	}
	if claimOpen && s.c2b != nil {
		if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return s.enq.EnqueueClaimCheck(ctx, tx, paymentID) }); err != nil {
			return out, err
		}
	}
	if s.daraja == nil {
		return out, ErrDarajaUnavailable
	}
	st, err := s.daraja.STKQuery(ctx, *checkout)
	switch {
	case errors.Is(err, daraja.ErrRejected):
		return s.rechecked(ctx, paymentID, operator, Recheck{Outcome: "pending", ResultDesc: "M-Pesa has no answer yet: " + err.Error()})
	case err != nil:
		return out, errors.Join(ErrDarajaUnavailable, err)
	}
	code := st.ResultCode
	switch st.State {
	case daraja.Pending:
		o := "pending"
		if claimOpen && s.c2b != nil {
			o = "asking"
		}
		return s.rechecked(ctx, paymentID, operator, Recheck{Outcome: o, ResultCode: &code, ResultDesc: st.ResultDesc})
	case daraja.Failed:
		status := "failed"
		if code == 1032 {
			status = "cancelled"
		}
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			p, err := store.New(tx).LockPayment(ctx, paymentID)
			if err != nil {
				return err
			}
			if p.Status == "credited" || p.Status == "reversed" {
				return ErrSettledAlready
			}
			now, err := s.clk.Now(ctx, tx)
			if err != nil {
				return err
			}
			return s.settle(ctx, tx, p.UserID, paymentID, status, "", &code, now)
		})
		if err != nil {
			return out, err
		}
		return s.rechecked(ctx, paymentID, operator, Recheck{Outcome: status, ResultCode: &code, ResultDesc: st.ResultDesc})
	}
	o := "paid_without_receipt"
	if claimOpen && s.c2b != nil {
		o = "asking"
	}
	return s.rechecked(ctx, paymentID, operator, Recheck{Outcome: o, ResultCode: &code, ResultDesc: st.ResultDesc})
}

// rechecked fills in the payment's status and records who asked.
func (s *Service) rechecked(ctx context.Context, paymentID, operator string, r Recheck) (Recheck, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := store.New(tx).GetPayment(ctx, paymentID)
		if err != nil {
			return err
		}
		r.Status = p.Status
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payment.topup.rechecked",
			EntityType: "payment", EntityID: paymentID, Details: map[string]any{"outcome": r.Outcome, "result_code": r.ResultCode}})
	})
	return r, err
}

// Dismiss closes an unconfirmed top-up an operator could not find on the statement: it becomes failed with the
// reason not_on_statement, and any receipt the player claimed is rejected with the operator's note. A note is
// required. A top-up that already failed only has its claim rejected.
func (s *Service) Dismiss(ctx context.Context, paymentID, note, operator string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return ErrReasonRequired
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := q.LockPayment(ctx, paymentID)
		if notFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !uncredited(p.Status) {
			return ErrNotDismissable
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		d, err := q.GetPaymentDetail(ctx, paymentID)
		if err != nil {
			return err
		}
		if c := claimOf(d); c != nil && c.Status != ClaimConfirmed {
			if err := q.SetClaimStatus(ctx, store.SetClaimStatusParams{Status: ptr(ClaimRejected), Note: &note, At: now, ID: paymentID}); err != nil {
				return err
			}
		}
		if p.Status == "unconfirmed" {
			if err := q.SetPaymentOutcome(ctx, store.SetPaymentOutcomeParams{Status: "failed", FailureReason: ptr("not_on_statement"), ResultCode: p.ResultCode, At: now, ID: paymentID}); err != nil {
				return err
			}
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payment.topup.dismissed",
			EntityType: "payment", EntityID: paymentID, Details: map[string]any{"user_id": p.UserID, "was": p.Status, "note": note}})
	})
}

// Resolution is an operator crediting a top-up from a receipt they found.
type Resolution struct {
	PaymentID string
	Receipt   string
	AmountKES int64
	Note      string
	Source    string // statement, claim, or operator
}

// Resolve credits a settled, uncredited top-up from a receipt an operator found on the M-Pesa statement or in the
// player's message. It credits exactly like a confirmed callback (excise booked to the operator, debt recovered,
// phone proved) and refuses a receipt that already credited another payment. A receipt the player claimed is
// confirmed when it is the one credited, otherwise rejected with a note saying which receipt was.
func (s *Service) Resolve(ctx context.Context, in Resolution, operator string) error {
	if in.AmountKES <= 0 || in.AmountKES > daraja.MaxSTKAmountKES {
		return ErrInvalidAmount
	}
	receipt, err := NormalizeReceipt(in.Receipt)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := q.LockPayment(ctx, in.PaymentID)
		if notFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !uncredited(p.Status) || p.Msisdn == nil {
			return ErrNotResolvable
		}
		used, err := q.ReceiptUsedElsewhere(ctx, store.ReceiptUsedElsewhereParams{MpesaReceipt: &receipt, ID: in.PaymentID})
		if err != nil {
			return err
		}
		if used {
			return ErrReceiptUsed
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		d, err := q.GetPaymentDetail(ctx, in.PaymentID)
		if err != nil {
			return err
		}
		if c := claimOf(d); c != nil && c.Status != ClaimConfirmed {
			status, note := ClaimConfirmed, nilIfEmpty(in.Note)
			if c.Receipt != receipt {
				status, note = ClaimRejected, ptr("credited with receipt "+receipt+" instead")
			}
			if err := q.SetClaimStatus(ctx, store.SetClaimStatusParams{Status: &status, Note: note, At: now, ID: in.PaymentID}); err != nil {
				return err
			}
		}
		source := in.Source
		if source == "" {
			source = "operator"
		}
		actor := audit.Event{ActorType: audit.Staff, ActorID: operator}
		return s.creditPayment(ctx, tx, creditInput{paymentID: in.PaymentID, user: p.UserID, target: *p.Msisdn, amount: in.AmountKES,
			receipt: receipt, now: now, actor: actor, source: source, note: in.Note, was: p.Status})
	})
}

// ReceiptClaimsOpen lists top-ups whose claimed receipt awaits M-Pesa or an operator.
func (s *Service) ReceiptClaimsOpen(ctx context.Context) ([]string, error) {
	return store.New(s.pool).ReceiptClaims(ctx)
}
