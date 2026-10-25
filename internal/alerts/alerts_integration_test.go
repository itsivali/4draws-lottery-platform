//go:build integration

package alerts_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/alerts"
	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/testpg"
)

var start = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

// inbox records what was sent; with fail set it refuses.
type inbox struct {
	mu   sync.Mutex
	got  []alerts.Message
	fail bool
}

func (b *inbox) Notify(_ context.Context, m alerts.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fail {
		return errors.New("channel down")
	}
	b.got = append(b.got, m)
	return nil
}

// take returns and forgets what was sent, as "event kind" strings.
func (b *inbox) take() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.got))
	for _, m := range b.got {
		out = append(out, m.Event+" "+m.Alert.Kind)
	}
	b.got = nil
	return out
}

type env struct {
	pool  *pgxpool.Pool
	clk   *clock.Manual
	inbox *inbox
	beats int
}

func setup(t *testing.T) *env {
	t.Helper()
	return &env{pool: testpg.Pool(t, "app_worker"), clk: clock.NewManual(start), inbox: &inbox{}}
}

func (e *env) service(checks ...alerts.Check) *alerts.Service {
	return alerts.NewService(e.pool, e.clk, e.inbox, audit.NewWriter(), alerts.Config{
		Heartbeat: func(context.Context) error { e.beats++; return nil },
	}, checks...)
}

// firing is a check whose alerts the test sets.
type firing struct {
	alerts []alerts.Alert
	err    error
}

func (f *firing) check(name string, kinds ...string) alerts.Check {
	return alerts.Check{Name: name, Kinds: kinds, Run: func(context.Context) ([]alerts.Alert, error) { return f.alerts, f.err }}
}

func same(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, g := range got {
		seen[g]++
	}
	for _, w := range want {
		seen[w]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func TestWatch_RaisesOnceRemindsPagesAndResolvesWhatClears(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	f := &firing{alerts: []alerts.Alert{
		{Kind: "test.page", Subject: "a", Severity: alerts.Page, Summary: "page a"},
		{Kind: "test.warn", Subject: "b", Severity: alerts.Warn, Summary: "warn b", Details: map[string]any{"n": 1}},
	}}
	svc := e.service(f.check("test", "test.page", "test.warn"))
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "raised test.page", "raised test.warn") {
		t.Fatalf("first watch sent %v", got)
	}
	// still firing: counted, not sent again
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); len(got) != 0 {
		t.Fatalf("sent again %v", got)
	}
	list, err := svc.List(ctx, false, 10)
	if err != nil || len(list) != 2 || list[0].Occurrences != 2 {
		t.Fatalf("list %+v %v", list, err)
	}
	// an hour on, the unacknowledged page is sent again; the warning is not
	e.clk.Advance(61 * time.Minute)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "reminder test.page") {
		t.Fatalf("reminders %v", got)
	}
	var page string
	for _, a := range list {
		if a.Kind == "test.page" {
			page = a.ID
		}
	}
	if _, err := svc.Acknowledge(ctx, page, "operator:alice", " "); !errors.Is(err, alerts.ErrOperator) {
		t.Fatalf("no note: %v", err)
	}
	a, err := svc.Acknowledge(ctx, page, "operator:alice", "looking into it")
	if err != nil || a.AcknowledgedBy != "operator:alice" || a.ResolvedAt != nil {
		t.Fatalf("ack %+v %v", a, err)
	}
	if _, err := svc.Acknowledge(ctx, page, "operator:bob", "me too"); !errors.Is(err, alerts.ErrAcknowledged) {
		t.Fatalf("acknowledged twice: %v", err)
	}
	e.clk.Advance(61 * time.Minute)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); len(got) != 0 {
		t.Fatalf("reminded after the acknowledgement: %v", got)
	}
	// the conditions clear: both are resolved and the resolutions sent
	f.alerts = nil
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "resolved test.page", "resolved test.warn") {
		t.Fatalf("resolutions %v", got)
	}
	if open, _ := svc.List(ctx, false, 10); len(open) != 0 {
		t.Fatalf("still open %+v", open)
	}
	if all, _ := svc.List(ctx, true, 10); len(all) != 2 || all[0].ResolvedBy != "system" {
		t.Fatalf("history %+v", all)
	}
	if _, err := svc.Acknowledge(ctx, list[1].ID, "operator:alice", "late"); !errors.Is(err, alerts.ErrClosed) && !errors.Is(err, alerts.ErrAcknowledged) {
		t.Fatalf("acknowledged a resolved alert: %v", err)
	}
	// firing again later is a new alert
	f.alerts = []alerts.Alert{{Kind: "test.page", Subject: "a", Severity: alerts.Page, Summary: "page a again"}}
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "raised test.page") {
		t.Fatalf("raised again %v", got)
	}
	if e.beats != 6 {
		t.Fatalf("heartbeats %d", e.beats)
	}
}

