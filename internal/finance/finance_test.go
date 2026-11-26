package finance_test

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/finance"
	"github.com/itsivali/4draws-lottery-platform/internal/finance/store"
)

func day(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }

func inputs() finance.Inputs {
	return finance.Inputs{
		Now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC),
		Flows: []store.FlowsRow{
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
		},
		Balances: []store.BalancesRow{
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
		},
		Activity: store.ActivityRow{Signups: 3, Plays: 3, Players: 2},
		Plays:    []store.PlaysByPeriodRow{{Bucket: day("2026-11-01"), Plays: 2, Players: 2}, {Bucket: day("2026-11-02"), Plays: 1, Players: 1}},
		Signups:  []store.SignupsByDayRow{{Day: day("2026-11-01"), Signups: 2}, {Day: day("2026-11-03"), Signups: 1}},
		Kinds:    []store.JournalKindsRow{{Kind: "deposit", Journals: 2, DebitKes: 1050, CreditKes: 1050}, {Kind: "stake", Journals: 3, DebitKes: 300, CreditKes: 300}},
		Accounts: []store.TrialBalanceRow{
			{Code: "MPESA_CLEARING", Name: "M-Pesa paybill clearing", Type: "asset", DebitsKes: 1000, CreditsKes: 100},
			{Code: "STAKES_REVENUE", Name: "Stakes revenue", Type: "revenue", DebitsKes: 160, CreditsKes: 300},
			{Code: "WALLET_LIABILITY", Name: "Player wallet liability", Type: "liability", DebitsKes: 400, CreditsKes: 1160},
		},
		TopUps: []store.TopUpFunnelRow{{Bucket: day("2026-11-01"), Started: 3, StartedKes: 1500, Credited: 2, CreditedKes: 1000, Cancelled: 1, CancelledKes: 500, MedianSecondsToCredit: 42},
			{Bucket: day("2026-11-02"), Started: 1, StartedKes: 200, PendingNow: 1, PendingNowKes: 200, MedianSecondsToCredit: -1}},
		Payouts: []store.PayoutFunnelRow{{Bucket: day("2026-11-02"), Requested: 2, RequestedKes: 490, Paid: 1, PaidKes: 72, Failed: 1, FailedKes: 400, TaxWithheldKes: 18}},
		Tax:     store.TaxPositionRow{AwaitingBatches: 1, AwaitingKes: 12, UnbatchedEvents: 1, UnbatchedKes: 18, OldestUnremittedAt: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)},
	}
}

