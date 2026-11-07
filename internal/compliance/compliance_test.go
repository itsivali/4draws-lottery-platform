package compliance

import (
	"context"
	"testing"
)

func TestParseMode_KnowsThreeModesAndNothingElse(t *testing.T) {
	for _, s := range []string{"disabled", "sandbox", "live"} {
		if m, err := ParseMode(s); err != nil || string(m) != s {
			t.Errorf("ParseMode(%q) = %q, %v", s, m, err)
		}
	}
	for _, s := range []string{"", "on", "LIVE", "production"} {
		if _, err := ParseMode(s); err == nil {
			t.Errorf("ParseMode(%q) accepted", s)
		}
	}
}

func TestLower_IsTheMoreRestrictiveAndUnknownIsDisabled(t *testing.T) {
	cases := []struct{ a, b, want Mode }{
		{Live, Sandbox, Sandbox}, {Sandbox, Live, Sandbox}, {Live, Live, Live},
		{Sandbox, Disabled, Disabled}, {Disabled, Live, Disabled}, {"bogus", Live, "bogus"},
	}
	for _, c := range cases {
		if got := lower(c.a, c.b); got != c.want {
			t.Errorf("lower(%s, %s) = %s, want %s", c.a, c.b, got, c.want)
		}
	}
	if Mode("bogus").rank() != 0 {
		t.Error("an unknown mode must rank as disabled")
	}
}

func TestNewGate_AnUnknownConfiguredModeIsDisabled(t *testing.T) {
	if g := NewGate(nil, nil, nil, Config{Mode: "on"}); g.Configured() != Disabled {
		t.Fatalf("configured = %s, want disabled", g.Configured())
	}
}

func TestNilGate_AllowsEverything(t *testing.T) {
	var g *Gate
	if err := g.Assert(context.Background(), nil, OpPlay); err != nil {
		t.Fatalf("nil gate refused: %v", err)
	}
}

func TestValidActor_IsAStaffID(t *testing.T) {
	if !validActor("staff:0192f000-0000-7000-8000-000000000001") {
		t.Error("a staff id was refused")
	}
	for _, a := range []string{"", "staff:", "operator:jane", "staff:jane", "0192f000-0000-7000-8000-000000000001"} {
		if validActor(a) {
			t.Errorf("actor %q accepted", a)
		}
	}
}
