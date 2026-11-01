//go:build unit

package phonemizer

import (
	"fmt"
	"strings"
	"testing"
)

// #58. A markdown emphasis marker kept the word out of the dictionary, so it
// fell to LTS and LTS got several of them wrong:
//
//	"I am here."   → ˈI æm hˈɪɹ.    "EYE am here"
//	"*I am here."  → ˈi æm hˈɪɹ.    "EE am here"
//
// The assertion is that the marker makes NO difference, rather than that any
// particular reading is produced — pinning the reading would go green if the
// dictionary changed underneath and both sides moved together.
func TestMarkdownMarkersDoNotChangePronunciation(t *testing.T) {
	p := loadTestPhonemizer(t)

	cases := []struct{ plain, marked string }{
		{"I am here.", "*I am here."},
		{"He said I'm fine.", "He said *I'm fine."},
		{"That is the point.", "That *is the point."},
		{"The interview was long.", "The *interview* was long."},
		// The marker with punctuation stacked after it, which is how emphasis
		// actually ends a clause. The strip loop has to peel both.
		{"She was right, he said.", "She was *right*, he said."},
		{"Do it now.", "Do *it now*."},
	}

	// ⚑ Compared as TOKENS, not as the phoneme string. A stripped marker is
	// returned as a prefix and written into the phoneme text, so the strings
	// legitimately differ by a literal "*" — but '*' has no entry in
	// token_map.json (like '[' and ']'), so the tokenizer drops it and the
	// model never sees it. Tokens are what synthesis feeds the model, and this
	// lane's own note is that token-correct and phoneme-correct are different
	// claims. The one that decides the audio is this one.
	flat := func(t *testing.T, text string) string {
		t.Helper()
		paras, err := p.TokenizeText(text, nil)
		if err != nil {
			t.Fatalf("TokenizeText(%q): %v", text, err)
		}
		var b strings.Builder
		for _, para := range paras {
			for _, g := range para {
				for _, tok := range g.AllTokens {
					fmt.Fprintf(&b, "%d,", tok)
				}
			}
		}
		return b.String()
	}

	for _, c := range cases {
		want := flat(t, c.plain)
		got := flat(t, c.marked)
		if want == "" {
			t.Fatalf("%q tokenized to nothing — the comparison would be empty vs empty", c.plain)
		}
		if got != want {
			pw, _ := p.Phonemize(c.plain)
			pg, _ := p.Phonemize(c.marked)
			t.Errorf("emphasis markers changed what the model receives:\n  %q\n    → %s\n  %q\n    → %s\n"+
				"A marker must be stripped like any other punctuation; leaving it glued sends the "+
				"word past both dictionaries into LTS (#58).", c.plain, pw, c.marked, pg)
		}
	}
}

// Guard the guard: if Phonemize ever returned "" for everything, the comparison
// above would be "" == "" and pass forever.
func TestMarkdownMarkerCasesProduceRealPhonemes(t *testing.T) {
	p := loadTestPhonemizer(t)
	got, err := p.Phonemize("*I am here.")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("phonemized to nothing — the equality test above compares two empty " +
			"strings and cannot fail")
	}
	// The specific defect, kept as a regression anchor: this is the reading the
	// bug produced, and it must not come back.
	if strings.Contains(got, "ˈi ") {
		t.Errorf("%q still contains the LTS reading ˈi (\"ee\") rather than gold's ˈI (\"eye\")", got)
	}
}

// Arithmetic and internal markers must survive: the fix strips leading and
// trailing only, and a change to that would be silent.
func TestInternalAsteriskIsNotStripped(t *testing.T) {
	prefix, core, suffix := splitPunctuation("2*3")
	if core != "2*3" || prefix != "" || suffix != "" {
		t.Errorf("splitPunctuation(%q) = (%q,%q,%q) — an interior marker is not punctuation "+
			"and stripping it would rewrite the token", "2*3", prefix, core, suffix)
	}
	// And the stacked case the corpus actually contains.
	_, core, _ = splitPunctuation("right*,")
	if core != "right" {
		t.Errorf("splitPunctuation(%q) core = %q, want %q — the strip loop must peel the "+
			"comma AND the marker, which is how emphasis ends a clause", "right*,", core, "right")
	}
}
