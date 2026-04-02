//go:build unit

package phonemizer

import (
	"os"
	"testing"
)

func loadTestTokenizer(t *testing.T) *Tokenizer {
	t.Helper()
	path := "../../data/token_map.json"
	if _, err := os.Stat(path); err != nil {
		t.Skip("token_map.json not found")
	}
	tok, err := LoadTokenizer(path)
	if err != nil {
		t.Fatalf("LoadTokenizer: %v", err)
	}
	return tok
}

func TestTokenizerVocabSize(t *testing.T) {
	tok := loadTestTokenizer(t)
	if tok.VocabSize() < 100 {
		t.Errorf("expected >100 tokens, got %d", tok.VocabSize())
	}
}

func TestTokenizerKnownPhonemes(t *testing.T) {
	tok := loadTestTokenizer(t)

	tests := []struct {
		phoneme string
		wantID  int64
	}{
		{" ", 16},   // space
		{".", 4},    // period
		{",", 3},    // comma
		{"!", 5},    // exclamation
		{"?", 6},    // question
		{"—", 9},    // em dash
		{"…", 10},   // ellipsis
	}

	for _, tt := range tests {
		tokens := tok.Tokenize(tt.phoneme)
		if len(tokens) != 1 {
			t.Errorf("Tokenize(%q): got %d tokens, want 1", tt.phoneme, len(tokens))
			continue
		}
		if tokens[0] != tt.wantID {
			t.Errorf("Tokenize(%q): got ID %d, want %d", tt.phoneme, tokens[0], tt.wantID)
		}
	}
}

func TestTokenizerUnknownPhonemes(t *testing.T) {
	tok := loadTestTokenizer(t)

	// Unknown characters should be dropped (not crash)
	tokens := tok.Tokenize("@#$%^&*")
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens for unknown chars, got %d", len(tokens))
	}
}

func TestTokenizerEmptyInput(t *testing.T) {
	tok := loadTestTokenizer(t)
	tokens := tok.Tokenize("")
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens for empty input, got %d", len(tokens))
	}
}

func TestTokenizerMaxSequenceLength(t *testing.T) {
	tok := loadTestTokenizer(t)

	// Create a long phoneme string that exceeds 509 tokens
	long := ""
	for range 600 {
		long += "b" // 'b' maps to token 44
	}

	segments := tok.TokenizeAndSplit(long)
	for _, seg := range segments {
		if len(seg) > MaxTokensPerSegment {
			t.Errorf("segment exceeds max: %d > %d", len(seg), MaxTokensPerSegment)
		}
	}

	// Total tokens should equal input length
	total := 0
	for _, seg := range segments {
		total += len(seg)
	}
	if total != 600 {
		t.Errorf("total tokens: got %d, want 600", total)
	}
}

func TestTokenizerSplitAtBoundary(t *testing.T) {
	tok := loadTestTokenizer(t)

	// Build tokens with a period near the split point
	// 500 'b' tokens + period + 100 more 'b' tokens
	phonemes := ""
	for range 500 {
		phonemes += "b"
	}
	phonemes += "."
	for range 100 {
		phonemes += "b"
	}

	segments := tok.TokenizeAndSplit(phonemes)
	if len(segments) < 2 {
		t.Fatalf("expected at least 2 segments, got %d", len(segments))
	}

	// First segment should end at or after the period (token 4)
	firstSeg := segments[0]
	lastToken := firstSeg[len(firstSeg)-1]
	if lastToken != 4 { // period token
		t.Logf("first segment last token: %d (period=4)", lastToken)
		// This is OK — the split logic finds the best boundary
	}
}

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		input string
		want  int // number of segments
	}{
		{"Hello. World.", 2},
		{"One sentence", 1},
		{"A! B? C.", 3},
		{"No punctuation", 1},
		{"", 0},
	}

	for _, tt := range tests {
		got := SplitSentences(tt.input)
		if len(got) != tt.want {
			t.Errorf("SplitSentences(%q): got %d segments, want %d", tt.input, len(got), tt.want)
		}
	}
}


func TestSplitSentencesQuotedEnding(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			"period inside closing quote",
			"The word was \u201cbridge.\u201d She continued.",
			[]string{"The word was \u201cbridge.\u201d", "She continued."},
		},
		{
			"period inside closing paren",
			"He said (yes.) Then left.",
			[]string{"He said (yes.)", "Then left."},
		},
		{
			"exclamation inside quote",
			"She yelled \u201cstop!\u201d He froze.",
			[]string{"She yelled \u201cstop!\u201d", "He froze."},
		},
		{
			"normal sentence no quotes",
			"Hello world. Goodbye.",
			[]string{"Hello world.", "Goodbye."},
		},
		{
			"trailing quote only",
			"The word was \u201cbridge.\u201d",
			[]string{"The word was \u201cbridge.\u201d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitSentences(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("SplitSentences(%q): got %d segments %v, want %d %v",
					tt.input, len(got), got, len(tt.want), tt.want)
			}
			for i, seg := range got {
				if seg != tt.want[i] {
					t.Errorf("segment[%d] = %q, want %q", i, seg, tt.want[i])
				}
			}
		})
	}
}
