//go:build integration

package payments_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

// paybillReceipt is a payment the customer made on the paybill whose confirmation never reached the platform: M-Pesa
// knows the receipt, the platform does not. f shapes what Transaction Status later says about it.
func paybillReceipt(t *testing.T, e *env, amount int64, msisdn string, f darajamock.C2BFault) string {
	t.Helper()
	f.DropConfirmation = true
	e.mock.SetNextC2BFault(f)
	receipt, err := e.mock.Pay(context.Background(), amount, msisdn, msisdn)
	if err != nil {
		t.Fatal(err)
	}
	if jobs := e.q.take(); len(jobs) != 0 {
		t.Fatalf("jobs = %+v for a confirmation that never came", jobs)
	}
	return receipt
}

// ADR 0028: a claimed receipt credits a top-up only when M-Pesa confirms a completed payment of the amount asked for,
// from the player's number. A receipt M-Pesa knows but describes otherwise is left for an operator with the reason, and
// nothing is credited.
func TestReconcile_AReceiptMpesaDescribesOtherwiseIsNotCredited(t *testing.T) {
	cases := []struct {
		name  string
		fault darajamock.C2BFault
		note  string
	}{
		{"reversed", darajamock.C2BFault{StatusState: "Reversed"}, "M-Pesa shows the transaction as Reversed"},
		{"another amount", darajamock.C2BFault{StatusAmountKES: 10}, "M-Pesa says KES 10 was paid; the top-up asked for KES 100"},
		{"another payer", darajamock.C2BFault{StatusPayer: "254711000111"}, "the receipt was paid from another number"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setup(t)
			ctx := context.Background()
			user, msisdn := e.player(t)
			tu, _ := paidWithoutCallback(t, e, user, 100)
			receipt := paybillReceipt(t, e, 100, msisdn, tc.fault)

			if _, err := e.svc.ClaimReceipt(ctx, user, tu.ID, receipt); err != nil {
				t.Fatal(err)
			}
			e.run(t)
			var credited *string
			if err := e.pool.QueryRow(ctx, `SELECT mpesa_receipt FROM payments.payments WHERE id = $1`, tu.ID).Scan(&credited); err != nil {
				t.Fatal(err)
			}
			if st, _ := e.status(t, tu.ID); st != "unconfirmed" || e.balance(t, user) != 0 || credited != nil {
				t.Fatalf("status %s balance %d receipt %v: credited on an answer M-Pesa did not confirm", st, e.balance(t, user), credited)
			}
			got := e.get(t, user, tu.ID)
			if got.Claim == nil || got.Claim.Status != payments.ClaimUnverified || got.Claim.Receipt != receipt || !strings.Contains(got.Claim.Note, tc.note) {
				t.Fatalf("claim = %+v, want unverified with %q", got.Claim, tc.note)
			}
			var note string
			if err := e.pool.QueryRow(ctx, `SELECT details->>'note' FROM audit.audit_log WHERE action = 'payment.topup.receipt_unverified' AND entity_id = $1`,
				tu.ID).Scan(&note); err != nil || note != got.Claim.Note {
				t.Fatalf("audit note %q (%v), want the claim's note", note, err)
			}
			if reqs := e.mock.StatusRequests(); len(reqs) != 1 || reqs[0] != receipt {
				t.Fatalf("transaction status requests = %v", reqs)
			}
			// the operator still sees it, and the claim job running again does not ask M-Pesa about it a second time
			if claims, _ := e.svc.ReceiptClaimsOpen(ctx); len(claims) != 1 || claims[0] != tu.ID {
				t.Fatalf("open claims = %v", claims)
			}
			if d, err := e.svc.CheckClaim(ctx, tu.ID); err != nil || d != 0 || len(e.mock.StatusRequests()) != 1 {
				t.Fatalf("check of an unverified claim: %v %v, requests %v", d, err, e.mock.StatusRequests())
			}
		})
	}
}