func TestWatch_AFreezeIsClosedOnlyByAPerson(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	f := &firing{alerts: []alerts.Alert{{Kind: "test.books", Subject: "run-1", Severity: alerts.Page, FreezesPayouts: true, Summary: "books"}}}
	svc := e.service(f.check("books", "test.books"))
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	msgs := e.inbox.got
	if len(msgs) != 1 || !strings.Contains(msgs[0].Text, "Payouts are frozen") || !strings.Contains(msgs[0].Text, "lotteryctl alerts ack -alert "+msgs[0].Alert.ID) {
		t.Fatalf("message %+v", msgs)
	}
	e.inbox.take()
	// a later clean run does not close it
	f.alerts = nil
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if frozen(t, e) != true {
		t.Fatal("the freeze closed without a person")
	}
	open, _ := svc.List(ctx, false, 10)
	a, err := svc.Acknowledge(ctx, open[0].ID, "operator:alice", "a rounding bug in the export, fixed in #40")
	if err != nil || a.ResolvedAt == nil || a.ResolvedBy != "operator:alice" {
		t.Fatalf("ack %+v %v", a, err)
	}
	if frozen(t, e) {
		t.Fatal("still frozen after the acknowledgement")
	}
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "resolved test.books") {
		t.Fatalf("resolution %v", got)
	}
	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM audit.audit_log WHERE action = 'alert.acknowledged' AND entity_id = $1`, a.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit %d %v", n, err)
	}
	for _, id := range []string{"0192f000-0000-7000-8000-000000000000", "not-a-uuid"} {
		if _, err := svc.Acknowledge(ctx, id, "operator:alice", "x"); !errors.Is(err, alerts.ErrNotFound) {
			t.Errorf("%s: %v", id, err)
		}
	}
}

func frozen(t *testing.T, e *env) bool {
	t.Helper()
	var f bool
	if err := e.pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM audit.alerts WHERE freezes = 'payouts' AND resolved_at IS NULL)`).Scan(&f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestWatch_ACheckThatCannotRunIsAnAlertAndClearsNothing(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	f := &firing{alerts: []alerts.Alert{{Kind: "test.page", Subject: "a", Severity: alerts.Page, Summary: "page"}}}
	svc := e.service(f.check("flaky", "test.page"))
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	e.inbox.take()
	f.alerts, f.err = nil, errors.New("database away")
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "raised alerts.check_failed") {
		t.Fatalf("sent %v", got)
	}
	if open, _ := svc.List(ctx, false, 10); len(open) != 2 {
		t.Fatalf("the failing check cleared its alerts: %+v", open)
	}
	if e.beats != 1 {
		t.Fatalf("a heartbeat while a check failed: %d", e.beats)
	}
	f.err = nil
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "resolved alerts.check_failed", "resolved test.page") {
		t.Fatalf("after recovery %v", got)
	}
}

