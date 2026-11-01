//go:build unit

package phonemizer

import "testing"

// Every letter of the alphabet has a spoken NAME, and the engine reaches it by two
// different routes depending on how the letter appears:
//
//	single letter   "The W Group"   → gold holds "W" itself → dˈʌbᵊlju
//	initialism      "The AR Group"  → split into letter names → ay + ar, which
//	                                  are NOT in gold and fall to LTS
//
// This pins the first route for all 26. The second is #82's other half.
//
// ⚑ WHY THE SOURCE ASSERTION IS LOAD-BEARING, not decoration.
// A test that checked only phonemes would still pass if someone banked these in
// custom_dict — and banking them is precisely the change bank_redundancy_test
// refuses, because a redundant entry SILENTLY COVERS a regression in the layer
// beneath it. #82 proposed that bank; the guard caught it; this is what the fix
// became. Asserting WHICH LAYER answers is what makes an LTS or gold regression
// visible instead of absorbed.
//
// ⛔ `A` IS PINNED WRONG ON PURPOSE. See #86.
// gold holds "A": ˈA — the correct letter name — and phonemizer.go:285 returns
// the article schwa for any lower=="a" before gold is ever consulted, with no
// case check and no sentence-position check. So "Plan A worked" speaks as
// "Plan UH worked". This test records ə because that is what SHIPS, not because
// it is right: pinning current behaviour means the test goes RED when #86 is
// fixed, which is the correct signal, and it puts the defect where anyone
// touching that branch will read it.
func TestLetterNamesResolveWithTheirSource(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct {
		letter     string
		want       string
		wantSource string
		note       string
	}{
		{"A", "ə", "builtin", "⛔ #86 — the ARTICLE reading, shadowing gold's ˈA. " +
			"Wrong for \"Plan A\"/\"Section A\"; correct for \"A man walked in\". " +
			"Pinned as-shipped so a fix turns this red"},
		{"B", "bˈi", "gold", ""},
		{"C", "sˈi", "gold", ""},
		{"D", "dˈi", "gold", ""},
		{"E", "ˈi", "gold", "note ee (the spelled name) is NOT here — it falls to LTS"},
		{"F", "ˈɛf", "gold", ""},
		{"G", "ʤˈi", "gold", ""},
		{"H", "ˈAʧ", "gold", ""},
		{"I", "ˈI", "gold", ""},
		{"J", "ʤˈA", "gold", ""},
		{"K", "kˈA", "gold", ""},
		{"L", "ˈɛl", "gold", ""},
		{"M", "ˈɛm", "gold", ""},
		{"N", "ˈɛn", "gold", ""},
		{"O", "ˈO", "gold", ""},
		{"P", "pˈi", "gold", ""},
		{"Q", "kjˈu", "gold", ""},
		{"R", "ˈɑɹ", "gold", "the LETTER; the spelled name `ar` is the LTS one"},
		{"S", "ˈɛs", "gold", ""},
		{"T", "tˈi", "gold", ""},
		{"U", "jˈu", "gold", ""},
		{"V", "vˈi", "gold", ""},
		{"W", "dˈʌbᵊlju", "gold", "the only multi-syllable letter name"},
		{"X", "ˈɛks", "gold", ""},
		{"Y", "wˈI", "gold", ""},
		{"Z", "zˈi", "gold", ""},
	}

	if len(cases) != 26 {
		t.Fatalf("the alphabet has 26 letters, this table has %d — a missing row is a "+
			"letter nobody is checking, which is the failure this test exists to prevent",
			len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.letter, func(t *testing.T) {
			// One letter per sentence so the probe isolates a single occurrence.
			text := "The " + tc.letter + " Group met."

			if src := sourceForWord(t, p, text, tc.letter); src != tc.wantSource {
				t.Errorf("%q resolved from %q, want %q — the phoneme assertion below may "+
					"hold for the WRONG REASON. A letter answered by `custom` means someone "+
					"banked it, which masks regressions in the layer beneath (see "+
					"bank_redundancy_test and #82). %s",
					tc.letter, src, tc.wantSource, tc.note)
			}
			if got := phonemesForWord(t, p, text, tc.letter); got != tc.want {
				t.Errorf("%q = %q, want %q\n  %s", tc.letter, got, tc.want, tc.note)
			}
		})
	}
}

// The three spelled-out letter names the initialism splitter emits that gold does
// NOT hold. They are correct today only because LTS happens to render them right.
//
// ⚠️ #82 proposed banking these to "pin" them. That was wrong and the repo's own
// guard refused it: a banked entry the engine would produce anyway is invisible,
// so if LTS regresses on `ar` the bank silently covers it and nothing reports that
// the generator got worse. Asserting them here reports the regression instead.
// initialismProbe builds a sentence whose acronym expands to the spelled name under
// test, and expands to it EXACTLY ONCE.
//
// ⛔ One sentence carrying the whole alphabet does not work, for two independent
// reasons, and both are load-bearing:
//
//  1. acronymRe is \b[A-Z]{2,4}\b, so a longer run is not an acronym at all.
//     "ABCDEFGHIJKLMNOPQRSTUVWXYZ" never splits, falls to LTS whole, and comes back
//     as ˈæbkdəfʤˈɪʤklmnˈɑpkɹstˈʌvwksˈIz. A test written that way would assert nothing
//     about letter names while looking thorough.
//  2. U and W share the word "you" ("you" / "double you"). With both in one probe,
//     phonemesForWord sees two hits and fatals — correctly, since "the reading" is
//     ambiguous. Isolating each letter is what lets W be checked at all.
//
// Pairing every letter with Z (and Z with A) keeps each probe a legal 2-letter
// acronym and avoids expandAcronym's word-acronym lists (OK/IT/IS/…, HVAC/NOC),
// which return early and never spell out. No exception word ends in Z.
func initialismProbe(letter string) string {
	if letter == "Z" {
		return "The ZA Group met."
	}
	return "The " + letter + "Z Group met."
}

