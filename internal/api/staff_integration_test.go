//go:build integration

package api_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/crypto"
	"github.com/itsivali/lottery-platform/internal/staff"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

// fakeStaff stands in for the Keycloak token check: a token is a staff member's name.
type fakeStaff struct {
	mu      sync.Mutex
	members map[string]staff.Member
}

func (f *fakeStaff) Authenticate(_ context.Context, token string) (staff.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.members[token]
	if !ok {
		return staff.Member{}, staff.ErrUnauthenticated
	}
	if len(m.Roles) == 0 {
		return staff.Member{}, staff.ErrNoRole
	}
	return m, nil
}

var staffSeq int

// member makes a staff member with roles and returns their token.
func (e *env) member(roles ...staff.Role) string {
	e.staff.mu.Lock()
	defer e.staff.mu.Unlock()
	staffSeq++
	tok := fmt.Sprintf("staff-token-%d", staffSeq)
	e.staff.members[tok] = staff.Member{ID: fmt.Sprintf("0192f000-0000-7000-8000-%012d", staffSeq), Name: tok, Roles: roles}
	return tok
}

type staffRoute struct {
	method, path string
	role         staff.Role
}

var staffRoutes = []staffRoute{
	{http.MethodGet, "/v1/staff/topups/0192f000-0000-7000-8000-000000000001", staff.Staff},
	{http.MethodPost, "/v1/staff/topups/0192f000-0000-7000-8000-000000000001/recheck", staff.Staff},
	{http.MethodPost, "/v1/staff/topups/0192f000-0000-7000-8000-000000000001/resolve", staff.Staff},
	{http.MethodPost, "/v1/staff/topups/0192f000-0000-7000-8000-000000000001/dismiss", staff.Staff},
	{http.MethodPost, "/v1/staff/payments/statement/match", staff.Staff},
	{http.MethodPost, "/v1/staff/payments/statement/apply", staff.Staff},
	{http.MethodGet, "/v1/staff/payments/statements", staff.Staff},
	{http.MethodGet, "/v1/staff/kyc/pending", staff.Staff},
	{http.MethodGet, "/v1/staff/kyc/queue", staff.Staff},
	{http.MethodGet, "/v1/staff/members", staff.Admin},
	{http.MethodPost, "/v1/staff/members", staff.Admin},
	{http.MethodPut, "/v1/staff/members/0192f000-0000-7000-8000-000000000001/roles", staff.Admin},
	{http.MethodPost, "/v1/staff/members/0192f000-0000-7000-8000-000000000001/disable", staff.Admin},
	{http.MethodPost, "/v1/staff/members/0192f000-0000-7000-8000-000000000001/enable", staff.Admin},
	{http.MethodPost, "/v1/staff/members/0192f000-0000-7000-8000-000000000001/password", staff.Admin},
	{http.MethodPut, "/v1/staff/members/0192f000-0000-7000-8000-000000000001/name", staff.Admin},
	{http.MethodPut, "/v1/staff/me/name", staff.Staff},
	{http.MethodGet, "/v1/staff/metrics", staff.Admin},
	{http.MethodGet, "/v1/staff/finance", staff.Staff},
	{http.MethodGet, "/v1/staff/finance/export", staff.Staff},
	{http.MethodGet, "/v1/staff/ledger/export", staff.Staff},
	{http.MethodGet, "/v1/staff/reconciliations?area=payments", staff.Staff},
	{http.MethodPost, "/v1/staff/kyc/0192f000-0000-7000-8000-000000000001/approve", staff.Staff},
	{http.MethodPost, "/v1/staff/kyc/0192f000-0000-7000-8000-000000000001/reject", staff.Staff},
	{http.MethodPost, "/v1/staff/payments/reconcile", staff.Staff},
	{http.MethodPost, "/v1/staff/payments/reversals", staff.Staff},
	{http.MethodPost, "/v1/staff/payments/0192f000-0000-7000-8000-000000000001/resolve", staff.Staff},
	{http.MethodPost, "/v1/staff/suspense/RCPT1/credit", staff.Staff},
	{http.MethodPost, "/v1/staff/suspense/RCPT1/refund", staff.Staff},
	{http.MethodPost, "/v1/staff/paybill/RCPT1/recheck", staff.Staff},
	{http.MethodGet, "/v1/staff/draws", staff.Staff},
	{http.MethodGet, "/v1/staff/draws/overdue", staff.Staff},
	{http.MethodPost, "/v1/staff/draws/reconcile", staff.Staff},
	{http.MethodPost, "/v1/staff/draws/0192f000-0000-7000-8000-000000000001/sign", staff.Staff},
	{http.MethodPost, "/v1/staff/draws/0192f000-0000-7000-8000-000000000001/approve", staff.Staff},
	{http.MethodPost, "/v1/staff/draws/0192f000-0000-7000-8000-000000000001/void", staff.Staff},
	{http.MethodPost, "/v1/staff/game/reconcile", staff.Staff},
	{http.MethodGet, "/v1/staff/payouts", staff.Staff},
	{http.MethodPost, "/v1/staff/payouts/reconcile", staff.Staff},
	{http.MethodPost, "/v1/staff/payouts/0192f000-0000-7000-8000-000000000001/approve", staff.Staff},
	{http.MethodPost, "/v1/staff/payouts/0192f000-0000-7000-8000-000000000001/reject", staff.Staff},
	{http.MethodPost, "/v1/staff/payouts/0192f000-0000-7000-8000-000000000001/settle", staff.Staff},
	{http.MethodPost, "/v1/staff/prizes/0192f000-0000-7000-8000-000000000001/retry", staff.Staff},
	{http.MethodGet, "/v1/staff/tax/batches", staff.Staff},
	{http.MethodPost, "/v1/staff/tax/batches", staff.Staff},
	{http.MethodGet, "/v1/staff/tax/batches/1/file", staff.Staff},
	{http.MethodPost, "/v1/staff/tax/batches/1/remit", staff.Staff},
	{http.MethodGet, "/v1/staff/alerts", staff.Staff},
	{http.MethodPost, "/v1/staff/alerts/0192f000-0000-7000-8000-000000000001/acknowledge", staff.Staff},
	{http.MethodGet, "/v1/staff/closures", staff.Staff},
	{http.MethodPost, "/v1/staff/closures/0192f000-0000-7000-8000-000000000001/close", staff.Staff},
	{http.MethodGet, "/v1/staff/plays", staff.Staff},
	{http.MethodGet, "/v1/staff/topups", staff.Staff},
	{http.MethodGet, "/v1/staff/system", staff.Staff},
	{http.MethodGet, "/v1/staff/activity", staff.Admin},
	{http.MethodGet, "/v1/staff/players", staff.Staff},
	{http.MethodPost, "/v1/staff/players/0192f000-0000-7000-8000-000000000001/suspend", staff.Staff},
	{http.MethodPost, "/v1/staff/players/0192f000-0000-7000-8000-000000000001/reinstate", staff.Staff},
	{http.MethodPost, "/v1/staff/players/0192f000-0000-7000-8000-000000000001/password", staff.Staff},
	{http.MethodPost, "/v1/staff/players/0192f000-0000-7000-8000-000000000001/refund", staff.Admin},
	{http.MethodPost, "/v1/staff/players/0192f000-0000-7000-8000-000000000001/self-exclusion", staff.Staff},
	{http.MethodGet, "/v1/staff/activity/export", staff.Admin},
	{http.MethodGet, "/v1/staff/players/0192f000-0000-7000-8000-000000000001", staff.Staff},
	{http.MethodGet, "/v1/staff/players/0192f000-0000-7000-8000-000000000001/export", staff.Staff},
	{http.MethodGet, "/v1/staff/alerts/0192f000-0000-7000-8000-000000000001", staff.Staff},
	{http.MethodGet, "/v1/staff/operating", staff.Staff},
	{http.MethodPost, "/v1/staff/operating/stop", staff.Staff},
	{http.MethodPost, "/v1/staff/operating/resume", staff.Admin},
	{http.MethodPost, "/v1/staff/operating/live-requests", staff.Admin},
	{http.MethodPost, "/v1/staff/operating/live-requests/0192f000-0000-7000-8000-000000000001/approve", staff.Admin},
}

