//go:build integration

package api_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
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
	{http.MethodGet, "/v1/staff/kyc/pending", staff.KYCReviewer},
	{http.MethodPost, "/v1/staff/kyc/0192f000-0000-7000-8000-000000000001/approve", staff.KYCReviewer},
	{http.MethodPost, "/v1/staff/kyc/0192f000-0000-7000-8000-000000000001/reject", staff.KYCReviewer},
	{http.MethodPost, "/v1/staff/payments/reconcile", staff.PaymentsOperator},
	{http.MethodPost, "/v1/staff/payments/reversals", staff.PaymentsOperator},
	{http.MethodPost, "/v1/staff/payments/0192f000-0000-7000-8000-000000000001/resolve", staff.PaymentsOperator},
	{http.MethodPost, "/v1/staff/suspense/RCPT1/credit", staff.PaymentsOperator},
	{http.MethodPost, "/v1/staff/suspense/RCPT1/refund", staff.PaymentsOperator},
	{http.MethodPost, "/v1/staff/paybill/RCPT1/recheck", staff.PaymentsOperator},
	{http.MethodGet, "/v1/staff/draws", staff.DrawOfficial},
	{http.MethodGet, "/v1/staff/draws/overdue", staff.DrawOfficial},
	{http.MethodPost, "/v1/staff/draws/reconcile", staff.DrawOfficial},
	{http.MethodPost, "/v1/staff/draws/0192f000-0000-7000-8000-000000000001/sign", staff.DrawOfficial},
	{http.MethodPost, "/v1/staff/draws/0192f000-0000-7000-8000-000000000001/approve", staff.DrawOfficial},
	{http.MethodPost, "/v1/staff/draws/0192f000-0000-7000-8000-000000000001/void", staff.DrawOfficial},
	{http.MethodPost, "/v1/staff/game/reconcile", staff.Auditor},
	{http.MethodGet, "/v1/staff/payouts", staff.PayoutOperator},
	{http.MethodPost, "/v1/staff/payouts/reconcile", staff.PayoutOperator},
	{http.MethodPost, "/v1/staff/payouts/0192f000-0000-7000-8000-000000000001/approve", staff.PayoutOperator},
	{http.MethodPost, "/v1/staff/payouts/0192f000-0000-7000-8000-000000000001/reject", staff.PayoutOperator},
	{http.MethodPost, "/v1/staff/payouts/0192f000-0000-7000-8000-000000000001/settle", staff.PayoutOperator},
	{http.MethodPost, "/v1/staff/prizes/0192f000-0000-7000-8000-000000000001/retry", staff.PayoutOperator},
	{http.MethodGet, "/v1/staff/tax/batches", staff.TaxOfficer},
	{http.MethodPost, "/v1/staff/tax/batches", staff.TaxOfficer},
	{http.MethodGet, "/v1/staff/tax/batches/1/file", staff.TaxOfficer},
	{http.MethodPost, "/v1/staff/tax/batches/1/remit", staff.TaxOfficer},
	{http.MethodGet, "/v1/staff/alerts", staff.AlertsOperator},
	{http.MethodPost, "/v1/staff/alerts/0192f000-0000-7000-8000-000000000001/acknowledge", staff.AlertsOperator},
	{http.MethodGet, "/v1/staff/closures", staff.Support},
	{http.MethodPost, "/v1/staff/closures/0192f000-0000-7000-8000-000000000001/close", staff.Support},
}

func TestStaffAPI_EveryRouteNeedsAStaffTokenWithTheRightRole(t *testing.T) {
	e := setup(t)
	player := e.signIn(t)
	noRole := e.member()
	wrong := e.member(staff.Support)
	for _, rt := range staffRoutes {
		for name, tok := range map[string]string{"no token": "", "player token": player.access, "garbage": "x"} {
			if r := e.do(t, rt.method, rt.path, tok, `{}`); r.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with %s = %d, want 401", rt.method, rt.path, name, r.Code)
			}
		}
		if r := e.do(t, rt.method, rt.path, noRole, `{}`); r.Code != http.StatusForbidden {
			t.Errorf("%s %s without a role = %d, want 403", rt.method, rt.path, r.Code)
		}
		if rt.role != staff.Support {
			if r := e.do(t, rt.method, rt.path, wrong, `{}`); r.Code != http.StatusForbidden || !hasType(r, "forbidden") {
				t.Errorf("%s %s with the wrong role = %d, want 403", rt.method, rt.path, r.Code)
			}
		}
		if r := e.do(t, rt.method, rt.path, e.member(rt.role), `{}`); r.Code == http.StatusUnauthorized || r.Code == http.StatusForbidden {
			t.Errorf("%s %s refused its own role: %d %s", rt.method, rt.path, r.Code, r.Raw)
		}
		if rt.method != http.MethodGet {
			if r := e.doKey(t, rt.method, rt.path, e.member(rt.role), "", `{}`); r.Code != http.StatusBadRequest {
				t.Errorf("%s %s without an idempotency key = %d, want 400", rt.method, rt.path, r.Code)
			}
		}
	}
	// staff tokens never work on player routes
	if r := e.do(t, http.MethodGet, "/v1/account", e.member(staff.Auditor), nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("staff token on a player route: %d", r.Code)
	}
}

func TestStaffAPI_MeShowsRoles(t *testing.T) {
	e := setup(t)
	r := e.do(t, http.MethodGet, "/v1/staff/me", e.member(staff.KYCReviewer, staff.Auditor), nil)
	if r.Code != http.StatusOK || len(r.Body["roles"].([]any)) != 2 {
		t.Fatalf("me: %d %s", r.Code, r.Raw)
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
	reviewer := e.member(staff.KYCReviewer)
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
	alice, bob := e.member(staff.DrawOfficial), e.member(staff.DrawOfficial)
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
	support := e.member(staff.Support)
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
	op := e.member(staff.PayoutOperator)
	if r := e.do(t, http.MethodPost, "/v1/staff/payouts/"+payout+"/settle", op, map[string]any{"paid": true, "receipt": "SGH0000001"}); r.Code != http.StatusOK || r.Body["status"] != "succeeded" && r.Body["status"] != "paid" {
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
	e.freezeDaily(t) // today's draws (daily frozen; weekly, monthly, quarterly still open) hold this player's chances
	// pay out the rest of the wallet so only the draw stake remains
	w := e.do(t, http.MethodPost, "/v1/payouts/withdrawals", s.access, map[string]any{"amount_kes": 850})
	if w.Code != http.StatusCreated {
		t.Fatalf("withdraw: %d %s", w.Code, w.Raw)
	}
	for _, st := range []string{"sending", "sent", "manual"} {
		e.ownerExec(t, `UPDATE payouts.payouts SET status = $2 WHERE id = $1`, w.Body["id"], st)
	}
	if r := e.do(t, http.MethodPost, "/v1/staff/payouts/"+w.Body["id"].(string)+"/settle", e.member(staff.PayoutOperator), map[string]any{"paid": true, "receipt": "SGH0000002"}); r.Code != http.StatusOK {
		t.Fatalf("settle: %d %s", r.Code, r.Raw)
	}
	e.do(t, http.MethodPost, "/v1/account/closure", s.access, nil)
	r := e.do(t, http.MethodPost, "/v1/staff/closures/"+user+"/close", e.member(staff.Support), map[string]any{})
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
	r := e.do(t, http.MethodPost, "/v1/staff/closures/"+user+"/close", e.member(staff.Support), map[string]any{})
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