func TestDeliver_WhatCannotBeSentIsSentAtTheNextWatch(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	f := &firing{alerts: []alerts.Alert{{Kind: "test.warn", Subject: "a", Severity: alerts.Warn, Summary: "warn"}}}
	svc := e.service(f.check("test", "test.warn"))
	e.inbox.fail = true
	if err := svc.Watch(ctx); err == nil {
		t.Fatal("an undelivered alert was not reported")
	}
	if e.beats != 0 {
		t.Fatal("a heartbeat while alerts could not be sent")
	}
	e.inbox.fail = false
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "raised test.warn") {
		t.Fatalf("retried %v", got)
	}
	// without a notifier alerts are only logged, and count as sent
	logged := alerts.NewService(e.pool, e.clk, nil, audit.NewWriter(), alerts.Config{},
		(&firing{alerts: []alerts.Alert{{Kind: "test.log", Subject: "x", Severity: alerts.Warn, Summary: "logged"}}}).check("log", "test.log"))
	if err := logged.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ := logged.List(ctx, false, 10)
	for _, a := range list {
		if a.Kind == "test.log" && a.Notifications != 1 {
			t.Fatalf("logged %+v", a)
		}
	}
}

func TestReconciliationCheck_FailedRunsFreezePayoutsAndMissingRunsPage(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	svc := e.service(alerts.ReconciliationCheck(e.pool, e.clk, 26*time.Hour))
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); len(got) != 4 || got[0] != "raised reconciliation.missing" {
		t.Fatalf("never run %v", got)
	}
	run := func(schema string, ok bool, report string) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, `INSERT INTO `+schema+`.reconciliation_runs (ran_at, ok, report) VALUES ($1, $2, $3)`, now(t, e), ok, report); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range alerts.Reconciliations {
		run(s, true, `{}`)
	}
	e.clk.Advance(time.Second)
	run("payouts", false, `{"mismatches": {"tax_payable": ["x"], "b2c_float": ["y"]}}`)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got := e.inbox.got
	e.inbox.take()
	raised := 0
	for _, m := range got {
		if m.Event == "raised" {
			raised++
			if m.Alert.Kind != alerts.KindReconciliationFailed || !m.Alert.FreezesPayouts || !strings.Contains(m.Alert.Summary, "b2c_float, tax_payable") {
				t.Fatalf("failed run alert %+v", m.Alert)
			}
		}
	}
	if raised != 1 || len(got) != 5 || !frozen(t, e) {
		t.Fatalf("sent %+v", got)
	}
	// a clean run after it leaves the freeze for a person; a day without runs pages again
	e.clk.Advance(time.Second)
	run("payouts", true, `{}`)
	e.clk.Advance(27 * time.Hour)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	// four stale reconciliations, and a reminder of the unacknowledged freeze
	if got := e.inbox.take(); len(got) != 5 || !same(got[:1], "reminder reconciliation.failed") || !frozen(t, e) {
		t.Fatalf("stale %v frozen %v", got, frozen(t, e))
	}
}

func TestRemittanceCheck_WarnsThenPagesUntilTheTaxIsRemitted(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	svc := e.service(alerts.RemittanceCheck(e.pool, e.clk, 72*time.Hour, 120*time.Hour))
	var event int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO audit.kra_outbox (event_type, payload, created_at) VALUES ('withholding_tax', '{"tax_kes": 21, "gross_kes": 105}', $1)
		RETURNING id`, start).Scan(&event); err != nil {
		t.Fatal(err)
	}
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); len(got) != 0 {
		t.Fatalf("too early %v", got)
	}
	e.clk.Advance(73 * time.Hour)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "raised "+alerts.KindRemittanceDue) {
		t.Fatalf("due %v", got)
	}
	e.clk.Advance(48 * time.Hour)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "raised "+alerts.KindRemittanceOverdue, "resolved "+alerts.KindRemittanceDue) {
		t.Fatalf("overdue %v", got)
	}
	// remitted: the alert clears
	var batch string
	if err := e.pool.QueryRow(ctx, `INSERT INTO audit.kra_batches (batch_no, events, first_event_at, last_event_at, gross_kes, tax_kes, file_sha256,
		prepared_by, prepared_at, prn, remitted_kes, remitted_by, remitted_at)
		VALUES (1, 1, $1, $1, 105, 21, repeat('a', 64), 'operator:alice', $1, 'PRN', 21, 'operator:bob', $1) RETURNING id`, start).Scan(&batch); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE audit.kra_outbox SET batch_id = $1, exported_at = now() WHERE id = $2`, batch, event); err != nil {
		t.Fatal(err)
	}
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "resolved "+alerts.KindRemittanceOverdue) {
		t.Fatalf("remitted %v", got)
	}
}

