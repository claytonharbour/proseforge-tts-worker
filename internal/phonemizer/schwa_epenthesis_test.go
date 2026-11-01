//go:build unit

package phonemizer

import "testing"

// Restoring the schwa (#53) fixed 792 words and broke 7, on a 12,000-word paired sample.
// These are those 7. They are pinned rather than filtered because @wayland's proposed guard
// — drop any `ə` adjacent to another vowel — breaks 66 correct words to fix 3 of them:
// `ə` next to a vowel is ordinary English hiatus (`champion ʧˈæmpiən`, `comedian kəmˈidiən`,
// `conspicuous kənspˈɪkjuəs`), firing on 6% of the dictionary. Two rules and ~85 broken words
// to recover seven is a worse trade than the seven.
//
// They belong to #52 (wrong vowel, not missing one). Pinned so the blind spot is a failing
// expectation someone can find, rather than silence — the failure mode
// TestStructuralFlags_DocumentedBlindSpots exists to prevent.
//
// This calls lts.Phonemize directly, which is normally the wrong layer to measure. The reason
// is specific: all seven are gold words, so the pipeline shadows them and a user never hears
// these particular garbles. The shape is only reachable through the pipeline on words the
// dictionaries do not hold — TestSchwaEpenthesisIsLiveOnUnknownWords covers that.
func TestSchwaEpenthesisRegressions(t *testing.T) {
	p := loadTestPhonemizer(t)

	for _, c := range []struct{ word, lts, gold, shape string }{
		// Schwa directly after a vowel — the shape @wayland's four specimens shared.
		{"aisle", "ˈIəl", "ˈIl", "vowel-adjacent"},
		{"broadly", "bɹˈɔədli", "bɹˈɔdli", "vowel-adjacent"},
		{"plea", "plˈiə", "plˈi", "vowel-adjacent"},

		// Schwa inserted into a consonant cluster before `l`, where gold keeps the cluster
		// bare. Invisible to a vowel-adjacency rule — 4 of the 7 regressions are this.
		{"bobbly", "bˈɑbəli", "bˈɑbli", "cluster"},
		{"cyclist", "sˈIkəlɪst", "sˈIklɪst", "cluster"},
		{"halfling", "hˈæfəlɪŋ", "hˈæflɪŋ", "cluster"},
		{"starveling", "stˈɑɹvəlɪŋ", "stˈɑɹvlɪŋ", "cluster"},
	} {
		got := p.lts.Phonemize(c.word)
		if got != c.lts {
			t.Errorf("%s (%s): LTS = %q, pinned as %q — behaviour changed; if this is a fix, "+
				"update the pin and say so on #52 (gold has %q)",
				c.word, c.shape, got, c.lts, c.gold)
		}
		if gold, ok := p.gold.LookupWithSuffix(c.word); !ok || gold != c.gold {
			t.Errorf("%s: gold = %q (present=%v), expected %q — the target moved, so the "+
				"comparison this pin rests on is no longer the one described",
				c.word, gold, ok, c.gold)
		}
	}
}

// The seven above are dictionary words, so the pipeline never lets a user hear them wrong.
// This is the part that does reach a user: the same epenthesis on words no dictionary holds,
// which is every invented name and coinage in a manuscript.
func TestSchwaEpenthesisIsLiveOnUnknownWords(t *testing.T) {
	p := loadTestPhonemizer(t)

	want := map[string]string{
		"gulfling":  "ɡˈʌlfəlɪŋ", // cf. halfling; note LTS also loses the l of "gulf"
		"shelfling": "ʃˈɛlfəlɪŋ",
		"nobbly":    "nˈɑbəli", // cf. bobbly
	}

	for _, d := range p.DiagnoseText("gulfling shelfling nobbly.") {
		expect, ok := want[d.Word]
		if !ok {
			t.Fatalf("unexpected word %q — text and table disagree", d.Word)
		}
		if d.Source != "lts" {
			t.Fatalf("%s resolved from %q — pick a word the dictionaries still do not hold, "+
				"or this stops demonstrating that the shape reaches users", d.Word, d.Source)
		}
		if d.Phonemes != expect {
			t.Errorf("%s = %q, pinned as %q", d.Word, d.Phonemes, expect)
		}
		delete(want, d.Word)
	}
	for w := range want {
		t.Errorf("%s never appeared in the diagnosis", w)
	}
}
