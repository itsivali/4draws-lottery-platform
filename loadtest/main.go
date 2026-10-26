// Command loadtest drives the player journey against a running stack (make dev) with many players at once and then
// checks that the money still adds up. It is for local and staging stacks only: it signs up players, verifies them
// through the stack's KYC mock and tops them up through the Daraja mock.
//
//	go run ./loadtest -players 50 -concurrency 25
//
// Per player: sign up, submit KYC (the mock verifies it), top up KES 500 (the mock approves the prompt), then try six
// plays at once, one of them sent twice with the same idempotency key. The API must allow exactly four plays a day,
// charge a replayed key once, and leave each wallet at what was credited minus the plays.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type client struct {
	base string
	hc   *http.Client
	ip   string // sent as X-Forwarded-For with -spoof-ip (the stack must trust it: loadtest/compose.loadtest.yml)
}

type resp struct {
	code int
	body map[string]any
}

func (c client) do(ctx context.Context, method, path, token, key string, body any) (resp, time.Duration, error) {
	var rd io.Reader = http.NoBody
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return resp{}, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if c.ip != "" {
		req.Header.Set("X-Forwarded-For", c.ip)
	}
	start := time.Now() //nolint:forbidigo // measuring request latency, not business time
	r, err := c.hc.Do(req)
	took := time.Since(start)
	if err != nil {
		return resp{}, took, err
	}
	defer func() { _ = r.Body.Close() }()
	out := resp{code: r.StatusCode}
	_ = json.NewDecoder(r.Body).Decode(&out.body)
	return out, took, nil
}

func randomKey() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "lt-" + hex.EncodeToString(b)
}

func randomMSISDN() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(10_000_000))
	s := strconv.FormatInt(n.Int64(), 10)
	return "071" + strings.Repeat("0", 7-len(s)) + s
}

type stats struct {
	mu      sync.Mutex
	latency map[string][]time.Duration
	codes   map[string]map[int]int
}

func (s *stats) add(op string, code int, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latency[op] = append(s.latency[op], d)
	if s.codes[op] == nil {
		s.codes[op] = map[int]int{}
	}
	s.codes[op][code]++
}

type outcome struct {
	plays      int
	charged    int64
	credited   int64
	balance    int64
	violations []string
}

func poll(ctx context.Context, every time.Duration, max int, check func() (bool, error)) error {
	for range max {
		ok, err := check()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(every):
		}
	}
	return errors.New("timed out")
}

