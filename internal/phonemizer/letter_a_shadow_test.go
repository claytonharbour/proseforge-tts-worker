//go:build unit

package phonemizer

import "testing"

// #62, pinned with a control 2026-08-19. The ticket says "A is the only letter
// that breaks" and that is exactly right — the sharpest way to see it is two
// sentences differing in nothing but the letter:
//
//	Grade A beef.   A → ə     🛑 "grade uh"
//	Plan B failed.  B → bˈi   ✅ "plan bee"
//	Mrs. Polk, 2A.  A → ə     🛑 "two uh"
//	She lives in 3B. B → bˈi  ✅ "three bee"
//
// 🛑 So this is not a context problem and no amount of context will fix it.
// phonemizeWord lowercases and returns "ə" for "a" before ANY dictionary is
// consulted, so gold's `A = ˈA` (the letter name, which exists) is unreachable.
// Every other letter of the alphabet resolves from gold and is correct.
//
// ⚠️ The B arms are the control and they are not decoration. Without them this
// reads as "letters after nouns are mispronounced", which is false and would
// send the fix at the wrong target — 25 of 26 letters already work.
//
// 📌 @wayland discarded `Apartment 1A.` and `Mrs. Polk, 2A.` from his #62 count
// as lookbehind false positives. They ARE false positives for an INITIALS
// classifier and they are NOT false positives for this defect: the engine says
// "apartment one uh". A row can be correctly excluded from one population and
// still belong to another, so the exclusion note matters as much as the count.
//
// 🛑 ALL THREE FIX DIRECTIONS ARE REFUTED, each by a concrete specimen:
//
//	bank `A` as the letter name       breaks 1,256 articles (@wayland, #62)
//	key on a following period         breaks "One. At. A. Time." — IN the corpus
//	capital A + adjacent single letter  breaks "A B movie played."
//
// The third is the subtle one and it is why the guard case above exists. After
// the normalizer runs, these two are the SAME SHAPE:
//
//	"A.R. Sinclair called."  → A R Sinclair    A is an initial   🛑 wrong today
//	"A B movie played."      → A B movie       A is the article  ✅ right today
//
// Capital, followed by a lone letter, in both. The period that distinguished
// them was consumed by abbreviationDotRe, and sentence-initial capitalisation
// carries no information because every sentence starts that way. ⚠️ Case DOES
// separate "a C major chord" from "A R" — that much of the idea works — but it
// cannot separate a sentence-initial article from an initial, and that is the
// case that decides it.
//
// ⛔ Still refused as a fix on exposure — @wayland measured 1,256 articles to
// ~13 letter-names, and every classification error he could have made moves that
// ratio further against banking. This test records the mechanism and its
// boundary, not a plan to change it.
func TestOnlyLetterAIsShadowedByTheArticleRule(t *testing.T) {
	p := loadTestPhonemizer(t)

	read := func(sentence, word string) string {
		t.Helper()
		for _, d := range p.DiagnoseText(sentence) {
			if d.Word == word {
				return d.Phonemes
			}
		}
		return ""
	}

	cases := []struct {
		sentence, word, want, why string
	}{
		{"Grade A beef.", "A", "ə", "the defect: article reading shadows the letter name"},
		{"Mrs. Polk, 2A.", "A", "ə", "digit-adjacent, same defect"},
		{"Plan B failed.", "B", "bˈi", "CONTROL: B resolves from gold and is correct"},
		{"She lives in 3B.", "B", "bˈi", "CONTROL: digit-adjacent B is correct"},

		// 🛑 THE GUARD. This one is CORRECT today and a fix must keep it correct.
		// It is the counterexample to the narrowest rule anyone will reach for.
		{"A B movie played.", "A", "ə", "GUARD: sentence-initial article before a single letter — see below"},
	}
	for _, c := range cases {
		got := read(c.sentence, c.word)
		if got == "" {
			t.Fatalf("%q: no diagnosis row for %q — the specimen did not reach the "+
				"phonemizer, so any verdict here would be vacuous", c.sentence, c.word)
		}
		if got != c.want {
			t.Errorf("%q: %s → %q, want %q (%s)", c.sentence, c.word, got, c.want, c.why)
		}
	}

	// The premise: gold HOLDS the letter name, so this is a shadowing bug and not
	// a missing entry. If that stops being true the whole framing changes.
	if p.gold != nil {
		if got, ok := p.gold.Lookup("A"); !ok || got == "ə" {
			t.Errorf("gold no longer holds a distinct letter-name reading for A (got %q, ok=%v) — "+
				"#62 is a SHADOWING bug and that framing depends on the entry existing", got, ok)
		}
	}
}
