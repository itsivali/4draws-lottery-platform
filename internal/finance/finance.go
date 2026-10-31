// Package finance reads the ledger into the figures the back office's financial dashboard shows: money in and out per
// day, week or month, what the operator earned, what is owed to players and to KRA, every journal kind posted, the
// trial balance, the top-up and payout funnels, and the tax position. It only reads; every money figure comes from
// ledger postings, so it agrees with the reconciliations. Its two exports (the figures and the raw ledger) are the
// auditor's copies, and each one is recorded in the audit trail (ADR 0023).
package finance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/finance/store"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
)

// MaxDays is the longest range one summary covers.
const MaxDays = 366

// DefaultLedgerRowCap is how many ledger entries one export may hold before the operator is asked to narrow the range.
const DefaultLedgerRowCap = 100_000

// Errors the API maps to problems.
var (
	// ErrRange is a range that is reversed or longer than MaxDays.
	ErrRange = errors.New("finance: the range must run forward and cover at most 366 days")
	// ErrGroup is a grouping other than day, week or month.
	ErrGroup = errors.New("finance: group must be day, week or month")
	// ErrTooManyRows is a ledger export of more entries than the cap.
	ErrTooManyRows = errors.New("finance: too many ledger entries for one export; narrow the range")
)

// Group is how a range is cut into periods.
type Group string

// Groupings. Weeks start on Monday and months are calendar months, both in Nairobi time; a first or last period that
// the range cuts into is clipped to the range.
const (
	ByDay   Group = "day"
	ByWeek  Group = "week"
	ByMonth Group = "month"
)

// Figures are the money movements of a period or a range, in whole KES.
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

// Period is one day, week or month of the range (Start and End inclusive, Nairobi dates) with its figures and activity.
type Period struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Figures
	Plays   int64 `json:"plays"`
	Players int64 `json:"players"` // distinct players who played in the period
	Signups int64 `json:"signups"`
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

// KindTotals is one journal kind posted in the range: how many journals, and what they debited and credited in all.
type KindTotals struct {
	Kind      string `json:"kind"`
	Journals  int64  `json:"journals"`
	DebitKES  int64  `json:"debit_kes"`
	CreditKES int64  `json:"credit_kes"`
}

// Account is one ledger account's line of the trial balance. Balance carries the account's natural sign: debits less
// credits for assets and expenses, credits less debits for liabilities and revenue.
type Account struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	DebitsKES  int64  `json:"debits_kes"`
	CreditsKES int64  `json:"credits_kes"`
	BalanceKES int64  `json:"balance_kes"`
}

// TrialBalance is every account as at the end of the range's last day. Balanced says the ledger's debits equal its
// credits, which double entry guarantees; false means a posting is broken and the figures cannot be trusted.
type TrialBalance struct {
	AsAt       string    `json:"as_at"`
	Accounts   []Account `json:"accounts"`
	DebitsKES  int64     `json:"debits_kes"`
	CreditsKES int64     `json:"credits_kes"`
	Balanced   bool      `json:"balanced"`
}

// Count is how many of something and how much money they carry.
type Count struct {
	Count int64 `json:"count"`
	KES   int64 `json:"kes"`
}

// TopUpPeriod is the top-ups started in one period, by how they ended (requested amounts, except credited and
// reversed, which carry what M-Pesa confirmed). Reversed top-ups count among the credited ones too.
type TopUpPeriod struct {
	Start                 string `json:"start"`
	End                   string `json:"end"`
	Started               Count  `json:"started"`
	Credited              Count  `json:"credited"`
	Failed                Count  `json:"failed"`
	Cancelled             Count  `json:"cancelled"`
	Expired               Count  `json:"expired"`
	Unconfirmed           Count  `json:"unconfirmed"`
	Reversed              Count  `json:"reversed"`
	PendingNow            Count  `json:"pending_now"`              // still waiting for M-Pesa
	MedianSecondsToCredit *int64 `json:"median_seconds_to_credit"` // null when nothing was credited
}

