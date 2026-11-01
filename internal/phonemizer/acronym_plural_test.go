//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// Acronym plurals never reached the acronym path: the lowercase s is a word
// character, so acronymRe's closing \b fails on "LLCs". They fell to LTS as
// consonant strings with no vowel at all — LLCs → lks, SREs → sɹz. 24
// occurrences over 8 forms in the corpus (Wayland, #39).
func TestAcronymPluralsAreSpokenAsLetters(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Two LLCs were listed.", "ell ell sees"},
		{"The SREs were paged.", "ess ar ees"},
		{"He collected IDs.", "eye dees"},
		{"The LEDs flickered.", "ell ee dees"},
		{"Their SLOs slipped.", "ess ell ohs"},
		{"Both CEOs resigned.", "see ee ohs"},
		{"The APIs returned.", "ay pee eyes"},
		{"Compare the UUIDs.", "you you eye dees"},
	}
	for _, c := range cases {
		got := NormalizeText(c.in)
		if !strings.Contains(got, c.want) {
			t.Errorf("NormalizeText(%q)\n  got:  %q\n  want it to contain: %q", c.in, got, c.want)
		}
	}
}

// The plural attaches to the last letter NAME, so sibilant-final names take
// -es. Without this, S → "esss" and X → "exs", which LTS cannot pronounce —
// exactly the failure the fix exists to remove.
func TestAcronymPluralSibilantEndings(t *testing.T) {
	for in, want := range map[string]string{
		"The SASs ran.":    "ess ay esses",
		"Compare the PBXs": "pee bee exes",
	} {
		if got := NormalizeText(in); !strings.Contains(got, want) {
			t.Errorf("NormalizeText(%q)\n  got:  %q\n  want it to contain: %q", in, got, want)
		}
	}
}

// Word-acronyms stay words when pluralised: NOC is "knock", not "en oh see",
// so NOCs must not become "en oh sees". This is why the plural path routes
// through expandAcronym rather than expanding letters itself.
func TestAcronymPluralKeepsWordAcronyms(t *testing.T) {
	got := NormalizeText("The NOCs were staffed.")
	if strings.Contains(got, "en oh sees") {
		t.Errorf("NOCs was spelled out letter-by-letter: %q", got)
	}
	if !strings.Contains(got, "nocs") {
		t.Errorf("NOCs should stay a word (nocs), got: %q", got)
	}
}

// Guard the boundary: a capitalised ordinary word followed by s must not be
// read as an acronym plural. "As" has only one capital, so the {2,4} quantifier
// already excludes it — this pins that rather than trusting it.
func TestAcronymPluralDoesNotEatOrdinaryWords(t *testing.T) {
	for _, in := range []string{"As it happens.", "Is the door shut?", "Us too."} {
		got := NormalizeText(in)
		if strings.Contains(got, "ay ess") || strings.Contains(got, "eye ess") {
			t.Errorf("NormalizeText(%q) spelled out an ordinary word: %q", in, got)
		}
	}
}
