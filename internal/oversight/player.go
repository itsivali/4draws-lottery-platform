package oversight

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/audit"
)

// One player's timeline (owner, 2026-10-31): what happened to this player and their money, in one place, for an
// auditor, a support call or a data-subject request. Everything is read here with fixed SQL on the staff pool; the
// name and ID number come from the identity submissions through the API (which audits their unmasking), and the
// responsible-gaming limits from the rg service.

// ErrNoSuchPlayer is a player id no account has.
var ErrNoSuchPlayer = errors.New("oversight: no such player")

// PlayerRow is the account as the player list shows it. FullName and NationalID are filled in by the API.
type PlayerRow struct {
	UserID             string     `json:"user_id"`
	MSISDN             string     `json:"msisdn"`
	Email              *string    `json:"email"`
	KYCStatus          string     `json:"kyc_status"`
	Status             string     `json:"status"`
	PhoneVerified      bool       `json:"phone_verified"`
	BalanceKES         int64      `json:"balance_kes"`
	Plays              int64      `json:"plays"`
	CreatedAt          time.Time  `json:"created_at"`
	LastSignInAt       *time.Time `json:"last_sign_in_at"`
	ClosureRequestedAt *time.Time `json:"closure_requested_at"`
	FullName           string     `json:"full_name"`
	NationalID         string     `json:"national_id"`
}

// WalletTx is one movement of the wallet: credits positive, debits negative.
type WalletTx struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	AmountKES int64     `json:"amount_kes"`
	JournalID string    `json:"journal_id"`
	Reference *string   `json:"reference"` // "<type>:<id>" of what moved the money, when known
	CreatedAt time.Time `json:"created_at"`
}

// Wallet is the balance, the debt from reversed top-ups and the latest movements.
type Wallet struct {
	BalanceKES   int64      `json:"balance_kes"`
	DebtKES      int64      `json:"debt_kes"`
	Transactions []WalletTx `json:"transactions"`
}

// PlayerTopUp is one top-up of the player.
type PlayerTopUp struct {
	ID                 string    `json:"id"`
	Channel            string    `json:"channel"`
	Status             string    `json:"status"`
	AmountRequestedKES int64     `json:"amount_requested_kes"`
	AmountConfirmedKES *int64    `json:"amount_confirmed_kes"`
	Receipt            *string   `json:"receipt"`
	FailureReason      *string   `json:"failure_reason"`
	ResultCode         *int32    `json:"result_code"`
	CreatedAt          time.Time `json:"created_at"`
}

// PlayerPlay is one play of the player.
type PlayerPlay struct {
	TicketID  string    `json:"ticket_id"`
	PlayDate  string    `json:"play_date"`
	Seq       int       `json:"seq"`
	PriceKES  int64     `json:"price_kes"`
	Status    string    `json:"status"`
	Slot      *int      `json:"slot"`
	Lucky     *bool     `json:"lucky"`
	CreatedAt time.Time `json:"created_at"`
}

// Plays is how many paid plays the player has made and the latest ones.
type Plays struct {
	Count int64        `json:"count"`
	Last  []PlayerPlay `json:"last"`
}

// PlayerPayout is one payout to the player; State is the internal state.
type PlayerPayout struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	State     string    `json:"state"`
	GrossKES  int64     `json:"gross_kes"`
	TaxKES    int64     `json:"tax_kes"`
	NetKES    int64     `json:"net_kes"`
	Receipt   *string   `json:"receipt"`
	CreatedAt time.Time `json:"created_at"`
}

// KYCCheck is one identity submission's outcome, without the personal data it carries.
type KYCCheck struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Provider   string    `json:"provider"`
	ReviewedBy *string   `json:"reviewed_by"`
	Reason     *string   `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}

// Session is one sign-in and how it ended.
type Session struct {
	ID            string     `json:"id"`
	Platform      string     `json:"platform"`
	CreatedAt     time.Time  `json:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	RevokedReason *string    `json:"revoked_reason"`
	DeviceVerdict string     `json:"device_verdict"`
}

