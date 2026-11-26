// Package metrics reads how the player apps are used, for the back office's App metrics page (ADR 0019): sign-ups,
// sign-ins, players seen, plays, top-ups and withdrawals per Nairobi day and platform (web, android or ios: the
// platform of the device a session was opened from). It only reads the platform's own tables.
package metrics

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/4draws-lottery-platform/internal/metrics/store"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
)

// MaxDays is the longest range one summary covers; DefaultDays the range shown when none is asked for.
const (
	MaxDays     = 366
	DefaultDays = 30
)

// ErrRange is a range that is reversed or longer than MaxDays.
var ErrRange = errors.New("metrics: the range must run forward and cover at most 366 days")

// Platforms are the player apps, in the order the back office shows them. Unknown covers sessions without a device
// record (there should be none) and is listed only when it occurs.
var Platforms = []string{"web", "android", "ios"}

// Unknown is the platform of events that cannot be tied to a device.
const Unknown = "unknown"

// Figures are one day's (or a range's) counts for one platform. Money is in whole KES.
type Figures struct {
	Signups        int64 `json:"signups"`         // accounts created, by the platform of their first device
	Sessions       int64 `json:"sessions"`        // sign-ins
	ActivePlayers  int64 `json:"active_players"`  // players seen (a token issued to them), once per platform per day
	Plays          int64 `json:"plays"`           // paid plays
	StakesKES      int64 `json:"stakes_kes"`      // what those plays cost
	TopUps         int64 `json:"top_ups"`         // top-ups credited to wallets
	TopUpsKES      int64 `json:"top_ups_kes"`     // gross, as M-Pesa confirmed
	Withdrawals    int64 `json:"withdrawals"`     // withdrawals requested (a failed one comes back as a refund)
	WithdrawalsKES int64 `json:"withdrawals_kes"` // gross
}

func (f *Figures) add(o Figures) {
	f.Signups += o.Signups
	f.Sessions += o.Sessions
	f.ActivePlayers += o.ActivePlayers
	f.Plays += o.Plays
	f.StakesKES += o.StakesKES
	f.TopUps += o.TopUps
	f.TopUpsKES += o.TopUpsKES
	f.Withdrawals += o.Withdrawals
	f.WithdrawalsKES += o.WithdrawalsKES
}

// Cell is one platform's figures on one day.
type Cell struct {
	Day      string `json:"day"`
	Platform string `json:"platform"`
	Figures
}

// Total is one platform's figures over the range.
type Total struct {
	Platform string `json:"platform"`
	Figures
}

// Summary is the page: every day of the range for every platform (zeros where nothing happened), the totals per
// platform, and the totals over all platforms. Active players are not summed across days (a player seen on two days
// counts twice); the totals carry the sum of the days and say so in the back office.
type Summary struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Platforms []string `json:"platforms"`
	Days      []Cell   `json:"days"`
	Totals    []Total  `json:"totals"`
	All       Figures  `json:"all"`
}

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
	signups, err := q.Signups(ctx, store.SignupsParams{FromAt: fromAt, ToAt: toAt})
	if err != nil {
		return Summary{}, err
	}
	sessions, err := q.Sessions(ctx, store.SessionsParams{FromAt: fromAt, ToAt: toAt})
	if err != nil {
		return Summary{}, err
	}
	active, err := q.ActivePlayers(ctx, store.ActivePlayersParams{FromAt: fromAt, ToAt: toAt})
	if err != nil {
		return Summary{}, err
	}
	money, err := q.Money(ctx, store.MoneyParams{FromAt: fromAt, ToAt: toAt, FromDay: from, ToDay: to})
	if err != nil {
		return Summary{}, err
	}
	return Summarize(from, to, signups, sessions, active, money), nil
}

// Summarize lays the counts out over every day of the range and every platform.
func Summarize(from, to time.Time, signups []store.SignupsRow, sessions []store.SessionsRow, active []store.ActivePlayersRow, money []store.MoneyRow) Summary {
	platforms := slices.Clone(Platforms) // unknown joins only when something could not be placed
	seen := func(p string) {
		if !slices.Contains(platforms, p) {
			platforms = append(platforms, p)
		}
	}
	for _, r := range signups {
		seen(r.Platform)
	}
	for _, r := range sessions {
		seen(r.Platform)
	}
	for _, r := range active {
		seen(r.Platform)
	}
	for _, r := range money {
		seen(r.Platform)
	}
	cells := map[[2]string]*Cell{}
	var days []Cell
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		for _, p := range platforms {
			days = append(days, Cell{Day: d.Format(time.DateOnly), Platform: p})
		}
	}
	for i := range days {
		cells[[2]string{days[i].Day, days[i].Platform}] = &days[i]
	}
	at := func(day time.Time, platform string) *Cell {
		return cells[[2]string{day.Format(time.DateOnly), platform}]
	}
	for _, r := range signups {
		if c := at(r.Day, r.Platform); c != nil {
			c.Signups += r.N
		}
	}
	for _, r := range sessions {
		if c := at(r.Day, r.Platform); c != nil {
			c.Sessions += r.N
		}
	}
	for _, r := range active {
		if c := at(r.Day, r.Platform); c != nil {
			c.ActivePlayers += r.N
		}
	}
	for _, r := range money {
		c := at(r.Day, r.Platform)
		if c == nil {
			continue
		}
		switch r.Kind {
		case "play":
			c.Plays += r.N
			c.StakesKES += r.Kes
		case "top_up":
			c.TopUps += r.N
			c.TopUpsKES += r.Kes
		case "withdrawal":
			c.Withdrawals += r.N
			c.WithdrawalsKES += r.Kes
		}
	}
	totals := make([]Total, 0, len(platforms))
	var all Figures
	for _, p := range platforms {
		t := Total{Platform: p}
		for _, c := range days {
			if c.Platform == p {
				t.add(c.Figures)
			}
		}
		all.add(t.Figures)
		totals = append(totals, t)
	}
	return Summary{From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Platforms: platforms, Days: days, Totals: totals, All: all}
}
