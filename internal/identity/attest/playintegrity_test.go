package attest_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/itsivali/lottery-platform/internal/identity/attest"
)

const pkg = "ke.tajirika.lottery_mobile"

type fakeGoogle struct {
	srv        *httptest.Server
	key        *rsa.PrivateKey
	tokens     atomic.Int64
	decodes    atomic.Int64
	payload    map[string]any
	lastAuth   string
	lastDecode string
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	g := &fakeGoogle{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		claims := jwt.MapClaims{}
		_, err := jwt.ParseWithClaims(r.Form.Get("assertion"), claims, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
			jwt.WithValidMethods([]string{"RS256"}), jwt.WithTimeFunc(func() time.Time { return now }))
		if err != nil || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" ||
			claims["iss"] != "verifier@lottery.iam.gserviceaccount.com" || claims["scope"] != "https://www.googleapis.com/auth/playintegrity" {
			http.Error(w, "bad assertion", http.StatusUnauthorized)
			return
		}
		g.tokens.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "google-access", "expires_in": 3600, "token_type": "Bearer"})
	})
	mux.HandleFunc("POST /v1/{name}", func(w http.ResponseWriter, r *http.Request) {
		g.decodes.Add(1)
		g.lastAuth = r.Header.Get("Authorization")
		g.lastDecode = r.PathValue("name")
		var in struct {
			Token string `json:"integrity_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Token != "integrity-token" {
			http.Error(w, `{"error":{"code":400,"message":"invalid token"}}`, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tokenPayloadExternal": g.payload})
	})
	g.srv = httptest.NewServer(mux)
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGoogle) serviceAccount(t *testing.T) []byte {
	t.Helper()
	der, _ := x509.MarshalPKCS8PrivateKey(g.key)
	sa, _ := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "verifier@lottery.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": g.srv.URL + "/token",
	})
	return sa
}

func goodPayload(challenge string) map[string]any {
	return map[string]any{
		"requestDetails": map[string]any{"requestPackageName": pkg, "nonce": challenge, "timestampMillis": json.Number(itoa(now.Add(-30 * time.Second).UnixMilli()))},
		"appIntegrity": map[string]any{"appRecognitionVerdict": "PLAY_RECOGNIZED", "packageName": pkg,
			"certificateSha256Digest": []string{"Y2VydC1kaWdlc3Q"}, "versionCode": "12"},
		"deviceIntegrity": map[string]any{"deviceRecognitionVerdict": []string{"MEETS_DEVICE_INTEGRITY"}},
		"accountDetails":  map[string]any{"appLicensingVerdict": "LICENSED"},
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func verifier(t *testing.T, g *fakeGoogle, digests ...string) *attest.PlayIntegrity {
	t.Helper()
	d, err := attest.NewGoogleDecoder(g.serviceAccount(t), pkg, g.srv.URL, http.DefaultClient, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return attest.NewPlayIntegrity(pkg, digests, d, func() time.Time { return now })
}

func TestPlayIntegrity_AcceptsARecognisedAppOnAGenuineDeviceForThisChallenge(t *testing.T) {
	g := newFakeGoogle(t)
	g.payload = goodPayload("challenge-1")
	v := verifier(t, g, "Y2VydC1kaWdlc3Q")
	ok, err := v.Verify(context.Background(), "integrity-token", "challenge-1")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if g.lastAuth != "Bearer google-access" || g.lastDecode != pkg+":decodeIntegrityToken" {
		t.Fatalf("decode call: %q %q", g.lastAuth, g.lastDecode)
	}
	// the OAuth token is reused until it nears expiry
	if _, err := v.Verify(context.Background(), "integrity-token", "challenge-1"); err != nil {
		t.Fatal(err)
	}
	if g.tokens.Load() != 1 || g.decodes.Load() != 2 {
		t.Fatalf("tokens %d decodes %d", g.tokens.Load(), g.decodes.Load())
	}
}

func TestPlayIntegrity_AStandardRequestHashOfTheChallengeIsAccepted(t *testing.T) {
	g := newFakeGoogle(t)
	p := goodPayload("")
	h := sha256.Sum256([]byte("challenge-9"))
	p["requestDetails"].(map[string]any)["nonce"] = nil
	p["requestDetails"].(map[string]any)["requestHash"] = base64.RawURLEncoding.EncodeToString(h[:])
	g.payload = p
	if ok, err := verifier(t, g).Verify(context.Background(), "integrity-token", "challenge-9"); !ok {
		t.Fatalf("request hash refused: %v", err)
	}
}

func TestPlayIntegrity_RefusesEveryWeakOrMismatchedVerdict(t *testing.T) {
	cases := map[string]struct {
		edit   func(map[string]any)
		reason string
	}{
		"another challenge": {func(p map[string]any) { rd(p)["nonce"] = "challenge-2" }, "challenge"},
		"another package":   {func(p map[string]any) { rd(p)["requestPackageName"] = "com.evil" }, "package"},
		"stale":             {func(p map[string]any) { rd(p)["timestampMillis"] = json.Number(itoa(now.Add(-time.Hour).UnixMilli())) }, "old"},
		"unrecognised app": {func(p map[string]any) {
			p["appIntegrity"].(map[string]any)["appRecognitionVerdict"] = "UNRECOGNIZED_VERSION"
		}, "not recognised"},
		"basic integrity only": {func(p map[string]any) {
			p["deviceIntegrity"].(map[string]any)["deviceRecognitionVerdict"] = []string{"MEETS_BASIC_INTEGRITY"}
		}, "device"},
		"no device verdict": {func(p map[string]any) { p["deviceIntegrity"] = map[string]any{} }, "device"},
		"another signing cert": {func(p map[string]any) {
			p["appIntegrity"].(map[string]any)["certificateSha256Digest"] = []string{"b3RoZXI"}
		}, "certificate"},
	}
	for name, tc := range cases {
		g := newFakeGoogle(t)
		p := goodPayload("challenge-1")
		tc.edit(p)
		g.payload = p
		ok, err := verifier(t, g, "Y2VydC1kaWdlc3Q").Verify(context.Background(), "integrity-token", "challenge-1")
		if ok || err == nil || !strings.Contains(err.Error(), tc.reason) {
			t.Errorf("%s: ok=%v err=%v, want a refusal for %q", name, ok, err, tc.reason)
		}
	}
	g := newFakeGoogle(t)
	g.payload = goodPayload("challenge-1")
	if ok, err := verifier(t, g).Verify(context.Background(), "garbage", "challenge-1"); ok || err == nil {
		t.Errorf("a token Google rejects passed: %v", err)
	}
}

func rd(p map[string]any) map[string]any { return p["requestDetails"].(map[string]any) }

func TestGoogleDecoder_RefusesABadServiceAccount(t *testing.T) {
	if _, err := attest.NewGoogleDecoder([]byte(`{"client_email":"x"}`), pkg, "https://example.invalid", http.DefaultClient, time.Now); err == nil {
		t.Fatal("service account without a key accepted")
	}
}
