//go:build unit

package phonemizer

import "testing"

// "Source: gold" and "listed in gold" are different claims, and the diagnose
// output only ever supported the first. @wayland read two `gold` lines as two
// gold entries and came one post from refuting #56 — a real defect nearly
// dismissed because the instrument answered a question adjacent to the one
// being asked.
//
// Rule == "" is the membership claim. Anything else means this code BUILT the
// form, and a built form can disagree with every listed one — which is #56.
func TestLookupWithSuffixRuleDistinguishesListedFromBuilt(t *testing.T) {
	d := &MisakiDict{entries: map[string]string{
		"glass":   "ɡlˈæs",
		"glasses": "ɡlˈæsᵻz", // listed, and it disagrees with what we'd build
		"dress":   "dɹˈɛs",   // dresses is NOT listed — it gets built
		"story":   "stˈɔɹi",
		"stop":    "stˈɑp",
		"carry":   "kˈæɹi",
		"vasquez": "ˈvæskɛz",
	}}

	cases := []struct {
		word     string
		wantIPA  string
		wantRule string
	}{
		{"glasses", "ɡlˈæsᵻz", ""},    // listed — the dictionary's own answer
		{"dresses", "dɹˈɛsɪz", "-es"}, // built — and note ɪz vs the listed ᵻz above
		{"stories", "stˈɔɹiz", "-ies"},
		{"stops", "stˈɑps", "-s"},
		{"carrying", "kˈæɹiɪŋ", "-ing"},
		{"stopped", "stˈɑpt", "-ed"},
		{"vasquez's", "ˈvæskɛzɪz", "'s"},
	}

	for _, c := range cases {
		ipa, rule, ok := d.LookupWithSuffixRule(c.word)
		if !ok {
			t.Errorf("%s: not found", c.word)
			continue
		}
		if ipa != c.wantIPA {
			t.Errorf("%s: ipa = %q, want %q", c.word, ipa, c.wantIPA)
		}
		if rule != c.wantRule {
			t.Errorf("%s: rule = %q, want %q — the rule is what separates "+
				"'the dictionary says this' from 'we assembled this'",
				c.word, rule, c.wantRule)
		}
	}

	// The pair that matters, stated as one assertion so it cannot half-pass:
	// same morphology, one listed and one built, and they disagree.
	_, listedRule, _ := d.LookupWithSuffixRule("glasses")
	builtIPA, builtRule, _ := d.LookupWithSuffixRule("dresses")
	if listedRule != "" || builtRule == "" {
		t.Fatalf("the #56 pair stopped being a listed/built pair: glasses rule=%q dresses rule=%q",
			listedRule, builtRule)
	}
	if builtIPA != "dɹˈɛsɪz" {
		t.Errorf("dresses = %q — if this changed to ᵻz then #56 was implemented "+
			"and this test should be re-stated rather than deleted", builtIPA)
	}
}

// A miss must report no rule, or a caller checking `rule == ""` reads a failed
// lookup as a direct hit.
func TestLookupWithSuffixRuleReportsNothingOnAMiss(t *testing.T) {
	d := &MisakiDict{entries: map[string]string{"glass": "ɡlˈæs"}}
	ipa, rule, ok := d.LookupWithSuffixRule("umbrella")
	if ok {
		t.Fatalf("found %q for a word that is not there", ipa)
	}
	if rule != "" {
		t.Errorf("rule = %q on a miss, want %q", rule, "")
	}
}

// LookupWithSuffix is now a wrapper. If it ever stops agreeing with the
// function it delegates to, every existing caller silently changes behaviour.
func TestLookupWithSuffixAgreesWithTheRuleVariant(t *testing.T) {
	d := &MisakiDict{entries: map[string]string{
		"glass": "ɡlˈæs", "glasses": "ɡlˈæsᵻz", "dress": "dɹˈɛs",
		"story": "stˈɔɹi", "stop": "stˈɑp", "carry": "kˈæɹi",
	}}
	for _, w := range []string{
		"glasses", "dresses", "stories", "stops", "carrying", "stopped",
		"glass's", "umbrella", "",
	} {
		wantIPA, wantOK := d.LookupWithSuffix(w)
		gotIPA, _, gotOK := d.LookupWithSuffixRule(w)
		if wantIPA != gotIPA || wantOK != gotOK {
			t.Errorf("%q: wrapper gave (%q,%v), rule variant gave (%q,%v)",
				w, wantIPA, wantOK, gotIPA, gotOK)
		}
	}
}
