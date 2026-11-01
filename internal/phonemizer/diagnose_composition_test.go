//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// A SECOND, independent guard on `84f8030`.
//
// Batched red-proofing of four real defects (309 test functions, whole suite)
// gave this discrimination profile:
//
//	'*' not punctuation      #58   3 tests noticed
//	decimal guard removed    #59   3
//	AX dropped from the map  #53   6
//	DiagnoseText whole-text  #47   1     ← this file exists because of that 1
//
// One guard, on the fix every pronunciation measurement in this lane now rests
// on: it moved three output streams, invalidated a 2,760-call census, and
// surfaced #58, #59 and #60. Six tests defend a JSON field name; one defended
// the foundation.
//
// ⚑ Deliberately a DIFFERENT assertion, not a copy. TestDiagnoseAgreesWith-
// SynthesisOnHomographs compares readings against Phonemize — it fails when the
// window is wrong. This asserts COMPOSITION: diagnosing a whole text must equal
// diagnosing its sentences and concatenating. That holds for reasons the other
// test does not check (word order, counts, sources, suffix rules) and fails for
// reasons it would miss. Two tests that fail for the same reason are one test.
//
// ⚠️ Since #112 a sentence can carry a hint from the PREVIOUS sentence of its paragraph (a bare
// "Read" after past-tense narration). Where a hint applies, diagnosing a sentence alone differs
// from diagnosing it in its paragraph, by design. None of the texts below triggers one, so the
// composition property still holds for them; TestNarratedPast_DiagnoseMatchesSynthesis (in
// narrated_past_read_test.go) covers the hinted case against the synthesis path.
func TestDiagnoseTextComposesFromItsSentences(t *testing.T) {
	p := loadTestPhonemizer(t)

	texts := []string{
		"She had written you a letter. Read it first.",
		"He lowered his head. Then the bow. The room was silent.",
		"It cost $2.8 million. He paid in full. Dr. Hanson signed.",
		"One sentence only.",
		"A breath, in a confrontation measured in minutes, is the difference.",
	}

	for _, text := range texts {
		whole := p.DiagnoseText(text)

		var perSentence []WordDiagnosis
		for _, para := range SplitParagraphs(text) {
			for _, sent := range SplitSentences(para) {
				perSentence = append(perSentence, p.DiagnoseText(sent)...)
			}
		}

		if len(whole) == 0 {
			t.Fatalf("%q diagnosed to nothing — the comparison below would be empty vs empty", text)
		}
		if len(whole) != len(perSentence) {
			t.Errorf("%q\n  whole text: %d words\n  per sentence: %d words\n"+
				"DiagnoseText must decompose over sentences; a different count means it is "+
				"tokenizing across a boundary synthesis never crosses (#47).",
				text, len(whole), len(perSentence))
			continue
		}
		for i := range whole {
			w, s := whole[i], perSentence[i]
			if w.Word != s.Word || w.Phonemes != s.Phonemes || w.Source != s.Source || w.Rule != s.Rule {
				t.Errorf("%q\n  word %d differs between whole-text and per-sentence diagnosis:\n"+
					"    whole:       %+v\n    per sentence: %+v", text, i, w, s)
			}
		}
	}
}

// The composition test is vacuous on text that never splits, so prove the
// corpus of cases actually exercises multi-sentence input.
func TestCompositionCasesActuallySplit(t *testing.T) {
	const multi = "He lowered his head. Then the bow. The room was silent."
	if got := len(SplitSentences(multi)); got != 3 {
		t.Fatalf("SplitSentences gave %d parts, want 3 — if this stops splitting, the "+
			"composition test above compares a text with itself and cannot fail", got)
	}
	// And at least one case must contain a homograph, or the strongest signal
	// the assertion can carry is absent from every specimen.
	d := loadTestPhonemizer(t).DiagnoseText(multi)
	var sources []string
	for _, w := range d {
		sources = append(sources, w.Source)
	}
	if !strings.Contains(strings.Join(sources, ","), "homograph") {
		t.Errorf("no case resolves through the homograph path (%v) — that is the only path "+
			"that reads the word slice, so without it this guards nothing", sources)
	}
}
