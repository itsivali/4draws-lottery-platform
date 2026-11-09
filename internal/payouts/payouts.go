// Package payouts sends money out through M-Pesa B2C: withdrawals of the wallet and, from slice 3, prizes
// (docs/checkpoints/phase-5.md). It mirrors money in: a payout is recorded and its ledger posting made in one
// transaction (a withdrawal debits the wallet when it is requested), and a job sends it. The B2C request is never
// repeated: one whose outcome is unknown may have paid. Daraja's result is a claim; both success and failure are
// confirmed with the Transaction Status API before a payout is final, and money returns to the wallet only on a
// confirmed failure. What cannot be confirmed goes to an operator. No network call runs inside a transaction.
package payouts

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/compliance"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/payments/daraja"
	"github.com/itsivali/lottery-platform/internal/payouts/store"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
)

// Errors.
var (
	ErrInvalidAmount     = errors.New("payouts: invalid amount")
	ErrUnavailable       = errors.New("payouts: payouts are unavailable")
	ErrNotFound          = errors.New("payouts: not found")
	ErrInDebt            = errors.New("payouts: the player owes money from a reversed top-up")
	ErrInsufficientFunds = errors.New("payouts: the balance is below the amount")
	ErrDailyLimit        = errors.New("payouts: no withdrawals left today")
	ErrState             = errors.New("payouts: the payout is not in a state that allows this")
	// ErrPayeeIneligible: the account may no longer be paid (suspended, closed, no longer verified) or its proved
	// number is not the one the payout names. It wraps the reason.
	ErrPayeeIneligible = errors.New("payouts: the account cannot be paid now")
	ErrOperator        = errors.New("payouts: an operator's name and a reason are required")
	ErrReceiptRequired = errors.New("payouts: the M-Pesa receipt of the payment is required")
	ErrBadCursor       = errors.New("payouts: bad cursor")
)

// Tax rules applied to payouts (versioned in ledger.tax_rules; the rates are placeholders, phase 5 checkpoint).
const (
	WithdrawalTax = "withdrawal_wht"
	WinningsTax   = "winnings_wht"
)

// Defaults. The bounds are placeholders until the owner sets them (phase 5 checkpoint).
const (
	DefaultMinWithdrawalKES  int64 = 50
	DefaultMaxWithdrawalKES  int64 = 150_000
	DefaultDailyWithdrawals        = 3
	DefaultManualAboveKES    int64 = 100_000
	DefaultResultWait              = 2 * time.Minute // after the request, how long to wait for Daraja's result before asking
	DefaultStatusWait              = time.Minute     // how long to wait for a Transaction Status answer
	DefaultMaxStatusRequests       = 10
	DefaultStaleAfter              = 10 * time.Minute // an open payout untouched this long is looked at again
)

// Player-facing states.
const (
	StatusProcessing = "processing"
	StatusPaid       = "paid"
	StatusFailed     = "failed"
)

func publicStatus(s string) string {
	switch s {
	case "succeeded":
		return StatusPaid
	case "failed":
		return StatusFailed
	default: // held, pending, sending, sent, timed_out, result_received, manual
		return StatusProcessing
	}
}

// Daraja is the part of the M-Pesa client payouts use.
type Daraja interface {
	B2C(ctx context.Context, r daraja.B2CRequest) (daraja.B2CAccepted, error)
	B2CStatus(ctx context.Context, receipt, originator, occasion string) (daraja.StatusRequest, error)
}

// Enqueuer schedules payout jobs inside the caller's transaction.
type Enqueuer interface {
	// EnqueueSend schedules the B2C request; it runs at most once.
	EnqueueSend(ctx context.Context, tx pgx.Tx, payoutID string) error
	// EnqueueVerify schedules verification (which waits by snoozing); jobs for the same payout and source collapse into one.
	EnqueueVerify(ctx context.Context, tx pgx.Tx, payoutID string, source Source) error
}

// Source says what asked for a verification.
type Source string

