//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// The LTS tree speaks Flite's phone set; `arpabet_to_ipa.json` was written for CMUdict's.
// The two disagree on exactly one symbol that matters: CMUdict spells schwa `AH0`, Flite
// spells it `AX`. The table carried only `AH0`, `arpabetToIPA` skips phones it cannot map,
// and so every schwa the tree produced was dropped without a word of complaint —
// `unjammable` → `nʤˈæmbl`, `Þórsson` → `θˈɔɹsn`. That is #53's root cause and the reason
// #50 and #51 exist.
//
// This is the test that would have caught it: a phone the tree can emit and the table
// cannot translate is a silent deletion, whatever the phone happens to be.
func TestEveryLTSPhoneHasAnIPAMapping(t *testing.T) {
	lts, err := NewLTS("../../data/arpabet_to_ipa.json")
	if err != nil {
		t.Fatalf("NewLTS: %v", err)
	}

	checked := 0
	for i, phone := range ltsPhoneTable {
		if phone == "epsilon" {
			continue
		}
		// Phonemize splits compounds ("ax0-l" → "ax0", "l") before conversion, so the
		// parts are what must be mappable, not the compound string.
		for _, part := range strings.Split(phone, "-") {
			base, _ := splitARPABETStress(part)
			// AH0 is a distinct table key, not AH with a stress digit — mirror the
			// special case in arpabetToIPA rather than restating its logic.
			if base == "AH" && strings.HasSuffix(part, "0") {
				base = "AH0"
			}
			if _, ok := lts.arpToIPA[base]; !ok {
				t.Errorf("ltsPhoneTable[%d] = %q: no IPA mapping for %q — "+
					"arpabetToIPA will drop it silently", i, phone, base)
			}
			checked++
		}
	}

	if checked < len(ltsPhoneTable) {
		t.Fatalf("only checked %d phone parts across %d table entries — "+
			"the loop is not covering the table", checked, len(ltsPhoneTable))
	}
}

// The observable consequence, through the pipeline rather than through LTS directly.
//
// These are the four words @clayton's ear-check and the corpus sweep flagged, plus the
// two #25 specimens whose tails were being eaten by the same missing key. All six must
// still reach LTS — if one gets banked in a dictionary this stops testing the generator,
// hence the source assertion.
func TestLTSProducesSchwaInUnstressedSyllables(t *testing.T) {
	p := loadTestPhonemizer(t)

	want := map[string]string{
		"unjammable": "ənʤˈæməbəl", // #51's un- and #50's -able, both restored
		"hearable":   "hˈɪɹəbəl",
		"purest":     "pjˈʊɹəst",
		"realest":    "ɹˈiləst",
		"Schumann":   "ʃˈumən",
		"Þórsson":    "θˈɔɹsən",
	}

	text := "unjammable hearable purest realest Schumann Þórsson."
	for _, d := range p.DiagnoseText(text) {
		expect, ok := want[d.Word]
		if !ok {
			t.Fatalf("unexpected word %q in diagnosis — text and table disagree", d.Word)
		}
		if d.Source != "lts" {
			t.Fatalf("%s resolved from %q, not LTS — this no longer tests the "+
				"letter-to-sound path", d.Word, d.Source)
		}
		if d.Phonemes != expect {
			t.Errorf("%s = %q, want %q", d.Word, d.Phonemes, expect)
		}
		delete(want, d.Word)
	}
	for w := range want {
		t.Errorf("%s never appeared in the diagnosis", w)
	}
}
