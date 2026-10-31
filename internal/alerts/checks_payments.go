package alerts

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/alerts/store"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
)

// Kinds of alert about money that needs a human (owner, 2026-10-31: "expand the alerts system").
const (
	KindTopUpUnconfirmed     = "topup.unconfirmed"
	KindTopUpOverduePending  = "topup.overdue_pending"
	KindTopUpOrphanCallbacks = "topup.orphan_callbacks"
	// KindTopUpReceiptClaim: a receipt a player gave for an uncredited top-up that M-Pesa did not confirm (warn,
	// page after TopUpUnconfirmedPage), or has not answered about after ReceiptClaimWait (warn).
	KindTopUpReceiptClaim = "topup.receipt_claim"
	KindSuspenseOpen      = "paybill.suspense_open"
	KindPaybillUnverified = "paybill.unverified"
	KindPayoutAwaiting    = "payout.awaiting_approval"
	KindPayoutFailed      = "payout.failed"
	KindB2CFloatLow       = "b2c.float_low"
)

// TopUpsCheck follows top-ups that need a person: one M-Pesa may have taken but never confirmed (warns, then pages;
// cleared when an operator resolves it from the statement), one still pending past its expiry and a grace (a stuck
// verify job; page), and STK callbacks of the last day that matched no payment (warn, per checkout id).
// ReceiptClaimWait is how long a receipt claim may wait for M-Pesa's answer before a person is told.
const ReceiptClaimWait = 15 * time.Minute

