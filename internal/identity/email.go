package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/identity/store"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/mail"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/password"
)

// Email addresses (ADR 0021): given at sign-up, unique among accounts, and the way a player who forgot their password
// gets back in by themselves. Accounts from before the address was asked for have none until they add one.
var (
	ErrInvalidEmail     = errors.New("identity: invalid email address")
	ErrEmailRegistered  = errors.New("identity: another account uses this email address")
	ErrResetUnavailable = errors.New("identity: password reset by email is not configured")
	ErrResetLinkInvalid = errors.New("identity: the reset link is unknown, used or expired")
)

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// NormalizeEmail trims and lower-cases an address and refuses what is not one.
func NormalizeEmail(raw string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(raw))
	if e == "" || len(e) > 254 || !emailRE.MatchString(e) {
		return "", ErrInvalidEmail
	}
	return e, nil
}

// ResetLinkTTL is how long an emailed password-reset link works.
const ResetLinkTTL = 30 * time.Minute

// PasswordResets is how a player who forgot their password gets back in: an emailed link that opens the web app's
// reset page (WebAppURL/reset-password) and works for ResetLinkTTL.
type PasswordResets struct {
	Sender    mail.Sender
	WebAppURL string
}

// WithPasswordResets switches on RequestPasswordReset.
func (s *Service) WithPasswordResets(p PasswordResets) *Service {
	s.resets = &p
	return s
}

// RequestPasswordReset emails email a reset link if it belongs to an active account. It answers the same, and takes
// about the same time, whether or not it does, so nobody learns which addresses have accounts.
func (s *Service) RequestPasswordReset(ctx context.Context, raw string) error {
	if s.resets == nil {
		return ErrResetUnavailable
	}
	email, err := NormalizeEmail(raw)
	if err != nil {
		return nil
	}
	u, err := store.New(s.pool).UserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = password.Hash("no such account: same cost", s.pepper)
		return nil
	}
	if err != nil {
		return err
	}
	if u.Status != "active" {
		return nil
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	hash := sha256.Sum256([]byte(token))
	var expires time.Time
	err = s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		expires = now.Add(ResetLinkTTL)
		if err := q.InsertPlayerPasswordReset(ctx, store.InsertPlayerPasswordResetParams{UserID: u.ID, TokenHash: hash[:], ExpiresAt: expires, At: now}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "auth.password_reset_requested", EntityType: "user", EntityID: u.ID})
	})
	if err != nil {
		return err
	}
	link := strings.TrimRight(s.resets.WebAppURL, "/") + "/reset-password?token=" + url.QueryEscape(token)
	return s.resets.Sender.Send(ctx, resetNotice(link, s.resets.WebAppURL, expires).Message([]string{email}, "4draws: reset your password"))
}

func resetNotice(link, site string, expires time.Time) mail.Notice {
	nairobi := time.FixedZone("EAT", 3*60*60)
	return mail.Notice{
		Preheader: "Choose a new password for your 4draws account.",
		Heading:   "Reset your password",
		Intro: []string{
			"Someone asked to reset the password of the 4draws account with this email address. If it was you, choose a new password now.",
		},
		Action:     &mail.Action{Label: "Choose a new password", URL: link},
		ActionNote: "The link works until " + expires.In(nairobi).Format("15:04 MST") + " (30 minutes) and can be used once.",
		Outro: []string{
			"When the password changes, every device signed in to the account is signed out.",
		},
		Safety:  "Ignore this email. Nothing changes until the link is used, and it is sent only to this address.",
		Reason:  "You are receiving this email because a password reset was asked for with this address.",
		SiteURL: site,
	}
}

// ResetPasswordWithToken sets the password of the account an emailed link belongs to, ends its sessions and spends
// the link (and any other open link of the account). Expired, used or unknown tokens are ErrResetLinkInvalid.
func (s *Service) ResetPasswordWithToken(ctx context.Context, token, next string) error {
	if token == "" {
		return ErrResetLinkInvalid
	}
	hash := sha256.Sum256([]byte(token))
	return s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		r, err := q.PlayerPasswordResetByHash(ctx, hash[:])
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrResetLinkInvalid
		} else if err != nil {
			return err
		}
		if r.UsedAt != nil || !now.Before(r.ExpiresAt) || r.Status != "active" {
			return ErrResetLinkInvalid
		}
		if err := password.CheckPolicy(next, r.Msisdn); err != nil {
			return err
		}
		h, err := password.Hash(next, s.pepper)
		if err != nil {
			return err
		}
		if err := q.SetPassword(ctx, store.SetPasswordParams{ID: r.UserID, PasswordHash: &h, PasswordChangedAt: &now}); err != nil {
			return err
		}
		if err := q.UsePlayerPasswordResets(ctx, store.UsePlayerPasswordResetsParams{At: &now, UserID: r.UserID}); err != nil {
			return err
		}
		if err := q.RevokeSessionsForPasswordChange(ctx, store.RevokeSessionsForPasswordChangeParams{At: &now, UserID: r.UserID}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "auth.password_reset_by_email", EntityType: "user", EntityID: r.UserID})
	})
}

// SetEmail changes (or, for an account from before addresses were asked for, adds) the player's email address. An
// account with a password must give it, since the address is a way to reset that password.
func (s *Service) SetEmail(ctx context.Context, userID, raw, currentPassword string) error {
	email, err := NormalizeEmail(raw)
	if err != nil {
		return err
	}
	u, err := store.New(s.pool).GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if u.PasswordHash != nil {
		if len(currentPassword) > maxPasswordBytes {
			return ErrInvalidCredentials
		}
		if ok, verr := password.Verify(currentPassword, *u.PasswordHash, s.pepper); verr != nil || !ok {
			return ErrInvalidCredentials
		}
	}
	err = s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		if err := emailFree(ctx, q, email, &userID); err != nil {
			return err
		}
		if err := q.SetUserEmail(ctx, store.SetUserEmailParams{Email: email, At: now, ID: userID}); err != nil {
			return emailError(err)
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: userID, Action: "identity.email_changed", EntityType: "user", EntityID: userID})
	})
	if err != nil {
		return err
	}
	s.sendVerificationAfter(ctx, userID, "address change") // a new address is not confirmed until its own link is opened
	return nil
}

// emailFree refuses an address another account has confirmed (ADR 0041). An address others have only claimed is
// free to claim too: whoever confirms it keeps it.
func emailFree(ctx context.Context, q *store.Queries, email string, except *string) error {
	taken, err := q.EmailConfirmedByAnother(ctx, store.EmailConfirmedByAnotherParams{Email: email, ExceptID: except})
	if err != nil {
		return err
	}
	if taken {
		return ErrEmailRegistered
	}
	return nil
}

// releaseClaims takes a now-confirmed address from every other account that only claimed it, and records each.
func (s *Service) releaseClaims(ctx context.Context, tx pgx.Tx, q *store.Queries, ownerID, email string, now time.Time) error {
	lost, err := q.ReleaseEmailClaims(ctx, store.ReleaseEmailClaimsParams{At: now, Email: email, OwnerID: ownerID})
	if err != nil {
		return err
	}
	for _, id := range lost {
		if err := s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "identity.email_claim_released", EntityType: "user", EntityID: id,
			Details: map[string]any{"reason": "another account confirmed the address"}}); err != nil {
			return err
		}
	}
	return nil
}

// emailError turns the unique index on confirmed addresses into ErrEmailRegistered.
func emailError(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "users_email_confirmed_key" {
		return ErrEmailRegistered
	}
	return err
}
