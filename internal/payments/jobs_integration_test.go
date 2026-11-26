//go:build integration

package payments_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/identity"
	"github.com/itsivali/4draws-lottery-platform/internal/ledger"
	"github.com/itsivali/4draws-lottery-platform/internal/payments"
	"github.com/itsivali/4draws-lottery-platform/internal/payments/daraja"
	"github.com/itsivali/4draws-lottery-platform/internal/payments/darajamock"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/db"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/testpg"
	"github.com/itsivali/4draws-lottery-platform/internal/rg"
	"github.com/itsivali/4draws-lottery-platform/internal/wallet"
)

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second) //nolint:forbidigo // test polling deadline
	for !ok() {
		if time.Now().After(deadline) { //nolint:forbidigo // test polling deadline
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond) //nolint:forbidigo // polling helper
	}
}

// The job wiring end to end: real River workers push, verify on the callback and credit; the reconcile and sweep
// jobs run from the queue.
func TestJobs_ATopUpIsSettledByTheQueueAlone(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t, "app_worker")
	clk := clock.Postgres{}
	aw := audit.NewWriter()

	var svc *payments.Service
	cb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := svc.HandleCallback(r.Context(), raw); err != nil {
			t.Errorf("callback: %v", err)
		}
	}))
	t.Cleanup(cb.Close)
	mock := darajamock.New(darajamock.Options{ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk", HTTPClient: cb.Client()})
	mock.AutoComplete(ctx, 0)
	ms := httptest.NewServer(mock)
	t.Cleanup(ms.Close)
	dc, err := daraja.New(daraja.Config{BaseURL: ms.URL, ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk", CallbackURL: cb.URL},
		ms.Client(), clock.Wall{})
	if err != nil {
		t.Fatal(err)
	}

	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: db.RiverSchema})
	if err != nil {
		t.Fatal(err)
	}
	ls := ledger.NewStore()
	ids := identity.NewService(pool, clk, nil, aw, identity.Config{AuthKey: []byte("0123456789abcdef0123456789abcdef")})
	svc = payments.NewService(pool, clk, dc, payments.NewRiverEnqueuer(insertOnly), ids,
		rg.NewService(pool, clk, aw, rg.Config{CoolingOff: 24 * time.Hour}), wallet.NewService(ls), ls, aw, payments.Config{})

	workers := river.NewWorkers()
	payments.AddWorkers(workers, svc)
	wc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: db.RiverSchema, Workers: workers, PeriodicJobs: payments.PeriodicJobs(),
		Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := wc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wc.Stop(context.Background()) })

	user, msisdn := "", "254700000999"
	if err := pool.QueryRow(ctx, `INSERT INTO identity.users (msisdn, kyc_status, national_id_hash, password_hash)
		VALUES ($1, 'verified', 'jobs-test', 'x') RETURNING id::text`, msisdn).Scan(&user); err != nil {
		t.Fatal(err)
	}
	tu, err := svc.StartTopUp(ctx, user, 200, "jobs-key")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the top-up to be credited by the workers", func() bool {
		got, err := svc.Get(ctx, user, tu.ID)
		return err == nil && got.Status == payments.StatusCredited
	})

	runs := func() int {
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM payments.reconciliation_runs WHERE ok`).Scan(&n)
		return n
	}
	waitFor(t, "the reconcile job that runs on start", func() bool { return runs() >= 1 })
	before := runs()
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := insertOnly.InsertTx(ctx, tx, payments.ReconcileArgs{}, nil); err != nil {
			return err
		}
		_, err := insertOnly.InsertTx(ctx, tx, payments.SweepArgs{}, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "an enqueued reconcile job", func() bool { return runs() > before })
	var swept int
	waitFor(t, "the sweep job", func() bool {
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM river.river_job WHERE kind = 'payments.sweep' AND state = 'completed'`).Scan(&swept)
		return swept >= 1
	})
}

