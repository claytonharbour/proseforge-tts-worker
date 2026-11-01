//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// #46 proposed fixing compound stress "in the LTS stress assignment" — turning the
// second element's `ˈ` into `ˌ`. That fix cannot live there: LTS is structurally
// incapable of emitting a secondary stress mark at all.
//
// `arpabetToIPA` writes `ˌ` only when a phone carries stress digit 2, and the phone
// table generated from Flite's cmu_lts_rules.c contains no such phone. The branch is
// dead code on the LTS path, so any compound-stress fix has to be a post-LTS rewrite
// over the finished IPA string.
//
// Pinned as a test rather than left in a ticket comment because it is the load-bearing
// fact under #46's scoping: if a future regeneration of lts_data.go ever introduces a
// stress-2 phone, the tree gains a capability the ticket assumed it already had, and
// whoever adds it should be told rather than left to rediscover it.
func TestLTSPhoneTableHasNoSecondaryStress(t *testing.T) {
	var stress1, stress0, unstressed int
	for i, phone := range ltsPhoneTable {
		for _, part := range strings.Split(phone, "-") {
			if part == "" {
				continue
			}
			switch part[len(part)-1] {
			case '2':
				t.Errorf("ltsPhoneTable[%d] = %q carries stress digit 2; "+
					"LTS can now emit ˌ and #46's scoping needs revisiting", i, phone)
			case '1':
				stress1++
			case '0':
				stress0++
			default:
				unstressed++
			}
		}
	}

	// Guard the guard: if the table were empty or the stress digits vanished, the
	// loop above would pass vacuously and stop meaning anything.
	if stress1 == 0 || stress0 == 0 {
		t.Fatalf("phone table looks wrong: %d primary, %d unstressed-vowel, %d consonant "+
			"entries over %d phones — expected both stressed and unstressed vowels",
			stress1, stress0, unstressed, len(ltsPhoneTable))
	}
}

// The engine must not emit a secondary mark for an LTS word, which is the observable
// consequence of the table fact above. Compound names are the case #46 cares about.
//
// These are invented names, deliberately: they must miss every bundled dictionary so
// the assertion is about LTS and not about whatever gold happens to hold. If one of
// them is ever banked, this test starts measuring the wrong layer — hence the
// source check rather than a bare string comparison.
func TestLTSNeverEmitsSecondaryStress(t *testing.T) {
	p := loadTestPhonemizer(t)

	// Tails that gold always reduces (-ford → fəɹd, -land → lᵊnd) alongside tails it
	// keeps full, so the sample spans both sides of the gold rule.
	names := "Marchford Kettleford Thrushland Alderland Marshburn Brackenbury " +
		"Hazelmouth Larkhill Bramblewood Willowbank Ashcombe Ravensbury."

	// DiagnoseText, not lts.Phonemize — the component answer is always cleaner and
	// always slightly false. This has to be what a user's text actually flows through.
	diag := p.DiagnoseText(names)
	if len(diag) != 12 {
		t.Fatalf("diagnosed %d words, want 12", len(diag))
	}

	for _, d := range diag {
		if d.Source != "lts" {
			t.Fatalf("%s resolved from %q, not LTS — pick a name that is still unknown, "+
				"or this asserts nothing about the letter-to-sound path", d.Word, d.Source)
		}
		if strings.Contains(d.Phonemes, "ˌ") {
			t.Errorf("%s → %s contains a secondary mark; LTS was believed unable to "+
				"produce one (see TestLTSPhoneTableHasNoSecondaryStress)", d.Word, d.Phonemes)
		}
	}
}
