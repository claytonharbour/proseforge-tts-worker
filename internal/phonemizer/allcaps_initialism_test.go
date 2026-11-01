//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// Initialisms that MUST keep spelling out, pinned ahead of the pending `[WARN]`
// ruling (#42) rather than after it.
//
// Smiley's proposed test — "is the all-caps token itself an English word?" —
// would lowercase every one of these, because `da`, `led`, `id`, `pa`, `ag` and
// `os` are all in the bundled dictionaries. Measured against all 38 corpus
// snapshots it would change 151 tokens over 476 occurrences, led by DA ×70 —
// which is Wayland's original `DA. FBI.` specimen, and the same defect his
// 33-false-negative critique used to kill the first #41 attempt.
//
// These assertions are true under the current code AND under any narrower gate
// that might replace it, so they belong here now: whatever rule lands for WARN,
// this is the boundary it must not cross.
func TestInitialismsKeepSpellingOut(t *testing.T) {
	p := loadTestPhonemizer(t)
	for _, c := range []struct{ in, want, why string }{
		{"The DA declined to charge.", "dee ay", "District Attorney — but `da` is a dictionary word"},
		{"The LED blinked twice.", "ell ee dee", "`led` is the past tense of lead"},
		{"The EPA issued a notice.", "ee pee ay", "`epa` is not a word — the easy case"},
		{"An ID was required.", "eye dee", "`id` is a dictionary word"},
	} {
		got := NormalizeTextWithWords(c.in, p.isEnglishWord)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q\n  got:  %q\n  want it to contain %q — %s", c.in, got, c.want, c.why)
		}
	}
}

// AM/PM are the named exception and predate all of this: all-caps AM is the
// meridiem, and lowercasing it to the verb gives "seven əm" — the AM bug that
// normalizer.go's own comment calls out. Any rule keyed on dictionary
// membership must exclude them explicitly, because `am` and `pm` are both
// dictionary words.
func TestMeridiemStillSpellsOut(t *testing.T) {
	p := loadTestPhonemizer(t)
	for _, c := range []struct{ in, want string }{
		{"He left at 7 AM sharp.", "ay em"},
		{"The train goes at 9 PM.", "pee em"},
	} {
		if got := NormalizeTextWithWords(c.in, p.isEnglishWord); !strings.Contains(got, c.want) {
			t.Errorf("%q\n  got:  %q\n  want it to contain %q — this is the AM bug", c.in, got, c.want)
		}
	}
}

// The ALL-CAPS run rule (#41) must keep working — it is the two-factor case,
// and the pending change is about LONE tokens, which must not disturb it.
func TestAllCapsRunStillReadsAsProse(t *testing.T) {
	p := loadTestPhonemizer(t)
	got := NormalizeTextWithWords("I AM WATCHING YOU", p.isEnglishWord)
	if strings.Contains(got, "ay em") {
		t.Errorf("AM inside an all-caps run was spelled out: %q — the run rule "+
			"should read it as the verb here", got)
	}
}

// Smiley's ruling (#42): spell out initialisms, speak truncations. WARN is a
// truncation of "warning" and has no expansion — no engineer reading a log
// aloud spells it.
func TestTruncationsAreSpokenAsWords(t *testing.T) {
	p := loadTestPhonemizer(t)
	for _, c := range []struct{ in, want string }{
		{"02:17:33 [WARN] source=unknown", "warn"},
		{"The [INFO] tag appeared.", "info"},
		{"The RICO case collapsed.", "rico"},
		{"An OSHA inspector called.", "osha"},
	} {
		got := NormalizeTextWithWords(c.in, p.isEnglishWord)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q\n  got:  %q\n  want it to contain %q", c.in, got, c.want)
		}
	}
}

// The load-bearing red-proof: removing the length gate must break the
// initialism boundary, proving the gate is doing work rather than merely
// correlating. DA is the specimen — District Attorney, 70 occurrences across 12
// books, and `da` is in the dictionary.
//
// If this ever passes with the gate removed, the corpus has changed and the
// measurement behind the gate needs re-running.
func TestLengthGateIsLoadBearing(t *testing.T) {
	p := loadTestPhonemizer(t)
	// DA is 2 letters and a dictionary word: it is exactly the token that
	// dictionary-membership-alone would wrongly lowercase.
	if !p.isEnglishWord("da") {
		t.Fatal("`da` is not in the dictionary — the premise of this test is gone, " +
			"so the length gate may no longer be necessary. Re-measure before deleting it.")
	}
	got := NormalizeTextWithWords("The DA declined to charge.", p.isEnglishWord)
	if !strings.Contains(got, "dee ay") {
		t.Errorf("DA → %q: the length gate is not protecting 2-letter initialisms", got)
	}
}

// A CHARACTERIZATION test: this asserts behaviour that is WRONG, on purpose.
//
// `THE` ×41 in the corpus is shouted prose and should be spoken, but it is 3
// letters so the gate spells it out. Smiley's reasoning for pinning it (#42):
// a known-wrong case with a test is a decision; a known-wrong case without one
// is a thing nobody remembers deciding.
//
// ⛔ Do NOT "fix" this by widening the gate to 3. The 3-letter band is
// irreducible by length — `THE` (shouting, wants speaking) and `LED` (initialism,
// wants spelling) are the same shape, and TestLengthGateIsLoadBearing pins the
// other side. A short deny-list of 3-letter initialisms is the closeable route,
// and it is a separate ticket.
//
// If this starts failing, the band was addressed: check LED still spells, then
// delete this test and record what was decided.
func TestThreeLetterBandIsKnownWrong(t *testing.T) {
	p := loadTestPhonemizer(t)
	got := NormalizeTextWithWords("THE DOOR WAS OPEN", p.isEnglishWord)
	if strings.Contains(got, "tee aitch ee") {
		return // spelled out — the documented wrong behaviour, unchanged
	}
	if !strings.Contains(got, "the") {
		t.Errorf("THE in a shouted line → %q: neither spelled nor spoken, which is "+
			"a third outcome nobody predicted", got)
	}
}

// The lone case, which is the one the gate actually governs. A shouted line is
// a RUN and the #41 rule already speaks it; an isolated THE is not.
func TestLoneThreeLetterWordStillSpellsOut(t *testing.T) {
	p := loadTestPhonemizer(t)
	got := NormalizeTextWithWords("She saw THE sign.", p.isEnglishWord)
	if !strings.Contains(got, "tee aitch ee") {
		t.Errorf("lone THE → %q. This is the documented 3-letter band defect; if it "+
			"now speaks, the band was addressed — verify LED still spells out, then "+
			"delete this test and record the decision", got)
	}
}
