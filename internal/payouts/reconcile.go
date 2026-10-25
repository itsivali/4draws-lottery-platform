package payouts

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/payouts/store"
)

// Report is one payout reconciliation run. Mismatches break an invariant between payouts, their journals, the B2C float,
// the tax withheld and the KRA feed; they must never happen (a bug or tampering: investigate).
type Report struct {
	RanAt      time.Time           `json:"ran_at"`
	OK         bool                `json:"ok"`
	Mismatches map[string][]string `json:"mismatches"`
	Totals     map[string]int64    `json:"totals"`
}

// Reconcile checks the payout books, stores the run and returns it.
func (s *Service) Reconcile(ctx context.Context) (Report, error) {
	r := Report{Mismatches: map[string][]string{}, Totals: map[string]int64{}}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		// one consistent snapshot for every check
		if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY"); err != nil {
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		r.RanAt = now
		for name, check := range map[string]func(context.Context) ([]string, error){
			"payout_without_journal":   q.PayoutsWithoutJournal,
			"failure_journal_mismatch": q.FailureJournalMismatch,
			"journal_amount_mismatch":  q.JournalAmountMismatch,
			"prize_amount_mismatch":    q.PrizeAmountMismatch,
			"missing_kra_event":        q.MissingKRAEvents,
		} {
			ids, err := check(ctx)
			if err != nil {
				return err
			}
			if len(ids) > 0 {
				r.Mismatches[name] = ids
			}
		}
		live, err := q.LivePayoutTotals(ctx)
		if err != nil {
			return err
		}
		float, err := q.AccountBalance(ctx, "B2C_FLOAT")
		if err != nil {
			return err
		}
		tax, err := q.AccountBalance(ctx, "TAX_PAYABLE")
		if err != nil {
			return err
		}
		r.Totals["payouts_live"], r.Totals["net_kes"], r.Totals["tax_withheld_kes"] = live.Payouts, live.NetKes, live.TaxKes
		r.Totals["b2c_float_credit_kes"], r.Totals["tax_payable_kes"] = float, tax
		// every payout not failed moved its net out through the float and its tax to TAX_PAYABLE (payouts are the only
		// postings to either account)
		if float != live.NetKes {
			r.Mismatches["b2c_float"] = []string{"B2C_FLOAT " + strconv.FormatInt(float, 10) + " != net of live payouts " + strconv.FormatInt(live.NetKes, 10)}
		}
		if tax != live.TaxKes {
			r.Mismatches["tax_payable"] = []string{"TAX_PAYABLE " + strconv.FormatInt(tax, 10) + " != tax of live payouts " + strconv.FormatInt(live.TaxKes, 10)}
		}
		r.OK = len(r.Mismatches) == 0
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return Report{}, err
	}
	if err := store.New(s.pool).InsertReconciliationRun(ctx, store.InsertReconciliationRunParams{RanAt: r.RanAt, Ok: r.OK, Report: raw}); err != nil {
		return Report{}, err
	}
	if !r.OK {
		slog.ErrorContext(ctx, "payout reconciliation found mismatches", "mismatches", r.Mismatches, "totals", r.Totals)
	} else {
		slog.InfoContext(ctx, "payout reconciliation clean", "totals", r.Totals)
	}
	return r, nil
}
