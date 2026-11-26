package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/identity/store"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/mail"
)

// Email verification (ADR 0040): an address is a claim until the player opens the link sent to it. Until then it
// still receives password-reset links (so nobody is locked out), but Google sign-in does not find the account by it
// and a copy of the player's data is not sent to it.
var (
	ErrVerificationUnavailable = errors.New("identity: email verification is not configured")
	ErrNoEmail                 = errors.New("identity: the account has no email address")
	ErrEmailAlreadyVerified    = errors.New("identity: the email address is already confirmed")
	ErrVerifyLinkInvalid       = errors.New("identity: the confirmation link is unknown, used, expired or for another address")
	// ErrSSOEmailUnverified means a Google identity's address belongs to an account that has not confirmed it, so
	// Google may not sign that account in yet.
	ErrSSOEmailUnverified = errors.New("identity: an account has this email address but has not confirmed it")
)

// VerifyLinkTTL is how long an emailed confirmation link works.
const VerifyLinkTTL = 24 * time.Hour

// SendEmailVerification emails the player a link that confirms the address on their account. It is sent at sign-up
// and when the address changes, and again whenever the player asks.
func (s *Service) SendEmailVerification(ctx context.Context, userID string) error {
	if s.resets == nil {
		return ErrVerificationUnavailable
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	hash := sha256.Sum256([]byte(token))
	var email string
	var expires time.Time
	err := s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		u, err := q.UserEmail(ctx, userID)
		if err != nil {
			return err
		}
		switch {
		case u.Status == "closed" || u.Email == nil:
			return ErrNoEmail
		case u.EmailVerified:
			return ErrEmailAlreadyVerified
		}
		email, expires = *u.Email, now.Add(VerifyLinkTTL)
		if err := q.InsertEmailVerification(ctx, store.InsertEmailVerificationParams{UserID: userID, Email: email, TokenHash: hash[:], ExpiresAt: expires, At: now}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "identity.email_verification_sent", EntityType: "user", EntityID: userID})
	})
	if err != nil {
		return err
	}
	link := strings.TrimRight(s.resets.WebAppURL, "/") + "/verify-email?token=" + url.QueryEscape(token)
	return s.resets.Sender.Send(ctx, verifyNotice(link, s.resets.WebAppURL).Message([]string{email}, "4draws: confirm your email address"))
}

func verifyNotice(link, site string) mail.Notice {
	return mail.Notice{
		Preheader: "One tap confirms that this address is yours.",
		Heading:   "Confirm your email address",
		Intro: []string{
			"This address was given for a 4draws account. Confirm that it is yours, so we know where to send a password-reset link or a copy of your data.",
		},
		Action:     &mail.Action{Label: "Confirm my email address", URL: link},
		ActionNote: "The link works for 24 hours and can be used once. You can ask for a new one in the app, under Account.",
		Outro: []string{
			"Once it is confirmed you can also sign in with Google, if this is the address of your Google account.",
		},
		Safety:  "If you did not create a 4draws account, do not open the link. Nothing is confirmed until it is opened, and the address can be removed by writing to " + mail.SupportAddress + ".",
		Reason:  "You are receiving this email because this address was entered on a 4draws account.",
		SiteURL: site,
	}
}

// VerifyEmail confirms the address an emailed link was sent to, if it is still the address on the account, and
// spends the link (and any other open link of the account). Anything else is ErrVerifyLinkInvalid.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	if token == "" {
		return ErrVerifyLinkInvalid
	}
	hash := sha256.Sum256([]byte(token))
	return s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		v, err := q.EmailVerificationByHash(ctx, hash[:])
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrVerifyLinkInvalid
		} else if err != nil {
			return err
		}
		if v.UsedAt != nil || !now.Before(v.ExpiresAt) {
			return ErrVerifyLinkInvalid
		}
		ok, err := s.confirmEmail(ctx, tx, q, v.UserID, v.Email, now)
		if err != nil {
			return err
		}
		if !ok { // the address changed since, the account closed, it is confirmed already, or another account confirmed it
			return ErrVerifyLinkInvalid
		}
		if err := q.UseEmailVerifications(ctx, store.UseEmailVerificationsParams{At: &now, UserID: v.UserID}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: v.UserID, Action: "identity.email_verified", EntityType: "user", EntityID: v.UserID})
	})
}

// confirmEmail marks email as the confirmed address of userID if it is the address on that account and no other
// account has confirmed it, and takes it from the accounts that only claimed it (ADR 0041). It reports whether the
// account's address became confirmed.
func (s *Service) confirmEmail(ctx context.Context, tx pgx.Tx, q *store.Queries, userID, email string, now time.Time) (bool, error) {
	if err := emailFree(ctx, q, email, &userID); errors.Is(err, ErrEmailRegistered) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	n, err := q.MarkEmailVerified(ctx, store.MarkEmailVerifiedParams{At: &now, ID: userID, Email: email})
	if err != nil || n == 0 {
		return false, err
	}
	return true, s.releaseClaims(ctx, tx, q, userID, email, now)
}