// One verify job per payment, whatever asked for it; a callback brings the waiting one forward (2026-10-30: three
// chains per payment tripled the status queries Daraja throttles).
func TestJobs_APaymentHasOneVerifyJobAndACallbackBringsItForward(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t, "app_worker")
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: db.RiverSchema})
	if err != nil {
		t.Fatal(err)
	}
	enq := payments.NewRiverEnqueuer(rc)
	const id = "01a19000-0000-7000-8000-000000000001"
	in := func(f func(tx pgx.Tx) error) {
		t.Helper()
		if err := pgx.BeginFunc(ctx, pool, f); err != nil {
			t.Fatal(err)
		}
	}
	// the verify job after the push, snoozed until the customer could have answered
	in(func(tx pgx.Tx) error {
		_, err := rc.InsertTx(ctx, tx, payments.VerifyArgs{PaymentID: id, Source: payments.AfterPush},
			&river.InsertOpts{ScheduledAt: time.Now().Add(time.Hour), UniqueOpts: river.UniqueOpts{ByArgs: true}}) //nolint:forbidigo // river schedules by wall time
		return err
	})
	in(func(tx pgx.Tx) error { return enq.EnqueueVerify(ctx, tx, id, payments.FromSweep) })
	in(func(tx pgx.Tx) error { return enq.EnqueueVerify(ctx, tx, id, payments.AfterCallback) })
	var n int
	var due bool
	if err := pool.QueryRow(ctx, `SELECT count(*), bool_and(scheduled_at <= now()) FROM river.river_job
		WHERE kind = 'payments.stk_verify' AND args->>'payment_id' = $1`, id).Scan(&n, &due); err != nil {
		t.Fatal(err)
	}
	if n != 1 || !due {
		t.Fatalf("%d verify jobs, due now = %v; want one, brought forward by the callback", n, due)
	}
}

