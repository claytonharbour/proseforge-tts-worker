//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// The fusing class (#42), distinct from the silent class in the same ticket.
// `=` and `_` produce no sound either way — LTS skips them — but they glue the
// words either side into a single token it then pronounces as one mangled word:
//
//	source=unknown           → sˈɔɹsɪˈʌnnˈOn
//	event_type=unclassified  → ˈivˈɛnttˈIpˈʌnklˈæsfˈId  (four primary stresses)
//
// This is the Keeper's Door log line, so it is published prose, not a test
// fixture.
func TestFusingSeparatorsDoNotGlueWords(t *testing.T) {
	for in, want := range map[string][]string{
		"The line read source=unknown.":    {"source", "unknown"},
		"It said event_type=unclassified.": {"event", "type", "unclassified"},
		"He wrote x=y on the board.":       {"x", "y"},
	} {
		got := NormalizeText(in)
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("NormalizeText(%q)\n  got:  %q\n  the word %q was fused away", in, got, w)
			}
		}
	}
}

// Separating must not silently start SPEAKING the symbol. Replacing `=` with a
// space preserves exactly the silence it already had; that is what makes this
// change decision-free. If someone later rules that `=` reads as "equals", this
// test failing is the prompt to record that decision on #42, not to delete it.
func TestSeparatorsRemainSilent(t *testing.T) {
	got := NormalizeText("The line read source=unknown.")
	if strings.Contains(got, "equals") {
		t.Errorf("`=` is now spoken (%q) — that was an open decision on #42, not a "+
			"default. Record the ruling and update this test", got)
	}
}

// Ordinary prose must not acquire spurious splits. Underscores and equals signs
// are rare in narrative text, so the risk is low, but "low risk" is not a
// measurement and this is cheap to pin.
func TestOrdinaryProseIsUnaffectedBySeparators(t *testing.T) {
	in := "She opened the door and walked through the empty hall."
	if got := NormalizeText(in); !strings.Contains(got, "walked through the empty hall") {
		t.Errorf("ordinary prose changed: %q", got)
	}
}

// Brackets were absent from isPunctuation while parentheses were present, so a
// bracket stayed glued to its word and LTS pronounced the pair as one word.
// This is ordinary prose, not only the log line that surfaced it:
//
//	She read [the note] aloud.  →  "note]" → nˈOtˈɛ  ("NOH-teh", two syllables)
func TestBracketsDoNotGlueToWords(t *testing.T) {
	p := loadTestPhonemizer(t)
	for _, s := range []string{
		"She read [the note] aloud.",
		"She read {the note} aloud.",
	} {
		for _, d := range p.DiagnoseText(s) {
			if strings.ContainsAny(d.Word, "[]{}") && d.Source == "lts" {
				t.Errorf("%q: %q kept its bracket and fell to LTS as %q",
					s, d.Word, d.Phonemes)
			}
		}
	}
}

// The control: parentheses already behaved correctly, and must continue to.
// They are why the bracket gap was a gap rather than a policy — the same
// sentence with parens was always fine.
func TestParenthesesStillDoNotGlue(t *testing.T) {
	p := loadTestPhonemizer(t)
	for _, d := range p.DiagnoseText("She read (the note) aloud.") {
		if strings.ContainsAny(d.Word, "()") && d.Source == "lts" {
			t.Errorf("%q kept its parenthesis and fell to LTS as %q", d.Word, d.Phonemes)
		}
	}
}
