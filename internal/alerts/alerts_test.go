package alerts_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/alerts"
	"github.com/itsivali/lottery-platform/internal/platform/mail"
)

func TestText_SaysWhatToDo(t *testing.T) {
	freeze := alerts.View{ID: "a1", Kind: "reconciliation.failed", Subject: "payouts/r", Severity: alerts.Page, FreezesPayouts: true, Summary: "books"}
	page := alerts.View{ID: "a2", Kind: "draw.overdue", Subject: "d/signed", Severity: alerts.Page, Summary: "late"}
	warn := alerts.View{ID: "a3", Kind: "kra.remittance_due", Subject: "withholding", Severity: alerts.Warn, Summary: "remit"}
	for _, tc := range []struct {
		event string
		a     alerts.View
		has   []string
		not   []string
	}{
		{"raised", freeze, []string{"[PAGE] books", "Payouts are frozen", "alerts ack -alert a1"}, nil},
		{"reminder", page, []string{"[REMINDER] [PAGE] late", "alerts ack -alert a2"}, []string{"frozen"}},
		{"raised", warn, []string{"[WARN] remit"}, []string{"ack"}},
		{"resolved", freeze, []string{"[RESOLVED] [PAGE] books"}, []string{"frozen", "ack"}},
	} {
		got := alerts.Text(tc.event, tc.a)
		for _, h := range tc.has {
			if !strings.Contains(got, h) {
				t.Errorf("%q lacks %q", got, h)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(got, n) {
				t.Errorf("%q has %q", got, n)
			}
		}
	}
}

func TestJobs(t *testing.T) {
	if (alerts.WatchArgs{}).Kind() != "alerts.watch" || len(alerts.PeriodicJobs()) != 1 {
		t.Fatal("jobs")
	}
}

func TestEmail_SubjectAndBodySayWhatWhereAndHow(t *testing.T) {
	first := time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)
	a := alerts.View{ID: "a9", Kind: "topup.unconfirmed", Subject: "p1", Severity: alerts.Page, Summary: "A top-up is not confirmed",
		Details:     json.RawMessage(`{"amount_kes": 50, "msisdn": "254705***001", "reason": "still_pending_at_expiry\nsecond line"}`),
		FirstSeenAt: first, LastSeenAt: first.Add(time.Hour), Occurrences: 13}
	e := alerts.Email{BackOfficeURL: "https://backoffice.example.com/"}
	m := alerts.Message{Event: "raised", Alert: a, Text: alerts.Text("raised", a)}
	if got := e.Subject(m); got != "[Tajirika] PAGE topup.unconfirmed: A top-up is not confirmed" {
		t.Fatalf("subject %q", got)
	}
	if got := e.Subject(alerts.Message{Event: "escalated", Alert: a}); !strings.HasPrefix(got, "[Tajirika] ESCALATED PAGE ") {
		t.Fatalf("escalated subject %q", got)
	}
	if got := e.Subject(alerts.Message{Event: "resolved", Alert: a}); !strings.HasPrefix(got, "[Tajirika] RESOLVED PAGE ") {
		t.Fatalf("resolved subject %q", got)
	}
	body := e.Body(m)
	for _, want := range []string{"[PAGE] A top-up is not confirmed", "Kind:       topup.unconfirmed", "13 occurrence(s)", "amount_kes: 50",
		"msisdn: 254705***001", "reason: still_pending_at_expiry second line", "https://backoffice.example.com/alerts?alert=a9", "docs/runbooks/alerts.md"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(alerts.Email{}.Body(m), "alerts?alert=") {
		t.Fatal("a link without a back office URL")
	}
	// a subject with a line break in the summary stays one line
	a.Summary = "two\nlines"
	if got := e.Subject(alerts.Message{Event: "raised", Alert: a}); strings.ContainsAny(got, "\r\n") {
		t.Fatalf("subject %q", got)
	}
}

type fakeSender struct {
	sent []mail.Message
	err  error
}

func (f *fakeSender) Send(_ context.Context, m mail.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

type fakeNotifier struct {
	got int
	err error
}

func (f *fakeNotifier) Notify(context.Context, alerts.Message) error {
	if f.err != nil {
		return f.err
	}
	f.got++
	return nil
}

func TestMulti_EveryChannelGetsTheEventAndOneFailureDoesNotStopTheOthers(t *testing.T) {
	ctx := context.Background()
	m := alerts.Message{Event: "raised", Alert: alerts.View{ID: "a1", Kind: "k", Severity: alerts.Warn}}
	sender := &fakeSender{}
	ok := &fakeNotifier{}
	down := &fakeNotifier{err: errors.New("webhook unreachable")}
	multi := alerts.Multi{down, alerts.Email{Sender: sender, To: []string{"ops@example.com", "owner@example.com"}}, ok}
	if err := multi.Notify(ctx, m); err != nil {
		t.Fatalf("one channel down: %v", err)
	}
	if ok.got != 1 || len(sender.sent) != 1 || len(sender.sent[0].To) != 2 {
		t.Fatalf("delivered %d emails %+v", ok.got, sender.sent)
	}
	sender.err = errors.New("smtp down")
	if err := (alerts.Multi{down, alerts.Email{Sender: sender, To: []string{"ops@example.com"}}}).Notify(ctx, m); err == nil {
		t.Fatal("every channel down: no error")
	}
	if err := (alerts.Email{Sender: sender}).Notify(ctx, m); err != nil {
		t.Fatalf("no addresses: %v", err)
	}
	if err := (alerts.Multi{}).Notify(ctx, m); err != nil {
		t.Fatalf("empty multi logs: %v", err)
	}
}

func TestParseState(t *testing.T) {
	for in, want := range map[string]alerts.State{"": alerts.Open, "open": alerts.Open, "acknowledged": alerts.Acknowledged, "resolved": alerts.Resolved, "all": alerts.All} {
		if got, ok := alerts.ParseState(in); !ok || got != want {
			t.Errorf("%q: %q %v", in, got, ok)
		}
	}
	if _, ok := alerts.ParseState("closed"); ok {
		t.Error("closed accepted")
	}
}

func TestDefaultThresholds(t *testing.T) {
	th := alerts.DefaultThresholds()
	if th.TopUpUnconfirmedWarn != 30*time.Minute || th.TopUpUnconfirmedPage != 4*time.Hour || th.SuspensePage != 24*time.Hour ||
		th.JobRetryWarnAttempts != 5 || th.KYCQueuePage != 72*time.Hour || th.PayoutApprovalWarn != 2*time.Hour || th.B2CFloatFloorKES != 0 ||
		th.ClosureWaitWarn != 7*24*time.Hour || th.DarajaThrottleWindow != 15*time.Minute || th.DarajaThrottleCount != 5 {
		t.Fatalf("%+v", th)
	}
}
