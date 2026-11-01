//go:build unit

package phonemizer

import (
	"os"
	"strings"
	"testing"
)

// #49 (@wayland): the same binary on the same corpus resolved differently
// across runs. Cause: the homograph resolver iterated a Go map, and
// `dist < bestDist` keeps the first-seen candidate on a TIE — so an
// equal-distance tie was decided by randomised map order.
//
// Specimen: `wound` flipping between wuːnd (the injury) and wWnd (past tense
// of wind). A meaning change in prose, and it made narration unreproducible —
// sections render in separate passes, so s5 and s10 could disagree.
//
// Go randomises map iteration on EVERY range, so repeated iterations in one
// process are the reproduction. 200 rounds caught it on round 1.
//
// ⚠️ #49's fix has TWO halves and they are INDEPENDENTLY SUFFICIENT. Measured
// 2026-08-18 by disabling each in turn (each injection compile-checked first,
// because an injection that breaks the build reports the compiler's exit code,
// not the guard's):
//
//	sort.Strings(prons) replaced with a no-op ordering   → 0 of 14 probes flip
//	tie-break toward entry.Default removed               → 0 of 14 probes flip
//	BOTH removed                                         → 5 of 14 flip, ~171/29
//
// 🛑 So either half alone holds the line, which is the dangerous shape: a
// refactor can delete one, verify determinism honestly, and be correct — and a
// later refactor can delete the other, verify nothing locally, and reintroduce
// #49. Two individually-safe changes composing into a regression. Neither
// mechanism is load-bearing on its own, so do not treat a green run after
// removing one as licence to remove the other.
//
// 📌 And before restoring the old behaviour looks attractive: under the bug,
// "A minute difference" resolved CORRECTLY 171 times in 200. #49 made it
// deterministically wrong there. Still the right trade — @wayland measured the
// equidistant class at 29 correct of 30 on real text (#65) — but what #49
// bought was reproducibility, not accuracy, and it should not be remembered as
// though it were both.
func TestHomographResolutionIsDeterministic(t *testing.T) {
	p := loadTestPhonemizer(t)
	text := `The wound was deep. He wound the clock before the wound closed.
	She read the letter and will read it again. He led the way; the lead was heavy.
	They live where they will live. He took a bow to the crowd and bowed his head.`

	first := ""
	for i := 0; i < 300; i++ {
		var b strings.Builder
		for _, d := range p.DiagnoseText(text) {
			b.WriteString(d.Word + "=" + d.Phonemes + "|")
		}
		if got := b.String(); i == 0 {
			first = got
		} else if got != first {
			a, c := strings.Split(first, "|"), strings.Split(got, "|")
			for j := range a {
				if j < len(c) && a[j] != c[j] {
					t.Fatalf("round %d: %q vs %q — map-order dependence is back", i, a[j], c[j])
				}
			}
			t.Fatalf("round %d differs in length", i)
		}
	}
}

// The same, over real corpus prose rather than constructed sentences. My
// hand-written probe ran 400 rounds clean and found nothing; the real corpus
// failed on round 1. Constructed text did not contain the tie.
func TestDeterminismOnRealCorpusSample(t *testing.T) {
	raw, err := os.ReadFile("testdata/homograph_corpus_sample.txt")
	if err != nil {
		t.Skipf("corpus sample not present: %v", err)
	}
	p := loadTestPhonemizer(t)
	first := ""
	for i := 0; i < 100; i++ {
		var b strings.Builder
		for _, d := range p.DiagnoseText(string(raw)) {
			b.WriteString(d.Word + "=" + d.Phonemes + "|")
		}
		if got := b.String(); i == 0 {
			first = got
		} else if got != first {
			a, c := strings.Split(first, "|"), strings.Split(got, "|")
			for j := range a {
				if j < len(c) && a[j] != c[j] {
					t.Fatalf("round %d: %q vs %q", i, a[j], c[j])
				}
			}
		}
	}
}
