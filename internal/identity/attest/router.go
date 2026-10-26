package attest

import (
	"context"
	"errors"
)

// Router sends each platform's evidence to its verifier. A platform without one is undecided (an error), which the
// identity service records as "none", never as a pass.
type Router struct {
	IOS     *AppAttest
	Android *PlayIntegrity
}

// Verify implements identity.Attestor.
func (r Router) Verify(ctx context.Context, platform, token, challenge string) (bool, error) {
	switch {
	case platform == "ios" && r.IOS != nil:
		return r.IOS.Verify(token, challenge)
	case platform == "android" && r.Android != nil:
		return r.Android.Verify(ctx, token, challenge)
	}
	return false, errors.New("attest: no verifier for " + platform)
}