func TestStaffAPI_EveryRouteNeedsAStaffTokenWithTheRightRole(t *testing.T) {
	e := setup(t)
	player := e.signIn(t)
	noRole := e.member()
	staffOnly := e.member(staff.Staff)
	for _, rt := range staffRoutes {
		for name, tok := range map[string]string{"no token": "", "player token": player.access, "garbage": "x"} {
			if r := e.do(t, rt.method, rt.path, tok, `{}`); r.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with %s = %d, want 401", rt.method, rt.path, name, r.Code)
			}
		}
		if r := e.do(t, rt.method, rt.path, noRole, `{}`); r.Code != http.StatusForbidden {
			t.Errorf("%s %s without a role = %d, want 403", rt.method, rt.path, r.Code)
		}
		if rt.role == staff.Admin {
			if r := e.do(t, rt.method, rt.path, staffOnly, `{}`); r.Code != http.StatusForbidden || !hasType(r, "forbidden") {
				t.Errorf("%s %s as staff = %d, want 403 (admins only)", rt.method, rt.path, r.Code)
			}
		}
		// the role itself, and an admin (who may do everything staff do)
		for name, tok := range map[string]string{"its role": e.member(rt.role), "an admin": e.member(staff.Admin)} {
			if r := e.do(t, rt.method, rt.path, tok, `{}`); r.Code == http.StatusUnauthorized || r.Code == http.StatusForbidden {
				t.Errorf("%s %s refused %s: %d %s", rt.method, rt.path, name, r.Code, r.Raw)
			}
		}
		if rt.method != http.MethodGet {
			if r := e.doKey(t, rt.method, rt.path, e.member(rt.role), "", `{}`); r.Code != http.StatusBadRequest {
				t.Errorf("%s %s without an idempotency key = %d, want 400", rt.method, rt.path, r.Code)
			}
		}
	}
	// staff tokens never work on player routes
	if r := e.do(t, http.MethodGet, "/v1/account", e.member(staff.Staff), nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("staff token on a player route: %d", r.Code)
	}
}