// Transaction Status refused outright (HTTP 4xx): asking again will not change M-Pesa's mind, so the claim goes to an
// operator at once with M-Pesa's reason, and nothing is credited.
func TestReconcile_AClaimMpesaRefusesToLookUpWaitsForAnOperator(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, msisdn := e.player(t)
	tu, _ := paidWithoutCallback(t, e, user, 100)
	receipt := paybillReceipt(t, e, 100, msisdn, darajamock.C2BFault{StatusHTTP: http.StatusBadRequest})
	if _, err := e.svc.ClaimReceipt(ctx, user, tu.ID, receipt); err != nil {
		t.Fatal(err)
	}
	e.retries = nil
	e.run(t)
	got := e.get(t, user, tu.ID)
	if got.Claim == nil || got.Claim.Status != payments.ClaimUnverified || !strings.HasPrefix(got.Claim.Note, "M-Pesa refused to look the receipt up: ") {
		t.Fatalf("claim = %+v", got.Claim)
	}
	if got.Status == payments.StatusCredited || e.balance(t, user) != 0 || len(e.retries) != 0 {
		t.Fatalf("status %s balance %d retries %v", got.Status, e.balance(t, user), e.retries)
	}
	// an operator who finds it on the statement credits it; the claim is confirmed with that receipt
	if err := e.svc.Resolve(ctx, payments.Resolution{PaymentID: tu.ID, Receipt: receipt, AmountKES: 100, Source: "statement"}, "staff:1"); err != nil {
		t.Fatal(err)
	}
	if got := e.get(t, user, tu.ID); got.Status != payments.StatusCredited || got.Claim == nil || got.Claim.Status != payments.ClaimConfirmed || e.balance(t, user) != 100 {
		t.Fatalf("after resolve: %+v %+v balance %d", got, got.Claim, e.balance(t, user))
	}
}

// M-Pesa unavailable (5xx) says nothing about the receipt: the claim stays open and the job asks again later, slower
// than the status query's own pace so as not to prolong an outage.
func TestReconcile_AClaimIsAskedAgainLaterWhenMpesaIsUnavailable(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, msisdn := e.player(t)
	tu, _ := paidWithoutCallback(t, e, user, 100)
	receipt := paybillReceipt(t, e, 100, msisdn, darajamock.C2BFault{StatusHTTP: http.StatusServiceUnavailable})
	if _, err := e.svc.ClaimReceipt(ctx, user, tu.ID, receipt); err != nil {
		t.Fatal(err)
	}
	e.retries = nil
	e.run(t)
	if len(e.retries) != 1 || e.retries[0] != 2*payments.DefaultVerifyAfter {
		t.Fatalf("retries = %v, want one of %v", e.retries, 2*payments.DefaultVerifyAfter)
	}
	got := e.get(t, user, tu.ID)
	if got.Claim == nil || got.Claim.Status != payments.ClaimClaimed || got.Claim.Note != "" {
		t.Fatalf("claim = %+v, want still claimed", got.Claim)
	}
	if got.Status == payments.StatusCredited || e.balance(t, user) != 0 {
		t.Fatalf("status %s balance %d", got.Status, e.balance(t, user))
	}
	// the retry asks again, and finds M-Pesa still down: still open, nothing credited
	if d, err := e.svc.CheckClaim(ctx, tu.ID); err != nil || d != 2*payments.DefaultVerifyAfter {
		t.Fatalf("retry: %v %v", d, err)
	}
	if reqs := e.mock.StatusRequests(); len(reqs) != 2 || e.balance(t, user) != 0 {
		t.Fatalf("requests %v balance %d", reqs, e.balance(t, user))
	}
	if claims, _ := e.svc.ReceiptClaimsOpen(ctx); len(claims) != 1 || claims[0] != tu.ID {
		t.Fatalf("open claims = %v", claims)
	}
}

// A claim-check job for a payment without an open claim asks M-Pesa nothing; one for a payment that does not exist
// fails rather than being taken as done.
func TestReconcile_AClaimCheckWithoutAnOpenClaimAsksNothing(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, _ := e.player(t)
	tu, _ := paidWithoutCallback(t, e, user, 100)
	if d, err := e.svc.CheckClaim(ctx, tu.ID); err != nil || d != 0 {
		t.Fatalf("no claim: %v %v", d, err)
	}
	if _, err := e.svc.CheckClaim(ctx, "01a19000-0000-7000-8000-0000000000ff"); err == nil {
		t.Fatal("a check for an unknown payment succeeded")
	}
	if reqs := e.mock.StatusRequests(); len(reqs) != 0 {
		t.Fatalf("transaction status requests = %v", reqs)
	}
	if got := e.get(t, user, tu.ID); got.Claim != nil || e.balance(t, user) != 0 {
		t.Fatalf("claim %+v balance %d", got.Claim, e.balance(t, user))
	}
}

