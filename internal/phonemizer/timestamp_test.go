//go:build unit

package phonemizer

import (
	"regexp"
	"strings"
	"testing"
)

// digitColonRe finds a colon between digits — the gluing defect. A prose colon
// is ordinary punctuation and must not be flagged.
var digitColonRe = regexp.MustCompile(`\d:\d`)

// The four occurrences of 02:17:33 in The Keeper's Door — three prose, one in
// the fenced log line, across sections 5 and 10. Smiley pulled the passage and
// the finding was that this is a MOTIF, not a log format: s10 is the payoff.
//
// 🛑 Identical rendering is the load-bearing property. A listener who hears one
// thing in s5 and another in s10 cannot connect them, and connecting them is
// the point. This test would fail if a future rule made the fenced occurrence
// diverge from the prose ones — which is exactly what a log-format-aware rule
// would do.
func TestKeepersDoorTimestampRendersIdenticallyEverywhere(t *testing.T) {
	occurrences := []string{
		"She found it at timestamp 02:17:33.",
		"02:17:33.847 [WARN] source=unknown event_type=unclassified",
		"the entry at 02:17:33 sat quietly, already closed",
		"At 02:17:33, a single entry: gobble. Source unknown.",
	}
	const want = "oh two seventeen thirty three"

	for _, s := range occurrences {
		got := NormalizeText(s)
		if !strings.Contains(got, want) {
			t.Errorf("occurrence %q\n  got:  %q\n  want it to contain: %q", s, got, want)
		}
		// Only a colon BETWEEN DIGITS is the gluing defect. Prose colons are
		// ordinary punctuation — "a single entry: gobble" is a sentence, and an
		// assertion that banned every colon flagged it, which was the test being
		// wrong rather than the code.
		if digitColonRe.MatchString(got) {
			t.Errorf("occurrence %q left a colon between digits, which glues two words "+
				"into one token: %q", s, got)
		}
	}
}

// The milliseconds appear only in the fenced occurrence. They must be read, not
// dropped — losing data from a log line is the defect class this whole lane
// spent a night on — but the shared prefix has to land first so the match with
// the prose occurrences still registers.
func TestTimestampMillisecondsAreSpokenAfterTheSharedPrefix(t *testing.T) {
	got := NormalizeText("02:17:33.847 [WARN] source=unknown")
	prefix := strings.Index(got, "oh two seventeen thirty three")
	ms := strings.Index(got, "point eight four seven")
	if prefix < 0 || ms < 0 {
		t.Fatalf("expected both the timestamp and its milliseconds, got %q", got)
	}
	if ms < prefix {
		t.Errorf("milliseconds precede the shared prefix in %q — the match with the "+
			"prose occurrences would not register first", got)
	}
}

// The two-part rule must be untouched: 10:30 is a clock time, not a timestamp,
// and reading it as digits would be wrong in ordinary prose.
func TestOrdinaryClockTimesAreUnchanged(t *testing.T) {
	for in, want := range map[string]string{
		"He arrived at 10:30.":    "ten thirty",
		"The meeting is at 9:00.": "nine o'clock",
		"She left at 8:05.":       "eight oh five",
	} {
		if got := NormalizeText(in); !strings.Contains(got, want) {
			t.Errorf("NormalizeText(%q) = %q, want it to contain %q — the three-part "+
				"timestamp rule may be matching two-part clock times", in, got, want)
		}
	}
}
