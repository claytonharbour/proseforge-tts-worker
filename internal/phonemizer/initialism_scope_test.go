//go:build unit

package phonemizer

import "testing"

// #68 / #62. Three fix directions for the shadowed letter-name `A` were refuted,
// each by a concrete specimen (see letter_a_shadow_test.go). This pins a FOURTH
// that survives all of them, and the reason it survives is worth stating exactly:
//
// 🛑 It keys on the dot pattern BEFORE abbreviationDotRe consumes it. Every
// refuted rule ran at phonemizeWord, by which point "A.R." and "A B movie" are
// both "capital A followed by a lone letter" and nothing can separate them. At
// normalization time they are not remotely alike.
//
//	abbreviationDotRe = `\b([A-Z]\.){2,}`
//
//	MATCH    A.R. · U.S.A. · A.M.              ← the letter-name cases
//	NO       One. At. A. Time.                 ← @wayland's CORPUS counterexample
//	         A B movie played.                 ← sentence-initial article
//	         He watched a C major.             ← @wayland's language counterexample
//	         Grade A beef. · 1A. · 2A.         ← periodless, #62's refused class
//	         It was 9 a.m. sharp.              ← lowercase, already banked
//
// ⚑ The two counterexamples that killed direction three do not even look like
// initialisms here. "A B movie" has no periods at all; "One. At. A. Time." has
// periods but never two adjacent single-capital-plus-dot pairs, because a space
// breaks the run.
//
// ⚠️ THIS DOES NOT REOPEN #62. That ticket is the PERIODLESS class — "Exhibit A",
// "Grade A", "Plan A" — which has no dot pattern to key on and stays refused at
// ~84:1. This covers the dotted subclass only, which is #68's A.R. (8 occ) and
// the uppercase A.M./P.M. whose corpus exposure @wayland measured at ZERO.
//
// ⛔ Not implemented. Emitting letter-name readings from here needs either a
// custom-dictionary oracle inside NormalizeTextWithWords (its isEnglishWord sees
// gold and silver only) or a way to mark the emitted letters so the article rule
// skips them. Both are signature changes on a shared path. This test exists so
// the SOUNDNESS of the scope is settled before anyone spends that.
func TestDottedInitialismScopeExcludesEveryKnownCounterexample(t *testing.T) {
	mustMatch := []string{
		"A.R. Sinclair called.",
		"She left the U.S.A.",
		"It was 9 A.M. sharp.",
	}
	mustNotMatch := []string{
		"One. At. A. Time.",     // in the corpus — the article with a full stop
		"A B movie played.",     // sentence-initial article before a lone letter
		"He watched a C major.", // lowercase article before a lone letter
		"Grade A beef.",         // #62's periodless class
		"Apartment 1A.",
		"Mrs. Polk, 2A.",
		"It was 9 a.m. sharp.", // lowercase, already resolved by the bank
	}

	for _, s := range mustMatch {
		if !abbreviationDotRe.MatchString(s) {
			t.Errorf("%q no longer matches abbreviationDotRe — the dotted-initialism "+
				"scope has narrowed and #68's fix path may no longer reach its own specimen", s)
		}
	}
	for _, s := range mustNotMatch {
		if abbreviationDotRe.MatchString(s) {
			t.Errorf("%q NOW MATCHES abbreviationDotRe (%q) — the scope has widened onto a "+
				"known counterexample, and a letter-name rule built on it would mispronounce "+
				"this sentence. This is the assertion that makes the scope safe to build on.",
				s, abbreviationDotRe.FindString(s))
		}
	}
}
