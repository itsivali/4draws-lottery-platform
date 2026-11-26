package compliance

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/4draws-lottery-platform/internal/compliance/store"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
)

// ErrOutsideKenya: with real money only requests the edge places in Kenya are served (ADR 0031).
var ErrOutsideKenya = errors.New("compliance: this service is only available in Kenya")

// Where a geo signal is taken (compliance.geo_signals.action).
const (
	GeoSignup     = "signup"
	GeoTopUp      = "topup"
	GeoPlay       = "play"
	GeoWithdrawal = "withdrawal"
)

var countryCode = regexp.MustCompile(`^[A-Z0-9]{2}$`)

// Geo records the country the edge reports for sign-ups and money moves, and with real money refuses anything but KE
// (ADR 0031). The country is a risk signal from the client's IP address, not proof of where the player is: a VPN
// changes it, and the phone prefix proves nothing either. A nil Geo records nothing and refuses nothing.
type Geo struct {
	pool    *pgxpool.Pool
	clk     clock.Clock
	enforce bool
}

// NewGeo returns a Geo; enforce refuses every country but KE, an unknown one included (the live policy).
func NewGeo(pool *pgxpool.Pool, clk clock.Clock, enforce bool) *Geo {
	return &Geo{pool: pool, clk: clk, enforce: enforce}
}

// Country normalises what the edge sent: a two-character code (Cloudflare uses XX for unknown and T1 for Tor), or ""
// when it sent nothing usable.
func Country(raw string) string {
	c := strings.ToUpper(strings.TrimSpace(raw))
	if !countryCode.MatchString(c) {
		return ""
	}
	return c
}

// Allowed reports whether action from country may go on. A withdrawal always may: it pays the player's own money to
// their Kenyan M-Pesa number, and refusing it abroad would only keep the money from them.
func (g *Geo) Allowed(action, country string) bool {
	return g == nil || !g.enforce || action == GeoWithdrawal || country == "KE"
}

// Record writes one signal in q (a transaction, to record it with what it is about, or the pool). userID is "" when
// the account does not exist yet.
func (g *Geo) Record(ctx context.Context, q Querier, userID, action, country string) error {
	if g == nil {
		return nil
	}
	if q == nil {
		q = g.pool
	}
	now, err := g.clk.Now(ctx, q)
	if err != nil {
		return err
	}
	var user, c *string
	if userID != "" {
		user = &userID
	}
	if country != "" {
		c = &country
	}
	return store.New(q).InsertGeoSignal(ctx, store.InsertGeoSignalParams{UserID: user, Action: action, Country: c, Refused: !g.Allowed(action, country), At: now})
}

// Check records the signal on its own and returns ErrOutsideKenya when the request may not go on.
func (g *Geo) Check(ctx context.Context, userID, action, country string) error {
	if g == nil {
		return nil
	}
	if err := g.Record(ctx, nil, userID, action, country); err != nil {
		return err
	}
	if !g.Allowed(action, country) {
		return ErrOutsideKenya
	}
	return nil
}
