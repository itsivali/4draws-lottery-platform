package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/compliance"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/kyc"
	"github.com/itsivali/lottery-platform/internal/platform/httpx"
	"github.com/itsivali/lottery-platform/internal/platform/password"
)

type deviceJSON struct {
	Fingerprint          string `json:"fingerprint"`
	Platform             string `json:"platform"`
	AttestationToken     string `json:"attestation_token"`
	AttestationChallenge string `json:"attestation_challenge"`
}

// device converts the client's device description. The attestation token is only evidence for the server-side
// verifier, and only together with a challenge this server issued, used here once; there is no way for a client to
// state a verdict.
func (h *handlers) device(ctx context.Context, d deviceJSON) identity.Device {
	dev := identity.Device{Fingerprint: d.Fingerprint, Platform: d.Platform, AttestationToken: d.AttestationToken}
	if d.AttestationToken != "" && d.AttestationChallenge != "" && h.Challenges != nil {
		if ok, err := h.Challenges.Consume(ctx, d.AttestationChallenge); err == nil && ok {
			dev.AttestationChallenge = d.AttestationChallenge
		}
	}
	return dev
}

type tokensJSON struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	// true after a sign-in with a temporary password staff gave the player: the app asks for a new one first
	MustChangePassword bool `json:"must_change_password,omitempty"`
}

func writeTokens(w http.ResponseWriter, t identity.Tokens) {
	httpx.WriteJSON(w, http.StatusOK, tokensJSON{AccessToken: t.Access, RefreshToken: t.Refresh, ExpiresAt: t.AccessExpires, MustChangePassword: t.MustChangePassword})
}

// authError maps identity errors to problem responses. Credential failures are deliberately uniform.
func authError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrInvalidMSISDN):
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-msisdn", "Invalid phone number", "use a Kenyan mobile number such as 0712345678")
	case errors.Is(err, password.ErrWeak):
		httpx.WriteProblem(w, http.StatusBadRequest, "weak-password", "Password too weak", strings.TrimPrefix(err.Error(), "password: too weak: "))
	case errors.Is(err, identity.ErrInvalidCredentials):
		httpx.WriteProblem(w, http.StatusUnauthorized, "invalid-credentials", "Invalid phone number or password", "")
	case errors.Is(err, identity.ErrPhoneRegistered):
		httpx.WriteProblem(w, http.StatusConflict, "phone-registered", "This number already has an account", "sign in instead")
	case errors.Is(err, identity.ErrInvalidEmail):
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-email", "Invalid email address", "use an address like name@example.com")
	case errors.Is(err, identity.ErrEmailRegistered):
		httpx.WriteProblem(w, http.StatusConflict, "email-registered", "This email address already has an account", "sign in instead, or use another address")
	case errors.Is(err, identity.ErrResetLinkInvalid):
		httpx.WriteProblem(w, http.StatusGone, "invalid-token", "This link has expired or was already used", "ask for a new one")
	case errors.Is(err, identity.ErrResetUnavailable):
		httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "unavailable", "Password reset by email is not configured", "ask us for a temporary password", 60)
	case errors.Is(err, identity.ErrTokenReuse), errors.Is(err, identity.ErrTokenInvalid):
		httpx.WriteProblem(w, http.StatusUnauthorized, "invalid-token", "Invalid token", "")
	case errors.Is(err, identity.ErrDeviceRequired):
		httpx.WriteProblem(w, http.StatusBadRequest, "device-required", "Device information required", "")
	case errors.Is(err, identity.ErrAttestationFailed):
		httpx.WriteProblem(w, http.StatusForbidden, "attestation-required", "This device could not be verified",
			"install the app from the official store and sign in again; plays, top-ups and withdrawals need a verified device")
	case errors.Is(err, identity.ErrDeviceCap):
		httpx.WriteProblem(w, http.StatusForbidden, "device-limit", "Too many accounts on this device", "")
	case errors.Is(err, identity.ErrAccountSuspended):
		httpx.WriteProblem(w, http.StatusForbidden, "account-suspended", "Account suspended", "")
	case errors.Is(err, identity.ErrSSONotLinked):
		httpx.WriteProblem(w, http.StatusConflict, "sso-not-linked", "No account is linked to this identity",
			"give your phone number to create one, or sign in with your phone number and password")
	case errors.Is(err, identity.ErrSSOAlreadyLinked):
		httpx.WriteProblem(w, http.StatusConflict, "sso-already-linked", "This identity already signs in to an account", "sign in with it instead")
	case errors.Is(err, identity.ErrKYCRequired):
		httpx.WriteProblem(w, http.StatusForbidden, "kyc-required", "Identity verification required", "")
	case errors.Is(err, identity.ErrPhoneNotVerified):
		httpx.WriteProblem(w, http.StatusForbidden, "phone-not-verified", "Phone number not verified yet", "make your first top-up to verify it")
	default:
		internalError(w, err)
	}
}

