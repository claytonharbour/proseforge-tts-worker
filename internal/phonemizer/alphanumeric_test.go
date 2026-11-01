//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// #43. LTS skips any character outside a-z, so digits stuck to letters were
// DELETED — the same mechanism as the diacritic bug in #25. The room number,
// the funding amount and the 2 in water were simply gone.
func TestAlphanumericDigitsSurvive(t *testing.T) {
	for in, want := range map[string]string{
		"She waited in Room 3B.":     "three",
		"He raised 14M in the round": "fourteen",
		"He drank H2O.":              "two",
		"He played mp3 files.":       "three",
		"Section 7A was sealed.":     "seven",
		"The gate was 4mm wide.":     "four",
	} {
		got := NormalizeText(in)
		if !strings.Contains(got, want) {
			t.Errorf("NormalizeText(%q)\n  got:  %q\n  the digit was dropped; want it to contain %q",
				in, got, want)
		}
	}
}

// The number-FORMAT rules need their digits and letters adjacent, so the split
// has to run after them. If it is ever moved earlier, 1st becomes "1 st" and
// these break — which is the whole reason the ordering is pinned here rather
// than only described in a comment.
func TestNumberFormatsStillExpandAfterTheSplit(t *testing.T) {
	for in, want := range map[string]string{
		"The 1st of May.":      "first",
		"The 23rd of May.":     "twenty third",
		"It was the 1980s.":    "eighties",
		"He arrived at 10:30.": "ten thirty",
		"It rose 14%.":         "percent",
		"In 1987 he left.":     "nineteen eighty seven",
	} {
		if got := NormalizeText(in); !strings.Contains(got, want) {
			t.Errorf("NormalizeText(%q) = %q — want it to contain %q; the alphanumeric "+
				"split may have moved before the number-format rules", in, got, want)
		}
	}
}

// Q3/B2 went through alphaNumRe before this change and must be unaffected.
func TestSingleLetterAlphanumericUnchanged(t *testing.T) {
	for in, want := range map[string]string{
		"Revenue rose in Q3.": "Q three",
		"See B2 for details.": "B two",
	} {
		if got := NormalizeText(in); !strings.Contains(got, want) {
			t.Errorf("NormalizeText(%q) = %q, want it to contain %q", in, got, want)
		}
	}
}

// What the split does NOT settle, recorded so it is not mistaken for finished:
// "14M" in a funding sentence is fourteen MILLION, not fourteen em. Preserving
// the digit is unambiguous and shipped; choosing the letter's reading needs the
// same decision → and × are waiting on (#42).
func TestMagnitudeSuffixReadingIsStillOpen(t *testing.T) {
	got := NormalizeText("He raised 14M in the round.")
	if strings.Contains(got, "million") {
		t.Errorf("14M now expands to millions (%q) — good, but that was an open "+
			"decision. Delete this test and record the choice on #43.", got)
	}
}
