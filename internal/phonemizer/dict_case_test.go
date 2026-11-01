//go:build unit

package phonemizer

import "testing"

// Gold differentiates 831 key pairs on case (1,673 entries). The loader used to
// lowercase the key, so both wrote the same slot and the winner was whichever
// `range raw` reached last — Go's randomised map order, DECIDED FRESHLY IN
// EVERY PROCESS.
//
// Six separate runs before the fix:
//
//	calm   = kˈɑlm x5, kˈɑm x1
//	polish = pˈɑlɪʃ x3, pˈOlɪʃ x3
//	dram   = dɹˈæm x4, dˈiɹˌæm x2
//
// Narration renders section-by-section in separate processes, so one book could
// say a word two ways. This is the same failure the Keeper's Door timestamp work
// exists to prevent, arriving through the dictionary rather than the text.
//
// ⚠️ In-process repetition CANNOT see this — the dictionary loads once. My
// 4-round in-process determinism test passed throughout. Cross-process is the
// only control that works, which is why Wayland's cross-build runs showed
// variance while my in-process rounds showed stability on the same words.
func TestGoldCasePairsAreDistinctAndCorrect(t *testing.T) {
	p := loadTestPhonemizer(t)
	for _, c := range []struct{ word, want, why string }{
		{"August", "ˈɔɡəst", "the month"},
		{"august", "ɔɡˈʌst", "the adjective"},
		{"DRAM", "dˈiɹˌæm", "the memory"},
		{"dram", "dɹˈæm", "the measure"},
		{"CALM", "kˈɑlm", "spelling pronunciation"},
		{"calm", "kˈɑm", "ordinary reading"},
	} {
		got, ok := p.gold.Lookup(c.word)
		if !ok {
			t.Errorf("%s not found", c.word)
			continue
		}
		if got != c.want {
			t.Errorf("%s (%s) = %q, want %q — the case pair is being folded", c.word, c.why, got, c.want)
		}
	}
}

// An unknown-case word must still fall back to lowercase, or every capitalised
// sentence-initial word stops resolving.
func TestUnknownCaseFallsBackToLowercase(t *testing.T) {
	p := loadTestPhonemizer(t)
	lower, _ := p.gold.Lookup("table")
	if upper, ok := p.gold.Lookup("Table"); !ok || upper != lower {
		t.Errorf("\"Table\" = %q (found=%v), want the lowercase reading %q", upper, ok, lower)
	}
}
