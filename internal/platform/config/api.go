package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// API is the configuration of the public /v1 API. It is all-or-nothing: with none of the secrets set the process
// serves only its health probes; with some but not all set it refuses to start, so a half-configured deployment can
// never expose a half-working (or half-secured) API.
type API struct {
	Enabled bool

	AuthKey       []byte          // HMAC root key: password pepper and device-fingerprint keys derive from it
	TokenSeed     []byte          // ed25519 seed for access tokens
	IdentityKey   []byte          // HMAC key for the one-account-per-national-ID lookup hash
	DataKeys      map[byte][]byte // key-encryption keys for KYC fields, by id
	DataKeyActive byte
	ValkeyAddr    string

	// TrustedProxyHeader is the header our load balancer sets with the client address; "" means use the socket address.
	// TrustedProxyHops is how many trusted proxies appended to X-Forwarded-For.
	TrustedProxyHeader string
	TrustedProxyHops   int

	KYCURL, KYCKey string

	// SignIn is the players' Google and Apple sign-in (ADR 0020); every id unset means neither is offered.
	SignIn SignIn

	// CORSOrigins are the browser origins (the player web app) allowed to call the API; empty turns CORS off.
	CORSOrigins []string

	// Staff configures the back office's staff sign-in (off when unset).
	Staff Staff

	// Attestation configures server-side device attestation.
	Attestation Attestation
}

// SignIn holds the client ids the player apps request Google and Apple ID tokens for, which are the audiences the API
// accepts (ADR 0020). GOOGLE_CLIENT_ID is the web app's OAuth client, also the server client id the phone apps name,
// so every Google token carries it or GOOGLE_IOS_CLIENT_ID (the iOS app's own client). APPLE_CLIENT_ID is the Services
// ID the web app and Android use; APPLE_BUNDLE_ID is the iOS app, whose native Sign in with Apple tokens name it.
// ANDROID_PACKAGE is the Android app Apple's return URL sends the browser back to.
type SignIn struct {
	GoogleClientID    string
	GoogleIOSClientID string
	AppleClientID     string
	AppleBundleID     string
	AndroidPackage    string
}

// Google reports whether Google sign-in is offered.
func (s SignIn) Google() bool { return s.GoogleClientID != "" }

// Apple reports whether Apple sign-in is offered (on iOS alone when only the bundle id is set).
func (s SignIn) Apple() bool { return s.AppleClientID != "" || s.AppleBundleID != "" }

// GoogleAudiences are the client ids a Google ID token may name.
func (s SignIn) GoogleAudiences() []string { return nonEmpty(s.GoogleClientID, s.GoogleIOSClientID) }

// AppleAudiences are the client ids an Apple ID token may name.
func (s SignIn) AppleAudiences() []string { return nonEmpty(s.AppleClientID, s.AppleBundleID) }

