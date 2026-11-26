//go:build integration

package alerts_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/alerts"
)

func TestWatch_AWarningThatWaitsTooLongBecomesAPageAndIsSentAgain(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	f := &firing{alerts: []alerts.Alert{{Kind: "test.wait", Subject: "a", Severity: alerts.Warn, Summary: "waiting 1 h", Details: map[string]any{"age": "1 h"}}}}
	svc := e.service(f.check("test", "test.wait"))
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "raised test.wait") {
		t.Fatalf("first %v", got)
	}
	e.clk.Advance(3 * time.Hour)
	f.alerts[0].Severity, f.alerts[0].Summary, f.alerts[0].Details = alerts.Page, "waiting 4 h", map[string]any{"age": "4 h"}
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	msgs := e.inbox.got
	if len(msgs) != 1 || msgs[0].Event != "escalated" || msgs[0].Alert.Severity != alerts.Page || !strings.HasPrefix(msgs[0].Text, "[ESCALATED] [PAGE] waiting 4 h") {
		t.Fatalf("escalation %+v", msgs)
	}
	e.inbox.take()
	open, _ := openAlerts(svc)
	if len(open) != 1 || open[0].Severity != alerts.Page || open[0].Occurrences != 2 || open[0].Notifications != 2 {
		t.Fatalf("after escalation %+v", open)
	}
	var details map[string]any
	if err := json.Unmarshal(open[0].Details, &details); err != nil || details["escalated_from"] != "warn" || details["age"] != "4 h" || details["escalated_at"] == nil {
		t.Fatalf("details %s", open[0].Details)
	}
	// still a page an hour on: a reminder, and the escalation record survives the refresh of the details
	e.clk.Advance(61 * time.Minute)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "reminder test.wait") {
		t.Fatalf("reminder %v", got)
	}
	// raised again as a warning, it stays a page
	f.alerts[0].Severity = alerts.Warn
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	d, err := svc.Get(ctx, open[0].ID)
	if err != nil || d.Severity != alerts.Page || d.Check != "test" || len(d.History) != 2 || d.History[0].Event != "raised" || d.History[1].Event != "escalated" {
		t.Fatalf("detail %+v %v", d, err)
	}
	if err := json.Unmarshal(d.Details, &details); err != nil || details["escalated_from"] != "warn" {
		t.Fatalf("details after the refresh %s", d.Details)
	}
	if _, err := svc.Get(ctx, "not-a-uuid"); !errors.Is(err, alerts.ErrNotFound) {
		t.Fatalf("bad id: %v", err)
	}
}

