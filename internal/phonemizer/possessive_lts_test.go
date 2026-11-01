//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// A name no dictionary knows must keep its pronunciation in the possessive (#97).
//
// The LTS fallback used to receive the whole token, so the trailing 's changed
// the guess for the stem: Lila lˈilə, Lila's lˈIlˈæz. The names are protagonists
// from the test stories and the lighthouse books; every possessive of a main
// character shifted.
func TestPossessiveOfLTSNameKeepsTheStem(t *testing.T) {
	p := newTestPhonemizer(t)

	for _, name := range []string{"Lila", "Zara", "Xexu", "Felix", "Elara", "Merrin", "Tamsin", "Garrick"} {
		bareText := "And " + name + " was there."
		bare := phonemesForWord(t, p, bareText, name)
		if src := sourceForWord(t, p, bareText, name); src != "lts" {
			// The arm only tests the LTS path while the name is unknown to
			// every dictionary; a later bank entry would make it vacuous.
			t.Fatalf("%q resolved from %q, not lts — pick a name no dictionary holds", name, src)
		}

		poss := name + "'s"
		possText := "And " + poss + " hat was there."
		want := bare + possessiveSuffix(bare)
		if got := phonemesForWord(t, p, possText, poss); got != want {
			t.Errorf("%q = %q, want %q (stem %q + possessive)", poss, got, want, bare)
		}

		// The speech path must emit the same string the diagnosis reports.
		spoken, err := p.Phonemize(poss)
		if err != nil {
			t.Fatalf("Phonemize(%q): %v", poss, err)
		}
		if strings.TrimSpace(spoken) != want {
			t.Errorf("Phonemize(%q) = %q, want %q", poss, spoken, want)
		}
	}

	// CONTROL: a dictionary stem already took this path, and must not change.
	if got := phonemesForWord(t, p, "And Mary's hat was there.", "Mary's"); got != "mˈɛɹiz" {
		t.Errorf("CONTROL: Mary's = %q, want mˈɛɹiz", got)
	}
}
