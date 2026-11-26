package metrics_test

import (
	"testing"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/metrics"
	"github.com/itsivali/4draws-lottery-platform/internal/metrics/store"
)

func day(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }

func TestSummarize_LaysEveryDayAndPlatformOutAndTotalsThem(t *testing.T) {
	from, to := day("2026-11-01"), day("2026-11-03")
	sum := metrics.Summarize(from, to,
		[]store.SignupsRow{{Day: day("2026-11-01"), Platform: "android", N: 3}, {Day: day("2026-11-02"), Platform: "web", N: 1}},
		[]store.SessionsRow{{Day: day("2026-11-01"), Platform: "android", N: 5}, {Day: day("2026-11-03"), Platform: "ios", N: 2}},
		[]store.ActivePlayersRow{{Day: day("2026-11-01"), Platform: "android", N: 4}, {Day: day("2026-11-02"), Platform: "android", N: 2}},
		[]store.MoneyRow{
			{Day: day("2026-11-01"), Platform: "android", Kind: "play", N: 6, Kes: 300},
			{Day: day("2026-11-01"), Platform: "android", Kind: "top_up", N: 2, Kes: 1000},
			{Day: day("2026-11-02"), Platform: "web", Kind: "withdrawal", N: 1, Kes: 500},
			{Day: day("2026-10-31"), Platform: "web", Kind: "play", N: 9, Kes: 900}, // outside the range: ignored
		})
	if sum.From != "2026-11-01" || sum.To != "2026-11-03" || len(sum.Platforms) != 3 || len(sum.Days) != 9 {
		t.Fatalf("shape: %+v", sum)
	}
	if c := sum.Days[1]; c.Day != "2026-11-01" || c.Platform != "android" || c.Signups != 3 || c.Sessions != 5 || c.ActivePlayers != 4 ||
		c.Plays != 6 || c.StakesKES != 300 || c.TopUps != 2 || c.TopUpsKES != 1000 {
		t.Fatalf("android 1 Nov: %+v", c)
	}
	if c := sum.Days[3]; c.Day != "2026-11-02" || c.Platform != "web" || c.Signups != 1 || c.Withdrawals != 1 || c.WithdrawalsKES != 500 {
		t.Fatalf("web 2 Nov: %+v", c)
	}
	if c := sum.Days[8]; c.Platform != "ios" || c.Sessions != 2 || c.Plays != 0 {
		t.Fatalf("ios 3 Nov: %+v", c)
	}
	if sum.Totals[1].Platform != "android" || sum.Totals[1].ActivePlayers != 6 || sum.Totals[1].Plays != 6 {
		t.Fatalf("android total: %+v", sum.Totals[1])
	}
	if sum.All.Signups != 4 || sum.All.Sessions != 7 || sum.All.Plays != 6 || sum.All.StakesKES != 300 || sum.All.WithdrawalsKES != 500 {
		t.Fatalf("all: %+v", sum.All)
	}
}

func TestSummarize_ListsUnknownOnlyWhenSomethingCouldNotBePlaced(t *testing.T) {
	from := day("2026-11-01")
	if sum := metrics.Summarize(from, from, nil, nil, nil, nil); len(sum.Platforms) != 3 || len(sum.Days) != 3 {
		t.Fatalf("empty: %+v", sum.Platforms)
	}
	sum := metrics.Summarize(from, from, nil, []store.SessionsRow{{Day: from, Platform: "unknown", N: 1}}, nil, nil)
	if len(sum.Platforms) != 4 || sum.Platforms[3] != "unknown" || sum.Days[3].Sessions != 1 {
		t.Fatalf("unknown: %+v", sum)
	}
}
