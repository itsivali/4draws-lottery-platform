//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/dataexport"
)

func TestAPI_APlayerIsEmailedACopyOfTheirDataOnceADay(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	user := e.userID(t, s)
	e.canPlay(t, s, 1000) // an identity, a confirmed number and money in the wallet
	if r := e.do(t, http.MethodPost, "/v1/plays", s.access, map[string]any{}); r.Code != http.StatusCreated && r.Code != http.StatusOK {
		t.Fatalf("play: %d %s", r.Code, r.Raw)
	}

	// a copy goes only to an address the player has confirmed (ADR 0040)
	if r := e.do(t, http.MethodPost, "/v1/account/data-export", s.access, nil); r.Code != http.StatusConflict || !hasType(r, "email-not-verified") || len(e.mail.msgs) != 0 {
		t.Fatalf("to an unconfirmed address: %d %s", r.Code, r.Raw)
	}
	e.exec(t, `UPDATE identity.users SET email_verified_at = now() WHERE id = $1`, user)
	r := e.do(t, http.MethodPost, "/v1/account/data-export", s.access, nil)
	if r.Code != http.StatusAccepted || r.Body["next_allowed_at"] == nil {
		t.Fatalf("export: %d %s", r.Code, r.Raw)
	}
	masked, _ := r.Body["sent_to"].(string)
	if masked == s.email || !strings.HasPrefix(masked, s.email[:1]+"***@") {
		t.Fatalf("the answer shows the address as %q", masked)
	}
	if len(e.mail.msgs) != 1 {
		t.Fatalf("sent %d messages", len(e.mail.msgs))
	}
	m := e.mail.msgs[0]
	// two files: the page to read first, then the same data for a program
	if len(m.To) != 1 || m.To[0] != s.email || len(m.Attachments) != 2 || m.Attachments[0].ContentType != "text/html" ||
		m.Attachments[1].ContentType != "application/json" || !strings.HasPrefix(m.Attachments[0].Name, "4draws-data-") ||
		!strings.HasSuffix(m.Attachments[0].Name, ".html") || !strings.HasSuffix(m.Attachments[1].Name, ".json") {
		t.Fatalf("message to %v with %d attachments", m.To, len(m.Attachments))
	}
	page := string(m.Attachments[0].Data)
	for _, want := range []string{"<h1>Your data", "4 DRAWS TICKET LTD", "You and your account", s.email, "Plays", "Wallet movements", "ends in "} {
		if !strings.Contains(page, want) {
			t.Fatalf("the readable page lacks %q", want)
		}
	}
	if strings.Contains(page, "<script") {
		t.Fatal("the readable page carries a script")
	}
	m.Attachments = m.Attachments[1:] // the checks below are of the data file
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(m.Attachments[0].Data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"about", "account", "identity", "sign_ins", "devices", "deposit_limits", "breaks_and_exclusions",
		"top_ups", "wallet", "plays", "draw_entries", "prizes", "withdrawals_and_payouts", "sign_in_links"} {
		if _, ok := doc[section]; !ok {
			t.Fatalf("the file has no %q section: %s", section, m.Attachments[0].Data)
		}
	}
	var account map[string]any
	_ = json.Unmarshal(doc["account"], &account)
	if account["phone_number"] != s.msisdn || account["email"] != s.email || account["user_id"] != user {
		t.Fatalf("account section: %s", doc["account"])
	}
	var plays, wallet []map[string]any
	_ = json.Unmarshal(doc["plays"], &plays)
	_ = json.Unmarshal(doc["wallet"], &wallet)
	if len(plays) != 1 || plays[0]["slot"] == nil || len(wallet) == 0 {
		t.Fatalf("plays %s, wallet %s", doc["plays"], doc["wallet"])
	}
	// the whole ID number never travels by email, and nothing internal rides along
	file := string(m.Attachments[0].Data)
	var ident map[string]any
	_ = json.Unmarshal(doc["identity"], &ident)
	if ending, _ := ident["id_number_ending"].(string); len(ending) != 3 {
		t.Fatalf("identity section: %s", doc["identity"])
	}
	for _, secret := range []string{"password", "token_hash", "idempotency", "fingerprint", "raw_callback", "national_id_hash"} {
		if strings.Contains(strings.ReplaceAll(file, "Your password (stored only as a one-way hash)", ""), secret) {
			t.Fatalf("the file mentions %q", secret)
		}
	}
	// recorded with its size and hash, and audited as the player's own act
	var n, audited int
	if err := e.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM identity.data_exports WHERE user_id = $1 AND bytes = $2),
		(SELECT count(*) FROM audit.audit_log WHERE action = 'identity.data_exported' AND actor_type = 'player' AND entity_id = $1::text)`,
		user, len(m.Attachments[0].Data)).Scan(&n, &audited); err != nil || n != 1 || audited != 1 {
		t.Fatalf("recorded %d, audited %d (%v)", n, audited, err)
	}

	// once in 24 hours
	if r := e.do(t, http.MethodPost, "/v1/account/data-export", s.access, nil); r.Code != http.StatusTooManyRequests || !hasType(r, "data-export-too-soon") || len(e.mail.msgs) != 1 {
		t.Fatalf("a second copy the same day: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/account/data-export", "", nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("without a session: %d", r.Code)
	}
	// (the access token does not live a day, so the service is asked directly once the clock has moved)
	e.clk.Advance(23 * time.Hour)
	var soon dataexport.TooSoonError
	if _, err := e.deps.DataExport.Send(context.Background(), user); !errors.As(err, &soon) {
		t.Fatalf("after 23 hours: %v", err)
	}
	e.clk.Advance(2 * time.Hour)
	if rc, err := e.deps.DataExport.Send(context.Background(), user); err != nil || len(e.mail.msgs) != 2 || rc.SentTo != masked {
		t.Fatalf("the next day: %+v %v, %d sent", rc, err, len(e.mail.msgs))
	}
}

func TestAPI_ACopyNeedsAnEmailAddressAndAMailServer(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	if _, err := e.pool.Exec(context.Background(), `UPDATE identity.users SET email = NULL WHERE msisdn = $1`, s.msisdn); err != nil {
		t.Fatal(err)
	}
	if r := e.do(t, http.MethodPost, "/v1/account/data-export", s.access, nil); r.Code != http.StatusConflict || !hasType(r, "no-email") || len(e.mail.msgs) != 0 {
		t.Fatalf("without an email address: %d %s", r.Code, r.Raw)
	}
	off := setupWith(t, func(d *apiDeps) { d.DataExport = nil })
	if r := off.do(t, http.MethodPost, "/v1/account/data-export", off.signIn(t).access, nil); r.Code != http.StatusServiceUnavailable || !hasType(r, "data-export-unavailable") {
		t.Fatalf("without a mail server: %d %s", r.Code, r.Raw)
	}
}
