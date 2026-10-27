// Package app builds the running services from configuration. Both cmd/api and cmd/worker use it, so the wiring that
// production runs is the wiring the tests exercise.
package app

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/valkey-io/valkey-go"

	"github.com/itsivali/lottery-platform/internal/alerts"
	"github.com/itsivali/lottery-platform/internal/api"
	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/draws/drand"
	"github.com/itsivali/lottery-platform/internal/finance"
	"github.com/itsivali/lottery-platform/internal/game"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/identity/attest"
	"github.com/itsivali/lottery-platform/internal/kyc"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/payments"
	"github.com/itsivali/lottery-platform/internal/payments/daraja"
	"github.com/itsivali/lottery-platform/internal/payouts"
	"github.com/itsivali/lottery-platform/internal/platform/challenge"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/config"
	"github.com/itsivali/lottery-platform/internal/platform/crypto"
	"github.com/itsivali/lottery-platform/internal/platform/db"
	"github.com/itsivali/lottery-platform/internal/platform/idempotency"
	"github.com/itsivali/lottery-platform/internal/platform/ratelimit"
	"github.com/itsivali/lottery-platform/internal/rg"
	"github.com/itsivali/lottery-platform/internal/staff"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

const (
	issuer   = "lottery-platform"
	audience = "lottery-api"
)

// App is the assembled application.
type App struct {
	// Mount registers the /v1 routes; it is nil when the API is not configured (health probes only).
	Mount func(chi.Router)

	cfg       config.Config
	pool      *pgxpool.Pool
	staffPool *pgxpool.Pool
	valkey    valkey.Client
	kyc       *kyc.Service
	payments  *payments.Service
	game      *game.Service
	draws     *draws.Service
	payouts   *payouts.Service
	live      *payments.Live
	stopLive  context.CancelFunc
	workers   *river.Client[pgx.Tx]
}

