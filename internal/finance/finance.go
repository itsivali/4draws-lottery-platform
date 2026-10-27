// Package finance reads the ledger into the figures the back office's financial dashboard shows: money in and out per
// day, what the operator earned, what is owed to players and to KRA. It only reads; every figure comes from ledger
// postings, so it agrees with the reconciliations.
package finance

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/finance/store"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
)

// MaxDays is the longest range one summary covers.
const MaxDays = 366

// ErrRange is a range that is reversed or longer than MaxDays.
var ErrRange = errors.New("finance: the range must run forward and cover at most 366 days")

// Figures are the money movements of a day or a range, in whole KES.
type Figures struct {
	DepositsKES      int64 `json:"deposits_kes"`       // top-ups confirmed by M-Pesa (gross)
	StakesKES        int64 `json:"stakes_kes"`         // plays, less refunded ones
	PrizesAwardedKES int64 `json:"prizes_awarded_kes"` // prize pools set aside when draws are approved
	PrizesPaidKES    int64 `json:"prizes_paid_kes"`    // prizes sent to winners (gross of tax), less failed payouts
	WithdrawalsKES   int64 `json:"withdrawals_kes"`    // wallet withdrawals, less failed ones
	ReversalsKES     int64 `json:"reversals_kes"`      // top-ups reversed by Safaricom
	ExciseKES        int64 `json:"excise_kes"`         // deposit excise the operator owes (ADR 0016)
	TaxWithheldKES   int64 `json:"tax_withheld_kes"`   // withholding tax taken from prizes
	TaxRemittedKES   int64 `json:"tax_remitted_kes"`   // withholding tax paid to KRA
	GGRKES           int64 `json:"ggr_kes"`            // gross gaming revenue: stakes less prizes awarded
}

// Day is one Nairobi day's figures.
type Day struct {
	Day string `json:"day"`
	Figures
}

// Balances are what the ledger holds now, each with its natural sign (owed amounts positive).
type Balances struct {
	OwedToPlayersKES int64 `json:"owed_to_players_kes"` // wallet balances
	PlayerDebtKES    int64 `json:"player_debt_kes"`     // owed by players after reversed top-ups
	ExciseOwedKES    int64 `json:"excise_owed_kes"`
	TaxOwedKES       int64 `json:"tax_owed_kes"` // withheld and not yet remitted
	PrizePoolsKES    int64 `json:"prize_pools_kes"`
	SuspenseKES      int64 `json:"suspense_kes"` // paybill money not yet matched to a player
	MpesaClearingKES int64 `json:"mpesa_clearing_kes"`
	B2CFloatKES      int64 `json:"b2c_float_kes"`
}

// Activity counts players and plays in the range.
type Activity struct {
	Signups int64 `json:"signups"`
	Plays   int64 `json:"plays"`
	Players int64 `json:"players"`
}

// Summary is the dashboard: the range's totals and days, the balances now, and the activity.
type Summary struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Totals   Figures  `json:"totals"`
	Days     []Day    `json:"days"`
	Balances Balances `json:"balances"`
	Activity Activity `json:"activity"`
}

// DefaultDays is the range shown when none is asked for: the last 30 days, today included.
const DefaultDays = 30

// Service reads the summary.
type Service struct {
	pool *pgxpool.Pool
	clk  clock.Clock
}

// NewService returns a Service.
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service { return &Service{pool: pool, clk: clk} }

// Summary covers the Nairobi days from to to, inclusive. A zero to is today; a zero from is DefaultDays before to.
func (s *Service) Summary(ctx context.Context, from, to time.Time) (Summary, error) {
	if to.IsZero() {
		now, err := s.clk.Now(ctx, s.pool)
		if err != nil {
			return Summary{}, err
		}
		y, m, d := clock.InNairobi(now).Date()
		to = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	if from.IsZero() {
		from = to.AddDate(0, 0, 1-DefaultDays)
	}
	if to.Before(from) || to.Sub(from) >= MaxDays*24*time.Hour {
		return Summary{}, ErrRange
	}
	fromAt := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, clock.Nairobi)
	toAt := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, clock.Nairobi).AddDate(0, 0, 1)
	q := store.New(s.pool)
	flows, err := q.Flows(ctx, store.FlowsParams{FromAt: fromAt, ToAt: toAt})
	if err != nil {
		return Summary{}, err
	}
	balances, err := q.Balances(ctx)
	if err != nil {
		return Summary{}, err
	}
	act, err := q.Activity(ctx, store.ActivityParams{FromAt: fromAt, ToAt: toAt, FromDay: from, ToDay: to})
	if err != nil {
		return Summary{}, err
	}
	return Summarize(from, to, flows, balances, act), nil
}

