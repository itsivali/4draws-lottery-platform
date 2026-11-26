//go:build integration

package staff_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/staff"
)

// ADR 0035: a member who signs in with Google gets that account's photo, refreshed each time and only from Google's
// image host; one without a password can set one right after signing in, not later.
func TestStaff_GooglePhotoAndAFirstPasswordRightAfterSigningIn(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	const photo = "https://lh3.googleusercontent.com/a/owner-photo=s96-c"
	if err := e.svc.Bootstrap(ctx, "owner@example.com", ""); err != nil {
		t.Fatal(err)
	}
	e.ids["google:owner"] = jwt.MapClaims{"sub": "g-owner", "email": "owner@example.com", "email_verified": true, "picture": photo}
	m := e.signIn(t, "google", "owner")
	if m.PhotoURL != photo || m.HasPassword {
		t.Fatalf("member %+v", m)
	}
	e.ids["google:owner"] = jwt.MapClaims{"sub": "g-owner", "email": "owner@example.com", "email_verified": true, "picture": "http://lh3.googleusercontent.com/a/plain-http"}
	if m = e.signIn(t, "google", "owner"); m.PhotoURL != "" {
		t.Fatalf("a photo that is not https on Google's host was kept: %q", m.PhotoURL)
	}

	s, err := e.svc.SignIn(ctx, "google", "owner", "n")
	if err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(staff.FreshSignIn + time.Minute)
	late, err := e.svc.Authenticate(ctx, s.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ChangePassword(ctx, late, s.Token, "", "a long first password here"); !errors.Is(err, staff.ErrSignInAgain) {
		t.Fatalf("long after signing in: %v", err)
	}
	s, err = e.svc.SignIn(ctx, "google", "owner", "n")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := e.svc.Authenticate(ctx, s.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ChangePassword(ctx, fresh, s.Token, "", "a long first password here"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SignInWithPassword(ctx, "owner@example.com", "a long first password here"); err != nil {
		t.Fatalf("signing in with the new password: %v", err)
	}
	// with a password, changing it needs the current one, whenever the sign-in was
	e.clk.Advance(staff.FreshSignIn + time.Minute)
	again, err := e.svc.Authenticate(ctx, s.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !again.HasPassword {
		t.Fatal("has_password not reported")
	}
	if err := e.svc.ChangePassword(ctx, again, s.Token, "a long first password here", "another long password now"); err != nil {
		t.Fatalf("changing a password later: %v", err)
	}
}
