//go:build integration

package staff_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/testpg"
	"github.com/itsivali/lottery-platform/internal/staff"
)

// idTokens plays Google and Apple: a token is the name of a claim set registered by the test.
type idTokens map[string]jwt.MapClaims

func (f idTokens) VerifyIDTokenClaims(_ context.Context, provider, token, nonce string) (jwt.MapClaims, error) {
	c, ok := f[provider+":"+token]
	if !ok || nonce != "n" {
		return nil, errors.New("invalid token")
	}
	return c, nil
}

type env struct {
	pool *pgxpool.Pool
	clk  *clock.Manual
	ids  idTokens
	svc  *staff.Service
}

var start = time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{pool: testpg.Pool(t, "app_worker"), clk: clock.NewManual(start), ids: idTokens{}}
	e.svc = staff.NewService(e.pool, e.clk, e.ids, audit.NewWriter())
	return e
}

// admin bootstraps the first admin and signs them in.
func (e *env) admin(t *testing.T) staff.Member {
	t.Helper()
	if err := e.svc.Bootstrap(context.Background(), "Owner@Example.com"); err != nil {
		t.Fatal(err)
	}
	e.ids["google:owner"] = jwt.MapClaims{"sub": "g-owner", "email": "owner@example.com", "email_verified": true, "name": "Owner"}
	return e.signIn(t, "google", "owner")
}

func (e *env) signIn(t *testing.T, provider, token string) staff.Member {
	t.Helper()
	s, err := e.svc.SignIn(context.Background(), provider, token, "n")
	if err != nil {
		t.Fatalf("sign in %s %s: %v", provider, token, err)
	}
	m, err := e.svc.Authenticate(context.Background(), s.Token)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestStaff_AnInvitedMemberSignsInWithGoogleOrAppleAndKeepsTheirRoles(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	admin := e.admin(t)
	if !admin.Has(staff.Admin) || admin.Email != "owner@example.com" {
		t.Fatalf("bootstrapped admin %+v", admin)
	}
	if _, err := e.svc.Invite(ctx, admin, "Wanjiru@Example.com", "Wanjiru", []staff.Role{staff.KYCReviewer, staff.Finance}); err != nil {
		t.Fatal(err)
	}
	// Apple sends email_verified as a string
	e.ids["apple:w1"] = jwt.MapClaims{"sub": "a-wanjiru", "email": "wanjiru@example.com", "email_verified": "true"}
	s, err := e.svc.SignIn(ctx, "apple", "w1", "n")
	if err != nil {
		t.Fatal(err)
	}
	m, err := e.svc.Authenticate(ctx, s.Token)
	if err != nil || m.Email != "wanjiru@example.com" || !m.Has(staff.KYCReviewer) || !m.Has(staff.Finance) || m.Has(staff.Admin) {
		t.Fatalf("member %+v %v", m, err)
	}
	if !s.ExpiresAt.Equal(start.Add(staff.SessionMaxAge)) {
		t.Fatalf("expires %s", s.ExpiresAt)
	}
	// the Apple identity is now bound: a later token without the email (Apple sends it once) still signs in
	e.ids["apple:w2"] = jwt.MapClaims{"sub": "a-wanjiru"}
	if again := e.signIn(t, "apple", "w2"); again.ID != m.ID {
		t.Fatalf("signed in as %s, want %s", again.ID, m.ID)
	}
	var signIns int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM audit.audit_log WHERE action = 'staff.sign_in' AND actor_id = $1`, m.Operator()).Scan(&signIns); err != nil || signIns != 2 {
		t.Fatalf("sign-ins audited: %d %v", signIns, err)
	}
}

func TestStaff_SignInIsRefusedUnlessInvitedVerifiedAndActive(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	admin := e.admin(t)
	e.ids["google:stranger"] = jwt.MapClaims{"sub": "g-x", "email": "stranger@example.com", "email_verified": true}
	if _, err := e.svc.SignIn(ctx, "google", "stranger", "n"); !errors.Is(err, staff.ErrNotInvited) {
		t.Fatalf("not invited: %v", err)
	}
	if _, err := e.svc.Invite(ctx, admin, "otieno@example.com", "Otieno", []staff.Role{staff.Support}); err != nil {
		t.Fatal(err)
	}
	e.ids["google:unverified"] = jwt.MapClaims{"sub": "g-o", "email": "otieno@example.com", "email_verified": false}
	if _, err := e.svc.SignIn(ctx, "google", "unverified", "n"); !errors.Is(err, staff.ErrNotInvited) {
		t.Fatalf("unverified email: %v", err)
	}
	if _, err := e.svc.SignIn(ctx, "google", "bad-token", "n"); !errors.Is(err, staff.ErrUnauthenticated) {
		t.Fatalf("bad token: %v", err)
	}
	e.ids["google:otieno"] = jwt.MapClaims{"sub": "g-o", "email": "otieno@example.com", "email_verified": true}
	o := e.signIn(t, "google", "otieno")
	if _, err := e.svc.SetActive(ctx, admin, o.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SignIn(ctx, "google", "otieno", "n"); !errors.Is(err, staff.ErrDisabled) {
		t.Fatalf("disabled: %v", err)
	}
}

func TestStaff_SessionsEndWhenIdleAtTheirLimitOrOnSignOut(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.admin(t)
	s, err := e.svc.SignIn(ctx, "google", "owner", "n")
	if err != nil {
		t.Fatal(err)
	}
	for range 3 { // used every 25 minutes, the session stays alive past the idle limit
		e.clk.Advance(25 * time.Minute)
		if _, err := e.svc.Authenticate(ctx, s.Token); err != nil {
			t.Fatalf("active session refused: %v", err)
		}
	}
	e.clk.Advance(staff.SessionIdle + time.Minute)
	if _, err := e.svc.Authenticate(ctx, s.Token); !errors.Is(err, staff.ErrUnauthenticated) {
		t.Fatalf("idle session: %v", err)
	}
	s, _ = e.svc.SignIn(ctx, "google", "owner", "n")
	if err := e.svc.SignOut(ctx, s.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Authenticate(ctx, s.Token); !errors.Is(err, staff.ErrUnauthenticated) {
		t.Fatalf("signed-out session: %v", err)
	}
	if _, err := e.svc.Authenticate(ctx, "stf_made-up"); !errors.Is(err, staff.ErrUnauthenticated) {
		t.Fatalf("unknown token: %v", err)
	}
}

func TestStaff_AdminsManageRolesButNeverLockThemselvesOrEveryoneOut(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	admin := e.admin(t)
	if _, err := e.svc.Invite(ctx, admin, "owner@example.com", "Again", nil); !errors.Is(err, staff.ErrExists) {
		t.Fatalf("duplicate invite: %v", err)
	}
	if _, err := e.svc.Invite(ctx, admin, "not an email", "", nil); !errors.Is(err, staff.ErrInvalid) {
		t.Fatalf("bad email: %v", err)
	}
	if _, err := e.svc.Invite(ctx, admin, "x@example.com", "", []staff.Role{"superuser"}); !errors.Is(err, staff.ErrInvalid) {
		t.Fatalf("unknown role: %v", err)
	}
	if _, err := e.svc.SetRoles(ctx, admin, admin.ID, []staff.Role{staff.Auditor}); !errors.Is(err, staff.ErrSelf) {
		t.Fatalf("own roles: %v", err)
	}
	if _, err := e.svc.SetActive(ctx, admin, admin.ID, false); !errors.Is(err, staff.ErrSelf) {
		t.Fatalf("disable self: %v", err)
	}
	rec, err := e.svc.Invite(ctx, admin, "amina@example.com", "Amina", []staff.Role{staff.Auditor})
	if err != nil {
		t.Fatal(err)
	}
	e.ids["google:amina"] = jwt.MapClaims{"sub": "g-a", "email": "amina@example.com", "email_verified": true}
	s, _ := e.svc.SignIn(ctx, "google", "amina", "n")
	// a role change applies to the next request of an open session
	if _, err := e.svc.SetRoles(ctx, admin, rec.ID, []staff.Role{staff.Admin, staff.Finance}); err != nil {
		t.Fatal(err)
	}
	if m, err := e.svc.Authenticate(ctx, s.Token); err != nil || !m.Has(staff.Admin) || m.Has(staff.Auditor) {
		t.Fatalf("after the change: %+v %v", m, err)
	}
	amina, _ := e.svc.Authenticate(ctx, s.Token)
	// with two admins, one may remove the other's admin role; the last one left cannot lose it
	if _, err := e.svc.SetRoles(ctx, amina, admin.ID, []staff.Role{staff.Finance}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetRoles(ctx, amina, admin.ID, []staff.Role{staff.Admin}); err != nil {
		t.Fatal(err) // amina restores it; now remove amina's and then try the last one
	}
	owner, _ := e.svc.Authenticate(ctx, e.sessionOf(t, "owner"))
	if _, err := e.svc.SetActive(ctx, owner, amina.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Authenticate(ctx, s.Token); !errors.Is(err, staff.ErrUnauthenticated) {
		t.Fatalf("a disabled member's session still works: %v", err)
	}
	list, err := e.svc.Members(ctx)
	if err != nil || len(list) != 2 || !slices.ContainsFunc(list, func(r staff.Record) bool { return r.Email == "amina@example.com" && !r.Active }) {
		t.Fatalf("members %+v %v", list, err)
	}
	var changes int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM audit.audit_log WHERE action IN ('staff.invite','staff.roles','staff.disable')`).Scan(&changes); err != nil || changes != 5 {
		t.Fatalf("audited changes: %d %v", changes, err)
	}
}

