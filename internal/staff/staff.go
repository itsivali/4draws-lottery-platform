// Package staff identifies back-office staff and names the two roles that gate the back office (ADR 0019). Staff sign
// in with their email address and password, or with Google or Apple when those are configured (ADR 0018); who they are
// and what they may do is kept in the staff schema and managed by admins in the back office. Staff never use player
// accounts: they are a separate identity with their own sessions.
package staff

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/mail"
	"github.com/itsivali/lottery-platform/internal/platform/password"
	"github.com/itsivali/lottery-platform/internal/staff/store"
)

// Role is a back-office permission, granted to a member by an admin.
type Role string

// The roles (ADR 0019). An admin may do everything a staff member may; the two-person rules in draws and tax compare
// people, not roles, so they still need two different members.
const (
	Admin Role = "admin" // the whole back office: staff management and the app metrics, on top of everything staff do
	Staff Role = "staff" // every operational queue (identity review, payments, draws, payouts, tax, alerts, closures) and the finance dashboard
)

// AllRoles lists every role, in the order the back office shows them.
var AllRoles = []Role{Admin, Staff}

// Password rules for staff, on top of the common-password list in package password: longer than players' (the back
// office moves money), never the member's own email address.
const (
	MinPasswordRunes = 12
	MaxPasswordRunes = 128
)

// MaxNameRunes bounds a member's name, which the back office shows in place of their email address.
const MaxNameRunes = 80

// validName reports whether name (already trimmed) can be a member's name.
func validName(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > MaxNameRunes {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

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
	ErrInvalidName     = errors.New("staff: a name is 1 to 80 characters, without control characters")
	ErrNotFound        = errors.New("staff: no such member")
	ErrSelf            = errors.New("staff: members cannot change their own roles or disable themselves")
	ErrLastAdmin       = errors.New("staff: the last admin cannot be removed")
	ErrWrongPassword   = errors.New("staff: the current password is wrong")
	ErrNoPassword      = errors.New("staff: this member has no password; an admin sets a temporary one")
	ErrMustChange      = errors.New("staff: the temporary password must be changed first")
	ErrInvalidToken    = errors.New("staff: this reset link has expired or was already used")
)

// ResetLinkTTL is how long an emailed password-reset link works.
const ResetLinkTTL = 30 * time.Minute

// ErrWeakPassword wraps every password rule refusal (password.ErrWeak for the common rules).
var ErrWeakPassword = password.ErrWeak

// Member is a signed-in staff member, as they are now.
type Member struct {
	ID    string // the member's uuid
	Name  string
	Email string
	Roles []Role
	// MustChangePassword: this session was opened with a temporary password and may do nothing else until the member
	// changes it (a session opened with Google or Apple is not held back by it).
	MustChangePassword bool
	// HasPassword: the member can sign in with a password; one who signs in with Google or Apple only can set one
	// within FreshSignIn of signing in (ADR 0035).
	HasPassword bool
	// PhotoURL is the photo of the Google account the member signs in with, or "" (ADR 0035).
	PhotoURL string
	// SignedInAt is when this session's sign-in happened.
	SignedInAt time.Time
}

// FreshSignIn is how recent the sign-in must be for a member without a password to set one (ADR 0035).
const FreshSignIn = 10 * time.Minute

// ErrSignInAgain: setting a first password needs a sign-in within FreshSignIn.
var ErrSignInAgain = errors.New("staff: sign in again to do this")

// Has reports whether the member may act as role: admins may act as anyone.
func (m Member) Has(r Role) bool {
	return slices.Contains(m.Roles, r) || (r == Staff && slices.Contains(m.Roles, Admin))
}

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
	pool      *pgxpool.Pool
	clk       clock.Clock
	ids       IDTokenVerifier // nil: Google and Apple sign-in are off; passwords still work
	audit     *audit.Writer
	pepper    []byte          // password pepper (derived from the deployment's AUTH_KEY)
	dummyHash string          // compared when the email is unknown, so a miss costs the same as a wrong password
	resets    *PasswordResets // nil: forgotten passwords are reset by an admin only
}

// NewService returns a Service. pepper is the password pepper (internal/platform/password).
func NewService(pool *pgxpool.Pool, clk clock.Clock, ids IDTokenVerifier, a *audit.Writer, pepper []byte) *Service {
	s := &Service{pool: pool, clk: clk, ids: ids, audit: a, pepper: pepper}
	s.dummyHash, _ = password.Hash("never matches: no such member", pepper)
	return s
}

