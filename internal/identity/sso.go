package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/identity/store"
)

// claimsVerifier is a verifier that also returns the token's claims (the real one does), for the account's photo.
type claimsVerifier interface {
	VerifyIDTokenClaims(ctx context.Context, provider, idToken, nonce string) (jwt.MapClaims, error)
}

// verifyIDToken returns the token's subject and, for Google, the account's photo (ADR 0035) and the email address
// Google vouches for (ADR 0039; "" when it vouches for none).
func (s *Service) verifyIDToken(ctx context.Context, provider, idToken, nonce string) (sub, photo, email string, err error) {
	if cv, ok := s.oidc.(claimsVerifier); ok {
		claims, err := cv.VerifyIDTokenClaims(ctx, provider, idToken, nonce)
		if err != nil {
			return "", "", "", errors.Join(ErrTokenInvalid, err)
		}
		if sub, _ = claims.GetSubject(); sub == "" {
			return "", "", "", ErrTokenInvalid
		}
		return sub, GooglePhoto(provider, claims), GoogleEmail(provider, claims), nil
	}
	sub, err = s.oidc.VerifyIDToken(ctx, provider, idToken, nonce)
	if err != nil || sub == "" {
		return "", "", "", errors.Join(ErrTokenInvalid, err)
	}
	return sub, "", "", nil
}

// GoogleEmail returns the email address of a verified Google ID token when Google is the authority for it: the
// token says the address is verified, and it is a gmail.com address or belongs to a Google Workspace domain (the hd
// claim). Any other address on a Google account proves only that someone once received a message there, so it is
// not returned. Lower-cased, as accounts store theirs.
func GoogleEmail(provider string, claims jwt.MapClaims) string {
	if provider != "google" {
		return ""
	}
	if v, _ := claims["email_verified"].(bool); !v {
		if sv, _ := claims["email_verified"].(string); sv != "true" {
			return ""
		}
	}
	email, _ := claims["email"].(string)
	email = strings.ToLower(strings.TrimSpace(email))
	hd, _ := claims["hd"].(string)
	vouched := strings.HasSuffix(email, "@gmail.com") || hd != ""
	if email == "" || len(email) > 254 || !vouched {
		return ""
	}
	return email
}

// keepPhoto records the photo of the Google account the player signed in with; a Google account without one clears
// it. Apple gives no photo and changes nothing.
func keepPhoto(ctx context.Context, q *store.Queries, userID, provider, photo string) error {
	if provider != "google" {
		return nil
	}
	var p *string
	if photo != "" {
		p = &photo
	}
	return q.SetPhoto(ctx, store.SetPhotoParams{PhotoUrl: p, ID: userID})
}

// SignInSSO signs in a Google or Apple identity. One already linked to an account signs that account in. A Google
// identity linked to none signs in the account that has its email address, and is linked to it from then on
// (ADR 0039), but only once the account has confirmed that address (ADR 0040): until then the address is only what
// someone typed, and ErrSSOEmailUnverified tells its owner to sign in the other way and confirm it. With no such
// account it is ErrSSONotLinked, and the app offers to create one (SignupSSO).
func (s *Service) SignInSSO(ctx context.Context, provider, idToken, nonce string, dev Device) (Tokens, error) {
	sub, photo, email, err := s.verifyIDToken(ctx, provider, idToken, nonce)
	if err != nil {
		return Tokens{}, err
	}
	verdict := s.verdict(ctx, dev)
	var tokens Tokens
	err = s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		userID, err := q.FindAuthIdentity(ctx, store.FindAuthIdentityParams{Provider: provider, Subject: sub})
		linkedNow := false
		if errors.Is(err, pgx.ErrNoRows) {
			if email == "" {
				return ErrSSONotLinked
			}
			u, err := q.UserByEmail(ctx, email)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrSSONotLinked
			}
			if err != nil {
				return err
			}
			if !u.EmailVerified { // whoever typed the address has not shown it is theirs (ADR 0040)
				return ErrSSOEmailUnverified
			}
			userID, linkedNow = u.ID, true
		} else if err != nil {
			return err
		}
		if err := s.assertActive(ctx, q, userID); err != nil {
			return err
		}
		if linkedNow {
			if err := q.InsertAuthIdentity(ctx, store.InsertAuthIdentityParams{UserID: userID, Provider: provider, Subject: sub}); err != nil {
				return err
			}
			if err := s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: userID, Action: "sso.linked_by_email",
				EntityType: "user", EntityID: userID, Details: map[string]any{"provider": provider}}); err != nil {
				return err
			}
		}
		if err := keepPhoto(ctx, q, userID, provider, photo); err != nil {
			return err
		}
		if email != "" && !linkedNow {
			// Google vouches for this address and the identity is already the account's: if the account has the same
			// address, it is confirmed (a no-op when the address differs or is confirmed already)
			if _, err := s.confirmEmail(ctx, tx, q, userID, email, now); err != nil {
				return err
			}
		}
		tokens, err = s.startSession(ctx, q, userID, dev, verdict, now)
		return err
	})
	return tokens, err
}