func TestStaffAPI_MeShowsRoles(t *testing.T) {
	e := setup(t)
	r := e.do(t, http.MethodGet, "/v1/staff/me", e.member(staff.Staff), nil)
	if r.Code != http.StatusOK || len(r.Body["roles"].([]any)) != 1 || r.Body["roles"].([]any)[0] != "staff" || r.Body["must_change_password"] != false {
		t.Fatalf("me: %d %s", r.Code, r.Raw)
	}
}

func TestStaffAPI_KYCQueueCountsWaitingSubmissionsWithoutUnmaskingThem(t *testing.T) {
	e := setup(t)
	reviewer := e.member(staff.Staff)
	if r := e.do(t, http.MethodGet, "/v1/staff/kyc/queue", reviewer, nil); r.Code != http.StatusOK || r.Body["waiting"] != float64(0) ||
		r.Body["oldest_submitted_at"] != nil {
		t.Fatalf("empty queue: %d %s", r.Code, r.Raw)
	}
	for i, id := range []string{"87654321", "87654322"} {
		s := e.signIn(t)
		if r := e.do(t, http.MethodPost, "/v1/kyc", s.access, map[string]any{"national_id": id, "full_name": "Amina Otieno", "date_of_birth": "1992-03-04"}); r.Code != http.StatusAccepted {
			t.Fatalf("submit %d: %d %s", i, r.Code, r.Raw)
		}
	}
	e.drainKYC(t)
	r := e.do(t, http.MethodGet, "/v1/staff/kyc/queue", reviewer, nil)
	if oldest, _ := r.Body["oldest_submitted_at"].(string); r.Code != http.StatusOK || r.Body["waiting"] != float64(2) || oldest == "" {
		t.Fatalf("queue: %d %s", r.Code, r.Raw)
	}
	if len(r.Body) != 2 {
		t.Fatalf("the queue carries counts only, never personal details: %s", r.Raw)
	}
	var unmasked int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit.audit_log WHERE action = 'kyc.unmask'`).Scan(&unmasked); err != nil || unmasked != 0 {
		t.Fatalf("reading the queue unmasked %d submissions (%v)", unmasked, err)
	}
}

func TestStaffAPI_KYCReviewShowsTheSubmissionAndRecordsTheDecision(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	// not in the registry: the submission waits for a person
	if r := e.do(t, http.MethodPost, "/v1/kyc", s.access, map[string]any{"national_id": "87654321", "full_name": "Amina Otieno", "date_of_birth": "1992-03-04"}); r.Code != http.StatusAccepted {
		t.Fatalf("submit: %d %s", r.Code, r.Raw)
	}
	e.drainKYC(t)
	reviewer := e.member(staff.Staff)
	r := e.do(t, http.MethodGet, "/v1/staff/kyc/pending", reviewer, nil)
	subs, _ := r.Body["submissions"].([]any)
	if r.Code != http.StatusOK || len(subs) != 1 || subs[0].(map[string]any)["full_name"] != "Amina Otieno" {
		t.Fatalf("pending: %d %s", r.Code, r.Raw)
	}
	user := subs[0].(map[string]any)["user_id"].(string)
	for _, step := range []string{"reject", "approve"} {
		if r := e.do(t, http.MethodPost, "/v1/staff/kyc/"+user+"/"+step, reviewer, map[string]any{}); r.Code != http.StatusBadRequest || !hasType(r, "reason-required") {
			t.Fatalf("%s without a reason: %d", step, r.Code)
		}
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/kyc/"+user+"/approve", reviewer, map[string]any{"reason": "ID card matches the details"}); r.Code != http.StatusOK || r.Body["status"] != "verified" {
		t.Fatalf("approve: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/kyc", s.access, nil); r.Body["status"] != "verified" {
		t.Fatalf("player sees %s", r.Raw)
	}
	var actor string
	if err := e.pool.QueryRow(context.Background(), `SELECT actor_id FROM audit.audit_log WHERE action LIKE 'kyc.%' AND actor_type = 'staff' ORDER BY id DESC LIMIT 1`).Scan(&actor); err != nil ||
		!strings.HasPrefix(actor, "staff:0192f000-") {
		t.Fatalf("audited as %q (%v)", actor, err)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/kyc/"+user+"/approve", reviewer, map[string]any{"reason": "again"}); r.Code != http.StatusConflict || !hasType(r, "nothing-to-review") {
		t.Fatalf("second decision: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/kyc/queue", reviewer, nil); r.Body["waiting"] != float64(0) {
		t.Fatalf("a decided submission still waits: %s", r.Raw)
	}
}

// ownerExec runs SQL as the test database's owner, standing in for the worker's background steps.
func (e *env) ownerExec(t *testing.T, sql string, args ...any) {
	t.Helper()
	cfg := e.pool.Config().Copy()
	cfg.AfterConnect = nil
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

// freezeDaily schedules and freezes the daily draw of 21 October with the owner's role (as the worker does).
func (e *env) freezeDaily(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	cfg := e.pool.Config().Copy()
	cfg.AfterConnect = nil
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	nairobi := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, clock.Nairobi).UTC() }
	clk := clock.NewManual(nairobi(21, 15, 0))
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	kr, _ := crypto.NewKeyring(1, map[byte][]byte{1: k})
	svc := draws.NewService(pool, clk, kr, drawBeacon{}, ledger.NewStore())
	for _, at := range []time.Time{nairobi(21, 15, 0), nairobi(22, 0, 6)} {
		clk.Set(at)
		if err := svc.Schedule(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM draws.draws WHERE draw_type = 'daily' AND period_start = '2026-10-21'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestStaffAPI_ADrawIsSignedOnlyByTwoDifferentOfficials(t *testing.T) {
	e := setup(t)
	p := e.signIn(t)
	e.canPlay(t, p, 1000)
	if r := e.do(t, http.MethodPost, "/v1/plays", p.access, nil); r.Code != http.StatusCreated { // a draw with no entries is voided
		t.Fatalf("play: %d %s", r.Code, r.Raw)
	}
	id := e.freezeDaily(t)
	e.clk.Set(time.Date(2026, 10, 22, 0, 30, 0, 0, clock.Nairobi).UTC())
	alice, bob := e.member(staff.Staff), e.member(staff.Staff)
	r := e.do(t, http.MethodPost, "/v1/staff/draws/"+id+"/sign", alice, map[string]any{})
	if r.Code != http.StatusOK || r.Body["status"] != "frozen" {
		t.Fatalf("first signature: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/draws/"+id+"/sign", alice, map[string]any{}); r.Code != http.StatusConflict || !hasType(r, "already-signed") {
		t.Fatalf("the same official twice: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/draws/"+id+"/sign", bob, map[string]any{}); r.Code != http.StatusOK || r.Body["status"] != "signed" {
		t.Fatalf("second official: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/draws/"+id+"/void", bob, map[string]any{}); r.Code != http.StatusBadRequest {
		t.Fatalf("void without a reason: %d", r.Code)
	}
}

func TestStaffAPI_AnAccountClosesOnlyOnceItsMoneyIsOut(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	user := e.userID(t, s)
	support := e.member(staff.Staff)
	if r := e.do(t, http.MethodPost, "/v1/staff/closures/"+user+"/close", support, map[string]any{}); r.Code != http.StatusNotFound {
		t.Fatalf("close without a request: %d %s", r.Code, r.Raw)
	}
	e.canPlay(t, s, 1000) // 950 in the wallet
	e.do(t, http.MethodPost, "/v1/account/closure", s.access, nil)
	list := e.do(t, http.MethodGet, "/v1/staff/closures", support, nil)
	if c, _ := list.Body["closures"].([]any); len(c) != 1 || c[0].(map[string]any)["user_id"] != user {
		t.Fatalf("closures: %s", list.Raw)
	}
	var viewed int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit.audit_log WHERE action = 'identity.closures_viewed' AND actor_type = 'staff'`).Scan(&viewed); err != nil || viewed != 1 {
		t.Fatalf("viewing players' numbers audited %d times (%v), want once", viewed, err)
	}
	r := e.do(t, http.MethodPost, "/v1/staff/closures/"+user+"/close", support, map[string]any{})
	if r.Code != http.StatusConflict || !hasType(r, "closure-blocked") || !strings.Contains(r.Raw, "950") {
		t.Fatalf("close with money in the wallet: %d %s", r.Code, r.Raw)
	}
	// pay it out: withdraw everything, and M-Pesa confirms
	w := e.do(t, http.MethodPost, "/v1/payouts/withdrawals", s.access, map[string]any{"amount_kes": 950})
	if w.Code != http.StatusCreated {
		t.Fatalf("withdraw: %d %s", w.Code, w.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/closures/"+user+"/close", support, map[string]any{}); r.Code != http.StatusConflict || !strings.Contains(r.Raw, "payout") {
		t.Fatalf("close with a payout still moving: %d %s", r.Code, r.Raw)
	}
	payout := w.Body["id"].(string)
	// M-Pesa's answer was unclear and the payout went to manual settlement (the worker's job, done here as the owner)
	for _, st := range []string{"sending", "sent", "manual"} {
		e.ownerExec(t, `UPDATE payouts.payouts SET status = $2 WHERE id = $1`, payout, st)
	}
	// ADR 0032: one staff member proposes, a second confirms
	op, second := e.member(staff.Staff), e.member(staff.Staff)
	if r := e.do(t, http.MethodPost, "/v1/staff/payouts/"+payout+"/settle", op, map[string]any{"paid": true, "receipt": "SGH0000001"}); r.Code != http.StatusOK || r.Body["state"] != "manual" || r.Body["proposal"] == nil {
		t.Fatalf("proposal: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/payouts/"+payout+"/settle", op, map[string]any{"paid": true, "receipt": "SGH0000001"}); r.Code != http.StatusConflict || r.Body["title"] == nil {
		t.Fatalf("confirmed by the proposer: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/payouts/"+payout+"/settle", second, map[string]any{"paid": true, "receipt": "SGH0000001"}); r.Code != http.StatusOK || r.Body["state"] != "succeeded" {
		t.Fatalf("settle: %d %s", r.Code, r.Raw)
	}
	r = e.do(t, http.MethodPost, "/v1/staff/closures/"+user+"/close", support, map[string]any{})
	if r.Code != http.StatusOK || r.Body["closed_at"] == nil {
		t.Fatalf("close: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/account", s.access, nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("the closed account's session still works: %d", r.Code)
	}
	if list := e.do(t, http.MethodGet, "/v1/staff/closures", support, nil); len(list.Body["closures"].([]any)) != 0 {
		t.Fatalf("still listed: %s", list.Raw)
	}
}

func TestStaffAPI_UnconfiguredStaffSignInAnswers503(t *testing.T) {
	e := setupWith(t, func(d *apiDeps) { d.Staff = nil })
	if r := e.do(t, http.MethodGet, "/v1/staff/me", "anything", nil); r.Code != http.StatusServiceUnavailable || !hasType(r, "staff-unavailable") {
		t.Fatalf("%d %s", r.Code, r.Raw)
	}
}

func TestStaffAPI_AnAccountWithAStakeInADrawIsNotClosedUntilTheDrawIsSettled(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	user := e.userID(t, s)
	e.canPlay(t, s, 1000)
	if r := e.do(t, http.MethodPost, "/v1/plays", s.access, nil); r.Code != http.StatusCreated {
		t.Fatalf("play: %d %s", r.Code, r.Raw)
	}
	e.freezeDaily(t) // the day's draw (frozen) holds this player's chances
	// pay out the rest of the wallet so only the draw stake remains
	w := e.do(t, http.MethodPost, "/v1/payouts/withdrawals", s.access, map[string]any{"amount_kes": 850})
	if w.Code != http.StatusCreated {
		t.Fatalf("withdraw: %d %s", w.Code, w.Raw)
	}
	for _, st := range []string{"sending", "sent", "manual"} {
		e.ownerExec(t, `UPDATE payouts.payouts SET status = $2 WHERE id = $1`, w.Body["id"], st)
	}
	for _, m := range []string{e.member(staff.Staff), e.member(staff.Staff)} { // two people (ADR 0032)
		if r := e.do(t, http.MethodPost, "/v1/staff/payouts/"+w.Body["id"].(string)+"/settle", m, map[string]any{"paid": true, "receipt": "SGH0000002"}); r.Code != http.StatusOK {
			t.Fatalf("settle: %d %s", r.Code, r.Raw)
		}
	}
	e.do(t, http.MethodPost, "/v1/account/closure", s.access, nil)
	r := e.do(t, http.MethodPost, "/v1/staff/closures/"+user+"/close", e.member(staff.Staff), map[string]any{})
	if r.Code != http.StatusConflict || !hasType(r, "closure-blocked") || !strings.Contains(r.Raw, "draw") {
		t.Fatalf("closed with chances still in draws: %d %s", r.Code, r.Raw)
	}
}

func TestStaffAPI_AnAccountThatOwesMoneyIsNotClosedSilently(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	user := e.userID(t, s)
	e.do(t, http.MethodPost, "/v1/account/closure", s.access, nil)
	e.debtFor(t, user, 300)
	r := e.do(t, http.MethodPost, "/v1/staff/closures/"+user+"/close", e.member(staff.Staff), map[string]any{})
	if r.Code != http.StatusConflict || !strings.Contains(r.Raw, "owes") {
		t.Fatalf("closed an account in debt: %d %s", r.Code, r.Raw)
	}
}

// debtFor books a reversed top-up of gross against an empty wallet, so the player owes it (less the excise).
func (e *env) debtFor(t *testing.T, user string, gross int64) {
	t.Helper()
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		var payment string
		if err := tx.QueryRow(ctx, `SELECT platform.uuid_v7()::text`).Scan(&payment); err != nil {
			return err
		}
		ls := ledger.NewStore()
		p, err := ledger.DepositReversal(payment, user, gross, gross*5/100, 0)
		if err != nil {
			return err
		}
		_, err = wallet.NewService(ls).Apply(ctx, tx, p)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStaffAPI_PayoutsPageOldestFirstWithACursor(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	e.canPlay(t, s, 1000) // 950 in the wallet, phone proved, ID verified
	var ids []string
	for i := range 3 { // the day's three withdrawals, all waiting for M-Pesa
		r := e.doKey(t, http.MethodPost, "/v1/payouts/withdrawals", s.access, fmt.Sprintf("w-%d", i), map[string]any{"amount_kes": 100})
		if r.Code != http.StatusCreated {
			t.Fatalf("withdraw %d: %d %s", i, r.Code, r.Raw)
		}
		ids = append(ids, r.Body["id"].(string))
	}
	op := e.member(staff.Staff)
	first := e.do(t, http.MethodGet, "/v1/staff/payouts?state=pending&limit=2", op, nil)
	got := func(r resp) []string {
		var out []string
		for _, p := range r.Body["payouts"].([]any) {
			out = append(out, p.(map[string]any)["id"].(string))
		}
		return out
	}
	if first.Code != http.StatusOK || fmt.Sprint(got(first)) != fmt.Sprint(ids[:2]) || first.Body["next_after"] != ids[1] {
		t.Fatalf("first page: %d %s, want %v", first.Code, first.Raw, ids[:2])
	}
	second := e.do(t, http.MethodGet, "/v1/staff/payouts?state=pending&limit=2&after="+ids[1], op, nil)
	if second.Code != http.StatusOK || fmt.Sprint(got(second)) != fmt.Sprint(ids[2:]) || second.Body["next_after"] != nil {
		t.Fatalf("second page: %d %s, want %v", second.Code, second.Raw, ids[2:])
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/payouts?state=pending", op, nil); r.Code != http.StatusOK || len(r.Body["payouts"].([]any)) != 3 || r.Body["next_after"] != nil {
		t.Fatalf("one page: %d %s", r.Code, r.Raw)
	}
	for _, q := range []string{"after=not-a-uuid", "limit=0"} {
		if r := e.do(t, http.MethodGet, "/v1/staff/payouts?"+q, op, nil); r.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", q, r.Code, r.Raw)
		}
	}
}

func TestStaffAPI_PlayersAreListedSearchedAndSuspended(t *testing.T) {
	e := setup(t)
	a, b := e.signIn(t), e.signIn(t)
	if r := e.do(t, http.MethodPost, "/v1/kyc", a.access, map[string]any{"national_id": "12345678", "full_name": "Amina Otieno", "date_of_birth": "1992-03-04"}); r.Code != http.StatusAccepted {
		t.Fatalf("submit identity: %d %s", r.Code, r.Raw)
	}
	e.canPlay(t, a, 1000)
	if r := e.doKey(t, http.MethodPost, "/v1/plays", a.access, "pl-1", nil); r.Code != http.StatusCreated {
		t.Fatalf("play: %d %s", r.Code, r.Raw)
	}
	op := e.member(staff.Staff)
	list := e.do(t, http.MethodGet, "/v1/staff/players?limit=1", op, nil)
	players, _ := list.Body["players"].([]any)
	if list.Code != http.StatusOK || len(players) != 1 || list.Body["next_before"] == nil {
		t.Fatalf("first page: %d %s", list.Code, list.Raw)
	}
	newest := players[0].(map[string]any)
	if newest["msisdn"] != b.msisdn || newest["status"] != "active" || newest["plays"] != float64(0) {
		t.Fatalf("newest first: %v", newest)
	}
	second := e.do(t, http.MethodGet, "/v1/staff/players?limit=1&before="+list.Body["next_before"].(string), op, nil)
	older := second.Body["players"].([]any)[0].(map[string]any)
	if older["msisdn"] != a.msisdn || older["plays"] != float64(1) || older["balance_kes"] != float64(850) || older["kyc_status"] != "verified" || older["phone_verified"] != true {
		t.Fatalf("second page: %v", older)
	}
	// the identity submission once there is one, else what the account gave at sign-up (owner, 2026-10-30)
	if older["full_name"] != "Amina Otieno" || older["national_id"] != "12345678" || newest["full_name"] != "Amina Otieno" || newest["national_id"] != "24681357" {
		t.Fatalf("names and ids follow the identity submission, else sign-up: %v / %v", older, newest)
	}
	// a fragment of the number, as a player would read it out, and the local form of it
	for _, q := range []string{a.msisdn[len(a.msisdn)-6:], "0" + a.msisdn[3:]} {
		r := e.do(t, http.MethodGet, "/v1/staff/players?q="+q, op, nil)
		if ps := r.Body["players"].([]any); r.Code != http.StatusOK || len(ps) != 1 || ps[0].(map[string]any)["user_id"] != older["user_id"] {
			t.Fatalf("search %s: %d %s", q, r.Code, r.Raw)
		}
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/players?status=suspended", op, nil); len(r.Body["players"].([]any)) != 0 {
		t.Fatalf("suspended before any suspension: %s", r.Raw)
	}
	for _, bad := range []string{"status=banned", "before=nope", "limit=0"} {
		if r := e.do(t, http.MethodGet, "/v1/staff/players?"+bad, op, nil); r.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", bad, r.Code, r.Raw)
		}
	}
	user := older["user_id"].(string)
	if r := e.do(t, http.MethodPost, "/v1/staff/players/"+user+"/suspend", op, map[string]any{"reason": "  "}); r.Code != http.StatusBadRequest || !hasType(r, "reason-required") {
		t.Fatalf("no reason: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/players/"+user+"/suspend", op, map[string]any{"reason": "chargeback dispute"}); r.Code != http.StatusOK || r.Body["status"] != "suspended" {
		t.Fatalf("suspend: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/account", a.access, nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("the suspended player's session still works: %d", r.Code)
	}
	if r := e.do(t, http.MethodPost, "/v1/auth/login", "", map[string]any{"msisdn": a.msisdn, "password": strongPassword, "device": map[string]any{"fingerprint": a.fp, "platform": "android"}}); r.Code != http.StatusForbidden {
		t.Fatalf("suspended sign-in: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/players/"+user+"/suspend", op, map[string]any{"reason": "again"}); r.Code != http.StatusConflict {
		t.Fatalf("suspend twice: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/players?status=suspended", op, nil); len(r.Body["players"].([]any)) != 1 {
		t.Fatalf("suspended list: %s", r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/players/"+user+"/reinstate", op, map[string]any{"reason": "dispute resolved"}); r.Code != http.StatusOK || r.Body["status"] != "active" {
		t.Fatalf("reinstate: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/players/"+user+"/reinstate", op, map[string]any{"reason": "again"}); r.Code != http.StatusConflict {
		t.Fatalf("reinstate twice: %d %s", r.Code, r.Raw)
	}
	var audited int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit.audit_log WHERE action IN ('identity.players_viewed','identity.account_suspended','identity.account_reinstated')`).Scan(&audited); err != nil || audited < 6 {
		t.Fatalf("audited %d (%v)", audited, err)
	}
}

// The back office sees every play and top-up by player name and time, the system's state, and (administrators) the
// audit trail (owner, 2026-10-30).
func TestStaffAPI_PlaysTopUpsSystemAndActivity(t *testing.T) {
	e := setup(t)
	s := e.signIn(t)
	if r := e.do(t, http.MethodPost, "/v1/kyc", s.access, map[string]any{"national_id": "24681357", "full_name": "Amina Otieno", "date_of_birth": "1992-03-04"}); r.Code != http.StatusAccepted {
		t.Fatalf("kyc: %d %s", r.Code, r.Raw)
	}
	e.canPlay(t, s, 1000)
	for i := range 2 {
		if r := e.doKey(t, http.MethodPost, "/v1/plays", s.access, fmt.Sprint("p-", i), nil); r.Code != http.StatusCreated {
			t.Fatalf("play: %d %s", r.Code, r.Raw)
		}
	}
	if r := e.do(t, http.MethodPost, "/v1/payments/topups", s.access, map[string]any{"amount_kes": 50}); r.Code != http.StatusAccepted {
		t.Fatalf("top-up: %d %s", r.Code, r.Raw)
	}
	tok := e.member(staff.Staff)

	r := e.do(t, http.MethodGet, "/v1/staff/plays?limit=1", tok, nil)
	plays, _ := r.Body["plays"].([]any)
	if r.Code != http.StatusOK || len(plays) != 1 || r.Body["next_before"] == nil {
		t.Fatalf("plays page 1: %d %s", r.Code, r.Raw)
	}
	first := plays[0].(map[string]any)
	if first["full_name"] != "Amina Otieno" || first["msisdn"] != s.msisdn || first["seq"] != float64(2) || first["slot"] == nil ||
		first["created_at"] == nil || first["price_kes"] == nil {
		t.Fatalf("a play: %v", first)
	}
	next := e.do(t, http.MethodGet, "/v1/staff/plays?limit=1&before="+url.QueryEscape(r.Body["next_before"].(string)), tok, nil)
	if p := next.Body["plays"].([]any); next.Code != http.StatusOK || len(p) != 1 || p[0].(map[string]any)["seq"] != float64(1) {
		t.Fatalf("plays page 2: %d %s", next.Code, next.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/plays?q=0799999999", tok, nil); len(r.Body["plays"].([]any)) != 0 {
		t.Fatalf("another number: %s", r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/plays?before=nonsense", tok, nil); r.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor: %d", r.Code)
	}

	r = e.do(t, http.MethodGet, "/v1/staff/topups?q="+s.msisdn[3:], tok, nil)
	topups, _ := r.Body["topups"].([]any)
	if r.Code != http.StatusOK || len(topups) < 1 {
		t.Fatalf("top-ups: %d %s", r.Code, r.Raw)
	}
	latest := topups[0].(map[string]any)
	if latest["full_name"] != "Amina Otieno" || latest["amount_requested_kes"] != float64(50) || latest["status"] != "initiated" || latest["sandbox"] != false {
		t.Fatalf("a top-up: %v", latest)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/topups?status=bogus", tok, nil); r.Code != http.StatusBadRequest {
		t.Fatalf("bad status: %d", r.Code)
	}

	r = e.do(t, http.MethodGet, "/v1/staff/system", tok, nil)
	settings, _ := r.Body["settings"].(map[string]any)
	if r.Code != http.StatusOK || settings["daraja"] != "sandbox" || settings["sandbox_settlement"] != true ||
		r.Body["plays_today"] == nil || len(r.Body["reconciliations"].([]any)) != 4 || settings["play_price_kes"] == nil {
		t.Fatalf("system: %d %s", r.Code, r.Raw)
	}

	admin := e.member(staff.Admin)
	r = e.do(t, http.MethodGet, "/v1/staff/activity?q=payment.topup", admin, nil)
	events, _ := r.Body["events"].([]any)
	if r.Code != http.StatusOK || len(events) == 0 || events[0].(map[string]any)["action"] != "payment.topup.started" {
		t.Fatalf("activity: %d %s", r.Code, r.Raw)
	}
	if r := e.do(t, http.MethodGet, "/v1/staff/activity?actor_type=robot", admin, nil); r.Code != http.StatusBadRequest {
		t.Fatalf("bad actor: %d", r.Code)
	}
}