// Matching never picks for the operator: two prompts that fit a row equally are shown as candidates, not credited;
// rows that are not completed payments in (a failed transaction, a withdrawal) are skipped even when a prompt fits.
func TestStatement_AnAmbiguousOrUnpaidRowIsNotOfferedForCredit(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, msisdn := e.player(t)
	start, _ := e.clk.Now(ctx, e.pool)
	first, _ := paidWithoutCallback(t, e, user, 100)
	second, _ := paidWithoutCallback(t, e, user, 100)
	lone, _ := paidWithoutCallback(t, e, user, 70)

	at := start.Add(20 * time.Minute) // after both 100s were prompted, within the window of each
	failed := strings.Replace(statementLine("FA44444444", at.Add(time.Minute), 70, msisdn, "JANE"), ",Completed,", ",Failed,", 1)
	body := statementHeader +
		statementLine("AM44444444", at, 100, msisdn, "JANE") +
		failed +
		"WD44444444," + at.In(time.FixedZone("EAT", 3*3600)).Format("2006-01-02 15:04:05") + ",,Withdrawal,Completed,,\"70.00\",,,,,,\n"
	rep, err := e.svc.MatchStatement(ctx, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.ToCredit) != 0 || rep.Counts["ambiguous"] != 1 || rep.Counts["skipped"] != 2 {
		t.Fatalf("to_credit %v counts %v", rep.ToCredit, rep.Counts)
	}
	for _, m := range rep.Matches {
		switch m.Row.Receipt {
		case "AM44444444":
			if m.Kind != "ambiguous" || len(m.Candidates) != 2 || m.Candidates[0].PaymentID != second.ID || m.Candidates[1].PaymentID != first.ID {
				t.Fatalf("ambiguous row = %+v (newest prompt first)", m)
			}
		case "FA44444444":
			if m.Kind != "skipped" || m.Reason != "status Failed" {
				t.Fatalf("failed row = %+v", m)
			}
		case "WD44444444":
			if m.Kind != "skipped" || m.Reason != "not a payment in" {
				t.Fatalf("withdrawal row = %+v", m)
			}
		}
	}
	for _, id := range []string{first.ID, second.ID, lone.ID} {
		if st, _ := e.status(t, id); st != "unconfirmed" {
			t.Fatalf("%s is %s: matching credits nothing", id, st)
		}
	}
	if e.balance(t, user) != 0 {
		t.Fatalf("balance %d", e.balance(t, user))
	}
}

