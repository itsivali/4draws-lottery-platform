package darajamock_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/itsivali/lottery-platform/internal/payments/daraja"
	"github.com/itsivali/lottery-platform/internal/payments/darajamock"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
)

type b2cEnv struct {
	mock   *darajamock.Server
	client *daraja.Client
	recv   *receiver
}

func newB2C(t *testing.T) *b2cEnv {
	t.Helper()
	recv := &receiver{got: map[string][][]byte{}, verdict: "0"}
	api := httptest.NewServer(recv)
	t.Cleanup(api.Close)
	m := darajamock.New(darajamock.Options{ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk",
		Initiator: "testapi", SecurityCredential: "cred", B2CShortCode: "600999", HTTPClient: api.Client()})
	ms := httptest.NewServer(m)
	t.Cleanup(ms.Close)
	c, err := daraja.New(daraja.Config{BaseURL: ms.URL, ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk",
		CallbackURL: api.URL + "/stk"}, ms.Client(), clock.NewManual(c2bStart))
	if err != nil {
		t.Fatal(err)
	}
	if c, err = c.WithB2C(daraja.B2CConfig{ShortCode: "600999", Initiator: "testapi", SecurityCredential: "cred",
		ResultURL: api.URL + "/result", TimeoutURL: api.URL + "/timeout",
		StatusResultURL: api.URL + "/status-result", StatusTimeoutURL: api.URL + "/status-timeout"}); err != nil {
		t.Fatal(err)
	}
	return &b2cEnv{mock: m, client: c, recv: recv}
}

func req(orig string) daraja.B2CRequest {
	return daraja.B2CRequest{OriginatorConversationID: orig, MSISDN: "254708374149", AmountKES: 500,
		CommandID: daraja.BusinessPayment, Remarks: "Withdrawal", Occasion: orig}
}

