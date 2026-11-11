// Package dataexport sends a player a copy of the data the platform holds about them, to the email address on their
// account (ADR 0038; the right of access, Data Protection Act 2019 s.26). It reads every module's tables for one
// player and writes nothing but the record that a copy was sent.
package dataexport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/kyc"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/mail"
)

// Every is how often a player may be sent a copy.
const Every = 24 * time.Hour

// Operator is the data controller named in the file and the email.
const Operator = "4 DRAWS TICKET LTD"

var (
	// ErrNoEmail means the account has no email address to send to.
	ErrNoEmail = errors.New("dataexport: the account has no email address")
	// ErrNotActive means the account is closed (its email address is erased, ADR 0037).
	ErrNotActive = errors.New("dataexport: the account is closed")
)

// TooSoonError means a copy was sent less than Every ago; Next is when another may be asked for.
type TooSoonError struct{ Next time.Time }

func (e TooSoonError) Error() string { return "dataexport: a copy was sent recently" }

// Identity reads the player's own name and ID number (kyc.Service).
type Identity interface {
	OwnIdentity(ctx context.Context, userID string) (kyc.Own, error)
}

// Service builds and sends copies.
type Service struct {
	pool   *pgxpool.Pool
	clk    clock.Clock
	audit  *audit.Writer
	ident  Identity
	sender mail.Sender
}

// NewService returns a Service that sends through sender. ident may be nil (the file then has no identity section).
func NewService(pool *pgxpool.Pool, clk clock.Clock, a *audit.Writer, ident Identity, sender mail.Sender) *Service {
	return &Service{pool: pool, clk: clk, audit: a, ident: ident, sender: sender}
}

// Receipt says where a copy went.
type Receipt struct {
	// SentTo is the address with its name masked ("j***@example.com").
	SentTo      string
	RequestedAt time.Time
	// Next is when another copy may be asked for.
	Next time.Time
}

// sections is the whole file but its heading and the identity, as one JSON object built by the database: each list
// names its columns, so nothing internal (token hashes, callbacks, idempotency keys, staff ids) can ride along.
const sections = `
SELECT jsonb_build_object(
  'account', (SELECT jsonb_build_object('user_id', u.id, 'phone_number', u.msisdn, 'email', u.email, 'status', u.status,
      'identity_status', u.kyc_status, 'language', u.language, 'opened_at', u.created_at,
      'phone_confirmed_at', u.msisdn_verified_at,
      'closure_requested_at', (SELECT c.requested_at FROM identity.closure_requests c WHERE c.user_id = u.id))
    FROM identity.users u WHERE u.id = $1),
  'sign_in_links', (SELECT coalesce(jsonb_agg(jsonb_build_object('provider', a.provider, 'linked_at', a.created_at) ORDER BY a.created_at), '[]')
    FROM identity.auth_identities a WHERE a.user_id = $1),
  'devices', (SELECT coalesce(jsonb_agg(jsonb_build_object('platform', d.platform, 'first_seen_at', d.first_seen_at, 'last_seen_at', d.last_seen_at) ORDER BY d.first_seen_at), '[]')
    FROM identity.devices d WHERE d.user_id = $1),
  'sign_ins', (SELECT coalesce(jsonb_agg(jsonb_build_object('at', s.created_at, 'platform', s.platform, 'ended_at', s.revoked_at) ORDER BY s.created_at), '[]')
    FROM identity.sessions s WHERE s.user_id = $1),
  'deposit_limits', (SELECT coalesce(jsonb_agg(jsonb_build_object('period', l.period, 'amount_kes', l.amount_kes, 'requested_at', l.requested_at,
      'effective_from', l.effective_from, 'cancelled_at', l.cancelled_at) ORDER BY l.requested_at), '[]')
    FROM rg.limits l WHERE l.user_id = $1),
  'breaks_and_exclusions', (SELECT coalesce(jsonb_agg(jsonb_build_object('kind', r.kind, 'starts_at', r.starts_at, 'ends_at', r.ends_at) ORDER BY r.starts_at), '[]')
    FROM rg.restrictions r WHERE r.user_id = $1),
  'top_ups', (SELECT coalesce(jsonb_agg(jsonb_build_object('id', p.id, 'channel', p.channel, 'status', p.status,
      'amount_requested_kes', p.amount_requested_kes, 'amount_confirmed_kes', p.amount_confirmed_kes, 'phone_number', p.msisdn,
      'mpesa_receipt', p.mpesa_receipt, 'failure_reason', p.failure_reason, 'at', p.created_at, 'reversed_at', p.reversed_at) ORDER BY p.created_at), '[]')
    FROM payments.payments p WHERE p.user_id = $1),
  'wallet', (SELECT coalesce(jsonb_agg(jsonb_build_object('kind', t.kind, 'amount_kes', t.amount_kes, 'for', t.reference_type, 'at', t.created_at) ORDER BY t.created_at, t.id), '[]')
    FROM wallet.wallet_transactions t WHERE t.user_id = $1),
  'plays', (SELECT coalesce(jsonb_agg(jsonb_build_object('id', k.id, 'play_date', k.play_date, 'number_that_day', k.seq, 'price_kes', k.price_kes,
      'status', k.status, 'slot', sp.slot, 'lucky', sp.lucky, 'at', k.created_at) ORDER BY k.created_at), '[]')
    FROM tickets.tickets k LEFT JOIN game.spins sp ON sp.ticket_id = k.id WHERE k.user_id = $1),
  'draw_entries', (SELECT coalesce(jsonb_agg(jsonb_build_object('draw', d.draw_type, 'period_start', d.period_start, 'period_end', d.period_end,
      'entry_number', e.entry_no, 'from_play_on', e.play_date, 'draw_status', d.status) ORDER BY d.period_start, e.entry_no), '[]')
    FROM draws.draw_entries e JOIN draws.draws d ON d.id = e.draw_id WHERE e.user_id = $1),
  'prizes', (SELECT coalesce(jsonb_agg(jsonb_build_object('draw', d.draw_type, 'period_start', d.period_start, 'period_end', d.period_end,
      'rank', x.prize_rank, 'amount_kes', x.amount_kes, 'at', x.created_at) ORDER BY x.created_at), '[]')
    FROM draws.draw_results x JOIN draws.draw_entries e ON e.id = x.entry_id JOIN draws.draws d ON d.id = x.draw_id WHERE e.user_id = $1),
  'withdrawals_and_payouts', (SELECT coalesce(jsonb_agg(jsonb_build_object('id', o.id, 'kind', o.kind, 'status', o.status, 'gross_kes', o.gross_kes,
      'tax_kes', o.tax_kes, 'net_kes', o.net_kes, 'phone_number', o.msisdn, 'mpesa_receipt', o.mpesa_receipt, 'at', o.created_at,
      'settled_at', o.settled_at) ORDER BY o.created_at), '[]')
    FROM payouts.payouts o WHERE o.user_id = $1),
  'copies_of_this_data', (SELECT coalesce(jsonb_agg(jsonb_build_object('sent_at', x.requested_at) ORDER BY x.requested_at), '[]')
    FROM identity.data_exports x WHERE x.user_id = $1)
)`

