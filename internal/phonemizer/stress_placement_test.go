//go:build unit

package phonemizer

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

// #120: the narration said "a project" for "project" (and "a minute" for "minute").
//
// 🛑 Cause: homographs.json spelled the singular noun `ˈpɹɑʤɛkt`, with the stress mark in front
// of the first CONSONANT. Every dictionary Kokoro was trained against (Misaki's gold and silver)
// puts the mark directly in front of the stressed VOWEL: `pɹˈɑʤɛkt`. The model read the
// misplaced mark as a sound of its own. Measured 2026-10-22 on two voices (am_michael, the voice
// of the audiobook where it was reported, and af_sarah): with the marks moved, ten words that
// the old data garbled or padded ("though a project", "I-O-Bidjik", "egg conduct", "connectent",
// "Mid Desert", "potty") came out right, and six control sentences were byte-identical.
//
// ⚑ The tokenizer does not complain about this: a stress mark in the wrong place is a perfectly
// good token. It is one of the defects this repo has no other guard for, so it is pinned here.
//
// The rule being pinned: every ˈ and ˌ in every phoneme string we ship is immediately followed by
// a vowel symbol. Gold and silver already satisfy it (0 violations in about 184,000 entries,
// measured 2026-10-22), so the test passes on the dictionaries and bites only on the files we
// hand-edit: homographs.json and custom_dict.json.

// vowelSymbols are the symbols that carry a syllable in Kokoro's vocabulary: the diphthong
// capitals (A I O Q W Y), the plain vowels, and ᵊ (the syllabic schwa). Written out rather than
// derived, because no rule separates a vowel from a consonant in the vocabulary file.
const vowelSymbols = "AIOQWYaeiouyɑɐɒæɔəɚɛɜɨɪɯøœʊʌɤᵻᵊ"

// stressViolations returns the rune index of every ˈ or ˌ in s that is NOT directly followed by
// a vowel symbol.
func stressViolations(s string) []int {
	var bad []int
	r := []rune(s)
	for i, c := range r {
		if c != 'ˈ' && c != 'ˌ' {
			continue
		}
		if i+1 >= len(r) || !strings.ContainsRune(vowelSymbols, r[i+1]) {
			bad = append(bad, i)
		}
	}
	return bad
}

// The instrument first: planted strings must be flagged and the corrected forms must not be,
// otherwise a green result below proves nothing.
func TestStressViolationsFindsPlantedMisplacedMarks(t *testing.T) {
	flagged := []string{
		"ˈpɹɑʤɛkt",   // the #120 string: mark before the consonant cluster
		"mIˈnjuːt",   // mark before a consonant in the middle
		"ˈɹɛzəmˌA",   // the old résumé: first mark before ɹ
		"pɹˈɑʤɛkt ˈ", // a mark with nothing after it
	}
	for _, s := range flagged {
		if len(stressViolations(s)) == 0 {
			t.Errorf("%q should be flagged and was not", s)
		}
	}
	clean := []string{"pɹˈɑʤɛkt", "mInjˈuːt", "ɹˈɛzəmˌA", "ɹəkˈɔɹd", "ˈɑbʤɛkt", "", "pɹˈɑʤˌɛkt"}
	for _, s := range clean {
		if v := stressViolations(s); len(v) != 0 {
			t.Errorf("%q should be clean and was flagged at %v", s, v)
		}
	}
}

// TestEveryStressMarkSitsDirectlyBeforeAVowel reads all four phoneme files and fails with the
// offending words. Honours MISAKI_DATA_DIR so it can be red-proofed against a scratch copy
// instead of the shared data/.
func TestEveryStressMarkSitsDirectlyBeforeAVowel(t *testing.T) {
	dir := testDataDir()
	type hit struct{ file, word, kind, phonemes string }
	var hits []hit
	checked := map[string]int{}

	scan := func(file, word, kind, s string) {
		checked[file]++
		if len(stressViolations(s)) > 0 {
			hits = append(hits, hit{file, word, kind, s})
		}
	}

	for _, f := range []string{"custom_dict.json", "us_gold.json", "us_silver.json"} {
		raw, err := os.ReadFile(dir + "/" + f)
		if err != nil {
			t.Skipf("%s absent: %v", f, err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for word, v := range m {
			if strings.HasPrefix(word, "_") {
				continue // metadata, not a word
			}
			var s string
			if json.Unmarshal(v, &s) == nil {
				scan(f, word, "entry", s)
				continue
			}
			var pos map[string]string
			if json.Unmarshal(v, &pos) == nil {
				for k, ps := range pos {
					scan(f, word, k, ps)
				}
			}
		}
	}

	raw, err := os.ReadFile(dir + "/homographs.json")
	if err != nil {
		t.Skipf("homographs.json absent: %v", err)
	}
	var homo map[string]json.RawMessage
	if err := json.Unmarshal(raw, &homo); err != nil {
		t.Fatalf("homographs.json: %v", err)
	}
	for word, v := range homo {
		if strings.HasPrefix(word, "_") {
			continue
		}
		var e struct {
			Default        string                     `json:"default"`
			Pronunciations map[string]json.RawMessage `json:"pronunciations"`
		}
		if err := json.Unmarshal(v, &e); err != nil {
			t.Fatalf("homographs.json %s: %v", word, err)
		}
		scan("homographs.json", word, "default", e.Default)
		for p := range e.Pronunciations {
			scan("homographs.json", word, "reading", p)
		}
	}

	// Positive control on the population: the scan must have looked at something in each file,
	// or "no violations" would also be what an empty or unreadable file returns.
	for _, f := range []string{"custom_dict.json", "homographs.json", "us_gold.json", "us_silver.json"} {
		if checked[f] == 0 {
			t.Fatalf("%s: no phoneme strings were checked, so this test measured nothing", f)
		}
	}

	if len(hits) > 0 {
		const show = 25
		var b strings.Builder
		for i, h := range hits {
			if i == show {
				b.WriteString("\n  ... and " + strconv.Itoa(len(hits)-show) + " more")
				break
			}
			b.WriteString("\n  " + h.file + "  " + h.word + " [" + h.kind + "]  " + h.phonemes)
		}
		t.Errorf("%d phoneme string(s) put a stress mark in front of something that is not a vowel "+
			"(#120: the model reads a misplaced mark as a sound of its own). Move each mark to sit "+
			"directly before its stressed vowel, as gold does:%s", len(hits), b.String())
	}
}

// The behaviour the ticket reported, through the real path: the singular noun carries its stress
// on the vowel, and the string the old data produced does not come back.
func TestProjectAndMinuteAreSpokenWithStressOnTheVowel(t *testing.T) {
	p := newTestPhonemizer(t)
	for _, c := range []struct{ text, want, notWant string }{
		{"Create the project.", "pɹˈɑʤɛkt", "ˈpɹɑʤɛkt"},
		{"Each minute counts.", "mˈɪnɪt", "ˈmɪnɪt"},
	} {
		got, err := p.Phonemize(c.text)
		if err != nil {
			t.Fatalf("Phonemize(%q): %v", c.text, err)
		}
		if !strings.Contains(got, c.want) {
			t.Errorf("Phonemize(%q) = %q, want it to contain %q", c.text, got, c.want)
		}
		if strings.Contains(got, c.notWant) {
			t.Errorf("Phonemize(%q) = %q contains the misplaced form %q (#120)", c.text, got, c.notWant)
		}
	}
}