func TestDrawsOverdue_PagesPerLateDrawUntilItMovesOn(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	late := []draws.Late{{DrawID: "d1", DrawType: "daily", PeriodStart: "2026-10-24", Status: "conducted", Due: start, Problem: "awaits approval"}}
	svc := e.service(alerts.DrawsOverdue(func(context.Context) ([]draws.Late, error) { return late, nil }))
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got := e.inbox.got
	if len(got) != 1 || got[0].Alert.Subject != "d1/conducted" || !strings.Contains(got[0].Text, "awaits approval") || got[0].Alert.Severity != alerts.Page {
		t.Fatalf("sent %+v", got)
	}
	e.inbox.take()
	late = nil
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "resolved "+alerts.KindDrawOverdue) {
		t.Fatalf("moved on %v", got)
	}
}

func TestAlerts_TheDatabaseKeepsWhatWasRaisedAndAcknowledged(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	svc := e.service((&firing{alerts: []alerts.Alert{{Kind: "test.page", Subject: "a", Severity: alerts.Page, Summary: "p"}}}).check("t", "test.page"))
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	open, _ := svc.List(ctx, false, 10)
	if _, err := svc.Acknowledge(ctx, open[0].ID, "operator:alice", "mine"); err != nil {
		t.Fatal(err)
	}
	for name, sql := range map[string]string{
		"deleted":          `DELETE FROM audit.alerts WHERE id = $1`,
		"kind changed":     `UPDATE audit.alerts SET kind = 'other' WHERE id = $1`,
		"severity lowered": `UPDATE audit.alerts SET severity = 'warn' WHERE id = $1`,
		"ack rewritten":    `UPDATE audit.alerts SET ack_note = 'someone else' WHERE id = $1`,
	} {
		if _, err := e.pool.Exec(ctx, sql, open[0].ID); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestWebhookAndHeartbeat(t *testing.T) {
	ctx := context.Background()
	var body string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = r.Method + " " + string(raw)
		w.WriteHeader(status)
	}))
	defer srv.Close()
	m := alerts.Message{Event: "raised", Text: "[PAGE] books", Alert: alerts.View{ID: "a1", Kind: "k"}}
	hook := alerts.Webhook{URL: srv.URL + "/secret-path", Client: srv.Client()}
	if err := hook.Notify(ctx, m); err != nil || !strings.HasPrefix(body, "POST ") || !strings.Contains(body, `"text":"[PAGE] books"`) {
		t.Fatalf("webhook %v %s", err, body)
	}
	beat := alerts.Heartbeat(srv.URL+"/ping", srv.Client())
	if err := beat(ctx); err != nil || !strings.HasPrefix(body, "GET ") {
		t.Fatalf("heartbeat %v %s", err, body)
	}
	status = http.StatusInternalServerError
	if err := hook.Notify(ctx, m); err == nil || strings.Contains(err.Error(), "secret-path") {
		t.Fatalf("a refused webhook: %v", err)
	}
	if err := beat(ctx); err == nil {
		t.Fatal("a refused heartbeat")
	}
	srv.Close()
	if err := hook.Notify(ctx, m); err == nil || strings.Contains(err.Error(), "secret-path") {
		t.Fatalf("an unreachable webhook: %v", err)
	}
	if err := (alerts.Webhook{URL: "://bad", Client: http.DefaultClient}).Notify(ctx, m); err == nil {
		t.Fatal("a bad URL")
	}
	if err := alerts.Heartbeat("://bad", http.DefaultClient)(ctx); err == nil {
		t.Fatal("a bad heartbeat URL")
	}
}

func now(t *testing.T, e *env) time.Time {
	t.Helper()
	at, err := e.clk.Now(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return at
}
