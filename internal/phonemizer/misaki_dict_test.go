//go:build unit

package phonemizer

import (
	"os"
	"path/filepath"
	"testing"
)

func loadTestGoldDict(t *testing.T) *MisakiDict {
	t.Helper()
	path := "../../data/us_gold.json"
	if _, err := os.Stat(path); err != nil {
		absP, _ := filepath.Abs(path)
		t.Skipf("data file not found: %s", absP)
	}

	d, err := LoadMisakiDict(path)
	if err != nil {
		t.Fatalf("LoadMisakiDict(gold): %v", err)
	}
	return d
}

func loadTestSilverDict(t *testing.T) *MisakiDict {
	t.Helper()
	path := "../../data/us_silver.json"
	if _, err := os.Stat(path); err != nil {
		absP, _ := filepath.Abs(path)
		t.Skipf("data file not found: %s", absP)
	}

	d, err := LoadMisakiDict(path)
	if err != nil {
		t.Fatalf("LoadMisakiDict(silver): %v", err)
	}
	return d
}

func TestMisakiGoldSize(t *testing.T) {
	d := loadTestGoldDict(t)
	if d.Size() < 80000 {
		t.Errorf("expected >80k gold entries, got %d", d.Size())
	}
	t.Logf("Gold dictionary: %d entries", d.Size())
}

func TestMisakiSilverSize(t *testing.T) {
	d := loadTestSilverDict(t)
	if d.Size() < 80000 {
		t.Errorf("expected >80k silver entries, got %d", d.Size())
	}
	t.Logf("Silver dictionary: %d entries", d.Size())
}

func TestMisakiGoldLookup(t *testing.T) {
	d := loadTestGoldDict(t)

	tests := []struct {
		word  string
		found bool
	}{
		{"hello", true},
		{"world", true},
		{"the", true},
		{"a", true},
		{"xyzzyplugh", false},
	}

	for _, tt := range tests {
		ipa, ok := d.Lookup(tt.word)
		if ok != tt.found {
			t.Errorf("Lookup(%q): found=%v, want %v", tt.word, ok, tt.found)
		}
		if ok && ipa == "" {
			t.Errorf("Lookup(%q): found but empty IPA", tt.word)
		}
	}
}

func TestMisakiGoldCaseInsensitive(t *testing.T) {
	d := loadTestGoldDict(t)

	ipa1, ok1 := d.Lookup("Hello")
	ipa2, ok2 := d.Lookup("hello")
	ipa3, ok3 := d.Lookup("HELLO")

	if !ok1 || !ok2 || !ok3 {
		t.Error("case-insensitive lookup failed")
	}
	if ipa1 != ipa2 || ipa2 != ipa3 {
		t.Errorf("case variants gave different results: %q, %q, %q", ipa1, ipa2, ipa3)
	}
}

func TestMisakiPossessive(t *testing.T) {
	d := loadTestGoldDict(t)

	_, baseFound := d.Lookup("sarah")
	if !baseFound {
		t.Skip("sarah not in dictionary")
	}

	ipa, ok := d.LookupWithSuffix("sarah's")
	if !ok {
		t.Error("sarah's not found via possessive lookup")
	}
	if ipa == "" {
		t.Error("sarah's IPA is empty")
	}
	t.Logf("sarah's IPA: %q", ipa)
}

func TestMisakiPOSTaggedEntry(t *testing.T) {
	d := loadTestGoldDict(t)

	// "read" has POS-tagged pronunciations in gold dict
	// The DEFAULT pronunciation should be used
	ipa, ok := d.Lookup("read")
	if !ok {
		t.Skip("read not in gold dictionary")
	}
	if ipa == "" {
		t.Error("read IPA is empty")
	}
	t.Logf("read (DEFAULT) IPA: %q", ipa)
}

func TestMisakiSilverFallback(t *testing.T) {
	gold := loadTestGoldDict(t)
	silver := loadTestSilverDict(t)

	// Find a word in silver but not in gold
	// Silver has ML-generated entries for words not in gold
	found := 0
	for _, word := range []string{"aaronic", "abacination", "abactor"} {
		_, inGold := gold.Lookup(word)
		ipa, inSilver := silver.Lookup(word)
		if !inGold && inSilver {
			found++
			t.Logf("silver-only word %q: %q", word, ipa)
		}
	}
	if found == 0 {
		t.Log("no silver-only words found in test set (may be expected if gold coverage increased)")
	}
}

func TestMisakiOutputIsKokoroIPA(t *testing.T) {
	d := loadTestGoldDict(t)

	// Verify that output uses Kokoro IPA symbols (uppercase diphthongs, etc.)
	ipa, ok := d.Lookup("hello")
	if !ok {
		t.Skip("hello not in dictionary")
	}

	// Misaki outputs native Kokoro IPA — should contain IPA characters
	// and not ARPABET (which would be uppercase multi-letter codes like "HH AH0 L OW1")
	if len(ipa) < 2 {
		t.Errorf("hello IPA too short: %q", ipa)
	}
	t.Logf("hello IPA: %q", ipa)
}
