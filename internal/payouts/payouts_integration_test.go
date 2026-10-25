//go:build integration

package payouts_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/payments/daraja"
	"github.com/itsivali/lottery-platform/internal/payments/darajamock"
	"github.com/itsivali/lottery-platform/internal/payouts"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/testpg"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

// queue records the jobs the service enqueues so a test runs them when it chooses.
type queue struct {
	mu   sync.Mutex
	jobs []job
}

type job struct{ kind, id string }

func (q *queue) EnqueueSend(_ context.Context, _ pgx.Tx, id string) error { return q.add("send", id) }
func (q *queue) EnqueueVerify(_ context.Context, _ pgx.Tx, id string, _ payouts.Source) error {
	return q.add("verify", id)
}

func (q *queue) add(kind, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, job{kind, id})
	return nil
}

func (q *queue) take() []job {
	q.mu.Lock()
	defer q.mu.Unlock()
	j := q.jobs
	q.jobs = nil
	return j
}

type env struct {
	pool  *pgxpool.Pool // the worker's role
	api   *pgxpool.Pool // the api's role, on the same database
	clk   *clock.Manual
	mock  *darajamock.Server
	q     *queue
	svc   *payouts.Service // runs as the worker
	hooks *payouts.Service // runs as the api: receives Daraja's posts
}

var start = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) *env { return setupWith(t, true, payouts.Config{MaxStatusRequests: 3}) }

func setupWith(t *testing.T, withDaraja bool, cfg payouts.Config) *env {
	t.Helper()
	pool := testpg.Pool(t, "app_worker")
	apiCfg := pool.Config().Copy()
	apiCfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE app_api")
		return err
	}
	api, err := pgxpool.NewWithConfig(context.Background(), apiCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(api.Close)
	e := &env{pool: pool, api: api, clk: clock.NewManual(start), q: &queue{}}
	aw := audit.NewWriter()
	ls := ledger.NewStore()
	build := func(p *pgxpool.Pool, d payouts.Daraja) *payouts.Service {
		ids := identity.NewService(p, e.clk, nil, aw, identity.Config{AuthKey: []byte("0123456789abcdef0123456789abcdef")})
		return payouts.NewService(p, e.clk, d, e.q, ids, wallet.NewService(ls), ls, aw, cfg, wallet.ErrInsufficientFunds)
	}
	if !withDaraja {
		e.svc, e.hooks = build(pool, nil), build(api, nil)
		return e
	}
	// plays the api's payout hooks
	hooks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		kind := map[string]string{"/result": payouts.CallbackResult, "/timeout": payouts.CallbackTimeout,
			"/status-result": payouts.CallbackStatusResult, "/status-timeout": payouts.CallbackStatusTimeout}[r.URL.Path]
		if err := e.hooks.HandleCallback(r.Context(), kind, raw); err != nil {
			t.Errorf("%s: %v", r.URL.Path, err)
		}
	}))
	t.Cleanup(hooks.Close)
	e.mock = darajamock.New(darajamock.Options{ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk",
		Initiator: "testapi", SecurityCredential: "cred", B2CShortCode: "600992", HTTPClient: hooks.Client()})
	ms := httptest.NewServer(e.mock)
	t.Cleanup(ms.Close)
	c, err := daraja.New(daraja.Config{BaseURL: ms.URL, ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk",
		CallbackURL: hooks.URL + "/stk"}, ms.Client(), e.clk)
	if err != nil {
		t.Fatal(err)
	}
	if c, err = c.WithB2C(daraja.B2CConfig{ShortCode: "600992", Initiator: "testapi", SecurityCredential: "cred", ResultURL: hooks.URL + "/result",
		TimeoutURL: hooks.URL + "/timeout", StatusResultURL: hooks.URL + "/status-result", StatusTimeoutURL: hooks.URL + "/status-timeout"}); err != nil {
		t.Fatal(err)
	}
	e.svc, e.hooks = build(pool, c), build(api, c)
	return e
}

var seq atomic.Int64

