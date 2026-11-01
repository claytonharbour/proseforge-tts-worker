//go:build unit

package phonemizer

import "testing"

// Names and words from the Past Imperfect bundle (forge/stories#90) that fell
// to LTS and came out wrong (#109). The three books are re-narrated on Kokoro,
// so each of these would be said wrong for hours of audio.
//
// Measured on The Time Traveling Tailor with `diagnose -text`; the values
// below follow gold's own neighbours (Birmingham, wyatt, ... noted per row).
//
// ⚠️ Haydn and Mozart were first listed on #109 as "acceptable by eye". They are
// not: LTS gave hˈAdn ("HAYdn") and mˈOzɑɹt ("MOZ-art"). Banked here.
func TestPastImperfectNamesAreBanked(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct{ word, want, was string }{
		{"Buckingham", "bˈʌkɪŋəm", "bˈʌkɪŋhˈæm"}, // BUCK-ing-um, was "-HAM"
		{"Argyll", "ɑɹɡˈIl", "ɑɹɡɪl"},            // ar-GYLE, was "AR-gill"
		{"Wickham", "wˈɪkəm", "wˈɪkhəm"},         // WICK-um, was "WICK-hum"
		{"canapé", "kˈænəpˌA", "kˈænˈAp"},        // was two primary stresses
		{"canapés", "kˈænəpˌAz", "kˈænəps"},      // was no -AY at all
		{"resewn", "ɹisˈOn", "ɹisˈun"},           // ree-SOHN, was "ree-SOON"
		{"Kew", "kjˈu", "kˈu"},                   // KYOO, was "KOO"
		{"Catherine", "kˈæθɹɪn", "kˈæðɜɹən"},     // same as the banked "katherine"
		{"Haydn", "hˈIdᵊn", "hˈAdn"},             // HIGH-dn, was "HAYdn"
		{"Mozart", "mˈOtsɑɹt", "mˈOzɑɹt"},        // MOHT-sart, was "MOZ-art"

		// The Tailor's "un-change" (chapter 5, Jonathan on undoing what they
		// changed). LTS read the prefix as "un-KAYNJ", stress on the wrong syllable.
		{"un-change", "ʌnʧˈAnʤ", "ənkˈAnʤ"},
		{"un-changing", "ʌnʧˈAnʤɪŋ", "ənkˈAnʤɪŋ"},

		// The other two books, scanned after their rewrites merged (#109).
		// Tortoise: the French slogans, a Roman, a pirate island, one contraction.
		{"Egalité", "ˌAɡɑlitˈA", "ˈɛɡˈælˈIt"},       // was "EG-AL-ITE"
		{"Égalité", "ˌAɡɑlitˈA", "ˈɛɡˈælˈIt"},       // accented É: miss the plain-E key, was guessed ("Ega light")
		{"Liberté", "lˌibɜɹtˈA", "lˈɪbɜɹt"},         // was "LIB-ert"
		{"Robespierre", "ɹˈObzpˌiɹ", "ɹˈObspˈɪˈɛɹ"}, // ROHBZ-peer
		{"Tortuga", "tɔɹtˈuɡə", "tˈɔɹʧˈuɡə"},        // tor-TOO-ga, was "tor-CHOO-ga"
		{"ochre", "ˈOkɜɹ", "ˈɑkɜɹ"},                 // OH-ker, was "AH-ker"
		{"Lucius", "lˈuʃəs", "lˈusəs"},              // LOO-shus
		{"Where'd", "wɛɹd", "wɛɹɪd"},                // was "WAIR-id"
		// Comedian: the heroine, said hundreds of times, and Simon.
		{"Eliza", "ɪlˈIzə", "ˈɛlˈIzə"}, // ih-LYE-za, was two primary stresses
		{"Simon", "sˈImən", "sˈɪmən"},  // SIGH-mon, was "SIM-on"
	}

	for _, tc := range cases {
		text := "And " + tc.word + " was there."
		if got := phonemesForWord(t, p, text, tc.word); got != tc.want {
			t.Errorf("%q = %q, want %q (LTS gave %q)", tc.word, got, tc.want, tc.was)
		}
		if src := sourceForWord(t, p, text, tc.word); src != "custom" {
			t.Errorf("%q resolved from %q, not custom", tc.word, src)
		}
	}
}
