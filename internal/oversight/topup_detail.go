package oversight

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/payments/daraja"
)

// TopUpDetail is everything the back office shows about one top-up (owner, 2026-10-31): the payment and its M-Pesa
// identifiers, Safaricom's callback as parsed and as received, the receipt the player claimed and M-Pesa's answer
// about it, orphan callbacks for its checkout, the background jobs that worked on it, the audit trail, and the
// player's standing.
type TopUpDetail struct {
	TopUp
	MerchantRequestID *string         `json:"merchant_request_id"`
	CheckoutRequestID *string         `json:"checkout_request_id"`
	ReversedAt        *time.Time      `json:"reversed_at"`
	ReversalReason    *string         `json:"reversal_reason"`
	Callback          *CallbackView   `json:"callback"`
	RawCallback       json.RawMessage `json:"raw_callback,omitempty"`
	Claim             *ClaimView      `json:"claim"`
	Orphans           []CallbackView  `json:"orphan_callbacks"`
	Jobs              []JobView       `json:"jobs"`
	Events            []Event         `json:"events"`
	Player            PlayerStanding  `json:"player"`
}

// CallbackView is Safaricom's STK result, as parsed.
type CallbackView struct {
	ReceivedAt *time.Time `json:"received_at,omitempty"`
	ResultCode int32      `json:"result_code"`
	ResultDesc string     `json:"result_desc"`
	AmountKES  int64      `json:"amount_kes,omitempty"`
	Receipt    string     `json:"receipt,omitempty"`
	Payer      string     `json:"payer,omitempty"`
	Unreadable bool       `json:"unreadable,omitempty"`
}

// ClaimView is the receipt the player gave and what came of it.
type ClaimView struct {
	Receipt    string          `json:"receipt"`
	Status     string          `json:"status"`
	ClaimedAt  time.Time       `json:"claimed_at"`
	Note       *string         `json:"note"`
	Originator *string         `json:"originator"` // the Transaction Status request
	Result     json.RawMessage `json:"result,omitempty"`
}