// PayoutPeriod is the payouts requested in one period, by where they stand now. Requested and failed carry gross
// amounts; paid, held and manual the net the player gets.
type PayoutPeriod struct {
	Start          string `json:"start"`
	End            string `json:"end"`
	Requested      Count  `json:"requested"`
	Paid           Count  `json:"paid"`
	Failed         Count  `json:"failed"`
	Held           Count  `json:"held"`   // waiting for a staff approval
	Manual         Count  `json:"manual"` // waiting for manual settlement
	TaxWithheldKES int64  `json:"tax_withheld_kes"`
}

// PrizePool is one prize pool: what it holds now, and what went in and out in the range.
type PrizePool struct {
	Type       string `json:"type"`
	BalanceKES int64  `json:"balance_kes"`
	FundedKES  int64  `json:"funded_kes"`
	PaidKES    int64  `json:"paid_kes"`
}

// Tax is the withholding tax position: the range's withholding and remittances, and what is still owed to KRA now.
type Tax struct {
	WithheldKES        int64 `json:"withheld_kes"`
	RemittedKES        int64 `json:"remitted_kes"`
	AwaitingRemittance struct {
		Batches int64 `json:"batches"`
		KES     int64 `json:"kes"`
	} `json:"awaiting_remittance"` // batches prepared and not yet remitted
	Unbatched struct {
		Events int64 `json:"events"`
		KES    int64 `json:"kes"`
	} `json:"unbatched"` // withholding events not yet in a batch
	OldestUnremittedAgeSeconds *int64 `json:"oldest_unremitted_age_seconds"` // null when everything is remitted
}

// Summary is the dashboard: the range's totals and periods, the balances now, the activity, and the audit sections.
type Summary struct {
	From         string         `json:"from"`
	To           string         `json:"to"`
	Group        Group          `json:"group"`
	Totals       Figures        `json:"totals"`
	Periods      []Period       `json:"periods"`
	Balances     Balances       `json:"balances"`
	Activity     Activity       `json:"activity"`
	ByKind       []KindTotals   `json:"by_kind"`
	TrialBalance TrialBalance   `json:"trial_balance"`
	TopUps       []TopUpPeriod  `json:"top_ups"`
	Payouts      []PayoutPeriod `json:"payouts"`
	PrizePools   []PrizePool    `json:"prize_pools"`
	Tax          Tax            `json:"tax"`
}

// DefaultDays is the range shown when none is asked for: the last 30 days, today included.
const DefaultDays = 30

// Service reads the summary and writes the exports.
type Service struct {
	pool   *pgxpool.Pool
	clk    clock.Clock
	audit  *audit.Writer
	rowCap int64
}

// NewService returns a Service. Exports are recorded with aw.
func NewService(pool *pgxpool.Pool, clk clock.Clock, aw *audit.Writer) *Service {
	return &Service{pool: pool, clk: clk, audit: aw, rowCap: DefaultLedgerRowCap}
}

// WithLedgerRowCap changes how many entries one ledger export may hold (tests).
func (s *Service) WithLedgerRowCap(n int64) *Service { s.rowCap = n; return s }

// Range is a resolved range: Nairobi dates from..to inclusive, and the instants they span.
type Range struct {
	From, To     time.Time // dates (midnight UTC, date part only)
	FromAt, ToAt time.Time // [FromAt, ToAt) in Nairobi time
}

// resolve fills in defaults (a zero to is today; a zero from is DefaultDays before to) and checks the range.
func (s *Service) resolve(ctx context.Context, from, to time.Time) (Range, time.Time, error) {
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return Range{}, now, err
	}
	if to.IsZero() {
		y, m, d := clock.InNairobi(now).Date()
		to = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	if from.IsZero() {
		from = to.AddDate(0, 0, 1-DefaultDays)
	}
	from, to = dateOf(from), dateOf(to)
	if to.Before(from) || to.Sub(from) >= MaxDays*24*time.Hour {
		return Range{}, now, ErrRange
	}
	return Range{From: from, To: to,
		FromAt: time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, clock.Nairobi),
		ToAt:   time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, clock.Nairobi).AddDate(0, 0, 1)}, now, nil
}

