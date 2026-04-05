//go:build unit

package phonemizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadTestHomographs(t *testing.T) *HomographResolver {
	t.Helper()
	// Find the data directory relative to the test file
	// Tests run from the package directory, so we need to go up to project root
	paths := []string{
		"../../data/homographs.json",
		filepath.Join(os.Getenv("PWD"), "../../data/homographs.json"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			h, err := LoadHomographs(p)
			if err != nil {
				t.Fatalf("LoadHomographs: %v", err)
			}
			return h
		}
	}

	t.Skip("homographs.json not found")
	return nil
}

func TestHomographRead(t *testing.T) {
	h := loadTestHomographs(t)

	tests := []struct {
		name     string
		sentence string
		idx      int
		wantIPA  string
	}{
		{
			name:     "read past tense (yesterday)",
			sentence: "I read the book yesterday",
			idx:      1,
			wantIPA:  "ɹɛd",
		},
		{
			name:     "read future (will)",
			sentence: "I will read the book",
			idx:      2,
			wantIPA:  "ɹiːd",
		},
		{
			name:     "read past participle (had)",
			sentence: "She had read it before",
			idx:      2,
			wantIPA:  "ɹɛd",
		},
		{
			name:     "read past tense (screen)",
			sentence: "The screen read",
			idx:      2,
			wantIPA:  "ɹɛd",
		},
		{
			name:     "read past tense (it)",
			sentence: "it read",
			idx:      1,
			wantIPA:  "ɹɛd",
		},
		{
			name:     "read past tense with trailing colon",
			sentence: "The sentence on her screen read: Q3",
			idx:      5,
			wantIPA:  "ɹɛd",
		},
		{
			name:     "read past tense with trailing period",
			sentence: "The budget. report read.",
			idx:      3,
			wantIPA:  "ɹɛd",
		},
		{
			name:     "read past tense (she — 3rd person singular)",
			sentence: "She read the book",
			idx:      1,
			wantIPA:  "ɹɛd",
		},
		{
			name:     "read past tense (he — 3rd person singular)",
			sentence: "He read it aloud",
			idx:      1,
			wantIPA:  "ɹɛd",
		},
		{
			name:     "read past tense (then — adverb)",
			sentence: "She then read the letter",
			idx:      2,
			wantIPA:  "ɹɛd",
		},
		{
			name:     "read past tense (finally — adverb)",
			sentence: "He finally read the report",
			idx:      2,
			wantIPA:  "ɹɛd",
		},
		{
			name:     "read default (no context)",
			sentence: "read",
			idx:      0,
			wantIPA:  "ɹɛd", // default is past tense (more common in prose)
		},
		{
			name:     "read present tense (to read)",
			sentence: "I want to read this book",
			idx:      3,
			wantIPA:  "ɹiːd",
		},
		{
			name:     "read present tense (can read)",
			sentence: "I can read fast",
			idx:      2,
			wantIPA:  "ɹiːd",
		},
		{
			name:     "read past tense (proper noun subject)",
			sentence: "Corbin read the file",
			idx:      1,
			wantIPA:  "ɹɛd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := splitWords(tt.sentence)
			got, ok := h.Resolve(words, tt.idx)
			if !ok {
				t.Fatal("expected homograph match")
			}
			if got != tt.wantIPA {
				t.Errorf("got %q, want %q", got, tt.wantIPA)
			}
		})
	}
}

func TestHomographLead(t *testing.T) {
	h := loadTestHomographs(t)

	tests := []struct {
		name     string
		sentence string
		idx      int
		wantIPA  string
	}{
		{
			name:     "lead noun (pipe)",
			sentence: "The lead pipe",
			idx:      1,
			wantIPA:  "lɛd",
		},
		{
			name:     "lead verb (the way)",
			sentence: "Lead the way",
			idx:      0,
			wantIPA:  "liːd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := splitWords(tt.sentence)
			got, ok := h.Resolve(words, tt.idx)
			if !ok {
				t.Fatal("expected homograph match")
			}
			if got != tt.wantIPA {
				t.Errorf("got %q, want %q", got, tt.wantIPA)
			}
		})
	}
}

func TestHomographLive(t *testing.T) {
	h := loadTestHomographs(t)

	words := splitWords("A live performance")
	got, ok := h.Resolve(words, 1)
	if !ok {
		t.Fatal("expected homograph match")
	}
	// "live" before "performance" should be /laɪv/ (adjective)
	if got != "lIv" {
		t.Errorf("got %q, want %q", got, "lIv")
	}
}

func TestHomographWind(t *testing.T) {
	h := loadTestHomographs(t)

	tests := []struct {
		name     string
		sentence string
		idx      int
		wantIPA  string
	}{
		{
			name:     "wind noun (blew)",
			sentence: "The wind blew hard",
			idx:      1,
			wantIPA:  "wɪnd",
		},
		{
			name:     "wind verb (clock)",
			sentence: "Wind the clock",
			idx:      0,
			wantIPA:  "wInd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := splitWords(tt.sentence)
			got, ok := h.Resolve(words, tt.idx)
			if !ok {
				t.Fatal("expected homograph match")
			}
			if got != tt.wantIPA {
				t.Errorf("got %q, want %q", got, tt.wantIPA)
			}
		})
	}
}

func TestHomographClose(t *testing.T) {
	h := loadTestHomographs(t)

	tests := []struct {
		name     string
		sentence string
		idx      int
		wantIPA  string
	}{
		{
			name:     "close verb (door)",
			sentence: "Close the door",
			idx:      0,
			wantIPA:  "klOz",
		},
		{
			name:     "close adjective (call)",
			sentence: "A close call",
			idx:      1,
			wantIPA:  "klOs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := splitWords(tt.sentence)
			got, ok := h.Resolve(words, tt.idx)
			if !ok {
				t.Fatal("expected homograph match")
			}
			if got != tt.wantIPA {
				t.Errorf("got %q, want %q", got, tt.wantIPA)
			}
		})
	}
}

func TestHomographTear(t *testing.T) {
	h := loadTestHomographs(t)

	tests := []struct {
		name     string
		sentence string
		idx      int
		wantIPA  string
	}{
		{
			name:     "tear noun (rolled)",
			sentence: "A tear rolled down",
			idx:      1,
			wantIPA:  "tɪɹ",
		},
		{
			name:     "tear verb (paper)",
			sentence: "Tear the paper",
			idx:      0,
			wantIPA:  "tɛɹ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := splitWords(tt.sentence)
			got, ok := h.Resolve(words, tt.idx)
			if !ok {
				t.Fatal("expected homograph match")
			}
			if got != tt.wantIPA {
				t.Errorf("got %q, want %q", got, tt.wantIPA)
			}
		})
	}
}

func TestHomographNotFound(t *testing.T) {
	h := loadTestHomographs(t)

	words := splitWords("hello world")
	_, ok := h.Resolve(words, 0)
	if ok {
		t.Error("expected no match for non-homograph word")
	}
}

func TestHomographCount(t *testing.T) {
	h := loadTestHomographs(t)
	if h.Count() < 10 {
		t.Errorf("expected at least 10 homographs, got %d", h.Count())
	}
}

func splitWords(s string) []string {
	return strings.Fields(s)
}