// The receipt-claim journey on the real queue (owner, 2026-10-31): a top-up the customer paid whose callback never
// came settles unconfirmed; the receipt the player types in is checked by the claim job through Transaction Status
// and credited only on M-Pesa's answer. A claim met by an M-Pesa outage stays open and its job is snoozed, not
// dropped: the queue asks again.
func TestJobs_AClaimedReceiptIsCheckedByTheQueue(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t, "app_worker")
	clk := clock.Postgres{}
	aw := audit.NewWriter()

	var svc *payments.Service
	cb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var err error
		switch r.URL.Path {
		case "/confirm":
			err = svc.HandleC2BConfirmation(r.Context(), raw)
		case "/status-result":
			err = svc.HandleC2BStatusResult(r.Context(), raw)
		case "/status-timeout":
			svc.HandleC2BStatusTimeout(r.Context(), raw)
		default:
			err = svc.HandleCallback(r.Context(), raw)
		}
		if err != nil {
			t.Errorf("%s: %v", r.URL.Path, err)
		}
	}))
	t.Cleanup(cb.Close)
	mock := darajamock.New(darajamock.Options{ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk",
		C2BShortCode: "600982", Initiator: "testapi", SecurityCredential: "cred", HTTPClient: cb.Client()})
	mock.AutoComplete(ctx, 0)
	ms := httptest.NewServer(mock)
	t.Cleanup(ms.Close)
	dc, err := daraja.New(daraja.Config{BaseURL: ms.URL, ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk", CallbackURL: cb.URL},
		ms.Client(), clock.Wall{})
	if err != nil {
		t.Fatal(err)
	}
	if dc, err = dc.WithC2B(daraja.C2BConfig{ShortCode: "600982", Initiator: "testapi", SecurityCredential: "cred",
		ResultURL: cb.URL + "/status-result", TimeoutURL: cb.URL + "/status-timeout"}); err != nil {
		t.Fatal(err)
	}
	if err := dc.RegisterC2BURLs(ctx, cb.URL+"/validate", cb.URL+"/confirm", daraja.C2BCancelOnTimeout); err != nil {
		t.Fatal(err)
	}

	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: db.RiverSchema})
	if err != nil {
		t.Fatal(err)
	}
	ls := ledger.NewStore()
	ids := identity.NewService(pool, clk, nil, aw, identity.Config{AuthKey: []byte("0123456789abcdef0123456789abcdef")})
	// short waits so the prompt settles unconfirmed within the test
	cfg := payments.Config{ShortCode: "174379", VerifyAfter: 200 * time.Millisecond, Expiry: time.Second}
	svc = payments.NewService(pool, clk, dc, payments.NewRiverEnqueuer(insertOnly), ids,
		rg.NewService(pool, clk, aw, rg.Config{CoolingOff: 24 * time.Hour}), wallet.NewService(ls), ls, aw, cfg).
		WithC2B(dc, payments.C2BConfig{ShortCode: "600982", MaxStatusRequests: 3})

	workers := river.NewWorkers()
	payments.AddWorkers(workers, svc)
	wc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: db.RiverSchema, Workers: workers, Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := wc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wc.Stop(context.Background()) })

	user, msisdn := "", "254700000998"
	if err := pool.QueryRow(ctx, `INSERT INTO identity.users (msisdn, kyc_status, national_id_hash, password_hash)
		VALUES ($1, 'verified', 'jobs-claim-test', 'x') RETURNING id::text`, msisdn).Scan(&user); err != nil {
		t.Fatal(err)
	}
	balance := func() int64 {
		var b int64
		_ = pool.QueryRow(ctx, `SELECT coalesce((SELECT balance_kes FROM wallet.wallets WHERE user_id = $1), 0)`, user).Scan(&b)
		return b
	}
	unconfirmed := func(key string) payments.TopUp {
		t.Helper()
		before := balance()
		mock.SetNextFault(darajamock.Fault{DropCallback: true})
		tu, err := svc.StartTopUp(ctx, user, 100, key)
		if err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the uncalled-back top-up to settle unconfirmed", func() bool {
			got, err := svc.Get(ctx, user, tu.ID)
			return err == nil && got.Status != payments.StatusPending
		})
		if got, _ := svc.Get(ctx, user, tu.ID); got.Status != payments.StatusUnconfirmed || balance() != before {
			t.Fatalf("top-up %+v, balance %d: credited without a callback", got, balance())
		}
		return tu
	}

	tu := unconfirmed("claim-key-1")
	pushes := mock.Pushes()
	receipt := mock.ReceiptOf(pushes[len(pushes)-1].CheckoutRequestID)
	if _, err := svc.ClaimReceipt(ctx, user, tu.ID, receipt); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the claim job to credit the confirmed receipt", func() bool {
		got, err := svc.Get(ctx, user, tu.ID)
		return err == nil && got.Status == payments.StatusCredited
	})
	if got, _ := svc.Get(ctx, user, tu.ID); got.Claim == nil || got.Claim.Status != payments.ClaimConfirmed || balance() != 100 {
		t.Fatalf("claim %+v balance %d", got.Claim, balance())
	}

	// M-Pesa down for the second claim: the job snoozes and the queue asks again; nothing is credited meanwhile
	tu2 := unconfirmed("claim-key-2")
	mock.SetNextC2BFault(darajamock.C2BFault{DropConfirmation: true, StatusHTTP: http.StatusServiceUnavailable})
	down, err := mock.Pay(ctx, 100, msisdn, msisdn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ClaimReceipt(ctx, user, tu2.ID, down); err != nil {
		t.Fatal(err)
	}
	asked := func() int {
		n := 0
		for _, r := range mock.StatusRequests() {
			if r == down {
				n++
			}
		}
		return n
	}
	waitFor(t, "the snoozed claim job to ask M-Pesa again", func() bool { return asked() >= 2 })
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM river.river_job WHERE kind = 'payments.claim_check' AND args->>'payment_id' = $1`, tu2.ID).Scan(&state); err != nil ||
		(state != "scheduled" && state != "running" && state != "available") {
		t.Fatalf("claim job state %q (%v): an outage must not end the job", state, err)
	}
	if got, _ := svc.Get(ctx, user, tu2.ID); got.Status == payments.StatusCredited || got.Claim == nil || got.Claim.Status != payments.ClaimClaimed || balance() != 100 {
		t.Fatalf("second top-up %+v claim %+v balance %d", got, got.Claim, balance())
	}
}