// StatusChange is one change of the account's status (ADR 0030); From is nil for its first.
type StatusChange struct {
	ID        int64     `json:"id"`
	From      *string   `json:"from"`
	To        string    `json:"to"`
	ActorType *string   `json:"actor_type"`
	Actor     *string   `json:"actor"`
	Reason    *string   `json:"reason"`
	At        time.Time `json:"at"`
}

// PlayerIdentity is the account's identity history: submissions, sign-ins, status changes and a closure request.
type PlayerIdentity struct {
	KYC                []KYCCheck     `json:"kyc"`
	Sessions           []Session      `json:"sessions"`
	StatusHistory      []StatusChange `json:"status_history"`
	ClosureRequestedAt *time.Time     `json:"closure_requested_at"`
}

// DepositLimit is a deposit limit in force, or a raise waiting out its cooling-off (Pending).
type DepositLimit struct {
	Period        string    `json:"period"`
	AmountKES     int64     `json:"amount_kes"`
	EffectiveFrom time.Time `json:"effective_from"`
	Pending       bool      `json:"pending"`
}

// Restriction is a time-out or self-exclusion in force; Until is nil for a permanent exclusion.
type Restriction struct {
	Kind  string     `json:"kind"`
	Until *time.Time `json:"until"`
}

// PlayerLimits are the responsible-gaming controls in force, filled in by the API from the rg service.
type PlayerLimits struct {
	Deposit     []DepositLimit `json:"deposit"`
	Restriction *Restriction   `json:"restriction"`
}

// Player is one player's timeline.
type Player struct {
	Player   PlayerRow      `json:"player"`
	Wallet   Wallet         `json:"wallet"`
	TopUps   []PlayerTopUp  `json:"top_ups"`
	Plays    Plays          `json:"plays"`
	Payouts  []PlayerPayout `json:"payouts"`
	Identity PlayerIdentity `json:"identity"`
	Limits   PlayerLimits   `json:"limits"`
	Events   []Event        `json:"events"`
}

// How much of each list the timeline shows; the export takes everything.
const (
	lastWalletTxs = 50
	lastTopUps    = 20
	lastPlays     = 20
	lastPayouts   = 20
	lastKYC       = 20
	lastSessions  = 10
	lastStatuses  = 20
	lastEvents    = 50
)

const (
	playerSQL = `
SELECT u.id::text, u.msisdn, u.email, u.kyc_status, u.status, (u.msisdn_verified_at IS NOT NULL), u.created_at,
       coalesce(w.balance_kes, 0)::bigint,
       (SELECT count(*) FROM tickets.tickets t WHERE t.user_id = u.id AND t.status = 'paid')::bigint,
       (SELECT max(s.created_at) FROM identity.sessions s WHERE s.user_id = u.id),
       c.requested_at
FROM identity.users u
LEFT JOIN wallet.wallets w ON w.user_id = u.id
LEFT JOIN identity.closure_requests c ON c.user_id = u.id AND c.closed_at IS NULL
WHERE u.id::text = $1`
	playerDebtSQL = `
SELECT coalesce(sum(CASE e.direction WHEN 'D' THEN e.amount_kes ELSE -e.amount_kes END), 0)::bigint
FROM ledger.ledger_entries e JOIN ledger.ledger_accounts a ON a.id = e.account_id
WHERE a.code = 'PLAYER_DEBT' AND e.user_id = $1::uuid`
	playerWalletSQL = `
SELECT id::text, kind, amount_kes, journal_id::text, reference_type, reference_id::text, created_at
FROM wallet.wallet_transactions WHERE user_id = $1::uuid ORDER BY created_at DESC, id DESC LIMIT $2`
	playerTopUpsSQL = `
SELECT id::text, channel, status, amount_requested_kes, amount_confirmed_kes, mpesa_receipt, failure_reason, result_code, created_at
FROM payments.payments WHERE user_id = $1::uuid ORDER BY created_at DESC, id DESC LIMIT $2`
	playerPlaysSQL = `
SELECT t.id::text, t.play_date::text, t.seq, t.price_kes, t.status, s.slot, s.lucky, t.created_at
FROM tickets.tickets t LEFT JOIN game.spins s ON s.ticket_id = t.id
WHERE t.user_id = $1::uuid ORDER BY t.created_at DESC, t.id DESC LIMIT $2`
	playerPayoutsSQL = `
SELECT id::text, kind, status, gross_kes, tax_kes, net_kes, mpesa_receipt, created_at
FROM payouts.payouts WHERE user_id = $1::uuid ORDER BY created_at DESC, id DESC LIMIT $2`
	playerKYCSQL = `
SELECT id::text, status, provider, reviewer_id, reviewer_reason, created_at
FROM kyc.kyc_checks WHERE user_id = $1::uuid ORDER BY created_at DESC, id DESC LIMIT $2`
	playerSessionsSQL = `
SELECT s.id::text, coalesce(s.platform, d.platform, ''), s.created_at, s.revoked_at, s.revoked_reason, coalesce(d.attestation_verdict, 'none')
FROM identity.sessions s LEFT JOIN identity.devices d ON d.id = s.device_id
WHERE s.user_id = $1::uuid ORDER BY s.created_at DESC, s.id DESC LIMIT $2`
	playerStatusSQL = `
SELECT id, from_status, to_status, actor_type, actor_id, reason, changed_at
FROM identity.user_status_history WHERE user_id = $1::uuid ORDER BY id DESC LIMIT $2`
	playerEventsSQL = `
SELECT ` + eventColumns + ` FROM audit.audit_log
WHERE actor_id = $1 OR entity_id = $1 OR details->>'user_id' = $1 ORDER BY id DESC LIMIT $2`
)

