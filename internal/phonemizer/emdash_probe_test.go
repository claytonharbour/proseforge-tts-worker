//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// TestEmDashBlocksEmphasis checks that an existing em-dash suppresses an
// injected "…" beat — Kokoro already pauses at the dash, so a second marker
// would stack pauses (#42 class 6).
//
// Each case carries its own CONTROL: the identical sentence with the dash
// removed MUST gain a beat. Without that, "no beat injected" is equally
// consistent with "this sentence never had a qualifying boundary", and the
// test would pass against an injector that does nothing at all.
func TestEmDashBlocksEmphasis(t *testing.T) {
	cases := []struct{ name, withDash, control string }{
		{"dash attached to the function word",
			"She walked into the room and saw the— device sitting quietly there",
			"She walked into the room and saw the device sitting quietly there"},
		{"dash as its own token",
			"She walked into the room and saw the — device sitting quietly there",
			"She walked into the room and saw the device sitting quietly there"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotDash := InjectEmphasis(c.withDash)
			gotCtl := InjectEmphasis(c.control)

			if strings.Contains(gotCtl, emphasisMarker) == false {
				t.Fatalf("CONTROL injected nothing — this case proves nothing about the dash.\n  control: %s\n  out:     %s", c.control, gotCtl)
			}
			if strings.Contains(gotDash, emphasisMarker) {
				t.Errorf("em-dash did NOT block injection — pauses will stack.\n  in:  %s\n  out: %s", c.withDash, gotDash)
			}
			t.Logf("control gained a beat ✓   dash suppressed it ✓")
		})
	}
}
