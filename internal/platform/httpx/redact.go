package httpx

import "strings"

// hookTails are the last segments of the paybill and payout hooks, after the secret.
var hookTails = map[string]bool{"validate": true, "confirm": true, "result": true, "timeout": true, "status-result": true, "status-timeout": true}

// RedactPath returns the path with the M-Pesa hooks' secret segment replaced by {secret}, for traces and logs
// (ADR 0028). The hooks are /hooks/mpesa/stk/{secret} and /hooks/{c2b|b2c}/{secret}/{tail}; any segment of a /hooks/
// path that is not one of those known words is redacted, so a mistyped or probing path never reaches telemetry
// either. Other paths are returned unchanged.
func RedactPath(path string) string {
	rest, ok := strings.CutPrefix(path, "/hooks/")
	if !ok {
		return path
	}
	segs := strings.Split(rest, "/")
	for i, s := range segs {
		keep := false
		switch i {
		case 0:
			keep = s == "mpesa" || s == "c2b" || s == "b2c"
		case 1:
			keep = segs[0] == "mpesa" && s == "stk"
		case 2:
			keep = (segs[0] == "c2b" || segs[0] == "b2c") && hookTails[s]
		}
		if !keep && s != "" {
			segs[i] = "{secret}"
		}
	}
	return "/hooks/" + strings.Join(segs, "/")
}
