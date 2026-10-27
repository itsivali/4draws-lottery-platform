package identity_test

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// Staff sign-in reads the ID token's claims (the email address); only the local staff realm may omit the nonce.
func TestOIDC_IDTokenClaimsAreReturnedAndOnlyTheLocalRealmMayOmitTheNonce(t *testing.T) {
	f := newOIDC(t)
	ctx := context.Background()
	good := f.claims(jwt.MapClaims{"email": "owner@example.com", "email_verified": true})
	claims, err := f.v.VerifyIDTokenClaims(ctx, "google", f.token(t, "k1", jwt.SigningMethodRS256, good), "n1")
	if err != nil || claims["sub"] != "g-42" || claims["email"] != "owner@example.com" {
		t.Fatalf("claims %v %v", claims, err)
	}
	noNonce := f.token(t, "k1", jwt.SigningMethodRS256, f.claims(jwt.MapClaims{"nonce": nil}))
	if _, err := f.v.VerifyIDTokenClaims(ctx, "google", noNonce, ""); err == nil {
		t.Fatal("Google token accepted without a nonce")
	}
	if _, err := f.v.VerifyIDTokenClaims(ctx, "dev", noNonce, ""); err != nil {
		t.Fatalf("local realm without a nonce: %v", err)
	}
	if _, err := f.v.VerifyIDTokenClaims(ctx, "dev", f.token(t, "k1", jwt.SigningMethodRS256, good), "other"); err == nil {
		t.Fatal("a nonce that is given must still match")
	}
	for name, c := range map[string]jwt.MapClaims{
		"wrong issuer":   f.claims(jwt.MapClaims{"iss": "https://evil.example"}),
		"wrong audience": f.claims(jwt.MapClaims{"aud": "someone-else"}),
		"expired":        f.claims(jwt.MapClaims{"exp": 1}),
		"no subject":     f.claims(jwt.MapClaims{"sub": nil}),
	} {
		if _, err := f.v.VerifyIDTokenClaims(ctx, "dev", f.token(t, "k1", jwt.SigningMethodRS256, c), ""); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := f.v.VerifyIDTokenClaims(ctx, "nobody", f.token(t, "k1", jwt.SigningMethodRS256, good), "n1"); err == nil {
		t.Error("unknown provider accepted")
	}
}