func TestList_FiltersPagesAndCountsKinds(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	var fired []alerts.Alert
	for i, sub := range []string{"a", "b", "c", "d", "e"} {
		sev := alerts.Warn
		if i%2 == 0 {
			sev = alerts.Page
		}
		fired = append(fired, alerts.Alert{Kind: "test.k" + strings.Repeat("x", i%2), Subject: sub, Severity: sev, Summary: "s " + sub})
	}
	f := &firing{}
	svc := e.service(f.check("test", "test.k", "test.kx"), (&firing{}).check("quiet", "test.quiet"))
	// Raise them a minute apart, a first, so "newest first" has one right answer: alerts raised in the same Watch
	// share first_seen_at, and ids made in the same millisecond do not sort in creation order.
	for i := range fired {
		f.alerts = fired[:i+1]
		if err := svc.Watch(ctx); err != nil {
			t.Fatal(err)
		}
		e.clk.Advance(time.Minute)
	}
	all, _ := openAlerts(svc)
	if len(all) != 5 {
		t.Fatalf("open %+v", all)
	}
	// acknowledge one, resolve another
	if _, err := svc.Acknowledge(ctx, all[0].ID, "operator:alice", "mine"); err != nil {
		t.Fatal(err)
	}
	f.alerts = fired[1:]
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		f    alerts.Filter
		want int
	}{
		"open":         {alerts.Filter{}, 4},
		"acknowledged": {alerts.Filter{State: alerts.Acknowledged}, 1},
		"resolved":     {alerts.Filter{State: alerts.Resolved}, 1},
		"all":          {alerts.Filter{State: alerts.All}, 5},
		"pages":        {alerts.Filter{Severity: alerts.Page}, 2},
		"warn of kind": {alerts.Filter{Kind: "test.kx", Severity: alerts.Warn}, 2},
		"no such kind": {alerts.Filter{Kind: "nope"}, 0},
	} {
		l, err := svc.List(ctx, tc.f)
		if err != nil || len(l.Alerts) != tc.want || l.NextBefore != "" {
			t.Errorf("%s: %d alerts (want %d), next %q, %v", name, len(l.Alerts), tc.want, l.NextBefore, err)
		}
	}
	// pages of two, newest first, continued with the cursor
	var seen []string
	before := ""
	for pages := 0; pages < 5; pages++ {
		l, err := svc.List(ctx, alerts.Filter{State: alerts.All, Limit: 2, Before: before})
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range l.Alerts {
			seen = append(seen, a.Subject)
		}
		if l.NextBefore == "" {
			break
		}
		before = l.NextBefore
	}
	if strings.Join(seen, "") != "edcba" {
		t.Fatalf("paged %v", seen)
	}
	if _, err := svc.List(ctx, alerts.Filter{Before: "yesterday|x"}); !errors.Is(err, alerts.ErrCursor) {
		t.Fatalf("bad cursor: %v", err)
	}
	kinds, err := svc.Kinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, k := range kinds {
		got[k.Kind] = k.Open
	}
	if len(kinds) != 4 || got["test.k"] != 2 || got["test.kx"] != 2 || got["test.quiet"] != 0 || got["alerts.check_failed"] != 0 {
		t.Fatalf("kinds %+v", kinds)
	}
}

