package attest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Payload is the part of a decoded Play Integrity verdict the platform relies on.
type Payload struct {
	RequestDetails struct {
		RequestPackageName string      `json:"requestPackageName"`
		Nonce              string      `json:"nonce"`
		RequestHash        string      `json:"requestHash"`
		TimestampMillis    json.Number `json:"timestampMillis"`
	} `json:"requestDetails"`
	AppIntegrity struct {
		AppRecognitionVerdict   string   `json:"appRecognitionVerdict"`
		PackageName             string   `json:"packageName"`
		CertificateSha256Digest []string `json:"certificateSha256Digest"`
	} `json:"appIntegrity"`
	DeviceIntegrity struct {
		DeviceRecognitionVerdict []string `json:"deviceRecognitionVerdict"`
	} `json:"deviceIntegrity"`
}

// Decoder turns an integrity token into its verdict (Google decrypts and verifies it).
type Decoder interface {
	Decode(ctx context.Context, token string) (Payload, error)
}

// PlayIntegrity judges Play Integrity verdicts
// (https://developer.android.com/google/play/integrity/verdicts).
type PlayIntegrity struct {
	pkg     string
	digests []string // allowed signing-certificate digests (base64url SHA-256); empty skips the check
	decoder Decoder
	now     func() time.Time
	maxAge  time.Duration
}

// NewPlayIntegrity returns a verifier for the app pkg. certDigests, when given, are the only signing certificates
// accepted (as Play reports them).
func NewPlayIntegrity(pkg string, certDigests []string, d Decoder, now func() time.Time) *PlayIntegrity {
	return &PlayIntegrity{pkg: pkg, digests: certDigests, decoder: d, now: now, maxAge: 5 * time.Minute}
}

// Verify decodes token and checks it was requested by this app, for challenge (as the classic nonce, or as the
// standard request's requestHash = base64url SHA-256(challenge)), recently, by a Play-recognised build on a device
// that meets device integrity.
func (v *PlayIntegrity) Verify(ctx context.Context, token, challenge string) (bool, error) {
	p, err := v.decoder.Decode(ctx, token)
	if err != nil {
		return false, err
	}
	rd, app := p.RequestDetails, p.AppIntegrity
	h := sha256.Sum256([]byte(challenge))
	if rd.RequestPackageName != v.pkg || app.PackageName != v.pkg {
		return false, errors.New("playintegrity: requested by another package")
	}
	if challenge == "" || (rd.Nonce != challenge && rd.RequestHash != base64.RawURLEncoding.EncodeToString(h[:])) {
		return false, errors.New("playintegrity: not bound to this challenge")
	}
	ms, err := rd.TimestampMillis.Int64()
	if err != nil {
		return false, errors.New("playintegrity: no request time")
	}
	if age := v.now().Sub(time.UnixMilli(ms)); age > v.maxAge || age < -time.Minute {
		return false, errors.New("playintegrity: verdict too old")
	}
	if app.AppRecognitionVerdict != "PLAY_RECOGNIZED" {
		return false, errors.New("playintegrity: app not recognised by Play")
	}
	if len(v.digests) > 0 && !slices.ContainsFunc(app.CertificateSha256Digest, func(d string) bool { return slices.Contains(v.digests, d) }) {
		return false, errors.New("playintegrity: signed with another certificate")
	}
	if !slices.Contains(p.DeviceIntegrity.DeviceRecognitionVerdict, "MEETS_DEVICE_INTEGRITY") {
		return false, errors.New("playintegrity: device integrity not met")
	}
	return true, nil
}

// GoogleDecoder calls Google's decodeIntegrityToken with a service account.
type GoogleDecoder struct {
	pkg      string
	base     string
	hc       *http.Client
	now      func() time.Time
	email    string
	tokenURI string
	key      any

	mu      sync.Mutex
	access  string
	expires time.Time
}

const playIntegrityScope = "https://www.googleapis.com/auth/playintegrity"

// NewGoogleDecoder reads a service account key (JSON, from Google Cloud) linked to the app in Play Console. base is
// the API root ("https://playintegrity.googleapis.com").
func NewGoogleDecoder(serviceAccountJSON []byte, pkg, base string, hc *http.Client, now func() time.Time) (*GoogleDecoder, error) {
	var sa struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal(serviceAccountJSON, &sa); err != nil {
		return nil, fmt.Errorf("playintegrity: service account: %w", err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(sa.PrivateKey))
	if err != nil || sa.ClientEmail == "" {
		return nil, errors.New("playintegrity: the service account needs client_email and private_key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return &GoogleDecoder{pkg: pkg, base: strings.TrimRight(base, "/"), hc: hc, now: now, email: sa.ClientEmail, tokenURI: sa.TokenURI, key: key}, nil
}

// Decode implements Decoder.
func (g *GoogleDecoder) Decode(ctx context.Context, token string) (Payload, error) {
	access, err := g.accessToken(ctx)
	if err != nil {
		return Payload{}, err
	}
	body, _ := json.Marshal(map[string]string{"integrity_token": token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.base+"/v1/"+url.PathEscape(g.pkg)+":decodeIntegrityToken", bytes.NewReader(body))
	if err != nil {
		return Payload{}, err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.hc.Do(req)
	if err != nil {
		return Payload{}, fmt.Errorf("playintegrity: decode: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Payload{}, fmt.Errorf("playintegrity: decode status %d", resp.StatusCode)
	}
	var out struct {
		TokenPayloadExternal Payload `json:"tokenPayloadExternal"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return Payload{}, fmt.Errorf("playintegrity: decode body: %w", err)
	}
	return out.TokenPayloadExternal, nil
}

func (g *GoogleDecoder) accessToken(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if g.access != "" && now.Before(g.expires.Add(-time.Minute)) {
		return g.access, nil
	}
	assertion, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": g.email, "scope": playIntegrityScope, "aud": g.tokenURI, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}).SignedString(g.key)
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("playintegrity: token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("playintegrity: token status %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("playintegrity: no access token")
	}
	g.access, g.expires = tok.AccessToken, now.Add(time.Duration(tok.ExpiresIn)*time.Second)
	return g.access, nil
}
