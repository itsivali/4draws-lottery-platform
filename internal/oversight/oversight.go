// Package oversight reads what the back office shows about the system as a whole (owner, 2026-10-30): every play and
// every top-up with who made it and when, the audit trail, and the system's state (its settings, the job queue and the
// last reconciliation runs). It only reads, on the staff database pool, with fixed SQL; names are added by the API
// from identity submissions, which audits their unmasking.
package oversight

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/platform/clock"
)

// ErrCursor is a paging cursor that is not one this package handed out.
var ErrCursor = errors.New("oversight: invalid cursor")

// ErrNotFound is a top-up that does not exist (or an id that is not one).
var ErrNotFound = errors.New("oversight: not found")

// Settings are the system's settings that matter to an operator, as the api was started with. No secrets.
type Settings struct {
	Daraja            string `json:"daraja"`             // sandbox, production, or a mock's address; "" when top-ups are off
	SandboxSettlement bool   `json:"sandbox_settlement"` // DARAJA_SANDBOX_SETTLE
	TopUpMinKES       int64  `json:"top_up_min_kes"`     // TOP_UP_MIN_KES (0: the default)
	TopUpCooldownS    int64  `json:"top_up_cooldown_s"`  // TOP_UP_COOLDOWN
	BuyGoods          bool   `json:"buy_goods"`          // prompts pay a till rather than a paybill
	Paybill           bool   `json:"paybill"`            // paybill (C2B) payments configured
	Payouts           bool   `json:"payouts"`            // B2C configured
	Email             bool   `json:"email"`              // SMTP configured
	GoogleSignIn      bool   `json:"google_sign_in"`     // players
	AppleSignIn       bool   `json:"apple_sign_in"`      // players
	Attestation       string `json:"attestation"`        // ATTESTATION_MODE
	KYCRegistry       bool   `json:"kyc_registry"`       // an automatic identity registry (else manual review)
	AlertsDelivery    string `json:"alerts_delivery"`    // webhook, or log when none is set
	Heartbeat         bool   `json:"heartbeat"`          // a dead man's switch is pinged
	Workers           string `json:"workers"`            // in-process, or a separate worker
	StartedAt         string `json:"started_at"`         // when this api process started (RFC 3339)
	Version           string `json:"version,omitempty"`  // the deployed commit, when the host says
	PlayPriceKES      *int64 `json:"play_price_kes"`     // today's price of a play
	WheelSlots        *int   `json:"wheel_slots"`        // today's wheel
	WheelLuckySlots   *int   `json:"wheel_lucky_slots"`  // today's lucky slots
}

// Service reads.
type Service struct {
	pool     *pgxpool.Pool
	clk      clock.Clock
	settings Settings
}

// NewService returns a Service that reports settings as the api's own.
func NewService(pool *pgxpool.Pool, clk clock.Clock, settings Settings) *Service {
	return &Service{pool: pool, clk: clk, settings: settings}
}

// Page is a page of rows, newest first, and the cursor for the next (empty at the end).
type Page[T any] struct {
	Rows       []T
	NextBefore string
}

// cursor is "created_at|id": the last row of a page.
func cursor(at time.Time, id string) string { return at.UTC().Format(time.RFC3339Nano) + "|" + id }

func parseCursor(s string) (*time.Time, *string, error) {
	if s == "" {
		return nil, nil, nil
	}
	at, id, ok := strings.Cut(s, "|")
	t, err := time.Parse(time.RFC3339Nano, at)
	if !ok || err != nil || id == "" {
		return nil, nil, ErrCursor
	}
	return &t, &id, nil
}

func like(q string) *string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	q = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q)
	q = "%" + q + "%"
	return &q
}

