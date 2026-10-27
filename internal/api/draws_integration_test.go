//go:build integration

package api_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/draws/drand"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/crypto"
)

// drawBeacon answers any published round with deterministic randomness.
type drawBeacon struct{}

func (drawBeacon) Chain() drand.Chain { return drand.Quicknet }

func (drawBeacon) Fetch(_ context.Context, now time.Time, round uint64) (drand.Beacon, error) {
	if now.Before(drand.Quicknet.TimeOf(round)) {
		return drand.Beacon{}, drand.ErrNotYet
	}
	var r [8]byte
	binary.BigEndian.PutUint64(r[:], round)
	sum := sha256.Sum256(r[:])
	return drand.Beacon{Round: round, Randomness: hex.EncodeToString(sum[:]), Signature: "00"}, nil
}

func (drawBeacon) Verify(drand.Beacon) error { return nil }

// conductDaily runs the real draw lifecycle for the env's play day (2026-10-21) as the worker would: its own clock,
// moved through the night, and a pool with the worker's rights on the same database. It returns the daily draw.
func (e *env) conductDaily(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	cfg := e.pool.Config().Copy()
	cfg.AfterConnect = nil // the test database's owner, standing in for app_worker (the api role cannot change draws)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	nairobi := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, clock.Nairobi).UTC() }
	clk := clock.NewManual(nairobi(21, 15, 0))
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	kr, err := crypto.NewKeyring(1, map[byte][]byte{1: k})
	if err != nil {
		t.Fatal(err)
	}
	svc := draws.NewService(pool, clk, kr, drawBeacon{}, ledger.NewStore())
	if err := svc.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	clk.Set(nairobi(22, 0, 6))
	if err := svc.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM draws.draws WHERE draw_type = 'daily' AND period_start = '2026-10-21'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, official := range []string{"operator:alice", "operator:bob"} {
		if _, err := svc.Sign(ctx, id, official); err != nil {
			t.Fatal(err)
		}
	}
	clk.Set(nairobi(22, 9, 1))
	if err := svc.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) userID(t *testing.T, s session) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(), `SELECT id::text FROM identity.users WHERE msisdn = $1`, s.msisdn).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAPI_DrawsArePublicAndPlayersSeeOnlyTheirOwnEntries(t *testing.T) {
	e := setup(t)
	a, b, c := e.signIn(t), e.signIn(t), e.signIn(t)
	for s, plays := range map[session]int{a: 2, b: 1} {
		e.canPlay(t, s, 1000)
		for range plays {
			if r := e.do(t, http.MethodPost, "/v1/plays", s.access, nil); r.Code != http.StatusCreated {
				t.Fatalf("play: %d %s", r.Code, r.Raw)
			}
		}
	}
	id := e.conductDaily(t)

	list := e.do(t, http.MethodGet, "/v1/draws", "", nil) // no token
	// yesterday's daily draw and today's; the longer draws open only when a player's own period ends (ADR 0017)
	if all, _ := list.Body["draws"].([]any); list.Code != http.StatusOK || len(all) < 2 || all[0].(map[string]any)["basis"] != "calendar" {
		t.Fatalf("list: %d %s", list.Code, list.Raw)
	}
	// each player sees their own periods, counted from their first play; before playing there are none
	if r := e.do(t, http.MethodGet, "/v1/draws/periods", a.access, nil); r.Code != http.StatusOK || len(r.Body["periods"].([]any)) != 4 {
		t.Fatalf("periods: %d %s", r.Code, r.Raw)
	} else if w := r.Body["periods"].([]any)[1].(map[string]any); w["draw_type"] != "weekly" || w["start"] == nil || w["drawn_on"] == nil {
		t.Fatalf("weekly period: %v", w)
	}
	if r := e.do(t, http.MethodGet, "/v1/draws/periods", c.access, nil); r.Code != http.StatusOK || len(r.Body["periods"].([]any)) != 0 {
		t.Fatalf("periods before playing: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/draws/periods", "", nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("periods without a token: %d", r.Code)
	}
	if one := e.do(t, http.MethodGet, "/v1/draws?limit=1", "", nil); len(one.Body["draws"].([]any)) != 1 {
		t.Fatalf("limit 1: %s", one.Raw)
	}
	d := e.do(t, http.MethodGet, "/v1/draws/"+id, "", nil)
	if d.Code != http.StatusOK || d.Body["status"] != "conducted" || len(d.Body["certificate_hash"].(string)) != 64 ||
		d.Body["beacon"] == nil || len(d.Body["signoffs"].([]any)) != 2 || len(d.Body["results"].([]any)) != 2 {
		t.Fatalf("draw: %d %s", d.Code, d.Raw)
	}
	// officials' names stay internal; the public sees winners by entry number only
	if strings.Contains(d.Raw, "operator:") || strings.Contains(d.Raw, "user_id") {
		t.Fatalf("draw exposes names or user ids: %s", d.Raw)
	}

	cert := e.do(t, http.MethodGet, "/v1/draws/"+id+"/certificate", "", nil)
	sum := sha256.Sum256([]byte(cert.Raw))
	if cert.Code != http.StatusOK || hex.EncodeToString(sum[:]) != d.Body["certificate_hash"] || cert.Header.Get("X-Certificate-SHA256") != d.Body["certificate_hash"] {
		t.Fatalf("certificate: %d %s", cert.Code, cert.Raw)
	}
	if strings.Contains(cert.Raw, "user_id") {
		t.Fatalf("the certificate names its winners: %s", cert.Raw)
	}

	entries := 0
	prizes := 0
	for _, s := range []session{a, b} {
		mine := e.do(t, http.MethodGet, "/v1/draws/"+id+"/entries/mine", s.access, nil)
		got := mine.Body["entries"].([]any)
		if mine.Code != http.StatusOK || mine.Body["user_id"] != e.userID(t, s) || len(got) == 0 {
			t.Fatalf("mine: %d %s", mine.Code, mine.Raw)
		}
		entries += len(got)
		prizes += len(mine.Body["prizes"].([]any))
	}
	if entries != int(d.Body["entries"].(float64)) || prizes != 2 {
		t.Fatalf("entries %d of %v, prizes %d", entries, d.Body["entries"], prizes)
	}
	// a player who did not play sees nothing of the others'
	if none := e.do(t, http.MethodGet, "/v1/draws/"+id+"/entries/mine", c.access, nil); none.Code != http.StatusOK ||
		len(none.Body["entries"].([]any)) != 0 || len(none.Body["prizes"].([]any)) != 0 {
		t.Fatalf("other player: %d %s", none.Code, none.Raw)
	}
}

func TestAPI_DrawErrorMapping(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	e.canPlay(t, s, 1000)
	if r := e.do(t, http.MethodPost, "/v1/plays", s.access, nil); r.Code != http.StatusCreated {
		t.Fatalf("play: %d %s", r.Code, r.Raw)
	}
	e.conductDaily(t) // the other periods stay open
	var open string
	if err := e.pool.QueryRow(context.Background(), `SELECT id::text FROM draws.draws WHERE status = 'open' LIMIT 1`).Scan(&open); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, token string
		code        int
		slug        string
	}{
		{"/v1/draws?limit=0", "", http.StatusBadRequest, "invalid-limit"},
		{"/v1/draws?limit=101", "", http.StatusBadRequest, "invalid-limit"},
		{"/v1/draws/0192f000-0000-7000-8000-000000000000", "", http.StatusNotFound, "not-found"},
		{"/v1/draws/not-a-uuid", "", http.StatusNotFound, "not-found"},
		{"/v1/draws/" + open + "/certificate", "", http.StatusConflict, "not-conducted"},
		{"/v1/draws/0192f000-0000-7000-8000-000000000000/certificate", "", http.StatusNotFound, "not-found"},
		{"/v1/draws/0192f000-0000-7000-8000-000000000000/entries/mine", s.access, http.StatusNotFound, "not-found"},
		{"/v1/draws/" + open + "/entries/mine", "", http.StatusUnauthorized, "unauthenticated"},
	} {
		if r := e.do(t, http.MethodGet, tc.path, tc.token, nil); r.Code != tc.code || !hasType(r, tc.slug) {
			t.Errorf("%s: %d %s, want %d %s", tc.path, r.Code, r.Raw, tc.code, tc.slug)
		}
	}
	if r := e.do(t, http.MethodGet, "/v1/draws/"+open, "", nil); r.Code != http.StatusOK || r.Body["beacon"] != nil || r.Body["drand_round"] != nil {
		t.Fatalf("open draw: %d %s", r.Code, r.Raw)
	}
}
