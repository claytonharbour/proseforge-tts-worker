//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// A name suffix expands whatever punctuation follows its period (#100).
//
// Jr. sits at the end of a name, so it is usually followed by a comma, a closing
// quote or 's rather than a space, and every one of those forms fell through to
// LTS as a vowel-less ʤɹ. Esq. was not in the list at all.
func TestNameSuffixAbbreviationsExpandBeforePunctuation(t *testing.T) {
	tests := []struct{ input, want string }{
		{"Jester Jr. left.", "junior"},
		{"Jester Jr., left.", "junior"},
		{"Jester Jr.'s hat.", "junior's"},
		{"Jester Jr.’s hat.", "junior"},
		{`They called "Jester Jr."`, "junior"},
		{"(Jester Jr.)", "junior"},
		{"Smith Sr., the elder.", "senior"},
		{"Foot, Esq. came.", "esquire"},
		{"Foot, Esq., came.", "esquire"},
	}
	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if !strings.Contains(got, tt.want) {
			t.Errorf("NormalizeText(%q) = %q, want it to contain %q", tt.input, got, tt.want)
		}
		for _, f := range strings.Fields(strings.ToLower(got)) {
			if w := strings.Trim(f, `.,'"()`); w == "jr" || w == "sr" || w == "esq" || strings.HasPrefix(f, "jr.") {
				t.Errorf("NormalizeText(%q) = %q still carries the abbreviation", tt.input, got)
			}
		}
	}

	// CONTROL: the tolerance is for name suffixes only. "no." also expands (to
	// "number"), and a quoted "No." in dialogue must never read as a number.
	if got := NormalizeText(`"No." she said.`); strings.Contains(got, "number") {
		t.Errorf(`NormalizeText("No." she said.) = %q, dialogue "No." became a number`, got)
	}
}
