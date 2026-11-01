//go:build unit

package phonemizer

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// #25, re-measured 2026-08-19 after @wayland's post-banking count reduced the
// live class to 5 distinct / 7 occurrences, all IPA quoted inside prose in one
// story about pronunciation, and none of them a name.
//
// 🛑 THE CHARACTERS BEING DROPPED ARE VALID KOKORO PHONEMES. That is the part
// the ticket's framing missed for months. It reads as "the engine cannot say
// these sounds"; the truth is the model can, and the TEXT PATH strips them
// before the tokenizer ever sees them:
//
//	ɛ → token  86      bɛd   → bd     the ɛ contributes nothing
//	ɪ → token 102      lɪvd  → lvd    the ɪ contributes nothing
//	ɔ → token  76
//
// Exposure, partitioned by TestDroppedVocabPartitionIsPinned below: of 66 vocab
// symbols with no asciiFold entry, 51 are base phoneme letters.
//
// So the fix is NOT transliteration. @wayland is right that folding bɛd→bed is
// wrong in a sentence whose whole purpose is to contrast the two — but that
// argument only rules out the fold, and the tokenizer route survives it: emit
// b-ɛ-d as phonemes and the contrast is preserved exactly.
//
// ⚠️ The ambiguity that stops this being a one-line change: Akan and Yoruba use
// ɛ and ɔ as ORTHOGRAPHY, not as IPA. A rule keyed on "all characters are valid
// phonemes" would read an Akan word as a phonetic transcription. That is
// roughly right for Akan, whose spelling is close to phonemic, and it is a
// guess. It needs an ear before it ships, and @clayton has four clips waiting.
//
// This test therefore PINS TODAY'S BEHAVIOUR rather than asserting the fix. It
// is meant to go red when someone implements the passthrough — at which point
// the assertion below is what tells them the old behaviour was deliberate and
// measured, not an oversight they are free to reinterpret.
func TestIPAInProseIsDroppedDespiteBeingValidTokens(t *testing.T) {
	raw, err := os.ReadFile("../../data/token_map.json")
	if err != nil {
		t.Skipf("token map not present: %v", err)
	}
	var tm struct {
		Vocab map[string]int64 `json:"vocab"`
	}
	if err := json.Unmarshal(raw, &tm); err != nil {
		t.Fatalf("token_map.json: %v", err)
	}

	// The premise. If these ever leave the vocabulary the rest of this test is
	// measuring something else entirely, so check it rather than assume it.
	for _, sym := range []string{"ɛ", "ɪ", "ɔ"} {
		if _, ok := tm.Vocab[sym]; !ok {
			t.Fatalf("premise broken: %q is no longer a Kokoro token, so #25's "+
				"reframing (model can voice it, text path drops it) no longer holds", sym)
		}
	}

	p := loadTestPhonemizer(t)
	cases := []struct {
		word      string
		dropped   string
		stillSays string
	}{
		{"bɛd", "ɛ", "bd"},
		{"lɪvd", "ɪ", "lvd"},
	}

	for _, c := range cases {
		got := ""
		for _, d := range p.DiagnoseText(c.word) {
			if d.Word == c.word {
				got = d.Phonemes
			}
		}
		if got == "" {
			t.Fatalf("%q produced no diagnosis row — the specimen did not reach the "+
				"phonemizer, so a pass here would mean nothing", c.word)
		}
		if strings.Contains(got, c.dropped) {
			t.Errorf("%q now renders %q, which CONTAINS %q — the IPA passthrough appears "+
				"to be implemented.\n"+
				"That is the intended outcome, not a regression: update this pin, and see "+
				"#25 for the Akan/Yoruba orthography ambiguity that has to be settled by ear first.",
				c.word, got, c.dropped)
			continue
		}
		if got != c.stillSays {
			t.Errorf("%q renders %q, expected the documented drop %q — behaviour changed "+
				"in some third way, which #25's measurement does not describe",
				c.word, got, c.stillSays)
		}
	}
}

// TestDroppedVocabPartitionIsPinned fixes the number three of us quoted
// differently. I published 52, @wayland got 63 mechanically and ~51 excluding
// modifiers. 51 is right; my 52 subtracted five modifier letters (ˈ ˌ ː ʰ ʲ)
// when there are six — ᵝ is one too.
//
// 🛑 AND NO UNICODE-CATEGORY RULE CAN MAKE THIS SPLIT. ᵊ is category Lm, exactly
// like ˈ — and ᵊ is token 42, the reduced schwa #71 is about, while ˈ is a
// stress mark that is not a phoneme at all. Sorting by category puts them in the
// same bucket and is therefore wrong, which is why the three sets below are
// written out by hand rather than derived. That is a phonetic judgement and it
// should look like one.
//
// The partition is pinned so the figure cannot drift again while being requoted.
func TestDroppedVocabPartitionIsPinned(t *testing.T) {
	raw, err := os.ReadFile("../../data/token_map.json")
	if err != nil {
		t.Skipf("token map not present: %v", err)
	}
	var tm struct {
		Vocab map[string]int64 `json:"vocab"`
	}
	if err := json.Unmarshal(raw, &tm); err != nil {
		t.Fatalf("token_map.json: %v", err)
	}

	stressLength := "ˈˌː"
	articulatory := "ʰʲᵝ̃"
	punctProsody := "—…“”→↓↗↘"

	var base, stress, artic, punct int
	for sym := range tm.Vocab {
		r := []rune(sym)
		if len(r) != 1 || r[0] < 128 {
			continue
		}
		if _, folded := asciiFold[r[0]]; folded {
			continue
		}
		switch {
		case strings.ContainsRune(stressLength, r[0]):
			stress++
		case strings.ContainsRune(articulatory, r[0]):
			artic++
		case strings.ContainsRune(punctProsody, r[0]):
			punct++
		default:
			base++
		}
	}

	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"base phoneme letters", base, 51},
		{"stress/length marks", stress, 3},
		{"articulatory modifiers", artic, 4},
		{"punctuation/prosody", punct, 8},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d — the exposure figure quoted on #25 has moved. "+
				"Update the ticket and the room BEFORE changing this number, because it has "+
				"already been requoted three ways once.", c.name, c.got, c.want)
		}
	}
	if total := base + stress + artic + punct; total != 66 {
		t.Errorf("partition totals %d, want 66 — a vocab symbol is unaccounted for", total)
	}
}