func nonEmpty(vs ...string) []string {
	var out []string
	for _, v := range vs {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

var androidPackageRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)

func loadSignIn() (SignIn, error) {
	s := SignIn{GoogleClientID: os.Getenv("GOOGLE_CLIENT_ID"), GoogleIOSClientID: os.Getenv("GOOGLE_IOS_CLIENT_ID"),
		AppleClientID: os.Getenv("APPLE_CLIENT_ID"), AppleBundleID: os.Getenv("APPLE_BUNDLE_ID"), AndroidPackage: os.Getenv("ANDROID_PACKAGE")}
	if s.GoogleIOSClientID != "" && s.GoogleClientID == "" {
		return SignIn{}, errors.New("config: GOOGLE_IOS_CLIENT_ID needs GOOGLE_CLIENT_ID (the web client every app names as its server client)")
	}
	if s.AndroidPackage != "" && !androidPackageRe.MatchString(s.AndroidPackage) {
		return SignIn{}, errors.New("config: ANDROID_PACKAGE must be an Android application id such as ke.tajirika.lottery_mobile")
	}
	for _, v := range []struct{ k, v string }{{"GOOGLE_CLIENT_ID", s.GoogleClientID}, {"GOOGLE_IOS_CLIENT_ID", s.GoogleIOSClientID}, {"APPLE_CLIENT_ID", s.AppleClientID}, {"APPLE_BUNDLE_ID", s.AppleBundleID}} {
		if strings.ContainsAny(v.v, " \t\n,") {
			return SignIn{}, fmt.Errorf("config: %s must be a single client id", v.k)
		}
	}
	return s, nil
}

// Attestation is ATTESTATION_MODE and its verifiers. Modes: off (never verify), record (verify and store verdicts;
// the default), refuse_failed (devices that failed may not play, top up or withdraw), require (phones must have
// passed). refuse_failed and require need both verifiers.
type Attestation struct {
	Mode               string
	Web                string // allow (default) or refuse: web sessions cannot be attested, and anyone can claim to be one
	AppID              string // App Attest: "<team id>.<bundle id>"
	AllowDevelopment   bool   // App Attest: also accept development builds (never in production)
	PlayPackage        string
	PlayServiceAccount []byte // Play Integrity: the service account key (JSON)
	PlayCertDigests    []string
}

func loadAttestation() (Attestation, error) {
	a := Attestation{Mode: os.Getenv("ATTESTATION_MODE"), AppID: os.Getenv("APP_ATTEST_APP_ID"),
		AllowDevelopment: os.Getenv("APP_ATTEST_ALLOW_DEVELOPMENT") == "true", PlayPackage: os.Getenv("PLAY_INTEGRITY_PACKAGE")}
	if a.Mode == "" {
		a.Mode = "record"
	}
	switch a.Web = os.Getenv("ATTESTATION_WEB"); a.Web {
	case "":
		a.Web = "allow"
	case "allow", "refuse":
	default:
		return Attestation{}, errors.New("config: ATTESTATION_WEB must be allow or refuse")
	}
	if !slices.Contains([]string{"off", "record", "refuse_failed", "require"}, a.Mode) {
		return Attestation{}, errors.New("config: ATTESTATION_MODE must be off, record, refuse_failed or require")
	}
	if v := os.Getenv("PLAY_INTEGRITY_SERVICE_ACCOUNT"); v != "" {
		b, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return Attestation{}, errors.New("config: PLAY_INTEGRITY_SERVICE_ACCOUNT must be the service account JSON, base64-encoded")
		}
		a.PlayServiceAccount = b
	}
	for d := range strings.SplitSeq(os.Getenv("PLAY_INTEGRITY_CERT_DIGESTS"), ",") {
		if d = strings.TrimSpace(d); d != "" {
			a.PlayCertDigests = append(a.PlayCertDigests, d)
		}
	}
	if (a.PlayPackage == "") != (len(a.PlayServiceAccount) == 0) {
		return Attestation{}, errors.New("config: PLAY_INTEGRITY_PACKAGE and PLAY_INTEGRITY_SERVICE_ACCOUNT go together")
	}
	if a.Mode == "refuse_failed" || a.Mode == "require" {
		if a.AppID == "" || a.PlayPackage == "" {
			return Attestation{}, fmt.Errorf("config: ATTESTATION_MODE=%s needs both verifiers: APP_ATTEST_APP_ID and PLAY_INTEGRITY_PACKAGE (with PLAY_INTEGRITY_SERVICE_ACCOUNT)", a.Mode)
		}
	}
	return a, nil
}

// Staff is the back office's sign-in (ADR 0018, 0019): every member has an email address and password; Google and
// Apple ID tokens for the back office's own client ids are optional, as is the local staff realm for local development
// and the e2e journey. Members and roles are in the database; BootstrapAdmin is made an admin while there is none, and
// BootstrapPassword becomes their temporary password while they have none. DatabaseURL is what the back office acts
// through (the worker's role; empty uses DATABASE_URL, for single-role mode).
type Staff struct {
	Enabled           bool   // always: password sign-in needs no provider
	GoogleClientID    string // STAFF_GOOGLE_CLIENT_ID: the back office's OAuth web client
	AppleClientID     string // STAFF_APPLE_CLIENT_ID: the back office's Sign in with Apple Services ID
	Dev               *DevOIDC
	BootstrapAdmin    string
	BootstrapPassword string // STAFF_BOOTSTRAP_PASSWORD: a secret; set once, changed at the first sign-in
	// BootstrapReset (STAFF_BOOTSTRAP_RESET=true) is the way back in when the only administrator has lost their
	// password and nobody can reach the database: every start gives the bootstrap admin STAFF_BOOTSTRAP_PASSWORD again
	// as a temporary password (changed at their next sign-in), re-enables them and ends their sessions. Set it, deploy,
	// sign in, choose a password, then remove it; it is logged as a warning on every start while it is on.
	BootstrapReset bool
	DatabaseURL    string
}

// DevOIDC is the local staff realm (Keycloak in compose): never configured in production.
type DevOIDC struct {
	Issuer    string
	ClientIDs []string
	JWKSURL   string
}

// Providers names the sign-in providers configured, in the order the back office offers them. Password is always one.
func (s Staff) Providers() []string {
	out := []string{"password"}
	if s.GoogleClientID != "" {
		out = append(out, "google")
	}
	if s.AppleClientID != "" {
		out = append(out, "apple")
	}
	if s.Dev != nil {
		out = append(out, "dev")
	}
	return out
}