// Summary covers the Nairobi days from to to, inclusive, cut by group (an empty group is by day).
func (s *Service) Summary(ctx context.Context, from, to time.Time, group Group) (Summary, error) {
	if group == "" {
		group = ByDay
	}
	if group != ByDay && group != ByWeek && group != ByMonth {
		return Summary{}, ErrGroup
	}
	rng, now, err := s.resolve(ctx, from, to)
	if err != nil {
		return Summary{}, err
	}
	in, err := s.read(ctx, rng, group)
	if err != nil {
		return Summary{}, err
	}
	in.Now = now
	return Summarize(rng.From, rng.To, group, in), nil
}

// Inputs is everything one summary is built from.
type Inputs struct {
	Now      time.Time
	Flows    []store.FlowsRow
	Balances []store.BalancesRow
	Activity store.ActivityRow
	Plays    []store.PlaysByPeriodRow
	Signups  []store.SignupsByDayRow
	Kinds    []store.JournalKindsRow
	Accounts []store.TrialBalanceRow
	TopUps   []store.TopUpFunnelRow
	Payouts  []store.PayoutFunnelRow
	Tax      store.TaxPositionRow
}

func (s *Service) read(ctx context.Context, r Range, group Group) (Inputs, error) {
	q := store.New(s.pool)
	var in Inputs
	var err error
	unit := string(group)
	if in.Flows, err = q.Flows(ctx, store.FlowsParams{FromAt: r.FromAt, ToAt: r.ToAt}); err != nil {
		return in, err
	}
	if in.Balances, err = q.Balances(ctx); err != nil {
		return in, err
	}
	if in.Activity, err = q.Activity(ctx, store.ActivityParams{FromAt: r.FromAt, ToAt: r.ToAt, FromDay: r.From, ToDay: r.To}); err != nil {
		return in, err
	}
	if in.Plays, err = q.PlaysByPeriod(ctx, store.PlaysByPeriodParams{Unit: unit, FromDay: r.From, ToDay: r.To}); err != nil {
		return in, err
	}
	if in.Signups, err = q.SignupsByDay(ctx, store.SignupsByDayParams{FromAt: r.FromAt, ToAt: r.ToAt}); err != nil {
		return in, err
	}
	if in.Kinds, err = q.JournalKinds(ctx, store.JournalKindsParams{FromAt: r.FromAt, ToAt: r.ToAt}); err != nil {
		return in, err
	}
	if in.Accounts, err = q.TrialBalance(ctx, r.ToAt); err != nil {
		return in, err
	}
	if in.TopUps, err = q.TopUpFunnel(ctx, store.TopUpFunnelParams{Unit: unit, FromAt: r.FromAt, ToAt: r.ToAt}); err != nil {
		return in, err
	}
	if in.Payouts, err = q.PayoutFunnel(ctx, store.PayoutFunnelParams{Unit: unit, FromAt: r.FromAt, ToAt: r.ToAt}); err != nil {
		return in, err
	}
	if in.Tax, err = q.TaxPosition(ctx); err != nil {
		return in, err
	}
	return in, nil
}

// dateOf is t's calendar date as a date-only value (midnight UTC), however t was scanned or parsed.
func dateOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// bucketOf is the first day of the period that holds day: the day itself, its Monday, or the first of its month.
func bucketOf(day time.Time, g Group) time.Time {
	switch g {
	case ByWeek:
		return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	case ByMonth:
		return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return day
	}
}