// Play is one paid play: who, when, what it cost and where the wheel stopped.
type Play struct {
	TicketID  string    `json:"ticket_id"`
	UserID    string    `json:"user_id"`
	MSISDN    string    `json:"msisdn"`
	FullName  *string   `json:"full_name"` // from the identity submission, added by the API
	PlayDate  string    `json:"play_date"`
	Seq       int       `json:"seq"`
	PriceKES  int64     `json:"price_kes"`
	Status    string    `json:"status"` // paid or refunded
	Slot      *int      `json:"slot"`
	Lucky     *bool     `json:"lucky"`
	CreatedAt time.Time `json:"created_at"`
}

// PlayFilter narrows the plays: Query matches the player's number or id, Day a Nairobi play date (YYYY-MM-DD).
type PlayFilter struct {
	Query string
	Day   string
}

const playsSQL = `
SELECT t.id::text, t.user_id::text, u.msisdn, t.play_date::text, t.seq, t.price_kes, t.status, s.slot, s.lucky, t.created_at
FROM tickets.tickets t
JOIN identity.users u ON u.id = t.user_id
LEFT JOIN game.spins s ON s.ticket_id = t.id
WHERE ($1::timestamptz IS NULL OR (t.created_at, t.id::text) < ($1, $2::text))
  AND ($3::text IS NULL OR u.msisdn LIKE $3 OR t.user_id::text LIKE $3)
  AND ($4::date IS NULL OR t.play_date = $4)
ORDER BY t.created_at DESC, t.id DESC
LIMIT $5`

// Plays lists plays, newest first.
func (s *Service) Plays(ctx context.Context, f PlayFilter, limit int, before string) (Page[Play], error) {
	at, id, err := parseCursor(before)
	if err != nil {
		return Page[Play]{}, err
	}
	var day *string
	if f.Day != "" {
		if _, err := time.Parse(time.DateOnly, f.Day); err != nil {
			return Page[Play]{}, ErrCursor
		}
		day = &f.Day
	}
	rows, err := s.pool.Query(ctx, playsSQL, at, id, like(f.Query), day, limit+1)
	if err != nil {
		return Page[Play]{}, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Play, error) {
		var p Play
		var slot *int16
		err := r.Scan(&p.TicketID, &p.UserID, &p.MSISDN, &p.PlayDate, &p.Seq, &p.PriceKES, &p.Status, &slot, &p.Lucky, &p.CreatedAt)
		if slot != nil {
			n := int(*slot)
			p.Slot = &n
		}
		return p, err
	})
	if err != nil {
		return Page[Play]{}, err
	}
	return page(out, limit, func(p Play) string { return cursor(p.CreatedAt, p.TicketID) }), nil
}

func page[T any](rows []T, limit int, key func(T) string) Page[T] {
	if rows == nil {
		rows = []T{}
	}
	if len(rows) <= limit {
		return Page[T]{Rows: rows}
	}
	rows = rows[:limit]
	return Page[T]{Rows: rows, NextBefore: key(rows[len(rows)-1])}
}

