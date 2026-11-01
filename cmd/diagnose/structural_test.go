//go:build unit

package main

import "testing"

// The control set is the load-bearing half of this test. Wayland verified these
// eight names as CORRECT in audio despite all falling to LTS — Corbin alone is
// 2147 occurrences across 13 books. A filter that flags any of them is worse
// than useless, because it would send an ear-check after words that already
// sound right. See proseforge-tts-worker#39.
func TestStructuralFlags_KnownGoodNamesAreNotFlagged(t *testing.T) {
	controls := map[string]string{
		"Corbin":   "kˈɔɹbɪn",
		"Voss":     "vˈɔs",
		"Claire":   "klˈɛɹ",
		"Kinney":   "kˈɪni",
		"Rennick":  "ɹˈɛnɪk",
		"Aldridge": "ˈɔldɹɪʤ",
		"Kessler":  "kˈɛslɜɹ",
		"Liu":      "liˈu",
	}
	for name, ipa := range controls {
		if flags := structuralFlags(ipa); len(flags) > 0 {
			t.Errorf("%s (%s) is verified-correct but was flagged %v", name, ipa, flags)
		}
	}
}

// Ordinary English words, not names. -able is a productive suffix, so this
// defect scales with anything an author writes — which is why the fix is one
// LTS rule rather than a set of dictionary entries.
func TestStructuralFlags_OrdinaryWordDefectsAreFlagged(t *testing.T) {
	defects := map[string]string{
		"hearable":   "hˈɪɹbl",
		"unjammable": "nʤˈæmbl",
	}
	for word, ipa := range defects {
		if flags := structuralFlags(ipa); len(flags) == 0 {
			t.Errorf("%s (%s) is a known defect but was NOT flagged", word, ipa)
		}
	}
}

func TestStructuralFlags_KnownBadNamesAreFlagged(t *testing.T) {
	defects := map[string]string{
		"Castillo":      "kˈæstˈɪlO",
		"Eliza":         "ˈɛlˈIzə",
		"Dunbar":        "dˈʌnbˈɑɹ",
		"Carmen":        "kˈɑɹmn",
		"Darnell":       "dˈɑɹnl",
		"Percival":      "pˈɜɹsvl",
		"Schumann":      "ʃˈumn",
		"Featherbottom": "fˈɛðɜɹbtm",
	}
	for name, ipa := range defects {
		if flags := structuralFlags(ipa); len(flags) == 0 {
			t.Errorf("%s (%s) is a known defect but was NOT flagged", name, ipa)
		}
	}
}

// THE BLIND SPOT IS A CLASS, NOT TWO NAMES: a dropped nucleus behind a LEGAL
// coda. Wayland's framing (#50), and it is better than the version this test
// used to carry — I had two specimens and no class, so it read as a quirk about
// -est rather than a shape.
//
//	purest   pjˈʊɹst   "PYOORST"    missing ə before a legal "st"
//	realest  ɹˈilst    "REELST"     missing ə before a legal "lst"
//	okayest  ˈOkˈAst   "OH-KAYST"   missing ə before a legal "st"
//
// A coda test cannot separate these from `poorest pˈʊɹɪst`, which has the same
// legal ending and is correct. The difference is a nucleus earlier in the word,
// which is not where the rule looks.
//
// ⚠️ okayest is caught here only by DOUBLE-STRESS, incidentally — not by the
// coda rule, and not for the right reason. Do not read its flag as coverage.
//
// Pin the blind spots as blind spots. If a later change starts catching these,
// that is a real improvement — but it must be a deliberate one, with the
// false-positive cost re-measured against the control set above, not a silent
// side effect. Never report this filter's silence as a clean word.
func TestStructuralFlags_DocumentedBlindSpots(t *testing.T) {
	blind := map[string]string{
		// dropped syllable COUNT, structurally well-formed: "MAR-gurt"
		"Margaret": "mˈɑɹɡɜɹt",
		// wrong vowel CHOICE, structurally clean: the Moira lesson
		"Moira": "mˈYɹə",
		// Instances of the class described above — a dropped nucleus behind a
		// legal coda. They must still reach the ear-check, via recurrence rather
		// than via this filter.
		"purest":  "pjˈʊɹst",
		"realest": "ɹˈilst",
	}
	for name, ipa := range blind {
		if flags := structuralFlags(ipa); len(flags) > 0 {
			t.Errorf("%s (%s) is a documented blind spot but was flagged %v — "+
				"if this is intentional, re-measure false positives against the "+
				"control set and update the comment on structuralFlags", name, ipa, flags)
		}
	}
}

// Legal English codas must never be flagged. burnt/worst are the specimens that
// killed the first version of this filter: it used run length alone, so ɹ+CC
// tripped it. They end in an OBSTRUENT and are fine; the rule now keys on a
// trailing sonorant instead. Whilst is here because "lst" is a legal three-
// consonant coda, which run-length would also have caught.
func TestStructuralFlags_LegalCodasAreNotFlagged(t *testing.T) {
	for word, ipa := range map[string]string{
		"heart":  "hˈɑɹt",
		"board":  "bˈɔɹd",
		"park":   "pˈɑɹk",
		"burnt":  "bˈɜɹnt",
		"worst":  "wˈɜɹst",
		"whilst": "wˈIlst",
	} {
		if flags := structuralFlags(ipa); len(flags) > 0 {
			t.Errorf("%s (%s) is a normal English coda but was flagged %v", word, ipa, flags)
		}
	}
}

// Zero-nucleus strings are malformed by definition — there is nothing to
// pronounce. Vågå is issue #25, live in the corpus and previously unflagged
// because it ends in an obstruent. Wayland's catch (#39).
func TestStructuralFlags_ZeroNucleusIsFlagged(t *testing.T) {
	for word, ipa := range map[string]string{
		"Vågå":   "vɡ",
		"Proulx": "pɹlks",
		"LLCs":   "lks",
		"SREs":   "sɹz",
	} {
		flags := structuralFlags(ipa)
		var found bool
		for _, f := range flags {
			if f == "NO-NUCLEUS" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s (%s) has no vowel at all but was not NO-NUCLEUS: %v", word, ipa, flags)
		}
	}
}

// Clitics are correctly vowel-less: gold spells 'd as "d" and 's as "s".
// The length guard is what keeps them out.
func TestStructuralFlags_CliticsAreNotFlagged(t *testing.T) {
	for word, ipa := range map[string]string{"'d": "d", "'s": "s", "'ll": "l"} {
		if flags := structuralFlags(ipa); len(flags) > 0 {
			t.Errorf("clitic %s (%s) was flagged %v", word, ipa, flags)
		}
	}
}
