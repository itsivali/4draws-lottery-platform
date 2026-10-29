// Package api assembles the public HTTP API: handlers over the identity, kyc and rg services, with authentication,
// idempotency and rate limiting applied uniformly. Modules stay free of HTTP; this package is the only place that
// knows routes and JSON shapes (api/openapi.yaml is the contract, and a test keeps the two in step).
package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/game"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/kyc"
	"github.com/itsivali/lottery-platform/internal/payments"
	"github.com/itsivali/lottery-platform/internal/payouts"
	"github.com/itsivali/lottery-platform/internal/platform/challenge"
	"github.com/itsivali/lottery-platform/internal/platform/httpx"
	"github.com/itsivali/lottery-platform/internal/platform/idempotency"
	"github.com/itsivali/lottery-platform/internal/platform/ratelimit"
	"github.com/itsivali/lottery-platform/internal/rg"
	"github.com/itsivali/lottery-platform/internal/staff"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

// Deps are the collaborators the API needs.
type Deps struct {
	Identity *identity.Service
	KYC      *kyc.Service
	RG       *rg.Service
	// Payments is nil when Daraja is not configured: the routes then answer 503.
	Payments *payments.Service
	// Game sells plays; nil answers 503.
	Game *game.Service
	// Draws publishes draws and their results; nil answers 503.
	Draws *draws.Service
	// Payouts sends withdrawals through M-Pesa B2C; nil answers 503.
	Payouts *payouts.Service
	// Staff authenticates back-office sessions (opened with a password, Google or Apple; ADR 0018, 0019); nil closes the /v1/staff routes (503).
	Staff staff.Authenticator
	// StaffServices are what the back office acts through (on the staff database pool).
	StaffServices StaffServices
	// Wallet reads players' balances and transactions; nil answers 503.
	Wallet *wallet.Reader
	// Live streams payment changes to players; nil disables GET /v1/payments/events (503).
	Live *payments.Live
	// CallbackPath is the secret segment of the STK callback URL and of the paybill hook URLs.
	CallbackPath string
	Idem         *idempotency.Store
	Limiter      *ratelimit.Limiter
	// TrustedProxyHeader is the header our load balancer sets with the client address ("" = use the socket address).
	TrustedProxyHeader string
	// TrustedProxyHops is how many trusted proxies appended to X-Forwarded-For (ignored for other headers).
	TrustedProxyHops int
	// CORSOrigins are the browser origins allowed to call /v1 (the player web app); empty turns CORS off.
	CORSOrigins []string
	// Challenges issues the single-use challenges device attestations are made for; nil answers 503.
	Challenges *challenge.Store
	// AttestationMode is ATTESTATION_MODE: off, record, refuse_failed or require ("" behaves as record).
	AttestationMode string
	// AttestationWeb is ATTESTATION_WEB: allow (default) or refuse web sessions on money routes under the strict modes.
	AttestationWeb string
	// SignIn is the players' Google and Apple sign-in (ADR 0020): which to offer, with which client ids.
	SignIn SignInProviders
}

type ctxKey struct{}

func principalFrom(ctx context.Context) (identity.Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(identity.Principal)
	return p, ok
}

