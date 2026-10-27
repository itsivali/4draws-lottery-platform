//go:build integration

package draws_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/draws/drand"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/crypto"
	"github.com/itsivali/lottery-platform/internal/platform/testpg"
)

// fakeBeacon answers any published round with deterministic randomness; verifying real beacons is drand's own test.
type fakeBeacon struct {
	down  atomic.Bool
	calls atomic.Int32
}

func (*fakeBeacon) Chain() drand.Chain { return drand.Quicknet }

// Verify accepts exactly what Fetch serves.
func (*fakeBeacon) Verify(b drand.Beacon) error {
	var r [8]byte
	binary.BigEndian.PutUint64(r[:], b.Round)
	if sum := sha256.Sum256(r[:]); b.Signature != "00" || b.Randomness != hex.EncodeToString(sum[:]) {
		return drand.ErrInvalid
	}
	return nil
}

func (f *fakeBeacon) Fetch(_ context.Context, now time.Time, round uint64) (drand.Beacon, error) {
	f.calls.Add(1)
	if now.Before(drand.Quicknet.TimeOf(round)) {
		return drand.Beacon{}, drand.ErrNotYet
	}
	if f.down.Load() {
		return drand.Beacon{}, drand.ErrUnavailable
	}
	var r [8]byte
	binary.BigEndian.PutUint64(r[:], round)
	sum := sha256.Sum256(r[:])
	return drand.Beacon{Round: round, Randomness: hex.EncodeToString(sum[:]), Signature: "00"}, nil
}

type env struct {
	pool   *pgxpool.Pool
	clk    *clock.Manual
	beacon *fakeBeacon
	svc    *draws.Service
}

// nairobi is a wall-clock time in Nairobi.
func nairobi(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, clock.Nairobi).UTC()
}

func setup(t *testing.T, at time.Time) *env {
	t.Helper()
	pool := testpg.Pool(t, "app_worker")
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	kr, err := crypto.NewKeyring(1, map[byte][]byte{1: k})
	if err != nil {
		t.Fatal(err)
	}
	e := &env{pool: pool, clk: clock.NewManual(at), beacon: &fakeBeacon{}}
	e.svc = draws.NewService(pool, e.clk, kr, e.beacon, ledger.NewStore())
	return e
}

var seq atomic.Int64

// playerDay records a new player's day as the game would: plays tickets of KES 100 and the day's tally. It is the
// player's first play day unless an earlier one was recorded.
func (e *env) playerDay(t *testing.T, date string, plays int, lucky bool) string {
	t.Helper()
	n := seq.Add(1)
	var user string
	if err := e.pool.QueryRow(context.Background(), `INSERT INTO identity.users (msisdn, kyc_status, national_id_hash, password_hash, msisdn_verified_at)
		VALUES ($1, 'verified', $2, 'x', now()) RETURNING id::text`, fmt.Sprintf("2547%08d", 50_000_000+n), []byte(fmt.Sprint("draw-id-", n))).Scan(&user); err != nil {
		t.Fatal(err)
	}
	e.playOn(t, user, date, plays, lucky)
	return user
}