// PasswordResets is how a member who forgot their password gets back in by themselves: an emailed link that opens
// BackOfficeURL/reset-password and works for ResetLinkTTL.
type PasswordResets struct {
	Sender        mail.Sender
	BackOfficeURL string
}

// WithPasswordResets switches on RequestPasswordReset.
func (s *Service) WithPasswordResets(p PasswordResets) *Service {
	s.resets = &p
	return s
}

// RequestPasswordReset emails email a reset link if it belongs to an active member. It answers the same, and takes
// about the same time, whether or not it does, so nobody learns who the staff are. Without WithPasswordResets it
// returns ErrUnavailable.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	if s.resets == nil {
		return ErrUnavailable
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !emailRE.MatchString(email) {
		return nil
	}
	m, err := store.New(s.pool).MemberCredentials(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = password.Hash("no such member: same cost", s.pepper)
		return nil
	}
	if err != nil {
		return err
	}
	if !m.Active {
		return nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	var expires time.Time
	err = s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		expires = now.Add(ResetLinkTTL)
		if err := q.InsertPasswordReset(ctx, store.InsertPasswordResetParams{MemberID: m.ID, TokenHash: hash[:], ExpiresAt: expires, At: now}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "staff.password_reset_requested",
			EntityType: "staff_member", EntityID: m.ID})
	})
	if err != nil {
		return err
	}
	link := strings.TrimRight(s.resets.BackOfficeURL, "/") + "/reset-password?token=" + url.QueryEscape(token)
	return s.resets.Sender.Send(ctx, resetNotice(link, expires).Message([]string{email}, "4draws back office: reset your password"))
}

func resetNotice(link string, expires time.Time) mail.Notice {
	nairobi := time.FixedZone("EAT", 3*60*60)
	return mail.Notice{
		Staff:     true,
		Preheader: "Choose a new password for the 4draws back office.",
		Heading:   "Reset your back-office password",
		Intro: []string{
			"Someone asked to reset the password of the 4draws back-office account with this email address. If it was you, choose a new password now.",
		},
		Action:     &mail.Action{Label: "Choose a new password", URL: link},
		ActionNote: "The link works until " + expires.In(nairobi).Format("15:04 MST") + " (30 minutes) and can be used once.",
		Outro:      []string{"When the password changes, every back-office session of the account ends."},
		Safety:     "Ignore this email and tell an administrator. Nothing changes until the link is used, and it is sent only to this address.",
	}
}

// ResetPasswordWithToken sets the password of the member an emailed link belongs to, ends their sessions and spends
// the link (and any other open link of theirs). Expired, used or unknown tokens are ErrInvalidToken.
func (s *Service) ResetPasswordWithToken(ctx context.Context, token, next string) error {
	if token == "" {
		return ErrInvalidToken
	}
	hash := sha256.Sum256([]byte(token))
	return s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		r, err := q.PasswordResetByHash(ctx, hash[:])
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidToken
		} else if err != nil {
			return err
		}
		if r.UsedAt != nil || !now.Before(r.ExpiresAt) || !r.Active {
			return ErrInvalidToken
		}
		if err := CheckPassword(next, r.Email); err != nil {
			return err
		}
		h, err := password.Hash(next, s.pepper)
		if err != nil {
			return err
		}
		if err := q.SetPassword(ctx, store.SetPasswordParams{PasswordHash: &h, At: &now, MustChange: false, ID: r.MemberID}); err != nil {
			return err
		}
		if err := q.UseMemberPasswordResets(ctx, store.UseMemberPasswordResetsParams{At: &now, MemberID: r.MemberID}); err != nil {
			return err
		}
		if err := q.RevokeMemberSessions(ctx, store.RevokeMemberSessionsParams{At: &now, MemberID: r.MemberID}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "staff.password_reset_by_email",
			EntityType: "staff_member", EntityID: r.MemberID})
	})
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
	if s.ids == nil {
		return Session{}, ErrUnavailable
	}
	claims, err := s.ids.VerifyIDTokenClaims(ctx, provider, idToken, nonce)
	if err != nil {
		return Session{}, errors.Join(ErrUnauthenticated, err)
	}
	sub, _ := claims.GetSubject()
	if sub == "" {
		return Session{}, ErrUnauthenticated
	}
	token, err := newToken()
	if err != nil {
		return Session{}, err
	}
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
		if provider == "google" { // the account's photo, refreshed at each Google sign-in (ADR 0035)
			var photo *string
			if p := identity.GooglePhoto(provider, claims); p != "" {
				photo = &p
			}
			if err := q.SetMemberPhoto(ctx, store.SetMemberPhotoParams{PhotoUrl: photo, ID: m.ID}); err != nil {
				return err
			}
		}
		out, err = s.openSession(ctx, tx, q, now, token, provider, member)
		return err
	})
	return out, err
}