// Summarize turns ledger flows and balances into the dashboard's figures. Every day of the range appears, with zeros
// when nothing moved.
func Summarize(from, to time.Time, flows []store.FlowsRow, balances []store.BalancesRow, act store.ActivityRow) Summary {
	byDay := map[string]*Figures{}
	var days []Day
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		days = append(days, Day{Day: d.Format(time.DateOnly)})
	}
	for i := range days {
		byDay[days[i].Day] = &days[i].Figures
	}
	var total Figures
	for _, f := range flows {
		fig, ok := byDay[f.Day.Format(time.DateOnly)]
		if !ok {
			continue
		}
		for _, into := range []*Figures{fig, &total} {
			apply(into, f)
		}
	}
	for i := range days {
		days[i].GGRKES = days[i].StakesKES - days[i].PrizesAwardedKES
	}
	total.GGRKES = total.StakesKES - total.PrizesAwardedKES
	return Summary{From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Totals: total, Days: days,
		Balances: balancesOf(balances), Activity: Activity{Signups: act.Signups, Plays: act.Plays, Players: act.Players}}
}

// apply adds one flow to the figures it belongs to.
func apply(f *Figures, r store.FlowsRow) {
	amt := r.AmountKes
	debit, credit := r.Direction == "D", r.Direction == "C"
	pool := strings.HasPrefix(r.Account, "PRIZE_POOL_")
	switch {
	case r.Kind == "deposit" && r.Account == "MPESA_CLEARING" && debit:
		f.DepositsKES += amt
	case r.Kind == "stake" && r.Account == "STAKES_REVENUE" && credit:
		f.StakesKES += amt
	case r.Kind == "stake_refund" && r.Account == "STAKES_REVENUE" && debit:
		f.StakesKES -= amt
	case r.Kind == "prize_pool_funding" && pool && credit:
		f.PrizesAwardedKES += amt
	case r.Kind == "prize_payout" && pool && debit:
		f.PrizesPaidKES += amt
	case r.Kind == "prize_payout_failed" && pool && credit:
		f.PrizesPaidKES -= amt
	case r.Kind == "withdrawal" && r.Account == "WALLET_LIABILITY" && debit:
		f.WithdrawalsKES += amt
	case r.Kind == "withdrawal_failed" && r.Account == "WALLET_LIABILITY" && credit:
		f.WithdrawalsKES -= amt
	case r.Kind == "deposit_reversal" && r.Account == "MPESA_CLEARING" && credit:
		f.ReversalsKES += amt
	case r.Account == "EXCISE_PAYABLE":
		f.ExciseKES += signed(amt, credit)
	case r.Kind == "tax_remittance" && r.Account == "TAX_PAYABLE" && debit:
		f.TaxRemittedKES += amt
	case r.Account == "TAX_PAYABLE":
		f.TaxWithheldKES += signed(amt, credit)
	}
}

func signed(amt int64, positive bool) int64 {
	if positive {
		return amt
	}
	return -amt
}

func balancesOf(rows []store.BalancesRow) Balances {
	var b Balances
	for _, r := range rows {
		natural := r.DebitBalance // assets and expenses
		if r.Type == "liability" || r.Type == "revenue" {
			natural = -r.DebitBalance
		}
		switch {
		case r.Code == "WALLET_LIABILITY":
			b.OwedToPlayersKES = natural
		case r.Code == "PLAYER_DEBT":
			b.PlayerDebtKES = natural
		case r.Code == "EXCISE_PAYABLE":
			b.ExciseOwedKES = natural
		case r.Code == "TAX_PAYABLE":
			b.TaxOwedKES = natural
		case strings.HasPrefix(r.Code, "PRIZE_POOL_"):
			b.PrizePoolsKES += natural
		case r.Code == "SUSPENSE":
			b.SuspenseKES = natural
		case r.Code == "MPESA_CLEARING":
			b.MpesaClearingKES = natural
		case r.Code == "B2C_FLOAT":
			b.B2CFloatKES = natural
		}
	}
	return b
}
