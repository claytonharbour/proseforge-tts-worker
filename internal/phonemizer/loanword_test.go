//go:build unit

package phonemizer

import (
	"testing"
)

// Accented loanwords are banked directly rather than looked up by folded
// spelling (#69).
//
// 🛑 The fix I proposed on the ticket was WRONG and @wayland's population is
// what showed it. I wanted the pipeline to retry the ASCII-folded spelling
// against the dictionaries, on the premise that gold already holds the answer —
// `naïve`→`naive`→nɑˈiv. It does, for genuine twins. But 5 of the 7 words in
// the corpus fold onto a DIFFERENT ENGLISH WORD:
//
//	pâté    → pate      pˈAt        "PAYT", the top of the head
//	résumé  → resume    ɹəzˈum      "ri-ZOOM", the verb
//	soufflé → souffle   sˈufəl      a heart murmur
//
// English borrowed these WITHOUT the accent and gave the bare spelling its own
// meaning, so the folded form is a homograph, not a twin. A folded lookup would
// have resolved confidently onto the wrong lexeme — and my own ticket table
// listed `resume ɹəzˈum gold ✅`, marking the verb as the correct reading of the
// noun. @wayland checked three ways before contradicting it.
//
// ⚑ Banking needs no folding logic, no exclusion list, and no ordering
// judgement: custom_dict is consulted first and matches the exact key, so it is
// correct for twins and false-twins alike and cannot touch anything else.
//
// ⛔ `façade` is deliberately NOT banked. LTS already produces fəsˈɑd, and
// TestBankRedundancyIsPinned rejected the entry — a bank entry that duplicates
// what the generator already gets right is dead weight that would hide a future
// LTS regression behind it. That guard caught this within a minute of the entry
// being added.
func TestAccentedLoanwordsAreBanked(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct{ word, want, was, gloss string }{
		{"pâté", "pɑtˈA", "pˈAt", "pa-TAY — was \"PAYT\", a different word"},
		{"résumé", "ɹˈɛzəmˌA", "ɹɪzˈum", "REZ-oo-may — was the verb \"resume\""},
		{"soufflé", "suflˈA", "sufəl", "soo-FLAY — was stressless, and a heart murmur"},
		{"café", "kæfˈA", "kˈAf", "ka-FAY — was \"KAYF\". Genuine twin; gold's own string"},
		{"naïve", "nɑˈiv", "nɑv", "nah-EEV — was ONE syllable. Genuine twin"},
		{"Révolution", "ɹˌɛvəlˈuʃən", "ɹˈɛvəlˈuʃən", "was TWO primary stresses in one word"},
	}

	for _, tc := range cases {
		got := phonemesForWord(t, p, "The "+tc.word+" was there.", tc.word)
		if got != tc.want {
			t.Errorf("%q = %q, want %q (was %q)\n  %s", tc.word, got, tc.want, tc.was, tc.gloss)
		}
		// ⚠️ Assert the LAYER too. These are non-ASCII keys, and if custom_dict
		// ever stopped matching them the word would fall to LTS and produce the
		// `was` column again — a value assertion alone would still pass on any
		// entry whose LTS guess happened to agree.
		if src := sourceForWord(t, p, "The "+tc.word+" was there.", tc.word); src != "custom" {
			t.Errorf("%q resolved from %q, not custom — the bank is not being reached "+
				"for a non-ASCII key", tc.word, src)
		}
	}

	// ⚠️ CONTROL: Révolution is banked lowercase and appears capitalised in the
	// prose. If the lookup were case-sensitive this arm would fail while every
	// lowercase arm above passed.
	if src := sourceForWord(t, p, "The Révolution began.", "Révolution"); src != "custom" {
		t.Errorf("CONTROL: capitalised Révolution resolved from %q — the bank lookup "+
			"is case-sensitive, and every key here is stored lowercase", src)
	}
}
