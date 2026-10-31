//go:build integration

package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/staff"
)

// The back office helps with audits (owner, 2026-10-31): the activity log is filtered by day, action, entity and
// actor and taken away as a hashed, audited file.
func TestStaffAPI_ActivityIsFilteredAndExported(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	user := e.userID(t, s)
	e.canPlay(t, s, 1000)
	if r := e.doKey(t, http.MethodPost, "/v1/plays", s.access, "p-1", nil); r.Code != http.StatusCreated {
		t.Fatalf("play: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/payments/topups", s.access, map[string]any{"amount_kes": 50}); r.Code != http.StatusAccepted {
		t.Fatalf("top-up: %d %s", r.Code, r.Raw)
	}
	// a real staff member, so the file can name them
	ctx := context.Background()
	rec, err := e.deps.StaffServices.Members.Invite(ctx, staff.Member{ID: "0192f000-0000-7000-8000-00000000aaaa", Name: "Ada", Roles: []staff.Role{staff.Admin}}, "grace@example.com", "Grace Wanjiku", []staff.Role{staff.Staff}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO audit.audit_log (actor_type, actor_id, action, entity_type, entity_id, details) VALUES ('staff', $1, 'identity.account_suspended', 'user', $2, '{"reason":"test"}')`, "staff:"+rec.ID, user); err != nil {
		t.Fatal(err)
	}
	admin := e.member(staff.Admin)
	events := func(query string) []map[string]any {
		t.Helper()
		r := e.do(t, http.MethodGet, "/v1/staff/activity?"+query, admin, nil)
		if r.Code != http.StatusOK {
			t.Fatalf("activity?%s: %d %s", query, r.Code, r.Raw)
		}
		raw, _ := r.Body["events"].([]any)
		out := make([]map[string]any, 0, len(raw))
		for _, ev := range raw {
			out = append(out, ev.(map[string]any))
		}
		return out
	}
	// the action's prefix
	byAction := events("action=payment.")
	if len(byAction) == 0 {
		t.Fatal("no payment events")
	}
	for _, ev := range byAction {
		if !strings.HasPrefix(ev["action"].(string), "payment.") {
			t.Fatalf("action filter let through %v", ev["action"])
		}
	}
	// the entity, and the actor
	if got := events("entity_type=user&entity_id=" + user); len(got) == 0 || got[0]["entity_id"] != user {
		t.Fatalf("entity filter: %v", got)
	}
	byActor := events("actor_id=" + user)
	if len(byActor) == 0 {
		t.Fatal("no events by the player")
	}
	for _, ev := range byActor {
		if ev["actor_id"] != user {
			t.Fatalf("actor filter let through %v", ev["actor_id"])
		}
	}
	if got := events("actor_id=staff:" + rec.ID); len(got) != 1 || got[0]["action"] != "identity.account_suspended" {
		t.Fatalf("staff actor filter: %v", got)
	}
	// Nairobi days, inclusive: today holds everything, a day long gone nothing
	today := clock.InNairobi(time.Now()).Format(time.DateOnly)
	if got := events("from=" + today + "&to=" + today); len(got) == 0 {
		t.Fatal("today's events missing")
	}
	if got := events("from=2000-01-01&to=2000-01-02"); len(got) != 0 {
		t.Fatalf("events in the year 2000: %v", got)
	}
	if got := events("to=2000-01-01"); len(got) != 0 {
		t.Fatalf("events before 2000: %v", got)
	}
	for _, bad := range []string{"from=2026-13-01", "to=yesterday", "from=2026-10-30&to=2026-10-01", "actor_type=robot"} {
		if r := e.do(t, http.MethodGet, "/v1/staff/activity?"+bad, admin, nil); r.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", bad, r.Code, r.Raw)
		}
	}
	// the filters combine
	if got := events("action=identity.&actor_type=staff&entity_id=" + user); len(got) != 1 {
		t.Fatalf("combined filters: %v", got)
	}

	// the export: the same filters, a hashed file, the staff member named, and the export itself on the record
	r := e.do(t, http.MethodGet, "/v1/staff/activity/export?entity_id="+user+"&actor_type=staff", admin, nil)
	if r.Code != http.StatusOK || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/csv") {
		t.Fatalf("export: %d %s %s", r.Code, r.Header.Get("Content-Type"), r.Raw)
	}
	sum := sha256.Sum256([]byte(r.Raw))
	if r.Header.Get("X-File-SHA256") != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash header %s, file hashes to %x", r.Header.Get("X-File-SHA256"), sum)
	}
	rows, err := csv.NewReader(strings.NewReader(r.Raw)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rows[0], ",") != "id,at_eat,actor_type,actor_id,actor_name,action,entity_type,entity_id,details_json" {
		t.Fatalf("header: %v", rows[0])
	}
	if r.Header.Get("X-File-Rows") != "1" || len(rows) != 2 {
		t.Fatalf("rows: header %s, %d lines: %s", r.Header.Get("X-File-Rows"), len(rows)-1, r.Raw)
	}
	if row := rows[1]; row[2] != "staff" || row[3] != "staff:"+rec.ID || row[4] != "Grace Wanjiku" || row[5] != "identity.account_suspended" || row[7] != user || !strings.Contains(row[8], `"reason"`) ||
		!strings.HasPrefix(row[1], today) {
		t.Fatalf("row: %v", row)
	}
	if strings.Contains(r.Header.Get("Content-Disposition"), "activity-all.csv") == false {
		t.Fatalf("disposition: %s", r.Header.Get("Content-Disposition"))
	}
	var details string
	if err := e.pool.QueryRow(ctx, `SELECT details::text FROM audit.audit_log WHERE action = 'activity.exported' AND actor_type = 'staff'`).Scan(&details); err != nil ||
		!strings.Contains(details, `"rows": 1`) || !strings.Contains(details, `"sha256": "`+r.Header.Get("X-File-SHA256")+`"`) || !strings.Contains(details, `"entity_id": "`+user+`"`) || !strings.Contains(details, `"actor_type": "staff"`) {
		t.Fatalf("the export is not on the record: %s (%v)", details, err)
	}
	// a file that would be too big is refused before anything is sent
	e.ownerExec(t, `INSERT INTO audit.audit_log (actor_type, action, entity_type, entity_id) SELECT 'system', 'bulk.test', 'bulk', g::text FROM generate_series(1, 100001) g`)
	if r := e.do(t, http.MethodGet, "/v1/staff/activity/export?entity_type=bulk", admin, nil); r.Code != http.StatusBadRequest || !hasType(r, "too-many-rows") {
		t.Fatalf("over the cap: %d %s", r.Code, r.Header.Get("Content-Type"))
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/activity/export?entity_type=bulk&entity_id=7", admin, nil); r.Code != http.StatusOK || r.Header.Get("X-File-Rows") != "1" {
		t.Fatalf("narrowed: %d %s", r.Code, r.Header.Get("X-File-Rows"))
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/activity/export?from=2026-13-01", admin, nil); r.Code != http.StatusBadRequest {
		t.Fatalf("bad day on export: %d", r.Code)
	}
	// staff may not take the audit trail away; an admin may
	if r := e.do(t, http.MethodGet, "/v1/staff/activity/export", e.member(staff.Staff), nil); r.Code != http.StatusForbidden {
		t.Fatalf("staff exported the audit trail: %d", r.Code)
	}
}

// One player's whole story in one place, and as a file (owner, 2026-10-31).
func TestStaffAPI_PlayerTimelineAndPack(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	user := e.userID(t, s)
	if r := e.do(t, http.MethodPost, "/v1/kyc", s.access, map[string]any{"national_id": "24681357", "full_name": "Amina Otieno", "date_of_birth": "1992-03-04"}); r.Code != http.StatusAccepted {
		t.Fatalf("kyc: %d %s", r.Code, r.Raw)
	}
	e.drainKYC(t)
	e.canPlay(t, s, 1000) // 950 in the wallet
	for i := range 2 {
		if r := e.doKey(t, http.MethodPost, "/v1/plays", s.access, "p-"+string(rune('a'+i)), nil); r.Code != http.StatusCreated {
			t.Fatalf("play %d: %d %s", i, r.Code, r.Raw)
		}
	}
	if r := e.do(t, http.MethodPost, "/v1/payments/topups", s.access, map[string]any{"amount_kes": 50}); r.Code != http.StatusAccepted {
		t.Fatalf("top-up: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPut, "/v1/rg/limits", s.access, map[string]any{"period": "daily", "amount_kes": 500}); r.Code >= 300 {
		t.Fatalf("limit: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/rg/timeout", s.access, map[string]any{"duration": "24h"}); r.Code >= 300 {
		t.Fatalf("timeout: %d %s", r.Code, r.Raw)
	}
	op := e.member(staff.Staff)
	r := e.do(t, http.MethodGet, "/v1/staff/players/"+user, op, nil)
	if r.Code != http.StatusOK {
		t.Fatalf("timeline: %d %s", r.Code, r.Raw)
	}
	for _, k := range []string{"player", "wallet", "top_ups", "plays", "payouts", "identity", "limits", "events"} {
		if _, ok := r.Body[k]; !ok {
			t.Fatalf("timeline lacks %s: %s", k, r.Raw)
		}
	}
	player := r.Body["player"].(map[string]any)
	if player["user_id"] != user || player["msisdn"] != s.msisdn || player["full_name"] != "Amina Otieno" || player["national_id"] != "24681357" ||
		player["kyc_status"] != "verified" || player["plays"] != float64(2) || player["status"] != "active" {
		t.Fatalf("player: %v", player)
	}
	wallet := r.Body["wallet"].(map[string]any)
	txs := wallet["transactions"].([]any)
	if wallet["balance_kes"] != float64(750) || wallet["debt_kes"] != float64(0) || len(txs) < 3 {
		t.Fatalf("wallet: %v", wallet)
	}
	kinds := map[string]bool{}
	for _, tx := range txs {
		kinds[tx.(map[string]any)["kind"].(string)] = true
	}
	if !kinds["deposit"] || !kinds["stake"] {
		t.Fatalf("wallet movements: %v", kinds)
	}
	topUps := r.Body["top_ups"].([]any)
	if len(topUps) != 1 || topUps[0].(map[string]any)["status"] != "initiated" || topUps[0].(map[string]any)["amount_requested_kes"] != float64(50) {
		t.Fatalf("top-ups: %v", topUps)
	}
	plays := r.Body["plays"].(map[string]any)
	if plays["count"] != float64(2) || len(plays["last"].([]any)) != 2 || plays["last"].([]any)[0].(map[string]any)["seq"] != float64(2) || plays["last"].([]any)[0].(map[string]any)["slot"] == nil {
		t.Fatalf("plays: %v", plays)
	}
	if len(r.Body["payouts"].([]any)) != 0 {
		t.Fatalf("payouts: %v", r.Body["payouts"])
	}
	id := r.Body["identity"].(map[string]any)
	kyc := id["kyc"].([]any) // the submission as the mock registry left it, then the row the review would supersede
	if len(kyc) < 1 || kyc[0].(map[string]any)["status"] == "" || kyc[0].(map[string]any)["provider"] != "mock" || kyc[0].(map[string]any)["created_at"] == nil {
		t.Fatalf("kyc history: %v", kyc)
	}
	for _, k := range []string{"full_name", "national_id", "date_of_birth"} {
		if _, ok := kyc[0].(map[string]any)[k]; ok {
			t.Fatalf("kyc history carries %s", k)
		}
	}
	sessions := id["sessions"].([]any)
	if len(sessions) == 0 || sessions[0].(map[string]any)["platform"] != "android" || sessions[0].(map[string]any)["device_verdict"] == nil {
		t.Fatalf("sessions: %v", sessions)
	}
	limits := r.Body["limits"].(map[string]any)
	deposit := limits["deposit"].([]any)
	if len(deposit) != 1 || deposit[0].(map[string]any)["period"] != "daily" || deposit[0].(map[string]any)["amount_kes"] != float64(500) || deposit[0].(map[string]any)["pending"] != false {
		t.Fatalf("deposit limits: %v", deposit)
	}
	if restriction, _ := limits["restriction"].(map[string]any); restriction == nil || restriction["kind"] != "timeout" || restriction["until"] == nil {
		t.Fatalf("restriction: %v", limits["restriction"])
	}
	events := r.Body["events"].([]any)
	if len(events) == 0 {
		t.Fatal("no events about the player")
	}
	for _, ev := range events {
		m := ev.(map[string]any)
		if m["actor_id"] != user && m["entity_id"] != user && !strings.Contains(r.Raw, user) {
			t.Fatalf("an event not about the player: %v", m)
		}
	}
	// the view is on the record once, and the unmasking of the name
	count := func(action string) int {
		var n int
		if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit.audit_log WHERE action = $1 AND entity_id = $2 AND actor_type = 'staff'`, action, user).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("identity.player_viewed") != 1 || count("kyc.unmask") < 1 {
		t.Fatalf("audited: viewed %d, unmasked %d", count("identity.player_viewed"), count("kyc.unmask"))
	}
	for _, bad := range []string{"0192f000-0000-7000-8000-000000000001", "not-a-uuid"} {
		if r := e.do(t, http.MethodGet, "/v1/staff/players/"+bad, op, nil); r.Code != http.StatusNotFound || !hasType(r, "not-found") {
			t.Fatalf("%s: %d %s", bad, r.Code, r.Raw)
		}
		if r := e.do(t, http.MethodGet, "/v1/staff/players/"+bad+"/export", op, nil); r.Code != http.StatusNotFound {
			t.Fatalf("%s export: %d %s", bad, r.Code, r.Raw)
		}
	}

	// the player pack: every section, hashed, audited
	x := e.do(t, http.MethodGet, "/v1/staff/players/"+user+"/export", op, nil)
	if x.Code != http.StatusOK || !strings.HasPrefix(x.Header.Get("Content-Type"), "text/csv") || !strings.Contains(x.Header.Get("Content-Disposition"), "player-"+user+".csv") {
		t.Fatalf("pack: %d %s", x.Code, x.Raw)
	}
	sum := sha256.Sum256([]byte(x.Raw))
	if x.Header.Get("X-File-SHA256") != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash header %s, file hashes to %x", x.Header.Get("X-File-SHA256"), sum)
	}
	rows, err := csv.NewReader(strings.NewReader(x.Raw)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rows[0], ",") != "section,id,at_eat,kind,status,amount_kes,reference,details_json" || x.Header.Get("X-File-Rows") != itoa(len(rows)-1) {
		t.Fatalf("header %v, rows %s of %d", rows[0], x.Header.Get("X-File-Rows"), len(rows)-1)
	}
	sections := map[string]int{}
	for _, row := range rows[1:] {
		sections[row[0]]++
	}
	if sections["player"] != 1 || sections["wallet_transaction"] < 3 || sections["top_up"] != 1 || sections["play"] != 2 || sections["kyc_check"] < 1 ||
		sections["session"] < 1 || sections["deposit_limit"] != 1 || sections["restriction"] != 1 || sections["event"] < 3 {
		t.Fatalf("sections: %v", sections)
	}
	if !strings.Contains(x.Raw, "Amina Otieno") || !strings.Contains(x.Raw, "24681357") || !strings.Contains(x.Raw, s.msisdn) {
		t.Fatal("the pack lacks the player's name, ID number or number")
	}
	if count("identity.player_exported") != 1 || count("identity.player_viewed") != 1 {
		t.Fatalf("export audited %d times; views now %d", count("identity.player_exported"), count("identity.player_viewed"))
	}
	var details string
	if err := e.pool.QueryRow(context.Background(), `SELECT details::text FROM audit.audit_log WHERE action = 'identity.player_exported'`).Scan(&details); err != nil ||
		!strings.Contains(details, x.Header.Get("X-File-SHA256")) {
		t.Fatalf("export details: %s (%v)", details, err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
