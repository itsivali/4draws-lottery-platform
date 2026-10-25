package alerts_test

import (
	"strings"
	"testing"

	"github.com/itsivali/lottery-platform/internal/alerts"
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