// Verification sources.
const (
	AfterSend     Source = "send"
	AfterCallback Source = "callback"
	FromSweep     Source = "sweep"
)

// Identity is what payouts need from the identity module.
type Identity interface {
	AssertCanBePaid(ctx context.Context, tx pgx.Tx, userID string) (string, error)
}

// Wallet applies postings to wallets and reports player debt.
type Wallet interface {
	Apply(ctx context.Context, tx pgx.Tx, p ledger.Posting) (int64, error)
	Debt(ctx context.Context, tx pgx.Tx, user string) (int64, error)
	// Balance is the wallet balance, 0 for a player without a wallet.
	Balance(ctx context.Context, tx pgx.Tx, user string) (int64, error)
	// LockDebt is Debt under the wallet row lock that debt recovery takes.
	LockDebt(ctx context.Context, tx pgx.Tx, user string) (int64, error)
}

// Ledger returns the tax rule in force and posts journals that touch no wallet (prizes).
type Ledger interface {
	ActiveRule(ctx context.Context, tx pgx.Tx, code string, on time.Time) (ledger.TaxRule, error)
	Post(ctx context.Context, tx pgx.Tx, j ledger.Journal) (string, error)
}

// Config tunes the service; zero values take the defaults.
type Config struct {
	MinWithdrawalKES  int64
	MaxWithdrawalKES  int64
	DailyWithdrawals  int
	ManualAboveKES    int64
	ResultWait        time.Duration
	StatusWait        time.Duration
	MaxStatusRequests int
	StaleAfter        time.Duration
}

func (c Config) withDefaults() Config {
	if c.MinWithdrawalKES <= 0 {
		c.MinWithdrawalKES = DefaultMinWithdrawalKES
	}
	if c.MaxWithdrawalKES <= 0 {
		c.MaxWithdrawalKES = DefaultMaxWithdrawalKES
	}
	if c.DailyWithdrawals <= 0 {
		c.DailyWithdrawals = DefaultDailyWithdrawals
	}
	if c.ManualAboveKES <= 0 {
		c.ManualAboveKES = DefaultManualAboveKES
	}
	if c.ResultWait <= 0 {
		c.ResultWait = DefaultResultWait
	}
	if c.StatusWait <= 0 {
		c.StatusWait = DefaultStatusWait
	}
	if c.MaxStatusRequests <= 0 {
		c.MaxStatusRequests = DefaultMaxStatusRequests
	}
	if c.StaleAfter <= 0 {
		c.StaleAfter = DefaultStaleAfter
	}
	return c
}

// Service runs payouts.
type Service struct {
	pool   *pgxpool.Pool
	clk    clock.Clock
	daraja Daraja // nil: B2C is not configured; withdrawals are refused and payouts wait
	enq    Enqueuer
	ids    Identity
	wallet Wallet
	ledger Ledger
	audit  *audit.Writer
	cfg    Config
	// insufficient is the wallet's error for a balance below a debit (payouts does not import the wallet).
	insufficient error
	gate         *compliance.Gate // the operating mode (ADR 0027); nil allows everything (tests)
}

// WithGate makes withdrawals, approvals and sends ask the operating mode (ADR 0027). A payout that may not be sent
// stays pending and the sweep offers it again; results, verification and reversals of payouts already sent go on.
func (s *Service) WithGate(g *compliance.Gate) *Service {
	s.gate = g
	return s
}

// NewService returns a Service. A nil d (an untyped nil) turns payouts off. insufficient is the error w returns when a
// debit exceeds the balance.
func NewService(pool *pgxpool.Pool, clk clock.Clock, d Daraja, enq Enqueuer, ids Identity, w Wallet, l Ledger, a *audit.Writer, cfg Config, insufficient error) *Service {
	return &Service{pool: pool, clk: clk, daraja: d, enq: enq, ids: ids, wallet: w, ledger: l, audit: a, cfg: cfg.withDefaults(), insufficient: insufficient}
}

// Enabled reports whether B2C is configured.
func (s *Service) Enabled() bool { return s.daraja != nil }