func TestSplitInitialismLetterNamesComeFromLTS(t *testing.T) {
	p := newTestPhonemizer(t)

	// These four are the whole LTS-sourced set, and they are why this test exists:
	// no dictionary layer holds them, so an LTS retune moves them silently and every
	// acronym containing A, E, G or R starts speaking differently with no other signal.
	//
	// ⚑ G was NOT in #82's original list. The ticket named three (ay/ar/ee); measuring
	// all 26 through the splitting path turned up `jee` as a fourth. The original list
	// was written from the letters that had come up in conversation, not from a sweep.
	cases := []struct{ letter, word, want string }{
		{"A", "ay", "ˈA"},
		{"E", "ee", "ˈi"},
		{"G", "jee", "ʤˈi"},
		{"R", "ar", "ˈɑɹ"},
	}
	if len(cases) != 4 {
		t.Fatalf("the LTS-sourced set is 4 letter names; this table has %d. If one moved "+
			"into gold or custom, DELETE its row rather than editing the expected source — "+
			"a row asserting \"lts\" for something now banked fails for the right reason "+
			"and invites the wrong fix", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.letter+"/"+tc.word, func(t *testing.T) {
			text := initialismProbe(tc.letter)
			if src := sourceForWord(t, p, text, tc.word); src != "lts" {
				t.Errorf("%q (letter %s) resolved from %q, want \"lts\" — if this says "+
					"`custom` or `gold`, someone banked a letter name and an LTS regression "+
					"can no longer be seen (#82)", tc.word, tc.letter, src)
			}
			if got := phonemesForWord(t, p, text, tc.word); got != tc.want {
				t.Errorf("%q (letter %s) = %q, want %q — LTS's reading of this letter name "+
					"has MOVED. Acronyms containing this letter now speak differently and "+
					"nothing else would report it", tc.word, tc.letter, got, tc.want)
			}
		})
	}
}

// The other 22 letters' spelled names come from gold. Pinned for the same reason as
// the LTS four — a gold regression on `aitch` garbles every acronym with an H in it —
// and additionally because the SOURCE assert is what stops someone "helpfully"
// banking them into custom_dict, which bank_redundancy_test refuses.
//
// ⚠ `why` is wˌI with SECONDARY stress while the single letter Y is wˈI with primary.
// Recorded as measured, not endorsed: both ship today and no ear has been asked. If
// that is wrong it is a gold-notation question (#63), not this test's to decide.
func TestSplitInitialismLetterNamesComeFromGold(t *testing.T) {
	p := newTestPhonemizer(t)

	// U and W both expand through "you" ("you" / "double you"), so W carries two rows.
	// 22 letters, 23 words.
	cases := []struct{ letter, word, want string }{
		{"B", "bee", "bˈi"},
		{"C", "see", "sˈi"},
		{"D", "dee", "dˈi"},
		{"F", "eff", "ˈɛf"},
		{"H", "aitch", "ˈAʧ"},
		{"I", "eye", "ˈI"},
		{"J", "jay", "ʤˈA"},
		{"K", "kay", "kˈA"},
		{"L", "ell", "ˈɛl"},
		{"M", "em", "ˈɛm"},
		{"N", "en", "ˈɛn"},
		{"O", "oh", "ˈO"},
		{"P", "pee", "pˈi"},
		{"Q", "cue", "kjˈu"},
		{"S", "ess", "ˈɛs"},
		{"T", "tee", "tˈi"},
		{"U", "you", "ju"},
		{"V", "vee", "vˈi"},
		{"W", "double", "dˈʌbᵊl"},
		{"W", "you", "ju"},
		{"X", "ex", "ˈɛks"},
		{"Y", "why", "wˌI"},
		{"Z", "zee", "zˈi"},
	}

	seen := map[string]bool{}
	for _, tc := range cases {
		seen[tc.letter] = true
	}
	if len(seen) != 22 {
		t.Fatalf("22 letters have gold-sourced spelled names (26 minus the 4 in "+
			"TestSplitInitialismLetterNamesComeFromLTS); this table covers %d. A missing "+
			"letter is one nobody is checking", len(seen))
	}

	for _, tc := range cases {
		t.Run(tc.letter+"/"+tc.word, func(t *testing.T) {
			text := initialismProbe(tc.letter)
			if src := sourceForWord(t, p, text, tc.word); src != "gold" {
				t.Errorf("%q (letter %s) resolved from %q, want \"gold\" — `lts` means gold "+
					"LOST this entry and the reading is now generated; `custom` means someone "+
					"banked it and a gold regression is now invisible (#82)",
					tc.word, tc.letter, src)
			}
			if got := phonemesForWord(t, p, text, tc.word); got != tc.want {
				t.Errorf("%q (letter %s) = %q, want %q — every acronym containing this "+
					"letter now speaks differently", tc.word, tc.letter, got, tc.want)
			}
		})
	}
}