// Player reads the player's timeline: the latest of each list, or everything when all is set (the export). When
// viewer is named the view is audited as identity.player_viewed in the same transaction; the export audits itself.
// ErrNoSuchPlayer when no account has the id.
func (s *Service) Player(ctx context.Context, userID string, all bool, viewer string) (*Player, error) {
	lim := func(n int) *int {
		if all {
			return nil
		}
		return &n
	}
	p := &Player{TopUps: []PlayerTopUp{}, Payouts: []PlayerPayout{}, Events: []Event{}}
	p.Wallet.Transactions = []WalletTx{}
	p.Plays.Last = []PlayerPlay{}
	p.Identity.KYC, p.Identity.Sessions, p.Identity.StatusHistory = []KYCCheck{}, []Session{}, []StatusChange{}
	p.Limits.Deposit = []DepositLimit{}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		row := &p.Player
		err := tx.QueryRow(ctx, playerSQL, userID).Scan(&row.UserID, &row.MSISDN, &row.Email, &row.KYCStatus, &row.Status, &row.PhoneVerified,
			&row.CreatedAt, &row.BalanceKES, &row.Plays, &row.LastSignInAt, &row.ClosureRequestedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoSuchPlayer
		}
		if err != nil {
			return err
		}
		p.Wallet.BalanceKES, p.Plays.Count, p.Identity.ClosureRequestedAt = row.BalanceKES, row.Plays, row.ClosureRequestedAt
		if err := tx.QueryRow(ctx, playerDebtSQL, userID).Scan(&p.Wallet.DebtKES); err != nil {
			return err
		}
		if p.Wallet.Transactions, err = collect(ctx, tx, playerWalletSQL, userID, lim(lastWalletTxs), func(r pgx.CollectableRow) (WalletTx, error) {
			var t WalletTx
			var refType, refID *string
			err := r.Scan(&t.ID, &t.Kind, &t.AmountKES, &t.JournalID, &refType, &refID, &t.CreatedAt)
			if refType != nil && refID != nil {
				ref := *refType + ":" + *refID
				t.Reference = &ref
			}
			return t, err
		}); err != nil {
			return err
		}
		if p.TopUps, err = collect(ctx, tx, playerTopUpsSQL, userID, lim(lastTopUps), func(r pgx.CollectableRow) (PlayerTopUp, error) {
			var t PlayerTopUp
			err := r.Scan(&t.ID, &t.Channel, &t.Status, &t.AmountRequestedKES, &t.AmountConfirmedKES, &t.Receipt, &t.FailureReason, &t.ResultCode, &t.CreatedAt)
			return t, err
		}); err != nil {
			return err
		}
		if p.Plays.Last, err = collect(ctx, tx, playerPlaysSQL, userID, lim(lastPlays), func(r pgx.CollectableRow) (PlayerPlay, error) {
			var pl PlayerPlay
			var slot *int16
			err := r.Scan(&pl.TicketID, &pl.PlayDate, &pl.Seq, &pl.PriceKES, &pl.Status, &slot, &pl.Lucky, &pl.CreatedAt)
			if slot != nil {
				n := int(*slot)
				pl.Slot = &n
			}
			return pl, err
		}); err != nil {
			return err
		}
		if p.Payouts, err = collect(ctx, tx, playerPayoutsSQL, userID, lim(lastPayouts), func(r pgx.CollectableRow) (PlayerPayout, error) {
			var po PlayerPayout
			err := r.Scan(&po.ID, &po.Kind, &po.State, &po.GrossKES, &po.TaxKES, &po.NetKES, &po.Receipt, &po.CreatedAt)
			return po, err
		}); err != nil {
			return err
		}
		if p.Identity.KYC, err = collect(ctx, tx, playerKYCSQL, userID, lim(lastKYC), func(r pgx.CollectableRow) (KYCCheck, error) {
			var k KYCCheck
			err := r.Scan(&k.ID, &k.Status, &k.Provider, &k.ReviewedBy, &k.Reason, &k.CreatedAt)
			return k, err
		}); err != nil {
			return err
		}
		if p.Identity.Sessions, err = collect(ctx, tx, playerSessionsSQL, userID, lim(lastSessions), func(r pgx.CollectableRow) (Session, error) {
			var se Session
			err := r.Scan(&se.ID, &se.Platform, &se.CreatedAt, &se.RevokedAt, &se.RevokedReason, &se.DeviceVerdict)
			return se, err
		}); err != nil {
			return err
		}
		if p.Identity.StatusHistory, err = collect(ctx, tx, playerStatusSQL, userID, lim(lastStatuses), func(r pgx.CollectableRow) (StatusChange, error) {
			var c StatusChange
			err := r.Scan(&c.ID, &c.From, &c.To, &c.ActorType, &c.Actor, &c.Reason, &c.At)
			return c, err
		}); err != nil {
			return err
		}
		if p.Events, err = collect(ctx, tx, playerEventsSQL, userID, lim(lastEvents), scanEvent); err != nil {
			return err
		}
		if viewer == "" {
			return nil
		}
		return audit.NewWriter().Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: viewer, Action: "identity.player_viewed",
			EntityType: "user", EntityID: userID, Details: map[string]any{"purpose": "timeline"}})
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// collect runs a per-player query (user id, limit or nil for all) and scans every row; an empty list, never nil.
func collect[T any](ctx context.Context, tx pgx.Tx, sql, userID string, limit *int, scan func(pgx.CollectableRow) (T, error)) ([]T, error) {
	rows, err := tx.Query(ctx, sql, userID, limit)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, scan)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []T{}
	}
	return out, nil
}

