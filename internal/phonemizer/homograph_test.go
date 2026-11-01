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
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read future (will)",
			sentence: "I will read the book",
			idx:      2,
			wantIPA:  "ɹˈid",
		},
		{
			name:     "read past participle (had)",
			sentence: "She had read it before",
			idx:      2,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read past tense (screen)",
			sentence: "The screen read",
			idx:      2,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read past tense (it)",
			sentence: "it read",
			idx:      1,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read past tense with trailing colon",
			sentence: "The sentence on her screen read: Q3",
			idx:      5,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read past tense with trailing period",
			sentence: "The budget. report read.",
			idx:      3,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read past tense (she — 3rd person singular)",
			sentence: "She read the book",
			idx:      1,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read past tense (he — 3rd person singular)",
			sentence: "He read it aloud",
			idx:      1,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read past tense (then — adverb)",
			sentence: "She then read the letter",
			idx:      2,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read past tense (finally — adverb)",
			sentence: "He finally read the report",
			idx:      2,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read default (no context)",
			sentence: "read",
			idx:      0,
			wantIPA:  "ɹˈɛd", // default is past tense (more common in prose)
		},
		{
			name:     "read present tense (to read)",
			sentence: "I want to read this book",
			idx:      3,
			wantIPA:  "ɹˈid",
		},
		{
			name:     "read present tense (can read)",
			sentence: "I can read fast",
			idx:      2,
			wantIPA:  "ɹˈid",
		},
		{
			name:     "read past tense (proper noun subject)",
			sentence: "Corbin read the file",
			idx:      1,
			wantIPA:  "ɹˈɛd",
		},
		// Imperative: capitalized, clause-initial "Read …" is a command (present).
		{
			name:     "read imperative (Read it again — object 'it' must not trigger past)",
			sentence: "Read it again.",
			idx:      0,
			wantIPA:  "ɹˈid",
		},
		{
			name:     "read imperative (Read them slowly)",
			sentence: "Read them slowly.",
			idx:      0,
			wantIPA:  "ɹˈid",
		},
		// Negated modals: "couldn't/don't read" is present/infinitive, not past —
		// the nearer negated-modal trigger must beat a stray pronoun like "he".
		{
			name:     "read negated modal (He couldn't read the data)",
			sentence: "He couldn't read the data",
			idx:      2,
			wantIPA:  "ɹˈid",
		},
		{
			name:     "read negated modal (they don't read)",
			sentence: "they don't read as repetition",
			idx:      2,
			wantIPA:  "ɹˈid",
		},
		// Regression: lowercase "read" with no trigger stays past-tense default.
		{
			name:     "read lowercase no-trigger stays past",
			sentence: "the foundation read differently",
			idx:      2,
			wantIPA:  "ɹˈɛd",
		},
		// Imperative needs a clause boundary, not just a capital — stylized
		// ALL-CAPS mid-sentence must NOT flip to present.
		{
			name:     "read all-caps mid-sentence stays past (not imperative)",
			sentence: "THE WARDS READ THEM AS REPETITION",
			idx:      2,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read imperative after a period (Open the archive. Read them first)",
			sentence: "Open the archive. Read them first.",
			idx:      3,
			wantIPA:  "ɹˈid",
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

// TestHomographReadLiveProbe pins Sten's Deploy-4 heteronym probe (Clayton's
// ear finding): all 8 read/live instances with their correct English readings.
func TestHomographReadLiveProbe(t *testing.T) {
	h := loadTestHomographs(t)

	tests := []struct {
		name     string
		sentence string
		idx      int
		wantIPA  string
	}{
		{
			name:     "read past (had read the ledger)",
			sentence: "She had read the ledger twice last winter, and tonight she sat down to read it again.",
			idx:      2,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read present (sat down to read it)",
			sentence: "She had read the ledger twice last winter, and tonight she sat down to read it again.",
			idx:      14,
			wantIPA:  "ɹˈid",
		},
		{
			name:     "live adjective (went live on the air)",
			sentence: "He went live on the air at noon, though he had never learned to live with the silence after.",
			idx:      2,
			wantIPA:  "lˈIv",
		},
		{
			name:     "live verb (to live with the silence)",
			sentence: "He went live on the air at noon, though he had never learned to live with the silence after.",
			idx:      14,
			wantIPA:  "lˈɪv",
		},
		{
			name:     "read past (records he read that morning)",
			sentence: "The records he read that morning were the same records she would read tomorrow.",
			idx:      3,
			wantIPA:  "ɹˈɛd",
		},
		{
			name:     "read future (she would read tomorrow)",
			sentence: "The records he read that morning were the same records she would read tomorrow.",
			idx:      12,
			wantIPA:  "ɹˈid",
		},
		{
			name:     "live verb (they live three blocks away)",
			sentence: "They live three blocks from the studio where the live broadcast began.",
			idx:      1,
			wantIPA:  "lˈɪv",
		},
		{
			name:     "live adjective (the live broadcast)",
			sentence: "They live three blocks from the studio where the live broadcast began.",
			idx:      9,
			wantIPA:  "lˈIv",
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
	if got != "lˈIv" {
		t.Errorf("got %q, want %q", got, "lˈIv")
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
			wantIPA:  "wˈɪnd",
		},
		{
			name:     "wind verb (clock)",
			sentence: "Wind the clock",
			idx:      0,
			wantIPA:  "wˈInd",
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
			wantIPA:  "klˈOz",
		},
		{
			name:     "close adjective (call)",
			sentence: "A close call",
			idx:      1,
			wantIPA:  "klˈOs",
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
			wantIPA:  "tˈɪɹ",
		},
		{
			name:     "tear verb (paper)",
			sentence: "Tear the paper",
			idx:      0,
			wantIPA:  "tˈɛɹ",
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