// player creates a verified player with a proved phone and a wallet credited with gross net of the 5% excise.
func (e *env) player(t *testing.T, gross int64) string {
	t.Helper()
	ctx := context.Background()
	n := seq.Add(1)
	var user string
	if err := e.pool.QueryRow(ctx, `INSERT INTO identity.users (msisdn, kyc_status, national_id_hash, password_hash, msisdn_verified_at)
		VALUES ($1, 'verified', $2, 'x', now()) RETURNING id::text`, fmt.Sprintf("2547%08d", 60_000_000+n), []byte(fmt.Sprint("payout-id-", n))).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if gross > 0 {
		e.credit(t, user, gross)
	}
	return user
}

func (e *env) credit(t *testing.T, user string, gross int64) string {
	t.Helper()
	ctx := context.Background()
	ls := ledger.NewStore()
	var paymentID string
	err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT platform.uuid_v7()::text`).Scan(&paymentID); err != nil {
			return err
		}
		rule, err := ls.ActiveRule(ctx, tx, "deposit_excise", start)
		if err != nil {
			return err
		}
		p, err := ledger.Deposit(paymentID, user, gross, rule, true)
		if err != nil {
			return err
		}
		_, err = wallet.NewService(ls).Apply(ctx, tx, p)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return paymentID
}

func (e *env) balance(t *testing.T, user string) int64 {
	t.Helper()
	var b int64
	if err := e.pool.QueryRow(context.Background(), `SELECT coalesce((SELECT balance_kes FROM wallet.wallets WHERE user_id = $1), 0)`, user).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func (e *env) state(t *testing.T, id string) (string, string, string) {
	t.Helper()
	var st string
	var receipt, reason *string
	if err := e.pool.QueryRow(context.Background(), `SELECT status, mpesa_receipt, reason FROM payouts.payouts WHERE id = $1`, id).Scan(&st, &receipt, &reason); err != nil {
		t.Fatal(err)
	}
	return st, deref(receipt), deref(reason)
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// run works the queue and the payout's verification the way the worker would, moving the clock past every wait, until
// the payout is final or with an operator. It returns the final state.
func (e *env) run(t *testing.T, id string) string {
	t.Helper()
	ctx := context.Background()
	for range 40 {
		e.mock.Flush()
		for _, j := range e.q.take() {
			switch j.kind {
			case "send":
				if err := e.svc.Send(ctx, j.id); err != nil {
					t.Fatalf("send: %v", err)
				}
			case "verify":
				if _, err := e.svc.Verify(ctx, j.id); err != nil {
					t.Fatalf("verify: %v", err)
				}
			}
		}
		e.mock.Flush()
		st, _, _ := e.state(t, id)
		switch st {
		case "succeeded", "failed", "manual", "held":
			return st
		}
		wait, err := e.svc.Verify(ctx, id)
		if err != nil {
			t.Fatalf("verify: %v", err)
		}
		e.clk.Advance(wait + time.Second)
	}
	t.Fatal("the payout never settled")
	return ""
}

func (e *env) withdraw(t *testing.T, user string, amount int64, key string) payouts.Payout {
	t.Helper()
	p, _, err := e.svc.RequestWithdrawal(context.Background(), user, amount, key)
	if err != nil {
		t.Fatalf("withdraw %d: %v", amount, err)
	}
	return p
}

func TestWithdrawal_PaidAndConfirmedExactlyOnce(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 1000) // 950 after excise
	p := e.withdraw(t, u, 500, "w-1")
	if p.Status != payouts.StatusProcessing || p.State != "pending" || p.GrossKES != 500 || p.TaxKES != 0 || p.NetKES != 500 || e.balance(t, u) != 450 {
		t.Fatalf("requested %+v, balance %d", p, e.balance(t, u))
	}
	// the same key is the same payout, debited once
	again, replayed, err := e.svc.RequestWithdrawal(ctx, u, 500, "w-1")
	if err != nil || !replayed || again.ID != p.ID || e.balance(t, u) != 450 {
		t.Fatalf("replay: %+v %v %v", again, replayed, err)
	}
	if st := e.run(t, p.ID); st != "succeeded" {
		t.Fatalf("state %s", st)
	}
	st, receipt, _ := e.state(t, p.ID)
	sent := e.mock.Payouts()
	if st != "succeeded" || len(sent) != 1 || receipt != sent[0].Receipt || sent[0].AmountKES != 500 || sent[0].CommandID != "BusinessPayment" ||
		e.balance(t, u) != 450 {
		t.Fatalf("settled %s %s %+v balance %d", st, receipt, sent, e.balance(t, u))
	}
	got, err := e.svc.Get(ctx, u, p.ID)
	if err != nil || got.Status != payouts.StatusPaid || got.Receipt != receipt {
		t.Fatalf("player view %+v %v", got, err)
	}
	if _, err := e.svc.Get(ctx, e.player(t, 0), p.ID); !errors.Is(err, payouts.ErrNotFound) {
		t.Fatalf("another player saw it: %v", err)
	}
	// a late duplicate verification changes nothing
	if _, err := e.svc.Verify(ctx, p.ID); err != nil || e.balance(t, u) != 450 {
		t.Fatalf("late verify: %v", err)
	}
}

func TestWithdrawal_FaultMatrix(t *testing.T) {
	claimSuccess := 0
	for _, tc := range []struct {
		name          string
		fault         darajamock.B2CFault
		state, reason string
		refunded      bool
		paid          bool
	}{
		{"refused by daraja", darajamock.B2CFault{RequestHTTP: 400}, "failed", "b2c_refused", true, false},
		{"paid but the answer was lost", darajamock.B2CFault{LostAnswer: true}, "succeeded", "b2c_outcome_unknown", false, true},
		{"failure result", darajamock.B2CFault{ResultCode: 2001}, "failed", "b2c_failed_2001", true, false},
		{"queue timeout", darajamock.B2CFault{ResultTimeout: true}, "failed", "b2c_queue_timeout", true, false},
		{"a lying success", darajamock.B2CFault{ResultCode: 1, ClaimResultCode: &claimSuccess}, "manual", "success_not_confirmed", false, false},
		{"paid, result dropped", darajamock.B2CFault{DropResult: true}, "succeeded", "", false, true},
		{"paid, result twice", darajamock.B2CFault{DuplicateResult: true}, "succeeded", "", false, true},
		{"status reports another amount", darajamock.B2CFault{StatusAmountKES: 499}, "manual", "status_amount_mismatch", false, true},
		{"outage then nothing", darajamock.B2CFault{RequestHTTP: 503}, "manual", "unconfirmed", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setup(t)
			u := e.player(t, 1000)
			e.mock.SetNextB2CFault(tc.fault)
			p := e.withdraw(t, u, 300, "k")
			st := e.run(t, p.ID)
			_, receipt, reason := e.state(t, p.ID)
			want := int64(650)
			if tc.refunded {
				want = 950
			}
			if st != tc.state || reason != tc.reason || e.balance(t, u) != want || (tc.state == "succeeded") != (receipt != "") {
				t.Fatalf("state %s reason %q receipt %q balance %d", st, reason, receipt, e.balance(t, u))
			}
			paid := false
			for _, m := range e.mock.Payouts() {
				paid = paid || m.Paid
			}
			if paid != tc.paid || len(e.mock.Payouts()) > 1 {
				t.Fatalf("mock payouts %+v", e.mock.Payouts())
			}
		})
	}
}

func TestWithdrawal_StatusOutageIsRetriedNotSettled(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 1000)
	e.mock.SetNextB2CFault(darajamock.B2CFault{DropResult: true, StatusHTTP: 503})
	p := e.withdraw(t, u, 300, "k")
	for _, j := range e.q.take() {
		if err := e.svc.Send(ctx, j.id); err != nil {
			t.Fatal(err)
		}
	}
	e.q.take()
	e.clk.Advance(payouts.DefaultResultWait + time.Second)
	if _, err := e.svc.Verify(ctx, p.ID); !errors.Is(err, daraja.ErrUnavailable) {
		t.Fatalf("status outage: %v", err)
	}
	if st, _, _ := e.state(t, p.ID); st != "sent" || e.balance(t, u) != 650 {
		t.Fatalf("state %s balance %d", st, e.balance(t, u))
	}
}

func TestWithdrawal_AnInterruptedSendIsCheckedNeverSentAgain(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 1000)
	p := e.withdraw(t, u, 300, "k")
	e.q.take() // the send job is lost
	// the worker marked it sending and stopped before Daraja answered
	if _, err := e.pool.Exec(ctx, `UPDATE payouts.payouts SET status = 'sending' WHERE id = $1`, p.ID); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(payouts.DefaultStaleAfter + time.Minute)
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _, reason := e.state(t, p.ID); st != "timed_out" || reason != "send_interrupted" {
		t.Fatalf("after sweep: %s %s", st, reason)
	}
	if st := e.run(t, p.ID); st != "manual" || len(e.mock.Payouts()) != 0 {
		t.Fatalf("state %s, sent %d times", st, len(e.mock.Payouts()))
	}
	// an operator finds no payment on the statement: the money comes back
	if _, err := e.svc.Settle(ctx, p.ID, "operator:alice", false, "", "not on the statement"); err != nil || e.balance(t, u) != 950 {
		t.Fatalf("settle: %v balance %d", err, e.balance(t, u))
	}
}

func TestWithdrawal_AStillPendingPayoutIsQueuedAgain(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 1000)
	p := e.withdraw(t, u, 300, "k")
	e.q.take() // the send job is lost before it ran
	e.clk.Advance(payouts.DefaultStaleAfter + time.Minute)
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if st := e.run(t, p.ID); st != "succeeded" || len(e.mock.Payouts()) != 1 {
		t.Fatalf("state %s sent %d", st, len(e.mock.Payouts()))
	}
}

func TestWithdrawal_EligibilityAndLimits(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	rich := e.player(t, 10_000)
	for _, amount := range []int64{49, 150_001} {
		if _, _, err := e.svc.RequestWithdrawal(ctx, rich, amount, fmt.Sprint("b", amount)); !errors.Is(err, payouts.ErrInvalidAmount) {
			t.Errorf("amount %d: %v", amount, err)
		}
	}
	poor := e.player(t, 100) // 95
	if _, _, err := e.svc.RequestWithdrawal(ctx, poor, 100, "x"); !errors.Is(err, payouts.ErrInsufficientFunds) {
		t.Errorf("insufficient: %v", err)
	}
	unproved := e.player(t, 1000)
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET msisdn_verified_at = NULL WHERE id = $1`, unproved); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.RequestWithdrawal(ctx, unproved, 100, "x"); !errors.Is(err, identity.ErrPhoneNotVerified) {
		t.Errorf("unproved phone: %v", err)
	}
	unverified := e.player(t, 1000)
	if _, err := e.pool.Exec(ctx, `UPDATE identity.users SET kyc_status = 'manual_review' WHERE id = $1`, unverified); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.RequestWithdrawal(ctx, unverified, 100, "x"); !errors.Is(err, identity.ErrKYCRequired) {
		t.Errorf("unverified: %v", err)
	}
	// a debtor: spent most of a deposit that was then reversed; what is missing is owed (ADR 0007)
	debtor := e.player(t, 0)
	pid := e.credit(t, debtor, 1000)
	ls := ledger.NewStore()
	w := wallet.NewService(ls)
	if err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		var ticket string
		if err := tx.QueryRow(ctx, `SELECT platform.uuid_v7()::text`).Scan(&ticket); err != nil {
			return err
		}
		stake, err := ledger.Stake(ticket, debtor, 900)
		if err != nil {
			return err
		}
		if _, err := w.Apply(ctx, tx, stake); err != nil {
			return err
		}
		_, _, err = w.ReverseDeposit(ctx, tx, pid, debtor, 1000, 50)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.credit(t, debtor, 1000) // a later deposit that has not repaid the debt
	if _, _, err := e.svc.RequestWithdrawal(ctx, debtor, 100, "x"); !errors.Is(err, payouts.ErrInDebt) {
		t.Errorf("debtor: %v", err)
	}
	// three withdrawals a Nairobi day
	for i := range 3 {
		e.withdraw(t, rich, 100, fmt.Sprint("d", i))
	}
	if _, _, err := e.svc.RequestWithdrawal(ctx, rich, 100, "d3"); !errors.Is(err, payouts.ErrDailyLimit) {
		t.Errorf("fourth today: %v", err)
	}
	e.clk.Advance(24 * time.Hour)
	e.withdraw(t, rich, 100, "next-day")
}