var devOIDCVars = []string{"STAFF_DEV_OIDC_ISSUER", "STAFF_DEV_OIDC_CLIENT_IDS", "STAFF_DEV_OIDC_JWKS_URL"}

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func loadStaff() (Staff, error) {
	s := Staff{GoogleClientID: os.Getenv("STAFF_GOOGLE_CLIENT_ID"), AppleClientID: os.Getenv("STAFF_APPLE_CLIENT_ID"),
		BootstrapAdmin: strings.ToLower(strings.TrimSpace(os.Getenv("STAFF_BOOTSTRAP_ADMIN"))), BootstrapPassword: os.Getenv("STAFF_BOOTSTRAP_PASSWORD"),
		DatabaseURL: os.Getenv("STAFF_DATABASE_URL"), BootstrapReset: os.Getenv("STAFF_BOOTSTRAP_RESET") == "true"}
	var set, missing []string
	for _, k := range devOIDCVars {
		if os.Getenv(k) != "" {
			set = append(set, k)
		} else {
			missing = append(missing, k)
		}
	}
	switch {
	case len(set) > 0 && len(missing) > 0:
		return Staff{}, fmt.Errorf("config: the local staff realm is partly configured (%s set) but %s missing", strings.Join(set, ", "), strings.Join(missing, ", "))
	case len(set) > 0:
		d := &DevOIDC{Issuer: os.Getenv("STAFF_DEV_OIDC_ISSUER"), JWKSURL: os.Getenv("STAFF_DEV_OIDC_JWKS_URL")}
		for _, id := range strings.Split(os.Getenv("STAFF_DEV_OIDC_CLIENT_IDS"), ",") {
			if id = strings.TrimSpace(id); id != "" {
				d.ClientIDs = append(d.ClientIDs, id)
			}
		}
		if err := checkGatewayURL("STAFF_DEV_OIDC_JWKS_URL", d.JWKSURL); err != nil {
			return Staff{}, err
		}
		s.Dev = d
	}
	if s.BootstrapAdmin != "" && !emailRE.MatchString(s.BootstrapAdmin) {
		return Staff{}, errors.New("config: STAFF_BOOTSTRAP_ADMIN must be an email address")
	}
	if s.BootstrapPassword != "" && s.BootstrapAdmin == "" {
		return Staff{}, errors.New("config: STAFF_BOOTSTRAP_PASSWORD needs STAFF_BOOTSTRAP_ADMIN")
	}
	if s.BootstrapReset && s.BootstrapPassword == "" {
		return Staff{}, errors.New("config: STAFF_BOOTSTRAP_RESET needs STAFF_BOOTSTRAP_PASSWORD (and STAFF_BOOTSTRAP_ADMIN)")
	}
	s.Enabled = true
	return s, nil
}

var apiSecrets = []string{"AUTH_KEY", "TOKEN_SEED", "DATA_KEYS", "IDENTITY_KEY", "VALKEY_ADDR"}

// dataKeysEnv returns the key list. DATA_KEY (one base64 key) is shorthand for "1:<key>" so hosts that can only generate
// a bare random value can supply it; rotation uses the explicit DATA_KEYS form.
func dataKeysEnv() string {
	if v := os.Getenv("DATA_KEYS"); v != "" {
		return v
	}
	if v := os.Getenv("DATA_KEY"); v != "" {
		return "1:" + v
	}
	return ""
}

func loadAPI() (API, error) {
	var set, missing []string
	for _, k := range apiSecrets {
		present := os.Getenv(k) != ""
		if k == "DATA_KEYS" {
			present = dataKeysEnv() != ""
		}
		if present {
			set = append(set, k)
		} else {
			missing = append(missing, k)
		}
	}
	if len(set) == 0 {
		return API{}, nil
	}
	if len(missing) > 0 {
		return API{}, fmt.Errorf("config: the API is partly configured (%s set) but %s missing", strings.Join(set, ", "), strings.Join(missing, ", "))
	}

	a := API{Enabled: true, ValkeyAddr: os.Getenv("VALKEY_ADDR"), TrustedProxyHops: 1,
		KYCURL: os.Getenv("KYC_API_URL"), KYCKey: os.Getenv("KYC_API_KEY")}
	var err error
	if a.SignIn, err = loadSignIn(); err != nil {
		return API{}, err
	}
	if a.AuthKey, err = secret("AUTH_KEY", 32, false); err != nil {
		return API{}, err
	}
	if a.TokenSeed, err = secret("TOKEN_SEED", 32, true); err != nil {
		return API{}, err
	}
	if a.IdentityKey, err = secret("IDENTITY_KEY", 32, false); err != nil {
		return API{}, err
	}
	if err := a.loadDataKeys(); err != nil {
		return API{}, err
	}

	switch h := os.Getenv("TRUSTED_PROXY_HEADER"); h {
	case "":
		return API{}, errors.New("config: TRUSTED_PROXY_HEADER must be set (the header your load balancer sets, e.g. X-Forwarded-For; use 'none' only when there is no proxy), otherwise per-IP rate limits would key on the balancer's address")
	case "none":
	default:
		a.TrustedProxyHeader = h
	}
	if v := os.Getenv("TRUSTED_PROXY_HOPS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 5 {
			return API{}, errors.New("config: TRUSTED_PROXY_HOPS must be an integer from 1 to 5")
		}
		a.TrustedProxyHops = n
	}
	if a.CORSOrigins, err = corsOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")); err != nil {
		return API{}, err
	}
	if a.Staff, err = loadStaff(); err != nil {
		return API{}, err
	}
	if a.Attestation, err = loadAttestation(); err != nil {
		return API{}, err
	}
	for _, g := range []struct{ name, val string }{{"KYC_API_URL", a.KYCURL}} {
		if err := checkGatewayURL(g.name, g.val); err != nil {
			return API{}, err
		}
	}
	return a, nil
}

