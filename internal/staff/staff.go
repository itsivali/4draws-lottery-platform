// Package staff identifies back-office staff and names the roles that gate each back-office action. Staff sign in with
// Google or Apple (ADR 0018); who they are and what they may do is kept in the staff schema and managed by admins in
// the back office. Staff never use player accounts: they are a separate identity with their own sessions.
package staff

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/staff/store"
)

// Role is a back-office permission, granted to a member by an admin.
type Role string

// The roles. Each action needs one; nobody needs them all (the two-person rules in draws and tax still apply).
const (
	Admin            Role = "admin"             // invites staff, assigns roles, disables members
	Finance          Role = "finance"           // reads the financial dashboard
	KYCReviewer      Role = "kyc_reviewer"      // reads submitted identity details, approves or rejects them
	PaymentsOperator Role = "payments_operator" // reconciliation, reversals, unconfirmed top-ups, suspense
	DrawOfficial     Role = "draw_official"     // signs, approves and voids draws
	PayoutOperator   Role = "payout_operator"   // approves, rejects and settles payouts; retries prizes
	TaxOfficer       Role = "tax_officer"       // prepares and records KRA remittance batches
	AlertsOperator   Role = "alerts_operator"   // acknowledges alerts (lifts a payout freeze)
	Support          Role = "support"           // closes accounts on request
	Auditor          Role = "auditor"           // runs reconciliations and reads, changes nothing
)

// AllRoles lists every role, in the order the back office shows them.
var AllRoles = []Role{Admin, Finance, KYCReviewer, PaymentsOperator, DrawOfficial, PayoutOperator, TaxOfficer, AlertsOperator, Support, Auditor}

// Session limits: a session ends 12 hours after sign-in, or after 30 minutes without a request.
const (
	SessionMaxAge = 12 * time.Hour
	SessionIdle   = 30 * time.Minute
	touchEvery    = time.Minute
	tokenPrefix   = "stf_"
)

// Errors.
var (
	ErrUnauthenticated = errors.New("staff: invalid or missing token")
	ErrNoRole          = errors.New("staff: no back-office role")
	ErrUnavailable     = errors.New("staff: sign-in is not configured")
	ErrNotInvited      = errors.New("staff: no active member is invited with this verified email address")
	ErrDisabled        = errors.New("staff: this member is disabled")
	ErrExists          = errors.New("staff: a member with this email address exists")
	ErrInvalid         = errors.New("staff: invalid email address or role")
	ErrNotFound        = errors.New("staff: no such member")
	ErrSelf            = errors.New("staff: members cannot change their own roles or disable themselves")
	ErrLastAdmin       = errors.New("staff: the last admin cannot be removed")
)

// Member is a signed-in staff member, as they are now.
type Member struct {
	ID    string // the member's uuid
	Name  string
	Email string
	Roles []Role
}

// Has reports whether the member holds role.
func (m Member) Has(r Role) bool { return slices.Contains(m.Roles, r) }

// Operator is how the member appears in audit records and in the services' operator fields. Two different members
// always give two different operators, which is what the two-person rules compare.
func (m Member) Operator() string { return "staff:" + m.ID }

// Authenticator turns staff session tokens into members.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (Member, error)
}

// Unavailable refuses everyone: the staff routes are closed until sign-in is configured.
type Unavailable struct{}

// Authenticate implements Authenticator.
func (Unavailable) Authenticate(context.Context, string) (Member, error) {
	return Member{}, ErrUnavailable
}

// IDTokenVerifier checks a Google or Apple ID token (signature, issuer, audience, expiry, nonce) and returns its
// claims (identity.OIDCVerifier).
type IDTokenVerifier interface {
	VerifyIDTokenClaims(ctx context.Context, provider, idToken, nonce string) (jwt.MapClaims, error)
}

// Service signs staff in and manages members.
type Service struct {
	pool  *pgxpool.Pool
	clk   clock.Clock
	ids   IDTokenVerifier
	audit *audit.Writer
}

// NewService returns a Service.
func NewService(pool *pgxpool.Pool, clk clock.Clock, ids IDTokenVerifier, a *audit.Writer) *Service {
	return &Service{pool: pool, clk: clk, ids: ids, audit: a}
}

// Session is a signed-in back-office session. Token is shown once; only its hash is stored.
type Session struct {
	Token     string
	ExpiresAt time.Time
	Member    Member
}

