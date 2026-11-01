//go:build unit

package phonemizer

import (
	"testing"
)

// Names from the Gutenberg corpus (milestone #26) that fell to LTS and came out
// wrong. Found by @sten's STT round trip on Alice (#98): whisper heard "Dinah"
// as "dinna" 7 times. LTS gave dɪnə, DIN-uh; the name is DIE-nuh, the same
// vowel gold uses for china (ʧˈInə) and dine (dˈIn).
func TestGutenbergNamesAreBanked(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct{ word, want, was string }{
		{"Dinah", "dˈInə", "dɪnə"}, // #102

		// #105, found by @sten auditing narrations. Values follow gold's own
		// neighbours: withdraw wɪðdɹˈɔ, lighter lˈIɾəɹ, seaside sˈisˌId, café kæfˈA.
		{"fiancé", "fiˌɑnsˈA", "fiˈæns"},
		{"fiance", "fiˌɑnsˈA", "fiˈæns"},
		{"fiancée", "fiˌɑnsˈA", "fiˈænsi"},
		{"withdrew", "wɪðdɹˈu", "wɪθdɹu"},
		{"tighter", "tˈIɾəɹ", "tˈItɜɹ"},
		{"Douglas", "dˈʌɡləs", "dˈWˈʌɡlæs"},
		{"seawall", "sˈiwˌɔl", "sˈiˈɔl"},
	}

	for _, tc := range cases {
		text := "And " + tc.word + " was there."
		if got := phonemesForWord(t, p, text, tc.word); got != tc.want {
			t.Errorf("%q = %q, want %q (LTS gave %q)", tc.word, got, tc.want, tc.was)
		}
		// Assert the layer as well as the value, so a future LTS change that
		// happens to agree cannot hide an unreachable bank entry.
		if src := sourceForWord(t, p, text, tc.word); src != "custom" {
			t.Errorf("%q resolved from %q, not custom", tc.word, src)
		}
	}
}