// TopUp is one top-up (an M-Pesa prompt, or a paybill payment matched to the player) and how it ended.
type TopUp struct {
	ID                 string    `json:"id"`
	UserID             string    `json:"user_id"`
	MSISDN             string    `json:"msisdn"`
	FullName           *string   `json:"full_name"`
	Channel            string    `json:"channel"` // stk or c2b
	AmountRequestedKES int64     `json:"amount_requested_kes"`
	AmountConfirmedKES *int64    `json:"amount_confirmed_kes"`
	Status             string    `json:"status"`
	FailureReason      *string   `json:"failure_reason"`
	ResultCode         *int32    `json:"result_code"`
	Receipt            *string   `json:"receipt"`
	Sandbox            bool      `json:"sandbox"`         // credited by sandbox settlement, not confirmed by M-Pesa
	ClaimedReceipt     *string   `json:"claimed_receipt"` // the receipt the player gave for an uncredited top-up
	ClaimStatus        *string   `json:"claim_status"`    // claimed, checking, confirmed, unverified or rejected
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// TopUpFilter narrows the top-ups: Query matches the number, player id, payment id, checkout id or a receipt;
// Status one internal status; Reason a failure reason; Channel stk or c2b; From/To a created-at window (To
// exclusive); Claimed only top-ups whose player gave a receipt that awaits M-Pesa or an operator.
type TopUpFilter struct {
	Query   string
	Status  string
	Reason  string
	Channel string
	From    *time.Time
	To      *time.Time
	Claimed bool
}

const topUpsSQL = `
SELECT p.id::text, p.user_id::text, u.msisdn, p.channel, p.amount_requested_kes, p.amount_confirmed_kes, p.status,
       p.failure_reason, p.result_code, p.mpesa_receipt, p.claimed_receipt, p.claim_status, p.created_at, p.updated_at
FROM payments.payments p
JOIN identity.users u ON u.id = p.user_id
WHERE ($1::timestamptz IS NULL OR (p.created_at, p.id::text) < ($1, $2::text))
  AND ($3::text IS NULL OR u.msisdn LIKE $3 OR p.user_id::text LIKE $3 OR p.mpesa_receipt LIKE $3
       OR p.id::text LIKE $3 OR p.checkout_request_id LIKE $3 OR p.claimed_receipt LIKE $3)
  AND ($4::text IS NULL OR p.status = $4)
  AND ($6::text IS NULL OR p.failure_reason = $6)
  AND ($7::text IS NULL OR p.channel = $7)
  AND ($8::timestamptz IS NULL OR p.created_at >= $8)
  AND ($9::timestamptz IS NULL OR p.created_at < $9)
  AND (NOT $10::bool OR p.claim_status IN ('claimed','checking','unverified'))
ORDER BY p.created_at DESC, p.id DESC
LIMIT $5`

// TopUps lists top-ups, newest first.
func (s *Service) TopUps(ctx context.Context, f TopUpFilter, limit int, before string) (Page[TopUp], error) {
	at, id, err := parseCursor(before)
	if err != nil {
		return Page[TopUp]{}, err
	}
	var status *string
	if f.Status != "" {
		status = &f.Status
	}
	rows, err := s.pool.Query(ctx, topUpsSQL, at, id, like(f.Query), status, limit+1, nilIfEmpty(f.Reason), nilIfEmpty(f.Channel), f.From, f.To, f.Claimed)
	if err != nil {
		return Page[TopUp]{}, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (TopUp, error) {
		var t TopUp
		err := r.Scan(&t.ID, &t.UserID, &t.MSISDN, &t.Channel, &t.AmountRequestedKES, &t.AmountConfirmedKES, &t.Status,
			&t.FailureReason, &t.ResultCode, &t.Receipt, &t.ClaimedReceipt, &t.ClaimStatus, &t.CreatedAt, &t.UpdatedAt)
		t.Sandbox = t.Receipt != nil && strings.HasPrefix(*t.Receipt, "SANDBOX-")
		return t, err
	})
	if err != nil {
		return Page[TopUp]{}, err
	}
	return page(out, limit, func(t TopUp) string { return cursor(t.CreatedAt, t.ID) }), nil
}

// JobState counts one kind of background job in one state.
type JobState struct {
	Kind  string `json:"kind"`
	State string `json:"state"`
	Count int64  `json:"count"`
}

// FailedJob is a job that ran out of attempts or keeps failing, with its last error.
type FailedJob struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	State     string    `json:"state"`
	Attempt   int       `json:"attempt"`
	LastError string    `json:"last_error"`
	At        time.Time `json:"at"`
}

// Reconciliation is the last run of one area's reconciliation.
type Reconciliation struct {
	Area  string     `json:"area"` // payments, game, draws or payouts
	RanAt *time.Time `json:"ran_at"`
	OK    *bool      `json:"ok"`
}

