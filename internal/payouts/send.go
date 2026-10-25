package payouts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/payments/daraja"
	"github.com/itsivali/lottery-platform/internal/payouts/store"
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func open(status string) bool {
	switch status {
	case "sent", "timed_out", "result_received":
		return true
	}
	return false
}

// Send makes the B2C request for a pending payout, exactly once. The payout is marked sending (and committed) before
// the request, so a crash during it leaves a payout that is checked, never one that is sent again. A refusal fails the
// payout and returns the money; an unknown outcome leaves it timed_out for verification. Nothing is sent while payouts
// are frozen.
func (s *Service) Send(ctx context.Context, id string) error {
	if s.daraja == nil {
		return ErrUnavailable
	}
	var p store.PayoutsPayout
	claimed := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		if p, err = q.LockPayout(ctx, id); err != nil {
			return err
		}
		if p.Status != "pending" {
			return nil
		}
		// an open alert freezes payouts (a reconciliation found the books wrong) until a person acknowledges it; the
		// payout stays pending, never sent, and the sweep queues it again
		if frozen, err := q.PayoutsFrozen(ctx); err != nil || frozen {
			if frozen {
				slog.WarnContext(ctx, "payouts are frozen by an open alert; the payout waits", "payout", id)
			}
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		claimed = true
		return q.SetPayoutStatus(ctx, store.SetPayoutStatusParams{Status: "sending", At: now, ID: id})
	})
	if err != nil || !claimed {
		return err
	}
	cmd, remarks := daraja.BusinessPayment, "Withdrawal"
	if p.Kind == "prize" {
		cmd, remarks = daraja.PromotionPayment, "Prize"
	}
	res, sendErr := s.daraja.B2C(ctx, daraja.B2CRequest{OriginatorConversationID: p.OriginatorConversationID, MSISDN: deref(p.Msisdn),
		AmountKES: p.NetKes, CommandID: cmd, Remarks: remarks, Occasion: p.ID})
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.LockPayout(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status != "sending" { // the sweep already treated it as timed out
			return nil
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		switch {
		case sendErr == nil:
			if err := q.MarkSent(ctx, store.MarkSentParams{ConversationID: &res.ConversationID, At: now, ID: id}); err != nil {
				return err
			}
		case errors.Is(sendErr, daraja.ErrRejected), errors.Is(sendErr, daraja.ErrInvalidRequest):
			slog.WarnContext(ctx, "b2c request refused; the payout fails", "payout", id, "err", sendErr)
			return s.fail(ctx, tx, cur, "b2c_refused", audit.Event{ActorType: audit.System}, now)
		default:
			slog.WarnContext(ctx, "b2c outcome unknown; checking with Daraja", "payout", id, "err", sendErr)
			if err := q.SetPayoutStatus(ctx, store.SetPayoutStatusParams{Status: "timed_out", Reason: strPtr("b2c_outcome_unknown"), At: now, ID: id}); err != nil {
				return err
			}
		}
		return s.enq.EnqueueVerify(ctx, tx, id, AfterSend)
	})
}

// Callback kinds: what Daraja posted, and to which hook.
const (
	CallbackResult        = "result"
	CallbackTimeout       = "timeout"
	CallbackStatusResult  = "status_result"
	CallbackStatusTimeout = "status_timeout"
)