// playOn records another day of plays for user, as the game would (the first play day is kept).
func (e *env) playOn(t *testing.T, user, date string, plays int, lucky bool) {
	t.Helper()
	ctx := context.Background()
	for i := 1; i <= plays; i++ {
		if _, err := e.pool.Exec(ctx, `INSERT INTO tickets.tickets (user_id, play_date, seq, price_kes, idempotency_key) VALUES ($1, $2, $3, 100, $4)`,
			user, date, i, fmt.Sprint(date, "-", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO tickets.player_days (user_id, play_date, paid_plays, lucky) VALUES ($1, $2, $3, $4)`,
		user, date, plays, lucky); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO tickets.player_starts (user_id, first_play_date) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		user, date); err != nil {
		t.Fatal(err)
	}
}

func (e *env) draw(t *testing.T, typ, start, status string) draws.Draw {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(), `SELECT id::text FROM draws.draws WHERE draw_type = $1 AND period_start = $2 AND status = $3
		ORDER BY created_at DESC LIMIT 1`, typ, start, status).Scan(&id); err != nil {
		t.Fatalf("no %s %s draw %s: %v", status, typ, start, err)
	}
	d, err := e.svc.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (e *env) schedule(t *testing.T) {
	t.Helper()
	if err := e.svc.Schedule(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func (e *env) account(t *testing.T, code string) int64 {
	t.Helper()
	var b int64
	if err := e.pool.QueryRow(context.Background(), `SELECT coalesce(sum(CASE e.direction WHEN 'C' THEN e.amount_kes ELSE -e.amount_kes END), 0)
		FROM ledger.ledger_entries e JOIN ledger.ledger_accounts a ON a.id = e.account_id WHERE a.code = $1`, code).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDraw_FullLifecycleFromPlaysToAnApprovedFundedResult(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 24, 12, 0))
	a := e.playerDay(t, "2026-10-24", 2, false) // tier 2 → 2 entries
	b := e.playerDay(t, "2026-10-24", 4, true)  // tier 5, lucky → 10 entries
	c := e.playerDay(t, "2026-10-24", 1, false) // 1 entry

	e.schedule(t) // today's daily draw is open; nobody's own week, month or quarter ends today (they started today)
	e.draw(t, "daily", "2026-10-24", "open")
	if n := e.count(t, `SELECT count(*) FROM draws.draws WHERE draw_type <> 'daily'`); n != 0 {
		t.Fatalf("%d longer draws opened: periods are the players' own (ADR 0017)", n)
	}

	e.clk.Set(nairobi(2026, 10, 25, 0, 6)) // after 00:05: the day's draw freezes
	e.schedule(t)
	d := e.draw(t, "daily", "2026-10-24", "frozen")
	if d.Entries != 13 || len(d.SeedCommit) != 64 || d.RoundAt == nil {
		t.Fatalf("frozen: %+v", d)
	}
	if want := drand.Quicknet.RoundAt(nairobi(2026, 10, 25, 9, 0)); d.BeaconRound != int64(want) { //nolint:gosec // test round
		t.Fatalf("round %d, want the first at or after 09:00 (%d)", d.BeaconRound, want)
	}
	// the entry list hash is reproducible from the entries
	h := sha256.New()
	rows, err := e.pool.Query(ctx, `SELECT entry_no, user_id::text, play_date, ticket_id::text FROM draws.draw_entries WHERE draw_id = $1 ORDER BY entry_no`, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	perUser := map[string]int{}
	for rows.Next() {
		var no int
		var u, ticket string
		var pd time.Time
		_ = rows.Scan(&no, &u, &pd, &ticket)
		perUser[u]++
		_, _ = fmt.Fprintf(h, "%d|%s|%s|%s\n", no, u, pd.Format(time.DateOnly), ticket)
	}
	rows.Close()
	if hex.EncodeToString(h.Sum(nil)) != d.EntryListHash || perUser[a] != 2 || perUser[b] != 10 || perUser[c] != 1 {
		t.Fatalf("entries %v, hash %s", perUser, d.EntryListHash)
	}

	// two different officials sign before the round
	if d, err = e.svc.Sign(ctx, d.ID, "operator:alice"); err != nil || d.Status != "frozen" {
		t.Fatalf("first signature: %v %v", d.Status, err)
	}
	if _, err := e.svc.Sign(ctx, d.ID, "operator:alice"); !errors.Is(err, draws.ErrAlreadySigned) {
		t.Fatalf("same official twice: %v", err)
	}
	if d, err = e.svc.Sign(ctx, d.ID, "operator:bob"); err != nil || d.Status != "signed" || len(d.Signoffs) != 2 {
		t.Fatalf("second signature: %+v %v", d, err)
	}

	e.clk.Set(nairobi(2026, 10, 25, 8, 59)) // before the round nothing is drawn
	e.schedule(t)
	if e.draw(t, "daily", "2026-10-24", "signed"); e.beacon.calls.Load() != 0 {
		t.Fatal("asked drand before the round")
	}

	e.clk.Set(nairobi(2026, 10, 25, 9, 1))
	e.schedule(t)
	d = e.draw(t, "daily", "2026-10-24", "conducted")
	// stakes 7 tickets × 100 = 700; pool 30% = 210; ranks 50/30/20% = 105/63/42; three players, three different winners
	if len(d.Results) != 3 || d.Results[0].AmountKES != 105 || d.Results[1].AmountKES != 63 || d.Results[2].AmountKES != 42 {
		t.Fatalf("results %+v", d.Results)
	}
	if d.Results[0].UserID == d.Results[1].UserID || d.Results[1].UserID == d.Results[2].UserID || d.Results[0].UserID == d.Results[2].UserID {
		t.Fatalf("a player won twice: %+v", d.Results)
	}
	// anyone can recompute the winners from the certificate
	var cert draws.Certificate
	if err := json.Unmarshal(d.Certificate, &cert); err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(d.Certificate); hex.EncodeToString(sum[:]) != d.CertificateHash || cert.PoolKES != 210 || cert.StakesKES != 700 {
		t.Fatalf("certificate %s", d.Certificate)
	}
	seed, _ := hex.DecodeString(cert.RevealedSeed)
	if c := sha256.Sum256(seed); hex.EncodeToString(c[:]) != d.SeedCommit {
		t.Fatal("revealed seed does not match the commitment signed by the officials")
	}
	randomness, _ := hex.DecodeString(cert.Beacon.Randomness)
	key := draws.Key(randomness, seed, cert.EntryListHash, d.ID)
	byNo := map[int]string{}
	rows, _ = e.pool.Query(ctx, `SELECT entry_no, user_id::text FROM draws.draw_entries WHERE draw_id = $1`, d.ID)
	for rows.Next() {
		var no int
		var u string
		_ = rows.Scan(&no, &u)
		byNo[no] = u
	}
	rows.Close()
	won, r := map[string]bool{}, 0
	for i := 0; r < 3; i++ {
		u := byNo[draws.Pick(key, i, cert.Entries)]
		if won[u] {
			continue
		}
		won[u] = true
		if d.Results[r].UserID != u {
			t.Fatalf("rank %d: recomputed %s, stored %s", r+1, u, d.Results[r].UserID)
		}
		r++
	}

	if _, err := e.svc.Sign(ctx, d.ID, "operator:carol"); !errors.Is(err, draws.ErrState) {
		t.Fatalf("signing a conducted draw: %v", err)
	}
	if _, err := e.svc.Void(ctx, d.ID, "operator:carol", "do not like it"); !errors.Is(err, draws.ErrState) {
		t.Fatalf("voiding a conducted draw: %v", err)
	}
	revenueBefore := e.account(t, ledger.StakesRevenue)
	if d, err = e.svc.Approve(ctx, d.ID, "operator:alice"); err != nil || d.Status != "conducted" {
		t.Fatalf("first approval: %v %v", d.Status, err)
	}
	if d, err = e.svc.Approve(ctx, d.ID, "operator:dave"); err != nil || d.Status != "approved" {
		t.Fatalf("second approval: %v %v", d.Status, err)
	}
	if e.account(t, "PRIZE_POOL_DAILY") != 210 || revenueBefore-e.account(t, ledger.StakesRevenue) != 210 {
		t.Fatalf("pool %d", e.account(t, "PRIZE_POOL_DAILY"))
	}
}

func TestDraw_NotSignedBeforeTheRoundIsVoidedAndRerun(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 25, 0, 6))
	e.playerDay(t, "2026-10-24", 3, false)
	e.schedule(t) // opens today's draws; yesterday's daily draw is created by freezing below
	if _, err := e.pool.Exec(ctx, `INSERT INTO draws.draws (draw_type, period_start, period_end) VALUES ('daily', '2026-10-24', '2026-10-24')`); err != nil {
		t.Fatal(err)
	}
	e.schedule(t)
	first := e.draw(t, "daily", "2026-10-24", "frozen")
	if _, err := e.svc.Sign(ctx, first.ID, "operator:alice"); err != nil {
		t.Fatal(err)
	}
	e.clk.Set(nairobi(2026, 10, 25, 9, 1)) // one signature only when the round is published
	if _, err := e.svc.Sign(ctx, first.ID, "operator:bob"); !errors.Is(err, draws.ErrTooLate) {
		t.Fatalf("signing after the round: %v", err)
	}
	e.schedule(t)
	voided := e.draw(t, "daily", "2026-10-24", "voided")
	rerun := e.draw(t, "daily", "2026-10-24", "frozen")
	if voided.ID != first.ID || rerun.RerunOf != first.ID || rerun.EntryListHash != first.EntryListHash || rerun.SeedCommit == first.SeedCommit {
		t.Fatalf("rerun %+v of %+v", rerun, voided)
	}
	if want := drand.Quicknet.RoundAt(nairobi(2026, 10, 26, 9, 0)); rerun.BeaconRound != int64(want) { //nolint:gosec // test round
		t.Fatalf("rerun round %d, want the next 09:00 (%d)", rerun.BeaconRound, want)
	}
	if e.beacon.calls.Load() != 0 {
		t.Fatal("the beacon was fetched for an unsigned draw")
	}
}

func TestDraw_AnOfficialMayVoidBeforeTheRoundButNotAfter(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 25, 0, 6))
	e.playerDay(t, "2026-10-24", 1, false)
	if _, err := e.pool.Exec(ctx, `INSERT INTO draws.draws (draw_type, period_start, period_end) VALUES ('daily', '2026-10-24', '2026-10-24')`); err != nil {
		t.Fatal(err)
	}
	e.schedule(t)
	d := e.draw(t, "daily", "2026-10-24", "frozen")
	if _, err := e.svc.Void(ctx, d.ID, "", "x"); !errors.Is(err, draws.ErrOfficial) {
		t.Fatalf("void without an official: %v", err)
	}
	rerun, err := e.svc.Void(ctx, d.ID, "operator:alice", "a ticket was missing")
	if err != nil || rerun.RerunOf != d.ID || rerun.Status != "frozen" {
		t.Fatalf("void: %+v %v", rerun, err)
	}
	for _, off := range []string{"operator:alice", "operator:bob"} {
		if _, err := e.svc.Sign(ctx, rerun.ID, off); err != nil {
			t.Fatal(err)
		}
	}
	e.clk.Set(nairobi(2026, 10, 25, 9, 1))
	if _, err := e.svc.Void(ctx, rerun.ID, "operator:alice", "after the round"); !errors.Is(err, draws.ErrTooLate) {
		t.Fatalf("void after the round: %v", err)
	}
}

func TestDraw_OnePrizePerPlayerAndEmptyPeriods(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 25, 0, 6))
	solo := e.playerDay(t, "2026-10-24", 4, true) // 10 entries, one player, three ranks
	for _, p := range []string{"2026-10-24", "2026-10-23"} {
		if _, err := e.pool.Exec(ctx, `INSERT INTO draws.draws (draw_type, period_start, period_end) VALUES ('daily', $1, $1)`, p); err != nil {
			t.Fatal(err)
		}
	}
	e.schedule(t)
	if v := e.draw(t, "daily", "2026-10-23", "voided"); v.VoidedReason != "no entries" {
		t.Fatalf("empty day: %+v", v)
	}
	d := e.draw(t, "daily", "2026-10-24", "frozen")
	for _, off := range []string{"operator:a", "operator:b"} {
		if _, err := e.svc.Sign(ctx, d.ID, off); err != nil {
			t.Fatal(err)
		}
	}
	e.clk.Set(nairobi(2026, 10, 25, 9, 1))
	e.beacon.down.Store(true) // drand down: the draw waits, signed
	e.schedule(t)
	e.draw(t, "daily", "2026-10-24", "signed")
	e.beacon.down.Store(false)
	e.schedule(t)
	d = e.draw(t, "daily", "2026-10-24", "conducted")
	if len(d.Results) != 1 || d.Results[0].UserID != solo || d.Results[0].Rank != 1 || d.Results[0].AmountKES != 60 { // 400 × 30% × 50%
		t.Fatalf("results %+v", d.Results)
	}
}

func TestDraw_TheDatabaseRefusesSkippedOrBackwardTransitions(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 25, 0, 6))
	e.playerDay(t, "2026-10-24", 1, false)
	if _, err := e.pool.Exec(ctx, `INSERT INTO draws.draws (draw_type, period_start, period_end) VALUES ('daily', '2026-10-24', '2026-10-24')`); err != nil {
		t.Fatal(err)
	}
	e.schedule(t)
	d := e.draw(t, "daily", "2026-10-24", "frozen")
	for _, sql := range []string{
		`UPDATE draws.draws SET status = 'approved' WHERE id = $1`,
		`UPDATE draws.draws SET status = 'open' WHERE id = $1`,
		`UPDATE draws.draws SET entry_list_hash = repeat('0', 64) WHERE id = $1`,
		`UPDATE draws.draws SET beacon_round = beacon_round + 1 WHERE id = $1`,
		`DELETE FROM draws.draw_entries WHERE draw_id = $1`,
	} {
		if _, err := e.pool.Exec(ctx, sql, d.ID); err == nil {
			t.Errorf("allowed: %s", sql)
		}
	}
	if _, err := e.svc.Get(ctx, "not-a-uuid"); !errors.Is(err, draws.ErrNotFound) {
		t.Fatalf("get: %v", err)
	}
	if _, err := e.svc.Sign(ctx, "0192f000-0000-7000-8000-000000000000", "operator:a"); !errors.Is(err, draws.ErrNotFound) {
		t.Fatalf("sign unknown: %v", err)
	}
	if list, err := e.svc.List(ctx, 10); err != nil || len(list) == 0 {
		t.Fatalf("list: %v %v", len(list), err)
	}
}

func TestDraw_StepsRefuseTheWrongStateAndUnknownDraws(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 25, 0, 6))
	e.playerDay(t, "2026-10-24", 1, false)
	if _, err := e.pool.Exec(ctx, `INSERT INTO draws.draws (draw_type, period_start, period_end) VALUES ('daily', '2026-10-24', '2026-10-24')`); err != nil {
		t.Fatal(err)
	}
	unknown := "0192f000-0000-7000-8000-000000000000"
	if _, err := e.svc.Freeze(ctx, unknown); !errors.Is(err, draws.ErrNotFound) {
		t.Errorf("freeze unknown: %v", err)
	}
	if _, err := e.svc.Conduct(ctx, unknown); !errors.Is(err, draws.ErrNotFound) {
		t.Errorf("conduct unknown: %v", err)
	}
	e.schedule(t)
	d := e.draw(t, "daily", "2026-10-24", "frozen")
	if _, err := e.svc.Freeze(ctx, d.ID); !errors.Is(err, draws.ErrState) {
		t.Errorf("freeze twice: %v", err)
	}
	if _, err := e.svc.Conduct(ctx, d.ID); !errors.Is(err, draws.ErrState) {
		t.Errorf("conduct unsigned: %v", err)
	}
	if _, err := e.svc.Approve(ctx, d.ID, "operator:a"); !errors.Is(err, draws.ErrState) {
		t.Errorf("approve unconducted: %v", err)
	}
	for _, f := range []func() error{
		func() error { _, err := e.svc.Sign(ctx, d.ID, " "); return err },
		func() error { _, err := e.svc.Approve(ctx, d.ID, ""); return err },
		func() error { _, err := e.svc.Void(ctx, d.ID, "operator:a", ""); return err },
	} {
		if err := f(); !errors.Is(err, draws.ErrOfficial) {
			t.Errorf("missing official or reason: %v", err)
		}
	}
	for _, off := range []string{"operator:a", "operator:b"} {
		if _, err := e.svc.Sign(ctx, d.ID, off); err != nil {
			t.Fatal(err)
		}
	}
	// the round is not out: a direct conduct waits for drand
	if _, err := e.svc.Conduct(ctx, d.ID); !errors.Is(err, drand.ErrNotYet) {
		t.Errorf("conduct before the round: %v", err)
	}
	e.clk.Set(nairobi(2026, 10, 25, 9, 1))
	if _, err := e.svc.Conduct(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Conduct(ctx, d.ID); !errors.Is(err, draws.ErrState) {
		t.Errorf("conduct twice: %v", err)
	}
	if _, err := e.svc.Approve(ctx, d.ID, "operator:a"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Approve(ctx, d.ID, "operator:a"); !errors.Is(err, draws.ErrAlreadySigned) {
		t.Errorf("same official approving twice: %v", err)
	}
}

func (e *env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// entriesOf counts a draw's entries per player.
func (e *env) entriesOf(t *testing.T, drawID string) map[string]int {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), `SELECT user_id::text FROM draws.draw_entries WHERE draw_id = $1`, drawID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatal(err)
		}
		out[u]++
	}
	return out
}

func TestPersonalDraws_AWeeklyDrawHoldsThePlayersWhoseOwnWeekEndsThatDay(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 27, 12, 0))
	c := e.playerDay(t, "2026-10-27", 1, false) // C's first week: 27 Oct – 2 Nov
	a := e.playerDay(t, "2026-10-28", 2, false) // A's first week: 28 Oct – 3 Nov
	b := e.playerDay(t, "2026-10-29", 1, false) // B's first week: 29 Oct – 4 Nov
	e.playOn(t, a, "2026-11-01", 1, false)
	e.playOn(t, a, "2026-11-03", 1, false)
	e.playOn(t, b, "2026-11-03", 1, false)
	e.playOn(t, c, "2026-11-03", 1, false) // C's second week

	e.clk.Set(nairobi(2026, 11, 3, 12, 0))
	e.schedule(t) // A's week ends today: a weekly draw over 28 Oct – 3 Nov opens; no month or quarter ends
	w := e.draw(t, "weekly", "2026-10-28", "open")
	if w.PeriodEnd != "2026-11-03" || w.Basis != "personal" {
		t.Fatalf("weekly draw %+v", w)
	}
	if n := e.count(t, `SELECT count(*) FROM draws.draws WHERE draw_type IN ('monthly','quarterly')`); n != 0 {
		t.Fatalf("%d monthly/quarterly draws opened", n)
	}

	e.clk.Set(nairobi(2026, 11, 4, 0, 6))
	e.schedule(t)
	w = e.draw(t, "weekly", "2026-10-28", "frozen")
	if got := e.entriesOf(t, w.ID); len(got) != 1 || got[a] != 4 { // A's days: 2 plays → 2 chances, then 1 and 1
		t.Fatalf("weekly entries %v, want only A's 4", got)
	}
	if got := e.entriesOf(t, e.draw(t, "daily", "2026-11-03", "frozen").ID); len(got) != 3 {
		t.Fatalf("the daily draw is everyone's: %v", got)
	}

	for _, o := range []string{"operator:alice", "operator:bob"} {
		if _, err := e.svc.Sign(ctx, w.ID, o); err != nil {
			t.Fatal(err)
		}
	}
	e.clk.Set(nairobi(2026, 11, 4, 9, 1))
	e.schedule(t)
	w = e.draw(t, "weekly", "2026-10-28", "conducted")
	var cert draws.Certificate
	if err := json.Unmarshal(w.Certificate, &cert); err != nil {
		t.Fatal(err)
	}
	// the pool is a share of the stakes of the play days in the draw: A's 4 tickets, not everyone's
	if cert.Basis != "personal" || cert.StakesKES != 400 || cert.PoolKES != 40 || len(w.Results) != 1 || w.Results[0].UserID != a {
		t.Fatalf("certificate %s results %+v", w.Certificate, w.Results)
	}
}

func TestPersonalDraws_APlayDayAlreadyInAFrozenCalendarDrawIsNotEnteredAgain(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 28, 12, 0))
	a := e.playerDay(t, "2026-10-28", 1, false)
	e.playOn(t, a, "2026-11-03", 1, false)
	// a calendar week from before the switch, frozen with A's 28 October in it
	var old string
	if err := e.pool.QueryRow(ctx, `INSERT INTO draws.draws (draw_type, period_start, period_end) VALUES ('weekly', '2026-10-26', '2026-11-01')
		RETURNING id::text`).Scan(&old); err != nil {
		t.Fatal(err)
	}
	e.clk.Set(nairobi(2026, 11, 2, 0, 6))
	if _, err := e.svc.Freeze(ctx, old); err != nil {
		t.Fatal(err)
	}
	e.clk.Set(nairobi(2026, 11, 3, 12, 0))
	e.schedule(t)
	e.clk.Set(nairobi(2026, 11, 4, 0, 6))
	e.schedule(t)
	w := e.draw(t, "weekly", "2026-10-28", "frozen")
	if got := e.entriesOf(t, w.ID); got[a] != 1 {
		t.Fatalf("entries %v: 28 October is already in the calendar draw, only 3 November enters", got)
	}
}

func TestPersonalDraws_PlayersSeeTheirOwnPeriods(t *testing.T) {
	ctx := context.Background()
	e := setup(t, nairobi(2026, 10, 30, 12, 0))
	a := e.playerDay(t, "2026-10-01", 1, false)
	got, err := e.svc.PeriodsOf(ctx, a)
	if err != nil || len(got) != 4 {
		t.Fatalf("periods %+v %v", got, err)
	}
	want := map[string][2]string{"daily": {"2026-10-30", "2026-10-30"}, "weekly": {"2026-10-29", "2026-11-04"},
		"monthly": {"2026-10-01", "2026-10-30"}, "quarterly": {"2026-10-01", "2026-12-29"}}
	for _, p := range got {
		if w := want[p.DrawType]; p.Start != w[0] || p.End != w[1] || p.DrawnOn != day(p.End).AddDate(0, 0, 1).Format(time.DateOnly) {
			t.Errorf("%s: %+v, want %v", p.DrawType, p, w)
		}
	}
	if got, err := e.svc.PeriodsOf(ctx, e.newUser(t)); err != nil || len(got) != 0 {
		t.Fatalf("a player who has not played has no periods yet: %+v %v", got, err)
	}
}

func (e *env) newUser(t *testing.T) string {
	t.Helper()
	n := seq.Add(1)
	var user string
	if err := e.pool.QueryRow(context.Background(), `INSERT INTO identity.users (msisdn, kyc_status, national_id_hash, password_hash)
		VALUES ($1, 'verified', $2, 'x') RETURNING id::text`, fmt.Sprintf("2547%08d", 50_000_000+n), []byte(fmt.Sprint("draw-id-", n))).Scan(&user); err != nil {
		t.Fatal(err)
	}
	return user
}

// openWeekly opens a personal weekly draw that has not been drawn (the scheduler opens one only when some player's
// own week ends that day).
func (e *env) openWeekly(t *testing.T) draws.Draw {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(), `INSERT INTO draws.draws (draw_type, basis, period_start, period_end)
		VALUES ('weekly', 'personal', '2026-10-19', '2026-10-25') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
