package daraja_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itsivali/4draws-lottery-platform/internal/payments/daraja"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
)

var b2cConfig = daraja.B2CConfig{
	ShortCode: "600999", Initiator: "testapi", SecurityCredential: "c2VjcmV0",
	ResultURL: "https://api.example/hooks/b2c/tok/result", TimeoutURL: "https://api.example/hooks/b2c/tok/timeout",
	StatusResultURL: "https://api.example/hooks/b2c/tok/status-result", StatusTimeoutURL: "https://api.example/hooks/b2c/tok/status-timeout",
}

func b2cServer(t *testing.T, h func(w http.ResponseWriter, path string, body map[string]any)) *daraja.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/v1/generate" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":"3599"}`))
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		h(w, r.URL.Path, body)
	}))
	t.Cleanup(srv.Close)
	c, err := daraja.New(testConfig(srv.URL), srv.Client(), clock.NewManual(at))
	if err != nil {
		t.Fatal(err)
	}
	if c, err = c.WithB2C(b2cConfig); err != nil {
		t.Fatal(err)
	}
	return c
}

var payout = daraja.B2CRequest{OriginatorConversationID: "0192f000-0000-7000-8000-000000000001", MSISDN: "254708374149",
	AmountKES: 500, CommandID: daraja.BusinessPayment, Remarks: "Withdrawal", Occasion: "0192f000-0000-7000-8000-000000000001"}

func TestWithB2C_RefusesIncompleteConfiguration(t *testing.T) {
	c, err := daraja.New(testConfig("https://sandbox.safaricom.co.ke"), http.DefaultClient, clock.NewManual(at))
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*daraja.B2CConfig){
		"no shortcode":      func(c *daraja.B2CConfig) { c.ShortCode = "" },
		"no initiator":      func(c *daraja.B2CConfig) { c.Initiator = "" },
		"no credential":     func(c *daraja.B2CConfig) { c.SecurityCredential = "" },
		"no status url":     func(c *daraja.B2CConfig) { c.StatusResultURL = "" },
		"relative result":   func(c *daraja.B2CConfig) { c.ResultURL = "/hooks/b2c/x/result" },
		"http to safaricom": func(c *daraja.B2CConfig) { c.TimeoutURL = "http://api.example/t" },
	} {
		cfg := b2cConfig
		mutate(&cfg)
		if _, err := c.WithB2C(cfg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// without B2C the calls are refused
	if _, err := c.B2C(context.Background(), payout); !errors.Is(err, daraja.ErrInvalidRequest) {
		t.Errorf("b2c without config: %v", err)
	}
	if _, err := c.B2CStatus(context.Background(), "", payout.OriginatorConversationID, "x"); !errors.Is(err, daraja.ErrInvalidRequest) {
		t.Errorf("status without config: %v", err)
	}
}

func TestB2C_SendsThePaymentAndReturnsTheConversation(t *testing.T) {
	var got map[string]any
	c := b2cServer(t, func(w http.ResponseWriter, path string, body map[string]any) {
		if path != "/mpesa/b2c/v3/paymentrequest" {
			t.Errorf("path %s", path)
		}
		got = body
		_, _ = w.Write([]byte(`{"ConversationID":"AG_20261025_1","OriginatorConversationID":"x","ResponseCode":"0","ResponseDescription":"Accept the service request successfully."}`))
	})
	res, err := c.B2C(context.Background(), payout)
	if err != nil || res.ConversationID != "AG_20261025_1" {
		t.Fatalf("%+v %v", res, err)
	}
	for k, want := range map[string]any{"OriginatorConversationID": payout.OriginatorConversationID, "InitiatorName": "testapi",
		"SecurityCredential": "c2VjcmV0", "CommandID": "BusinessPayment", "Amount": float64(500), "PartyA": "600999",
		"PartyB": "254708374149", "ResultURL": b2cConfig.ResultURL, "QueueTimeOutURL": b2cConfig.TimeoutURL, "Occassion": payout.Occasion} {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}
}

func TestB2C_ClassifiesRefusalsAndOutages(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"bad request":       {400, `{"errorCode":"400.002.02","errorMessage":"Invalid Initiator c2VjcmV0"}`, daraja.ErrRejected},
		"response code":     {200, `{"ConversationID":"AG_1","ResponseCode":"1","ResponseDescription":"no"}`, daraja.ErrRejected},
		"server error":      {500, `{"errorCode":"500.003.02","errorMessage":"System busy"}`, daraja.ErrUnavailable},
		"throttled":         {429, `{}`, daraja.ErrUnavailable},
		"no conversation":   {200, `{"ResponseCode":"0"}`, daraja.ErrUnavailable},
		"unreadable answer": {200, `not json`, daraja.ErrUnavailable},
	} {
		c := b2cServer(t, func(w http.ResponseWriter, _ string, _ map[string]any) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})
		_, err := c.B2C(context.Background(), payout)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
		if err != nil && strings.Contains(err.Error(), "c2VjcmV0") {
			t.Errorf("%s: the credential leaked into the error: %v", name, err)
		}
	}
}

func TestB2C_RefusesBadRequestsBeforeSendingThem(t *testing.T) {
	c := b2cServer(t, func(http.ResponseWriter, string, map[string]any) { t.Error("a bad request was sent") })
	for name, mutate := range map[string]func(*daraja.B2CRequest){
		"zero amount":     func(r *daraja.B2CRequest) { r.AmountKES = 0 },
		"too much":        func(r *daraja.B2CRequest) { r.AmountKES = daraja.MaxSTKAmountKES + 1 },
		"bad phone":       func(r *daraja.B2CRequest) { r.MSISDN = "0712345678" },
		"bad originator":  func(r *daraja.B2CRequest) { r.OriginatorConversationID = "x y" },
		"unknown command": func(r *daraja.B2CRequest) { r.CommandID = "SalaryPayment" },
		"no remarks":      func(r *daraja.B2CRequest) { r.Remarks = "" },
	} {
		r := payout
		mutate(&r)
		if _, err := c.B2C(context.Background(), r); !errors.Is(err, daraja.ErrInvalidRequest) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestB2CStatus_AsksByReceiptOrByOriginator(t *testing.T) {
	var got []map[string]any
	c := b2cServer(t, func(w http.ResponseWriter, path string, body map[string]any) {
		if path != "/mpesa/transactionstatus/v1/query" {
			t.Errorf("path %s", path)
		}
		got = append(got, body)
		_, _ = w.Write([]byte(`{"OriginatorConversationID":"ts-1","ConversationID":"AG_TS","ResponseCode":"0"}`))
	})
	if req, err := c.B2CStatus(context.Background(), "NLJ41HAY6Q", "", "payout-1"); err != nil || req.OriginatorConversationID != "ts-1" {
		t.Fatalf("%+v %v", req, err)
	}
	if _, err := c.B2CStatus(context.Background(), "", payout.OriginatorConversationID, "payout-1"); err != nil {
		t.Fatal(err)
	}
	if got[0]["TransactionID"] != "NLJ41HAY6Q" || got[1]["OriginalConversationID"] != payout.OriginatorConversationID ||
		got[1]["PartyA"] != "600999" || got[1]["ResultURL"] != b2cConfig.StatusResultURL || got[1]["Occasion"] != "payout-1" {
		t.Fatalf("bodies %v", got)
	}
	for name, args := range map[string][3]string{
		"nothing to ask about": {"", "", "x"},
		"bad receipt":          {"nope", "", "x"},
		"bad originator":       {"", "a b", "x"},
	} {
		if _, err := c.B2CStatus(context.Background(), args[0], args[1], args[2]); !errors.Is(err, daraja.ErrInvalidRequest) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestParseB2CResult(t *testing.T) {
	ok := `{"Result":{"ResultType":0,"ResultCode":0,"ResultDesc":"The service request is processed successfully.",
		"OriginatorConversationID":"orig-12345678","ConversationID":"AG_1","TransactionID":"NLJ41HAY6Q",
		"ResultParameters":{"ResultParameter":[{"Key":"TransactionAmount","Value":500},{"Key":"TransactionReceipt","Value":"NLJ41HAY6Q"},
		{"Key":"ReceiverPartyPublicName","Value":"254708374149 - John Doe"},{"Key":"B2CUtilityAccountAvailableFunds","Value":10116.00}]}}}`
	r, err := daraja.ParseB2CResult([]byte(ok))
	if err != nil || !r.Succeeded() || r.Receipt != "NLJ41HAY6Q" || r.AmountKES != 500 || r.OriginatorConversationID != "orig-12345678" ||
		r.Receiver != "254708374149 - John Doe" {
		t.Fatalf("%+v %v", r, err)
	}
	failed := `{"Result":{"ResultType":0,"ResultCode":2001,"ResultDesc":"The initiator information is invalid.","OriginatorConversationID":"orig-12345678","ConversationID":"AG_1","TransactionID":"NLJ0000000"}}`
	if r, err := daraja.ParseB2CResult([]byte(failed)); err != nil || r.Succeeded() || r.ResultCode != 2001 || r.Receipt != "" {
		t.Fatalf("failure: %+v %v", r, err)
	}
	for name, body := range map[string]string{
		"not json":           `nope`,
		"no result code":     `{"Result":{"OriginatorConversationID":"o"}}`,
		"no originator":      `{"Result":{"ResultCode":0}}`,
		"success no receipt": `{"Result":{"ResultCode":0,"OriginatorConversationID":"o","ResultParameters":{"ResultParameter":[{"Key":"TransactionAmount","Value":5}]}}}`,
		"fractional amount":  `{"Result":{"ResultCode":0,"OriginatorConversationID":"o","TransactionID":"NLJ41HAY6Q","ResultParameters":{"ResultParameter":[{"Key":"TransactionAmount","Value":5.5}]}}}`,
	} {
		if _, err := daraja.ParseB2CResult([]byte(body)); !errors.Is(err, daraja.ErrBadCallback) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestStatusResult_CreditedMSISDN(t *testing.T) {
	for party, want := range map[string]string{"254708374149 - John Doe": "254708374149", "2547****149 - John": "", "": "", "600999 - Org": ""} {
		if got := (daraja.StatusResult{CreditParty: party}).CreditedMSISDN(); got != want {
			t.Errorf("%q: %q, want %q", party, got, want)
		}
	}
}
