//go:build integration

package payouts_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/payments/daraja"
	"github.com/itsivali/4draws-lottery-platform/internal/payouts"
)

// failStatus makes the fake refuse (err) or accept (nil) every Transaction Status request from now on.
func (f *fakeDaraja) failStatus(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusErr = err
}

// manualPayout sends a withdrawal through the fake and leaves it with an operator: Daraja refuses the status question,
// so nobody can confirm whether the money moved. The fake accepts status questions again afterwards.
func (e *env) manualPayout(t *testing.T, d *fakeDaraja, user string, amount int64, key string) payouts.Payout {
	t.Helper()
	ctx := context.Background()
	d.failStatus(daraja.ErrRejected)
	defer d.failStatus(nil)
	p := e.withdraw(t, user, amount, key)
	e.q.take()
	if err := e.svc.Send(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(payouts.DefaultResultWait + time.Second)
	if _, err := e.svc.Verify(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if st, _, reason := e.state(t, p.ID); st != "manual" || reason != "status_request_rejected" {
		t.Fatalf("payout %s %s, want manual", st, reason)
	}
	return p
}

// msisdn is the player's proved number.
func (e *env) msisdn(t *testing.T, user string) string {
	t.Helper()
	var m string
	if err := e.pool.QueryRow(context.Background(), `SELECT msisdn FROM identity.users WHERE id = $1`, user).Scan(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

// claim records a receipt staff found on the statement for a manual payout; it returns the originator of the
// Transaction Status question asked about it.
func (e *env) claim(t *testing.T, id, operator, receipt string) string {
	t.Helper()
	got, err := e.svc.Settle(context.Background(), id, operator, true, receipt, "")
	if err != nil || got.State != "manual" || got.Check == nil || got.Check.Receipt != receipt || got.Check.Outcome != "" {
		t.Fatalf("claim %s: %+v %v", receipt, got, err)
	}
	var orig string
	if err := e.pool.QueryRow(context.Background(), `SELECT status_originator FROM payouts.receipt_checks WHERE payout_id = $1
		ORDER BY id DESC LIMIT 1`, id).Scan(&orig); err != nil {
		t.Fatal(err)
	}
	return orig
}

// statusAnswer is M-Pesa's Transaction Status answer to the question orig, which carried the payout as its occasion.
func statusAnswer(orig, payout string, code int, status, receipt string, amount int64, credit string) []byte {
	return []byte(fmt.Sprintf(`{"Result":{"ResultType":0,"ResultCode":%d,"ResultDesc":"desc","OriginatorConversationID":%q,"ConversationID":"AG",
		"ResultParameters":{"ResultParameter":[{"Key":"ReceiptNo","Value":%q},{"Key":"Amount","Value":%d},{"Key":"TransactionStatus","Value":%q},
		{"Key":"CreditPartyName","Value":%q}]},"ReferenceData":{"ReferenceItem":{"Key":"Occasion","Value":%q}}}}`,
		code, orig, receipt, amount, status, credit, payout))
}

// latestCheck is the operator's view of the payout's latest receipt check.
func (e *env) latestCheck(t *testing.T, id string) payouts.ReceiptCheck {
	t.Helper()
	list, _, err := e.svc.InState(context.Background(), "manual", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.ID == id {
			if p.Check == nil {
				t.Fatalf("payout %s has no receipt check", id)
			}
			return *p.Check
		}
	}
	t.Fatalf("payout %s is not manual", id)
	return payouts.ReceiptCheck{}
}

func (e *env) audits(t *testing.T, action, id string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit.audit_log WHERE action = $1 AND entity_id = $2`, action, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// ADR 0034: a receipt an operator claims settles a manual payout only when M-Pesa confirms that very receipt, for the
// payout's net amount, paid to the payout's number. An answer naming another receipt, another amount or another
// number refutes the claim; an answer that timed out decides nothing. Each leaves the payout manual, unpaid, with why
// shown to the operator, and audited. A late answer to a check already decided does not reopen it.
func TestReceiptCheck_OnlyTheReceiptMPesaConfirmsForThisAmountAndNumberSettlesIt(t *testing.T) {
	ctx := context.Background()
	d := &fakeDaraja{}
	e := setupFake(t, d)
	u := e.player(t, 1_000) // 950
	msisdn := e.msisdn(t, u)
	p := e.manualPayout(t, d, u, 300, "k")
	answer := func(kind string, body []byte) {
		t.Helper()
		if err := e.hooks.HandleCallback(ctx, kind, body); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Verify(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
	}

	// until M-Pesa answers, the verify job decides nothing
	orig := e.claim(t, p.ID, "staff:alice", "MBCAMOUNT1")
	if _, err := e.svc.Verify(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if c := e.latestCheck(t, p.ID); c.Outcome != "" {
		t.Fatalf("decided before M-Pesa answered: %+v", c)
	}
	answer(payouts.CallbackStatusResult, statusAnswer(orig, p.ID, 0, "Completed", "MBCAMOUNT1", 299, msisdn+" - Jane"))
	if c := e.latestCheck(t, p.ID); c.Outcome != "refuted" || c.Detail != "M-Pesa reported KES 299, the payout is KES 300" {
		t.Fatalf("another amount: %+v", c)
	}

	orig = e.claim(t, p.ID, "staff:alice", "MBCCLAIMED")
	answer(payouts.CallbackStatusResult, statusAnswer(orig, p.ID, 0, "Completed", "MBCDIFFER1", 300, msisdn+" - Jane"))
	if c := e.latestCheck(t, p.ID); c.Outcome != "refuted" || c.Detail != "M-Pesa reported receipt MBCDIFFER1" {
		t.Fatalf("another receipt: %+v", c)
	}

	orig = e.claim(t, p.ID, "staff:alice", "MBCNUMBER1")
	answer(payouts.CallbackStatusResult, statusAnswer(orig, p.ID, 0, "Completed", "MBCNUMBER1", 300, "254799999999 - Someone"))
	if c := e.latestCheck(t, p.ID); c.Outcome != "refuted" || c.Detail != "M-Pesa paid another number" {
		t.Fatalf("another number: %+v", c)
	}

	orig = e.claim(t, p.ID, "staff:alice", "MBCTIMEOUT")
	answer(payouts.CallbackStatusTimeout, statusAnswer(orig, p.ID, 1, "", "", 0, ""))
	if c := e.latestCheck(t, p.ID); c.Outcome != "no_answer" || c.Detail != "M-Pesa's answer timed out" {
		t.Fatalf("timed out: %+v", c)
	}
	// M-Pesa's answer arrives after the check was decided as unanswered: the decided check stays as it was
	answer(payouts.CallbackStatusResult, statusAnswer(orig, p.ID, 0, "Completed", "MBCTIMEOUT", 300, msisdn+" - Jane"))
	if c := e.latestCheck(t, p.ID); c.Outcome != "no_answer" {
		t.Fatalf("a decided check was reopened: %+v", c)
	}

	if st, receipt, _ := e.state(t, p.ID); st != "manual" || receipt != "" || e.balance(t, u) != 650 {
		t.Fatalf("after unconfirmed claims: %s %q balance %d", st, receipt, e.balance(t, u))
	}
	if r, n := e.audits(t, "payout.receipt_refuted", p.ID), e.audits(t, "payout.receipt_no_answer", p.ID); r != 3 || n != 1 {
		t.Fatalf("audited %d refuted, %d unanswered", r, n)
	}

	// asked again, M-Pesa confirms it: paid under the receipt, by the person who claimed it
	orig = e.claim(t, p.ID, "staff:bob", "MBCTIMEOUT")
	answer(payouts.CallbackStatusResult, statusAnswer(orig, p.ID, 0, "Completed", "MBCTIMEOUT", 300, msisdn+" - Jane"))
	if st, receipt, _ := e.state(t, p.ID); st != "succeeded" || receipt != "MBCTIMEOUT" || e.balance(t, u) != 650 {
		t.Fatalf("confirmed: %s %q balance %d", st, receipt, e.balance(t, u))
	}
	var by string
	if err := e.pool.QueryRow(ctx, `SELECT settled_by FROM payouts.payouts WHERE id = $1`, p.ID).Scan(&by); err != nil || by != "staff:bob" {
		t.Fatalf("settled by %q %v", by, err)
	}

	// a receipt that already settled one payout cannot be claimed for another
	other := e.manualPayout(t, d, u, 200, "k2")
	if _, err := e.svc.Settle(ctx, other.ID, "staff:alice", true, "MBCTIMEOUT", ""); !errors.Is(err, payouts.ErrReceiptUsed) {
		t.Fatalf("a used receipt claimed again: %v", err)
	}
	var checks int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM payouts.receipt_checks WHERE payout_id = $1`, other.ID).Scan(&checks); err != nil || checks != 0 {
		t.Fatalf("checks recorded for the refused claim: %d %v", checks, err)
	}
	if st, _, _ := e.state(t, other.ID); st != "manual" || e.balance(t, u) != 450 {
		t.Fatalf("other payout %s balance %d", st, e.balance(t, u))
	}
}

// ADR 0034: a question M-Pesa did not take is recorded as unanswered at once, so the operator may claim again; a
// question M-Pesa took but never answered blocks other claims for ReceiptCheckWait only, then is recorded as
// unanswered when the next claim is made. Without B2C no receipt can be checked.
func TestReceiptCheck_AnUnansweredQuestionDoesNotBlockTheOperatorForever(t *testing.T) {
	ctx := context.Background()
	d := &fakeDaraja{}
	e := setupFake(t, d)
	u := e.player(t, 1_000)
	p := e.manualPayout(t, d, u, 300, "k")

	d.failStatus(daraja.ErrUnavailable)
	if _, err := e.svc.Settle(ctx, p.ID, "staff:alice", true, "MBCASKED01", ""); !errors.Is(err, payouts.ErrNoAnswer) {
		t.Fatalf("M-Pesa did not take the question: %v", err)
	}
	if c := e.latestCheck(t, p.ID); c.Receipt != "MBCASKED01" || c.Outcome != "no_answer" || c.Detail != "M-Pesa did not take the question" {
		t.Fatalf("check %+v", c)
	}
	d.failStatus(nil)

	// asked again at once: M-Pesa takes it this time, but never answers
	e.claim(t, p.ID, "staff:alice", "MBCASKED01")
	if _, err := e.svc.Settle(ctx, p.ID, "staff:bob", true, "MBCASKED02", ""); !errors.Is(err, payouts.ErrCheckPending) {
		t.Fatalf("a second claim while M-Pesa may answer: %v", err)
	}
	e.clk.Advance(payouts.ReceiptCheckWait + time.Second)
	e.claim(t, p.ID, "staff:bob", "MBCASKED02")
	var outcomes []string
	rows, err := e.pool.Query(ctx, `SELECT coalesce(outcome, '') || '/' || coalesce(detail, '') FROM payouts.receipt_checks WHERE payout_id = $1 ORDER BY id`, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		outcomes = append(outcomes, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(outcomes) != "[no_answer/M-Pesa did not take the question no_answer/M-Pesa did not answer in time /]" {
		t.Fatalf("checks %q", outcomes)
	}
	if st, _, _ := e.state(t, p.ID); st != "manual" || e.balance(t, u) != 650 {
		t.Fatalf("payout %s balance %d", st, e.balance(t, u))
	}

	// M-Pesa did not take the question and the worker failed before recording that: the claim is left as if M-Pesa
	// might still answer, so it blocks other claims for ReceiptCheckWait, and no longer
	e.clk.Advance(payouts.ReceiptCheckWait + time.Second)
	flaky := &flakyClock{inner: e.clk}
	flaky.left.Store(1)
	broken := e.service(flaky, &fakeDaraja{statusErr: daraja.ErrUnavailable})
	if _, err := broken.Settle(ctx, p.ID, "staff:alice", true, "MBCASKED03", ""); err == nil || errors.Is(err, payouts.ErrNoAnswer) {
		t.Fatalf("claim with a failing worker: %v", err)
	}
	if c := e.latestCheck(t, p.ID); c.Receipt != "MBCASKED03" || c.Outcome != "" {
		t.Fatalf("interrupted check %+v", c)
	}
	if _, err := e.svc.Settle(ctx, p.ID, "staff:bob", true, "MBCASKED04", ""); !errors.Is(err, payouts.ErrCheckPending) {
		t.Fatalf("a claim right after the interrupted one: %v", err)
	}
	e.clk.Advance(payouts.ReceiptCheckWait + time.Second)
	e.claim(t, p.ID, "staff:bob", "MBCASKED04")
	if st, _, _ := e.state(t, p.ID); st != "manual" || e.balance(t, u) != 650 {
		t.Fatalf("payout %s balance %d", st, e.balance(t, u))
	}

	off := setupWith(t, false, payouts.Config{})
	if _, err := off.svc.Settle(ctx, "0192f000-0000-7000-8000-000000000000", "staff:alice", true, "MBCASKED03", ""); !errors.Is(err, payouts.ErrUnavailable) {
		t.Fatalf("claim without B2C: %v", err)
	}
}
