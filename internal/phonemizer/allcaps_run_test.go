//go:build unit

package phonemizer

import "testing"

// isWordStub stands in for a gold/silver lookup.
func isWordStub(w string) bool {
	words := map[string]bool{
		"stop": true, "wait": true, "help": true, "fire": true, "area": true,
		"site": true, "not": true, "the": true, "and": true, "they": true,
		"knew": true, "am": true, "pm": true, "da": true, "er": true,
		"watching": true, "ops": true, "sync": true, "restricted": true,
	}
	return words[w]
}

func TestAllCapsRunRule(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		// --- emphasis: inside a run, dictionary words read as words ---
		{"shouted prose", "STOP WAIT HELP", "stop wait help"},
		{"two-token run", "THEY KNEW", "they knew"},
		{"acronym inside a run still expands",
			"RESTRICTED AREA EPA SITE", "RESTRICTED area ee pee ay site"},

		// --- @Wayland's decisive specimen: . breaks a run ---
		{"abbreviations in prose are NOT a run",
			"outside the department. DA. FBI. Someone",
			"outside the department. dee ay. eff bee eye. Someone"},

		// --- AM in both directions: the case the deny-list could not do ---
		{"AM with a digit neighbour is the meridiem",
			"He arrived at 7 AM", "He arrived at seven ay em"},
		{"AM with a caps neighbour is the verb",
			"I AM WATCHING", "I am WATCHING"},

		// --- @Wayland flagged "FBI CIA" unpunctuated as a possible false run.
		// It is a run, and the rule still handles it: the SECOND factor decides,
		// and neither is an English word. No corpus evidence required.
		{"all-acronym run still expands", "FBI CIA", "eff bee eye see eye ay"},

		// --- isolated acronyms unchanged ---
		{"isolated DA expands", "The DA said", "The dee ay said"},
		{"isolated ER expands", "the ER was full", "the ee ar was full"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeTextWithWords(tt.input, isWordStub)
			if got != tt.want {
				t.Errorf("NormalizeTextWithWords(%q)\n  got  %q\n  want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestAllCapsRuleIsOptIn pins that the no-dictionary path is byte-identical to
// the old behaviour, so existing callers cannot be changed by this feature.
func TestAllCapsRuleIsOptIn(t *testing.T) {
	for _, in := range []string{"STOP WAIT HELP", "I AM WATCHING", "The DA said"} {
		if NormalizeText(in) != NormalizeTextWithWords(in, nil) {
			t.Errorf("nil oracle diverges from NormalizeText for %q", in)
		}
	}
}