// nextBucket is the first day of the period after the one starting at bucket.
func nextBucket(bucket time.Time, g Group) time.Time {
	switch g {
	case ByWeek:
		return bucket.AddDate(0, 0, 7)
	case ByMonth:
		return bucket.AddDate(0, 1, 0)
	default:
		return bucket.AddDate(0, 0, 1)
	}
}

// Periods cuts from..to into the periods of g, clipping the first and last to the range. Every period appears, with
// zeros when nothing moved; a period's key is the date its bucket starts on (before clipping).
func Periods(from, to time.Time, g Group) (periods []Period, keys []string) {
	for start := from; !start.After(to); {
		b := bucketOf(start, g)
		next := nextBucket(b, g)
		end := next.AddDate(0, 0, -1)
		if end.After(to) {
			end = to
		}
		periods = append(periods, Period{Start: start.Format(time.DateOnly), End: end.Format(time.DateOnly)})
		keys = append(keys, b.Format(time.DateOnly))
		start = next
	}
	return periods, keys
}

// Summarize turns what was read into the dashboard's figures.
func Summarize(from, to time.Time, g Group, in Inputs) Summary {
	periods, keys := Periods(from, to, g)
	index := map[string]int{}
	for i, k := range keys {
		index[k] = i
	}
	at := func(day time.Time) (int, bool) {
		i, ok := index[bucketOf(dateOf(day), g).Format(time.DateOnly)]
		return i, ok
	}

	var total Figures
	pools := map[string]*PrizePool{}
	for _, f := range in.Flows {
		i, ok := at(f.Day)
		if !ok {
			continue
		}
		apply(&periods[i].Figures, f)
		apply(&total, f)
		if t, ok := strings.CutPrefix(f.Account, "PRIZE_POOL_"); ok {
			p := pools[t]
			if p == nil {
				p = &PrizePool{Type: strings.ToLower(t)}
				pools[t] = p
			}
			credit := f.Direction == "C"
			switch {
			case f.Kind == "prize_pool_funding" && credit:
				p.FundedKES += f.AmountKes
			case f.Kind == "prize_payout" && !credit:
				p.PaidKES += f.AmountKes
			case f.Kind == "prize_payout_failed" && credit:
				p.PaidKES -= f.AmountKes
			}
		}
	}
	for i := range periods {
		periods[i].GGRKES = periods[i].StakesKES - periods[i].PrizesAwardedKES
	}
	total.GGRKES = total.StakesKES - total.PrizesAwardedKES
	for _, p := range in.Plays {
		if i, ok := at(p.Bucket); ok {
			periods[i].Plays, periods[i].Players = p.Plays, p.Players
		}
	}
	for _, su := range in.Signups {
		if i, ok := at(su.Day); ok {
			periods[i].Signups += su.Signups
		}
	}

	byKind := make([]KindTotals, 0, len(in.Kinds))
	for _, k := range in.Kinds {
		byKind = append(byKind, KindTotals{Kind: k.Kind, Journals: k.Journals, DebitKES: k.DebitKes, CreditKES: k.CreditKes})
	}

	tb := TrialBalance{AsAt: to.Format(time.DateOnly), Accounts: make([]Account, 0, len(in.Accounts))}
	for _, a := range in.Accounts {
		bal := a.DebitsKes - a.CreditsKes
		if a.Type == "liability" || a.Type == "revenue" {
			bal = -bal
		}
		tb.Accounts = append(tb.Accounts, Account{Code: a.Code, Name: a.Name, Type: a.Type, DebitsKES: a.DebitsKes, CreditsKES: a.CreditsKes, BalanceKES: bal})
		tb.DebitsKES += a.DebitsKes
		tb.CreditsKES += a.CreditsKes
	}
	tb.Balanced = tb.DebitsKES == tb.CreditsKES

	topUps := make([]TopUpPeriod, len(periods))
	for i, p := range periods {
		topUps[i] = TopUpPeriod{Start: p.Start, End: p.End}
	}
	for _, r := range in.TopUps {
		i, ok := at(r.Bucket)
		if !ok {
			continue
		}
		t := &topUps[i]
		t.Started = Count{r.Started, r.StartedKes}
		t.Credited = Count{r.Credited, r.CreditedKes}
		t.Failed = Count{r.Failed, r.FailedKes}
		t.Cancelled = Count{r.Cancelled, r.CancelledKes}
		t.Expired = Count{r.Expired, r.ExpiredKes}
		t.Unconfirmed = Count{r.Unconfirmed, r.UnconfirmedKes}
		t.Reversed = Count{r.Reversed, r.ReversedKes}
		t.PendingNow = Count{r.PendingNow, r.PendingNowKes}
		if r.MedianSecondsToCredit >= 0 {
			m := r.MedianSecondsToCredit
			t.MedianSecondsToCredit = &m
		}
	}

	payouts := make([]PayoutPeriod, len(periods))
	for i, p := range periods {
		payouts[i] = PayoutPeriod{Start: p.Start, End: p.End}
	}
	for _, r := range in.Payouts {
		i, ok := at(r.Bucket)
		if !ok {
			continue
		}
		payouts[i] = PayoutPeriod{Start: periods[i].Start, End: periods[i].End,
			Requested: Count{r.Requested, r.RequestedKes}, Paid: Count{r.Paid, r.PaidKes}, Failed: Count{r.Failed, r.FailedKes},
			Held: Count{r.Held, r.HeldKes}, Manual: Count{r.Manual, r.ManualKes}, TaxWithheldKES: r.TaxWithheldKes}
	}

	balances := balancesOf(in.Balances)
	for _, b := range in.Balances {
		if t, ok := strings.CutPrefix(b.Code, "PRIZE_POOL_"); ok {
			p := pools[t]
			if p == nil {
				p = &PrizePool{Type: strings.ToLower(t)}
				pools[t] = p
			}
			p.BalanceKES = -b.DebitBalance
		}
	}
	prizePools := make([]PrizePool, 0, len(pools))
	for _, t := range []string{"DAILY", "WEEKLY", "MONTHLY", "QUARTERLY"} {
		if p, ok := pools[t]; ok {
			prizePools = append(prizePools, *p)
			delete(pools, t)
		}
	}
	for _, p := range pools { // any pool added later, after the known ones
		prizePools = append(prizePools, *p)
	}

	tax := Tax{WithheldKES: total.TaxWithheldKES, RemittedKES: total.TaxRemittedKES}
	tax.AwaitingRemittance.Batches, tax.AwaitingRemittance.KES = in.Tax.AwaitingBatches, in.Tax.AwaitingKes
	tax.Unbatched.Events, tax.Unbatched.KES = in.Tax.UnbatchedEvents, in.Tax.UnbatchedKes
	if !in.Tax.OldestUnremittedAt.IsZero() {
		age := max(int64(in.Now.Sub(in.Tax.OldestUnremittedAt)/time.Second), 0)
		tax.OldestUnremittedAgeSeconds = &age
	}

	return Summary{From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Group: g, Totals: total, Periods: periods,
		Balances: balances, Activity: Activity{Signups: in.Activity.Signups, Plays: in.Activity.Plays, Players: in.Activity.Players},
		ByKind: byKind, TrialBalance: tb, TopUps: topUps, Payouts: payouts, PrizePools: prizePools, Tax: tax}
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

// File is a finished export: its name, bytes and SHA-256 (hex).
type File struct {
	Name   string
	Data   []byte
	SHA256 string
	Rows   int
}

// ExportSummary writes the summary as CSV (one row per period, the totals, then the trial balance) and records the
// export as finance.exported by viewer, with the range, the row count and the file's hash.
func (s *Service) ExportSummary(ctx context.Context, from, to time.Time, group Group, viewer string) (File, error) {
	sum, err := s.Summary(ctx, from, to, group)
	if err != nil {
		return File{}, err
	}
	var buf bytes.Buffer
	rows, err := WriteSummaryCSV(&buf, sum)
	if err != nil {
		return File{}, err
	}
	sha := sha256.Sum256(buf.Bytes())
	f := File{Name: "finance-" + sum.From + "-" + sum.To + ".csv", Data: buf.Bytes(), SHA256: hex.EncodeToString(sha[:]), Rows: rows}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: viewer, Action: "finance.exported", EntityType: "finance_export",
			Details: map[string]any{"from": sum.From, "to": sum.To, "group": sum.Group, "rows": rows, "sha256": f.SHA256}})
	})
	if err != nil {
		return File{}, err
	}
	return f, nil
}