// seed makes a player and returns their id.
func seed(t *testing.T, e *env, msisdn string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(), `INSERT INTO identity.users (msisdn) VALUES ($1) RETURNING id::text`, msisdn).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func exec(t *testing.T, e *env, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// asAPI runs one statement as the api's role (which, unlike the worker's, may insert closure requests).
func asAPI(t *testing.T, e *env, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := e.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET ROLE app_api"); err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, sql, args...)
	if _, rerr := conn.Exec(ctx, "SET ROLE app_worker"); rerr != nil {
		t.Fatal(rerr)
	}
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func kindsOf(msgs []alerts.Message) map[string]alerts.Message {
	out := map[string]alerts.Message{}
	for _, m := range msgs {
		out[m.Event+" "+m.Alert.Kind] = m
	}
	return out
}

func TestTopUpsCheck_UnconfirmedWarnsThenPagesStuckPendingPagesOrphansWarn(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	th := alerts.DefaultThresholds()
	svc := e.service(alerts.TopUpsCheck(e.pool, e.clk, 15*time.Minute, th))
	user := seed(t, e, "254705000001")
	at := now(t, e)
	var unconfirmed, pending string
	if err := e.pool.QueryRow(ctx, `INSERT INTO payments.payments (user_id, channel, status, amount_requested_kes, msisdn, idempotency_key, failure_reason, created_at, updated_at)
		VALUES ($1, 'stk', 'unconfirmed', 50, '254705000001', 'k1', 'still_pending_at_expiry', $2, $2) RETURNING id::text`, user, at.Add(-20*time.Minute)).Scan(&unconfirmed); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, `INSERT INTO payments.payments (user_id, channel, status, amount_requested_kes, msisdn, idempotency_key, created_at, updated_at)
		VALUES ($1, 'stk', 'stk_sent', 20, '254705000001', 'k2', $2, $2) RETURNING id::text`, user, at.Add(-20*time.Minute)).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	exec(t, e, `INSERT INTO payments.stk_orphan_callbacks (checkout_request_id, raw_payload, received_at) VALUES ('ws_CO_1', '{}', $1), ('ws_CO_1', '{}', $1)`, at.Add(-time.Hour))
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got := kindsOf(e.inbox.got)
	e.inbox.take()
	if len(got) != 1 {
		t.Fatalf("too early for the unconfirmed one and the pending one: %v", got)
	}
	orphan := got["raised "+alerts.KindTopUpOrphanCallbacks]
	if orphan.Alert.Subject != "ws_CO_1" || orphan.Alert.Severity != alerts.Warn || !strings.Contains(orphan.Alert.Summary, "2 STK callback(s)") {
		t.Fatalf("orphans %+v", orphan.Alert)
	}
	e.clk.Advance(11 * time.Minute) // 31 min: the unconfirmed one warns; 15 + 10 min: the pending one is stuck
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got = kindsOf(e.inbox.got)
	e.inbox.take()
	u := got["raised "+alerts.KindTopUpUnconfirmed]
	if u.Alert.Subject != unconfirmed || u.Alert.Severity != alerts.Warn || !strings.Contains(u.Alert.Summary, "KES 50 from 254705***001") || !strings.Contains(u.Alert.Summary, "31 min") {
		t.Fatalf("unconfirmed %+v", u.Alert)
	}
	var details map[string]any
	if json.Unmarshal(u.Alert.Details, &details) != nil || details["reason"] != "still_pending_at_expiry" || details["msisdn"] != "254705***001" {
		t.Fatalf("details %s", u.Alert.Details)
	}
	p := got["raised "+alerts.KindTopUpOverduePending]
	if p.Alert.Subject != pending || p.Alert.Severity != alerts.Page || !strings.Contains(p.Alert.Summary, "still stk_sent") {
		t.Fatalf("pending %+v", p.Alert)
	}
	// four hours on, the unconfirmed one pages; the pending one got settled and the orphan is older than a day
	exec(t, e, `UPDATE payments.payments SET status = 'unconfirmed', failure_reason = 'still_pending_at_expiry' WHERE id = $1`, pending)
	e.clk.Advance(4 * time.Hour)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got = kindsOf(e.inbox.got)
	e.inbox.take()
	if esc := got["escalated "+alerts.KindTopUpUnconfirmed]; esc.Alert.Subject != unconfirmed || esc.Alert.Severity != alerts.Page {
		t.Fatalf("escalated %v", got)
	}
	if _, ok := got["resolved "+alerts.KindTopUpOverduePending]; !ok {
		t.Fatalf("pending settled %v", got)
	}
	if _, ok := got["raised "+alerts.KindTopUpUnconfirmed]; !ok { // the formerly pending one, now unconfirmed for 4 h 31 min
		t.Fatalf("second unconfirmed %v", got)
	}
	e.clk.Advance(20 * time.Hour)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "resolved "+alerts.KindTopUpOrphanCallbacks, "reminder "+alerts.KindTopUpUnconfirmed, "reminder "+alerts.KindTopUpUnconfirmed) {
		t.Fatalf("a day later %v", got)
	}
	// an operator settles them: the alerts clear
	exec(t, e, `UPDATE payments.payments SET status = 'failed' WHERE status = 'unconfirmed'`)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "resolved "+alerts.KindTopUpUnconfirmed, "resolved "+alerts.KindTopUpUnconfirmed) {
		t.Fatalf("settled %v", got)
	}
}

