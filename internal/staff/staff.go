// Package staff identifies back-office staff from their Keycloak access tokens and names the roles that gate each
// back-office action. Staff never use player accounts: they are a separate identity, issued by the staff realm.
package staff

import (
	"context"
	"errors"
	"regexp"
	"slices"

	"github.com/golang-jwt/jwt/v5"
)

// Role is a back-office permission, granted as a realm role in Keycloak.
type Role string

// The roles. Each action needs one; nobody needs them all (the two-person rules in draws and tax still apply).
const (
	KYCReviewer      Role = "kyc_reviewer"      // reads submitted identity details, approves or rejects them
	PaymentsOperator Role = "payments_operator" // reconciliation, reversals, unconfirmed top-ups, suspense
	DrawOfficial     Role = "draw_official"     // signs, approves and voids draws
	PayoutOperator   Role = "payout_operator"   // approves, rejects and settles payouts; retries prizes
	TaxOfficer       Role = "tax_officer"       // prepares and records KRA remittance batches
	AlertsOperator   Role = "alerts_operator"   // acknowledges alerts (lifts a payout freeze)
	Support          Role = "support"           // closes accounts on request
	Auditor          Role = "auditor"           // runs reconciliations and reads, changes nothing
)

var allRoles = []Role{KYCReviewer, PaymentsOperator, DrawOfficial, PayoutOperator, TaxOfficer, AlertsOperator, Support, Auditor}

// Errors.
var (
	ErrUnauthenticated = errors.New("staff: invalid or missing token")
	ErrNoRole          = errors.New("staff: no back-office role")
	ErrUnavailable     = errors.New("staff: sign-in is not configured")
)

// Member is a signed-in staff member.
type Member struct {
	ID    string // the Keycloak subject (a uuid)
	Name  string
	Email string
	Roles []Role
}

// Has reports whether the member holds role.
func (m Member) Has(r Role) bool { return slices.Contains(m.Roles, r) }

// Operator is how the member appears in audit records and in the services' operator fields. Two different members
// always give two different operators, which is what the two-person rules compare.
func (m Member) Operator() string { return "staff:" + m.ID }

// AccessTokenVerifier checks an access token's signature, issuer, audience and expiry (identity.OIDCVerifier).
type AccessTokenVerifier interface {
	VerifyAccessToken(ctx context.Context, provider, token string) (jwt.MapClaims, error)
}

// Authenticator turns staff access tokens into members.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (Member, error)
}

type tokenAuthenticator struct {
	v        AccessTokenVerifier
	provider string
}

// NewAuthenticator authenticates tokens of the named provider (the staff realm).
func NewAuthenticator(v AccessTokenVerifier, provider string) Authenticator {
	return tokenAuthenticator{v: v, provider: provider}
}

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (a tokenAuthenticator) Authenticate(ctx context.Context, token string) (Member, error) {
	claims, err := a.v.VerifyAccessToken(ctx, a.provider, token)
	if err != nil {
		return Member{}, errors.Join(ErrUnauthenticated, err)
	}
	sub, _ := claims.GetSubject()
	if !uuidRE.MatchString(sub) {
		return Member{}, ErrUnauthenticated
	}
	m := Member{ID: sub}
	m.Name, _ = claims["preferred_username"].(string)
	m.Email, _ = claims["email"].(string)
	if ra, ok := claims["realm_access"].(map[string]any); ok {
		if list, ok := ra["roles"].([]any); ok {
			for _, r := range list {
				if s, ok := r.(string); ok && slices.Contains(allRoles, Role(s)) && !m.Has(Role(s)) {
					m.Roles = append(m.Roles, Role(s))
				}
			}
		}
	}
	if len(m.Roles) == 0 {
		return Member{}, ErrNoRole
	}
	return m, nil
}

// Unavailable refuses everyone: the staff routes are closed until sign-in is configured.
type Unavailable struct{}

// Authenticate implements Authenticator.
func (Unavailable) Authenticate(context.Context, string) (Member, error) {
	return Member{}, ErrUnavailable
}
