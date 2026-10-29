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

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/identity/store"
	"github.com/itsivali/lottery-platform/internal/platform/mail"
	"github.com/itsivali/lottery-platform/internal/platform/password"
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
// reset page (WebAppURL/#/reset-password) and works for ResetLinkTTL.
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
	link := strings.TrimRight(s.resets.WebAppURL, "/") + "/#/reset-password?token=" + url.QueryEscape(token)
	return s.resets.Sender.Send(ctx, mail.Message{To: []string{email}, Subject: "Tajirika: reset your password", Text: resetText(link, expires)})
}

func resetText(link string, expires time.Time) string {
	nairobi := time.FixedZone("EAT", 3*60*60)
	return "Someone asked to reset the password of the Tajirika account with this email address.\n\n" +
		"If it was you, choose a new password here (the link works until " + expires.In(nairobi).Format("15:04 MST") + "):\n" + link + "\n\n" +
		"If it was not you, ignore this email: nothing changes until the link is used, and it is sent only to this address.\n"
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
	return s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		if err := q.SetUserEmail(ctx, store.SetUserEmailParams{Email: email, At: now, ID: userID}); err != nil {
			return emailError(err)
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: userID, Action: "identity.email_changed", EntityType: "user", EntityID: userID})
	})
}

// emailError turns the unique index on addresses into ErrEmailRegistered.
func emailError(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "users_email_key" {
		return ErrEmailRegistered
	}
	return err
}