func TestWithdrawal_RefusedWithoutB2C(t *testing.T) {
	e := setupWith(t, false, payouts.Config{})
	u := e.player(t, 1000)
	if _, _, err := e.svc.RequestWithdrawal(context.Background(), u, 100, "k"); !errors.Is(err, payouts.ErrUnavailable) || e.balance(t, u) != 950 {
		t.Fatalf("%v balance %d", err, e.balance(t, u))
	}
}

func TestWithdrawal_LargeAmountsWaitForAnOperator(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 250_000) // 237,500
	held := e.withdraw(t, u, 120_000, "big")
	if held.State != "held" || len(e.q.take()) != 0 || e.balance(t, u) != 117_500 {
		t.Fatalf("held %+v", held)
	}
	if _, err := e.svc.Approve(ctx, held.ID, ""); !errors.Is(err, payouts.ErrOperator) {
		t.Fatalf("approve without operator: %v", err)
	}
	if _, err := e.svc.Settle(ctx, held.ID, "operator:alice", true, "MBC0000009", ""); !errors.Is(err, payouts.ErrState) {
		t.Fatalf("settle a held payout: %v", err)
	}
	if _, err := e.svc.Approve(ctx, held.ID, "operator:alice"); err != nil {
		t.Fatal(err)
	}
	if st := e.run(t, held.ID); st != "succeeded" {
		t.Fatalf("approved: %s", st)
	}
	rejected := e.withdraw(t, u, 110_000, "big-2")
	if _, err := e.svc.Reject(ctx, rejected.ID, "operator:bob", "unusual activity"); err != nil || e.balance(t, u) != 117_500 {
		t.Fatalf("reject: %v balance %d", err, e.balance(t, u))
	}
	if st, _, reason := e.state(t, rejected.ID); st != "failed" || reason != "rejected: unusual activity" {
		t.Fatalf("rejected: %s %s", st, reason)
	}
	held2, _ := e.svc.InState(ctx, "held", 10)
	if len(held2) != 0 {
		t.Fatalf("still held %+v", held2)
	}
}