func TestPaybillCheck_SuspenseWarnsThenPagesAndUnverifiedWarns(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	svc := e.service(alerts.PaybillCheck(e.pool, e.clk, alerts.DefaultThresholds()))
	at := now(t, e)
	exec(t, e, `INSERT INTO payments.c2b_unmatched (mpesa_receipt, msisdn, account_ref, amount_kes, raw_payload, reason, created_at)
		VALUES ('RCPT00001', '254705000002', '0799', 300, '{}', 'no_such_account', $1)`, at.Add(-30*time.Minute))
	exec(t, e, `INSERT INTO payments.c2b_confirmations (trans_id, shortcode, bill_ref, amount_kes, raw_confirmation, status, outcome_reason, created_at)
		VALUES ('TJK1AB2CD3', '600982', '254705000002', 100, '{}', 'unverified', 'status query failed', $1)`, at.Add(-30*time.Minute))
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got := kindsOf(e.inbox.got)
	e.inbox.take()
	if len(got) != 1 || got["raised "+alerts.KindPaybillUnverified].Alert.Subject != "TJK1AB2CD3" {
		t.Fatalf("first %v", got)
	}
	e.clk.Advance(31 * time.Minute)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got = kindsOf(e.inbox.got)
	e.inbox.take()
	if s := got["raised "+alerts.KindSuspenseOpen]; s.Alert.Subject != "RCPT00001" || s.Alert.Severity != alerts.Warn || !strings.Contains(s.Alert.Summary, "KES 300") || !strings.Contains(s.Alert.Summary, "no_such_account") {
		t.Fatalf("suspense %+v", s.Alert)
	}
	e.clk.Advance(24 * time.Hour)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got = kindsOf(e.inbox.got)
	e.inbox.take()
	if s := got["escalated "+alerts.KindSuspenseOpen]; s.Alert.Severity != alerts.Page {
		t.Fatalf("a day in suspense %v", got)
	}
	exec(t, e, `UPDATE payments.c2b_unmatched SET status = 'refunded', refund_ref = 'R1', resolved_at = now(), resolved_by = 'operator:alice' WHERE mpesa_receipt = 'RCPT00001'`)
	exec(t, e, `UPDATE payments.c2b_confirmations SET status = 'status_requested' WHERE trans_id = 'TJK1AB2CD3'`)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "resolved "+alerts.KindSuspenseOpen, "resolved "+alerts.KindPaybillUnverified) {
		t.Fatalf("settled %v", got)
	}
}

func TestPayoutsCheck_AwaitingApprovalWarnsThenPagesAndFailuresPageForADay(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	svc := e.service(alerts.PayoutsCheck(e.pool, e.clk, alerts.DefaultThresholds()))
	user := seed(t, e, "254705000003")
	at := now(t, e)
	var held, failed string
	if err := e.pool.QueryRow(ctx, `INSERT INTO payouts.payouts (user_id, kind, status, gross_kes, net_kes, msisdn, originator_conversation_id, idempotency_key, reason, created_at, updated_at)
		VALUES ($1, 'withdrawal', 'held', 5000, 5000, '254705000003', 'oc-1', 'k1', 'above the automatic limit', $2, $2) RETURNING id::text`, user, at.Add(-time.Hour)).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, `INSERT INTO payouts.payouts (user_id, kind, status, gross_kes, net_kes, msisdn, originator_conversation_id, idempotency_key, reason, settled_at, created_at, updated_at)
		VALUES ($1, 'withdrawal', 'failed', 200, 200, '254705000003', 'oc-2', 'k2', 'B2C: insufficient float', $2, $2, $2) RETURNING id::text`, user, at.Add(-time.Hour)).Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got := kindsOf(e.inbox.got)
	e.inbox.take()
	if len(got) != 1 || got["raised "+alerts.KindPayoutFailed].Alert.Subject != failed || got["raised "+alerts.KindPayoutFailed].Alert.Severity != alerts.Page {
		t.Fatalf("first %v", got)
	}
	e.clk.Advance(time.Hour + time.Minute)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got = kindsOf(e.inbox.got)
	e.inbox.take()
	if a := got["raised "+alerts.KindPayoutAwaiting]; a.Alert.Subject != held || a.Alert.Severity != alerts.Warn || !strings.Contains(a.Alert.Summary, "KES 5000") || !strings.Contains(a.Alert.Summary, "for approval") {
		t.Fatalf("held %+v (%v)", a.Alert, got)
	}
	e.clk.Advance(24 * time.Hour)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got = kindsOf(e.inbox.got)
	e.inbox.take()
	if a := got["escalated "+alerts.KindPayoutAwaiting]; a.Alert.Severity != alerts.Page {
		t.Fatalf("a day held %v", got)
	}
	if _, ok := got["resolved "+alerts.KindPayoutFailed]; !ok {
		t.Fatalf("a day-old failure still pages %v", got)
	}
}