// File is an export small enough to hold: its name, bytes, hash and row count.
type File struct {
	Name   string
	Data   []byte
	SHA256 string
	Rows   int
}

// PlayerPackColumns are the player pack's columns: every section shares them, and details_json carries the rest.
var PlayerPackColumns = []string{"section", "id", "at_eat", "kind", "status", "amount_kes", "reference", "details_json"}

// PlayerPack writes the player's whole timeline as one CSV file (one section per list, PlayerPackColumns) and records
// the export as identity.player_exported by viewer with the row count and the file's hash. More than ExportRowCap
// rows is refused (ErrTooManyRows).
func (s *Service) PlayerPack(ctx context.Context, p *Player, viewer string) (File, error) {
	var buf bytes.Buffer
	rows, err := WritePlayerCSV(&buf, p)
	if err != nil {
		return File{}, err
	}
	if rows > ExportRowCap {
		return File{}, ErrTooManyRows
	}
	sha := sha256.Sum256(buf.Bytes())
	f := File{Name: "player-" + p.Player.UserID + ".csv", Data: buf.Bytes(), SHA256: hex.EncodeToString(sha[:]), Rows: rows}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return audit.NewWriter().Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: viewer, Action: "identity.player_exported",
			EntityType: "user", EntityID: p.Player.UserID, Details: map[string]any{"rows": rows, "sha256": f.SHA256, "file": f.Name}})
	})
	if err != nil {
		return File{}, err
	}
	return f, nil
}

