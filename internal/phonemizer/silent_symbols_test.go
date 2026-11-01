//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// Class 9 of #42: symbols that produce NO phonemes, so the listener hears
// nothing where the author wrote something. Census by Wayland — 84 occurrences
// across the corpus.
func TestSilentSymbolsAreSpoken(t *testing.T) {
	for in, want := range map[string]string{
		"Smith & Wesson opened the shop.": "Smith and Wesson",
		// "twelve", not "12" — the number normaliser runs too, which is correct.
		"See § 12 for the clause.":      "section twelve",
		"The crowd was ≈ four hundred.": "approximately four hundred",
		"R&D was underfunded.":          "R and D",
	} {
		if got := NormalizeText(in); !strings.Contains(got, want) {
			t.Errorf("NormalizeText(%q)\n  got:  %q\n  want it to contain: %q", in, got, want)
		}
	}
}

// ≈ is one of only two symbols in the class that change MEANING rather than
// merely dropping information: a hedge silently becomes a precise claim. That
// makes it the highest-value member and worth pinning on its own.
func TestApproximatelyHedgeSurvives(t *testing.T) {
	got := NormalizeText("The crowd was ≈ four hundred strong.")
	if strings.Contains(got, "was four hundred strong") {
		t.Errorf("the hedge vanished — an approximation became a precise claim: %q", got)
	}
}

// & runs before the acronym pass, so letters either side become space-bounded
// and reach that pass at all. PB&J is the case where that is sufficient.
func TestAmpersandComposesWithAcronymExpansion(t *testing.T) {
	got := NormalizeText("He ordered a PBJ and left.")
	if !strings.Contains(got, "pee bee jay") {
		t.Errorf("PBJ should expand to letters, got: %q", got)
	}
}

// AT&T is where the ordering is NOT sufficient, and it is recorded rather than
// fixed. I asserted in a code comment that & ordering made this work; writing
// the test proved otherwise. The mechanism:
//
//	AT&T → "AT and T" → "AT" is now space-bounded → hits the common-word skip
//	list as the preposition "at" → and lone "T" is below acronymRe's {2,4}
//
// Pre-existing, not caused by the & change. NOT fixed here: pulling "AT" out of
// the skip list would spell out every ordinary "at" in the corpus, which is the
// AM bug (#41) all over again. A real fix needs the acronym pass to see AT&T as
// one token BEFORE & is expanded.
func TestAmpersandAcronymCollisionIsKnownAndUnfixed(t *testing.T) {
	got := NormalizeText("She joined AT&T last year.")
	if strings.Contains(got, "ay tee and tee") {
		t.Errorf("AT&T now expands correctly (%q) — good, but this test documented "+
			"it as broken. Delete this test and update the comment on the & block.", got)
	}
}

// Pin the deliberate omissions as deliberate. These are still silent, and that
// is a known open decision (#42), not an oversight. If someone expands them
// later it must be a decision with a reading chosen, not a default — so this
// test failing is the prompt to go record the choice, not to delete the test.
func TestUndecidedSymbolsRemainUnexpanded(t *testing.T) {
	for _, c := range []struct{ in, mustNotContain string }{
		{"The money flowed London → Zurich.", " to "}, // "to" is wrong for cause → effect
		{"The room was 3 × 4 metres.", "times"},       // "by" vs "times" is contextual
		{"The value fell ↓ sharply.", "down"},         // trend mark, probably wants a pause
	} {
		if got := NormalizeText(c.in); strings.Contains(got, c.mustNotContain) {
			t.Errorf("NormalizeText(%q) = %q — expanded a symbol whose reading has not been "+
				"decided; record the decision on #42 rather than defaulting", c.in, got)
		}
	}
}

// ± was already handled by plusMinusRe before tonight. Pinning it because I
// reported it as "cannot tell whether this is spoken" and it turned out the
// answer was in the file — a claim about the code that reading the code
// settles.
func TestPlusMinusWasAlreadyHandled(t *testing.T) {
	got := NormalizeText("The dose was 5 ± 2 milligrams.")
	if !strings.Contains(got, "plus or minus") {
		t.Errorf("± should expand to \"plus or minus\", got: %q", got)
	}
}