func TestSummarize_ReadsTheLedgerIntoTheFiguresStaffAskFor(t *testing.T) {
	s := finance.Summarize(day("2026-11-01"), day("2026-11-03"), finance.ByDay, inputs())

	want := finance.Figures{DepositsKES: 1000, StakesKES: 200, PrizesAwardedKES: 60, PrizesPaidKES: 60, WithdrawalsKES: 400,
		ReversalsKES: 100, ExciseKES: 50, TaxWithheldKES: 12, TaxRemittedKES: 12, GGRKES: 140}
	if s.Totals != want {
		t.Fatalf("totals\n got %+v\nwant %+v", s.Totals, want)
	}
	if s.Group != finance.ByDay || len(s.Periods) != 3 || s.Periods[0].Start != "2026-11-01" || s.Periods[0].End != "2026-11-01" ||
		s.Periods[0].DepositsKES != 1000 || s.Periods[0].Plays != 2 || s.Periods[0].Signups != 2 || s.Periods[1].PrizesPaidKES != 60 ||
		s.Periods[2].Start != "2026-11-03" || s.Periods[2].DepositsKES != 0 || s.Periods[2].Signups != 1 {
		t.Fatalf("periods %+v", s.Periods)
	}
	b := s.Balances
	if b.OwedToPlayersKES != 600 || b.PlayerDebtKES != 40 || b.ExciseOwedKES != 50 || b.TaxOwedKES != 0 || b.PrizePoolsKES != 15 ||
		b.SuspenseKES != 7 || b.MpesaClearingKES != 900 || b.B2CFloatKES != -400 {
		t.Fatalf("balances %+v", b)
	}
	if s.Activity.Signups != 3 || s.Activity.Plays != 3 || s.Activity.Players != 2 || s.From != "2026-11-01" || s.To != "2026-11-03" {
		t.Fatalf("summary %+v", s)
	}
	if len(s.ByKind) != 2 || s.ByKind[0] != (finance.KindTotals{Kind: "deposit", Journals: 2, DebitKES: 1050, CreditKES: 1050}) {
		t.Fatalf("by kind %+v", s.ByKind)
	}
	// prize pools: the daily one took 60 in and paid 90 less 30 returned; the weekly one only has a balance
	if len(s.PrizePools) != 2 || s.PrizePools[0] != (finance.PrizePool{Type: "daily", BalanceKES: 10, FundedKES: 60, PaidKES: 60}) ||
		s.PrizePools[1] != (finance.PrizePool{Type: "weekly", BalanceKES: 5}) {
		t.Fatalf("prize pools %+v", s.PrizePools)
	}
	// top-ups: every period appears; the median is null where nothing was credited
	if len(s.TopUps) != 3 || s.TopUps[0].Started != (finance.Count{Count: 3, KES: 1500}) || s.TopUps[0].Credited.KES != 1000 ||
		s.TopUps[0].MedianSecondsToCredit == nil || *s.TopUps[0].MedianSecondsToCredit != 42 ||
		s.TopUps[1].PendingNow.Count != 1 || s.TopUps[1].MedianSecondsToCredit != nil || s.TopUps[2].Start != "2026-11-03" {
		t.Fatalf("top-ups %+v", s.TopUps)
	}
	if len(s.Payouts) != 3 || s.Payouts[1].Paid != (finance.Count{Count: 1, KES: 72}) || s.Payouts[1].TaxWithheldKES != 18 || s.Payouts[0].Requested.Count != 0 {
		t.Fatalf("payouts %+v", s.Payouts)
	}
	tax := s.Tax
	if tax.WithheldKES != 12 || tax.RemittedKES != 12 || tax.AwaitingRemittance.Batches != 1 || tax.AwaitingRemittance.KES != 12 ||
		tax.Unbatched.Events != 1 || tax.Unbatched.KES != 18 || tax.OldestUnremittedAgeSeconds == nil || *tax.OldestUnremittedAgeSeconds != 24*3600 {
		t.Fatalf("tax %+v", tax)
	}
}

func TestSummarize_TrialBalanceCarriesNaturalSignsAndSaysWhetherItBalances(t *testing.T) {
	in := inputs()
	s := finance.Summarize(day("2026-11-01"), day("2026-11-03"), finance.ByDay, in)
	tb := s.TrialBalance
	if tb.AsAt != "2026-11-03" || len(tb.Accounts) != 3 || tb.DebitsKES != 1560 || tb.CreditsKES != 1560 || !tb.Balanced {
		t.Fatalf("trial balance %+v", tb)
	}
	if tb.Accounts[0].BalanceKES != 900 || tb.Accounts[1].BalanceKES != 140 || tb.Accounts[2].BalanceKES != 760 {
		t.Fatalf("natural signs %+v", tb.Accounts)
	}
	in.Accounts = append(in.Accounts, store.TrialBalanceRow{Code: "ODD", Name: "A broken posting", Type: "asset", DebitsKes: 1})
	if tb := finance.Summarize(day("2026-11-01"), day("2026-11-03"), finance.ByDay, in).TrialBalance; tb.Balanced || tb.DebitsKES != 1561 {
		t.Fatalf("an unbalanced ledger must say so: %+v", tb)
	}
}

