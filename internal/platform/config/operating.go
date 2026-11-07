package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Operating is the operating mode (ADR 0027): the most this process may do with money. The database can only lower
// it (internal/compliance), and makes it live only through two admins' approval naming LicenceReference.
type Operating struct {
	// Mode is OPERATING_MODE: disabled (the default), sandbox or live.
	Mode string
	// LicenceReference is REGULATORY_LICENCE_REFERENCE, the licence a live service operates under. Recording it is
	// not proof of a licence; a live request must name it and carry the evidence.
	LicenceReference string
}

// productionDaraja reports whether p moves real money: Safaricom's production host, or production explicitly allowed.
func productionDaraja(p Payments) bool {
	if !p.Enabled {
		return false
	}
	u, err := url.Parse(p.BaseURL)
	return p.AllowProduction || err != nil || u.Hostname() == "api.safaricom.co.ke"
}

func loadOperating(p Payments) (Operating, error) {
	o := Operating{Mode: strings.TrimSpace(os.Getenv("OPERATING_MODE")), LicenceReference: strings.TrimSpace(os.Getenv("REGULATORY_LICENCE_REFERENCE"))}
	if o.Mode == "" {
		o.Mode = "disabled" // nothing moves until someone says which money this is
	}
	switch o.Mode {
	case "disabled":
		// production credentials may be configured while stopped: nothing moves
	case "sandbox":
		if productionDaraja(p) {
			return Operating{}, errors.New("config: OPERATING_MODE=sandbox with production Daraja; sandbox mode takes no real money (ADR 0027)")
		}
	case "live":
		if !productionDaraja(p) {
			return Operating{}, errors.New("config: OPERATING_MODE=live needs production Daraja (DARAJA_BASE_URL=https://api.safaricom.co.ke, DARAJA_ALLOW_PRODUCTION=true)")
		}
		if o.LicenceReference == "" {
			return Operating{}, errors.New("config: OPERATING_MODE=live needs REGULATORY_LICENCE_REFERENCE, and two admins approving a live request that names it (ADR 0027)")
		}
	default:
		return Operating{}, fmt.Errorf("config: OPERATING_MODE=%q; use disabled, sandbox or live", o.Mode)
	}
	return o, nil
}