// WithdrawalBounds is the range a withdrawal may be, in whole KES (Config, or the defaults), for the apps to show.
func (s *Service) WithdrawalBounds() (minKES, maxKES int64) {
	return s.cfg.MinWithdrawalKES, s.cfg.MaxWithdrawalKES
}

// Proposal is one staff member's "not paid" settlement of a manual payout, waiting for a second to confirm it (ADR
// 0032). A "paid" settlement is a receipt check instead, decided by M-Pesa (ADR 0034).
type Proposal struct {
	Paid    bool      `json:"paid"`
	Receipt string    `json:"receipt,omitempty"`
	Reason  string    `json:"reason,omitempty"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
}

// Payout is what is known about a payout. Status is what the player sees; State is the internal state.
type Payout struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	State    string `json:"state"`
	GrossKES int64  `json:"gross_kes"`
	TaxKES   int64  `json:"tax_kes"`
	NetKES   int64  `json:"net_kes"`
	DebtKES  int64  `json:"debt_repaid_kes"`
	Receipt  string `json:"receipt,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Refund is set on a refund to source (ADR 0031): why, and which admin asked for it. Staff views only.
	Refund *Refund `json:"refund,omitempty"`
	// Approvals are the staff members who approved a held payout so far (ADR 0032; staff views only).
	Approvals []string `json:"approvals,omitempty"`
	// Proposal is the open settlement proposal of a manual payout, waiting for a second person (staff views only).
	Proposal *Proposal `json:"proposal,omitempty"`
	// Check is the latest receipt staff claimed for a manual payout and M-Pesa\'s answer (ADR 0034; staff views only).
	Check     *ReceiptCheck `json:"check,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func view(p store.PayoutsPayout) Payout {
	v := Payout{ID: p.ID, UserID: p.UserID, Kind: p.Kind, Status: publicStatus(p.Status), State: p.Status, GrossKES: p.GrossKes,
		TaxKES: p.TaxKes, NetKES: p.NetKes, DebtKES: p.DebtOffsetKes, Receipt: deref(p.MpesaReceipt), Reason: deref(p.Reason), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
	if p.RefundReason != nil {
		v.Refund = &Refund{Reason: *p.RefundReason, RequestedBy: deref(p.RefundRequestedBy), MSISDN: deref(p.Msisdn)}
	}
	return v
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func strPtr(s string) *string { return &s }

func notFound(err error) bool {
	var pe *pgconn.PgError
	return errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02")
}

// rateText writes a rate the way numeric(6,5) stores it.
func rateText(r ledger.Rate) string {
	e5 := r.E5()
	frac := strconv.FormatInt(e5%100_000, 10)
	return strconv.FormatInt(e5/100_000, 10) + "." + strings.Repeat("0", 5-len(frac)) + frac
}

// startOfDay is midnight Nairobi of t's Nairobi day.
func startOfDay(t time.Time) time.Time {
	y, m, d := clock.InNairobi(t).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, clock.Nairobi)
}

// RequestWithdrawal records a withdrawal of amount from the player's wallet to their proved phone, debits the wallet
// and queues the payment (or holds it for an operator above the manual threshold). key is the client's idempotency
// key: the same key returns the same payout, and replayed reports that.
func (s *Service) RequestWithdrawal(ctx context.Context, userID string, amount int64, key string) (Payout, bool, error) {
	if amount < s.cfg.MinWithdrawalKES || amount > s.cfg.MaxWithdrawalKES {
		return Payout{}, false, ErrInvalidAmount
	}
	if s.daraja == nil {
		return Payout{}, false, ErrUnavailable
	}
	var id string
	replayed := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		// one withdrawal decision at a time per player, so the daily count and the balance see every request
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('payouts.withdraw'), hashtext($1))", userID); err != nil {
			return err
		}
		existing, err := q.PayoutByKey(ctx, store.PayoutByKeyParams{UserID: userID, IdempotencyKey: key})
		if err == nil {
			id, replayed = existing, true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := s.gate.Assert(ctx, tx, compliance.OpWithdrawal); err != nil {
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		msisdn, err := s.ids.AssertCanBePaid(ctx, tx, userID)
		if err != nil {
			return err
		}
		if debt, err := s.wallet.Debt(ctx, tx, userID); err != nil {
			return err
		} else if debt > 0 {
			return ErrInDebt
		}
		n, err := q.WithdrawalsSince(ctx, store.WithdrawalsSinceParams{UserID: userID, Since: startOfDay(now)})
		if err != nil {
			return err
		}
		if int(n) >= s.cfg.DailyWithdrawals {
			return ErrDailyLimit
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
		status := "pending"
		if amount > s.cfg.ManualAboveKES {
			status = "held"
		}
		if id, err = q.InsertPayout(ctx, store.InsertPayoutParams{UserID: userID, Kind: "withdrawal", Route: "b2c", Status: status, GrossKes: amount,
			TaxKes: tax, NetKes: amount - tax, Msisdn: &msisdn, IdempotencyKey: key, TaxCode: strPtr(WithdrawalTax),
			TaxRate: rateText(rule.Rate), TaxRounding: strPtr(string(rule.Rounding)), At: now}); err != nil {
			return err
		}
		p, err := ledger.Withdrawal(id, userID, amount, rule)
		if err != nil {
			return err
		}
		if _, err := s.wallet.Apply(ctx, tx, p); err != nil {
			return err
		}
		if status == "pending" {
			if err := s.enq.EnqueueSend(ctx, tx, id); err != nil {
				return err
			}
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: userID, Action: "payout.withdrawal.requested",
			EntityType: "payout", EntityID: id, Details: map[string]any{"amount_kes": amount, "tax_kes": tax, "held": status == "held"}})
	})
	if err != nil {
		if errors.Is(err, s.insufficient) {
			return Payout{}, false, ErrInsufficientFunds
		}
		return Payout{}, false, err
	}
	p, err := s.Get(ctx, userID, id)
	return p, replayed, err
}

// Get returns one of the player's payouts. Another player's payout is ErrNotFound.
func (s *Service) Get(ctx context.Context, userID, id string) (Payout, error) {
	p, err := store.New(s.pool).GetPayout(ctx, id)
	if notFound(err) || (err == nil && p.UserID != userID) {
		return Payout{}, ErrNotFound
	}
	if err != nil {
		return Payout{}, err
	}
	return view(p), nil
}

// List returns the player's payouts, newest first.
func (s *Service) List(ctx context.Context, userID string, limit int) ([]Payout, error) {
	rows, err := store.New(s.pool).PlayerPayouts(ctx, store.PlayerPayoutsParams{UserID: userID, MaxRows: int32(limit)}) //nolint:gosec // small limits
	if err != nil {
		return nil, err
	}
	out := make([]Payout, 0, len(rows))
	for _, p := range rows {
		out = append(out, view(p))
	}
	return out, nil
}

// Unsettled counts the player's payouts not yet paid or failed, in the caller's transaction.
func (s *Service) Unsettled(ctx context.Context, tx pgx.Tx, userID string) (int, error) {
	n, err := store.New(tx).UnsettledPayouts(ctx, userID)
	return int(n), err
}

// PrizeInterest is what a player may still win or be owed from draws (see the PrizeInterest query).
type PrizeInterest struct {
	OpenDrawTickets, UndecidedEntries, UnpaidPrizes int
}

// Any reports whether anything remains.
func (p PrizeInterest) Any() bool { return p.OpenDrawTickets+p.UndecidedEntries+p.UnpaidPrizes > 0 }

// PrizeInterestOf reads it in the caller's transaction.
func (s *Service) PrizeInterestOf(ctx context.Context, tx pgx.Tx, userID string) (PrizeInterest, error) {
	r, err := store.New(tx).PrizeInterest(ctx, userID)
	return PrizeInterest{OpenDrawTickets: int(r.OpenDrawTickets), UndecidedEntries: int(r.UndecidedEntries), UnpaidPrizes: int(r.UnpaidPrizes)}, err
}
