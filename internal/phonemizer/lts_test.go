//go:build unit

package phonemizer

import (
	"os"
	"path/filepath"
	"testing"
)

func loadTestLTS(t *testing.T) *LTS {
	t.Helper()
	arpabetPath := "../../data/arpabet_to_ipa.json"
	if _, err := os.Stat(arpabetPath); err != nil {
		absP, _ := filepath.Abs(arpabetPath)
		t.Skipf("data file not found: %s", absP)
	}

	lts, err := NewLTS(arpabetPath)
	if err != nil {
		t.Fatalf("NewLTS: %v", err)
	}
	return lts
}

func TestLTSCommonWords(t *testing.T) {
	lts := loadTestLTS(t)

	// Test that common words produce non-empty output
	words := []string{
		"hello", "world", "computer", "beautiful", "strange",
		"phoneme", "language", "testing", "example", "algorithm",
	}

	for _, word := range words {
		ipa := lts.Phonemize(word)
		if ipa == "" {
			t.Errorf("Phonemize(%q) returned empty string", word)
		}
		t.Logf("LTS %q → %q", word, ipa)
	}
}

func TestLTSEmptyInput(t *testing.T) {
	lts := loadTestLTS(t)

	if ipa := lts.Phonemize(""); ipa != "" {
		t.Errorf("Phonemize(\"\") = %q, want empty", ipa)
	}
	if ipa := lts.Phonemize("   "); ipa != "" {
		t.Errorf("Phonemize(\"   \") = %q, want empty", ipa)
	}
}

func TestLTSNonLetters(t *testing.T) {
	lts := loadTestLTS(t)

	// Words with non-letter characters should still work (letters extracted)
	if ipa := lts.Phonemize("123"); ipa != "" {
		t.Errorf("Phonemize(\"123\") = %q, want empty (no letters)", ipa)
	}
}

func TestLTSCaseInsensitive(t *testing.T) {
	lts := loadTestLTS(t)

	ipa1 := lts.Phonemize("Hello")
	ipa2 := lts.Phonemize("hello")
	ipa3 := lts.Phonemize("HELLO")

	if ipa1 != ipa2 || ipa2 != ipa3 {
		t.Errorf("case variants differ: %q, %q, %q", ipa1, ipa2, ipa3)
	}
}

func TestLTSKnownPronunciations(t *testing.T) {
	lts := loadTestLTS(t)

	// These are approximate — LTS won't be perfect, but should be in the right ballpark.
	// We just check that output contains expected phoneme substrings.
	tests := []struct {
		word     string
		contains string // expected substring in IPA output
	}{
		{"cat", "k"},    // should start with k
		{"dog", "d"},    // should start with d
		{"fish", "f"},   // should start with f
		{"ship", "ʃ"},   // should start with ʃ
		{"think", "θ"},  // should start with θ
		{"sing", "s"},   // should start with s
	}

	for _, tt := range tests {
		ipa := lts.Phonemize(tt.word)
		if ipa == "" {
			t.Errorf("Phonemize(%q) returned empty", tt.word)
			continue
		}
		if len(ipa) > 0 && !containsRune(ipa, tt.contains) {
			t.Errorf("Phonemize(%q) = %q, expected to contain %q", tt.word, ipa, tt.contains)
		}
	}
}

func containsRune(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && contains(s, substr)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSplitARPABETStress(t *testing.T) {
	tests := []struct {
		input      string
		wantBase   string
		wantStress int
	}{
		{"eh1", "EH", 1},
		{"ax0", "AX", 0},
		{"aa2", "AA", 2},
		{"b", "B", -1},
		{"ch", "CH", -1},
		{"ng", "NG", -1},
		{"", "", -1},
	}

	for _, tt := range tests {
		base, stress := splitARPABETStress(tt.input)
		if base != tt.wantBase || stress != tt.wantStress {
			t.Errorf("splitARPABETStress(%q) = (%q, %d), want (%q, %d)",
				tt.input, base, stress, tt.wantBase, tt.wantStress)
		}
	}
}

func TestLTSDecisionTreeData(t *testing.T) {
	// Verify the data arrays are properly loaded
	if len(ltsRules) == 0 {
		t.Fatal("ltsRules is empty")
	}
	if len(ltsPhoneTable) == 0 {
		t.Fatal("ltsPhoneTable is empty")
	}
	if ltsPhoneTable[0] != "epsilon" {
		t.Errorf("ltsPhoneTable[0] = %q, want \"epsilon\"", ltsPhoneTable[0])
	}

	// Verify letter index entries point to valid locations
	for i := 0; i < 26; i++ {
		idx := ltsLetterIndex[i]
		if int(idx) >= len(ltsRules) {
			t.Errorf("letter %c index %d >= len(ltsRules) %d", 'a'+i, idx, len(ltsRules))
		}
	}

	// Count leaf nodes
	leaves := 0
	for _, node := range ltsRules {
		if node.feat == 255 {
			leaves++
		}
	}
	t.Logf("LTS data: %d total nodes, %d leaf nodes, %d phones",
		len(ltsRules), leaves, len(ltsPhoneTable))
}