// Each confirmed statement credit is tried on its own and a refused one says why, so the operator can act on it; a
// refusal credits nothing and does not stop the others. The import is recorded even when nothing was credited.
func TestStatement_ApplyReportsWhyEachRefusedCreditWasRefused(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, _ := e.player(t)
	open, _ := paidWithoutCallback(t, e, user, 100)
	done := e.topUp(t, user, 30)
	e.pushAndComplete(t)
	if st, _ := e.status(t, done.ID); st != "credited" {
		t.Fatalf("status %s", st)
	}

	credits := []payments.StatementCredit{
		{PaymentID: "01a19000-0000-7000-8000-0000000000ee", Receipt: "NF55555555", AmountKES: 100},
		{PaymentID: "not-a-payment-id", Receipt: "NF55555556", AmountKES: 100},
		{PaymentID: done.ID, Receipt: "DN55555555", AmountKES: 30},
		{PaymentID: open.ID, Receipt: "short", AmountKES: 100},
		{PaymentID: open.ID, Receipt: "AM55555555", AmountKES: 0},
		{PaymentID: open.ID, Receipt: "OK55555555", AmountKES: 100},
	}
	want := []string{"no such top-up", "no such top-up", "the top-up is not settled uncredited", "not an M-Pesa receipt", "invalid amount", ""}
	if _, err := e.svc.ApplyStatement(ctx, payments.StatementApply{SHA256: "not-a-sha", Credits: credits}, "staff:1"); err == nil {
		t.Fatal("an apply without MatchStatement's sha256 was accepted")
	}
	if e.balance(t, user) != 30 {
		t.Fatalf("balance %d after a refused apply", e.balance(t, user))
	}
	sha := strings.Repeat("ab", 32)
	applied, err := e.svc.ApplyStatement(ctx, payments.StatementApply{SHA256: sha, Rows: 6, Credits: credits}, "staff:1")
	if err != nil {
		t.Fatal(err)
	}
	if applied.Credited != 1 || applied.CreditedKES != 100 || len(applied.Results) != len(credits) {
		t.Fatalf("applied = %+v", applied)
	}
	for i, r := range applied.Results {
		if r.Error != want[i] || r.Credited != (want[i] == "") {
			t.Errorf("credit %d (%s %s): %+v, want error %q", i, r.PaymentID, r.Receipt, r, want[i])
		}
	}
	if e.balance(t, user) != 130 || e.receipt(t, open.ID) != "OK55555555" || e.receipt(t, done.ID) == "DN55555555" {
		t.Fatalf("balance %d, receipts %q %q", e.balance(t, user), e.receipt(t, open.ID), e.receipt(t, done.ID))
	}
	imports, err := e.svc.StatementImports(ctx, 10)
	if err != nil || len(imports) != 1 || imports[0].Filename != "statement.csv" || imports[0].Matched != 6 || imports[0].Credited != 1 {
		t.Fatalf("imports = %+v %v", imports, err)
	}
	var report struct {
		Matching json.RawMessage            `json:"matching"`
		Results  []payments.StatementResult `json:"results"`
	}
	if err := json.Unmarshal(imports[0].Report, &report); err != nil || string(report.Matching) != "{}" || len(report.Results) != len(credits) {
		t.Fatalf("kept report = %s (%v)", imports[0].Report, err)
	}
}

// One receipt credits one top-up. A receipt M-Pesa confirms is still refused for the claim when an operator credited
// it to another top-up while M-Pesa was being asked.
func TestReconcile_AConfirmedReceiptAlreadyCreditedElsewhereIsNotCreditedTwice(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, _ := e.player(t)
	tu, checkout := paidWithoutCallback(t, e, user, 100)
	other, _ := paidWithoutCallback(t, e, user, 100)
	receipt := e.mock.ReceiptOf(checkout)
	if _, err := e.svc.ClaimReceipt(ctx, user, tu.ID, receipt); err != nil {
		t.Fatal(err)
	}
	// before the claim job runs, an operator credits the other top-up with the same receipt from the statement
	if err := e.svc.Resolve(ctx, payments.Resolution{PaymentID: other.ID, Receipt: receipt, AmountKES: 100, Source: "statement"}, "staff:1"); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	got := e.get(t, user, tu.ID)
	if got.Status == payments.StatusCredited || got.Claim == nil || got.Claim.Status != payments.ClaimUnverified || got.Claim.Note != "the receipt already credited another top-up" {
		t.Fatalf("payment %+v claim %+v", got, got.Claim)
	}
	if e.balance(t, user) != 100 || lateAudits(t, e, tu.ID, "payment.topup.receipt_unverified") != 1 {
		t.Fatalf("balance %d: one receipt, one credit", e.balance(t, user))
	}
}

