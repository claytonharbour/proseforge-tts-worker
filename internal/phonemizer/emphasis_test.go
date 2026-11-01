package phonemizer

import (
	"strings"
	"testing"
)

func countMarkers(s string) int {
	return strings.Count(s, emphasisMarker)
}

func TestInjectEmphasis(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantMin  int    // minimum markers expected
		wantMax  int    // maximum markers expected
		mustHave string // a substring that must appear (a specific beat), optional
	}{
		{
			// Short sentence: no room for drama, left untouched.
			name:    "short sentence untouched",
			in:      "She stared at it.",
			wantMin: 0, wantMax: 0,
		},
		{
			// Function→content boundary: "the"→"device" is a canonical Gemini beat.
			name:    "function to content beat",
			in:      "She became aware of the device in a way she had not been.",
			wantMin: 1, wantMax: 2,
			mustHave: "the … device",
		},
		{
			// Existing punctuation already induces a pause — don't double it.
			name:    "skips existing punctuation",
			in:      "The device, small and quiet, recorded everything she did.",
			wantMin: 0, wantMax: 2,
		},
		{
			// Numbers must not anchor a beat (normalizer expands them).
			name:    "skips digits",
			in:      "Forty million people wore one and it was unremarkable today.",
			wantMin: 0, wantMax: 2,
		},
		{
			// Rate-limit: a long run of function→content boundaries must not stammer.
			name:    "rate limited cadence",
			in:      "The hum in the bone behind her ear had changed the pitch of the sound she knew.",
			wantMin: 1, wantMax: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := InjectEmphasis(tc.in)
			n := countMarkers(got)
			if n < tc.wantMin || n > tc.wantMax {
				t.Errorf("marker count = %d, want [%d,%d]\n  in:  %q\n  out: %q",
					n, tc.wantMin, tc.wantMax, tc.in, got)
			}
			if tc.mustHave != "" && !strings.Contains(got, tc.mustHave) {
				t.Errorf("missing expected beat %q\n  out: %q", tc.mustHave, got)
			}
			// The core text (markers stripped) must preserve every original word.
			stripped := strings.Join(strings.Fields(strings.ReplaceAll(got, emphasisMarker, "")), " ")
			if stripped != strings.Join(strings.Fields(tc.in), " ") {
				t.Errorf("word content changed\n  want: %q\n  got:  %q", tc.in, stripped)
			}
		})
	}
}

func TestInjectEmphasisDeterministic(t *testing.T) {
	in := "She became aware of the device in a way she had not been since the first week."
	a := InjectEmphasis(in)
	b := InjectEmphasis(in)
	if a != b {
		t.Errorf("non-deterministic:\n  a: %q\n  b: %q", a, b)
	}
}
