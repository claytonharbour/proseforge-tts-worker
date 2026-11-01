package phonemizer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
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
	clauseEnd := map[int64]bool{3: true, 1: true, 2: true} // , ; :
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

// isDecimalPoint checks whether the period at runes[dotIdx] sits between two
// digits — "$2.8", "0.31", "2.5 percent" — rather than ending a sentence.
//
// Without it SplitSentences cut a number in half and synthesis narrated the
// pieces as separate sentences (#59):
//
//	"It cost $2.8 million."       → ["It cost $2."  "8 million."]
//	"His BAC was 0.31 at the time." → ["His BAC was 0."  "31 at the time."]
//
// 88 occurrences across 14 books, including the 0.31 a plot turns on. Commas
// were never affected: "3,000" has no period to split on.
//
// ⚑ This is the third instance of one shape — a character-class list that was
// finished for the case that prompted it and no further. `0147227` added the
// abbreviation guard for "Dr."; #58 added '*' to isPunctuation beside brackets
// that had been added the same way. The guard belongs here, next to
// isAbbreviationDot, because both answer the same question: is this period
// structure inside a token rather than the end of a sentence?
func isDecimalPoint(runes []rune, dotIdx int) bool {
	if dotIdx == 0 || dotIdx+1 >= len(runes) {
		return false
	}
	return unicode.IsDigit(runes[dotIdx-1]) && unicode.IsDigit(runes[dotIdx+1])
}

// isInitialismDot reports whether the period at dotIdx sits INSIDE a run of
// single letters separated by periods — "a.m.", "p.m.", "A.R.", "U.S.A." — rather
// than ending a sentence.
//
// Third instance of one class, and the first two are the lines above:
//
//	#17  isAbbreviationDot   Dr. Mr. Mrs.      a word list
//	#59  isDecimalPoint      $2.8  0.001%      a digit on each side
//	#67  this                a.m. A.R.         a SINGLE letter on each side
//
// ⚑ Measured before building, @wayland's count on the 2026-08-06 corpus: 156
// period-flanked single-letter runs across 25 forms, of which 51 are defensibly
// MID-SENTENCE — a lowercase word or a comma/colon follows the dot, and no
// English sentence ends that way. 51 is a floor, not an estimate; a further 69
// are ambiguous (a capital follows) and are deliberately left alone.
//
// The cost of not having this is not a mispronunciation, it is a prosodic break:
// "It was 9 a.m. sharp." split into three fragments, each phonemized and
// tokenized as its own sentence, so a boundary lands inside a time.
//
// ⚠️ SINGLE letter each side is the whole discrimination. Requiring the letter
// before the dot to be preceded by a non-letter is what stops this matching an
// ordinary sentence end like "…in Ohio. And then" — there `o` is preceded by
// `i`, so the run is a word, not an initial.
func isInitialismDot(runes []rune, dotIdx int) bool {
	if dotIdx == 0 || dotIdx+1 >= len(runes) {
		return false
	}
	if !unicode.IsLetter(runes[dotIdx-1]) || !unicode.IsLetter(runes[dotIdx+1]) {
		return false
	}
	// The letter before the dot must stand alone: start of text, or preceded by
	// something that is not a letter. "a" in "9 a.m." qualifies; "o" in "Ohio."
	// does not.
	if dotIdx-2 >= 0 && unicode.IsLetter(runes[dotIdx-2]) {
		return false
	}
	return true
}

// initialismPrefixes are dotted forms whose TRAILING period is usually not a
// sentence end. Kept separate from the abbreviations map on purpose: that map
// also drives expansion (dr. -> doctor), and these must not be expanded.
var initialismPrefixes = map[string]bool{"a.m": true, "p.m": true}

// isInitialismTrailingDot reports whether the period at dotIdx closes a dotted
// form like "a.m." AND is followed by lowercase — in which case it is not a
// sentence end.
//
// ⚠️ The lowercase test is the whole rule and it must not be dropped. "The
// meeting is at 9 a.m." genuinely ends there, so guarding this unconditionally
// would weld two sentences together — trading @wayland's 11 mid-sentence splits
// for a different defect on the 4 that really are sentence ends.
//
// That discriminator is his, from the #67 count: a lowercase word or a comma or
// colon after the dot, because no English sentence ends that way. The ambiguous
// case — a capital follows — is deliberately left splitting, since a capital is
// exactly as consistent with a new sentence as with a continuation.
func isInitialismTrailingDot(runes []rune, dotIdx int) bool {
	end := dotIdx
	start := end
	for start > 0 && runes[start-1] != ' ' && runes[start-1] != '\t' {
		start--
	}
	if start == end || !initialismPrefixes[strings.ToLower(string(runes[start:end]))] {
		return false
	}
	for j := dotIdx + 1; j < len(runes); j++ {
		if runes[j] == ' ' || runes[j] == '\t' {
			continue
		}
		return unicode.IsLower(runes[j]) || runes[j] == ',' || runes[j] == ':'
	}
	return false
}

// SplitSentences splits text at sentence-ending punctuation (. ! ?),
// returning individual sentences that can be independently phonemized.
// Periods after known abbreviations (Dr., Mr., etc.) and periods inside
// decimals (0.31) are not treated as sentence boundaries.
func SplitSentences(text string) []string {
	runes := []rune(text)
	var segments []string
	start := 0

	for i := 0; i < len(runes); i++ {
		if runes[i] == '!' || runes[i] == '?' ||
			(runes[i] == '.' && !isAbbreviationDot(runes, i) && !isDecimalPoint(runes, i) &&
				!isInitialismDot(runes, i) && !isInitialismTrailingDot(runes, i)) {
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