// Send builds the player's copy and emails it to the address on their account, at most once every Every: a page to
// read (Report) and the same data as JSON. Both carry the ID number's last three digits only: email is not a safe
// place for the whole number.
func (s *Service) Send(ctx context.Context, userID string) (Receipt, error) {
	var out Receipt
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		// the account row is held, so two requests cannot both pass the once-a-day check
		var email *string
		var status string
		if err := tx.QueryRow(ctx, `SELECT email, status FROM identity.users WHERE id = $1 FOR UPDATE`, userID).Scan(&email, &status); err != nil {
			return err
		}
		if status == "closed" {
			return ErrNotActive
		}
		if email == nil || *email == "" {
			return ErrNoEmail
		}
		var last *time.Time
		if err := tx.QueryRow(ctx, `SELECT max(requested_at) FROM identity.data_exports WHERE user_id = $1`, userID).Scan(&last); err != nil {
			return err
		}
		if last != nil && now.Before(last.Add(Every)) {
			return TooSoonError{Next: last.Add(Every)}
		}

		var body json.RawMessage
		if err := tx.QueryRow(ctx, sections, userID).Scan(&body); err != nil {
			return fmt.Errorf("dataexport: read: %w", err)
		}
		doc := map[string]json.RawMessage{}
		if err := json.Unmarshal(body, &doc); err != nil {
			return err
		}
		about, _ := json.Marshal(map[string]any{
			"what":         "A copy of the personal data 4draws holds about you.",
			"controller":   Operator,
			"generated_at": now.UTC().Format(time.RFC3339),
			"times":        "UTC",
			"not_included": "Your password (stored only as a one-way hash), and the full ID number (its last three digits are shown). Write to privacy@4draws.co.ke for anything else.",
		})
		doc["about"] = about
		if s.ident != nil {
			own, err := s.ident.OwnIdentity(ctx, userID)
			if err != nil {
				return fmt.Errorf("dataexport: identity: %w", err)
			}
			ending := ""
			if n := len(own.IDNumber); n >= 3 {
				ending = own.IDNumber[n-3:]
			}
			ident, _ := json.Marshal(map[string]any{"full_name": own.FullName, "id_number_ending": ending, "status": own.Status})
			doc["identity"] = ident
		}
		file, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		page, err := Report(file)
		if err != nil {
			return fmt.Errorf("dataexport: report: %w", err)
		}
		sum := sha256.Sum256(file)
		if _, err := tx.Exec(ctx, `INSERT INTO identity.data_exports (user_id, requested_at, bytes, sha256) VALUES ($1, $2, $3, $4)`,
			userID, now, len(file), hex.EncodeToString(sum[:])); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: userID, Action: "identity.data_exported",
			EntityType: "user", EntityID: userID, Details: map[string]any{"bytes": len(file), "sha256": hex.EncodeToString(sum[:])}}); err != nil {
			return err
		}
		name := "4draws-data-" + now.In(eat).Format("2006-01-02")
		// sent before the record commits: if the mail server refuses it, nothing is recorded and the player may try again
		if err := s.sender.Send(ctx, mail.Message{
			To:      []string{*email},
			Subject: "4draws: a copy of your data",
			Text: "You asked for a copy of the personal data 4draws holds about you. Two files are attached.\n\n" +
				"1. " + name + ".html is the one to read. Open it and it shows in your browser as a page with your account, your top-ups, " +
				"plays, draws, prizes and withdrawals, your limits and breaks, and the devices you signed in with. You can print it or save it as a PDF.\n\n" +
				"2. " + name + ".json holds the same data for use by another service or program.\n\n" +
				"For your safety the files show only the last three digits of your ID number.\n\n" +
				"If you did not ask for this, change your password and write to support@4draws.co.ke.\n\n" + Operator + "\n",
			Attachments: []mail.Attachment{
				{Name: name + ".html", ContentType: "text/html", Data: page},
				{Name: name + ".json", ContentType: "application/json", Data: file},
			},
		}); err != nil {
			return fmt.Errorf("dataexport: send: %w", err)
		}
		out = Receipt{SentTo: Mask(*email), RequestedAt: now, Next: now.Add(Every)}
		return nil
	})
	return out, err
}

// Mask hides an email address's name but its first letter: "jane@example.com" → "j***@example.com".
func Mask(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}