// SignInWithPassword opens a session for the active member with this email address and password. A wrong password, an
// unknown address and a member without a password are refused alike (ErrUnauthenticated), after the same work.
func (s *Service) SignInWithPassword(ctx context.Context, email, pw string) (Session, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	token, err := newToken()
	if err != nil {
		return Session{}, err
	}
	var out Session
	err = s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		m, err := q.MemberCredentials(ctx, email)
		known := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		hash := s.dummyHash
		if known && m.PasswordHash != nil {
			hash = *m.PasswordHash
		}
		ok, err := password.Verify(pw, hash, s.pepper)
		if err != nil {
			return err
		}
		if !ok || !known || m.PasswordHash == nil {
			return ErrUnauthenticated
		}
		if !m.Active {
			return ErrDisabled
		}
		member := Member{ID: m.ID, Name: m.Name, Email: m.Email, Roles: roles(m.Roles), MustChangePassword: m.MustChangePassword}
		if len(member.Roles) == 0 {
			return ErrNoRole
		}
		out, err = s.openSession(ctx, tx, q, now, token, "password", member)
		return err
	})
	return out, err
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// openSession stores the session's hash and audits the sign-in.
func (s *Service) openSession(ctx context.Context, tx pgx.Tx, q *store.Queries, now time.Time, token, provider string, member Member) (Session, error) {
	h := sha256.Sum256([]byte(token))
	if err := q.InsertSession(ctx, store.InsertSessionParams{TokenHash: h[:], MemberID: member.ID, Provider: provider, At: now,
		ExpiresAt: now.Add(SessionMaxAge)}); err != nil {
		return Session{}, err
	}
	err := s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: member.Operator(), Action: "staff.sign_in",
		EntityType: "staff_member", EntityID: member.ID, Details: map[string]any{"provider": provider}})
	return Session{Token: token, ExpiresAt: now.Add(SessionMaxAge), Member: member}, err
}

// CheckPassword applies the staff password rules: MinPasswordRunes to MaxPasswordRunes characters, nothing from the
// common-password list, not the member's email address or its local part. Every refusal wraps ErrWeakPassword.
func CheckPassword(pw, email string) error {
	if n := utf8.RuneCountInString(pw); n < MinPasswordRunes {
		return fmt.Errorf("%w: use at least %d characters", ErrWeakPassword, MinPasswordRunes)
	} else if n > MaxPasswordRunes {
		return fmt.Errorf("%w: use at most %d characters", ErrWeakPassword, MaxPasswordRunes)
	}
	if err := password.CheckPolicy(pw, ""); err != nil {
		return err
	}
	lower := strings.ToLower(pw)
	email = strings.ToLower(strings.TrimSpace(email))
	local, _, _ := strings.Cut(email, "@")
	if email != "" && (lower == email || (len(local) >= 4 && strings.Contains(lower, local))) {
		return fmt.Errorf("%w: do not use your email address", ErrWeakPassword)
	}
	return nil
}

// ChangePassword sets the member's own password: the current one must match (unless they have none yet) and the new
// one must pass CheckPassword. Every other session of the member ends; the one making the change (token) goes on.
func (s *Service) ChangePassword(ctx context.Context, m Member, token, current, next string) error {
	if err := CheckPassword(next, m.Email); err != nil {
		return err
	}
	hash, err := password.Hash(next, s.pepper)
	if err != nil {
		return err
	}
	keep := sha256.Sum256([]byte(token))
	return s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		row, err := q.LockMember(ctx, m.ID)
		if err != nil {
			return err
		}
		if row.PasswordHash == nil && now.Sub(m.SignedInAt) > FreshSignIn {
			// a first password is a new way in: only right after signing in with Google or Apple (ADR 0035)
			return ErrSignInAgain
		}
		if row.PasswordHash != nil {
			ok, err := password.Verify(current, *row.PasswordHash, s.pepper)
			if err != nil {
				return err
			}
			if !ok {
				return ErrWrongPassword
			}
			if subtle.ConstantTimeCompare([]byte(current), []byte(next)) == 1 {
				return fmt.Errorf("%w: choose a password you have not used here", ErrWeakPassword)
			}
		}
		if err := q.SetPassword(ctx, store.SetPasswordParams{PasswordHash: &hash, At: &now, MustChange: false, ID: m.ID}); err != nil {
			return err
		}
		if err := q.RevokeOtherSessions(ctx, store.RevokeOtherSessionsParams{At: &now, MemberID: m.ID, KeepTokenHash: keep[:]}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: m.Operator(), Action: "staff.password_changed",
			EntityType: "staff_member", EntityID: m.ID})
	})
}

