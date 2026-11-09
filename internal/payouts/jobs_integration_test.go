//go:build integration

package payouts_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/payments/daraja"
	"github.com/itsivali/lottery-platform/internal/payments/darajamock"
	"github.com/itsivali/lottery-platform/internal/payouts"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/db"
	"github.com/itsivali/lottery-platform/internal/platform/testpg"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

// The job wiring end to end: real River workers send, receive the result through the hook, ask Transaction Status
// and settle, with nothing but the queue driving them.
func TestJobs_AWithdrawalIsPaidByTheQueueAlone(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t, "app_worker")
	clk := clock.Postgres{}
	aw := audit.NewWriter()
	var svc *payouts.Service
	hooks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		kind := map[string]string{"/result": payouts.CallbackResult, "/timeout": payouts.CallbackTimeout,
			"/status-result": payouts.CallbackStatusResult, "/status-timeout": payouts.CallbackStatusTimeout}[r.URL.Path]
		if err := svc.HandleCallback(r.Context(), kind, raw); err != nil {
			t.Errorf("%s: %v", r.URL.Path, err)
		}
	}))
	t.Cleanup(hooks.Close)
	mock := darajamock.New(darajamock.Options{ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk",
		Initiator: "testapi", SecurityCredential: "cred", B2CShortCode: "600992", HTTPClient: hooks.Client()})
	ms := httptest.NewServer(mock)
	t.Cleanup(ms.Close)
	dc, err := daraja.New(daraja.Config{BaseURL: ms.URL, ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk",
		CallbackURL: hooks.URL + "/stk"}, ms.Client(), clock.Wall{})
	if err != nil {
		t.Fatal(err)
	}
	if dc, err = dc.WithB2C(daraja.B2CConfig{ShortCode: "600992", Initiator: "testapi", SecurityCredential: "cred", ResultURL: hooks.URL + "/result",
		TimeoutURL: hooks.URL + "/timeout", StatusResultURL: hooks.URL + "/status-result", StatusTimeoutURL: hooks.URL + "/status-timeout"}); err != nil {
		t.Fatal(err)
	}
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: db.RiverSchema})
	if err != nil {
		t.Fatal(err)
	}
	ls := ledger.NewStore()
	ids := identity.NewService(pool, clk, nil, aw, identity.Config{AuthKey: []byte("0123456789abcdef0123456789abcdef")})
	svc = payouts.NewService(pool, clk, dc, payouts.NewRiverEnqueuer(insertOnly), ids, wallet.NewService(ls), ls, aw,
		payouts.Config{ResultWait: 50 * time.Millisecond, StatusWait: 200 * time.Millisecond}, wallet.ErrInsufficientFunds)
	workers := river.NewWorkers()
	payouts.AddWorkers(workers, svc)
	if len(payouts.PeriodicJobs()) != 3 || (payouts.SweepArgs{}).Kind() != "payouts.sweep" || (payouts.PrizesArgs{}).Kind() != "payouts.prizes" ||
		(payouts.ReconcileArgs{}).Kind() != "payouts.reconcile" {
		t.Fatal("periodic jobs")
	}
	wc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: db.RiverSchema, Workers: workers,
		PeriodicJobs: payouts.PeriodicJobs(), Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := wc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wc.Stop(context.Background()) })

	e := &env{pool: pool}
	u := e.player(t, 1000)
	p, _, err := svc.RequestWithdrawal(ctx, u, 300, "jobs-1")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second) //nolint:forbidigo // test polling deadline
	for {
		st, receipt, _ := e.state(t, p.ID)
		if st == "succeeded" && receipt != "" {
			break
		}
		if time.Now().After(deadline) { //nolint:forbidigo // test polling deadline
			t.Fatalf("the queue never paid it: %s", st)
		}
		time.Sleep(50 * time.Millisecond) //nolint:forbidigo // polling
	}
	if got := mock.Payouts(); len(got) != 1 || !got[0].Paid || e.balance(t, u) != 650 {
		t.Fatalf("payouts %+v balance %d", got, e.balance(t, u))
	}
	// the sweep runs from the queue too
	res, err := wc.Insert(ctx, payouts.SweepArgs{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for {
		j, err := wc.JobGet(ctx, res.Job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if j.State == rivertype.JobStateCompleted {
			break
		}
		if time.Now().After(deadline) { //nolint:forbidigo // test polling deadline
			t.Fatalf("sweep job %s", j.State)
		}
		time.Sleep(50 * time.Millisecond) //nolint:forbidigo // polling
	}
}

func TestOperator_RefusesBadInputAndUnknownPayouts(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	unknown := "0192f000-0000-7000-8000-000000000000"
	if _, err := e.svc.Approve(ctx, unknown, "staff:a"); err == nil {
		t.Error("approved an unknown payout")
	}
	if _, err := e.svc.Reject(ctx, unknown, "staff:a", ""); err == nil {
		t.Error("rejected without a reason")
	}
	if _, err := e.svc.Reject(ctx, "not-a-uuid", "staff:a", "x"); err == nil {
		t.Error("rejected an unknown payout")
	}
	if _, err := e.svc.Settle(ctx, unknown, "staff:a", false, "", ""); err == nil {
		t.Error("settled without a reason")
	}
	u := e.player(t, 1000)
	p := e.withdraw(t, u, 300, "k")
	if _, err := e.svc.Approve(ctx, p.ID, "staff:a"); err == nil {
		t.Error("approved a payout that was not held")
	}
	off := setupWith(t, false, payouts.Config{})
	if _, err := off.svc.Approve(ctx, unknown, "staff:a"); err == nil {
		t.Error("approved without B2C")
	}
	if err := off.svc.Send(ctx, unknown); err == nil {
		t.Error("sent without B2C")
	}
	if list, err := e.svc.List(ctx, u, 10); err != nil || len(list) != 1 {
		t.Fatalf("list %v %v", list, err)
	}
	if e.svc.Enabled() == off.svc.Enabled() {
		t.Error("enabled does not reflect B2C")
	}
}
