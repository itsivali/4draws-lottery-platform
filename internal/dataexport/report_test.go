package dataexport_test

import (
	"os"
	"strings"
	"testing"

	"github.com/itsivali/lottery-platform/internal/dataexport"
)

const sample = `{
 "about": {"generated_at": "2026-11-11T09:30:00Z", "not_included": "Your password and the full ID number."},
 "account": {"user_id": "0192f000-0000-7000-8000-000000000001", "phone_number": "254712345678", "email": "jane@example.com",
   "status": "active", "identity_status": "verified", "opened_at": "2026-10-01T06:15:00+00:00", "phone_confirmed_at": "2026-10-01T06:20:00+00:00", "closure_requested_at": null},
 "identity": {"full_name": "Jane Wanjiru <script>alert(1)</script>", "id_number_ending": "678", "status": "verified"},
 "top_ups": [
   {"at": "2026-10-01T06:20:00+00:00", "amount_requested_kes": 1000, "amount_confirmed_kes": 1000, "status": "succeeded", "channel": "stk", "mpesa_receipt": "SJA1ABCDE2", "phone_number": "254712345678", "failure_reason": null, "reversed_at": null},
   {"at": "2026-10-05T17:02:00+00:00", "amount_requested_kes": 250000, "amount_confirmed_kes": null, "status": "failed", "channel": "stk", "mpesa_receipt": null, "phone_number": "254712345678", "failure_reason": "cancelled_by_user", "reversed_at": null}],
 "wallet": [{"at": "2026-10-01T06:20:00+00:00", "kind": "deposit", "amount_kes": 950, "for": "payment"}, {"at": "2026-10-01T06:25:00+00:00", "kind": "stake", "amount_kes": -100, "for": "ticket"}],
 "plays": [{"at": "2026-10-01T06:25:00+00:00", "play_date": "2026-10-01", "number_that_day": 1, "price_kes": 100, "slot": 0, "lucky": true, "status": "paid"}],
 "draw_entries": [{"draw": "daily", "period_start": "2026-10-01", "period_end": "2026-10-01", "entry_number": 7, "from_play_on": "2026-10-01", "draw_status": "completed"}],
 "prizes": [], "withdrawals_and_payouts": [{"at": "2026-10-09T10:00:00+00:00", "kind": "withdrawal", "gross_kes": 850, "tax_kes": 0, "net_kes": 850, "status": "succeeded", "mpesa_receipt": "SJB2FGHIJ3", "phone_number": "254712345678", "settled_at": "2026-10-09T10:01:00+00:00"}],
 "deposit_limits": [{"period": "daily", "amount_kes": 2000, "requested_at": "2026-10-02T08:00:00+00:00", "effective_from": "2026-10-02T08:00:00+00:00", "cancelled_at": null}],
 "breaks_and_exclusions": [{"kind": "self_exclusion", "starts_at": "2026-10-20T08:00:00+00:00", "ends_at": null}],
 "devices": [{"platform": "android", "first_seen_at": "2026-10-01T06:15:00+00:00", "last_seen_at": "2026-11-10T19:00:00+00:00"}],
 "sign_ins": [{"at": "2026-10-01T06:15:00+00:00", "platform": "android", "ended_at": null}],
 "sign_in_links": [], "copies_of_this_data": []
}`

func TestReport_IsAReadablePageWithEverySectionAndNoScript(t *testing.T) {
	page, err := dataexport.Report([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	if out := os.Getenv("REPORT_OUT"); out != "" { // to look at it: REPORT_OUT=/tmp/r.html go test ./internal/dataexport
		_ = os.WriteFile(out, page, 0o600)
	}
	for _, want := range []string{
		"<h1>Your data, Jane Wanjiru", "4 DRAWS TICKET LTD", "Prepared 11 Nov 2026, 12:30 EAT", // East Africa Time
		"ends in 678", "254712345678", "jane@example.com",
		"Top-ups", "KES 1,000", "KES 250,000", "SJA1ABCDE2", "M-Pesa prompt", "Cancelled by user", "1 Oct 2026, 09:20",
		"Wallet movements", "− KES 100", "Plays", ">Yes<", "Draw entries", "Daily",
		"You have won no prizes yet.", "Withdrawals and payouts", "KES 850", "SJB2FGHIJ3",
		"Deposit limits", "KES 2,000", "Self exclusion", "Android", "You have linked no Google or Apple account.",
		`<a href="#top-ups">Top-ups</a> (2)`, "privacy@4draws.co.ke",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	// nothing a player typed can become markup, and the page runs no script
	if strings.Contains(html, "<script") || !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("a name with markup in it was not escaped")
	}
	// a column nobody has a value in is left out (no top-up here was reversed)
	if strings.Contains(html, ">Reversed<") {
		t.Error("an empty column is shown")
	}
	if strings.Contains(html, "http://") || strings.Contains(html, "https://") {
		t.Error("the page points at the network")
	}
}
