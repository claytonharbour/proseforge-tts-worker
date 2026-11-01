//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// #124: sentences from the dev corpus (392 sections, 2026-10-26) where "read" was said wrongly,
// plus the ones that were right and must stay right. "reed" is the base form (active present,
// infinitive); "red" is any past participle: past tense, perfect, passive, "a sign that read".
func TestHomographReadTense(t *testing.T) {
	h := loadTestHomographs(t)
	const reed, red = "ɹˈid", "ɹˈɛd"

	tests := []struct {
		sentence string
		want     string
	}{
		// A name or title right before it is the subject: narration, "red", whatever follows.
		{"Lily read the message aloud, her voice trembling.", red},
		{"Zorp read the message aloud, his voice flat.", red},
		{"Barty read aloud, his voice heavy with relief.", red},
		{"Zephyr read aloud from ancient texts.", red},
		{"The Abbot read in silence.", red},
		{"Mara read it twice before putting her phone down.", red},
		// ...but an auxiliary or modal before the name keeps the base form (Rambald's review of #125).
		{"Did Lily read it?", reed},
		{"Can Mara read this?", reed},
		{"Will Tom read the letter?", reed},
		{"Let Tom read it first.", reed},
		{"Does Ada read Latin?", reed},
		{"She wanted to see Lily read the message.", reed},
		// A capitalised PLURAL after a determiner is a noun, not a name: "the Troubadours read your
		// story aloud" is present tense (Sten's catch in the Guilds tutorial script).
		{"Everyone knows the Troubadours read your story aloud.", reed},
		{"The Troubadours read your story aloud.", reed},
		// A bare name that happens to end in s is still a name, so still narration (Rambald's review
		// of #126: these went to "reed" under the first version of the plural rule).
		{"Thomas read the message aloud, his voice trembling.", red},
		{"Silas read the letter aloud.", red},
		{"Marcus read aloud from the scroll.", red},
		{"Elias read the message aloud, her voice trembling.", red},
		{"Agnes read aloud, her voice heavy with relief.", red},
		{"Ross read the message aloud.", red},
		// A determiner in the PREVIOUS sentence or clause is not the name's determiner (Rambald's second
		// review of #126): the full stop or comma ends it.
		{"He looked at her. Thomas read the letter aloud.", red},
		{"She gave them all. Marcus read the letter aloud.", red},
		{"It was his, Silas read the letter aloud.", red},
		// Perfect contractions are participles, even when a later word ("about") pulls the other way.
		{"I've read about it my whole career.", red},
		{"I’ve read about it my whole career.", red},
		{"We've read them all.", red},
		// Already right, must stay right.
		{"A short line of text for the monk to read aloud.", reed},
		{"She read aloud, her voice trembling.", red},
		{"The message was about to be read.", red},
		{"I wouldn't read too much into that, sir.", reed},
		{"She had read it before.", red},
		{"Please read aloud.", reed},
		{"Do read this.", reed},
		{"I'll read it tomorrow.", reed},
		{"Whoever read it first will tell you.", red},
	}

	for _, tt := range tests {
		t.Run(tt.sentence, func(t *testing.T) {
			words := strings.Fields(tt.sentence)
			idx := -1
			for i, w := range words {
				if strings.EqualFold(stripWordPunctuation(w), "read") {
					idx = i
					break
				}
			}
			if idx < 0 {
				t.Fatalf("no \"read\" in %q", tt.sentence)
			}
			got, ok := h.Resolve(words, idx)
			if !ok || got != tt.want {
				t.Errorf("%q: got %q (ok=%v), want %q", tt.sentence, got, ok, tt.want)
			}
		})
	}
}