func TopUpsCheck(pool *pgxpool.Pool, clk clock.Clock, expiry time.Duration, th Thresholds) Check {
	return Check{Name: "topups", Kinds: []string{KindTopUpUnconfirmed, KindTopUpOverduePending, KindTopUpOrphanCallbacks, KindTopUpReceiptClaim}, Run: func(ctx context.Context) ([]Alert, error) {
		now, err := clk.Now(ctx, pool)
		if err != nil {
			return nil, err
		}
		q := store.New(pool)
		var out []Alert
		unconfirmed, err := q.UnconfirmedTopUps(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range unconfirmed {
			age := now.Sub(p.CreatedAt)
			if age < th.TopUpUnconfirmedWarn {
				continue
			}
			sev := Warn
			if age >= th.TopUpUnconfirmedPage {
				sev = Page
			}
			out = append(out, Alert{Kind: KindTopUpUnconfirmed, Subject: p.ID, Severity: sev,
				Summary: "A top-up of KES " + strconv.FormatInt(p.AmountRequestedKes, 10) + " from " + maskMSISDN(p.Msisdn) + " is not confirmed " +
					ages(age) + " after the player paid: credit or fail it from the M-Pesa statement",
				Details: map[string]any{"payment": p.ID, "amount_kes": p.AmountRequestedKes, "msisdn": maskMSISDN(p.Msisdn), "reason": deref(p.FailureReason),
					"started_at": p.CreatedAt, "age": ages(age)}})
		}
		pending, err := q.PendingTopUpsStartedBefore(ctx, now.Add(-(expiry + th.TopUpPendingGrace)))
		if err != nil {
			return nil, err
		}
		for _, p := range pending {
			age := now.Sub(p.CreatedAt)
			out = append(out, Alert{Kind: KindTopUpOverduePending, Subject: p.ID, Severity: Page,
				Summary: "A top-up of KES " + strconv.FormatInt(p.AmountRequestedKes, 10) + " is still " + p.Status + " " + ages(age) +
					" after it started: the verify job that settles it did not run",
				Details: map[string]any{"payment": p.ID, "status": p.Status, "amount_kes": p.AmountRequestedKes, "msisdn": maskMSISDN(p.Msisdn),
					"started_at": p.CreatedAt, "age": ages(age)}})
		}
		claims, err := q.OpenReceiptClaims(ctx)
		if err != nil {
			return nil, err
		}
		for _, c := range claims {
			age := now.Sub(*c.ClaimedAt)
			sev, why := Warn, ""
			switch {
			case deref(c.ClaimStatus) == "unverified":
				why = "M-Pesa did not confirm it: check the statement and credit or dismiss the top-up"
				if age >= th.TopUpUnconfirmedPage {
					sev = Page
				}
			case age >= ReceiptClaimWait:
				why = "M-Pesa has not answered about it " + ages(age) + " later: check the statement"
			default:
				continue // M-Pesa is being asked; usually answered within a minute
			}
			out = append(out, Alert{Kind: KindTopUpReceiptClaim, Subject: c.ID, Severity: sev,
				Summary: "A player gave receipt " + deref(c.ClaimedReceipt) + " for a KES " + strconv.FormatInt(c.AmountRequestedKes, 10) +
					" top-up from " + maskMSISDN(c.Msisdn) + ": " + why,
				Details: map[string]any{"payment": c.ID, "receipt": deref(c.ClaimedReceipt), "claim_status": deref(c.ClaimStatus), "note": deref(c.ClaimNote),
					"amount_kes": c.AmountRequestedKes, "msisdn": maskMSISDN(c.Msisdn), "claimed_at": c.ClaimedAt, "age": ages(age)}})
		}
		orphans, err := q.OrphanCallbacksSince(ctx, now.Add(-Lookback))
		if err != nil {
			return nil, err
		}
		for _, o := range orphans {
			out = append(out, Alert{Kind: KindTopUpOrphanCallbacks, Subject: o.CheckoutRequestID, Severity: Warn,
				Summary: strconv.FormatInt(o.Callbacks, 10) + " STK callback(s) for checkout " + o.CheckoutRequestID + " matched no top-up: a push never recorded, or a forgery",
				Details: map[string]any{"checkout_request_id": o.CheckoutRequestID, "callbacks": o.Callbacks, "first_at": o.FirstAt, "last_at": o.LastAt}})
		}
		return out, nil
	}}
}

// PaybillCheck follows paybill money nobody has settled: suspense items (warn, then page; cleared when credited or
// refunded) and confirmations Daraja could not verify (warn; cleared when rechecked).
func PaybillCheck(pool *pgxpool.Pool, clk clock.Clock, th Thresholds) Check {
	return Check{Name: "paybill", Kinds: []string{KindSuspenseOpen, KindPaybillUnverified}, Run: func(ctx context.Context) ([]Alert, error) {
		now, err := clk.Now(ctx, pool)
		if err != nil {
			return nil, err
		}
		q := store.New(pool)
		var out []Alert
		suspense, err := q.OpenSuspense(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range suspense {
			age := now.Sub(s.CreatedAt)
			if age < th.SuspenseWarn {
				continue
			}
			sev := Warn
			if age >= th.SuspensePage {
				sev = Page
			}
			out = append(out, Alert{Kind: KindSuspenseOpen, Subject: s.MpesaReceipt, Severity: sev,
				Summary: "KES " + strconv.FormatInt(s.AmountKes, 10) + " paid to the paybill (receipt " + s.MpesaReceipt + ") has sat in suspense " + ages(age) +
					" (" + s.Reason + "): credit a player or refund it",
				Details: map[string]any{"receipt": s.MpesaReceipt, "amount_kes": s.AmountKes, "reason": s.Reason, "received_at": s.CreatedAt, "age": ages(age)}})
		}
		unverified, err := q.UnverifiedPaybill(ctx)
		if err != nil {
			return nil, err
		}
		for _, c := range unverified {
			out = append(out, Alert{Kind: KindPaybillUnverified, Subject: c.TransID, Severity: Warn,
				Summary: "Paybill payment " + c.TransID + " (KES " + strconv.FormatInt(c.AmountKes, 10) + ", account " + c.BillRef + ") could not be verified with Daraja: " +
					deref(c.OutcomeReason) + "; recheck it or settle it from the statement",
				Details: map[string]any{"trans_id": c.TransID, "amount_kes": c.AmountKes, "bill_ref": c.BillRef, "reason": deref(c.OutcomeReason), "received_at": c.CreatedAt}})
		}
		return out, nil
	}}
}

// PayoutsCheck follows payouts: one awaiting an operator (held or manual) warns, then pages, until decided; one that
// failed in the last day pages (the money is back, the player is still owed an answer).
func PayoutsCheck(pool *pgxpool.Pool, clk clock.Clock, th Thresholds) Check {
	return Check{Name: "payouts", Kinds: []string{KindPayoutAwaiting, KindPayoutFailed}, Run: func(ctx context.Context) ([]Alert, error) {
		now, err := clk.Now(ctx, pool)
		if err != nil {
			return nil, err
		}
		q := store.New(pool)
		var out []Alert
		waiting, err := q.PayoutsAwaitingApproval(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range waiting {
			age := now.Sub(p.UpdatedAt)
			if age < th.PayoutApprovalWarn {
				continue
			}
			sev := Warn
			if age >= th.PayoutApprovalPage {
				sev = Page
			}
			what := "approval"
			if p.Status == "manual" {
				what = "a decision from the M-Pesa statement"
			}
			out = append(out, Alert{Kind: KindPayoutAwaiting, Subject: p.ID, Severity: sev,
				Summary: "A " + p.Kind + " payout of KES " + strconv.FormatInt(p.NetKes, 10) + " has waited " + ages(age) + " for " + what,
				Details: map[string]any{"payout": p.ID, "kind": p.Kind, "status": p.Status, "net_kes": p.NetKes, "reason": deref(p.Reason),
					"waiting_since": p.UpdatedAt, "age": ages(age)}})
		}
		failed, err := q.FailedPayoutsSince(ctx, now.Add(-Lookback))
		if err != nil {
			return nil, err
		}
		for _, p := range failed {
			out = append(out, Alert{Kind: KindPayoutFailed, Subject: p.ID, Severity: Page,
				Summary: "A " + p.Kind + " payout of KES " + strconv.FormatInt(p.NetKes, 10) + " failed: " + deref(p.Reason),
				Details: map[string]any{"payout": p.ID, "kind": p.Kind, "net_kes": p.NetKes, "reason": deref(p.Reason), "failed_at": p.UpdatedAt}})
		}
		return out, nil
	}}
}

// B2CFloatCheck warns when the B2C float, as the ledger sees it, is below the configured floor (0: off).
func B2CFloatCheck(pool *pgxpool.Pool, th Thresholds) Check {
	return Check{Name: "b2c.float", Kinds: []string{KindB2CFloatLow}, Run: func(ctx context.Context) ([]Alert, error) {
		if th.B2CFloatFloorKES <= 0 {
			return nil, nil
		}
		balance, err := store.New(pool).B2CFloatBalance(ctx)
		if err != nil {
			return nil, err
		}
		if balance >= th.B2CFloatFloorKES {
			return nil, nil
		}
		return []Alert{{Kind: KindB2CFloatLow, Subject: "B2C_FLOAT", Severity: Warn,
			Summary: "The B2C float is KES " + strconv.FormatInt(balance, 10) + ", below the floor of KES " + strconv.FormatInt(th.B2CFloatFloorKES, 10) + ": top it up before payouts stall",
			Details: map[string]any{"balance_kes": balance, "floor_kes": th.B2CFloatFloorKES}}}, nil
	}}
}

// maskMSISDN keeps the country code and the last three digits (254705***005); the back office shows the number.
func maskMSISDN(m *string) string {
	if m == nil || len(*m) < 7 {
		return "unknown number"
	}
	s := *m
	return s[:6] + "***" + s[len(s)-3:]
}

// ages says a duration the way a person would: 45 min, 3 h, 2 d 4 h.
func ages(d time.Duration) string {
	d = d.Round(time.Minute)
	switch {
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + " min"
	case d < 48*time.Hour:
		h := int(d / time.Hour)
		m := int(d%time.Hour) / int(time.Minute)
		if m == 0 {
			return strconv.Itoa(h) + " h"
		}
		return strconv.Itoa(h) + " h " + strconv.Itoa(m) + " min"
	}
	days := int(d / (24 * time.Hour))
	h := int(d%(24*time.Hour)) / int(time.Hour)
	if h == 0 {
		return strconv.Itoa(days) + " d"
	}
	return strconv.Itoa(days) + " d " + strconv.Itoa(h) + " h"
}
