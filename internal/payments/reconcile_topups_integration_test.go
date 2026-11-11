//go:build integration

package payments_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/payments"
	"github.com/itsivali/lottery-platform/internal/payments/darajamock"
)

// Owner, 2026-10-31: a top-up M-Pesa completed but the platform never credited is reconciled from the receipt:
// a late callback, the player's own message (confirmed by M-Pesa through Transaction Status), the statement, or an
// operator. These tests drive each path.

// paidWithoutCallback is a prompt the customer approved whose callback Safaricom never delivered: after expiry the
// status query says paid, there is no receipt, and the top-up settles unconfirmed/paid_without_callback.
func paidWithoutCallback(t *testing.T, e *env, user string, amount int64) (payments.TopUp, string) {
	t.Helper()
	e.mock.SetNextFault(darajamock.Fault{DropCallback: true})
	tu := e.topUp(t, user, amount)
	e.pushAndComplete(t)
	e.clk.Advance(payments.DefaultExpiry + time.Second)
	if _, err := e.svc.Verify(context.Background(), tu.ID); err != nil {
		t.Fatal(err)
	}
	if st, reason := e.status(t, tu.ID); st != "unconfirmed" || reason == nil || *reason != "paid_without_callback" {
		t.Fatalf("status = %s/%v, want unconfirmed/paid_without_callback", st, reason)
	}
	pushes := e.mock.Pushes()
	return tu, pushes[len(pushes)-1].CheckoutRequestID
}

func stkCallback(checkout string, amount int64, receipt, msisdn string) []byte {
	b, _ := json.Marshal(map[string]any{"Body": map[string]any{"stkCallback": map[string]any{
		"MerchantRequestID": "m", "CheckoutRequestID": checkout, "ResultCode": 0, "ResultDesc": "The service request is processed successfully.",
		"CallbackMetadata": map[string]any{"Item": []map[string]any{
			{"Name": "Amount", "Value": amount}, {"Name": "MpesaReceiptNumber", "Value": receipt}, {"Name": "PhoneNumber", "Value": msisdn},
		}}}}})
	return b
}

