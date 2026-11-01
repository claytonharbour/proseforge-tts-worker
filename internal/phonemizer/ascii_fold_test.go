//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// Issue #25, live in the corpus. LTS iterates bytes and skips anything outside
// a-z, so multi-byte letters were skipped byte by byte and vanished, leaving a
// consonant skeleton with nothing to say.
func TestLTSFoldsDiacriticsInsteadOfDroppingThem(t *testing.T) {
	lts := loadTestLTS(t)
	for _, c := range []struct {
		word string
		was  string // what it produced before the fold
	}{
		{"Þórsson", "ɹsn"},
		{"Vågå", "vɡ"},
		{"Müller", "mlɜɹ"},
	} {
		got := lts.Phonemize(c.word)
		if got == c.was {
			t.Errorf("%s still produces the pre-fold output %q", c.word, c.was)
		}
		if !hasVowelPhoneme(got) {
			t.Errorf("%s → %q has no vowel — still unspeakable", c.word, got)
		}
	}
}

// The clearest statement of the bug: two DIFFERENT names collapsed to one wrong
// output because the letter that distinguished them was deleted.
func TestLTSDistinguishesNamesThatDifferOnlyByDiacritic(t *testing.T) {
	lts := loadTestLTS(t)
	for _, pair := range [][2]string{
		{"Müller", "Møller"},
		{"Renee", "Renée"},
	} {
		a, b := lts.Phonemize(pair[0]), lts.Phonemize(pair[1])
		if a == "" || b == "" {
			t.Fatalf("empty phonemes for %v — test cannot discriminate", pair)
		}
		if pair[0] == "Müller" && a == b {
			t.Errorf("%s and %s both → %q; the distinguishing letter is still lost",
				pair[0], pair[1], a)
		}
	}
}

// A folded name must land near its ASCII twin — that is the whole claim, that
// the fold recovers a speakable spelling rather than inventing one.
func TestFoldedNameMatchesItsASCIITwin(t *testing.T) {
	lts := loadTestLTS(t)
	for _, c := range []struct{ accented, ascii string }{
		{"Vågå", "Vaga"},
		{"Müller", "Muller"},
		{"Renée", "Renee"},
	} {
		if got, want := lts.Phonemize(c.accented), lts.Phonemize(c.ascii); got != want {
			t.Errorf("%s → %q but its ASCII twin %s → %q; the fold should make them identical",
				c.accented, got, c.ascii, want)
		}
	}
}

// Pure-ASCII input must be byte-identical after the change. This is the whole
// existing corpus, so a regression here is far worse than #25.
func TestFoldLeavesASCIIUntouched(t *testing.T) {
	lts := loadTestLTS(t)
	for _, w := range []string{
		"hearable", "unjammable", "Corbin", "Percival", "Schumann", "Margaret", "burnt",
	} {
		if got, want := lts.Phonemize(w), lts.Phonemize(strings.ToLower(w)); got != want {
			t.Errorf("ASCII word %s is no longer case-stable: %q vs %q", w, got, want)
		}
		if lts.Phonemize(w) == "" {
			t.Errorf("ASCII word %s now produces nothing", w)
		}
	}
}

func TestFoldToASCII(t *testing.T) {
	for in, want := range map[string]string{
		"vågå":    "vaga",
		"þórsson": "thorsson",
		"møller":  "moller",
		"straße":  "strasse",
		"æther":   "aether",
		"corbin":  "corbin", // untouched
	} {
		if got := foldToASCII(in); got != want {
			t.Errorf("foldToASCII(%q) = %q, want %q", in, got, want)
		}
	}
}

func hasVowelPhoneme(ipa string) bool {
	for _, r := range ipa {
		switch r {
		case 'i', 'ɪ', 'e', 'ɛ', 'æ', 'a', 'ɑ', 'ɔ', 'o', 'ʊ', 'u', 'ʌ', 'ə', 'ɜ',
			'A', 'I', 'O', 'W', 'Y', 'U', 'Q', 'S', 'T':
			return true
		}
	}
	return false
}
