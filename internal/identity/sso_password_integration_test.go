//go:build integration

package identity_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/platform/password"
)

// claimsOIDC is fakeOIDC that also gives the token's claims, as the real verifier does: "ok:<sub>|<picture>".
type claimsOIDC struct{ fakeOIDC }

func (f claimsOIDC) VerifyIDTokenClaims(ctx context.Context, provider, token, nonce string) (jwt.MapClaims, error) {
	sub, picture, _ := strings.Cut(token, "|")
	s, err := f.VerifyIDToken(ctx, provider, sub, nonce)
	if err != nil {
		return nil, err
	}
	c := jwt.MapClaims{"sub": s}
	if picture != "" {
		c["picture"] = picture
	}
	return c, nil
}

func withClaims(t *testing.T, e *env) *identity.Service {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return identity.NewService(e.pool, e.clk, claimsOIDC{}, audit.NewWriter(), identity.Config{
		AuthKey: []byte("0123456789abcdef0123456789abcdef"), TokenKey: priv, Issuer: "lottery", Audience: "lottery-api",
		AccessTTL: 10 * time.Minute, RefreshTTL: 30 * 24 * time.Hour, DeviceCap: 2,
	})
}

// ADR 0035: the photo of the Google account is kept, refreshed at each Google sign-in, and only from Google's image
// host; Apple changes nothing.
func TestSSO_TheGooglePhotoIsKeptFromGoogleOnly(t *testing.T) {
	e := setup(t)
	svc := withClaims(t, e)
	ctx := context.Background()
	const photo = "https://lh3.googleusercontent.com/a/ACg8ocJ-photo=s96-c"
	tok, err := svc.SignupSSO(ctx, "google", "ok:photo-1|"+photo, "n1", e.newMSISDN(), phone)
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.Authenticate(ctx, tok.Access)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := svc.Account(ctx, p.UserID); a.PhotoURL != photo {
		t.Fatalf("photo %q, want %q", a.PhotoURL, photo)
	}
	// a picture on another host is never kept: the account's photo is cleared
	if _, err := svc.SignInSSO(ctx, "google", "ok:photo-1|https://evil.example/pixel.png", "n1", phone); err != nil {
		t.Fatal(err)
	}
	if a, _ := svc.Account(ctx, p.UserID); a.PhotoURL != "" {
		t.Fatalf("photo %q from another host was kept", a.PhotoURL)
	}
	if _, err := svc.SignInSSO(ctx, "google", "ok:photo-1|"+photo, "n1", phone); err != nil {
		t.Fatal(err)
	}
	if a, _ := svc.Account(ctx, p.UserID); a.PhotoURL != photo {
		t.Fatalf("photo not refreshed: %q", a.PhotoURL)
	}
	if got := identity.GooglePhoto("apple", jwt.MapClaims{"picture": photo}); got != "" {
		t.Fatalf("Apple photo %q", got)
	}
}

// ADR 0035: an account that signs in with Google or Apple only can set a password, right after signing in, and then
// sign in with its phone number too; later, or with a password already set, it cannot.
func TestSetPassword_AGoogleAccountGetsAPasswordRightAfterSigningIn(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	m := e.newMSISDN()
	tok, err := e.svc.SignupSSO(ctx, "google", "ok:setpw-1", "n1", m, phone)
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.svc.Authenticate(ctx, tok.Access)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetPassword(ctx, p.UserID, p.SessionID, "password"); !errors.Is(err, password.ErrWeak) {
		t.Fatalf("a weak password: %v", err)
	}
	e.clk.Advance(identity.FreshSignIn + time.Second)
	if err := e.svc.SetPassword(ctx, p.UserID, p.SessionID, goodPassword); !errors.Is(err, identity.ErrSignInAgain) {
		t.Fatalf("long after signing in: %v", err)
	}
	fresh, err := e.svc.SignInSSO(ctx, "google", "ok:setpw-1", "n1", phone)
	if err != nil {
		t.Fatal(err)
	}
	fp, err := e.svc.Authenticate(ctx, fresh.Access)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetPassword(ctx, fp.UserID, fp.SessionID, goodPassword); err != nil {
		t.Fatal(err)
	}
	if a, _ := e.svc.Account(ctx, fp.UserID); !a.HasPassword {
		t.Fatal("no password after setting it")
	}
	if _, err := e.svc.Login(ctx, m, goodPassword, phone); err != nil {
		t.Fatalf("signing in with the phone number and the new password: %v", err)
	}
	if err := e.svc.SetPassword(ctx, fp.UserID, fp.SessionID, "another good passphrase"); !errors.Is(err, identity.ErrPasswordExists) {
		t.Fatalf("setting it again: %v", err)
	}
	if _, err := e.svc.Authenticate(ctx, tok.Access); err == nil {
		t.Fatal("the older session still works after the password was set")
	}
}
