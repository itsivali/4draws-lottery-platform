package draws_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/draws/drand"
	"github.com/itsivali/lottery-platform/internal/ledger"
)

var update = flag.Bool("update", false, "rewrite the golden certificate in testdata")

// goldenEntry is one line of an entry list export (lotteryctl draws entries), as tools/verifier reads it.
type goldenEntry struct {
	EntryNo  int    `json:"entry_no"`
	UserID   string `json:"user_id"`
	PlayDate string `json:"play_date"`
	TicketID string `json:"ticket_id"`
}

// golden builds a certificate the way Conduct does, from a real recorded quicknet beacon, a fixed company seed and
// an entry list where one player holds most entries (so picks of a player who already won are skipped).
func golden(t *testing.T) ([]byte, []byte) {
	t.Helper()
	raw, err := os.ReadFile("drand/testdata/round-1000000.json")
	if err != nil {
		t.Fatal(err)
	}
	var b drand.Beacon
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	if err := drand.Quicknet.Verify(b); err != nil {
		t.Fatalf("recorded beacon: %v", err)
	}
	const drawID = "0192f3a0-0000-7000-8000-000000000001"
	players := []string{"0192f3a0-0000-7000-8000-00000000000a", "0192f3a0-0000-7000-8000-00000000000b",
		"0192f3a0-0000-7000-8000-00000000000c", "0192f3a0-0000-7000-8000-00000000000d"}
	holdings := []int{10, 2, 1, 1}
	var entries []goldenEntry
	var lines bytes.Buffer
	h := sha256.New()
	for p, n := range holdings {
		for range n {
			e := goldenEntry{EntryNo: len(entries) + 1, UserID: players[p], PlayDate: "2026-10-24",
				TicketID: fmt.Sprintf("0192f3a0-0000-7000-9000-%012d", len(entries)+1)}
			entries = append(entries, e)
			_, _ = fmt.Fprintf(h, "%d|%s|%s|%s\n", e.EntryNo, e.UserID, e.PlayDate, e.TicketID)
			line, _ := json.Marshal(e)
			lines.Write(append(line, '\n'))
		}
	}
	seed := sha256.Sum256([]byte("golden company seed"))
	commit := sha256.Sum256(seed[:])
	stakes := int64(1_400)
	rate, _ := ledger.ParseRate("0.30000")
	pool, _ := ledger.Apply(stakes, rate, ledger.Down)
	cert := draws.Certificate{DrawID: drawID, DrawType: "daily", PeriodStart: "2026-10-24", PeriodEnd: "2026-10-24", Entries: len(entries),
		EntryListHash: hex.EncodeToString(h.Sum(nil)), SeedCommit: hex.EncodeToString(commit[:]), RevealedSeed: hex.EncodeToString(seed[:]),
		Beacon: b, ChainHash: drand.Quicknet.Hash, StakesKES: stakes, PoolShare: "0.30000", PoolKES: pool, Rounding: "down", Algorithm: draws.Algorithm}
	users := make([]string, len(entries))
	for i, e := range entries {
		users[i] = e.UserID
	}
	randomness, _ := hex.DecodeString(b.Randomness)
	ranks := []draws.RankShare{{Rank: 1, Share: "0.50"}, {Rank: 2, Share: "0.30"}, {Rank: 3, Share: "0.20"}}
	cert.Prizes, err = draws.Award(draws.Key(randomness, seed[:], cert.EntryListHash, drawID), users, ranks, pool, ledger.Down)
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(cert)
	if err != nil {
		t.Fatal(err)
	}
	return out, lines.Bytes()
}

// The golden certificate is shared with tools/verifier, which must reproduce it without the server's code.
func TestGoldenCertificate_IsReproducibleAndSkipsRepeatWinners(t *testing.T) {
	cert, entries := golden(t)
	if *update {
		if err := os.WriteFile("testdata/certificate.json", cert, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("testdata/entries.jsonl", entries, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	wantCert, err := os.ReadFile("testdata/certificate.json")
	if err != nil {
		t.Fatal(err)
	}
	wantEntries, err := os.ReadFile("testdata/entries.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cert, wantCert) || !bytes.Equal(entries, wantEntries) {
		t.Fatal("the golden certificate changed: the draw algorithm or the certificate format moved (run with -update only if intended)")
	}
	var c draws.Certificate
	if err := json.Unmarshal(cert, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Prizes) != 3 || c.PoolKES != 420 || c.Prizes[0].AmountKES != 210 || c.Prizes[1].AmountKES != 126 || c.Prizes[2].AmountKES != 84 {
		t.Fatalf("prizes %+v", c.Prizes)
	}
	// the fixture is only useful if the selection had to skip a repeat winner
	seed, _ := hex.DecodeString(c.RevealedSeed)
	randomness, _ := hex.DecodeString(c.Beacon.Randomness)
	key := draws.Key(randomness, seed, c.EntryListHash, c.DrawID)
	picks := 0
	for i := 0; draws.Pick(key, i, c.Entries) != c.Prizes[2].EntryNo; i++ {
		picks++
	}
	if picks < 3 {
		t.Fatalf("no pick was skipped (%d picks before the last winner): choose another seed", picks)
	}
}