var figureColumns = []string{"deposits_kes", "stakes_kes", "prizes_awarded_kes", "prizes_paid_kes", "withdrawals_kes", "reversals_kes",
	"excise_kes", "tax_withheld_kes", "tax_remitted_kes", "ggr_kes"}

func figureValues(f Figures) []string {
	return []string{n(f.DepositsKES), n(f.StakesKES), n(f.PrizesAwardedKES), n(f.PrizesPaidKES), n(f.WithdrawalsKES), n(f.ReversalsKES),
		n(f.ExciseKES), n(f.TaxWithheldKES), n(f.TaxRemittedKES), n(f.GGRKES)}
}

func n(v int64) string { return strconv.FormatInt(v, 10) }

// WriteSummaryCSV writes s as the auditor's file: the periods with their figures and activity, a blank line, the
// range's totals, a blank line, the trial balance as at the range's end, and whether it balances. It returns how
// many period rows it wrote.
func WriteSummaryCSV(w io.Writer, s Summary) (int, error) {
	cw := csv.NewWriter(w)
	write := func(rec ...string) {
		if cw.Error() == nil {
			_ = cw.Write(rec) // the error is read once at the end
		}
	}
	write(append([]string{"period_start", "period_end"}, append(figureColumns, "plays", "players", "signups")...)...)
	for _, p := range s.Periods {
		write(append([]string{p.Start, p.End}, append(figureValues(p.Figures), n(p.Plays), n(p.Players), n(p.Signups))...)...)
	}
	write()
	write(append([]string{"total_from", "total_to"}, append(figureColumns, "plays", "players", "signups")...)...)
	write(append([]string{s.From, s.To}, append(figureValues(s.Totals), n(s.Activity.Plays), n(s.Activity.Players), n(s.Activity.Signups))...)...)
	write()
	write("trial_balance_as_at", s.TrialBalance.AsAt)
	write("account_code", "account_name", "type", "debits_kes", "credits_kes", "balance_kes")
	for _, a := range s.TrialBalance.Accounts {
		write(a.Code, a.Name, a.Type, n(a.DebitsKES), n(a.CreditsKES), n(a.BalanceKES))
	}
	write()
	write("total_debits_kes", "total_credits_kes", "balanced")
	write(n(s.TrialBalance.DebitsKES), n(s.TrialBalance.CreditsKES), strconv.FormatBool(s.TrialBalance.Balanced))
	cw.Flush()
	return len(s.Periods), cw.Error()
}