// corsOrigins parses CORS_ALLOWED_ORIGINS: a comma-separated list of exact origins. Each must be https, or http on
// localhost or 127.0.0.1 for development; wildcards, paths, queries and credentials are refused, so a typo can never
// open the API to every site.
func corsOrigins(v string) ([]string, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	var out []string
	for part := range strings.SplitSeq(v, ",") {
		o := strings.TrimSpace(part)
		u, err := url.Parse(o)
		local := u != nil && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")
		schemeOK := u != nil && (u.Scheme == "https" || (u.Scheme == "http" && local))
		if err != nil || !schemeOK || u.Host == "" || strings.Contains(o, "*") || u.User != nil || u.Path != "" || u.RawQuery != "" ||
			u.Fragment != "" || strings.HasSuffix(o, "?") {
			return nil, fmt.Errorf("config: CORS_ALLOWED_ORIGINS entry %q must be an exact origin such as https://play.example.com (http only on localhost)", o)
		}
		out = append(out, o)
	}
	return out, nil
}

// secret decodes a base64 variable. exact requires exactly n bytes, otherwise at least n.
func secret(name string, n int, exact bool) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(os.Getenv(name))
	if err != nil {
		return nil, fmt.Errorf("config: %s is not valid base64", name)
	}
	if (exact && len(b) != n) || len(b) < n {
		return nil, fmt.Errorf("config: %s must decode to %s%d bytes", name, map[bool]string{true: "exactly ", false: "at least "}[exact], n)
	}
	return b, nil
}

// loadDataKeys parses DATA_KEYS ("1:<base64>,2:<base64>") and DATA_KEY_ACTIVE (default: the highest id).
func (a *API) loadDataKeys() error {
	a.DataKeys = map[byte][]byte{}
	for _, part := range strings.Split(dataKeysEnv(), ",") {
		idStr, enc, ok := strings.Cut(strings.TrimSpace(part), ":")
		id, err := strconv.ParseUint(idStr, 10, 8)
		if !ok || err != nil {
			return errors.New("config: DATA_KEYS must look like 1:<base64>[,2:<base64>]")
		}
		key, err := base64.StdEncoding.DecodeString(enc)
		if err != nil || len(key) != 32 {
			return errors.New("config: DATA_KEYS entries must be base64 of exactly 32 bytes")
		}
		if _, dup := a.DataKeys[byte(id)]; dup {
			return fmt.Errorf("config: DATA_KEYS repeats key id %d", id)
		}
		a.DataKeys[byte(id)] = key
	}
	ids := make([]byte, 0, len(a.DataKeys))
	for id := range a.DataKeys {
		ids = append(ids, id)
	}
	a.DataKeyActive = slices.Max(ids)
	if v := os.Getenv("DATA_KEY_ACTIVE"); v != "" {
		n, err := strconv.ParseUint(v, 10, 8)
		if err != nil || a.DataKeys[byte(n)] == nil {
			return errors.New("config: DATA_KEY_ACTIVE must be the id of a key in DATA_KEYS")
		}
		a.DataKeyActive = byte(n)
	}
	return nil
}

// checkGatewayURL requires https for third-party gateways; plain http is allowed only for local or in-cluster hosts
// (names without a dot, localhost, loopback), which is how the Docker Compose mocks are reached.
func checkGatewayURL(name, raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("config: %s is not a valid URL", name)
	}
	if u.Scheme == "https" {
		return nil
	}
	host := u.Hostname()
	if u.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || !strings.Contains(host, ".")) {
		return nil
	}
	return fmt.Errorf("config: %s must use https (plain http is only allowed for local hosts)", name)
}