// HandleCallback stores what Daraja posted to a payout hook and enqueues the payout's verification. It moves no money
// and changes no payout (the api may not). A body that matches no payout is kept unlinked. A malformed body returns
// daraja.ErrBadCallback.
func (s *Service) HandleCallback(ctx context.Context, kind string, raw []byte) error {
	var originator, occasion string
	switch kind {
	case CallbackResult, CallbackTimeout:
		r, err := daraja.ParseB2CResult(raw)
		if err != nil {
			return err
		}
		originator = r.OriginatorConversationID
	case CallbackStatusResult, CallbackStatusTimeout:
		r, err := daraja.ParseStatusResult(raw)
		if err != nil {
			return err
		}
		originator, occasion = r.OriginatorConversationID, r.Occasion
	default:
		return fmt.Errorf("payouts: unknown callback kind %q", kind)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		var id string
		var err error
		switch kind {
		case CallbackResult, CallbackTimeout:
			id, err = q.PayoutByOriginator(ctx, originator)
		default:
			// the occasion is the payout id; the originator of our status request is the fallback (a malformed id would
			// abort the transaction, so only a uuid is looked up)
			if uuidRE.MatchString(occasion) {
				var p store.PayoutsPayout
				if p, err = q.GetPayout(ctx, occasion); err == nil {
					id = p.ID
				}
			}
			if id == "" {
				id, err = q.PayoutByStatusOriginator(ctx, &originator)
			}
		}
		if err != nil && !notFound(err) {
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		var payout *string
		if id != "" {
			payout = &id
		} else {
			slog.WarnContext(ctx, "payout callback for no known payout", "kind", kind)
		}
		if err := q.InsertCallback(ctx, store.InsertCallbackParams{Kind: kind, PayoutID: payout, Originator: &originator, Raw: raw, At: now}); err != nil {
			return err
		}
		if id == "" {
			return nil
		}
		return s.enq.EnqueueVerify(ctx, tx, id, AfterCallback)
	})
}

// evidence is what Daraja has said about a payout.
type evidence struct {
	result *daraja.B2CResult    // the latest payment result (a claim)
	queue  bool                 // the latest payment report was a queue timeout: not processed
	status *daraja.StatusResult // the Transaction Status answer to the latest request, if it came
}

func (s *Service) gather(ctx context.Context, p store.PayoutsPayout) (evidence, error) {
	cbs, err := store.New(s.pool).Callbacks(ctx, &p.ID)
	if err != nil {
		return evidence{}, err
	}
	var e evidence
	for _, c := range cbs {
		switch c.Kind {
		case CallbackResult:
			if r, err := daraja.ParseB2CResult(c.Raw); err == nil {
				e.result, e.queue = &r, false
			}
		case CallbackTimeout:
			e.result, e.queue = nil, true
		case CallbackStatusResult:
			r, err := daraja.ParseStatusResult(c.Raw)
			// only the answer to the latest request counts
			if err == nil && p.StatusOriginator != nil && r.OriginatorConversationID == *p.StatusOriginator {
				e.status = &r
			}
		}
	}
	return e, nil
}

// failureClaimed reports whether Daraja said the payment did not happen: a failure result or a queue timeout.
func (e evidence) failureClaimed() bool {
	return e.queue || (e.result != nil && !e.result.Succeeded())
}

// Verify moves an open payout forward: it waits for Daraja's result, asks Transaction Status, and settles from the
// answer. It returns a positive duration when the job should look again after it; an error is an outage to retry.
func (s *Service) Verify(ctx context.Context, id string) (time.Duration, error) {
	p, err := store.New(s.pool).GetPayout(ctx, id)
	if err != nil {
		return 0, err
	}
	if !open(p.Status) {
		return 0, nil
	}
	e, err := s.gather(ctx, p)
	if err != nil {
		return 0, err
	}
	if e.status != nil {
		done, err := s.decide(ctx, id, e)
		if err != nil || done {
			return 0, err
		}
	}
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return 0, err
	}
	if e.status == nil && p.StatusRequestedAt != nil {
		if wait := p.StatusRequestedAt.Add(s.cfg.StatusWait).Sub(now); wait > 0 {
			return wait, nil
		}
	}
	if p.StatusRequests == 0 && e.result == nil && !e.queue {
		if wait := p.UpdatedAt.Add(s.cfg.ResultWait).Sub(now); wait > 0 {
			return wait, nil
		}
	}
	if int(p.StatusRequests) >= s.cfg.MaxStatusRequests {
		reason := "unconfirmed"
		if e.result != nil && e.result.Succeeded() {
			reason = "success_not_confirmed"
		}
		return 0, s.toManualTx(ctx, id, reason)
	}
	if s.daraja == nil {
		return 0, ErrUnavailable
	}
	// always by our own originator id: a receipt in a result could be a lie
	req, err := s.daraja.B2CStatus(ctx, "", p.OriginatorConversationID, p.ID)
	switch {
	case errors.Is(err, daraja.ErrRejected), errors.Is(err, daraja.ErrInvalidRequest):
		return 0, s.toManualTx(ctx, id, "status_request_rejected")
	case err != nil:
		return 0, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.LockPayout(ctx, id)
		if err != nil {
			return err
		}
		if !open(cur.Status) {
			return nil
		}
		at, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		if cur.Status != "result_received" && (e.result != nil || e.queue) {
			if err := q.SetPayoutStatus(ctx, store.SetPayoutStatusParams{Status: "result_received", Reason: cur.Reason, At: at, ID: id}); err != nil {
				return err
			}
		}
		return q.MarkStatusRequested(ctx, store.MarkStatusRequestedParams{At: &at, Originator: &req.OriginatorConversationID, ID: id})
	})
	if err != nil {
		return 0, err
	}
	return s.cfg.StatusWait, nil
}