// EmergencyPassword is lotteryctl's way back in when every administrator is locked out (ADR 0019): it gives the
// member with this email address a temporary password (changed at their next sign-in), re-enables them and ends
// their sessions; with makeAdmin it also makes them an administrator, creating the member when none exists. It runs
// with database access only, so it is audited as the system, not as a member.
func (s *Service) EmergencyPassword(ctx context.Context, email, temporary string, makeAdmin bool) (Record, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !emailRE.MatchString(email) {
		return Record{}, ErrInvalid
	}
	if err := CheckPassword(temporary, email); err != nil {
		return Record{}, err
	}
	hash, err := password.Hash(temporary, s.pepper)
	if err != nil {
		return Record{}, err
	}
	var id string
	err = s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		if _, err := tx.Exec(ctx, `LOCK TABLE staff.members IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		m, err := q.MemberByEmail(ctx, email)
		switch {
		case errors.Is(err, pgx.ErrNoRows) && makeAdmin:
			if id, err = q.InsertMember(ctx, store.InsertMemberParams{Email: email, Roles: []string{string(Admin)}, InvitedBy: "lotteryctl", At: now}); err != nil {
				return err
			}
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNotFound
		case err != nil:
			return err
		default:
			id = m.ID
			rs := m.Roles
			if makeAdmin {
				rs = appendRole(rs, Admin)
			}
			if err := q.UpdateMember(ctx, store.UpdateMemberParams{Roles: rs, Active: true, At: now, ID: id}); err != nil {
				return err
			}
		}
		if err := q.SetPassword(ctx, store.SetPasswordParams{PasswordHash: &hash, At: &now, MustChange: true, ID: id}); err != nil {
			return err
		}
		if err := q.RevokeMemberSessions(ctx, store.RevokeMemberSessionsParams{At: &now, MemberID: id}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, ActorID: "lotteryctl", Action: "staff.emergency_password",
			EntityType: "staff_member", EntityID: id, Details: map[string]any{"email": email, "admin": makeAdmin}})
	})
	if err != nil {
		return Record{}, err
	}
	return s.record(ctx, id)
}

// ResetPassword is an admin giving a member a temporary password (told to them out of band): the member must change
// it at their next sign-in, and every session they have ends now. Admins reset their own through ChangePassword.
func (s *Service) ResetPassword(ctx context.Context, by Member, id, temporary string) error {
	if id == by.ID {
		return ErrSelf
	}
	err := s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		m, err := q.LockMember(ctx, id)
		var pe *pgconn.PgError
		if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02") {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if err := CheckPassword(temporary, m.Email); err != nil {
			return err
		}
		hash, err := password.Hash(temporary, s.pepper)
		if err != nil {
			return err
		}
		if err := q.SetPassword(ctx, store.SetPasswordParams{PasswordHash: &hash, At: &now, MustChange: true, ID: id}); err != nil {
			return err
		}
		if err := q.RevokeMemberSessions(ctx, store.RevokeMemberSessionsParams{At: &now, MemberID: id}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: by.Operator(), Action: "staff.password_reset",
			EntityType: "staff_member", EntityID: id})
	})
	return err
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
	// the temporary password holds back the sessions it opened; a Google or Apple sign-in is not using it
	m := Member{ID: r.ID, Name: r.Name, Email: r.Email, Roles: roles(r.Roles), MustChangePassword: r.MustChangePassword && r.Provider == "password",
		HasPassword: r.HasPassword, PhotoURL: deref(r.PhotoUrl), SignedInAt: r.SignedInAt}
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
	// HasPassword: the member can sign in with a password; MustChangePassword: it is a temporary one.
	HasPassword        bool `json:"has_password"`
	MustChangePassword bool `json:"must_change_password"`
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
			Providers: r.Providers, CreatedAt: r.CreatedAt, HasPassword: r.HasPassword, MustChangePassword: r.MustChangePassword}
		if t, ok := at[r.ID]; ok {
			rec.LastSignIn = &t
		}
		out = append(out, rec)
	}
	return out, nil
}

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Invite adds a member. With temporary (a password the admin tells them out of band) they sign in with their email
// address and must change it first; without, they sign in with Google or Apple using email, or wait for a reset.
func (s *Service) Invite(ctx context.Context, by Member, email, name string, rs []Role, temporary string) (Record, error) {
	email, name = strings.ToLower(strings.TrimSpace(email)), strings.TrimSpace(name)
	if !emailRE.MatchString(email) || !known(rs) || len(rs) == 0 {
		return Record{}, ErrInvalid
	}
	if name != "" && !validName(name) {
		return Record{}, ErrInvalidName
	}
	var hash *string
	if temporary != "" {
		if err := CheckPassword(temporary, email); err != nil {
			return Record{}, err
		}
		h, err := password.Hash(temporary, s.pepper)
		if err != nil {
			return Record{}, err
		}
		hash = &h
	}
	var id string
	err := s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		var err error
		id, err = q.InsertMember(ctx, store.InsertMemberParams{Email: email, Name: name, Roles: names(rs),
			InvitedBy: by.Operator(), At: now, PasswordHash: hash})
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

// SetName sets the name the back office shows for member id in place of their email address: members name themselves,
// and an admin may name anyone (the API decides who may call it for whom).
func (s *Service) SetName(ctx context.Context, by Member, id, name string) (Record, error) {
	name = strings.TrimSpace(name)
	if !validName(name) {
		return Record{}, ErrInvalidName
	}
	err := s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		n, err := q.SetName(ctx, store.SetNameParams{Name: name, At: now, ID: id})
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "22P02" {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: by.Operator(), Action: "staff.name",
			EntityType: "staff_member", EntityID: id, Details: map[string]any{"name": name}})
	})
	if err != nil {
		return Record{}, err
	}
	return s.record(ctx, id)
}

// SetRoles replaces a member's roles. Nobody changes their own, and the last active admin keeps theirs.
func (s *Service) SetRoles(ctx context.Context, by Member, id string, rs []Role) (Record, error) {
	if !known(rs) || len(rs) == 0 {
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
// admin can sign in and invite everyone else; with an admin in place that part does nothing. temporary, when given
// (STAFF_BOOTSTRAP_PASSWORD), becomes that member's temporary password while they have none, so the first admin can
// sign in without Google or Apple; once they have set a password it is never touched again.
func (s *Service) Bootstrap(ctx context.Context, email, temporary string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if !emailRE.MatchString(email) {
		return ErrInvalid
	}
	if temporary != "" {
		if err := CheckPassword(temporary, email); err != nil {
			return err
		}
	}
	return s.tx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		if _, err := tx.Exec(ctx, `LOCK TABLE staff.members IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		n, err := q.ActiveAdmins(ctx)
		if err != nil {
			return err
		}
		m, err := q.MemberCredentials(ctx, email)
		missing := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !missing {
			return err
		}
		if n > 0 && (missing || m.PasswordHash != nil || temporary == "") {
			return nil // an admin exists and this member needs no password: nothing to do
		}
		var id string
		switch {
		case missing:
			if id, err = q.InsertMember(ctx, store.InsertMemberParams{Email: email, Roles: []string{string(Admin)}, InvitedBy: "bootstrap", At: now}); err != nil {
				return err
			}
		case n == 0:
			id = m.ID
			if err := q.UpdateMember(ctx, store.UpdateMemberParams{Roles: appendRole(m.Roles, Admin), Active: true, At: now, ID: id}); err != nil {
				return err
			}
		default:
			id = m.ID
		}
		details := map[string]any{"email": email}
		if temporary != "" && (missing || m.PasswordHash == nil) {
			hash, err := password.Hash(temporary, s.pepper)
			if err != nil {
				return err
			}
			if err := q.SetPassword(ctx, store.SetPasswordParams{PasswordHash: &hash, At: &now, MustChange: true, ID: id}); err != nil {
				return err
			}
			details["temporary_password"] = "set"
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "staff.bootstrap", EntityType: "staff_member",
			EntityID: id, Details: details})
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

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