// New builds the application. With the API unconfigured it returns an App that serves nothing beyond the probes.
func New(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) (*App, error) {
	a := &App{cfg: cfg, pool: pool}
	if !cfg.API.Enabled {
		return a, nil
	}
	c := cfg.API
	hc := &http.Client{Timeout: 20 * time.Second}

	vk, err := newValkey(c.ValkeyAddr)
	if err != nil {
		return nil, fmt.Errorf("app: valkey: %w", err)
	}
	a.valkey = vk

	keyring, err := crypto.NewKeyring(c.DataKeyActive, c.DataKeys)
	if err != nil {
		return nil, err
	}
	aw := audit.NewWriter()
	clk := clock.Postgres{}

	// Anything not configured fails closed instead of pretending to work.
	// No registry by default: identities are reviewed by staff (ADR 0009). A registry gateway can still be plugged in.
	var provider kyc.Provider = kyc.ManualProvider{}
	if c.KYCURL != "" {
		provider = kyc.NewHTTPProvider(c.KYCURL, c.KYCKey, hc)
	}

	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: db.RiverSchema})
	if err != nil {
		return nil, err
	}
	attestor, err := newAttestor(c.Attestation, hc)
	if err != nil {
		return nil, err
	}
	sh := shared{cfg: cfg, clk: clk, hc: hc, keyring: keyring, aw: aw, provider: provider, insertOnly: insertOnly, attestor: attestor}
	if p := cfg.Payments; p.Enabled {
		if sh.daraja, err = NewDaraja(p); err != nil {
			return nil, err
		}
	}
	svc := buildServices(pool, sh)
	idSvc, rgSvc := svc.identity, svc.rg
	a.kyc, a.payments, a.game, a.payouts, a.draws = svc.kyc, svc.payments, svc.game, svc.payouts, svc.draws

	staffAuth, staffSvc, err := a.staff(ctx, sh, svc)
	if err != nil {
		return nil, err
	}

	// One connection per instance listens for payment changes and fans them out to the players' streams.
	listenCfg := pool.Config().ConnConfig.Copy()
	a.live = payments.NewLive(func(ctx context.Context) (*pgx.Conn, error) { return pgx.ConnectConfig(ctx, listenCfg) }, payments.LiveConfig{})
	liveCtx, stopLive := context.WithCancel(context.WithoutCancel(ctx))
	a.stopLive = stopLive
	go a.live.Run(liveCtx)

	a.Mount = api.Mount(api.Deps{
		Identity: idSvc, KYC: a.kyc, RG: rgSvc, Payments: a.payments, Game: a.game, Draws: a.draws, Payouts: a.payouts, Live: a.live, CallbackPath: cfg.Payments.CallbackPath,
		Idem:               idempotency.NewStore(pool),
		Limiter:            ratelimit.New(vk),
		TrustedProxyHeader: c.TrustedProxyHeader, TrustedProxyHops: c.TrustedProxyHops,
		Wallet:          wallet.NewReader(pool),
		CORSOrigins:     c.CORSOrigins,
		Staff:           staffAuth,
		StaffServices:   staffSvc,
		Challenges:      challenge.NewStore(vk, 5*time.Minute),
		AttestationMode: c.Attestation.Mode,
		AttestationWeb:  c.Attestation.Web,
	})
	slog.InfoContext(ctx, "v1 API enabled", "trusted_proxy_header", c.TrustedProxyHeader, "trusted_proxy_hops", c.TrustedProxyHops,
		"kyc_registry", c.KYCURL != "", "google", c.GoogleClientID != "", "apple", c.AppleClientID != "",
		"payments", cfg.Payments.Enabled, "buy_goods", cfg.Payments.TillNumber != "", "paybill", cfg.Payments.C2BEnabled(), "payouts", cfg.Payments.B2CEnabled(), "daraja", cfg.Payments.BaseURL, "cors_origins", len(c.CORSOrigins), "staff", c.Staff.Enabled, "staff_pool", c.Staff.DatabaseURL != "",
		"attestation", c.Attestation.Mode, "app_attest", c.Attestation.AppID != "", "play_integrity", c.Attestation.PlayPackage != "")
	return a, nil
}

// shared is what every set of services is built from.
type shared struct {
	cfg        config.Config
	clk        clock.Clock
	hc         *http.Client
	keyring    *crypto.Keyring
	aw         *audit.Writer
	provider   kyc.Provider
	insertOnly *river.Client[pgx.Tx]
	daraja     *daraja.Client // nil without Daraja configuration
	attestor   identity.Attestor
}

type services struct {
	identity *identity.Service
	kyc      *kyc.Service
	rg       *rg.Service
	payments *payments.Service
	game     *game.Service
	payouts  *payouts.Service
	draws    *draws.Service
}