func lateAudits(t *testing.T, e *env, id, action string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit.audit_log WHERE action = $1 AND entity_id = $2`, action, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// ADR 0028: a late callback is a claim, not proof. M-Pesa's Transaction Status confirms its receipt first.
func TestReconcile_APaidCallbackArrivingAfterTheTopUpSettledIsConfirmedByMpesaThenCredited(t *testing.T) {
	e := setup(t)
	user, msisdn := e.player(t)
	tu, checkout := paidWithoutCallback(t, e, user, 100)
	receipt := e.mock.ReceiptOf(checkout)
	if receipt == "" {
		t.Fatal("the mock issued no receipt")
	}

	// Safaricom finally delivers the callback (it retries for a while): nothing is credited on its word
	if err := e.svc.HandleCallback(context.Background(), stkCallback(checkout, 100, receipt, msisdn)); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.status(t, tu.ID); st != "unconfirmed" || e.balance(t, user) != 0 {
		t.Fatalf("status %s balance %d: credited before M-Pesa confirmed the receipt", st, e.balance(t, user))
	}
	if got := e.get(t, user, tu.ID); got.Claim == nil || got.Claim.Receipt != receipt || got.Claim.Status != payments.ClaimClaimed {
		t.Fatalf("the callback's receipt should be the claim being checked: %+v", got.Claim)
	}
	// a second delivery while it is checked changes nothing
	if err := e.svc.HandleCallback(context.Background(), stkCallback(checkout, 100, receipt, msisdn)); err != nil {
		t.Fatal(err)
	}
	// the claim job asks M-Pesa, which confirms the receipt: credited and the phone proved like any other
	e.run(t)
	if st, _ := e.status(t, tu.ID); st != "credited" {
		t.Fatalf("status = %s, want credited", st)
	}
	if e.balance(t, user) != 100 || !e.phoneVerified(t, user) || e.receipt(t, tu.ID) != receipt {
		t.Fatalf("balance %d, phone proved %v receipt %q", e.balance(t, user), e.phoneVerified(t, user), e.receipt(t, tu.ID))
	}
	if reqs := e.mock.StatusRequests(); len(reqs) != 1 || reqs[0] != receipt {
		t.Fatalf("transaction status requests = %v, want one for %s", reqs, receipt)
	}
	if late, credited := lateAudits(t, e, tu.ID, "payment.topup.late_callback"), lateAudits(t, e, tu.ID, "payment.topup.credited"); late != 1 || credited != 1 {
		t.Fatalf("audit: late_callback %d credited %d, want 1 and 1", late, credited)
	}
	// and once credited, another delivery changes nothing
	if err := e.svc.HandleCallback(context.Background(), stkCallback(checkout, 100, receipt, msisdn)); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if e.balance(t, user) != 100 {
		t.Fatalf("balance %d after a duplicate late callback", e.balance(t, user))
	}
}

// SEC-01: anyone holding the hook secret could credit an expired top-up with a made-up receipt. Now M-Pesa is asked.
func TestReconcile_AForgedLateCallbackIsNotCredited(t *testing.T) {
	e := setup(t)
	user, msisdn := e.player(t)
	tu, checkout := paidWithoutCallback(t, e, user, 100)

	if err := e.svc.HandleCallback(context.Background(), stkCallback(checkout, 100, "FAKE123456", msisdn)); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if st, _ := e.status(t, tu.ID); st != "unconfirmed" || e.balance(t, user) != 0 {
		t.Fatalf("status %s balance %d: a receipt M-Pesa does not know was credited", st, e.balance(t, user))
	}
	if got := e.get(t, user, tu.ID); got.Claim == nil || got.Claim.Status != payments.ClaimUnverified {
		t.Fatalf("claim = %+v, want unverified for an operator", got.Claim)
	}
	// Safaricom (or the forger) repeating it does not ask M-Pesa again
	before := len(e.mock.StatusRequests())
	if err := e.svc.HandleCallback(context.Background(), stkCallback(checkout, 100, "FAKE123456", msisdn)); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if after := len(e.mock.StatusRequests()); after != before {
		t.Fatalf("status requests %d -> %d for a receipt M-Pesa already answered about", before, after)
	}

	// a late callback naming another amount is not even asked about
	tu2, checkout2 := paidWithoutCallback(t, e, user, 50)
	if err := e.svc.HandleCallback(context.Background(), stkCallback(checkout2, 5000, e.mock.ReceiptOf(checkout2), msisdn)); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if got := e.get(t, user, tu2.ID); got.Claim != nil || e.balance(t, user) != 0 || lateAudits(t, e, tu2.ID, "payment.topup.callback_ignored") != 1 {
		t.Fatalf("claim %+v balance %d", got.Claim, e.balance(t, user))
	}
}

func TestReconcile_InTheSandboxALateCallbackStillSettlesFromSafaricomsReceipt(t *testing.T) {
	e := sandbox(t)
	user, msisdn := e.player(t)
	e.mock.SetNextFault(darajamock.Fault{DropCallback: true})
	tu := e.topUp(t, user, 100)
	e.pushAndComplete(t)
	e.clk.Advance(payments.DefaultExpiry + time.Second)
	if _, err := e.svc.Verify(context.Background(), tu.ID); err != nil {
		t.Fatal(err)
	}
	checkout := e.mock.Pushes()[0].CheckoutRequestID
	if err := e.svc.HandleCallback(context.Background(), stkCallback(checkout, 100, e.mock.ReceiptOf(checkout), msisdn)); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.status(t, tu.ID); st != "credited" || e.balance(t, user) != 100 {
		t.Fatalf("status %s balance %d", st, e.balance(t, user))
	}
	// but not one that names another amount
	e.mock.SetNextFault(darajamock.Fault{DropCallback: true})
	tu2 := e.topUp(t, user, 30)
	e.pushAndComplete(t)
	e.clk.Advance(payments.DefaultExpiry + time.Second)
	if _, err := e.svc.Verify(context.Background(), tu2.ID); err != nil {
		t.Fatal(err)
	}
	p := e.mock.Pushes()
	if err := e.svc.HandleCallback(context.Background(), stkCallback(p[len(p)-1].CheckoutRequestID, 3000, "SANDBOX001", msisdn)); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.status(t, tu2.ID); st == "credited" || e.balance(t, user) != 100 {
		t.Fatalf("status %s balance %d", st, e.balance(t, user))
	}
}

func TestReconcile_ThePlayersReceiptIsConfirmedByMpesaAndCredited(t *testing.T) {
	e := setup(t)
	user, _ := e.player(t)
	tu, checkout := paidWithoutCallback(t, e, user, 100)
	receipt := e.mock.ReceiptOf(checkout)

	// the player types the receipt from their M-Pesa message (lower case, spaces: normalised)
	c, err := e.svc.ClaimReceipt(context.Background(), user, tu.ID, " "+strings.ToLower(receipt)+" ")
	if err != nil || c.Status != payments.ClaimClaimed || c.Receipt != receipt {
		t.Fatalf("claim: %+v %v", c, err)
	}
	if got := e.get(t, user, tu.ID); got.Claim == nil || got.Claim.Status != payments.ClaimClaimed {
		t.Fatalf("player sees %+v", got.Claim)
	}
	// the claim job asks M-Pesa; the answer comes back through the status hook and credits the top-up
	e.run(t)
	if st, _ := e.status(t, tu.ID); st != "credited" {
		t.Fatalf("status = %s, want credited after M-Pesa confirmed the receipt", st)
	}
	if e.balance(t, user) != 100 || !e.phoneVerified(t, user) {
		t.Fatalf("balance %d proved %v", e.balance(t, user), e.phoneVerified(t, user))
	}
	got := e.get(t, user, tu.ID)
	if got.Claim == nil || got.Claim.Status != payments.ClaimConfirmed {
		t.Fatalf("claim after confirmation: %+v", got.Claim)
	}
	if r := e.receipt(t, tu.ID); r != receipt {
		t.Fatalf("credited with receipt %q, want the claimed %q", r, receipt)
	}
	if reqs := e.mock.StatusRequests(); len(reqs) != 1 || reqs[len(reqs)-1] != receipt {
		t.Fatalf("transaction status requests = %v", reqs)
	}
	// the receipt cannot credit a second top-up
	tu2, _ := paidWithoutCallback(t, e, user, 50)
	if _, err := e.svc.ClaimReceipt(context.Background(), user, tu2.ID, receipt); !errors.Is(err, payments.ErrReceiptUsed) {
		t.Fatalf("reused receipt: %v", err)
	}
}

func TestReconcile_AReceiptMpesaDoesNotKnowWaitsForAnOperator(t *testing.T) {
	e := setup(t)
	user, _ := e.player(t)
	tu, _ := paidWithoutCallback(t, e, user, 100)
	if _, err := e.svc.ClaimReceipt(context.Background(), user, tu.ID, "QQ11223344"); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if st, _ := e.status(t, tu.ID); st != "unconfirmed" {
		t.Fatalf("status = %s, want still unconfirmed", st)
	}
	got := e.get(t, user, tu.ID)
	if got.Claim == nil || got.Claim.Status != payments.ClaimUnverified || !strings.Contains(got.Claim.Note, "did not confirm") {
		t.Fatalf("claim: %+v", got.Claim)
	}
	if _, err := e.svc.ClaimReceipt(context.Background(), user, tu.ID, "QQ11223344"); err != nil { // the player may try again after a refusal
		t.Fatalf("second claim after an unverified one: %v", err)
	}
	if claims, _ := e.svc.ReceiptClaimsOpen(context.Background()); len(claims) != 1 || claims[0] != tu.ID {
		t.Fatalf("open claims = %v", claims)
	}
	// the reconciliation lists it for an operator
	if rep := e.reconcile(t); len(rep.Attention["receipt_claims"]) != 1 {
		t.Fatalf("attention = %v", rep.Attention)
	}
	// who confirms it from the statement with the real receipt: the claim is rejected with the reason
	if err := e.svc.Resolve(context.Background(), payments.Resolution{PaymentID: tu.ID, Receipt: "RR11223344", AmountKES: 100, Source: "statement"}, "staff:1"); err != nil {
		t.Fatal(err)
	}
	got = e.get(t, user, tu.ID)
	if got.Status != payments.StatusCredited || got.Claim == nil || got.Claim.Status != payments.ClaimRejected || !strings.Contains(got.Claim.Note, "RR11223344") {
		t.Fatalf("after resolve: %+v %+v", got, got.Claim)
	}
}

func TestReconcile_AClaimIsRefusedWhileSettlingOrOnceCredited(t *testing.T) {
	e := setup(t)
	user, _ := e.player(t)
	tu := e.topUp(t, user, 100)
	e.run(t) // pushed, waiting for the customer
	if _, err := e.svc.ClaimReceipt(context.Background(), user, tu.ID, "AB12345678"); !errors.Is(err, payments.ErrStillSettling) {
		t.Fatalf("pending: %v", err)
	}
	if _, err := e.svc.ClaimReceipt(context.Background(), user, tu.ID, "short"); !errors.Is(err, payments.ErrInvalidReceipt) {
		t.Fatalf("malformed: %v", err)
	}
	p := e.mock.Pushes()
	if err := e.mock.Complete(context.Background(), p[len(p)-1].CheckoutRequestID); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if _, err := e.svc.ClaimReceipt(context.Background(), user, tu.ID, "AB12345678"); !errors.Is(err, payments.ErrNotClaimable) {
		t.Fatalf("credited: %v", err)
	}
	other, _ := e.player(t)
	if _, err := e.svc.ClaimReceipt(context.Background(), other, tu.ID, "AB12345678"); !errors.Is(err, payments.ErrNotFound) {
		t.Fatalf("another player's top-up: %v", err)
	}
}

func TestReconcile_RecheckVerifiesAnOrphanAndRecordsWhatMpesaSays(t *testing.T) {
	e := setup(t)
	user, msisdn := e.player(t)

	// 1. paid, callback lost: M-Pesa says paid but there is no receipt
	tu, checkout := paidWithoutCallback(t, e, user, 100)
	rc, err := e.svc.Recheck(context.Background(), tu.ID, "staff:1")
	if err != nil || rc.Outcome != "paid_without_receipt" || rc.Status != "unconfirmed" {
		t.Fatalf("recheck 1: %+v %v", rc, err)
	}
	// 2. the callback then arrives as an orphan (the payment was final, so nothing verified it): a recheck credits it
	receipt := e.mock.ReceiptOf(checkout)
	if err := e.pool.QueryRow(context.Background(), `INSERT INTO payments.stk_orphan_callbacks (checkout_request_id, raw_payload) VALUES ($1, $2) RETURNING id`,
		checkout, stkCallback(checkout, 100, receipt, msisdn)).Scan(new(string)); err != nil {
		t.Fatal(err)
	}
	rc, err = e.svc.Recheck(context.Background(), tu.ID, "staff:1")
	if err != nil || rc.Outcome != "verifying" || rc.Status != "unconfirmed" || e.balance(t, user) != 0 {
		t.Fatalf("recheck 2: %+v %v balance %d (M-Pesa confirms the orphan's receipt first)", rc, err, e.balance(t, user))
	}
	e.run(t)
	if st, _ := e.status(t, tu.ID); st != "credited" || e.balance(t, user) != 100 {
		t.Fatalf("after M-Pesa confirmed the orphan: status %s balance %d", st, e.balance(t, user))
	}
	if _, err := e.svc.Recheck(context.Background(), tu.ID, "staff:1"); !errors.Is(err, payments.ErrSettledAlready) {
		t.Fatalf("recheck of a credited top-up: %v", err)
	}

	// 3. a prompt nobody answered: expiry leaves it unconfirmed; a recheck finds M-Pesa's failure and records it
	tu3 := e.topUp(t, user, 70)
	e.run(t)
	e.clk.Advance(payments.DefaultExpiry + time.Second)
	if _, err := e.svc.Verify(context.Background(), tu3.ID); err != nil {
		t.Fatal(err)
	}
	if st, reason := e.status(t, tu3.ID); st != "unconfirmed" || reason == nil || *reason != "still_pending_at_expiry" {
		t.Fatalf("status = %s/%v", st, reason)
	}
	rc, err = e.svc.Recheck(context.Background(), tu3.ID, "staff:1") // the mock's query answers Pending until completed
	if err != nil || rc.Outcome != "pending" {
		t.Fatalf("recheck 3 (still pending at M-Pesa): %+v %v", rc, err)
	}
	// the customer finally cancels it on the phone: M-Pesa now says 1032
	e.mock.SetNextFault(darajamock.Fault{ResultCode: 1032, DropCallback: true})
	tu4 := e.topUp(t, user, 60)
	e.run(t)
	p := e.mock.Pushes()
	if err := e.mock.Complete(context.Background(), p[len(p)-1].CheckoutRequestID); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(payments.DefaultExpiry + time.Second)
	rc, err = e.svc.Recheck(context.Background(), tu4.ID, "staff:1")
	if err != nil || rc.Outcome != "cancelled" || rc.Status != "cancelled" || rc.ResultCode == nil || *rc.ResultCode != 1032 {
		t.Fatalf("recheck 4: %+v %v", rc, err)
	}
}

func TestReconcile_DismissClosesWhatIsNotOnTheStatement(t *testing.T) {
	e := setup(t)
	user, _ := e.player(t)
	tu, _ := paidWithoutCallback(t, e, user, 100)
	if _, err := e.svc.ClaimReceipt(context.Background(), user, tu.ID, "ZZ99887766"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Dismiss(context.Background(), tu.ID, "  ", "staff:1"); !errors.Is(err, payments.ErrReasonRequired) {
		t.Fatalf("no note: %v", err)
	}
	if err := e.svc.Dismiss(context.Background(), tu.ID, "not on the 31 Oct statement", "staff:1"); err != nil {
		t.Fatal(err)
	}
	st, reason := e.status(t, tu.ID)
	if st != "failed" || reason == nil || *reason != "not_on_statement" {
		t.Fatalf("status = %s/%v", st, reason)
	}
	got := e.get(t, user, tu.ID)
	if got.Status != payments.StatusFailed || got.Claim == nil || got.Claim.Status != payments.ClaimRejected || got.Claim.Note != "not on the 31 Oct statement" {
		t.Fatalf("player sees %+v %+v", got, got.Claim)
	}
	if err := e.svc.Dismiss(context.Background(), tu.ID, "again", "staff:1"); err != nil { // a failed top-up: only the claim is touched
		t.Fatal(err)
	}
	if e.balance(t, user) != 0 {
		t.Fatal("dismissing credits nothing")
	}
	// a failed top-up can still be credited later, when it does turn up on a statement
	if err := e.svc.Resolve(context.Background(), payments.Resolution{PaymentID: tu.ID, Receipt: "ZZ99887766", AmountKES: 100, Source: "statement"}, "staff:2"); err != nil {
		t.Fatal(err)
	}
	if e.balance(t, user) != 100 {
		t.Fatalf("balance %d", e.balance(t, user))
	}
}

const statementHeader = "Receipt No.,Completion Time,Initiation Time,Details,Transaction Status,Paid In,Withdrawn,Balance,Balance Confirmed,Reason Type,Other Party Info,Linked Transaction ID,A/C No.\n"

func statementLine(receipt string, at time.Time, paid int64, msisdn, name string) string {
	masked := msisdn[:6] + "***" + msisdn[len(msisdn)-3:]
	return fmt.Sprintf("%s,%s,%s,Pay Bill from %s - %s,Completed,\"%d.00\",,\"1,000.00\",true,Pay Bill,%s - %s,,4draws\n",
		receipt, at.In(time.FixedZone("EAT", 3*3600)).Format("2006-01-02 15:04:05"), at.In(time.FixedZone("EAT", 3*3600)).Format("2006-01-02 15:04:05"), masked, name, paid, masked, name)
}

func TestStatement_ParsesSafaricomsExportByHeaderNames(t *testing.T) {
	body := "Some banner line the portal adds\n" + statementHeader +
		statementLine("UJU4RRBMDK", time.Date(2026, 10, 30, 18, 34, 51, 0, time.UTC), 1, "254796992457", "WILLIS IVALI") +
		"BAD,not a time,,,,1,,,,,,\n" +
		"UJT6YBR9IQ,30-10-2026 10:00:00,,Withdrawal,Completed,,500.00,,,,,,\n"
	rows, skipped, err := payments.ParseStatement(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(skipped) != 1 || !strings.Contains(skipped[0], "line 4") {
		t.Fatalf("rows %d skipped %v", len(rows), skipped)
	}
	r := rows[0]
	if r.Receipt != "UJU4RRBMDK" || r.PaidInKES != 1 || r.PartyNumber != "254796***457" || r.Account != "4draws" || !r.CompletedAt.Equal(time.Date(2026, 10, 30, 18, 34, 51, 0, time.UTC)) {
		t.Fatalf("row = %+v", r)
	}
	if rows[1].WithdrawnKES != 500 || rows[1].PaidInKES != 0 {
		t.Fatalf("withdrawal row = %+v", rows[1])
	}
	if _, _, err := payments.ParseStatement(strings.NewReader("a,b\n1,2\n")); !errors.Is(err, payments.ErrStatementEmpty) {
		t.Fatalf("no header: %v", err)
	}
}

func TestStatement_MatchesPaidRowsToUncreditedPromptsAndCreditsWhatIsConfirmed(t *testing.T) {
	e := setup(t)
	user, msisdn := e.player(t)
	other, otherMSISDN := e.player(t)

	// a normal credited top-up (on the statement), one paid without callback (to match by amount, number and time),
	// one the player claimed, one of another player's that failed and is not on the statement, and a statement row
	// nobody's prompt explains
	tu1 := e.topUp(t, user, 30)
	e.pushAndComplete(t)
	credited := e.receipt(t, tu1.ID)
	start, _ := e.clk.Now(context.Background(), e.pool)
	tu2, _ := paidWithoutCallback(t, e, user, 100)
	tu3, _ := paidWithoutCallback(t, e, other, 100)
	if _, err := e.svc.ClaimReceipt(context.Background(), other, tu3.ID, "CL11111111"); err != nil {
		t.Fatal(err)
	}
	e.run(t) // M-Pesa does not know that receipt: unverified, left for the operator
	tu4, _ := paidWithoutCallback(t, e, other, 45)
	_ = tu4

	at := start.Add(5 * time.Minute)
	body := statementHeader +
		statementLine(credited, at, 30, msisdn, "JANE") +
		statementLine("ST22222222", at.Add(16*time.Minute), 100, msisdn, "JANE") +
		statementLine("CL11111111", at.Add(32*time.Minute), 100, otherMSISDN, "AMINA") +
		statementLine("XX33333333", at.Add(40*time.Minute), 250, "254700000000", "NOBODY")
	rep, err := e.svc.MatchStatement(context.Background(), []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, m := range rep.Matches {
		kinds[m.Row.Receipt] = m.Kind
		if m.Row.Receipt == "ST22222222" && m.PaymentID != tu2.ID {
			t.Fatalf("ST22222222 matched %s, want %s (%s)", m.PaymentID, tu2.ID, m.Reason)
		}
		if m.Row.Receipt == "CL11111111" && m.PaymentID != tu3.ID {
			t.Fatalf("CL11111111 matched %s, want %s", m.PaymentID, tu3.ID)
		}
	}
	if kinds[credited] != "credited" || kinds["ST22222222"] != "matched" || kinds["CL11111111"] != "claimed" || kinds["XX33333333"] != "unmatched" {
		t.Fatalf("kinds = %v", kinds)
	}
	if len(rep.ToCredit) != 2 || rep.Counts["matched"] != 1 || rep.Counts["claimed"] != 1 || rep.Rows != 4 {
		t.Fatalf("report: to_credit %v counts %v rows %d", rep.ToCredit, rep.Counts, rep.Rows)
	}
	if len(rep.Missing) != 0 {
		t.Fatalf("missing = %v (every credited prompt in the window is on the statement)", rep.Missing)
	}
	if e.balance(t, user) != 30 || e.balance(t, other) != 0 {
		t.Fatal("matching credits nothing")
	}

	// the operator confirms the two; one of them is refused because the receipt was credited meanwhile
	if err := e.svc.Resolve(context.Background(), payments.Resolution{PaymentID: tu4.ID, Receipt: "ST22222222", AmountKES: 45}, "staff:9"); err != nil {
		t.Fatal(err)
	}
	report, _ := json.Marshal(rep)
	applied, err := e.svc.ApplyStatement(context.Background(), payments.StatementApply{Filename: "oct.csv", SHA256: rep.SHA256, Rows: rep.Rows, Credits: rep.ToCredit, Report: report}, "staff:1")
	if err != nil {
		t.Fatal(err)
	}
	if applied.Credited != 1 || applied.CreditedKES != 100 || len(applied.Results) != 2 || applied.ImportID == "" {
		t.Fatalf("applied = %+v", applied)
	}
	for _, r := range applied.Results {
		switch r.PaymentID {
		case tu2.ID:
			if r.Credited || !strings.Contains(r.Error, "already credited") {
				t.Fatalf("tu2 result = %+v (receipt taken by tu4)", r)
			}
		case tu3.ID:
			if !r.Credited {
				t.Fatalf("tu3 result = %+v", r)
			}
		}
	}
	if e.balance(t, other) != 145 {
		t.Fatalf("other's balance %d, want 100 (claimed, now credited) + 45", e.balance(t, other))
	}
	if got := e.get(t, other, tu3.ID); got.Claim == nil || got.Claim.Status != payments.ClaimConfirmed {
		t.Fatalf("claim after the statement credited it: %+v", got.Claim)
	}
	imports, err := e.svc.StatementImports(context.Background(), 10)
	if err != nil || len(imports) != 1 || imports[0].Credited != 1 || imports[0].Filename != "oct.csv" || imports[0].SHA256 != rep.SHA256 {
		t.Fatalf("imports = %+v %v", imports, err)
	}
	var audited int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit.audit_log WHERE action = 'payment.statement.imported'`).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("audit rows %d %v", audited, err)
	}
	// a second import of the same statement finds everything credited and nothing to do
	rep2, err := e.svc.MatchStatement(context.Background(), []byte(body))
	if err != nil || len(rep2.ToCredit) != 0 || rep2.Counts["credited"] != 3 { // tu1, tu3 and tu4 (which took ST22222222)
		t.Fatalf("second match: %v %+v", err, rep2.Counts)
	}
}