// Mount registers the /v1 routes on r.
func Mount(d Deps) func(chi.Router) {
	h := &handlers{Deps: d}
	return func(r chi.Router) {
		// Daraja's callback: outside /v1 because it carries no token and no Idempotency-Key (it is authenticated by
		// its secret path and deduplicated by the payments module).
		r.Post("/hooks/mpesa/stk/{path}", h.stkCallback)
		// Paybill hooks, under the same secret segment (C2B registration refuses URLs containing "mpesa").
		r.Post("/hooks/c2b/{path}/validate", h.c2bValidate)
		r.Post("/hooks/c2b/{path}/confirm", h.c2bConfirm)
		r.Post("/hooks/c2b/{path}/status-result", h.c2bStatusResult)
		r.Post("/hooks/c2b/{path}/status-timeout", h.c2bStatusTimeout)
		// Payout (B2C) hooks: payment results and the Transaction Status answers about payouts.
		r.Post("/hooks/b2c/{path}/result", h.b2cHook(payouts.CallbackResult))
		r.Post("/hooks/b2c/{path}/timeout", h.b2cHook(payouts.CallbackTimeout))
		r.Post("/hooks/b2c/{path}/status-result", h.b2cHook(payouts.CallbackStatusResult))
		r.Post("/hooks/b2c/{path}/status-timeout", h.b2cHook(payouts.CallbackStatusTimeout))
		// Apple's return URL for the Android app's Sign in with Apple: a form post from the browser, outside /v1 (no
		// token, no Idempotency-Key, no CORS); it only redirects back into the app.
		r.Post("/auth/apple/android", h.appleAndroidReturn)

		r.Route("/v1", func(r chi.Router) {
			r.Use(httpx.CORS(d.CORSOrigins))
			r.Use(h.staffAuth)
			r.Use(h.optionalAuth)
			r.Use(idempotency.Middleware(d.Idem, h.idempotencyPrincipal, idempotency.Options{}))

			// public (no token required)
			r.Post("/auth/signup", h.signup)
			r.Post("/auth/login", h.login)
			r.Post("/auth/refresh", h.refresh)
			r.Post("/auth/attestation/challenge", h.attestationChallenge)
			r.Get("/auth/providers", h.signInProviders)
			r.Post("/auth/sso/{provider}", h.ssoSignIn)
			r.Post("/auth/sso/{provider}/signup", h.ssoSignup)
			r.Get("/draws", h.listDraws)
			r.Get("/draws/{id}", h.getDraw)
			r.Get("/draws/{id}/certificate", h.drawCertificate)

			// back office: staff tokens only (authenticated by staffAuth), each route behind a role
			r.Route("/staff", h.mountStaff)

			// authenticated
			r.Group(func(r chi.Router) {
				r.Use(requireAuth)
				r.Post("/auth/logout", h.logout)
				r.Post("/auth/password/change", h.changePassword)
				r.Post("/auth/sso/{provider}/link", h.ssoLink)
				r.Get("/kyc", h.kycStatus)
				r.Post("/kyc", h.kycSubmit)
				r.Get("/rg/limits", h.rgLimits)
				r.Put("/rg/limits", h.rgSetLimit)
				r.Post("/rg/timeout", h.rgTimeout)
				r.Post("/rg/self-exclusion", h.rgSelfExclude)
				r.Post("/payments/topups", h.startTopUp)
				r.Get("/payments", h.listPayments)
				r.Get("/payments/events", h.paymentEvents)
				r.Get("/payments/{id}", h.getPayment)
				r.Post("/plays", h.play)
				r.Get("/plays/today", h.playsToday)
				r.Get("/fairness", h.fairness)
				r.Post("/fairness/rotate", h.rotateSeed)
				r.Get("/draws/periods", h.myDrawPeriods)
				r.Get("/draws/{id}/entries/mine", h.myDrawEntries)
				r.Post("/payouts/withdrawals", h.withdraw)
				r.Get("/payouts", h.listPayouts)
				r.Get("/payouts/{id}", h.getPayout)
				r.Get("/wallet", h.getWallet)
				r.Get("/account", h.getAccount)
				r.Post("/account/closure", h.requestClosure)
			})
		})
	}
}

type handlers struct{ Deps }

func (h *handlers) clientIP(r *http.Request) string {
	return httpx.ClientIPHops(r, h.TrustedProxyHeader, h.TrustedProxyHops)
}

// optionalAuth attaches the principal when a valid bearer token is present; protected routes enforce it separately.
func (h *handlers) optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			if p, err := h.Identity.Authenticate(r.Context(), tok); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, p))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := principalFrom(r.Context()); !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			httpx.WriteProblem(w, http.StatusUnauthorized, "unauthenticated", "Authentication required", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// idempotencyPrincipal scopes idempotency keys to the authenticated staff member or user, or to the client address
// before sign-in.
func (h *handlers) idempotencyPrincipal(r *http.Request) string {
	if m, ok := staffFrom(r.Context()); ok {
		return "staff:" + m.ID
	}
	if p, ok := principalFrom(r.Context()); ok {
		return "user:" + p.UserID
	}
	return "anon:" + h.clientIP(r)
}

// limit applies a fail-closed sliding-window limit to a key and writes the problem response when exceeded.
func (h *handlers) limit(w http.ResponseWriter, r *http.Request, key string, n int, window time.Duration) bool {
	d, err := h.Limiter.Allow(r.Context(), key, n, window)
	if err != nil {
		httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "unavailable", "Temporarily unavailable", "", 5)
		return false
	}
	if !d.Allowed {
		secs := int(d.RetryAfter.Seconds()) + 1
		httpx.WriteProblemRetry(w, http.StatusTooManyRequests, "rate-limited", "Too many requests", "", secs)
		return false
	}
	return true
}
