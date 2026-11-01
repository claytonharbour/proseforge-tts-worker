//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// #71 asks whether LTS's inability to emit ᵊ leaks into inflected words: a stem that
// gold spells with ᵊ, inflected into a form gold does not list, which would then fall
// through to LTS and come back with a full ə instead.
//
// It does not, for the six suffixes the dictionary handles — and the reason is
// structural rather than lucky. LookupWithSuffixRule returns `stemIPA + suffix`, so
// the stem's phonemes are carried through byte for byte and ᵊ survives by construction.
// LTS is never consulted for these.
//
// ⚠️ This pins the MECHANISM, not the audio. Whether ᵊ and ə sound different, and
// whether Kokoro treats tokens 42 and 83 differently, is still unmeasured on #71.
func TestSuffixBuiltInflectionsPreserveReducedSchwa(t *testing.T) {
	p := newTestPhonemizer(t)
	goldForBases = p.gold

	// Every suffix LookupWithSuffixRule knows, paired with a builder for the form.
	suffixes := []struct {
		rule  string
		build func(string) string
	}{
		{"-s", func(w string) string { return w + "s" }},
		{"-es", func(w string) string { return w + "es" }},
		{"-ed", func(w string) string { return w + "ed" }},
		{"-ing", func(w string) string { return w + "ing" }},
		{"'s", func(w string) string { return w + "'s" }},
	}

	// Collect gold stems whose reading carries ᵊ and which gold does NOT already list
	// in inflected form — the population #71 identifies as the only one where both a
	// gold reading and an LTS reading could exist.
	var stems []string
	for word, ipa := range p.gold.entries {
		if !strings.Contains(ipa, "ᵊ") || !isPlainWord(word) {
			continue
		}
		stems = append(stems, word)
		if len(stems) >= 400 {
			break
		}
	}
	if len(stems) == 0 {
		t.Fatal("no gold entries contain ᵊ — the test is not exercising its subject")
	}

	checked, preserved := 0, 0
	for _, stem := range stems {
		stemIPA := p.gold.entries[stem]
		for _, s := range suffixes {
			form := s.build(stem)
			if _, direct := p.gold.entries[form]; direct {
				continue // gold lists it; not the built-form case
			}
			ipa, rule, ok := p.gold.LookupWithSuffixRule(form)
			if !ok || rule == "" {
				continue // this suffix rule did not fire for this stem
			}
			// ⚠️ The rule may have resolved to a DIFFERENT base than ours and been
			// right to: "morales" strips to "morale" (məɹˈæl), a real gold word, not
			// to "moral" (mˈɔɹᵊl) — the "-s" rule fires before "-es". So only assert
			// when the rule that fired actually stripped back to OUR stem.
			//
			// ⛔ Deciding that by `strings.HasPrefix(ipa, stemIPA)` — my first fix —
			// makes this test BLIND to its own subject: mutate the suffix path to
			// rewrite ᵊ as ə and the prefix stops matching, so the case is skipped
			// rather than failed. Red-proofing caught it. The base comparison below
			// does not touch the IPA, so it cannot be defeated that way.
			if !resolvesToStem(form, rule, stem) {
				continue
			}
			checked++
			if strings.Contains(ipa, "ᵊ") {
				preserved++
				continue
			}
			t.Errorf("%q -> %q via rule %q LOST the stem's ᵊ (stem %q = %q)",
				form, ipa, rule, stem, stemIPA)
		}
	}

	if checked == 0 {
		t.Fatal("no suffix rule fired for any ᵊ stem — this test would pass vacuously, " +
			"which is the failure it exists to prevent")
	}
	t.Logf("suffix-built inflections of ᵊ stems: %d checked, %d preserved ᵊ", checked, preserved)
}

// The other half of #71's question: which suffixes are NOT handled, and therefore DO
// fall through to LTS where ᵊ cannot be produced. This is a scope statement, not a
// defect — it records where the exposure actually is, so nobody has to re-derive it.
func TestUnhandledSuffixesAreTheOnlyReducedSchwaExposure(t *testing.T) {
	p := newTestPhonemizer(t)

	var stem, stemIPA string
	for word, ipa := range p.gold.entries {
		if strings.Contains(ipa, "ᵊ") && isPlainWord(word) && len(word) > 4 {
			stem, stemIPA = word, ipa
			break
		}
	}
	if stem == "" {
		t.Fatal("no suitable ᵊ stem found")
	}

	handled := []string{"s", "es", "ed", "ing"}
	unhandled := []string{"ly", "ness", "ment", "er", "est"}

	var reached, missed []string
	for _, suf := range append(append([]string{}, handled...), unhandled...) {
		form := stem + suf
		if _, direct := p.gold.entries[form]; direct {
			continue
		}
		if _, rule, ok := p.gold.LookupWithSuffixRule(form); ok && rule != "" {
			reached = append(reached, suf)
		} else {
			missed = append(missed, suf)
		}
	}

	t.Logf("stem %q = %q", stem, stemIPA)
	t.Logf("  suffix rules that fire (ᵊ preserved): %v", reached)
	t.Logf("  no rule, would reach LTS (ᵊ lost):    %v", missed)

	if len(reached) == 0 {
		t.Error("no suffix rule fired at all — the dictionary path is not being exercised")
	}
	if len(missed) == 0 {
		t.Error("every suffix was handled, so this test cannot show where the exposure is; " +
			"if that is genuinely true, #71's inflected slice is empty and the ticket should say so")
	}
}

// isPlainWord excludes gold's affix entries (`-able`, `-ing`) and anything with
// punctuation. Concatenating a suffix onto `-able` produces `-ableed`, which is not a
// word and would inflate any count built on it — an error I made before writing this.
func isPlainWord(w string) bool {
	if w == "" || strings.HasPrefix(w, "-") || strings.HasSuffix(w, "-") {
		return false
	}
	for _, r := range w {
		if (r < 'a' || r > 'z') && r != '\'' {
			return false
		}
	}
	return true
}

// resolvesToStem reports whether the suffix rule that fired stripped `form` back to
// `stem` specifically, rather than to some other real word. It works on SPELLING only
// — deliberately, so that a mutation to the phoneme path cannot make it skip the case
// it is meant to catch.
func resolvesToStem(form, rule, stem string) bool {
	switch rule {
	case "-s", "'s":
		n := 1
		if rule == "'s" {
			n = 2
		}
		return len(form) > n && form[:len(form)-n] == stem
	case "-es":
		return len(form) > 2 && form[:len(form)-2] == stem
	case "-ies":
		return len(form) > 3 && form[:len(form)-3]+"y" == stem
	case "-ing":
		return firstBaseInGold(ingBases(form)) == stem
	case "-ed":
		return firstBaseInGold(edBases(form)) == stem
	}
	return false
}

// firstBaseInGold mirrors LookupWithSuffixRule's own resolution: it returns the FIRST
// candidate base the dictionary lists, which is the one that actually answered.
//
// ⚠️ Being merely PRESENT in the candidate list is not enough, and that was my second
// wrong discriminator. "musicaled" yields candidates ["musicale", "musical"]; both are
// real gold words, "musicale" is tried first and wins, and asking only "is my stem in
// the list" reported a lost ᵊ for a lookup that never consulted my stem.
var goldForBases *MisakiDict

func firstBaseInGold(cands []string) string {
	for _, b := range cands {
		if _, ok := goldForBases.entries[b]; ok {
			return b
		}
	}
	return ""
}