// ledgerSQL lists every ledger entry posted in [from_at, to_at), oldest first. It is read with pgx directly rather
// than through sqlc so the rows stream to the file instead of being collected in memory.
const ledgerSQL = `
SELECT j.id::text, j.kind, j.reference_type, j.reference_id::text, j.created_at, a.code, e.direction::text, e.amount_kes, e.user_id::text
FROM ledger.ledger_entries e
JOIN ledger.ledger_journals j ON j.id = e.journal_id
JOIN ledger.ledger_accounts a ON a.id = e.account_id
WHERE j.created_at >= $1 AND j.created_at < $2
ORDER BY j.created_at, j.id, e.id`

// LedgerColumns are the ledger export's columns, in order.
var LedgerColumns = []string{"journal_id", "journal_kind", "reference_type", "reference_id", "created_at_eat", "account_code", "direction", "amount_kes", "user_id"}

// LedgerExport is a ledger export ready to stream: its range, how many entries it holds and the file's SHA-256,
// known before the first byte is sent because the file was hashed once already from the same database snapshot.
type LedgerExport struct {
	From, To string
	Name     string
	Rows     int64
	SHA256   string
	tx       pgx.Tx
	rng      Range
}

// PrepareLedgerExport checks the range, refuses more than the cap of entries (ErrTooManyRows), opens a read-only
// snapshot of the ledger, hashes the file, and records the export as ledger.exported by viewer with the range, the
// row count and the hash. The caller streams the file with WriteTo and must Close the export.
func (s *Service) PrepareLedgerExport(ctx context.Context, from, to time.Time, viewer string) (*LedgerExport, error) {
	rng, _, err := s.resolve(ctx, from, to)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	x := &LedgerExport{From: rng.From.Format(time.DateOnly), To: rng.To.Format(time.DateOnly), tx: tx, rng: rng}
	x.Name = "ledger-" + x.From + "-" + x.To + ".csv"
	ok := false
	defer func() {
		if !ok {
			x.Close(ctx)
		}
	}()
	x.Rows, err = store.New(tx).CountLedgerEntries(ctx, store.CountLedgerEntriesParams{FromAt: rng.FromAt, ToAt: rng.ToAt})
	if err != nil {
		return nil, err
	}
	if x.Rows > s.rowCap {
		return nil, ErrTooManyRows
	}
	h := sha256.New()
	if _, err := x.stream(ctx, h); err != nil {
		return nil, err
	}
	x.SHA256 = hex.EncodeToString(h.Sum(nil))
	err = pgx.BeginFunc(ctx, s.pool, func(atx pgx.Tx) error {
		return s.audit.Record(ctx, atx, audit.Event{ActorType: audit.Staff, ActorID: viewer, Action: "ledger.exported", EntityType: "ledger_export",
			Details: map[string]any{"from": x.From, "to": x.To, "rows": x.Rows, "sha256": x.SHA256}})
	})
	if err != nil {
		return nil, err
	}
	ok = true
	return x, nil
}