// decide settles a payout from a Transaction Status answer. It reports false when the answer settles nothing and
// Daraja should be asked again.
func (s *Service) decide(ctx context.Context, id string, e evidence) (bool, error) {
	done := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := q.LockPayout(ctx, id)
		if err != nil {
			return err
		}
		if !open(p.Status) {
			done = true
			return nil
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		st, system := e.status, audit.Event{ActorType: audit.System}
		switch {
		case st.Completed() && st.AmountKES != p.NetKes:
			done = true
			return s.toManual(ctx, tx, p, "status_amount_mismatch", now)
		case st.Completed() && st.CreditedMSISDN() != "" && st.CreditedMSISDN() != deref(p.Msisdn):
			done = true
			return s.toManual(ctx, tx, p, "credited_elsewhere", now)
		case st.Completed():
			done = true
			return s.succeed(ctx, tx, p, st.Receipt, system, now)
		case e.failureClaimed():
			// Daraja reported a failure and Transaction Status does not contradict it
			done = true
			reason := "b2c_queue_timeout"
			if e.result != nil {
				reason = "b2c_failed_" + strconv.Itoa(int(e.result.ResultCode))
			}
			return s.fail(ctx, tx, p, reason, system, now)
		}
		return nil
	})
	return done, err
}

// succeed records a confirmed payment.
func (s *Service) succeed(ctx context.Context, tx pgx.Tx, p store.PayoutsPayout, receipt string, actor audit.Event, now time.Time) error {
	q := store.New(tx)
	// checked first: a unique violation would abort the transaction (two racing settlements: the loser rolls back and
	// its retry finds the receipt used)
	if used, err := q.ReceiptPaid(ctx, &receipt); err != nil {
		return err
	} else if used {
		return s.toManual(ctx, tx, p, "receipt_reused", now)
	}
	if err := q.SettlePayout(ctx, store.SettlePayoutParams{Status: "succeeded", Receipt: &receipt, Reason: p.Reason, SettledBy: settledBy(actor), At: &now, ID: p.ID}); err != nil {
		return err
	}
	if err := recordWithholding(ctx, tx, p, receipt, now); err != nil {
		return err
	}
	actor.Action, actor.EntityType, actor.EntityID = "payout.succeeded", "payout", p.ID
	actor.Details = map[string]any{"receipt": receipt, "net_kes": p.NetKes}
	return s.audit.Record(ctx, tx, actor)
}

// fail records a confirmed failure and gives the money back: to the wallet for a withdrawal, to its pool for a prize
// (still owed).
func (s *Service) fail(ctx context.Context, tx pgx.Tx, p store.PayoutsPayout, reason string, actor audit.Event, now time.Time) error {
	q := store.New(tx)
	if err := s.giveBack(ctx, tx, p); err != nil {
		return err
	}
	if err := q.SettlePayout(ctx, store.SettlePayoutParams{Status: "failed", Reason: &reason, SettledBy: settledBy(actor), At: &now, ID: p.ID}); err != nil {
		return err
	}
	actor.Action, actor.EntityType, actor.EntityID = "payout.failed", "payout", p.ID
	actor.Details = map[string]any{"reason": reason, "gross_kes": p.GrossKes}
	return s.audit.Record(ctx, tx, actor)
}