// System is the system's state now.
type System struct {
	Now             time.Time        `json:"now"`
	Settings        Settings         `json:"settings"`
	Jobs            []JobState       `json:"jobs"`
	FailedJobs      []FailedJob      `json:"failed_jobs"`
	Reconciliations []Reconciliation `json:"reconciliations"`
	OpenAlerts      int64            `json:"open_alerts"`
	Players         int64            `json:"players"`
	PlaysToday      int64            `json:"plays_today"`
	TopUpsPending   int64            `json:"top_ups_pending"`
	// TopUpsUnconfirmed need an operator (settled without M-Pesa's confirmation); ReceiptClaims are receipts players
	// gave that await M-Pesa or an operator.
	TopUpsUnconfirmed int64 `json:"top_ups_unconfirmed"`
	ReceiptClaims     int64 `json:"receipt_claims"`
}

const (
	jobsSQL = `SELECT kind, state::text, count(*) FROM river.river_job
WHERE state <> 'completed' OR finalized_at > now() - interval '1 day' GROUP BY kind, state ORDER BY kind, state`
	failedJobsSQL = `SELECT id, kind, state::text, attempt, coalesce(errors[array_length(errors, 1)]->>'error', ''),
       coalesce(finalized_at, attempted_at, created_at)
FROM river.river_job WHERE state IN ('discarded', 'retryable') ORDER BY id DESC LIMIT 20`
	reconSQL = `SELECT area, ran_at, ok FROM (
  (SELECT 'payments' AS area, ran_at, ok FROM payments.reconciliation_runs ORDER BY ran_at DESC LIMIT 1)
  UNION ALL (SELECT 'game', ran_at, ok FROM game.reconciliation_runs ORDER BY ran_at DESC LIMIT 1)
  UNION ALL (SELECT 'draws', ran_at, ok FROM draws.reconciliation_runs ORDER BY ran_at DESC LIMIT 1)
  UNION ALL (SELECT 'payouts', ran_at, ok FROM payouts.reconciliation_runs ORDER BY ran_at DESC LIMIT 1)) r`
	countsSQL = `SELECT
  (SELECT count(*) FROM audit.alerts WHERE resolved_at IS NULL),
  (SELECT count(*) FROM identity.users WHERE status <> 'closed'),
  (SELECT count(*) FROM tickets.tickets WHERE play_date = (now() AT TIME ZONE 'Africa/Nairobi')::date),
  (SELECT count(*) FROM payments.payments WHERE status IN ('initiated','stk_sent','callback_received')),
  (SELECT count(*) FROM payments.payments WHERE status = 'unconfirmed'),
  (SELECT count(*) FROM payments.payments WHERE claim_status IN ('claimed','checking','unverified'))`
	oddsSQL = `SELECT price_kes, slot_count, lucky_slots FROM game.odds_config
WHERE effective_from <= (now() AT TIME ZONE 'Africa/Nairobi')::date ORDER BY effective_from DESC LIMIT 1`
)

// System reads the system's state. A part that cannot be read (River's tables missing in a bare database) is left
// empty rather than failing the page.
func (s *Service) System(ctx context.Context) (System, error) {
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return System{}, err
	}
	out := System{Now: now, Settings: s.settings, Jobs: []JobState{}, FailedJobs: []FailedJob{}, Reconciliations: []Reconciliation{}}
	if rows, err := s.pool.Query(ctx, jobsSQL); err == nil {
		if js, err := pgx.CollectRows(rows, pgx.RowToStructByPos[JobState]); err == nil {
			out.Jobs = js
		}
	}
	if rows, err := s.pool.Query(ctx, failedJobsSQL); err == nil {
		if fj, err := pgx.CollectRows(rows, pgx.RowToStructByPos[FailedJob]); err == nil {
			out.FailedJobs = fj
		}
	}
	rows, err := s.pool.Query(ctx, reconSQL)
	if err != nil {
		return System{}, err
	}
	recon, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Reconciliation])
	if err != nil {
		return System{}, err
	}
	seen := map[string]Reconciliation{}
	for _, r := range recon {
		seen[r.Area] = r
	}
	for _, a := range []string{"payments", "game", "draws", "payouts"} {
		r, ok := seen[a]
		if !ok {
			r = Reconciliation{Area: a}
		}
		out.Reconciliations = append(out.Reconciliations, r)
	}
	if err := s.pool.QueryRow(ctx, countsSQL).Scan(&out.OpenAlerts, &out.Players, &out.PlaysToday, &out.TopUpsPending, &out.TopUpsUnconfirmed, &out.ReceiptClaims); err != nil {
		return System{}, err
	}
	var price int64
	var slots, lucky int16
	if err := s.pool.QueryRow(ctx, oddsSQL).Scan(&price, &slots, &lucky); err == nil {
		sl, lu := int(slots), int(lucky)
		out.Settings.PlayPriceKES, out.Settings.WheelSlots, out.Settings.WheelLuckySlots = &price, &sl, &lu
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return System{}, err
	}
	return out, nil
}

func nilIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// ErrArea is a reconciliation area that is not payments, game, draws or payouts.
var ErrArea = errors.New("oversight: unknown reconciliation area")

// Run is one stored reconciliation run: when it ran, whether the books matched, and how many findings of each kind it
// made (the findings themselves are in the run's report, shown when the reconciliation is run from the back office).
type Run struct {
	ID         string           `json:"id"`
	Area       string           `json:"area"`
	RanAt      time.Time        `json:"ran_at"`
	OK         bool             `json:"ok"`
	Mismatches map[string]int   `json:"mismatches"`
	Attention  map[string]int   `json:"attention"`
	Totals     map[string]int64 `json:"totals"`
}

// runsSQL lists one area's runs, newest first; the table is fixed per area (no SQL is built from the area).
var runsSQL = map[string]string{
	"payments": runsFrom("payments.reconciliation_runs"),
	"game":     runsFrom("game.reconciliation_runs"),
	"draws":    runsFrom("draws.reconciliation_runs"),
	"payouts":  runsFrom("payouts.reconciliation_runs"),
}

func runsFrom(table string) string {
	return `SELECT id::text, ran_at, ok, report FROM ` + table + `
WHERE ($1::timestamptz IS NULL OR (ran_at, id::text) < ($1, $2::text))
ORDER BY ran_at DESC, id DESC
LIMIT $3`
}

// Reconciliations lists area's stored runs, newest first.
func (s *Service) Reconciliations(ctx context.Context, area string, limit int, before string) (Page[Run], error) {
	sql, ok := runsSQL[area]
	if !ok {
		return Page[Run]{}, ErrArea
	}
	at, id, err := parseCursor(before)
	if err != nil {
		return Page[Run]{}, err
	}
	rows, err := s.pool.Query(ctx, sql, at, id, limit+1)
	if err != nil {
		return Page[Run]{}, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Run, error) {
		run := Run{Area: area, Mismatches: map[string]int{}, Attention: map[string]int{}, Totals: map[string]int64{}}
		var report []byte
		if err := r.Scan(&run.ID, &run.RanAt, &run.OK, &report); err != nil {
			return run, err
		}
		var rep struct {
			Mismatches map[string][]string `json:"mismatches"`
			Attention  map[string][]string `json:"attention"`
			Totals     map[string]int64    `json:"totals"`
		}
		if err := json.Unmarshal(report, &rep); err != nil {
			return run, err
		}
		for k, v := range rep.Mismatches {
			if len(v) > 0 {
				run.Mismatches[k] = len(v)
			}
		}
		for k, v := range rep.Attention {
			if len(v) > 0 {
				run.Attention[k] = len(v)
			}
		}
		if rep.Totals != nil {
			run.Totals = rep.Totals
		}
		return run, nil
	})
	if err != nil {
		return Page[Run]{}, err
	}
	return page(out, limit, func(r Run) string { return cursor(r.RanAt, r.ID) }), nil
}