// LinkSSO attaches a Google or Apple identity to a phone- and KYC-verified account.
func (s *Service) LinkSSO(ctx context.Context, userID, provider, idToken, nonce string) error {
	return s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, _ time.Time) error {
		u, err := q.GetUser(ctx, userID)
		if err != nil {
			return err
		}
		if u.KycStatus != "verified" {
			return ErrKYCRequired
		}
		if !u.PhoneVerified {
			return ErrPhoneNotVerified
		}
		sub, photo, _, err := s.verifyIDToken(ctx, provider, idToken, nonce)
		if err != nil {
			return err
		}
		if err := keepPhoto(ctx, q, userID, provider, photo); err != nil {
			return err
		}
		if err := q.InsertAuthIdentity(ctx, store.InsertAuthIdentityParams{UserID: userID, Provider: provider, Subject: sub}); err != nil {
			var pe *pgconn.PgError
			if errors.As(err, &pe) && pe.Code == "23505" {
				return ErrSSOAlreadyLinked
			}
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "sso.linked", EntityType: "user", EntityID: userID,
			Details: map[string]any{"provider": provider}})
	})
}

// SignupSSO creates an account for a Google or Apple identity that has none, from the phone number the player gives,
// links the identity and signs the account in (ADR 0020). The account has no password: the identity is how it signs
// in (staff may still give it a temporary one). As with Signup, the number is a claim until the first top-up proves it,
// and a number with a proved owner cannot be claimed.
func (s *Service) SignupSSO(ctx context.Context, provider, idToken, nonce, rawMSISDN string, dev Device, created ...Created) (Tokens, error) {
	msisdn, err := NormalizeMSISDN(rawMSISDN)
	if err != nil {
		return Tokens{}, err
	}
	sub, photo, email, err := s.verifyIDToken(ctx, provider, idToken, nonce)
	if err != nil {
		return Tokens{}, err
	}
	verdict := s.verdict(ctx, dev)
	var tokens Tokens
	err = s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		if _, err := q.FindAuthIdentity(ctx, store.FindAuthIdentityParams{Provider: provider, Subject: sub}); err == nil {
			return ErrSSOAlreadyLinked
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		taken, err := q.VerifiedAccountExists(ctx, msisdn)
		if err != nil {
			return err
		}
		if taken {
			return ErrPhoneRegistered
		}
		id, err := q.InsertAccount(ctx, store.InsertAccountParams{Msisdn: msisdn, At: &now})
		if err != nil {
			return err
		}
		for _, c := range created {
			if err := c(ctx, tx, id); err != nil {
				return err
			}
		}
		if err := keepPhoto(ctx, q, id, provider, photo); err != nil {
			return err
		}
		if email != "" {
			// the address Google vouches for becomes the account's, already confirmed (ADR 0040), and is taken from
			// accounts that only claimed it (ADR 0041). If another account has confirmed it, this one has none until
			// the player adds one.
			if err := emailFree(ctx, q, email, nil); err == nil {
				if err := q.SetVerifiedEmail(ctx, store.SetVerifiedEmailParams{Email: email, At: &now, ID: id}); err != nil {
					return emailError(err)
				}
				if err := s.releaseClaims(ctx, tx, q, id, email, now); err != nil {
					return err
				}
			} else if !errors.Is(err, ErrEmailRegistered) {
				return err
			}
		}
		if err := q.InsertAuthIdentity(ctx, store.InsertAuthIdentityParams{UserID: id, Provider: provider, Subject: sub}); err != nil {
			var pe *pgconn.PgError
			if errors.As(err, &pe) && pe.Code == "23505" {
				return ErrSSOAlreadyLinked
			}
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "user.created", EntityType: "user", EntityID: id,
			Details: map[string]any{"provider": provider}}); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Event{ActorType: audit.System, Action: "sso.linked", EntityType: "user", EntityID: id,
			Details: map[string]any{"provider": provider}}); err != nil {
			return err
		}
		tokens, err = s.startSession(ctx, q, id, dev, verdict, now)
		return err
	})
	return tokens, err
}