// giveBack reverses the payout's posting with the tax rule it was made with, so exactly what was taken comes back.
func (s *Service) giveBack(ctx context.Context, tx pgx.Tx, p store.PayoutsPayout) error {
	rate, err := store.New(tx).TaxRateText(ctx, p.ID)
	if err != nil {
		return err
	}
	r, err := ledger.ParseRate(rate)
	if err != nil {
		return err
	}
	rule := ledger.TaxRule{Code: deref(p.TaxCode), Rate: r, Rounding: ledger.Rounding(deref(p.TaxRounding))}
	switch p.Kind {
	case "withdrawal":
		posting, err := ledger.WithdrawalFailed(p.ID, p.UserID, p.GrossKes, rule)
		if err != nil {
			return err
		}
		if t := posting.WalletDelta(); t != p.GrossKes {
			return fmt.Errorf("payouts: payout %s refund %d is not its amount %d", p.ID, t, p.GrossKes)
		}
		_, err = s.wallet.Apply(ctx, tx, posting)
		return err
	case "prize":
		r, err := store.New(tx).PrizeResult(ctx, deref(p.DrawResultID))
		if err != nil {
			return err
		}
		if p.DebtOffsetKes > 0 {
			// the debt this prize repaid is owed again; recovery from deposits works under the same lock
			if _, err := s.wallet.LockDebt(ctx, tx, p.UserID); err != nil {
				return err
			}
		}
		posting, err := ledger.PrizePayoutFailedWithDebt(p.ID, r.DrawType, p.UserID, p.GrossKes, rule, p.DebtOffsetKes)
		if err != nil {
			return err
		}
		_, err = s.ledger.Post(ctx, tx, posting.Journal)
		return err
	default:
		return fmt.Errorf("payouts: payout %s: unknown kind %s", p.ID, p.Kind)
	}
}

// toManual hands the payout to an operator: its outcome could not be confirmed.
func (s *Service) toManual(ctx context.Context, tx pgx.Tx, p store.PayoutsPayout, reason string, now time.Time) error {
	if err := store.New(tx).SetPayoutStatus(ctx, store.SetPayoutStatusParams{Status: "manual", Reason: &reason, At: now, ID: p.ID}); err != nil {
		return err
	}
	slog.ErrorContext(ctx, "payout needs an operator", "payout", p.ID, "reason", reason)
	return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "payout.manual", EntityType: "payout", EntityID: p.ID,
		Details: map[string]any{"reason": reason}})
}

func (s *Service) toManualTx(ctx context.Context, id, reason string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := store.New(tx).LockPayout(ctx, id)
		if err != nil {
			return err
		}
		if !open(p.Status) {
			return nil
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		return s.toManual(ctx, tx, p, reason, now)
	})
}

func settledBy(actor audit.Event) *string {
	if actor.ActorType == audit.Staff {
		return &actor.ActorID
	}
	return nil
}

// Sweep looks at payouts left behind: a pending one whose send job never ran is queued again (it was never sent); one
// stuck in sending (the worker stopped during the request) is treated as timed out, never sent again; open ones get a
// verification.
func (s *Service) Sweep(ctx context.Context) error {
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return err
	}
	stale, err := store.New(s.pool).StalePayouts(ctx, now.Add(-s.cfg.StaleAfter))
	if err != nil {
		return err
	}
	var errs []error
	for _, st := range stale {
		errs = append(errs, pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			q := store.New(tx)
			p, err := q.LockPayout(ctx, st.ID)
			if err != nil || p.Status != st.Status {
				return err
			}
			switch p.Status {
			case "pending":
				if s.daraja == nil {
					return nil
				}
				return s.enq.EnqueueSend(ctx, tx, p.ID)
			case "sending":
				slog.WarnContext(ctx, "payout left in sending; checking with Daraja instead of sending again", "payout", p.ID)
				if err := q.SetPayoutStatus(ctx, store.SetPayoutStatusParams{Status: "timed_out", Reason: strPtr("send_interrupted"), At: now, ID: p.ID}); err != nil {
					return err
				}
			}
			return s.enq.EnqueueVerify(ctx, tx, p.ID, FromSweep)
		}))
	}
	return errors.Join(errs...)
}