// buildServices builds the domain services on pool. Anything not configured fails closed instead of pretending to work.
func buildServices(pool *pgxpool.Pool, sh shared) services {
	c, cfg := sh.cfg.API, sh.cfg
	var s services
	s.kyc = kyc.NewService(pool, sh.clk, sh.provider, sh.keyring, c.IdentityKey, sh.aw, kyc.NewRiverEnqueuer(sh.insertOnly, 0))
	s.identity = identity.NewService(pool, sh.clk, oidcVerifier(clock.Wall{}, sh.hc, c), sh.aw, identity.Config{
		AuthKey: c.AuthKey, TokenKey: ed25519.NewKeyFromSeed(c.TokenSeed), Issuer: issuer, Audience: audience,
		AccessTTL: 10 * time.Minute, RefreshTTL: 30 * 24 * time.Hour, DeviceCap: 2,
	}).WithAttestor(sh.attestor)
	if c.Attestation.Mode == "off" {
		s.identity.WithAttestationOff()
	}
	s.rg = rg.NewService(pool, sh.clk, sh.aw, rg.Config{CoolingOff: 24 * time.Hour})

	// Without Daraja configuration the payments service still answers reads but refuses top-ups (fail closed).
	var dc payments.Daraja
	if sh.daraja != nil {
		dc = sh.daraja
	}
	ls := ledger.NewStore()
	s.payments = payments.NewService(pool, sh.clk, dc, payments.NewRiverEnqueuer(sh.insertOnly), s.identity, s.rg, wallet.NewService(ls), ls, sh.aw,
		payments.Config{MinTopUpKES: cfg.Payments.MinTopUpKES})
	if p := cfg.Payments; p.C2BEnabled() {
		s.payments.WithC2B(sh.daraja, payments.C2BConfig{ShortCode: p.C2BShortCode})
	}
	s.game = game.NewService(pool, sh.clk, s.identity, s.rg, wallet.NewService(ls), sh.keyring, wallet.ErrInsufficientFunds)

	// Payouts need B2C; without it withdrawals are refused (fail closed).
	var pd payouts.Daraja
	if cfg.Payments.B2CEnabled() {
		pd = sh.daraja
	}
	s.payouts = payouts.NewService(pool, sh.clk, pd, payouts.NewRiverEnqueuer(sh.insertOnly), s.identity, wallet.NewService(ls), ls, sh.aw, payouts.Config{},
		wallet.ErrInsufficientFunds)

	// Draws use the League of Entropy's quicknet, verified against the pinned key, through its public relays.
	s.draws = draws.NewService(pool, sh.clk, sh.keyring, drand.New(drand.Quicknet, drand.DefaultRelays, sh.hc), ls)
	return s
}

// staff builds the back office's sign-in and the services it acts through. Those run on STAFF_DATABASE_URL (the
// worker's role: the API's role may not change draws or payouts); without it, in single-role mode, they share the
// API's pool. Without staff sign-in configured the /v1/staff routes answer 503.
func (a *App) staff(ctx context.Context, sh shared, base services) (staff.Authenticator, api.StaffServices, error) {
	c := sh.cfg.API.Staff
	if !c.Enabled {
		return nil, api.StaffServices{}, nil
	}
	pool, svc := a.pool, base
	if c.DatabaseURL != "" {
		p, err := db.Open(ctx, c.DatabaseURL)
		if err != nil {
			return nil, api.StaffServices{}, fmt.Errorf("app: staff database: %w", err)
		}
		a.staffPool, pool, svc = p, p, buildServices(p, sh)
	}
	members := staff.NewService(pool, sh.clk, identity.NewOIDCVerifier(clock.Wall{}, sh.hc, staffProviders(c)), sh.aw)
	if c.BootstrapAdmin != "" {
		if err := members.Bootstrap(ctx, c.BootstrapAdmin); err != nil {
			return nil, api.StaffServices{}, fmt.Errorf("app: staff bootstrap admin: %w", err)
		}
	}
	return members, api.StaffServices{
		Identity: svc.identity, KYC: svc.kyc, Payments: svc.payments, Draws: svc.draws, Payouts: svc.payouts, Game: svc.game,
		Alerts: a.alertsOn(pool, svc.draws), Members: members, Finance: finance.NewService(pool, sh.clk),
	}, nil
}

// staffProviders are the ID-token issuers staff may sign in with (ADR 0018): the back office's own Google and Apple
// clients, and the local staff realm in development (its password grant carries no nonce).
func staffProviders(c config.Staff) map[string]identity.OIDCProvider {
	p := map[string]identity.OIDCProvider{}
	if c.GoogleClientID != "" {
		p["google"] = identity.OIDCProvider{Issuers: []string{"https://accounts.google.com", "accounts.google.com"},
			Audiences: []string{c.GoogleClientID}, JWKSURL: "https://www.googleapis.com/oauth2/v3/certs"}
	}
	if c.AppleClientID != "" {
		p["apple"] = identity.OIDCProvider{Issuers: []string{"https://appleid.apple.com"}, Audiences: []string{c.AppleClientID},
			JWKSURL: "https://appleid.apple.com/auth/keys", NonceSHA256: true}
	}
	if c.Dev != nil {
		p["dev"] = identity.OIDCProvider{Issuers: []string{c.Dev.Issuer}, Audiences: c.Dev.ClientIDs, JWKSURL: c.Dev.JWKSURL, NonceOptional: true}
	}
	return p
}