func TestOpsChecks_JobsThrottlingKYCQueueClosuresAndFloat(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	th := alerts.DefaultThresholds()
	th.B2CFloatFloorKES = 1000
	svc := e.service(alerts.JobsCheck(e.pool, e.clk, th), alerts.KYCQueueCheck(e.pool, e.clk, th), alerts.ClosuresCheck(e.pool, e.clk, th), alerts.B2CFloatCheck(e.pool, th))
	at := now(t, e)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got := kindsOf(e.inbox.got)
	e.inbox.take()
	if len(got) != 1 || !strings.Contains(got["raised "+alerts.KindB2CFloatLow].Alert.Summary, "KES 0, below the floor of KES 1000") {
		t.Fatalf("quiet system %v", got)
	}
	// jobs: one discarded, one retrying at attempt 5, and throttling in the STK errors
	errs := func(msg string, n int) string {
		parts := make([]string, 0, n)
		for i := 0; i < n; i++ {
			raw, _ := json.Marshal(map[string]any{"at": at.Add(-time.Minute), "attempt": i + 1, "error": msg})
			parts = append(parts, string(raw))
		}
		return "{" + strings.Join(quoted(parts), ",") + "}"
	}
	exec(t, e, `INSERT INTO river.river_job (kind, args, queue, state, max_attempts, priority, attempt, finalized_at, errors)
		VALUES ('payouts.send', '{}', 'default', 'discarded', 25, 1, 25, $1, $2::jsonb[])`, at, errs("daraja: refused the initiator", 1))
	exec(t, e, `INSERT INTO river.river_job (kind, args, queue, state, max_attempts, priority, attempt, errors)
		VALUES ('payments.stk_verify', '{}', 'default', 'retryable', 25, 1, 5, $1::jsonb[])`, errs("daraja: throttled", 5))
	// kyc: one account in manual review for 25 h; a closure open for 8 days
	user := seed(t, e, "254705000004")
	exec(t, e, `UPDATE identity.users SET kyc_status = 'manual_review' WHERE id = $1`, user)
	exec(t, e, `INSERT INTO kyc.kyc_checks (user_id, id_number_enc, id_number_hash, full_name_enc, date_of_birth_enc, status, provider, created_at)
		VALUES ($1, '\x00', '\x00', '\x00', '\x00', 'manual_review', 'manual', $2)`, user, at.Add(-25*time.Hour))
	asAPI(t, e, `INSERT INTO identity.closure_requests (user_id, requested_at) VALUES ($1, $2)`, user, at.Add(-8*24*time.Hour))
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	sent := len(e.inbox.got)
	got = kindsOf(e.inbox.got)
	e.inbox.take()
	if sent != 5 { // two job kinds, the throttling, the queue and the closure
		t.Fatalf("sent %d: %v", sent, got)
	}
	l, _ := svc.List(ctx, alerts.Filter{Kind: alerts.KindJobsFailing})
	bySubject := map[string]alerts.View{}
	for _, a := range l.Alerts {
		bySubject[a.Subject] = a
	}
	if a := bySubject["payouts.send"]; a.Severity != alerts.Page || !strings.Contains(a.Summary, "1 payouts.send job(s) were given up on") || !strings.Contains(a.Summary, "refused the initiator") {
		t.Fatalf("discarded %+v", a)
	}
	if a := bySubject["payments.stk_verify"]; a.Severity != alerts.Warn || !strings.Contains(a.Summary, "after 5 attempts") {
		t.Fatalf("retrying %+v", a)
	}
	if thr := got["raised "+alerts.KindDarajaThrottled]; thr.Alert.Severity != alerts.Warn || !strings.Contains(thr.Alert.Summary, "throttled 5 STK requests") {
		t.Fatalf("throttled %+v", thr.Alert)
	}
	if k := got["raised "+alerts.KindKYCQueueStale]; k.Alert.Severity != alerts.Warn || k.Alert.Subject != "review" || !strings.Contains(k.Alert.Summary, "1 identity submission(s)") || !strings.Contains(k.Alert.Summary, "25 h") {
		t.Fatalf("kyc %+v", k.Alert)
	}
	if c := got["raised "+alerts.KindClosureWaiting]; c.Alert.Severity != alerts.Warn || c.Alert.Subject != user || !strings.Contains(c.Alert.Summary, "8 d ago") {
		t.Fatalf("closure %+v", c.Alert)
	}
	// the queue ages into a page; the jobs are retried and the closure carried out
	e.clk.Advance(48 * time.Hour)
	exec(t, e, `UPDATE river.river_job SET state = 'available', finalized_at = NULL, attempt = 0`)
	exec(t, e, `UPDATE identity.closure_requests SET closed_at = $1, closed_by = $2 WHERE user_id = $2`, at.Add(48*time.Hour), user)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox.take(); !same(got, "escalated "+alerts.KindKYCQueueStale, "resolved "+alerts.KindJobsFailing, "resolved "+alerts.KindJobsFailing,
		"resolved "+alerts.KindDarajaThrottled, "resolved "+alerts.KindClosureWaiting) {
		t.Fatalf("two days on %v", got)
	}
}