// An operator's recheck of a top-up whose claimed receipt is open hands it to the claim job (Transaction Status)
// and says so; it does not report "paid, receipt needed" when the receipt is already being checked.
func TestReconcile_RecheckOfAnOpenClaimAsksMpesaAboutTheReceipt(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, msisdn := e.player(t)

	// paid at M-Pesa, callback lost, the player's receipt not checked yet
	tu, checkout := paidWithoutCallback(t, e, user, 100)
	if _, err := e.svc.ClaimReceipt(ctx, user, tu.ID, e.mock.ReceiptOf(checkout)); err != nil {
		t.Fatal(err)
	}
	e.q.take() // the claim job is lost (a worker restart): the recheck enqueues it again
	rc, err := e.svc.Recheck(ctx, tu.ID, "staff:1")
	if err != nil || rc.Outcome != "asking" || rc.Status != "unconfirmed" || e.balance(t, user) != 0 {
		t.Fatalf("recheck: %+v %v balance %d", rc, err, e.balance(t, user))
	}
	e.run(t)
	if st, _ := e.status(t, tu.ID); st != "credited" || e.balance(t, user) != 100 {
		t.Fatalf("after the claim job: status %s balance %d", st, e.balance(t, user))
	}

	// a prompt still pending at M-Pesa when it expired, with a paybill receipt claimed for it
	tu2 := e.topUp(t, user, 60)
	e.run(t)
	e.clk.Advance(payments.DefaultExpiry + time.Second)
	if _, err := e.svc.Verify(ctx, tu2.ID); err != nil {
		t.Fatal(err)
	}
	receipt := paybillReceipt(t, e, 60, msisdn, darajamock.C2BFault{})
	if _, err := e.svc.ClaimReceipt(ctx, user, tu2.ID, receipt); err != nil {
		t.Fatal(err)
	}
	rc, err = e.svc.Recheck(ctx, tu2.ID, "staff:1")
	if err != nil || rc.Outcome != "asking" || rc.Status != "unconfirmed" || lateAudits(t, e, tu2.ID, "payment.topup.rechecked") != 1 {
		t.Fatalf("recheck of a pending prompt with a claim: %+v %v", rc, err)
	}
	e.run(t)
	if got := e.get(t, user, tu2.ID); got.Status != payments.StatusCredited || e.balance(t, user) != 160 || e.receipt(t, tu2.ID) != receipt {
		t.Fatalf("payment %+v balance %d", got, e.balance(t, user))
	}
}

// What an operator's recheck reports when M-Pesa cannot say whether the customer paid: no prompt ever went out, the
// status query is refused, or M-Pesa is down. None of these credits or settles anything.
func TestReconcile_RecheckReportsWhatMpesaCannotSay(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, _ := e.player(t)

	// the push was refused: there is no prompt to ask about
	e.mock.SetNextFault(darajamock.Fault{PushStatus: http.StatusBadRequest})
	noPrompt := e.topUp(t, user, 100)
	e.run(t)
	if st, reason := e.status(t, noPrompt.ID); st != "failed" || reason == nil || *reason != "push_rejected" {
		t.Fatalf("status = %s/%v", st, reason)
	}
	rc, err := e.svc.Recheck(ctx, noPrompt.ID, "staff:1")
	if err != nil || rc.Outcome != "no_prompt" || rc.Status != "failed" || lateAudits(t, e, noPrompt.ID, "payment.topup.rechecked") != 1 {
		t.Fatalf("no prompt: %+v %v", rc, err)
	}

	// M-Pesa refuses the status query: no answer yet, the top-up stays as it was
	e.mock.SetNextFault(darajamock.Fault{QueryStatus: http.StatusBadRequest})
	refused := e.topUp(t, user, 80)
	e.run(t)
	// before expiry a refused query is asked again later, the top-up left pending
	e.clk.Advance(payments.DefaultVerifyAfter + time.Second)
	if d, err := e.svc.Verify(ctx, refused.ID); err != nil || d != payments.DefaultVerifyAfter {
		t.Fatalf("verify before expiry: %v %v", d, err)
	}
	if st, _ := e.status(t, refused.ID); st != "stk_sent" {
		t.Fatalf("status = %s, want still stk_sent", st)
	}
	e.clk.Advance(payments.DefaultExpiry)
	if _, err := e.svc.Verify(ctx, refused.ID); err != nil {
		t.Fatal(err)
	}
	if st, reason := e.status(t, refused.ID); st != "unconfirmed" || reason == nil || *reason != "query_rejected" {
		t.Fatalf("status = %s/%v", st, reason)
	}
	rc, err = e.svc.Recheck(ctx, refused.ID, "staff:1")
	if err != nil || rc.Outcome != "pending" || rc.Status != "unconfirmed" || !strings.HasPrefix(rc.ResultDesc, "M-Pesa has no answer yet: ") {
		t.Fatalf("query refused: %+v %v", rc, err)
	}

	// M-Pesa down while the operator waits: reported as such, the top-up untouched
	e.mock.SetNextFault(darajamock.Fault{QueryStatus: http.StatusServiceUnavailable})
	down := e.topUp(t, user, 90)
	e.run(t)
	before, _ := e.status(t, down.ID)
	if _, err := e.svc.Recheck(ctx, down.ID, "staff:1"); !errors.Is(err, payments.ErrDarajaUnavailable) {
		t.Fatalf("outage: %v", err)
	}
	if after, _ := e.status(t, down.ID); after != before || e.balance(t, user) != 0 {
		t.Fatalf("status %s -> %s balance %d", before, after, e.balance(t, user))
	}

	// and a top-up that does not exist
	if _, err := e.svc.Recheck(ctx, "01a19000-0000-7000-8000-0000000000dd", "staff:1"); !errors.Is(err, payments.ErrNotFound) {
		t.Fatalf("unknown top-up: %v", err)
	}
	if err := e.svc.Dismiss(ctx, "01a19000-0000-7000-8000-0000000000dd", "not found", "staff:1"); !errors.Is(err, payments.ErrNotFound) {
		t.Fatalf("dismiss of an unknown top-up: %v", err)
	}
}