type credentialsJSON struct {
	MSISDN   string     `json:"msisdn"`
	Email    string     `json:"email"` // sign-up only (ADR 0021)
	Password string     `json:"password"`
	Device   deviceJSON `json:"device"`
	declaredJSON
	ageJSON
}

// ageJSON is every new account's date of birth and 18+ declaration (ADR 0029), checked here, before the account exists.
type ageJSON struct {
	DateOfBirth      string `json:"date_of_birth"`
	AdultDeclaration bool   `json:"adult_declaration"`
}

// declaredJSON is what a new account gives about itself at sign-up (owner, 2026-10-30), verified at the first cash-out.
type declaredJSON struct {
	FirstName  string `json:"first_name"`
	SecondName string `json:"second_name"`
	NationalID string `json:"national_id"`
}

// invalidIdentity answers a declaration that does not look like names and an ID number, at sign-up or later.
func invalidIdentity(w http.ResponseWriter) {
	httpx.WriteProblem(w, http.StatusBadRequest, "invalid-identity", "Check your names and ID number",
		"give your first and second names as on your national ID, and the ID number (7 to 9 digits)")
}

// declaration checks what a new account gave about itself and returns what stores it with the account. Without the
// identity service (a deployment with no KYC) nothing is asked.
func (h *handlers) declaration(w http.ResponseWriter, in declaredJSON) ([]identity.Created, bool) {
	if h.KYC == nil {
		return nil, true
	}
	d, err := kyc.Declaration{FirstName: in.FirstName, SecondName: in.SecondName, IDNumber: in.NationalID}.Normalized()
	if err != nil {
		invalidIdentity(w)
		return nil, false
	}
	return []identity.Created{func(ctx context.Context, tx pgx.Tx, userID string) error { return h.KYC.Declare(ctx, tx, userID, d) }}, true
}

// signupRecords checks what a new account gives about itself (names and ID number, date of birth and the 18+
// declaration) and returns what stores it with the account. It answers the problem itself when something is wrong.
func (h *handlers) signupRecords(w http.ResponseWriter, r *http.Request, d declaredJSON, a ageJSON) ([]identity.Created, bool) {
	// where the sign-up comes from (ADR 0031): a refusal is recorded on its own, an accepted one with the new account
	country := h.country(r)
	if !h.Geo.Allowed(compliance.GeoSignup, country) {
		if err := h.Geo.Record(r.Context(), nil, "", compliance.GeoSignup, country); err != nil {
			internalError(w, err)
			return nil, false
		}
		outsideKenya(w)
		return nil, false
	}
	created, ok := h.declaration(w, d)
	if ok && h.Geo != nil {
		created = append(created, func(ctx context.Context, tx pgx.Tx, userID string) error {
			return h.Geo.Record(ctx, tx, userID, compliance.GeoSignup, country)
		})
	}
	if !ok || h.KYC == nil {
		return created, ok
	}
	dob, err := h.KYC.CheckAge(r.Context(), kyc.AgeDeclaration{DateOfBirth: a.DateOfBirth, Adult: a.AdultDeclaration})
	switch {
	case err == nil:
		return append(created, func(ctx context.Context, tx pgx.Tx, userID string) error {
			return h.KYC.DeclareAge(ctx, tx, userID, dob)
		}), true
	case errors.Is(err, kyc.ErrAgeRequired):
		httpx.WriteProblem(w, http.StatusBadRequest, "age-required", "Give your date of birth and confirm you are 18 or older", "")
	case errors.Is(err, kyc.ErrInvalidInput):
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-date-of-birth", "Check your date of birth", "use YYYY-MM-DD")
	case errors.Is(err, kyc.ErrUnderage):
		httpx.WriteProblem(w, http.StatusForbidden, "underage", "You must be 18 or older to play", "")
	default:
		internalError(w, err)
	}
	return nil, false
}

func (h *handlers) signup(w http.ResponseWriter, r *http.Request) {
	var in credentialsJSON
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	// limits first, keyed on the normalised number so formatting tricks cannot dodge them
	key := in.MSISDN
	if n, err := identity.NormalizeMSISDN(in.MSISDN); err == nil {
		key = n
	}
	if !h.limit(w, r, "signup:ip:"+h.clientIP(r), 10, time.Hour) || !h.limit(w, r, "signup:msisdn:"+key, 5, 24*time.Hour) {
		return
	}
	created, ok := h.signupRecords(w, r, in.declaredJSON, in.ageJSON)
	if !ok {
		return
	}
	t, err := h.Identity.Signup(r.Context(), in.MSISDN, in.Email, in.Password, h.device(r.Context(), in.Device), created...)
	if err != nil {
		authError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, tokensJSON{AccessToken: t.Access, RefreshToken: t.Refresh, ExpiresAt: t.AccessExpires})
}

