package finance_test

import (
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/finance"
	"github.com/itsivali/lottery-platform/internal/finance/store"
)

func day(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }

func TestSummarize_ReadsTheLedgerIntoTheFiguresStaffAskFor(t *testing.T) {
	flows := []store.FlowsRow{
		// 1 Nov: two deposits of 500, plays of 300 (one of 100 refunded), excise the operator owes
		{Day: day("2026-11-01"), Kind: "deposit", Account: "MPESA_CLEARING", Direction: "D", AmountKes: 1000},
		{Day: day("2026-11-01"), Kind: "deposit", Account: "WALLET_LIABILITY", Direction: "C", AmountKes: 1000},
		{Day: day("2026-11-01"), Kind: "deposit", Account: "EXCISE_PAYABLE", Direction: "C", AmountKes: 50},
		{Day: day("2026-11-01"), Kind: "stake", Account: "STAKES_REVENUE", Direction: "C", AmountKes: 300},
		{Day: day("2026-11-01"), Kind: "stake_refund", Account: "STAKES_REVENUE", Direction: "D", AmountKes: 100},
		// 2 Nov: the draw's pool is set aside, a prize of 90 paid with 18 withheld, a failed one returned, a withdrawal
		{Day: day("2026-11-02"), Kind: "prize_pool_funding", Account: "STAKES_REVENUE", Direction: "D", AmountKes: 60},
		{Day: day("2026-11-02"), Kind: "prize_pool_funding", Account: "PRIZE_POOL_DAILY", Direction: "C", AmountKes: 60},
		{Day: day("2026-11-02"), Kind: "prize_payout", Account: "PRIZE_POOL_DAILY", Direction: "D", AmountKes: 90},
		{Day: day("2026-11-02"), Kind: "prize_payout", Account: "TAX_PAYABLE", Direction: "C", AmountKes: 18},
		{Day: day("2026-11-02"), Kind: "prize_payout_failed", Account: "PRIZE_POOL_DAILY", Direction: "C", AmountKes: 30},
		{Day: day("2026-11-02"), Kind: "prize_payout_failed", Account: "TAX_PAYABLE", Direction: "D", AmountKes: 6},
		{Day: day("2026-11-02"), Kind: "withdrawal", Account: "WALLET_LIABILITY", Direction: "D", AmountKes: 400},
		{Day: day("2026-11-02"), Kind: "deposit_reversal", Account: "MPESA_CLEARING", Direction: "C", AmountKes: 100},
		{Day: day("2026-11-02"), Kind: "tax_remittance", Account: "TAX_PAYABLE", Direction: "D", AmountKes: 12},
	}
	balances := []store.BalancesRow{
		{Code: "WALLET_LIABILITY", Type: "liability", DebitBalance: -600},
		{Code: "PLAYER_DEBT", Type: "asset", DebitBalance: 40},
		{Code: "EXCISE_PAYABLE", Type: "liability", DebitBalance: -50},
		{Code: "TAX_PAYABLE", Type: "liability", DebitBalance: 0},
		{Code: "PRIZE_POOL_DAILY", Type: "liability", DebitBalance: -10},
		{Code: "PRIZE_POOL_WEEKLY", Type: "liability", DebitBalance: -5},
		{Code: "SUSPENSE", Type: "liability", DebitBalance: -7},
		{Code: "MPESA_CLEARING", Type: "asset", DebitBalance: 900},
		{Code: "B2C_FLOAT", Type: "asset", DebitBalance: -400},
		{Code: "EXCISE_EXPENSE", Type: "expense", DebitBalance: 50},
	}
	s := finance.Summarize(day("2026-11-01"), day("2026-11-03"), flows, balances, store.ActivityRow{Signups: 3, Plays: 3, Players: 2})

	want := finance.Figures{DepositsKES: 1000, StakesKES: 200, PrizesAwardedKES: 60, PrizesPaidKES: 60, WithdrawalsKES: 400,
		ReversalsKES: 100, ExciseKES: 50, TaxWithheldKES: 12, TaxRemittedKES: 12, GGRKES: 140}
	if s.Totals != want {
		t.Fatalf("totals\n got %+v\nwant %+v", s.Totals, want)
	}
	if len(s.Days) != 3 || s.Days[0].Day != "2026-11-01" || s.Days[0].DepositsKES != 1000 || s.Days[1].PrizesPaidKES != 60 ||
		s.Days[2].Day != "2026-11-03" || s.Days[2].DepositsKES != 0 {
		t.Fatalf("days %+v", s.Days)
	}
	b := s.Balances
	if b.OwedToPlayersKES != 600 || b.PlayerDebtKES != 40 || b.ExciseOwedKES != 50 || b.TaxOwedKES != 0 || b.PrizePoolsKES != 15 ||
		b.SuspenseKES != 7 || b.MpesaClearingKES != 900 || b.B2CFloatKES != -400 {
		t.Fatalf("balances %+v", b)
	}
	if s.Activity.Signups != 3 || s.Activity.Plays != 3 || s.Activity.Players != 2 || s.From != "2026-11-01" || s.To != "2026-11-03" {
		t.Fatalf("summary %+v", s)
	}
}
