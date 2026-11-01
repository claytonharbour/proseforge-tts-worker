//go:build unit

package phonemizer

import "testing"

// #54. A surname absent from gold gets its final syllable replaced by a plural /z/ or
// /s/: LookupWithSuffix strips -s/-es, finds a real English base, and appends the
// possessive phoneme. Mercedes → məɹsˈɛdz, "mur-SEDZ".
//
// Pinned rather than filtered because the obvious guard is measurably wrong — see the
// second test. These resolve from GOLD, so --sweep cannot see them; this file is
// currently the only instrument that can.
func TestSurnameSuffixStripping_KnownBad(t *testing.T) {
	p := loadTestPhonemizer(t)

	// Deliberately not the banked ones (Torres, Reyes) — those are shadowed by
	// custom_dict and would pass while the defect is untouched.
	for _, c := range []struct{ word, got, why string }{
		{"Morales", "məɹˈælz", "-s → morale"},
		{"Chaves", "ʧˈævz", "-es → chav"},
		{"Mercedes", "məɹsˈɛdz", "-es → merced"},
		{"Nunes", "nˈʌnz", "-es → nun"},
		{"Lopes", "lˈOps", "-s → lope"},
	} {
		for _, d := range p.DiagnoseText(c.word + ".") {
			if d.Source != "gold" {
				t.Errorf("%s now resolves from %q, not gold — if it was banked, this test "+
					"stops measuring the suffix path and needs a different specimen",
					c.word, d.Source)
			}
			if d.Phonemes != c.got {
				t.Errorf("%s = %q, pinned as %q (%s) — if this is a fix for #54, update the "+
					"pin and say so on the ticket", c.word, d.Phonemes, c.got, c.why)
			}
		}
	}
}

// The fix that suggests itself — skip suffix-stripping on capitalised words — breaks
// ordinary plurals, because capitalisation in prose is positional. `Practices` at the
// start of a sentence has no exact gold key and needs the rule that mangles `Mercedes`.
//
// This is the constraint any #54 fix has to satisfy, pinned so it is checked rather
// than remembered.
func TestSuffixStrippingIsRequiredForOrdinaryPlurals(t *testing.T) {
	p := loadTestPhonemizer(t)

	for _, c := range []struct{ word, want string }{
		{"Practices", "pɹˈæktəsɪz"},
		{"Processes", "pɹˈɑsˌɛsɪz"},
		{"Watches", "wˈɑʧɪz"},
		{"Boxes", "bˈɑksɪz"},
	} {
		for _, d := range p.DiagnoseText(c.word + ".") {
			if d.Phonemes != c.want {
				t.Errorf("%s = %q, want %q — a #54 guard keyed on capitalisation would "+
					"break this, which is why it must not be", c.word, d.Phonemes, c.want)
			}
		}
	}
}
