package main

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

// opID is a staff member's id (their Keycloak subject), as -operator takes it.
const opID = "0192f000-0000-7000-8000-00000000a11c"

func TestParse(t *testing.T) {
	tests := []struct {
		args    []string
		want    command
		wantErr bool
	}{
		{[]string{"migrate", "up"}, command{name: "migrate up"}, false},
		{[]string{"migrate", "down"}, command{name: "migrate down"}, false},
		{[]string{"migrate"}, command{}, true},
		{[]string{"migrate", "sideways"}, command{}, true},
		{[]string{"payments", "reconcile"}, command{name: "payments reconcile"}, false},
		{[]string{"payments", "reverse", "-receipt", "TJK1AB2CD3", "-reason", "dispute upheld", "-operator", opID},
			command{name: "payments reverse", receipt: "TJK1AB2CD3", reason: "dispute upheld", operator: "staff:" + opID}, false},
		{[]string{"payments", "reverse", "-receipt", "R", "-operator", opID}, command{}, true}, // no reason
		{[]string{"payments", "reverse", "-receipt", "R", "-reason", "x"}, command{}, true},    // no operator
		{[]string{"payments", "reverse", "-reason", "x", "-operator", opID}, command{}, true},  // no receipt
		{[]string{"payments", "reverse", "-receipt", "R", "-reason", "x", "-operator", opID, "extra"}, command{}, true},
		{[]string{"payments", "resolve", "-payment", "0192f000-0000-7000-8000-000000000001", "-receipt", "TJK1", "-amount", "150", "-operator", opID},
			command{name: "payments resolve", payment: "0192f000-0000-7000-8000-000000000001", receipt: "TJK1", amount: 150, operator: "staff:" + opID}, false},
		{[]string{"payments", "resolve", "-payment", "p", "-receipt", "R", "-operator", opID}, command{}, true},                   // no amount
		{[]string{"payments", "resolve", "-payment", "p", "-receipt", "R", "-amount", "abc", "-operator", opID}, command{}, true}, // bad amount
		{[]string{"payments", "refund"}, command{}, true},
		{[]string{"payments", "suspense-credit", "-receipt", "MCC0000001", "-user", "u-1", "-operator", opID},
			command{name: "payments suspense-credit", receipt: "MCC0000001", user: "u-1", operator: "staff:" + opID}, false},
		{[]string{"payments", "suspense-credit", "-receipt", "R", "-operator", opID}, command{}, true}, // no user
		{[]string{"payments", "suspense-refund", "-receipt", "R", "-refund-ref", "TJK9", "-operator", opID},
			command{name: "payments suspense-refund", receipt: "R", refundRef: "TJK9", operator: "staff:" + opID}, false},
		{[]string{"payments", "suspense-refund", "-receipt", "R", "-operator", opID}, command{}, true}, // no refund reference
		{[]string{"payments", "c2b-recheck", "-receipt", "R", "-operator", opID}, command{name: "payments c2b-recheck", receipt: "R", operator: "staff:" + opID}, false},
		{[]string{"payments", "c2b-recheck", "-receipt", "R", "-operator", opID, "-amount", "5"}, command{}, true}, // flag it does not take
		{[]string{"payments", "c2b-register"}, command{name: "payments c2b-register"}, false},
		{[]string{"payments", "c2b-register", "-receipt", "R"}, command{}, true},
		{[]string{"payments", "c2b-simulate", "-amount", "50", "-msisdn", "254708374149", "-account", "0712345678"},
			command{name: "payments c2b-simulate", amount: 50, msisdn: "254708374149", account: "0712345678"}, false},
		{[]string{"payments", "c2b-simulate", "-amount", "-5", "-msisdn", "254708374149", "-account", "x"}, command{}, true},
		{[]string{"payments", "reconcile", "-operator", opID}, command{}, true},
		{[]string{"kyc", "pending", "-operator", opID}, command{name: "kyc pending", operator: "staff:" + opID}, false},
		{[]string{"kyc", "pending"}, command{}, true},
		{[]string{"kyc", "approve", "-user", "u-1", "-reason", "ID checked", "-operator", opID},
			command{name: "kyc approve", user: "u-1", reason: "ID checked", operator: "staff:" + opID}, false},
		{[]string{"kyc", "reject", "-user", "u-1", "-operator", opID}, command{}, true}, // no reason
		{[]string{"kyc", "promote", "-user", "u-1"}, command{}, true},
		{[]string{"game", "reconcile"}, command{name: "game reconcile"}, false},
		{[]string{"game", "reconcile", "-days", "5"}, command{}, true},
		{[]string{"game", "simulate"}, command{name: "game simulate"}, false},
		{[]string{"game", "simulate", "-days", "500", "-slots", "10", "-lucky", "2", "-price", "50"},
			command{name: "game simulate", days: 500, slots: 10, lucky: 2, price: 50}, false},
		{[]string{"game", "simulate", "-days", "-1"}, command{}, true},
		{[]string{"game", "simulate", "-operator", opID}, command{}, true},
		{[]string{"draws", "list"}, command{name: "draws list"}, false},
		{[]string{"draws", "show", "-draw", "d-1"}, command{name: "draws show", draw: "d-1"}, false},
		{[]string{"draws", "sign", "-draw", "d-1", "-operator", opID}, command{name: "draws sign", draw: "d-1", operator: "staff:" + opID}, false},
		{[]string{"draws", "sign", "-draw", "d-1"}, command{}, true}, // no official
		{[]string{"draws", "approve", "-draw", "d-1", "-operator", opID}, command{name: "draws approve", draw: "d-1", operator: "staff:" + opID}, false},
		{[]string{"draws", "void", "-draw", "d-1", "-operator", opID}, command{}, true}, // no reason
		{[]string{"draws", "void", "-draw", "d-1", "-reason", "wrong entries", "-operator", opID},
			command{name: "draws void", draw: "d-1", reason: "wrong entries", operator: "staff:" + opID}, false},
		{[]string{"draws", "entries", "-draw", "d-1"}, command{name: "draws entries", draw: "d-1"}, false},
		{[]string{"draws", "entries"}, command{}, true}, // which draw
		{[]string{"draws", "reconcile"}, command{name: "draws reconcile"}, false},
		{[]string{"draws", "overdue"}, command{name: "draws overdue"}, false},
		{[]string{"draws", "overdue", "-draw", "d-1"}, command{}, true}, // takes no flags
		{[]string{"payouts", "list", "-state", "manual"}, command{name: "payouts list", state: "manual"}, false},
		{[]string{"payouts", "list"}, command{}, true},
		{[]string{"payouts", "retry-prize", "-result", "r-1", "-operator", opID}, command{name: "payouts retry-prize", result: "r-1", operator: "staff:" + opID}, false},
		{[]string{"payouts", "retry-prize", "-result", "r-1"}, command{}, true}, // no operator
		{[]string{"payouts", "reconcile"}, command{name: "payouts reconcile"}, false},
		{[]string{"tax", "export"}, command{}, true},
		{[]string{"tax", "batch", "-operator", opID}, command{name: "tax batch", operator: "staff:" + opID}, false},
		{[]string{"tax", "batch"}, command{}, true},
		{[]string{"tax", "file", "-batch", "3"}, command{name: "tax file", batch: 3}, false},
		{[]string{"tax", "file", "-batch", "-1"}, command{}, true},
		{[]string{"tax", "remit", "-batch", "3", "-prn", "2026X", "-amount", "340", "-operator", opID},
			command{name: "tax remit", batch: 3, prn: "2026X", amount: 340, operator: "staff:" + opID}, false},
		{[]string{"tax", "remit", "-batch", "3", "-prn", "2026X", "-operator", opID}, command{}, true},
		{[]string{"tax", "batches"}, command{name: "tax batches"}, false},
		{[]string{"alerts", "list"}, command{name: "alerts list"}, false},
		{[]string{"alerts", "list", "-all"}, command{name: "alerts list", all: true}, false},
		{[]string{"alerts", "ack", "-alert", "a1", "-note", "checked", "-operator", opID},
			command{name: "alerts ack", alert: "a1", note: "checked", operator: "staff:" + opID}, false},
		{[]string{"alerts", "ack", "-alert", "a1", "-operator", opID}, command{}, true},
		{[]string{"alerts", "watch", "-all"}, command{}, true},
		{[]string{"tax", "export", "-state", "x"}, command{}, true},
		{[]string{"payouts", "approve", "-payout", "p-1", "-operator", opID}, command{name: "payouts approve", payout: "p-1", operator: "staff:" + opID}, false},
		{[]string{"payouts", "reject", "-payout", "p-1", "-operator", opID}, command{}, true}, // no reason
		{[]string{"payouts", "settle-paid", "-payout", "p-1", "-receipt", "MBC0000001", "-operator", opID},
			command{name: "payouts settle-paid", payout: "p-1", receipt: "MBC0000001", operator: "staff:" + opID}, false},
		{[]string{"payouts", "settle-paid", "-payout", "p-1", "-operator", opID}, command{}, true}, // no receipt
		{[]string{"payouts", "settle-failed", "-payout", "p-1", "-reason", "not on the statement", "-operator", opID},
			command{name: "payouts settle-failed", payout: "p-1", reason: "not on the statement", operator: "staff:" + opID}, false},
		{[]string{"bogus"}, command{}, true},
		{nil, command{}, true},
	}
	for _, tc := range tests {
		got, err := parse(tc.args)
		if (err != nil) != tc.wantErr || (err == nil && !reflect.DeepEqual(got, tc.want)) {
			t.Errorf("parse(%v) = %+v, %v; want %+v, err=%v", tc.args, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestRun_GameSimulateNeedsNoDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var out bytes.Buffer
	if err := run(context.Background(), command{name: "game simulate", days: 2000}, &out); err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Slots int   `json:"slots"`
		Price int64 `json:"price_kes"`
		Rows  []any `json:"by_plays_per_day"`
	}
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || rep.Slots != 12 || rep.Price != 100 || len(rep.Rows) != 4 {
		t.Fatalf("report %s (%v)", out.String(), err)
	}
	if err := run(context.Background(), command{name: "game simulate", slots: 3, lucky: 3}, &out); err == nil {
		t.Fatal("simulated an impossible odds version")
	}
}

func TestParse_TheOperatorIsAStaffIDSoBothPathsOfATwoPersonRuleNameTheSamePerson(t *testing.T) {
	for _, bad := range []string{"willis", "operator:willis", "staff:" + opID, "0192F000-0000-7000-8000-00000000A11C"} {
		if _, err := parse([]string{"payments", "reverse", "-receipt", "R", "-reason", "x", "-operator", bad}); err == nil {
			t.Errorf("-operator %q accepted", bad)
		}
	}
	c, err := parse([]string{"payments", "reverse", "-receipt", "R", "-reason", "x", "-operator", opID})
	if err != nil || c.operator != "staff:"+opID {
		t.Fatalf("%+v %v", c, err)
	}
}