func TestWithdrawal_OperatorSettlesAManualPayoutFromTheStatement(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 1000)
	claim := 0
	e.mock.SetNextB2CFault(darajamock.B2CFault{ResultCode: 1, ClaimResultCode: &claim})
	p := e.withdraw(t, u, 300, "k")
	if st := e.run(t, p.ID); st != "manual" {
		t.Fatalf("state %s", st)
	}
	if list, _ := e.svc.InState(ctx, "manual", 10); len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("manual list %+v", list)
	}
	if _, err := e.svc.Settle(ctx, p.ID, "operator:alice", true, "", ""); !errors.Is(err, payouts.ErrReceiptRequired) {
		t.Fatalf("no receipt: %v", err)
	}
	if _, err := e.svc.Settle(ctx, p.ID, "operator:alice", true, "MBCSTMT001", ""); err != nil {
		t.Fatal(err)
	}
	if st, receipt, _ := e.state(t, p.ID); st != "succeeded" || receipt != "MBCSTMT001" || e.balance(t, u) != 650 {
		t.Fatalf("settled %s %s balance %d", st, receipt, e.balance(t, u))
	}
	var by string
	if err := e.pool.QueryRow(ctx, `SELECT settled_by FROM payouts.payouts WHERE id = $1`, p.ID).Scan(&by); err != nil || by != "operator:alice" {
		t.Fatalf("settled by %q %v", by, err)
	}
	if _, err := e.svc.Settle(ctx, p.ID, "operator:alice", false, "", "changed my mind"); !errors.Is(err, payouts.ErrState) {
		t.Fatalf("a final payout was settled again: %v", err)
	}
}

