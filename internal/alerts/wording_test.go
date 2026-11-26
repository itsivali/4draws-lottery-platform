package alerts_test

import (
	"strings"
	"testing"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/alerts"
)

func TestAges_SaysDurationsTheWayAPersonWould(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0: "0 min", 20 * time.Second: "0 min", 31 * time.Minute: "31 min", time.Hour: "1 h", 90 * time.Minute: "1 h 30 min",
		25 * time.Hour: "25 h", 48 * time.Hour: "2 d", 8*24*time.Hour + 3*time.Hour: "8 d 3 h",
	} {
		if got := alerts.Ages(d); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
}

func TestMaskMSISDN(t *testing.T) {
	m := "254705369005"
	if got := alerts.MaskMSISDN(&m); got != "254705***005" {
		t.Fatalf("%q", got)
	}
	if got := alerts.MaskMSISDN(nil); got != "unknown number" {
		t.Fatalf("%q", got)
	}
}

func TestFirstLine(t *testing.T) {
	if got := alerts.FirstLine("  daraja: refused\nstack"); got != "daraja: refused" {
		t.Fatalf("%q", got)
	}
	if got := alerts.FirstLine(""); got != "see the System page" {
		t.Fatalf("%q", got)
	}
	if got := alerts.FirstLine(strings.Repeat("x", 300)); len([]rune(got)) != 201 {
		t.Fatalf("%d", len(got))
	}
}
