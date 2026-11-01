//go:build unit

package phonemizer

// A bare, capitalised "Read" at the start of a sentence is an imperative ("reed") in dialogue and
// in instructions, but in third-person past narration it is a fragment with the subject dropped:
// "He opened the file. Read the first page." The engine used to say "reed" for all of them
// (#112). Measured on real prose: 10 of 10 such narrated fragments were wrong and 6 of 7
// instructions were right, so the rule is: after a plain past-tense NARRATED sentence in the same
// paragraph, "Read" is the past reading, and everywhere else the imperative stays.
//
// The sentences below are synthetic on purpose: this repo may be made public and must not hold
// other people's story text.

import (
	"strings"
	"testing"
)

const (
	pastRed   = "ɹˈɛd"
	imperReed = "ɹˈid"
)

// readIPA returns the phonemes DiagnoseText chose for the k-th "read" word (0-based) in text.
func readIPA(t *testing.T, p *Phonemizer, text string, k int) string {
	t.Helper()
	n := 0
	for _, d := range p.DiagnoseText(text) {
		if strings.EqualFold(strings.Trim(d.Word, `.,;:!?"'“”’‘ `), "read") {
			if n == k {
				return d.Phonemes
			}
			n++
		}
	}
	t.Fatalf("no %dth \"read\" in %q", k, text)
	return ""
}

func TestNarratedPastRead(t *testing.T) {
	p := newTestPhonemizer(t)
	cases := []struct {
		name string
		text string
		k    int    // which "read" in the text is the one under test
		want string // pastRed or imperReed
	}{
		// The class this change is for: a narrated past sentence, then a subject-less "Read".
		{"after a -ed past sentence", "He opened the ledger. Read the first page.", 0, pastRed},
		{"after an irregular past sentence", "She looked up when he came in. Read his face.", 0, pastRed},
		{"after a past sentence with a past 'read' in it", "Maren read it twice. Read it again.", 1, pastRed},
		{"after a short -ed sentence", "He paused. Read the numbers a fifth time.", 0, pastRed},
		{"two in a row, both narrated", "He picked up the page. Read the first line. Read the second line.", 0, pastRed},
		{"two in a row, second one", "He picked up the page. Read the first line. Read the second line.", 1, pastRed},

		// The imperative stays: nothing before it is past narration.
		{"paragraph start", "Read the first page.", 0, imperReed},
		{"after an imperative", "Open the manual. Read the first page.", 0, imperReed},
		{"after a present-tense sentence", "Here is the checklist. Read each item before you start.", 0, imperReed},
		{"after a heading-like present sentence", "The log is below. Read it for the patterns.", 0, imperReed},

		// A colon introduces reported speech or an instruction, so what follows is not narration.
		{"after a past sentence with a colon", "She told the quill: this one is here. Read it directly.", 0, imperReed},

		// Dialogue is never narration, whatever precedes it.
		{"opening quote after a past sentence", "He opened the ledger. \"Read the first page.\"", 0, imperReed},
		{"previous sentence is itself dialogue", "\"I opened the ledger. Read the first page.\" he said.", 0, imperReed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := readIPA(t, p, c.text, c.k); got != c.want {
				t.Errorf("%q: got %s, want %s", c.text, got, c.want)
			}
		})
	}
}

// The hint must be the same whether the text goes through the synthesis path (TokenizeText) or the
// audit path (DiagnoseText), which mirrors it (#47). Both go through sentenceHints.
func TestNarratedPastHints_AreSharedByBothPaths(t *testing.T) {
	sents := SplitSentences("He opened the ledger. Read the first page. Open the manual. Read it.")
	got := sentenceHints(sents)
	// [0] nothing before it. [1] follows the narrated "He opened the ledger.". [2] follows the narrated
	// fragment "Read the first page." (the narration carries on through it; a "Read" in sentence 2
	// would take the narrated reading, and there is none). [3] follows "Open the manual.", which is
	// neither past narration nor a "Read" fragment, so the run ends.
	want := []bool{false, true, true, false}
	if len(got) != len(want) {
		t.Fatalf("hints = %v", got)
	}
	for i := range want {
		if got[i].PrevSentenceNarratedPast != want[i] {
			t.Errorf("hint[%d] = %v, want %v (sentences %q)", i, got[i].PrevSentenceNarratedPast, want[i], sents)
		}
	}
}

// Only "read" opts in. Another word with an imperative reading must not move.
func TestNarratedPast_DoesNotTouchOtherHomographs(t *testing.T) {
	p := newTestPhonemizer(t)
	a := p.DiagnoseText("Lead the way.")
	b := p.DiagnoseText("He opened the ledger. Lead the way.")
	if len(b) < 2 || len(a) < 1 {
		t.Fatalf("unexpected diagnosis lengths: %d %d", len(a), len(b))
	}
	var la, lb string
	for _, d := range a {
		if strings.EqualFold(d.Word, "lead") {
			la = d.Phonemes
		}
	}
	for _, d := range b {
		if strings.EqualFold(strings.Trim(d.Word, `.`), "lead") {
			lb = d.Phonemes
		}
	}
	if la == "" || la != lb {
		t.Errorf("\"Lead\" changed with a narrated sentence before it: alone %q, after %q", la, lb)
	}
}

// Resolve (no hint) must behave exactly as it did: existing callers and tests rely on it.
func TestResolve_WithoutHintKeepsTheImperative(t *testing.T) {
	h, err := LoadHomographs("../../data/homographs.json")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := h.Resolve([]string{"Read", "the", "first", "page."}, 0)
	if !ok || got != imperReed {
		t.Fatalf("Resolve without a hint = %q ok=%v, want the imperative %s", got, ok, imperReed)
	}
}

// DiagnoseText must report what synthesis speaks, hint included (#47, #112). Synthesis phonemizes
// each sentence with the hint computed from its paragraph, so compare against exactly that.
func TestNarratedPast_DiagnoseMatchesSynthesis(t *testing.T) {
	p := newTestPhonemizer(t)
	for _, para := range []string{
		"He opened the ledger. Read the first page.",
		"Open the manual. Read the first page.",
		"He picked up the page. Read the first line. Read the second line.",
	} {
		sents := SplitSentences(para)
		hints := sentenceHints(sents)
		var spoken []string // what synthesis phonemizes, sentence by sentence
		for i, s := range sents {
			ph, err := p.phonemizeHinted(s, hints[i])
			if err != nil {
				t.Fatal(err)
			}
			spoken = append(spoken, ph)
		}
		// The "read" in each sentence, as diagnosed from the whole paragraph, must appear in the
		// phonemes synthesis produced for that sentence.
		n := 0
		for _, d := range p.DiagnoseText(para) {
			if !strings.EqualFold(strings.Trim(d.Word, `.,;:!?"'`), "read") {
				continue
			}
			found := false
			for _, sp := range spoken {
				if strings.Contains(sp, d.Phonemes) {
					found = true
				}
			}
			if !found {
				t.Errorf("%q: DiagnoseText says %s for \"read\" #%d but synthesis never produced it (%q)", para, d.Phonemes, n, spoken)
			}
			n++
		}
	}
}
