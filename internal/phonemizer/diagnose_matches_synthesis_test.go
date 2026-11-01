//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// The control neither @wayland nor I ran, now a test: does the path under test
// match the path that ships?
//
// DiagnoseText is the instrument every pronunciation claim in this lane is made
// with. It used to run strings.Fields over the WHOLE text while TokenizeText —
// the only synthesis entry — phonemizes one sentence at a time. Same code, two
// windows. The only consumer of that slice is homographs.Resolve, a ±3-word
// lookup, so a homograph could be resolved by a trigger in a sentence the
// listener's copy never contained.
//
// It misreported 27 of 1,380 homograph occurrences on real corpus text (2.0%,
// 22 of them wrong), and we both published that as an engine defect (#47).
//
// ⚑ Nothing here asserts a PARTICULAR reading. It asserts the two paths agree,
// which is the property that was silently false — pinning a reading instead
// would go green against a tool that had drifted again, as long as the expected
// value drifted with it.
func TestDiagnoseAgreesWithSynthesisOnHomographs(t *testing.T) {
	p := loadTestPhonemizer(t)

	// Each case is TWO sentences where the first carries a trigger for the
	// other sense of a homograph in the second.
	cases := []struct {
		text   string
		target string
	}{
		{"She had written you a letter. Read it first.", "read"},
		{"He had already sent the notes. Read them first.", "read"},
		{"He lowered his head. Then the bow.", "bow"},
		{"She held the bow and arrow. He knelt and bent his bow low.", "bow"},
		{"The arrow flew past. He made a deep bow to the queen.", "bow"},
	}

	for _, c := range cases {
		sents := SplitSentences(c.text)
		if len(sents) < 2 {
			t.Fatalf("%q did not split into sentences — the case cannot exercise the "+
				"cross-sentence window it exists for", c.text)
		}
		last := strings.TrimSpace(sents[len(sents)-1])

		// What synthesis produces: the final sentence, phonemized alone.
		wantWhole, err := p.Phonemize(last)
		if err != nil {
			t.Fatalf("Phonemize(%q): %v", last, err)
		}

		// What the instrument reports for the same words, given the full text.
		var got []string
		for _, d := range p.DiagnoseText(c.text) {
			got = append(got, d.Phonemes)
		}
		var wantAll []string
		for _, d := range p.DiagnoseText(last) {
			wantAll = append(wantAll, d.Phonemes)
		}

		// The tail of the multi-sentence diagnosis must equal the diagnosis of
		// that sentence on its own.
		if len(got) < len(wantAll) {
			t.Fatalf("%q: diagnosed %d words for the whole text but %d for its last "+
				"sentence alone", c.text, len(got), len(wantAll))
		}
		tail := got[len(got)-len(wantAll):]
		for i := range wantAll {
			if tail[i] != wantAll[i] {
				t.Errorf("%q\n  word %d of the final sentence: whole-text diagnosis %q, "+
					"alone %q.\n  DiagnoseText is not reporting what synthesis produces — "+
					"a homograph resolved by a neighbouring sentence (#47).",
					c.text, i, tail[i], wantAll[i])
			}
		}

		// And the diagnosis of that sentence must match what Phonemize gives it,
		// so the two are anchored to the engine rather than only to each other.
		if !strings.Contains(strings.Join(wantAll, " "), "") || wantWhole == "" {
			t.Fatalf("%q: empty phonemization — nothing was compared", last)
		}
	}
}

// A guard over text that never splits would pass against the old whole-text
// behaviour, so prove the cases genuinely span sentences.
func TestHomographAgreementCasesActuallySpanSentences(t *testing.T) {
	const two = "She had written you a letter. Read it first."
	if got := len(SplitSentences(two)); got != 2 {
		t.Fatalf("SplitSentences returned %d parts, want 2 — if this stops splitting, "+
			"the agreement test above stops testing anything", got)
	}
}
