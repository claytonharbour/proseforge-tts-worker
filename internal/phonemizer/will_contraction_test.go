//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// An 'll contraction is its stem plus contracted "will" (#99).
//
// Gold banks only the pronoun forms, so everything else went whole to LTS:
// Mary'll mˈɛɹˈIl ("Mary ill"), There'll θˈɛɹəl (voicing lost), That'll θˈætl
// (no vowel). Found narrating Alice (#98).
func TestWillContractionKeepsTheStem(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct{ stem, contraction string }{
		{"There", "There'll"},   // gold stem, consonant-final
		{"That", "That'll"},     // gold stem
		{"Mary", "Mary'll"},     // gold stem, vowel-final
		{"Dinah", "Dinah'll"},   // custom stem (#102)
		{"slates", "slates'll"}, // inflected gold stem
		{"Lila", "Lila'll"},     // LTS stem
	}

	for _, tc := range cases {
		bare := phonemesForWord(t, p, "And "+tc.stem+" stood still.", tc.stem)
		want := bare + expectedWill(bare)
		text := "And " + tc.contraction + " stand still."
		if got := phonemesForWord(t, p, text, tc.contraction); got != want {
			t.Errorf("%q = %q, want %q (stem %q + will)", tc.contraction, got, want, bare)
		}
		spoken, err := p.Phonemize(tc.contraction)
		if err != nil {
			t.Fatalf("Phonemize(%q): %v", tc.contraction, err)
		}
		if strings.TrimSpace(spoken) != want {
			t.Errorf("Phonemize(%q) = %q, want %q", tc.contraction, spoken, want)
		}
	}

	// The two audible failures from the ticket, stated directly.
	if got := phonemesForWord(t, p, "And There'll be rain.", "There'll"); !strings.HasPrefix(got, "ð") {
		t.Errorf("There'll = %q, must begin with voiced ð", got)
	}
	if got := phonemesForWord(t, p, "And Mary'll be there.", "Mary'll"); strings.Count(got, "ˈ") > 1 {
		t.Errorf("Mary'll = %q has more than one primary stress (\"Mary ill\")", got)
	}

	// CONTROL: gold's own whole-contraction entries must still win.
	for word, want := range map[string]string{"it'll": "ˈɪɾᵊl", "she'll": "ʃil", "they'll": "ðAl"} {
		if got := phonemesForWord(t, p, "And "+word+" be there.", word); got != want {
			t.Errorf("CONTROL: %q = %q, want gold's %q", word, got, want)
		}
	}
}

// expectedWill restates the rule from gold's own 'll entries independently of
// willSuffix, so the test can fail against code that lacks it: l after a vowel
// (he'll hil), ᵊl after a consonant (it'll ˈɪɾᵊl).
func expectedWill(ipa string) string {
	r := []rune(ipa)
	if len(r) > 0 && strings.ContainsRune("aeiouɑæɛɪʊʌɔəᵊɜɐAIOWY", r[len(r)-1]) {
		return "l"
	}
	return "ᵊl"
}