func journey(ctx context.Context, c client, st *stats) (outcome, error) {
	var o outcome
	r, d, err := c.do(ctx, http.MethodPost, "/v1/auth/signup", "", randomKey(), map[string]any{
		"msisdn": randomMSISDN(), "password": "correct horse battery staple", "device": map[string]any{"platform": "web"}})
	if err != nil {
		return o, fmt.Errorf("signup: %w", err)
	}
	if r.code != http.StatusCreated {
		return o, errors.New("signup: HTTP " + strconv.Itoa(r.code))
	}
	st.add("signup", r.code, d)
	tok, _ := r.body["access_token"].(string)

	id, _ := rand.Int(rand.Reader, big.NewInt(90_000_000))
	r, d, err = c.do(ctx, http.MethodPost, "/v1/kyc", tok, randomKey(), map[string]any{
		"national_id": strconv.FormatInt(10_000_000+id.Int64(), 10), "full_name": "Load Test Player", "date_of_birth": "1990-01-01"})
	if err != nil {
		return o, fmt.Errorf("kyc: %w", err)
	}
	if r.code != http.StatusAccepted {
		return o, errors.New("kyc: HTTP " + strconv.Itoa(r.code))
	}
	st.add("kyc", r.code, d)
	if err := poll(ctx, 300*time.Millisecond, 100, func() (bool, error) {
		r, _, err := c.do(ctx, http.MethodGet, "/v1/kyc", tok, "", nil)
		return err == nil && r.body["status"] == "verified", err
	}); err != nil {
		return o, fmt.Errorf("kyc never verified: %w", err)
	}

	r, d, err = c.do(ctx, http.MethodPost, "/v1/payments/topups", tok, randomKey(), map[string]any{"amount_kes": 500})
	if err != nil {
		return o, fmt.Errorf("top-up: %w", err)
	}
	if r.code != http.StatusAccepted {
		return o, errors.New("top-up: HTTP " + strconv.Itoa(r.code))
	}
	st.add("topup", r.code, d)
	payment, _ := r.body["id"].(string)
	if err := poll(ctx, 500*time.Millisecond, 120, func() (bool, error) {
		r, _, err := c.do(ctx, http.MethodGet, "/v1/payments/"+payment, tok, "", nil)
		return err == nil && r.body["status"] == "credited", err
	}); err != nil {
		return o, fmt.Errorf("top-up never credited: %w", err)
	}
	r, _, _ = c.do(ctx, http.MethodGet, "/v1/wallet", tok, "", nil)
	start, _ := r.body["balance_kes"].(float64)
	o.credited = int64(start)

	// six plays at once, one key used twice: exactly four may succeed, the replay charges nothing more
	keys := []string{randomKey(), randomKey(), randomKey(), randomKey(), randomKey(), randomKey()}
	keys = append(keys, keys[0])
	var mu sync.Mutex
	tickets := map[string]bool{}
	var wg sync.WaitGroup
	for _, k := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, d, err := c.do(ctx, http.MethodPost, "/v1/plays", tok, k, nil)
			if err != nil {
				return
			}
			st.add("play", r.code, d)
			if r.code == http.StatusOK || r.code == http.StatusCreated {
				play, _ := r.body["play"].(map[string]any)
				t, _ := play["ticket_id"].(string)
				mu.Lock()
				tickets[t] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	o.plays = len(tickets)

	r, _, _ = c.do(ctx, http.MethodGet, "/v1/plays/today", tok, "", nil)
	if p, _ := r.body["plays"].(float64); int(p) != o.plays {
		o.violations = append(o.violations, "today says "+strconv.Itoa(int(p))+" plays, "+strconv.Itoa(o.plays)+" tickets seen")
	}
	price, _ := r.body["price_kes"].(float64)
	o.charged = int64(o.plays) * int64(price)
	r, _, _ = c.do(ctx, http.MethodGet, "/v1/wallet", tok, "", nil)
	bal, _ := r.body["balance_kes"].(float64)
	o.balance = int64(bal)
	if o.plays > 4 {
		o.violations = append(o.violations, strconv.Itoa(o.plays)+" plays in one day")
	}
	if o.balance != o.credited-o.charged {
		o.violations = append(o.violations, "wallet "+strconv.FormatInt(o.balance, 10)+", want "+strconv.FormatInt(o.credited, 10)+" - "+strconv.FormatInt(o.charged, 10))
	}
	return o, nil
}

func pct(ds []time.Duration, p float64) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	s := slices.Clone(ds)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[int(float64(len(s)-1)*p)]
}

func main() {
	base := flag.String("base", "http://localhost:8080", "API base URL (a local or staging stack, never production)")
	players := flag.Int("players", 50, "players to run")
	concurrency := flag.Int("concurrency", 25, "players at once")
	spoof := flag.Bool("spoof-ip", false, "give each player its own client address (X-Forwarded-For); needs loadtest/compose.loadtest.yml")
	flag.Parse()

	ctx := context.Background()
	c := client{base: *base, hc: &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 200}}}
	st := &stats{latency: map[string][]time.Duration{}, codes: map[string]map[int]int{}}
	sem := make(chan struct{}, *concurrency)
	var mu sync.Mutex
	var failures, violations []string
	var plays int
	var wg sync.WaitGroup
	began := time.Now() //nolint:forbidigo // measuring the run's duration
	for range *players {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			pc := c
			if *spoof {
				b := make([]byte, 3)
				_, _ = rand.Read(b)
				pc.ip = "10." + strconv.Itoa(int(b[0])) + "." + strconv.Itoa(int(b[1])) + "." + strconv.Itoa(int(b[2]))
			}
			o, err := journey(ctx, pc, st)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, err.Error())
				return
			}
			plays += o.plays
			violations = append(violations, o.violations...)
		}()
	}
	wg.Wait()
	took := time.Since(began)

	fmt.Printf("players %d (concurrency %d) in %s; journeys failed %d; plays %d\n", *players, *concurrency, took.Round(time.Millisecond), len(failures), plays)
	for _, op := range []string{"signup", "kyc", "topup", "play"} {
		ds := st.latency[op]
		fmt.Printf("%-7s n=%-4d p50 %-8s p95 %-8s p99 %-8s codes %v\n", op, len(ds), pct(ds, .5).Round(time.Millisecond), pct(ds, .95).Round(time.Millisecond), pct(ds, .99).Round(time.Millisecond), st.codes[op])
	}
	for _, f := range failures[:min(len(failures), 5)] {
		fmt.Println("failure:", f)
	}
	for _, v := range violations {
		fmt.Println("INVARIANT VIOLATED:", v)
	}
	if len(violations) > 0 || len(failures) > 0 {
		os.Exit(1)
	}
	fmt.Println("invariants hold: at most 4 plays a day, replays charged once, every wallet = credited - plays")
}