// newAttestor builds the device attestation verifiers that are configured (App Attest trusts Apple's root; Play
// Integrity decodes through Google). A platform without one never passes.
func newAttestor(c config.Attestation, hc *http.Client) (identity.Attestor, error) {
	now := func() time.Time { t, _ := clock.Wall{}.Now(context.Background(), nil); return t } // certificate and verdict ages
	var r attest.Router
	if c.AppID != "" {
		r.IOS = attest.NewAppAttest(c.AppID, attest.AppleAppAttestationRoot(), c.AllowDevelopment, now)
	}
	if c.PlayPackage != "" {
		d, err := attest.NewGoogleDecoder(c.PlayServiceAccount, c.PlayPackage, "https://playintegrity.googleapis.com", hc, now)
		if err != nil {
			return nil, err
		}
		r.Android = attest.NewPlayIntegrity(c.PlayPackage, c.PlayCertDigests, d, now)
	}
	if r.IOS == nil && r.Android == nil {
		return identity.UnavailableAttestor{}, nil
	}
	return r, nil
}

// NewDaraja builds the Daraja client from configuration, with the paybill calls when C2B is configured and the payout
// calls when B2C is. lotteryctl
// uses it too, to register the paybill URLs.
func NewDaraja(p config.Payments) (*daraja.Client, error) {
	client, err := daraja.New(daraja.Config{
		BaseURL: p.BaseURL, ConsumerKey: p.ConsumerKey, ConsumerSecret: p.ConsumerSecret, ShortCode: p.ShortCode,
		Passkey: p.Passkey, TillNumber: p.TillNumber, CallbackURL: p.CallbackURL(), AllowProduction: p.AllowProduction,
	}, &http.Client{Timeout: 35 * time.Second}, clock.Wall{})
	if err != nil {
		return nil, err
	}
	if p.C2BEnabled() {
		if client, err = client.WithC2B(daraja.C2BConfig{ShortCode: p.C2BShortCode, Initiator: p.Initiator, SecurityCredential: p.SecurityCredential,
			ResultURL: p.C2BURL("status-result"), TimeoutURL: p.C2BURL("status-timeout")}); err != nil {
			return nil, err
		}
	}
	if p.B2CEnabled() {
		if client, err = client.WithB2C(daraja.B2CConfig{ShortCode: p.B2CShortCode, Initiator: p.Initiator, SecurityCredential: p.SecurityCredential,
			ResultURL: p.B2CURL("result"), TimeoutURL: p.B2CURL("timeout"), StatusResultURL: p.B2CURL("status-result"),
			StatusTimeoutURL: p.B2CURL("status-timeout")}); err != nil {
			return nil, err
		}
	}
	return client, nil
}

// StartWorkers starts the background job workers (a no-op when the API is not configured).
func (a *App) StartWorkers(ctx context.Context) error {
	if a.kyc == nil {
		return nil
	}
	workers := river.NewWorkers()
	// the worker side needs the verification service without the enqueuer; it shares provider, keys and pool
	kyc.AddWorkers(workers, a.kyc)
	payments.AddWorkers(workers, a.payments)
	game.AddWorkers(workers, a.game)
	draws.AddWorkers(workers, a.draws)
	payouts.AddWorkers(workers, a.payouts)
	alerts.AddWorkers(workers, a.alerts())
	wc, err := river.NewClient(riverpgxv5.New(a.pool), &river.Config{
		Schema: db.RiverSchema, Workers: workers,
		PeriodicJobs: slices.Concat(payments.PeriodicJobs(), game.PeriodicJobs(), draws.PeriodicJobs(), payouts.PeriodicJobs(), alerts.PeriodicJobs()),
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}},
	})
	if err != nil {
		return err
	}
	if err := wc.Start(ctx); err != nil {
		return err
	}
	a.workers = wc
	return nil
}