// WritePlayerCSV writes p as the player pack and returns how many rows (after the header) it wrote.
func WritePlayerCSV(w io.Writer, p *Player) (int, error) {
	cw := csv.NewWriter(w)
	n := 0
	write := func(section, id string, at time.Time, kind, status, amount, reference string, details map[string]any) {
		if cw.Error() != nil {
			return
		}
		js, _ := json.Marshal(details)
		when := ""
		if !at.IsZero() {
			when = EAT(at)
		}
		_ = cw.Write([]string{section, id, when, kind, status, amount, reference, string(js)})
		n++
	}
	num := func(v int64) string { return strconv.FormatInt(v, 10) }
	if err := cw.Write(PlayerPackColumns); err != nil {
		return 0, err
	}
	a := p.Player
	write("player", a.UserID, a.CreatedAt, "account", a.Status, num(a.BalanceKES), a.MSISDN, map[string]any{
		"full_name": a.FullName, "national_id": a.NationalID, "email": a.Email, "kyc_status": a.KYCStatus, "phone_verified": a.PhoneVerified,
		"plays": a.Plays, "last_sign_in_at": a.LastSignInAt, "closure_requested_at": a.ClosureRequestedAt, "debt_kes": p.Wallet.DebtKES})
	for _, t := range p.Wallet.Transactions {
		write("wallet_transaction", t.ID, t.CreatedAt, t.Kind, "", num(t.AmountKES), deref(t.Reference), map[string]any{"journal_id": t.JournalID})
	}
	for _, t := range p.TopUps {
		amount := t.AmountRequestedKES
		if t.AmountConfirmedKES != nil {
			amount = *t.AmountConfirmedKES
		}
		write("top_up", t.ID, t.CreatedAt, t.Channel, t.Status, num(amount), deref(t.Receipt), map[string]any{
			"amount_requested_kes": t.AmountRequestedKES, "amount_confirmed_kes": t.AmountConfirmedKES, "failure_reason": t.FailureReason, "result_code": t.ResultCode})
	}
	for _, pl := range p.Plays.Last {
		write("play", pl.TicketID, pl.CreatedAt, "play", pl.Status, num(pl.PriceKES), pl.PlayDate+"#"+strconv.Itoa(pl.Seq), map[string]any{"slot": pl.Slot, "lucky": pl.Lucky})
	}
	for _, po := range p.Payouts {
		write("payout", po.ID, po.CreatedAt, po.Kind, po.State, num(po.GrossKES), deref(po.Receipt), map[string]any{"tax_kes": po.TaxKES, "net_kes": po.NetKES})
	}
	for _, k := range p.Identity.KYC {
		write("kyc_check", k.ID, k.CreatedAt, k.Provider, k.Status, "", deref(k.ReviewedBy), map[string]any{"reason": k.Reason})
	}
	for _, se := range p.Identity.Sessions {
		status := "open"
		if se.RevokedAt != nil {
			status = "ended"
		}
		write("session", se.ID, se.CreatedAt, se.Platform, status, "", deref(se.RevokedReason), map[string]any{"revoked_at": se.RevokedAt, "device_verdict": se.DeviceVerdict})
	}
	for _, c := range p.Identity.StatusHistory {
		write("status_change", strconv.FormatInt(c.ID, 10), c.At, "status", c.To, "", deref(c.Actor),
			map[string]any{"from": c.From, "actor_type": c.ActorType, "reason": c.Reason})
	}
	for _, l := range p.Limits.Deposit {
		status := "in force"
		if l.Pending {
			status = "pending"
		}
		write("deposit_limit", l.Period, l.EffectiveFrom, "deposit_limit", status, num(l.AmountKES), "", map[string]any{})
	}
	if r := p.Limits.Restriction; r != nil {
		until := "permanent"
		if r.Until != nil {
			until = EAT(*r.Until)
		}
		write("restriction", r.Kind, time.Time{}, r.Kind, "in force", "", until, map[string]any{})
	}
	for _, e := range p.Events {
		write("event", e.ID, e.At, e.Action, e.ActorType, "", deref(e.ActorID), map[string]any{"entity_type": e.EntityType, "entity_id": e.EntityID, "details": e.Details})
	}
	cw.Flush()
	return n, cw.Error()
}
