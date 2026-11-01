//go:build unit

package phonemizer

// Apple product names (#116). "iPhone" came out "ih-fone" (the LTS fallback guessed a short i), and
// iCloud, iTunes, macOS and MacBook were wrong the same way. Clayton's P0: Apple's is "eye-phone".
// The phonemes are pinned in Kokoro notation ("I" is the "eye" vowel); the ear check is Clayton's.

import (
	"strings"
	"testing"
)

// wordIPA returns the phonemes DiagnoseText chose for the first word equal to target (ignoring case
// and surrounding punctuation) in text.
func wordIPA(t *testing.T, p *Phonemizer, text, target string) string {
	t.Helper()
	for _, d := range p.DiagnoseText(text) {
		if strings.EqualFold(strings.Trim(d.Word, `.,;:!?"“”`), target) {
			return d.Phonemes
		}
	}
	t.Fatalf("no %q in %q", target, text)
	return ""
}

func TestAppleNames(t *testing.T) {
	p := newTestPhonemizer(t)
	cases := []struct {
		text, word, want string
	}{
		{"She bought an iPhone.", "iPhone", "ˈIfˌOn"},
		{"Two iPhones sat on the desk.", "iPhones", "ˈIfˌOnz"},
		{"The iPhone's screen cracked.", "iPhone's", "ˈIfˌOnz"},
		{"Back up to iCloud.", "iCloud", "ˈIklˌWd"},
		{"She opened iTunes.", "iTunes", "ˈItˌunz"},
		{"It runs on macOS.", "macOS", "mˈækˌOˈɛs"},
		{"He bought a MacBook.", "MacBook", "mˈækbˌʊk"},

		// Already right: must not move.
		{"He lost his iPad.", "iPad", "ˈIpəd"},
		{"The iPod was old.", "iPod", "ˈIpˌɑd"},
		{"It runs on iOS.", "iOS", "ˌIOˈɛs"},
		{"An iMac on the desk.", "iMac", "ˈImæk"},
		{"Send an iMessage.", "iMessage", "ˈImsəʤ"},
	}
	for _, c := range cases {
		if got := wordIPA(t, p, c.text, c.word); got != c.want {
			t.Errorf("%q in %q: got %s, want %s", c.word, c.text, got, c.want)
		}
	}
}