// ReconciliationStaleAfter is how long after its last run a daily reconciliation counts as not running.
const ReconciliationStaleAfter = 26 * time.Hour

// alerts builds the alert watch: late draws, failed or missing reconciliations (a failed one freezes payouts), and
// withheld tax nearing its remittance deadline, sent to ALERT_WEBHOOK_URL (else logged) with ALERT_HEARTBEAT_URL pinged.
func (a *App) alerts() *alerts.Service { return a.alertsOn(a.pool, a.draws) }

func (a *App) alertsOn(pool *pgxpool.Pool, ds *draws.Service) *alerts.Service {
	hc := &http.Client{Timeout: 10 * time.Second}
	clk := clock.Postgres{}
	var n alerts.Notifier
	if u := a.cfg.Alerts.WebhookURL; u != "" {
		n = alerts.Webhook{URL: u, Client: hc}
	}
	cfg := alerts.Config{}
	if u := a.cfg.Alerts.HeartbeatURL; u != "" {
		cfg.Heartbeat = alerts.Heartbeat(u, hc)
	}
	return alerts.NewService(pool, clk, n, audit.NewWriter(), cfg,
		alerts.DrawsOverdue(ds.Overdue),
		alerts.ReconciliationCheck(pool, clk, ReconciliationStaleAfter),
		alerts.RemittanceCheck(pool, clk, a.cfg.Alerts.KRARemitWarnAfter, a.cfg.Alerts.KRARemitDueAfter))
}

// StopStreams ends every open payment stream and refuses new ones. The server calls it when shutdown begins, since
// a stream would otherwise hold the shutdown until its client went away.
func (a *App) StopStreams() {
	if a.live != nil {
		a.live.Close()
	}
}

// Close stops the workers and releases clients.
func (a *App) Close(ctx context.Context) {
	a.StopStreams()
	if a.stopLive != nil {
		a.stopLive()
	}
	if a.workers != nil {
		_ = a.workers.Stop(ctx)
	}
	if a.valkey != nil {
		a.valkey.Close()
	}
	if a.staffPool != nil {
		a.staffPool.Close()
	}
}

func newValkey(addr string) (valkey.Client, error) {
	opt := valkey.ClientOption{InitAddress: []string{addr}, DisableCache: true}
	if strings.Contains(addr, "://") {
		parsed, err := valkey.ParseURL(addr)
		if err != nil {
			return nil, err
		}
		parsed.DisableCache = true
		opt = parsed
	}
	return valkey.NewClient(opt)
}

// refusingOIDC rejects every token: SSO stays off until a client id is configured.
type refusingOIDC struct{}

func (refusingOIDC) VerifyIDToken(context.Context, string, string, string) (string, error) {
	return "", errors.New("sso is not configured")
}

func oidcVerifier(clk clock.Clock, hc *http.Client, c config.API) identity.IDTokenVerifier {
	providers := map[string]identity.OIDCProvider{}
	if c.GoogleClientID != "" {
		providers["google"] = identity.OIDCProvider{
			Issuers: []string{"https://accounts.google.com", "accounts.google.com"}, Audiences: []string{c.GoogleClientID},
			JWKSURL: "https://www.googleapis.com/oauth2/v3/certs",
		}
	}
	if c.AppleClientID != "" {
		providers["apple"] = identity.OIDCProvider{
			Issuers: []string{"https://appleid.apple.com"}, Audiences: []string{c.AppleClientID},
			JWKSURL: "https://appleid.apple.com/auth/keys", NonceSHA256: true,
		}
	}
	if len(providers) == 0 {
		return refusingOIDC{}
	}
	return identity.NewOIDCVerifier(clk, hc, providers)
}