// SignIn verifies a Google or Apple ID token and opens a session for the member it belongs to: the member bound to
// that provider identity, or else the active member invited with the token's verified email address, to whom the
// identity is then bound.
func (s *Service) SignIn(ctx context.Context, provider, idToken, nonce string) (Session, error) {
	claims, err := s.ids.VerifyIDTokenClaims(ctx, provider, idToken, nonce)
	if err != nil {
		return Session{}, errors.Join(ErrUnauthenticated, err)
	}
	sub, _ := claims.GetSubject()
	if sub == "" {
		return Session{}, ErrUnauthenticated
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, err
	}
	token := tokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	var out Session
	err = s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		row, err := q.MemberByIdentity(ctx, store.MemberByIdentityParams{Provider: provider, Subject: sub})
		m := store.MemberByEmailRow(row)
		if errors.Is(err, pgx.ErrNoRows) {
			email, verified := verifiedEmail(claims)
			if !verified {
				return ErrNotInvited
			}
			if m, err = q.MemberByEmail(ctx, email); errors.Is(err, pgx.ErrNoRows) {
				return ErrNotInvited
			} else if err != nil {
				return err
			}
			if !m.Active {
				return ErrDisabled
			}
			if err := q.BindIdentity(ctx, store.BindIdentityParams{Provider: provider, Subject: sub, MemberID: m.ID}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if !m.Active {
			return ErrDisabled
		}
		member := Member{ID: m.ID, Name: m.Name, Email: m.Email, Roles: roles(m.Roles)}
		if len(member.Roles) == 0 {
			return ErrNoRole
		}
		h := sha256.Sum256([]byte(token))
		if err := q.InsertSession(ctx, store.InsertSessionParams{TokenHash: h[:], MemberID: m.ID, Provider: provider, At: now,
			ExpiresAt: now.Add(SessionMaxAge)}); err != nil {
			return err
		}
		out = Session{Token: token, ExpiresAt: now.Add(SessionMaxAge), Member: member}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: member.Operator(), Action: "staff.sign_in",
			EntityType: "staff_member", EntityID: m.ID, Details: map[string]any{"provider": provider}})
	})
	return out, err
}

// verifiedEmail returns the token's email address when the provider vouches for it (Apple sends "true" as a string).
func verifiedEmail(c jwt.MapClaims) (string, bool) {
	email, _ := c["email"].(string)
	v := c["email_verified"]
	ok := v == true || v == "true"
	return strings.ToLower(strings.TrimSpace(email)), ok && email != ""
}

// Authenticate implements Authenticator: a live session's member with their current roles. Disabled members and
// members without a role are refused.
func (s *Service) Authenticate(ctx context.Context, token string) (Member, error) {
	if !strings.HasPrefix(token, tokenPrefix) {
		return Member{}, ErrUnauthenticated
	}
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return Member{}, err
	}
	h := sha256.Sum256([]byte(token))
	q := store.New(s.pool)
	r, err := q.SessionMember(ctx, store.SessionMemberParams{TokenHash: h[:], Now: now, IdleAfter: now.Add(-SessionIdle)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrUnauthenticated
	} else if err != nil {
		return Member{}, err
	}
	if !r.Active {
		return Member{}, ErrUnauthenticated
	}
	m := Member{ID: r.ID, Name: r.Name, Email: r.Email, Roles: roles(r.Roles)}
	if len(m.Roles) == 0 {
		return Member{}, ErrNoRole
	}
	if now.Sub(r.LastSeenAt) >= touchEvery {
		if err := q.TouchSession(ctx, store.TouchSessionParams{At: now, ID: r.SessionID}); err != nil {
			return Member{}, err
		}
	}
	return m, nil
}

// SignOut ends the session of token.
func (s *Service) SignOut(ctx context.Context, token string) error {
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return err
	}
	h := sha256.Sum256([]byte(token))
	return store.New(s.pool).RevokeSession(ctx, store.RevokeSessionParams{At: &now, TokenHash: h[:]})
}

