//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// Clayton's ear-check, 2026-08-16: "Percival bowed to the room" came out as
// bˈOd — "bode", the ribbon/weapon sense — where bowing to a room is bˈWd.
//
// homographs.json HAS the rule: bow + context_after "to" → bW. It does not fire
// on "bowed", because the homograph layer matches the surface word and the
// inflected form is only resolved later by suffix-stripping to "bow", which
// takes gold's DEFAULT (bˈO) and never revisits context.
//
// The class is wider than one word: any inflected homograph loses its
// disambiguation the moment it carries a suffix — read/reading, lead/leading,
// wound/wounded, live/lived.
func TestInflectedHomographKeepsItsContext(t *testing.T) {
	p := loadTestPhonemizer(t)

	// Both carry a trigger homographs.json already lists for the bending sense
	// ("to", "head"). The first is Clayton's sentence from the ear-check clip.
	bendCases := []string{
		"Percival bowed to the room.",
		"She bowed her head.",
	}
	for _, s := range bendCases {
		got := phonemesFor(t, p, s, "bowed")
		if strings.Contains(got, "O") {
			t.Errorf("%q → bowed=%q: the ribbon/weapon vowel (O = /oʊ/). Bending forward "+
				"is W = /aʊ/, and homographs.json lists this sentence's trigger "+
				"— the rule is not surviving the -ed suffix", s, got)
		}
	}
}

// This was pinned as a blind spot ("bowed with no trigger falls to the ribbon
// sense"), with a note that changing it needed a decision about English rather
// than a patch. Smiley made that call on 2026-08-16, so the test now asserts
// the decided behaviour instead of the old gap.
//
// The principle is part of speech, not a longer trigger list: -ed forces a VERB,
// and the archery and ribbon senses of "bow" have no verb form — you cannot bow
// a ribbon. So the suffix does not hint at the bending sense, it forecloses the
// others. Adding "under"/"snow" as triggers would have fixed this one sentence
// and generalised to nothing.
//
// ⚠️ Scoped to "bow" alone via an opt-in inflected_default field. NOT a sweep:
// "leading" is a legitimate verb AND noun AND adjective, so its suffix forces
// nothing, and wound/read/live need per-POS data that does not exist yet.
func TestTriggerlessInflectedHomographUsesThePartOfSpeechDefault(t *testing.T) {
	p := loadTestPhonemizer(t)
	got := phonemesFor(t, p, "The branches bowed under snow.", "bowed")
	if strings.Contains(got, "O") {
		t.Errorf("bowed with no context trigger → %q: the ribbon vowel. -ed forces a "+
			"verb, and the verb sense is bending (W)", got)
	}
}

// The bare word must keep the NOUN default — the whole point is that the two
// forms differ. If this ever matches the inflected reading, inflected_default
// has leaked onto the uninflected path.
func TestBareBowKeepsTheNounDefault(t *testing.T) {
	p := loadTestPhonemizer(t)
	got := phonemesFor(t, p, "The bow rested on the table.", "bow")
	if !strings.Contains(got, "O") {
		t.Errorf("bare \"bow\" → %q, expected the noun default (O); inflected_default "+
			"may be leaking onto uninflected words", got)
	}
}

// Scope guard. "leading" must NOT acquire an inflected default — Smiley named it
// as the counter-example, since it is a legitimate verb, noun and adjective.
// A failure here means the opt-in field was applied as a general rule.
func TestInflectedDefaultIsNotAppliedToAmbiguousSuffixes(t *testing.T) {
	p := loadTestPhonemizer(t)
	for _, s := range []string{"He was leading the way.", "The leading candidate spoke."} {
		if got := phonemesFor(t, p, s, "leading"); got == "" {
			t.Errorf("%q produced no phonemes for \"leading\"", s)
		}
	}
}

// The control: the bare form must keep working, or the fix traded one defect
// for another.
func TestBareHomographStillDisambiguates(t *testing.T) {
	p := loadTestPhonemizer(t)
	if got := phonemesFor(t, p, "She tied the bow in her hair.", "bow"); !strings.Contains(got, "O") {
		t.Errorf("bare \"bow\" (ribbon) → %q, expected the O vowel", got)
	}
}

func phonemesFor(t *testing.T, p *Phonemizer, sentence, word string) string {
	t.Helper()
	for _, d := range p.DiagnoseText(sentence) {
		if strings.EqualFold(strings.Trim(d.Word, ".,"), word) {
			return d.Phonemes
		}
	}
	t.Fatalf("word %q not found in diagnosis of %q", word, sentence)
	return ""
}