func (e *b2cEnv) status(t *testing.T, receipt, orig string) daraja.StatusResult {
	t.Helper()
	n := len(e.recv.bodies("/status-result"))
	if _, err := e.client.B2CStatus(context.Background(), receipt, orig, "occasion-1"); err != nil {
		t.Fatal(err)
	}
	e.mock.Flush()
	got := e.recv.bodies("/status-result")
	if len(got) != n+1 {
		t.Fatalf("status results %d", len(got))
	}
	r, err := daraja.ParseStatusResult(got[n])
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestB2C_PaysReportsAndConfirms(t *testing.T) {
	e := newB2C(t)
	if _, err := e.client.B2C(context.Background(), req("payout-00000001")); err != nil {
		t.Fatal(err)
	}
	e.mock.Flush()
	results := e.recv.bodies("/result")
	if len(results) != 1 {
		t.Fatalf("results %d", len(results))
	}
	r, err := daraja.ParseB2CResult(results[0])
	if err != nil || !r.Succeeded() || r.AmountKES != 500 || r.OriginatorConversationID != "payout-00000001" {
		t.Fatalf("%+v %v", r, err)
	}
	p := e.mock.Payouts()
	if len(p) != 1 || !p[0].Paid || p[0].Receipt != r.Receipt {
		t.Fatalf("payouts %+v", p)
	}
	for _, s := range []daraja.StatusResult{e.status(t, r.Receipt, ""), e.status(t, "", "payout-00000001")} {
		if !s.Completed() || s.AmountKES != 500 || s.Receipt != r.Receipt || s.CreditedMSISDN() != "254708374149" || s.Occasion != "occasion-1" {
			t.Fatalf("status %+v", s)
		}
	}
	// the same originator conversation id is refused: a second guard against paying twice
	if _, err := e.client.B2C(context.Background(), req("payout-00000001")); !errors.Is(err, daraja.ErrRejected) {
		t.Fatalf("repeat: %v", err)
	}
}

func TestB2C_Faults(t *testing.T) {
	ctx := context.Background()
	e := newB2C(t)
	claimOK := 0

	e.mock.SetNextB2CFault(darajamock.B2CFault{RequestHTTP: 400})
	if _, err := e.client.B2C(ctx, req("payout-refused1")); !errors.Is(err, daraja.ErrRejected) {
		t.Fatalf("refused: %v", err)
	}
	e.mock.SetNextB2CFault(darajamock.B2CFault{LostAnswer: true})
	if _, err := e.client.B2C(ctx, req("payout-lostans1")); !errors.Is(err, daraja.ErrUnavailable) {
		t.Fatalf("lost answer: %v", err)
	}
	e.mock.Flush()
	if s := e.status(t, "", "payout-lostans1"); !s.Completed() {
		t.Fatalf("a lost answer still paid: %+v", s)
	}
	e.mock.SetNextB2CFault(darajamock.B2CFault{ResultCode: 2001})
	if _, err := e.client.B2C(ctx, req("payout-failed01")); err != nil {
		t.Fatal(err)
	}
	e.mock.Flush()
	if s := e.status(t, "", "payout-failed01"); s.Completed() {
		t.Fatalf("a failed payout completed: %+v", s)
	}
	e.mock.SetNextB2CFault(darajamock.B2CFault{ClaimResultCode: &claimOK, ResultCode: 1})
	if _, err := e.client.B2C(ctx, req("payout-liar0001")); err != nil {
		t.Fatal(err)
	}
	e.mock.Flush()
	all := e.recv.bodies("/result")
	if r, _ := daraja.ParseB2CResult(all[len(all)-1]); !r.Succeeded() {
		t.Fatalf("the lying result does not claim success: %+v", r)
	}
	if s := e.status(t, "", "payout-liar0001"); s.Completed() {
		t.Fatal("Transaction Status believed the lie")
	}
	e.mock.SetNextB2CFault(darajamock.B2CFault{ResultTimeout: true})
	if _, err := e.client.B2C(ctx, req("payout-timeout1")); err != nil {
		t.Fatal(err)
	}
	e.mock.SetNextB2CFault(darajamock.B2CFault{DuplicateResult: true})
	if _, err := e.client.B2C(ctx, req("payout-twice001")); err != nil {
		t.Fatal(err)
	}
	e.mock.SetNextB2CFault(darajamock.B2CFault{DropResult: true, StatusHTTP: 503})
	if _, err := e.client.B2C(ctx, req("payout-dropped1")); err != nil {
		t.Fatal(err)
	}
	e.mock.Flush()
	if n := len(e.recv.bodies("/timeout")); n != 1 {
		t.Fatalf("timeouts %d", n)
	}
	if n := len(e.recv.bodies("/result")); n != 5 { // lost answer, failed, liar, twice ×2; none for the refused or dropped
		t.Fatalf("results %d", n)
	}
	if _, err := e.client.B2CStatus(ctx, "", "payout-dropped1", "x"); !errors.Is(err, daraja.ErrUnavailable) {
		t.Fatalf("status outage: %v", err)
	}
	// accepted: lost answer (paid), failed, liar (not paid), timeout (not paid), twice (paid), dropped (paid)
	if paid := e.mock.Payouts(); len(paid) != 6 || !paid[0].Paid || paid[1].Paid || paid[2].Paid || paid[3].Paid || !paid[4].Paid || !paid[5].Paid {
		t.Fatalf("payouts %+v", paid)
	}
}

// In the sandbox the paybill and the payout account are one shortcode: status requests still reach the right side.
func TestB2C_StatusOnASharedShortcodeReachesPayoutsAndPaybills(t *testing.T) {
	recv := &receiver{got: map[string][][]byte{}, verdict: "0"}
	api := httptest.NewServer(recv)
	t.Cleanup(api.Close)
	m := darajamock.New(darajamock.Options{ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk",
		C2BShortCode: "600992", B2CShortCode: "600992", Initiator: "testapi", SecurityCredential: "cred", HTTPClient: api.Client()})
	ms := httptest.NewServer(m)
	t.Cleanup(ms.Close)
	c, err := daraja.New(daraja.Config{BaseURL: ms.URL, ConsumerKey: "ck", ConsumerSecret: "cs", ShortCode: "174379", Passkey: "pk",
		CallbackURL: api.URL + "/stk"}, ms.Client(), clock.NewManual(c2bStart))
	if err != nil {
		t.Fatal(err)
	}
	if c, err = c.WithC2B(daraja.C2BConfig{ShortCode: "600992", Initiator: "testapi", SecurityCredential: "cred",
		ResultURL: api.URL + "/c2b-status", TimeoutURL: api.URL + "/c2b-timeout"}); err != nil {
		t.Fatal(err)
	}
	if c, err = c.WithB2C(daraja.B2CConfig{ShortCode: "600992", Initiator: "testapi", SecurityCredential: "cred", ResultURL: api.URL + "/result",
		TimeoutURL: api.URL + "/timeout", StatusResultURL: api.URL + "/b2c-status", StatusTimeoutURL: api.URL + "/b2c-timeout"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.RegisterC2BURLs(ctx, api.URL+"/validate", api.URL+"/confirm", daraja.C2BCancelOnTimeout); err != nil {
		t.Fatal(err)
	}
	receipt, err := m.Pay(ctx, 100, "254708374149", "0708374149")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.B2C(ctx, req("payout-shared01")); err != nil {
		t.Fatal(err)
	}
	m.Flush()
	if _, err := c.TransactionStatus(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := c.B2CStatus(ctx, "", "payout-shared01", "occ"); err != nil {
		t.Fatal(err)
	}
	m.Flush()
	paybill, _ := daraja.ParseStatusResult(recv.bodies("/c2b-status")[0])
	payout, _ := daraja.ParseStatusResult(recv.bodies("/b2c-status")[0])
	if !paybill.Completed() || paybill.AmountKES != 100 || !payout.Completed() || payout.AmountKES != 500 {
		t.Fatalf("paybill %+v payout %+v", paybill, payout)
	}
}