// JobView is a background job that worked on the payment.
type JobView struct {
	Kind        string     `json:"kind"`
	State       string     `json:"state"`
	Attempt     int32      `json:"attempt"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	FinalizedAt *time.Time `json:"finalized_at"`
	LastError   string     `json:"last_error,omitempty"`
}

// PlayerStanding is where the player stands, for the operator deciding about their money.
type PlayerStanding struct {
	Status        string  `json:"status"`
	KYCStatus     string  `json:"kyc_status"`
	PhoneVerified bool    `json:"phone_verified"`
	Email         *string `json:"email"`
	BalanceKES    int64   `json:"balance_kes"`
	TopUps        int64   `json:"top_ups"`           // all their top-ups
	CreditedKES   int64   `json:"credited_kes"`      // what they have paid in, over time
	UnconfirmedN  int64   `json:"unconfirmed_count"` // how many of theirs settled unconfirmed
}

const (
	topUpDetailSQL = `
SELECT p.id::text, p.user_id::text, u.msisdn, p.channel, p.amount_requested_kes, p.amount_confirmed_kes, p.status,
       p.failure_reason, p.result_code, p.mpesa_receipt, p.claimed_receipt, p.claim_status, p.created_at, p.updated_at,
       p.merchant_request_id, p.checkout_request_id, p.reversed_at, p.reversal_reason, p.raw_callback,
       p.claimed_at, p.claim_note, p.claim_originator, p.claim_result,
       u.status, u.kyc_status, u.msisdn_verified_at IS NOT NULL, u.email,
       coalesce((SELECT balance_kes FROM wallet.wallets w WHERE w.user_id = p.user_id), 0),
       (SELECT count(*) FROM payments.payments x WHERE x.user_id = p.user_id),
       coalesce((SELECT sum(amount_confirmed_kes) FROM payments.payments x WHERE x.user_id = p.user_id AND x.status IN ('credited','reversed')), 0),
       (SELECT count(*) FROM payments.payments x WHERE x.user_id = p.user_id AND x.status = 'unconfirmed')
FROM payments.payments p JOIN identity.users u ON u.id = p.user_id
WHERE p.id::text = $1`
	orphansSQL     = `SELECT raw_payload, received_at FROM payments.stk_orphan_callbacks WHERE checkout_request_id = $1 ORDER BY received_at DESC LIMIT 5`
	paymentJobsSQL = `SELECT kind, state::text, attempt, scheduled_at, finalized_at, coalesce(errors[array_length(errors, 1)]->>'error', '')
FROM river.river_job WHERE kind IN ('payments.stk_push','payments.stk_verify','payments.claim_check') AND args->>'payment_id' = $1
ORDER BY id DESC LIMIT 10`
	paymentEventsSQL = `SELECT id::text, at, actor_type, actor_id, action, entity_type, entity_id, details FROM audit.audit_log
WHERE (entity_type = 'payment' AND entity_id = $1) OR (entity_type = 'user' AND entity_id = $2 AND action = 'user.phone_verified' AND details->>'payment_id' = $1)
ORDER BY id DESC LIMIT 50`
)

// TopUp reads one top-up in full. A missing or malformed id is ErrNotFound.
func (s *Service) TopUp(ctx context.Context, id string) (TopUpDetail, error) {
	var d TopUpDetail
	var raw, claimResult []byte
	var claimedAt *time.Time
	var claimNote, claimOriginator *string
	err := s.pool.QueryRow(ctx, topUpDetailSQL, id).Scan(&d.ID, &d.UserID, &d.MSISDN, &d.Channel, &d.AmountRequestedKES, &d.AmountConfirmedKES, &d.Status,
		&d.FailureReason, &d.ResultCode, &d.Receipt, &d.ClaimedReceipt, &d.ClaimStatus, &d.CreatedAt, &d.UpdatedAt,
		&d.MerchantRequestID, &d.CheckoutRequestID, &d.ReversedAt, &d.ReversalReason, &raw,
		&claimedAt, &claimNote, &claimOriginator, &claimResult,
		&d.Player.Status, &d.Player.KYCStatus, &d.Player.PhoneVerified, &d.Player.Email, &d.Player.BalanceKES, &d.Player.TopUps, &d.Player.CreditedKES, &d.Player.UnconfirmedN)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	d.Sandbox = d.Receipt != nil && len(*d.Receipt) > 8 && (*d.Receipt)[:8] == "SANDBOX-"
	if raw != nil {
		cb := parseCallback(raw, nil)
		d.Callback, d.RawCallback = &cb, json.RawMessage(raw)
	}
	if d.ClaimedReceipt != nil && d.ClaimStatus != nil && claimedAt != nil {
		d.Claim = &ClaimView{Receipt: *d.ClaimedReceipt, Status: *d.ClaimStatus, ClaimedAt: *claimedAt, Note: claimNote, Originator: claimOriginator, Result: claimResult}
	}
	d.Orphans, d.Jobs, d.Events = []CallbackView{}, []JobView{}, []Event{}
	if d.CheckoutRequestID != nil {
		rows, err := s.pool.Query(ctx, orphansSQL, *d.CheckoutRequestID)
		if err != nil {
			return d, err
		}
		d.Orphans, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (CallbackView, error) {
			var payload []byte
			var at time.Time
			if err := r.Scan(&payload, &at); err != nil {
				return CallbackView{}, err
			}
			return parseCallback(payload, &at), nil
		})
		if err != nil {
			return d, err
		}
	}
	rows, err := s.pool.Query(ctx, paymentJobsSQL, d.ID)
	if err == nil { // River's tables may be absent in a bare database: the page still shows the payment
		d.Jobs, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (JobView, error) {
			var j JobView
			err := r.Scan(&j.Kind, &j.State, &j.Attempt, &j.ScheduledAt, &j.FinalizedAt, &j.LastError)
			return j, err
		})
		if err != nil {
			d.Jobs = []JobView{}
		}
	}
	erows, err := s.pool.Query(ctx, paymentEventsSQL, d.ID, d.UserID)
	if err != nil {
		return d, err
	}
	d.Events, err = pgx.CollectRows(erows, func(r pgx.CollectableRow) (Event, error) {
		var e Event
		err := r.Scan(&e.ID, &e.At, &e.ActorType, &e.ActorID, &e.Action, &e.EntityType, &e.EntityID, &e.Details)
		return e, err
	})
	return d, err
}

func parseCallback(raw []byte, at *time.Time) CallbackView {
	cb, err := daraja.ParseSTKCallback(raw)
	if err != nil {
		return CallbackView{ReceivedAt: at, Unreadable: true}
	}
	return CallbackView{ReceivedAt: at, ResultCode: cb.ResultCode, ResultDesc: cb.ResultDesc, AmountKES: cb.AmountKES, Receipt: cb.Receipt, Payer: cb.MSISDN}
}
