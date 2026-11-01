//go:build unit

package phonemizer

import "testing"

// "St." is Saint and Street in the same corpus, and the map said Saint always.
//
// @wayland: 27 occurrences over 10 books, both senses live, "no custom_dict
// entry can fix this — the token is identical and only position disambiguates".
//
// ⚠️ The two ambiguous arms below are the point of the design. They go OPPOSITE
// ways — one is a street, one is a saint — so the bucket cannot be defaulted
// correctly, and both are left on the pre-existing "saint" rather than guessed.
func TestStreetSaintDisambiguation(t *testing.T) {
	p := newTestPhonemizer(t)
	isWord := func(w string) bool { return p.goldOwnsWord(w) }

	cases := []struct {
		in, want, why string
	}{
		// --- confident STREET: prev capitalised, next lowercase or absent ---
		{"He walked down Delaney St. yesterday.", "He walked down Delaney street yesterday.",
			"@wayland's specimen — was \"Delaney saint yesterday\""},
		{"Lakeshore purchases lot on Delaney St. adjacent to Phase three.",
			"Lakeshore purchases lot on Delaney street adjacent to Phase three.",
			"his second specimen, mid-sentence"},
		{"He lives on Baker St.", "He lives on Baker street",
			"END OF TEXT — no following word at all, which the next-word rule must not crash on"},
		{"They met on Delaney St., then walked north.", "They met on Delaney street, then walked north.",
			"punctuation immediately after the dot"},

		// --- confident SAINT: prev lowercase, next capitalised ---
		{"He prayed at St. Bernadette's.", "He prayed at saint Bernadette's.",
			"CONTROL: unchanged, and it must stay that way"},
		{"The second was St. Bernadette's, nine months ago.",
			"The second was saint Bernadette's, nine months ago.",
			"CONTROL: his saint specimen"},
		{"She met him at St. Petersburg.", "She met him at saint Petersburg.",
			"CONTROL: place name built on Saint"},
		// ⚠️ The arm that makes the prev-capitalised guard discriminate. Every
		// other lowercase-prev case here also has a CAPITALISED next word, so the
		// next-word guard was subsuming it and dropping prev-capitalised failed
		// NOTHING. A lowercase saint name — informal writing, OCR — is the gap:
		// without the guard this converts to "at street anne's".
		{"They were married at St. anne's.", "They were married at saint anne's.",
			"CONTROL: prev lowercase AND next lowercase — only the prev-capitalised " +
				"guard stands between this and \"street anne's\""},

		// --- AMBIGUOUS: both capitalised. Left on the default, and they DISAGREE ---
		{"Baker St. The door was red.", "Baker saint The door was red.",
			"AMBIGUOUS and actually a STREET — `The` is capitalised only because a " +
				"sentence starts. Left WRONG on purpose; a next-word rule alone would " +
				"get this one right and the following one wrong"},
		{"On St. James Street the lamps were lit.", "On saint James Street the lamps were lit.",
			"AMBIGUOUS and actually a SAINT — `On` is capitalised only because a " +
				"sentence starts. Left RIGHT by the default. THIS PAIR IS THE PROOF " +
				"the bucket cannot be guessed: same shape, opposite senses"},
		{"Mary St. Clair arrived.", "Mary saint Clair arrived.",
			"AMBIGUOUS — a surname. Correctly left alone rather than made a street"},
	}

	for _, tc := range cases {
		if got := NormalizeTextWithWords(tc.in, isWord); got != tc.want {
			t.Errorf("normalize(%q)\n  got  %q\n  want %q\n  %s", tc.in, got, tc.want, tc.why)
		}
	}
}