func (h *handlers) login(w http.ResponseWriter, r *http.Request) {
	var in credentialsJSON
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	key := in.MSISDN
	if n, err := identity.NormalizeMSISDN(in.MSISDN); err == nil {
		key = n
	}
	// the per-number budget is what stops password guessing against one account, the per-IP one stops sweeping many
	if !h.limit(w, r, "login:msisdn:"+key, 10, time.Hour) || !h.limit(w, r, "login:ip:"+h.clientIP(r), 60, time.Hour) {
		return
	}
	t, err := h.Identity.Login(r.Context(), in.MSISDN, in.Password, h.device(r.Context(), in.Device))
	if err != nil {
		authError(w, err)
		return
	}
	writeTokens(w, t)
}

func (h *handlers) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	p, _ := principalFrom(r.Context())
	if !h.limit(w, r, "pwchange:user:"+p.UserID, 10, time.Hour) {
		return
	}
	if err := h.Identity.ChangePassword(r.Context(), p.UserID, p.SessionID, in.Current, in.New); err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			// 403, not 401: the caller's token is fine, so clients must not treat this as an expired session
			httpx.WriteProblem(w, http.StatusForbidden, "wrong-password", "Current password is incorrect", "")
			return
		}
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// forgotPassword emails a reset link to an address that has an active account (ADR 0021). It always answers 202, so
// nobody learns which addresses have accounts; 503 when the api has no web-app address to link to.
func (h *handlers) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !httpx.DecodeJSON(w, r, &in) || !h.limit(w, r, "forgot:ip:"+h.clientIP(r), 10, time.Hour) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-email", "An email address is required", "")
		return
	}
	if !h.limit(w, r, "forgot:email:"+email, 3, time.Hour) {
		return
	}
	if err := h.Identity.RequestPasswordReset(r.Context(), email); err != nil {
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// resetPasswordWithToken sets a new password from an emailed link; every session of the account ends.
func (h *handlers) resetPasswordWithToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"new_password"`
	}
	if !httpx.DecodeJSON(w, r, &in) || !h.limit(w, r, "reset:ip:"+h.clientIP(r), 20, time.Hour) {
		return
	}
	if err := h.Identity.ResetPasswordWithToken(r.Context(), in.Token, in.Password); err != nil {
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken      string `json:"refresh_token"`
		DeviceFingerprint string `json:"device_fingerprint"`
	}
	if !httpx.DecodeJSON(w, r, &in) || !h.limit(w, r, "refresh:ip:"+h.clientIP(r), 60, time.Hour) {
		return
	}
	t, err := h.Identity.Refresh(r.Context(), in.RefreshToken, in.DeviceFingerprint)
	if err != nil {
		authError(w, err)
		return
	}
	writeTokens(w, t)
}

func validProvider(p string) bool { return p == "google" || p == "apple" }

func (h *handlers) ssoSignIn(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if !validProvider(provider) {
		httpx.WriteProblem(w, http.StatusNotFound, "unknown-provider", "Unknown identity provider", "")
		return
	}
	var in struct {
		IDToken string     `json:"id_token"`
		Nonce   string     `json:"nonce"`
		Device  deviceJSON `json:"device"`
	}
	if !httpx.DecodeJSON(w, r, &in) || !h.limit(w, r, "sso:ip:"+h.clientIP(r), 30, time.Hour) {
		return
	}
	t, err := h.Identity.SignInSSO(r.Context(), provider, in.IDToken, in.Nonce, h.device(r.Context(), in.Device))
	if err != nil {
		authError(w, err)
		return
	}
	writeTokens(w, t)
}

// SignInProviders is what the player apps need to offer Google and Apple (ADR 0020): the client ids they request ID
// tokens for, which are the audiences the API accepts. An empty id means the provider is not offered there.
type SignInProviders struct {
	GoogleClientID    string // the web app's OAuth client; the phone apps' server client id
	GoogleIOSClientID string // the iOS app's OAuth client, when it has one
	AppleClientID     string // the Services ID the web app and Android use
	AppleBundleID     string // the iOS app: its native Sign in with Apple tokens name it
	AndroidPackage    string // the Android app, for Apple's return to it
}

type googleProviderJSON struct {
	ClientID    string `json:"client_id"`
	IOSClientID string `json:"ios_client_id,omitempty"`
}

type appleProviderJSON struct {
	// empty when Apple is offered on iOS alone (the app signs in natively, no Services ID involved)
	ClientID string `json:"client_id,omitempty"`
}

type signInProvidersJSON struct {
	Google *googleProviderJSON `json:"google,omitempty"`
	Apple  *appleProviderJSON  `json:"apple,omitempty"`
}

// signInProviders tells the apps which of Google and Apple they may offer, and with which client ids. Nothing here is
// secret: the ids are in every app build and web page that uses them.
func (h *handlers) signInProviders(w http.ResponseWriter, _ *http.Request) {
	var out signInProvidersJSON
	if p := h.SignIn; p.GoogleClientID != "" {
		out.Google = &googleProviderJSON{ClientID: p.GoogleClientID, IOSClientID: p.GoogleIOSClientID}
	}
	if p := h.SignIn; p.AppleClientID != "" || p.AppleBundleID != "" {
		out.Apple = &appleProviderJSON{ClientID: p.AppleClientID}
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ssoSignup creates an account for a Google or Apple identity that has none, from the phone number the player gives
// after the provider's window (ADR 0020). The same budgets as a password sign-up apply.
func (h *handlers) ssoSignup(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if !validProvider(provider) {
		httpx.WriteProblem(w, http.StatusNotFound, "unknown-provider", "Unknown identity provider", "")
		return
	}
	var in struct {
		IDToken string     `json:"id_token"`
		Nonce   string     `json:"nonce"`
		MSISDN  string     `json:"msisdn"`
		Device  deviceJSON `json:"device"`
		declaredJSON
		ageJSON
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	key := in.MSISDN
	if n, err := identity.NormalizeMSISDN(in.MSISDN); err == nil {
		key = n
	}
	if !h.limit(w, r, "signup:ip:"+h.clientIP(r), 10, time.Hour) || !h.limit(w, r, "signup:msisdn:"+key, 5, 24*time.Hour) {
		return
	}
	created, ok := h.signupRecords(w, r, in.declaredJSON, in.ageJSON)
	if !ok {
		return
	}
	t, err := h.Identity.SignupSSO(r.Context(), provider, in.IDToken, in.Nonce, in.MSISDN, h.device(r.Context(), in.Device), created...)
	if err != nil {
		authError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, tokensJSON{AccessToken: t.Access, RefreshToken: t.Refresh, ExpiresAt: t.AccessExpires})
}

// appleAndroidReturn is the return URL Apple posts to when a Sign in with Apple was started by the Android app (which
// opens Apple in the browser, there being no native Apple sign-in on Android). It only hands Apple's answer back to the
// app, through the intent link the sign_in_with_apple plugin registers; the app then sends the token to
// /v1/auth/sso/apple like any other, where it is verified. Nothing is trusted or stored here.
func (h *handlers) appleAndroidReturn(w http.ResponseWriter, r *http.Request) {
	if h.SignIn.AndroidPackage == "" || h.SignIn.AppleClientID == "" {
		httpx.WriteProblem(w, http.StatusNotFound, "not-found", "Not found", "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-form", "Invalid form", "")
		return
	}
	q := url.Values{}
	for _, k := range []string{"code", "id_token", "state", "user", "error"} {
		if v := r.PostForm.Get(k); v != "" {
			q.Set(k, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "intent://callback?"+q.Encode()+"#Intent;package="+h.SignIn.AndroidPackage+";scheme=signinwithapple;end", http.StatusTemporaryRedirect)
}

func (h *handlers) ssoLink(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if !validProvider(provider) {
		httpx.WriteProblem(w, http.StatusNotFound, "unknown-provider", "Unknown identity provider", "")
		return
	}
	var in struct {
		IDToken string `json:"id_token"`
		Nonce   string `json:"nonce"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	p, _ := principalFrom(r.Context())
	if err := h.Identity.LinkSSO(r.Context(), p.UserID, provider, in.IDToken, in.Nonce); err != nil {
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) logout(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.Identity.Logout(r.Context(), p.SessionID); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) attestationChallenge(w http.ResponseWriter, r *http.Request) {
	if !h.limit(w, r, "challenge:ip:"+h.clientIP(r), 30, time.Minute) {
		return
	}
	if h.Challenges == nil {
		httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "attestation-unavailable", "Device attestation is not configured", "", 60)
		return
	}
	c, exp, err := h.Challenges.Issue(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"challenge": c, "expires_at": exp})
}

// deviceTrusted applies ATTESTATION_MODE before plays, top-ups and withdrawals: refuse_failed stops devices that
// failed attestation; require also stops phones that were never attested. Web sessions cannot be attested (and anyone
// can claim to be one): ATTESTATION_WEB decides whether they may transact under those modes.
func (h *handlers) deviceTrusted(w http.ResponseWriter, r *http.Request) bool {
	if h.AttestationMode != "refuse_failed" && h.AttestationMode != "require" {
		return true
	}
	p, _ := principalFrom(r.Context())
	err := h.Identity.DeviceTrusted(r.Context(), p.SessionID, h.AttestationMode == "require", h.AttestationWeb != "refuse")
	if err == nil {
		return true
	}
	authError(w, err)
	return false
}