func TestPeriods_WeeksStartOnMondayAndMonthsAreCalendarMonths_ClippedToTheRange(t *testing.T) {
	// 1 Nov 2026 is a Sunday: the first week is one day, the next full, the last clipped at the 17th (a Tuesday)
	weeks, keys := finance.Periods(day("2026-11-01"), day("2026-11-17"), finance.ByWeek)
	if len(weeks) != 4 || weeks[0].Start != "2026-11-01" || weeks[0].End != "2026-11-01" || keys[0] != "2026-10-26" ||
		weeks[1].Start != "2026-11-02" || weeks[1].End != "2026-11-08" || weeks[3].Start != "2026-11-16" || weeks[3].End != "2026-11-17" {
		t.Fatalf("weeks %+v keys %v", weeks, keys)
	}
	months, keys := finance.Periods(day("2026-10-15"), day("2026-12-03"), finance.ByMonth)
	if len(months) != 3 || months[0].Start != "2026-10-15" || months[0].End != "2026-10-31" || keys[0] != "2026-10-01" ||
		months[1].Start != "2026-11-01" || months[1].End != "2026-11-30" || months[2].Start != "2026-12-01" || months[2].End != "2026-12-03" {
		t.Fatalf("months %+v keys %v", months, keys)
	}
	days, _ := finance.Periods(day("2026-11-01"), day("2026-11-01"), finance.ByDay)
	if len(days) != 1 || days[0].Start != "2026-11-01" || days[0].End != "2026-11-01" {
		t.Fatalf("one day %+v", days)
	}
}

func TestSummarize_GroupsFlowsAndActivityIntoTheirWeeks(t *testing.T) {
	in := inputs()
	// the database buckets plays by the Monday of their week, as date_trunc does
	in.Plays = []store.PlaysByPeriodRow{{Bucket: day("2026-10-26"), Plays: 2, Players: 2}, {Bucket: day("2026-11-02"), Plays: 1, Players: 1}}
	in.TopUps[0].Bucket, in.TopUps[1].Bucket = day("2026-10-26"), day("2026-11-02")
	in.Payouts[0].Bucket = day("2026-11-02")
	s := finance.Summarize(day("2026-11-01"), day("2026-11-04"), finance.ByWeek, in)
	if len(s.Periods) != 2 || s.Periods[0].Start != "2026-11-01" || s.Periods[0].End != "2026-11-01" || s.Periods[1].Start != "2026-11-02" || s.Periods[1].End != "2026-11-04" {
		t.Fatalf("periods %+v", s.Periods)
	}
	sun, week := s.Periods[0], s.Periods[1]
	if sun.DepositsKES != 1000 || sun.Plays != 2 || sun.Signups != 2 || week.WithdrawalsKES != 400 || week.Plays != 1 || week.Signups != 1 || week.GGRKES != -60 {
		t.Fatalf("sunday %+v, week %+v", sun, week)
	}
	if s.TopUps[0].Started.Count != 3 || s.TopUps[1].PendingNow.Count != 1 || s.Payouts[1].Paid.Count != 1 {
		t.Fatalf("funnels %+v %+v", s.TopUps, s.Payouts)
	}
}

func TestWriteSummaryCSV_PeriodsThenTotalsThenTheTrialBalance(t *testing.T) {
	s := finance.Summarize(day("2026-11-01"), day("2026-11-03"), finance.ByDay, inputs())
	var buf bytes.Buffer
	rows, err := finance.WriteSummaryCSV(&buf, s)
	if err != nil || rows != 3 {
		t.Fatalf("rows %d err %v", rows, err)
	}
	text := buf.String()
	sections := strings.Split(strings.TrimRight(text, "\n"), "\n\n")
	if len(sections) != 4 {
		t.Fatalf("want periods, totals, trial balance and its check, separated by blank lines:\n%s", text)
	}
	periods, err := csv.NewReader(strings.NewReader(sections[0])).ReadAll()
	if err != nil || len(periods) != 4 || periods[0][0] != "period_start" || periods[0][14] != "signups" || periods[1][2] != "1000" || periods[1][12] != "2" {
		t.Fatalf("periods section %v %v", periods, err)
	}
	if !strings.HasPrefix(sections[1], "total_from,total_to,deposits_kes") || !strings.Contains(sections[1], "\n2026-11-01,2026-11-03,1000,200,") {
		t.Fatalf("totals section:\n%s", sections[1])
	}
	if !strings.HasPrefix(sections[2], "trial_balance_as_at,2026-11-03\naccount_code,account_name,type,debits_kes,credits_kes,balance_kes\nMPESA_CLEARING,M-Pesa paybill clearing,asset,1000,100,900") {
		t.Fatalf("trial balance section:\n%s", sections[2])
	}
	if sections[3] != "total_debits_kes,total_credits_kes,balanced\n1560,1560,true" {
		t.Fatalf("check section:\n%s", sections[3])
	}
}