func TestWithdrawal_ConcurrentRequestsNeverOverdraw(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 1000) // 950: two withdrawals of 400 fit, not three
	var wg sync.WaitGroup
	var ok, poor atomic.Int32
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprint("c", i%3) // pairs share a key
			_, _, err := e.svc.RequestWithdrawal(ctx, u, 400, key)
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, payouts.ErrInsufficientFunds):
				poor.Add(1)
			default:
				t.Errorf("%v", err)
			}
		}()
	}
	wg.Wait()
	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM payouts.payouts WHERE user_id = $1`, u).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 || e.balance(t, u) != 150 || ok.Load() != 4 || poor.Load() != 2 {
		t.Fatalf("payouts %d balance %d ok %d poor %d", n, e.balance(t, u), ok.Load(), poor.Load())
	}
}

func TestPayouts_TheDatabaseGuardsPayouts(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 1000)
	p := e.withdraw(t, u, 300, "k")
	for name, sql := range map[string]string{
		"amount changed":     `UPDATE payouts.payouts SET net_kes = 1, gross_kes = 1 WHERE id = $1`,
		"payee changed":      `UPDATE payouts.payouts SET msisdn = '254700000001' WHERE id = $1`,
		"skipped to success": `UPDATE payouts.payouts SET status = 'succeeded', mpesa_receipt = 'X', settled_at = now() WHERE id = $1`,
		"sent without send":  `UPDATE payouts.payouts SET status = 'sent' WHERE id = $1`,
		"deleted":            `DELETE FROM payouts.payouts WHERE id = $1`,
	} {
		if _, err := e.pool.Exec(ctx, sql, p.ID); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := e.api.Exec(ctx, `UPDATE payouts.payouts SET reason = 'x' WHERE id = $1`, p.ID); err == nil {
		t.Error("the api role changed a payout")
	}
	e.run(t, p.ID)
	if _, err := e.pool.Exec(ctx, `UPDATE payouts.payouts SET reason = 'later' WHERE id = $1`, p.ID); err == nil {
		t.Error("a final payout changed")
	}
	if _, err := e.pool.Exec(ctx, `DELETE FROM payouts.b2c_callbacks`); err == nil {
		t.Error("callbacks deleted")
	}
}

func TestHandleCallback_KeepsUnknownAndRefusesMalformed(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	unknown := `{"Result":{"ResultType":0,"ResultCode":0,"OriginatorConversationID":"nobody-000000","ConversationID":"AG_X","TransactionID":"MBC9999999",
		"ResultParameters":{"ResultParameter":[{"Key":"TransactionAmount","Value":5},{"Key":"TransactionReceipt","Value":"MBC9999999"}]}}}`
	if err := e.hooks.HandleCallback(ctx, payouts.CallbackResult, []byte(unknown)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM payouts.b2c_callbacks WHERE payout_id IS NULL`).Scan(&n); err != nil || n != 1 || len(e.q.take()) != 0 {
		t.Fatalf("unlinked %d %v", n, err)
	}
	if err := e.hooks.HandleCallback(ctx, payouts.CallbackResult, []byte(`nope`)); !errors.Is(err, daraja.ErrBadCallback) {
		t.Fatalf("malformed: %v", err)
	}
	if err := e.hooks.HandleCallback(ctx, "other", []byte(unknown)); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestHandleCallback_StatusAnswersFindTheirPayoutByOccasionOrOriginator(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	for _, occasion := range []string{"", "not-a-uuid", "0192f000-0000-7000-8000-000000000000"} {
		body := `{"Result":{"ResultType":0,"ResultCode":2001,"ResultDesc":"x","OriginatorConversationID":"ts-unknown","ConversationID":"AG_1",
			"ReferenceData":{"ReferenceItem":{"Key":"Occasion","Value":"` + occasion + `"}}}}`
		if err := e.hooks.HandleCallback(ctx, payouts.CallbackStatusResult, []byte(body)); err != nil {
			t.Fatalf("occasion %q: %v", occasion, err)
		}
	}
	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM payouts.b2c_callbacks WHERE payout_id IS NULL`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("unlinked %d %v", n, err)
	}
}

// fakeDaraja accepts every payout and status request; the test then posts the answers it wants.
type fakeDaraja struct {
	mu        sync.Mutex
	statusErr error
	n         int
}

func (f *fakeDaraja) B2C(_ context.Context, r daraja.B2CRequest) (daraja.B2CAccepted, error) {
	return daraja.B2CAccepted{ConversationID: "AG_" + r.OriginatorConversationID}, nil
}

func (f *fakeDaraja) B2CStatus(_ context.Context, _, _, _ string) (daraja.StatusRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.statusErr != nil {
		return daraja.StatusRequest{}, f.statusErr
	}
	f.n++
	return daraja.StatusRequest{OriginatorConversationID: fmt.Sprint("ts-", f.n)}, nil
}

func setupFake(t *testing.T, d *fakeDaraja) *env {
	t.Helper()
	e := setupWith(t, false, payouts.Config{MaxStatusRequests: 3})
	aw := audit.NewWriter()
	ls := ledger.NewStore()
	ids := identity.NewService(e.pool, e.clk, nil, aw, identity.Config{AuthKey: []byte("0123456789abcdef0123456789abcdef")})
	e.svc = payouts.NewService(e.pool, e.clk, d, e.q, ids, wallet.NewService(ls), ls, aw, payouts.Config{MaxStatusRequests: 3}, wallet.ErrInsufficientFunds)
	e.hooks = e.svc
	return e
}

// sendAndAsk sends the payout and asks Daraja once; it returns the originator of the status request.
func (e *env) sendAndAsk(t *testing.T, id string) string {
	t.Helper()
	ctx := context.Background()
	if err := e.svc.Send(ctx, id); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(payouts.DefaultResultWait + time.Second)
	if _, err := e.svc.Verify(ctx, id); err != nil {
		t.Fatal(err)
	}
	var orig string
	if err := e.pool.QueryRow(ctx, `SELECT status_originator FROM payouts.payouts WHERE id = $1`, id).Scan(&orig); err != nil {
		t.Fatal(err)
	}
	return orig
}

func completed(orig, receipt, credit string, amount int64) []byte {
	return []byte(fmt.Sprintf(`{"Result":{"ResultType":0,"ResultCode":0,"ResultDesc":"ok","OriginatorConversationID":%q,"ConversationID":"AG",
		"ResultParameters":{"ResultParameter":[{"Key":"ReceiptNo","Value":%q},{"Key":"Amount","Value":%d},{"Key":"TransactionStatus","Value":"Completed"},
		{"Key":"CreditPartyName","Value":%q}]}}}`, orig, receipt, amount, credit))
}

func TestVerify_ConfirmationsThatCannotBeTrusted(t *testing.T) {
	ctx := context.Background()
	e := setupFake(t, &fakeDaraja{})
	u := e.player(t, 10_000)
	var msisdn string
	if err := e.pool.QueryRow(ctx, `SELECT msisdn FROM identity.users WHERE id = $1`, u).Scan(&msisdn); err != nil {
		t.Fatal(err)
	}
	settle := func(id string, body []byte) (string, string) {
		t.Helper()
		if err := e.hooks.HandleCallback(ctx, payouts.CallbackStatusResult, body); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Verify(ctx, id); err != nil {
			t.Fatal(err)
		}
		st, _, reason := e.state(t, id)
		return st, reason
	}
	first := e.withdraw(t, u, 100, "a")
	if st, _ := settle(first.ID, completed(e.sendAndAsk(t, first.ID), "MBCSAME001", msisdn+" - Jane", 100)); st != "succeeded" {
		t.Fatalf("first: %s", st)
	}
	// the same receipt cannot pay a second payout
	second := e.withdraw(t, u, 100, "b")
	if st, reason := settle(second.ID, completed(e.sendAndAsk(t, second.ID), "MBCSAME001", msisdn+" - Jane", 100)); st != "manual" || reason != "receipt_reused" {
		t.Fatalf("reused receipt: %s %s", st, reason)
	}
	// money that went to another number is not this payout
	third := e.withdraw(t, u, 100, "c")
	if st, reason := settle(third.ID, completed(e.sendAndAsk(t, third.ID), "MBCOTHER01", "254799999999 - Someone", 100)); st != "manual" || reason != "credited_elsewhere" {
		t.Fatalf("credited elsewhere: %s %s", st, reason)
	}
	// an answer to an older request does not count
	fourth := e.withdraw(t, e.player(t, 1000), 100, "d") // a fourth withdrawal today would pass the daily limit
	e.sendAndAsk(t, fourth.ID)
	if st, _ := settle(fourth.ID, completed("ts-old", "MBCOLD0001", msisdn+" - Jane", 100)); st != "sent" {
		t.Fatalf("stale answer settled it: %s", st)
	}
}

func TestVerify_ARejectedStatusRequestGoesToAnOperator(t *testing.T) {
	ctx := context.Background()
	d := &fakeDaraja{statusErr: daraja.ErrRejected}
	e := setupFake(t, d)
	u := e.player(t, 1000)
	p := e.withdraw(t, u, 100, "a")
	if err := e.svc.Send(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(payouts.DefaultResultWait + time.Second)
	if _, err := e.svc.Verify(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if st, _, reason := e.state(t, p.ID); st != "manual" || reason != "status_request_rejected" {
		t.Fatalf("%s %s", st, reason)
	}
	// open payouts the sweep finds get a verification
	q := e.withdraw(t, u, 100, "b")
	if err := e.svc.Send(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	e.q.take()
	e.clk.Advance(payouts.DefaultStaleAfter + time.Minute)
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if jobs := e.q.take(); len(jobs) != 1 || jobs[0].kind != "verify" || jobs[0].id != q.ID {
		t.Fatalf("sweep jobs %+v", jobs)
	}
}

// flakyClock fails every read after the first n, standing in for the database dropping mid-step.
type flakyClock struct {
	inner clock.Clock
	left  atomic.Int32
}

func (c *flakyClock) Now(ctx context.Context, q clock.Querier) (time.Time, error) {
	if c.left.Add(-1) < 0 {
		return time.Time{}, errors.New("clock: unavailable")
	}
	return c.inner.Now(ctx, q)
}

// A failure in the middle of any step rolls the whole step back: no debit, no payout, no state change, no money back.
func TestPayouts_AFailureMidStepLeavesNothingBehind(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	u := e.player(t, 250_000)
	flaky := &flakyClock{inner: e.clk}
	aw := audit.NewWriter()
	ls := ledger.NewStore()
	ids := identity.NewService(e.pool, e.clk, nil, aw, identity.Config{AuthKey: []byte("0123456789abcdef0123456789abcdef")})
	broken := payouts.NewService(e.pool, flaky, &fakeDaraja{}, e.q, ids, wallet.NewService(ls), ls, aw, payouts.Config{}, wallet.ErrInsufficientFunds)
	before := e.balance(t, u)

	if _, _, err := broken.RequestWithdrawal(ctx, u, 100, "x"); err == nil || e.balance(t, u) != before {
		t.Fatalf("request: %v balance %d", err, e.balance(t, u))
	}
	p := e.withdraw(t, u, 100, "a")
	held := e.withdraw(t, u, 120_000, "big")
	e.q.take()
	for name, step := range map[string]func() error{
		"send":    func() error { return broken.Send(ctx, p.ID) },
		"approve": func() error { _, err := broken.Approve(ctx, held.ID, "operator:a"); return err },
		"reject":  func() error { _, err := broken.Reject(ctx, held.ID, "operator:a", "x"); return err },
		"callback": func() error {
			return broken.HandleCallback(ctx, payouts.CallbackResult, []byte(`{"Result":{"ResultCode":1,"OriginatorConversationID":"x-0000001"}}`))
		},
		"sweep": func() error { return broken.Sweep(ctx) },
	} {
		if err := step(); err == nil {
			t.Errorf("%s: no error with a failing clock", name)
		}
	}
	if st, _, _ := e.state(t, p.ID); st != "pending" {
		t.Fatalf("send changed the payout: %s", st)
	}
	if st, _, _ := e.state(t, held.ID); st != "held" || e.balance(t, u) != before-120_100 {
		t.Fatalf("held payout %s balance %d", st, e.balance(t, u))
	}
	// the clock fails after the claim: the payout is left sending (checked later, never sent twice)
	flaky.left.Store(1)
	if err := broken.Send(ctx, p.ID); err == nil {
		t.Fatal("send: no error")
	}
	if st, _, _ := e.state(t, p.ID); st != "sending" {
		t.Fatalf("after a failed send: %s", st)
	}
	// verification and settlement fail cleanly too
	e.clk.Advance(payouts.DefaultStaleAfter + time.Minute)
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := broken.Verify(ctx, p.ID); err == nil {
		t.Fatal("verify: no error")
	}
	if st, _, _ := e.state(t, p.ID); st != "timed_out" {
		t.Fatalf("verify changed the payout: %s", st)
	}
}
