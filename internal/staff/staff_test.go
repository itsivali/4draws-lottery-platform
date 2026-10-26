package staff_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/itsivali/lottery-platform/internal/staff"
)

type fakeVerifier struct {
	claims jwt.MapClaims
	err    error
}

func (f fakeVerifier) VerifyAccessToken(context.Context, string, string) (jwt.MapClaims, error) {
	return f.claims, f.err
}

func TestAuthenticate_MapsKeycloakClaimsToAStaffMemberWithKnownRolesOnly(t *testing.T) {
	a := staff.NewAuthenticator(fakeVerifier{claims: jwt.MapClaims{
		"sub": "0192f000-0000-7000-8000-0000000000aa", "preferred_username": "wanjiru", "email": "w@example.co.ke",
		"realm_access": map[string]any{"roles": []any{"kyc_reviewer", "draw_official", "offline_access", "default-roles-lottery-staff"}},
	}}, "keycloak")
	s, err := a.Authenticate(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "0192f000-0000-7000-8000-0000000000aa" || s.Name != "wanjiru" || s.Email != "w@example.co.ke" {
		t.Fatalf("staff: %+v", s)
	}
	if !s.Has(staff.KYCReviewer) || !s.Has(staff.DrawOfficial) || s.Has(staff.PayoutOperator) || len(s.Roles) != 2 {
		t.Fatalf("roles: %v", s.Roles)
	}
	if s.Operator() != "staff:0192f000-0000-7000-8000-0000000000aa" {
		t.Fatalf("operator: %s", s.Operator())
	}
}

func TestAuthenticate_RefusesInvalidTokensAndStaffWithoutAnyBackOfficeRole(t *testing.T) {
	if _, err := staff.NewAuthenticator(fakeVerifier{err: errors.New("bad signature")}, "keycloak").Authenticate(context.Background(), "t"); !errors.Is(err, staff.ErrUnauthenticated) {
		t.Fatalf("invalid token: %v", err)
	}
	noRoles := staff.NewAuthenticator(fakeVerifier{claims: jwt.MapClaims{"sub": "0192f000-0000-7000-8000-0000000000ab", "realm_access": map[string]any{"roles": []any{"offline_access"}}}}, "keycloak")
	if _, err := noRoles.Authenticate(context.Background(), "t"); !errors.Is(err, staff.ErrNoRole) {
		t.Fatalf("no role: %v", err)
	}
	notUUID := staff.NewAuthenticator(fakeVerifier{claims: jwt.MapClaims{"sub": "player-1", "realm_access": map[string]any{"roles": []any{"auditor"}}}}, "keycloak")
	if _, err := notUUID.Authenticate(context.Background(), "t"); !errors.Is(err, staff.ErrUnauthenticated) {
		t.Fatalf("non-uuid subject: %v", err)
	}
}

func TestUnavailable_RefusesEveryone(t *testing.T) {
	if _, err := (staff.Unavailable{}).Authenticate(context.Background(), "t"); !errors.Is(err, staff.ErrUnavailable) {
		t.Fatal(err)
	}
}