func TestReconcile_ALateCallbackLeavesThePlayersOpenClaimToMpesa(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, msisdn := e.player(t)
	tu, checkout := paidWithoutCallback(t, e, user, 100)
	receipt := e.mock.ReceiptOf(checkout)
	if _, err := e.svc.ClaimReceipt(ctx, user, tu.ID, receipt); err != nil {
		t.Fatal(err)
	}
	// the same receipt arrives in a late callback: the claim already open is not doubled
	if err := e.svc.HandleCallback(ctx, stkCallback(checkout, 100, receipt, msisdn)); err != nil {
		t.Fatal(err)
	}
	if n := lateAudits(t, e, tu.ID, "payment.topup.late_callback"); n != 0 {
		t.Fatalf("late_callback audits = %d for the receipt already claimed", n)
	}
	e.run(t)
	if st, _ := e.status(t, tu.ID); st != "credited" || e.balance(t, user) != 100 || len(e.mock.StatusRequests()) != 1 {
		t.Fatalf("status %s balance %d requests %v", st, e.balance(t, user), e.mock.StatusRequests())
	}

	// a different receipt in the callback while the player's claim is open: the claim is checked, the callback recorded
	tu2, checkout2 := paidWithoutCallback(t, e, user, 50)
	if _, err := e.svc.ClaimReceipt(ctx, user, tu2.ID, e.mock.ReceiptOf(checkout2)); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.HandleCallback(ctx, stkCallback(checkout2, 50, "OTHERRCPT1", msisdn)); err != nil {
		t.Fatal(err)
	}
	var note string
	if err := e.pool.QueryRow(ctx, `SELECT details->>'note' FROM audit.audit_log WHERE action = 'payment.topup.late_callback' AND entity_id = $1`, tu2.ID).Scan(&note); err != nil || !strings.Contains(note, "the player claimed receipt") {
		t.Fatalf("audit note %q (%v)", note, err)
	}
	e.run(t)
	if got := e.get(t, user, tu2.ID); got.Status != payments.StatusCredited || got.Claim == nil || got.Claim.Receipt != e.mock.ReceiptOf(checkout2) {
		t.Fatalf("payment %+v", got)
	}
}