// In the sandbox Safaricom's late callback settles the top-up with its own receipt. A different receipt the player
// had claimed is rejected with the reason, so the player sees which receipt was credited.
func TestReconcile_InTheSandboxALateCallbackSettlesThePlayersOpenClaim(t *testing.T) {
	e := sandbox(t)
	ctx := context.Background()
	user, msisdn := e.player(t)
	tu, checkout := paidWithoutCallback(t, e, user, 100)
	if _, err := e.svc.ClaimReceipt(ctx, user, tu.ID, "PL12345678"); err != nil {
		t.Fatal(err)
	}
	receipt := e.mock.ReceiptOf(checkout)
	if err := e.svc.HandleCallback(ctx, stkCallback(checkout, 100, receipt, msisdn)); err != nil {
		t.Fatal(err)
	}
	got := e.get(t, user, tu.ID)
	if got.Status != payments.StatusCredited || e.balance(t, user) != 100 || e.receipt(t, tu.ID) != receipt {
		t.Fatalf("payment %+v balance %d", got, e.balance(t, user))
	}
	if got.Claim == nil || got.Claim.Status != payments.ClaimRejected || got.Claim.Note != "M-Pesa's callback carried receipt "+receipt {
		t.Fatalf("claim = %+v", got.Claim)
	}

	// a recheck finds an orphaned paid callback and settles from it, as the sandbox does
	tu2, checkout2 := paidWithoutCallback(t, e, user, 40)
	receipt2 := e.mock.ReceiptOf(checkout2)
	if _, err := e.svc.ClaimReceipt(ctx, user, tu2.ID, receipt2); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO payments.stk_orphan_callbacks (checkout_request_id, raw_payload) VALUES ($1, $2)`,
		checkout2, stkCallback(checkout2, 40, receipt2, msisdn)); err != nil {
		t.Fatal(err)
	}
	rc, err := e.svc.Recheck(ctx, tu2.ID, "staff:1")
	if err != nil || rc.Outcome != "credited" || rc.Status != "credited" || e.balance(t, user) != 140 {
		t.Fatalf("recheck: %+v %v balance %d", rc, err, e.balance(t, user))
	}
	if got := e.get(t, user, tu2.ID); got.Claim == nil || got.Claim.Status != payments.ClaimConfirmed {
		t.Fatalf("claim = %+v", got.Claim)
	}
}

// M-Pesa's answer about a claimed receipt can be lost (Daraja accepts the request and never posts the result). The
// claim is not stuck: the sweep asks again once the wait is over, and nothing is credited meanwhile.
func TestReconcile_TheSweepAsksAgainAboutAClaimWhoseAnswerNeverCame(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, msisdn := e.player(t)
	tu, _ := paidWithoutCallback(t, e, user, 100)
	receipt := paybillReceipt(t, e, 100, msisdn, darajamock.C2BFault{StatusDrop: true})
	if _, err := e.svc.ClaimReceipt(ctx, user, tu.ID, receipt); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if got := e.get(t, user, tu.ID); got.Claim == nil || got.Claim.Status != payments.ClaimChecking {
		t.Fatalf("claim = %+v, want checking", got.Claim)
	}
	// not before the wait is over
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	for _, j := range e.q.take() {
		if j.kind == "claim" {
			t.Fatalf("claim %s asked about again at once", j.paymentID)
		}
	}
	e.clk.Advance(15 * time.Minute)
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if reqs := e.mock.StatusRequests(); len(reqs) != 2 || reqs[0] != receipt || reqs[1] != receipt {
		t.Fatalf("transaction status requests = %v, want the receipt asked about twice", reqs)
	}
	if got := e.get(t, user, tu.ID); got.Status == payments.StatusCredited || got.Claim == nil || got.Claim.Status != payments.ClaimChecking || e.balance(t, user) != 0 {
		t.Fatalf("payment %+v claim %+v balance %d", got, got.Claim, e.balance(t, user))
	}
}

// Exports that print only the initiation time, full local numbers, and rows the platform cannot read: each unreadable
// row is reported by line (never guessed at), and local numbers are read in 254 form so they compare with the prompts.
func TestStatement_ReadsOtherExportShapesAndReportsWhatItCannotRead(t *testing.T) {
	body := "Receipt No.,Initiation Time,Paid In,Other Party Info,Details\n" +
		"LC11111111,2026-10-30 18:34:51,100.00,0712345678 - JANE,Pay Bill\n" +
		"LC22222222,30/10/2026 18:40:00,50,712345678 - JANE,Pay Bill\n" +
		",2026-10-30 18:41:00,50,0712345678 - JANE,Pay Bill\n" +
		"LC33333333,yesterday,50,0712345678 - JANE,Pay Bill\n" +
		"LC44444444,2026-10-30 18:42:00,-50,0712345678 - JANE,Pay Bill\n"
	rows, skipped, err := payments.ParseStatement(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].PartyNumber != "254712345678" || rows[1].PartyNumber != "254712345678" || rows[0].PaidInKES != 100 {
		t.Fatalf("rows = %+v", rows)
	}
	if !rows[0].CompletedAt.Equal(time.Date(2026, 10, 30, 15, 34, 51, 0, time.UTC)) { // Nairobi time
		t.Fatalf("time = %v", rows[0].CompletedAt)
	}
	want := []string{"line 4: no receipt", "line 5: unreadable time yesterday", "line 6: unreadable amount -50"}
	if len(skipped) != len(want) {
		t.Fatalf("skipped = %v", skipped)
	}
	for i := range want {
		if skipped[i] != want[i] {
			t.Errorf("skipped[%d] = %q, want %q", i, skipped[i], want[i])
		}
	}
}

// A full number on the statement is the payer: a row from another number never matches a prompt, however well the
// amount and time fit; one from the prompt's own number does.
func TestStatement_AFullNumberOnlyMatchesItsOwnPrompt(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user, msisdn := e.player(t)
	start, _ := e.clk.Now(ctx, e.pool)
	tu, _ := paidWithoutCallback(t, e, user, 100)
	at := start.Add(5 * time.Minute).In(time.FixedZone("EAT", 3*3600)).Format("2006-01-02 15:04:05")
	body := "Receipt No.,Completion Time,Paid In,Other Party Info\n" +
		"FN11111111," + at + ",100.00,0799000222 - SOMEONE ELSE\n" +
		"FN22222222," + at + ",100.00," + local(msisdn) + " - JANE\n"
	rep, err := e.svc.MatchStatement(ctx, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	type found struct{ kind, payment, reason string }
	kinds := map[string]found{}
	for _, m := range rep.Matches {
		kinds[m.Row.Receipt] = found{m.Kind, m.PaymentID, m.Reason}
	}
	if k := kinds["FN11111111"]; k.kind != "unmatched" || k.payment != "" {
		t.Fatalf("another number's row = %+v", k)
	}
	if k := kinds["FN22222222"]; k.kind != "matched" || k.payment != tu.ID || k.reason != "same amount, number and time" {
		t.Fatalf("the player's row = %+v", k)
	}
	if len(rep.ToCredit) != 1 || rep.ToCredit[0].PaymentID != tu.ID || rep.ToCredit[0].Receipt != "FN22222222" {
		t.Fatalf("to credit = %+v", rep.ToCredit)
	}
}