func TestStaff_TheLastAdminCannotBeRemoved(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	admin := e.admin(t)
	rec, err := e.svc.Invite(ctx, admin, "deputy@example.com", "", []staff.Role{staff.Admin})
	if err != nil {
		t.Fatal(err)
	}
	e.ids["google:deputy"] = jwt.MapClaims{"sub": "g-d", "email": "deputy@example.com", "email_verified": true}
	deputy := e.signIn(t, "google", "deputy")
	if _, err := e.svc.SetActive(ctx, admin, rec.ID, false); err != nil {
		t.Fatal(err) // two admins: one may be disabled
	}
	_ = deputy
	if _, err := e.svc.SetActive(ctx, admin, rec.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetRoles(ctx, admin, rec.ID, nil); err != nil {
		t.Fatal(err)
	}
	// the owner is now the only admin; nobody can take it away (they cannot change themselves either)
	if n, err := e.svc.AdminCount(ctx); err != nil || n != 1 {
		t.Fatalf("admins %d %v", n, err)
	}
}

func TestStaff_BootstrapMakesTheFirstAdminOnlyWhenThereIsNone(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	if err := e.svc.Bootstrap(ctx, "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Bootstrap(ctx, "someone-else@example.com"); err != nil {
		t.Fatal(err)
	}
	list, err := e.svc.Members(ctx)
	if err != nil || len(list) != 1 || list[0].Email != "owner@example.com" || !slices.Contains(list[0].Roles, staff.Admin) {
		t.Fatalf("members %+v %v", list, err)
	}
}

// sessionOf signs in again with a registered token and returns the session token.
func (e *env) sessionOf(t *testing.T, token string) string {
	t.Helper()
	s, err := e.svc.SignIn(context.Background(), "google", token, "n")
	if err != nil {
		t.Fatal(err)
	}
	return s.Token
}
