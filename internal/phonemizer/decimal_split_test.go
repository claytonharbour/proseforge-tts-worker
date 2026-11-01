//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// #59, found by @wayland once DiagnoseText started reporting the window
// synthesis actually uses. SplitSentences cut numbers in half at the decimal
// point, so the worker narrated the halves as separate sentences:
//
//	"It cost $2.8 million."         → ["It cost $2."  "8 million."]
//	"His BAC was 0.31 at the time." → ["His BAC was 0."  "31 at the time."]
//
// 88 occurrences across 14 books. Pre-existing — it reached audio long before
// the diagnose fix; the old whole-text tokenizer glued "$2.8" back together
// into "dollars.eight", so the instrument showed a different wrong answer than
// the one shipping.
func TestSplitSentencesKeepsDecimalsWhole(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"It cost $2.8 million.", 1},
		{"Purity was 0.001% exactly.", 1},
		{"His BAC was 0.31 at the time.", 1},
		{"The rate fell 2.5 percent.", 1},
		{"He held 3,000 litres.", 1},          // commas never split
		{"Dr. Hanson arrived.", 1},            // the abbreviation guard still holds
		{"It cost $2.8 million. He paid.", 2}, // a real boundary after a decimal
		{"Version 1.2.3 shipped.", 1},         // multiple dots inside one token
	}

	for _, c := range cases {
		got := SplitSentences(c.in)
		if len(got) != c.want {
			t.Errorf("SplitSentences(%q)\n  = %q  (%d parts)\n  want %d — a period between "+
				"two digits is structure inside a number, not the end of a sentence (#59)",
				c.in, got, len(got), c.want)
		}
	}
}

// The guard must not swallow a genuine sentence break, which is the direction
// that would be silent: too few splits means two sentences narrated as one.
func TestDecimalGuardDoesNotSwallowSentenceBreaks(t *testing.T) {
	got := SplitSentences("He paid 2.5 percent. She paid more. Then it rose 0.31 points.")
	if len(got) != 3 {
		t.Fatalf("got %d parts %q, want 3 — the decimal guard must only suppress a break "+
			"BETWEEN DIGITS, never one that follows a word", len(got), got)
	}
	if !strings.Contains(got[0], "2.5") || !strings.Contains(got[2], "0.31") {
		t.Errorf("decimals did not survive intact: %q", got)
	}
}

// A period at the very start or end has no neighbour on one side; the bounds
// check is what stops that indexing out of range.
func TestDecimalGuardHandlesEdges(t *testing.T) {
	for _, in := range []string{".", "5.", ".5", "", "a.b"} {
		_ = SplitSentences(in) // must not panic
	}
	if got := SplitSentences("5."); len(got) != 1 {
		t.Errorf("SplitSentences(%q) = %q, want 1 part", "5.", got)
	}
}

// The whole point is what the worker narrates, so assert down the synthesis
// path rather than only on the splitter.
//
// 🛑 The first version of this counted TokenizeText's groups and could not
// fail: TokenizeText regroups adjacent sentences that fit under the token
// limit, so a split decimal was reassembled into one group and the count was
// 1 either way. It passed with the bug deliberately reintroduced — a test over
// the wrong unit, which is the failure this lane spent the day cataloguing.
//
// The unit that discriminates is the NORMALIZED TEXT of each piece synthesis
// phonemizes. Split, "0.31" becomes "zero" in one sentence and "thirty one" in
// the next, and the word "point" never appears anywhere.
func TestDecimalSurvivesToSynthesis(t *testing.T) {
	const in = "His BAC was 0.31 at the time."

	var spoken []string
	for _, sent := range SplitSentences(in) {
		spoken = append(spoken, NormalizeText(sent))
	}
	got := strings.Join(spoken, " ")

	if !strings.Contains(got, "point three one") {
		t.Errorf("synthesis narrates %q as %q\nwant it to contain %q — a decimal cut at the "+
			"period is read as two unrelated numbers with a sentence pause between them (#59)",
			in, got, "point three one")
	}
	// Direct anchor on the defect's own output, so a future change that produces
	// some third wrong rendering does not pass by merely avoiding this one.
	if strings.Contains(got, "thirty one") {
		t.Errorf("%q → %q: the fractional part is being read as a whole number, which is "+
			"what the split produced", in, got)
	}
}
