package identity_test

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// Staff access tokens (Keycloak) carry no nonce; the verifier checks signature, issuer, audience and expiry and
// returns the claims so the caller can read roles.
func TestOIDC_AccessTokenIsVerifiedWithoutANonceAndReturnsItsClaims(t *testing.T) {
	f := newOIDC(t)
	ctx := context.Background()
	good := f.claims(jwt.MapClaims{"nonce": nil, "realm_access": map[string]any{"roles": []any{"kyc_reviewer"}}})
	claims, err := f.v.VerifyAccessToken(ctx, "google", f.token(t, "k1", jwt.SigningMethodRS256, good))
	if err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "g-42" || claims["realm_access"] == nil {
		t.Fatalf("claims: %v", claims)
	}
	for name, c := range map[string]jwt.MapClaims{
		"wrong issuer":   f.claims(jwt.MapClaims{"iss": "https://evil.example"}),
		"wrong audience": f.claims(jwt.MapClaims{"aud": "someone-else"}),
		"expired":        f.claims(jwt.MapClaims{"exp": 1}),
		"no expiry":      f.claims(jwt.MapClaims{"exp": nil}),
		"no subject":     f.claims(jwt.MapClaims{"sub": nil}),
	} {
		if _, err := f.v.VerifyAccessToken(ctx, "google", f.token(t, "k1", jwt.SigningMethodRS256, c)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := f.v.VerifyAccessToken(ctx, "google", f.token(t, "k1", jwt.SigningMethodHS256, good)); err == nil {
		t.Error("HS256 accepted")
	}
	if _, err := f.v.VerifyAccessToken(ctx, "nobody", f.token(t, "k1", jwt.SigningMethodRS256, good)); err == nil {
		t.Error("unknown provider accepted")
	}
}