func quoted(parts []string) []string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = `"` + strings.ReplaceAll(p, `"`, `\"`) + `"`
	}
	return out
}

// A receipt a player gave for an uncredited top-up (ADR 0024) reaches a person when M-Pesa does not confirm it, or does
// not answer about it in time; a claim being asked about is left alone.
func TestTopUpsCheck_ReceiptClaimsMpesaDidNotConfirmReachAPerson(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	svc := e.service(alerts.TopUpsCheck(e.pool, e.clk, 15*time.Minute, alerts.DefaultThresholds()))
	user := seed(t, e, "254705000002")
	at := now(t, e)
	var checking, unverified string
	// started long ago so their unconfirmed alerts are out of the way; the claims are fresh
	for _, row := range []struct {
		dst         *string
		key, status string
		note        any
	}{{&checking, "c1", "checking", nil}, {&unverified, "c2", "unverified", "M-Pesa did not confirm the receipt: does not exist"}} {
		if err := e.pool.QueryRow(ctx, `INSERT INTO payments.payments (user_id, channel, status, amount_requested_kes, msisdn, idempotency_key, failure_reason,
			claimed_receipt, claimed_at, claim_status, claim_note, created_at, updated_at)
			VALUES ($1, 'stk', 'unconfirmed', 100, '254705000002', $2, 'paid_without_callback', $3, $4, $5, $6, $4, $4) RETURNING id::text`,
			user, row.key, "QJ7XK2M9P"+row.key[1:], at.Add(-5*time.Minute), row.status, row.note).Scan(row.dst); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got := kindsOf(e.inbox.got)
	e.inbox.take()
	c := got["raised "+alerts.KindTopUpReceiptClaim]
	if c.Alert.Subject != unverified || c.Alert.Severity != alerts.Warn || !strings.Contains(c.Alert.Summary, "did not confirm") {
		t.Fatalf("unverified claim %+v (all %v)", c.Alert, got)
	}
	e.clk.Advance(11 * time.Minute) // the checking one has waited 16 min for M-Pesa
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	got = kindsOf(e.inbox.got)
	e.inbox.take()
	if w := got["raised "+alerts.KindTopUpReceiptClaim]; w.Alert.Subject != checking || !strings.Contains(w.Alert.Summary, "has not answered") {
		t.Fatalf("waiting claim %v", got)
	}
	// both get settled: credited, and rejected by an operator
	exec(t, e, `UPDATE payments.payments SET status = 'credited', amount_confirmed_kes = 100, mpesa_receipt = claimed_receipt, claim_status = 'confirmed' WHERE id = $1`, checking)
	exec(t, e, `UPDATE payments.payments SET status = 'failed', failure_reason = 'not_on_statement', claim_status = 'rejected' WHERE id = $1`, unverified)
	if err := svc.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(e.inbox.got); n < 2 {
		t.Fatalf("both claim alerts resolve: %v", kindsOf(e.inbox.got))
	}
}
