//go:build unit

package phonemizer

import "testing"

// "Class III" reads as "Class eye eye eye" — the numeral takes the initialism
// path, so one token becomes three spoken words (#57).
//
// ⚠️ THE CONTROLS ARE THE POINT, and they outnumber the fixes. The Roman
// alphabet is a subset of English spelling, so a converter that matches
// numerals anywhere reads ordinary words as numbers. `CD` is the sharpest: a
// canonical numeral (400) AND a common noun.
//
// Red-proof these together — dropping any one guard turns a different group red:
//
//	trigger word     drop it → CD, MIX, DID convert
//	canonical form   drop it → IIII, VX convert
//	dictionary word  drop it → MIX converts even though a trigger precedes it
func TestRomanNumeralExpansion(t *testing.T) {
	p := newTestPhonemizer(t)
	isWord := func(w string) bool { return p.goldOwnsWord(w) }

	cases := []struct {
		in, want, why string
	}{
		// --- the defect, and @wayland's whole corpus population is this shape ---
		{"We tag it Asset Class III.", "We tag it Asset Class three.",
			"#57's specimen — 6 of 6 corpus occurrences are `Class III`"},
		{"Section IV, paragraph II.", "Section four, paragraph two.",
			"two in one sentence; `paragraph` is a trigger"},
		{"Chapter IX begins here.", "Chapter nine begins here.",
			"subtractive pair"},
		{"World War II ended.", "World War two ended.",
			"`war` is a trigger"},

		// --- CONTROLS: must NOT convert ---
		// ⚠️ These assert the PRE-EXISTING initialism reading, not "unchanged".
		// I first wrote them as unchanged and they went red: the acronym expander
		// legitimately still fires on CD and MIX. A control's job here is to prove
		// MY converter did not turn them into NUMBERS — "see dee", not "four
		// hundred" — so asserting the wrong baseline would have made them fail for
		// a reason that had nothing to do with this change.
		{"A CD player.", "A see dee player.",
			"CONTROL: CD is canonical Roman (400) AND a noun. No trigger → not a number"},
		{"MIX it well.", "em eye ex it well.",
			"CONTROL: MIX is canonical Roman (1009). No trigger → not \"one thousand nine\""},
		{"I was there.", "I was there.",
			"CONTROL: bare I is the pronoun; {2,} excludes it structurally"},
		{"Henry V was crowned.", "Henry V was crowned.",
			"CONTROL: regnal ordinal wants \"the Fifth\", not \"five\" — `Henry` is " +
				"not a numbering word, so this is left WRONG rather than guessed"},
		// ⚠️ The case that makes the dictionary guard discriminate. Without it,
		// the batched red-proof showed NOTHING failing when that guard was
		// removed — a guard I had already described in the source as
		// "load-bearing". MIX is M+IX = 1009, fully canonical, and `mix` is a
		// dictionary word, so a trigger in front of it is the whole exposure.
		{"Volume MIX went missing.", "Volume em eye ex went missing.",
			"CONTROL: trigger + CANONICAL numeral + real word → the dictionary " +
				"guard is the only thing between this and \"Volume one thousand nine\""},
		{"Class IIII held.", "Class eye eye eye eye held.",
			"CONTROL: IIII has a trigger but is NOT canonical, so the converter " +
				"declines and the old initialism path keeps it. Not \"Class four\""},
	}

	for _, tc := range cases {
		if got := NormalizeTextWithWords(tc.in, isWord); got != tc.want {
			t.Errorf("normalize(%q)\n  got  %q\n  want %q\n  %s", tc.in, got, tc.want, tc.why)
		}
	}
}

func TestRomanToInt(t *testing.T) {
	for _, tc := range []struct {
		s string
		n int
	}{
		{"II", 2}, {"III", 3}, {"IV", 4}, {"IX", 9}, {"XII", 12},
		{"XL", 40}, {"XC", 90}, {"CD", 400}, {"CM", 900}, {"MCMXCIV", 1994},
	} {
		if got := romanToInt(tc.s); got != tc.n {
			t.Errorf("romanToInt(%q) = %d, want %d", tc.s, got, tc.n)
		}
	}
}