// Record is a member as admins see them.
type Record struct {
	ID         string     `json:"id"`
	Email      string     `json:"email"`
	Name       string     `json:"name"`
	Roles      []Role     `json:"roles"`
	Active     bool       `json:"active"`
	InvitedBy  string     `json:"invited_by"`
	Providers  []string   `json:"providers"`
	LastSignIn *time.Time `json:"last_sign_in"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Members lists every member, active ones first.
func (s *Service) Members(ctx context.Context) ([]Record, error) {
	q := store.New(s.pool)
	rows, err := q.ListMembers(ctx)
	if err != nil {
		return nil, err
	}
	last, err := q.LastSignIns(ctx)
	if err != nil {
		return nil, err
	}
	at := map[string]time.Time{}
	for _, l := range last {
		at[l.MemberID] = l.LastSignIn
	}
	out := make([]Record, 0, len(rows))
	for _, r := range rows {
		rec := Record{ID: r.ID, Email: r.Email, Name: r.Name, Roles: roles(r.Roles), Active: r.Active, InvitedBy: r.InvitedBy,
			Providers: r.Providers, CreatedAt: r.CreatedAt}
		if t, ok := at[r.ID]; ok {
			rec.LastSignIn = &t
		}
		out = append(out, rec)
	}
	return out, nil
}

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Invite adds a member who may sign in with Google or Apple using email.
func (s *Service) Invite(ctx context.Context, by Member, email, name string, rs []Role) (Record, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !emailRE.MatchString(email) || !known(rs) {
		return Record{}, ErrInvalid
	}
	var id string
	err := s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		var err error
		id, err = q.InsertMember(ctx, store.InsertMemberParams{Email: email, Name: strings.TrimSpace(name), Roles: names(rs),
			InvitedBy: by.Operator(), At: now})
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return ErrExists
		} else if err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: by.Operator(), Action: "staff.invite",
			EntityType: "staff_member", EntityID: id, Details: map[string]any{"email": email, "roles": names(rs)}})
	})
	if err != nil {
		return Record{}, err
	}
	return s.record(ctx, id)
}

// SetRoles replaces a member's roles. Nobody changes their own, and the last active admin keeps theirs.
func (s *Service) SetRoles(ctx context.Context, by Member, id string, rs []Role) (Record, error) {
	if !known(rs) {
		return Record{}, ErrInvalid
	}
	return s.change(ctx, by, id, "staff.roles", func(m *store.LockMemberRow) {
		m.Roles = names(rs)
	})
}

// SetActive disables or re-enables a member; disabling ends their sessions at once.
func (s *Service) SetActive(ctx context.Context, by Member, id string, active bool) (Record, error) {
	action := "staff.enable"
	if !active {
		action = "staff.disable"
	}
	return s.change(ctx, by, id, action, func(m *store.LockMemberRow) { m.Active = active })
}

func (s *Service) change(ctx context.Context, by Member, id, action string, f func(*store.LockMemberRow)) (Record, error) {
	if id == by.ID {
		return Record{}, ErrSelf
	}
	err := s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		// admins are locked in a fixed order so two concurrent removals cannot leave nobody
		if _, err := tx.Exec(ctx, `SELECT 1 FROM staff.members WHERE 'admin' = ANY (roles) ORDER BY id FOR UPDATE`); err != nil {
			return err
		}
		m, err := q.LockMember(ctx, id)
		var pe *pgconn.PgError
		if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02") {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		f(&m)
		if err := q.UpdateMember(ctx, store.UpdateMemberParams{Roles: m.Roles, Active: m.Active, At: now, ID: id}); err != nil {
			return err
		}
		if n, err := q.ActiveAdmins(ctx); err != nil {
			return err
		} else if n == 0 {
			return ErrLastAdmin
		}
		if !m.Active {
			if err := q.RevokeMemberSessions(ctx, store.RevokeMemberSessionsParams{At: &now, MemberID: id}); err != nil {
				return err
			}
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: by.Operator(), Action: action,
			EntityType: "staff_member", EntityID: id, Details: map[string]any{"roles": m.Roles, "active": m.Active}})
	})
	if err != nil {
		return Record{}, err
	}
	return s.record(ctx, id)
}

// AdminCount is how many active members hold the admin role.
func (s *Service) AdminCount(ctx context.Context) (int64, error) {
	return store.New(s.pool).ActiveAdmins(ctx)
}

// Bootstrap makes email an admin when there is no active admin (STAFF_BOOTSTRAP_ADMIN, or lotteryctl), so the first
// admin can sign in and invite everyone else. With an admin in place it does nothing.
func (s *Service) Bootstrap(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if !emailRE.MatchString(email) {
		return ErrInvalid
	}
	return s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		if _, err := tx.Exec(ctx, `LOCK TABLE staff.members IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		if n, err := q.ActiveAdmins(ctx); err != nil || n > 0 {
			return err
		}
		m, err := q.MemberByEmail(ctx, email)
		var id string
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if id, err = q.InsertMember(ctx, store.InsertMemberParams{Email: email, Roles: []string{string(Admin)}, InvitedBy: "bootstrap", At: now}); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			id = m.ID
			if err := q.UpdateMember(ctx, store.UpdateMemberParams{Roles: appendRole(m.Roles, Admin), Active: true, At: now, ID: id}); err != nil {
				return err
			}
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "staff.bootstrap", EntityType: "staff_member",
			EntityID: id, Details: map[string]any{"email": email}})
	})
}

func (s *Service) record(ctx context.Context, id string) (Record, error) {
	list, err := s.Members(ctx)
	if err != nil {
		return Record{}, err
	}
	for _, r := range list {
		if r.ID == id {
			return r, nil
		}
	}
	return Record{}, ErrNotFound
}

func (s *Service) tx(ctx context.Context, f func(pgx.Tx, *store.Queries, time.Time) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		return f(tx, store.New(tx), now)
	})
}

// ParseRoles reads role names; unknown names are an error.
func ParseRoles(in []string) ([]Role, error) {
	out := make([]Role, 0, len(in))
	for _, r := range in {
		if !slices.Contains(AllRoles, Role(r)) {
			return nil, ErrInvalid
		}
		if !slices.Contains(out, Role(r)) {
			out = append(out, Role(r))
		}
	}
	return out, nil
}

func roles(in []string) []Role {
	out := []Role{}
	for _, r := range in {
		if slices.Contains(AllRoles, Role(r)) && !slices.Contains(out, Role(r)) {
			out = append(out, Role(r))
		}
	}
	return out
}

func known(rs []Role) bool {
	for _, r := range rs {
		if !slices.Contains(AllRoles, r) {
			return false
		}
	}
	return true
}

func names(rs []Role) []string {
	out := []string{}
	for _, r := range rs {
		if !slices.Contains(out, string(r)) {
			out = append(out, string(r))
		}
	}
	return out
}

func appendRole(in []string, r Role) []string {
	if slices.Contains(in, string(r)) {
		return in
	}
	return append(in, string(r))
}
