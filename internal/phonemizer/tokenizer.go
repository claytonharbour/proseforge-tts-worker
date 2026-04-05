package phonemizer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const MaxTokensPerSegment = 509

// Tokenizer converts IPA phoneme strings to Kokoro token IDs.
type Tokenizer struct {
	vocab map[rune]int64
}

// LoadTokenizer loads the phoneme→token ID vocabulary.
func LoadTokenizer(tokenMapPath string) (*Tokenizer, error) {
	data, err := os.ReadFile(tokenMapPath)
	if err != nil {
		return nil, fmt.Errorf("read token map: %w", err)
	}

	var raw struct {
		Vocab map[string]int64 `json:"vocab"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse token map: %w", err)
	}

	vocab := make(map[rune]int64, len(raw.Vocab))
	for key, id := range raw.Vocab {
		runes := []rune(key)
		if len(runes) != 1 {
			continue // skip non-single-character keys
		}
		vocab[runes[0]] = id
	}

	if len(vocab) == 0 {
		return nil, fmt.Errorf("token map is empty")
	}

	return &Tokenizer{vocab: vocab}, nil
}

// Tokenize converts an IPA phoneme string to a sequence of token IDs.
// Characters not in the vocabulary are silently dropped.
func (t *Tokenizer) Tokenize(phonemes string) []int64 {
	var tokens []int64
	for _, r := range phonemes {
		if id, ok := t.vocab[r]; ok {
			tokens = append(tokens, id)
		}
	}
	return tokens
}

// SplitTokens splits a token sequence into segments of at most MaxTokensPerSegment.
// It tries to split at sentence boundaries (period, exclamation, question) or
// silence markers (comma, semicolon) when possible.
func (t *Tokenizer) SplitTokens(tokens []int64) [][]int64 {
	if len(tokens) <= MaxTokensPerSegment {
		return [][]int64{tokens}
	}

	var segments [][]int64
	start := 0

	for start < len(tokens) {
		end := start + MaxTokensPerSegment
		if end >= len(tokens) {
			segments = append(segments, tokens[start:])
			break
		}

		// Find best split point: look backwards for sentence/silence boundary
		splitAt := findSplitPoint(tokens, start, end)
		segments = append(segments, tokens[start:splitAt])
		start = splitAt
	}

	return segments
}

// findSplitPoint finds the best position to split tokens, looking backwards from end.
// Prefers: sentence end (. ! ?) > clause boundary (, ; :) > space > forced split.
func findSplitPoint(tokens []int64, start, end int) int {
	// Sentence-ending punctuation token IDs
	sentenceEnd := map[int64]bool{4: true, 5: true, 6: true} // . ! ?
	// Clause boundary token IDs
	clauseEnd := map[int64]bool{3: true, 1: true, 2: true}   // , ; :
	// Space token
	const spaceToken int64 = 16

	// Look backwards from end for best split point
	bestSentence := -1
	bestClause := -1
	bestSpace := -1

	// Search the last 30% of the segment for split points
	searchStart := start + (end-start)*7/10
	if searchStart < start {
		searchStart = start
	}

	for i := end - 1; i >= searchStart; i-- {
		if sentenceEnd[tokens[i]] {
			bestSentence = i + 1 // split after the punctuation
			break
		}
		if clauseEnd[tokens[i]] && bestClause < 0 {
			bestClause = i + 1
		}
		if tokens[i] == spaceToken && bestSpace < 0 {
			bestSpace = i + 1
		}
	}

	if bestSentence > start {
		return bestSentence
	}
	if bestClause > start {
		return bestClause
	}
	if bestSpace > start {
		return bestSpace
	}
	return end // forced split at max length
}

// VocabSize returns the number of tokens in the vocabulary.
func (t *Tokenizer) VocabSize() int {
	return len(t.vocab)
}

// TokenizeAndSplit tokenizes the phoneme string and splits into segments if needed.
func (t *Tokenizer) TokenizeAndSplit(phonemes string) [][]int64 {
	tokens := t.Tokenize(phonemes)
	return t.SplitTokens(tokens)
}

// abbreviationPrefixes is the set of lowercased words (without the dot) that
// should NOT trigger a sentence split when followed by ".". Built from the
// abbreviations map in normalizer.go.
var abbreviationPrefixes = buildAbbreviationPrefixes()

func buildAbbreviationPrefixes() map[string]bool {
	prefixes := make(map[string]bool, len(abbreviations))
	for abbr := range abbreviations {
		// "dr." → "dr", "mrs." → "mrs"
		prefixes[strings.TrimSuffix(abbr, ".")] = true
	}
	return prefixes
}

// isAbbreviationDot checks whether the period at runes[dotIdx] belongs to an
// abbreviation like "Dr." or "Mrs." rather than ending a sentence.
func isAbbreviationDot(runes []rune, dotIdx int) bool {
	// Walk backwards to find the start of the word before the dot
	end := dotIdx
	start := end
	for start > 0 && runes[start-1] != ' ' && runes[start-1] != '\t' {
		start--
	}
	if start == end {
		return false
	}
	word := strings.ToLower(string(runes[start:end]))
	return abbreviationPrefixes[word]
}

// SplitSentences splits text at sentence-ending punctuation (. ! ?),
// returning individual sentences that can be independently phonemized.
// Periods after known abbreviations (Dr., Mr., etc.) are not treated
// as sentence boundaries.
func SplitSentences(text string) []string {
	runes := []rune(text)
	var segments []string
	start := 0

	for i := 0; i < len(runes); i++ {
		if runes[i] == '!' || runes[i] == '?' || (runes[i] == '.' && !isAbbreviationDot(runes, i)) {
			// Absorb trailing closing punctuation (quotes, parens)
			for i+1 < len(runes) && isClosingPunct(runes[i+1]) {
				i++
			}
			seg := strings.TrimSpace(string(runes[start : i+1]))
			if seg != "" {
				segments = append(segments, seg)
			}
			start = i + 1
		}
	}

	// Remaining text without sentence-ending punctuation
	if start < len(runes) {
		seg := strings.TrimSpace(string(runes[start:]))
		if seg != "" {
			segments = append(segments, seg)
		}
	}

	return segments
}

func isClosingPunct(r rune) bool {
	switch r {
	case '"', ')', '\u201d', '\u2019', '\'':
		return true
	}
	return false
}

