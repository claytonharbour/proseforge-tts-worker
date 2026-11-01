//go:build unit

package phonemizer

// "live" is the adjective (rhymes with "five") right after a determiner: "the live thing", "a live
// backup", "this live setup". A determiner cannot be followed directly by the verb, so the
// adjective is the only grammatical reading there. It used to be said as the verb ("liv") unless a
// listed noun followed ("concert", "wire"...), which missed "the live thing" (Clayton's ear on the
// #112 clips, 2026-10-13). The sentences are synthetic; this repo must not hold other people's text.

import (
	"strings"
	"testing"
)

const (
	liveAdjIPA  = "lˈIv"
	liveVerbIPA = "lˈɪv"
)

// liveIPA returns the phonemes DiagnoseText chose for the k-th "live" word (0-based) in text.
func liveIPA(t *testing.T, p *Phonemizer, text string, k int) string {
	t.Helper()
	n := 0
	for _, d := range p.DiagnoseText(text) {
		if strings.EqualFold(strings.Trim(d.Word, `.,;:!?"'“”’‘ `), "live") {
			if n == k {
				return d.Phonemes
			}
			n++
		}
	}
	t.Fatalf("no %dth \"live\" in %q", k, text)
	return ""
}

func TestLiveAfterADeterminerIsTheAdjective(t *testing.T) {
	p := newTestPhonemizer(t)
	cases := []struct {
		text string
		want string
	}{
		// The adjective after a determiner, with no listed noun after it.
		{"Read it back from the live thing, not from the system that told you it worked.", liveAdjIPA},
		{"They kept a live backup of the files.", liveAdjIPA},
		{"This live setup is fragile.", liveAdjIPA},
		{"Check his live numbers before you decide.", liveAdjIPA},
		{"We tested every live path.", liveAdjIPA},
		{"Where the live data goes is a separate question.", liveAdjIPA}, // a verb trigger ("where") sits further away

		// Predicate adjective: after a form of "be" ("the game is live", "check what's live") or
		// "for" ("for live collaboration"), found by the #112 sampling audit (3 of 20 "live" wrong).
		{"The game is live now.", liveAdjIPA},
		{"Check what's live before you say what isn't.", liveAdjIPA},
		{"Check what’s live before you say what isn’t.", liveAdjIPA},
		{"It's live from the stage tonight.", liveAdjIPA},
		{"They're live on the air.", liveAdjIPA},
		{"There's live music every night.", liveAdjIPA},
		{"For live collaboration, use the room.", liveAdjIPA},
		{"The release has not been live for long.", liveAdjIPA},
		// ...and where the verb must stay: "let's live" is let us live; a subject pronoun is the verb.
		{"Let's live in the moment.", liveVerbIPA},
		{"People who live there know it.", liveVerbIPA},
		{"I hope they live to see it.", liveVerbIPA},

		// The determiner must be DIRECTLY before the word; one further back (a noun in between, or after
		// the word) is not evidence. Found by the existing guards when the first attempt used a window.
		{"The database your stories live in is copied once a day.", liveVerbIPA},
		{"We'll live without the fancy plaster in the bedrooms.", liveVerbIPA},
		{"Let her live in peace.", liveVerbIPA},

		// The verb is unaffected.
		{"They live in a small house by the river.", liveVerbIPA},
		{"I want to live without the noise.", liveVerbIPA},
		{"Where you'd live is up to you.", liveVerbIPA},
		{"Those who live near the water know the tides.", liveVerbIPA},
		{"We live here now.", liveVerbIPA},
	}
	for _, c := range cases {
		if got := liveIPA(t, p, c.text, 0); got != c.want {
			t.Errorf("%q: got %s, want %s", c.text, got, c.want)
		}
	}
}