// WriteTo streams the file to w from the same snapshot it was hashed from, and checks the bytes sent hash to SHA256.
func (x *LedgerExport) WriteTo(ctx context.Context, w io.Writer) error {
	h := sha256.New()
	rows, err := x.stream(ctx, io.MultiWriter(w, h))
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != x.SHA256 || rows != x.Rows {
		return fmt.Errorf("finance: ledger export changed while streaming (%d rows, sha %s; announced %d, %s)", rows, got, x.Rows, x.SHA256)
	}
	return nil
}

// Close ends the snapshot.
func (x *LedgerExport) Close(ctx context.Context) {
	if err := x.tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.WarnContext(ctx, "finance: closing a ledger export snapshot", "err", err)
	}
}

func (x *LedgerExport) stream(ctx context.Context, w io.Writer) (int64, error) {
	rows, err := x.tx.Query(ctx, ledgerSQL, x.rng.FromAt, x.rng.ToAt)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cw := csv.NewWriter(w)
	if err := cw.Write(LedgerColumns); err != nil {
		return 0, err
	}
	var count int64
	for rows.Next() {
		var journalID, kind, refType, refID, account, direction string
		var userID *string
		var at time.Time
		var amount int64
		if err := rows.Scan(&journalID, &kind, &refType, &refID, &at, &account, &direction, &amount, &userID); err != nil {
			return count, err
		}
		user := ""
		if userID != nil {
			user = *userID
		}
		if err := cw.Write([]string{journalID, kind, refType, refID, clock.InNairobi(at).Format("2006-01-02 15:04:05"), account, direction, n(amount), user}); err != nil {
			return count, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	cw.Flush()
	return count, cw.Error()
}
