package phonemizer

import (
	"strings"
	"unicode"
)

// Phonemizer is the main G2P (grapheme-to-phoneme) pipeline.
type Phonemizer struct {
	custom     *MisakiDict // optional author-provided pronunciations
	gold       *MisakiDict
	silver     *MisakiDict
	homographs *HomographResolver
	lts        *LTS
	tokenizer  *Tokenizer
}

// Config holds paths to data files needed by the phonemizer.
type Config struct {
	GoldDictPath   string
	SilverDictPath string
	HomographsPath string
	TokenMapPath   string
	ARPABETMapPath string // needed by LTS for ARPABET→IPA conversion
	CustomDictPath string // optional — custom pronunciations for proper nouns etc.
}

// New creates a new Phonemizer with all components loaded.
func New(cfg Config) (*Phonemizer, error) {
	gold, err := LoadMisakiDict(cfg.GoldDictPath)
	if err != nil {
		return nil, err
	}

	silver, err := LoadMisakiDict(cfg.SilverDictPath)
	if err != nil {
		return nil, err
	}

	homographs, err := LoadHomographs(cfg.HomographsPath)
	if err != nil {
		return nil, err
	}

	lts, err := NewLTS(cfg.ARPABETMapPath)
	if err != nil {
		return nil, err
	}

	tokenizer, err := LoadTokenizer(cfg.TokenMapPath)
	if err != nil {
		return nil, err
	}

	// Custom dict is optional — nil if path is empty or file doesn't exist
	var custom *MisakiDict
	if cfg.CustomDictPath != "" {
		custom, _ = LoadMisakiDict(cfg.CustomDictPath)
	}

	return &Phonemizer{
		custom:     custom,
		gold:       gold,
		silver:     silver,
		homographs: homographs,
		lts:        lts,
		tokenizer:  tokenizer,
	}, nil
}

// Phonemize converts text to a Kokoro IPA phoneme string.
func (p *Phonemizer) Phonemize(text string) (string, error) {
	if text == "" {
		return "", nil
	}

	// Step 1: Normalize text
	normalized := NormalizeText(text)

	// Step 2: Split into words and phonemize each
	words := strings.Fields(normalized)
	var result strings.Builder

	for i, word := range words {
		// Strip surrounding punctuation
		prefix, core, suffix := splitPunctuation(word)

		if prefix != "" {
			result.WriteString(prefix)
		}

		if core != "" {
			phonemes := p.phonemizeWord(words, i, core)
			result.WriteString(phonemes)
		}

		if suffix != "" {
			result.WriteString(suffix)
		}

		// Add space between words
		if i < len(words)-1 {
			result.WriteString(" ")
		}
	}

	return result.String(), nil
}

// SentenceTokens holds the token sequence for one or more sentences grouped
// for a single inference call. AllTokens is used post-inference to locate
// clause boundaries and insert silence at the corresponding waveform positions.
type SentenceTokens struct {
	Segments  [][]int64 // 509-token-limited chunks for ONNX inference
	AllTokens []int64   // flat token sequence for clause boundary detection
}

// TokenizeText runs the full pipeline: text → paragraphs → sentences → phonemes → token IDs.
// Consecutive sentences whose combined tokens fit under 509 are grouped into a
// single inference call so the model produces natural cross-sentence prosody.
// Returns [paragraph][group]SentenceTokens.
func (p *Phonemizer) TokenizeText(text string) ([][]SentenceTokens, error) {
	paragraphs := SplitParagraphs(text)
	if len(paragraphs) == 0 {
		return nil, nil
	}

	const spaceToken int64 = 16

	result := make([][]SentenceTokens, 0, len(paragraphs))
	for _, para := range paragraphs {
		sentences := SplitSentences(para)

		// Tokenize each sentence individually
		var sentTokens [][]int64
		for _, sent := range sentences {
			phonemes, err := p.Phonemize(sent)
			if err != nil {
				return nil, err
			}
			tokens := p.tokenizer.Tokenize(phonemes)
			if len(tokens) > 0 {
				sentTokens = append(sentTokens, tokens)
			}
		}

		// Group consecutive sentences that fit under MaxTokensPerSegment
		var paraGroups []SentenceTokens
		i := 0
		for i < len(sentTokens) {
			allTokens := append([]int64{}, sentTokens[i]...)
			j := i + 1
			for j < len(sentTokens) {
				// +1 for space token between sentences
				if len(allTokens)+1+len(sentTokens[j]) > MaxTokensPerSegment {
					break
				}
				allTokens = append(allTokens, spaceToken)
				allTokens = append(allTokens, sentTokens[j]...)
				j++
			}

			segments := p.tokenizer.SplitTokens(allTokens)
			paraGroups = append(paraGroups, SentenceTokens{
				Segments:  segments,
				AllTokens: allTokens,
			})
			i = j
		}

		if len(paraGroups) > 0 {
			result = append(result, paraGroups)
		}
	}
	return result, nil
}

// phonemizeWord converts a single word to IPA phonemes.
func (p *Phonemizer) phonemizeWord(words []string, idx int, word string) string {
	lower := strings.ToLower(word)

	// The article "a" is always reduced to schwa in connected speech.
	// The gold dictionary has A (FACE diphthong /eɪ/) which overpronounces it.
	if lower == "a" {
		return "ə"
	}

	// The article "the" is reduced to /ðə/ in connected speech.
	// The gold dictionary has /ði/ ("thee") which sounds unnatural before consonants.
	if lower == "the" {
		return "ðə"
	}

	// Step 0: Custom dictionary (author-provided pronunciations for proper nouns etc.)
	if p.custom != nil {
		if ipa, ok := p.custom.LookupWithSuffix(lower); ok {
			return ipa
		}
	}

	// Step 1: Check homograph resolver
	if ipa, ok := p.homographs.Resolve(words, idx); ok {
		return ipa
	}

	// Step 2: Gold dictionary lookup (with suffix handling)
	if ipa, ok := p.gold.LookupWithSuffix(lower); ok {
		return ipa
	}

	// Step 3: Silver dictionary lookup (with suffix handling)
	if ipa, ok := p.silver.LookupWithSuffix(lower); ok {
		return ipa
	}

	// Step 4: Handle hyphenated words — try both dicts for each part
	if strings.Contains(lower, "-") {
		parts := strings.Split(lower, "-")
		var combined strings.Builder
		allFound := true
		for j, part := range parts {
			ipa, ok := p.lookupAllDicts(part)
			if !ok {
				allFound = false
				break
			}
			if j > 0 {
				combined.WriteString(" ")
			}
			combined.WriteString(ipa)
		}
		if allFound && combined.Len() > 0 {
			return combined.String()
		}
	}

	// Step 5: Flite LTS fallback
	if ipa := p.lts.Phonemize(word); ipa != "" {
		// Heuristic: capitalized words ending in 'a' likely need final schwa
		// (e.g., Lina→lˈinə, Sara→sˈɛɹə). LTS drops the final vowel.
		if isCapitalized(word) && strings.HasSuffix(lower, "a") && !strings.HasSuffix(ipa, "ə") {
			return ipa + "ə"
		}
		return ipa
	}

	// Step 6: Skip word
	return ""
}

// WordDiagnosis holds the result of diagnosing a single word's phonemization path.
type WordDiagnosis struct {
	Word     string // original word (after punctuation stripping)
	Phonemes string // IPA output
	Source   string // "custom", "homograph", "gold", "silver", "lts", or "skip"
}

// DiagnoseText runs the phonemization pipeline and reports which dictionary
// each word resolved from. Useful for auditing LTS fallback coverage.
func (p *Phonemizer) DiagnoseText(text string) []WordDiagnosis {
	normalized := NormalizeText(text)
	words := strings.Fields(normalized)
	var results []WordDiagnosis

	for i, word := range words {
		_, core, _ := splitPunctuation(word)
		if core == "" {
			continue
		}
		lower := strings.ToLower(core)

		// Mirror phonemizeWord resolution order
		source := "skip"
		phonemes := ""

		if lower == "a" {
			source, phonemes = "builtin", "ə"
		} else if lower == "the" {
			source, phonemes = "builtin", "ðə"
		} else if p.custom != nil {
			if ipa, ok := p.custom.LookupWithSuffix(lower); ok {
				source, phonemes = "custom", ipa
			}
		}

		if source == "skip" {
			if ipa, ok := p.homographs.Resolve(words, i); ok {
				source, phonemes = "homograph", ipa
			}
		}
		if source == "skip" {
			if ipa, ok := p.gold.LookupWithSuffix(lower); ok {
				source, phonemes = "gold", ipa
			}
		}
		if source == "skip" {
			if ipa, ok := p.silver.LookupWithSuffix(lower); ok {
				source, phonemes = "silver", ipa
			}
		}
		if source == "skip" && strings.Contains(lower, "-") {
			parts := strings.Split(lower, "-")
			allFound := true
			var combined strings.Builder
			for j, part := range parts {
				ipa, ok := p.lookupAllDicts(part)
				if !ok {
					allFound = false
					break
				}
				if j > 0 {
					combined.WriteString(" ")
				}
				combined.WriteString(ipa)
			}
			if allFound && combined.Len() > 0 {
				source, phonemes = "hyphenated", combined.String()
			}
		}
		if source == "skip" {
			if ipa := p.lts.Phonemize(core); ipa != "" {
				source, phonemes = "lts", ipa
			}
		}

		results = append(results, WordDiagnosis{
			Word:     core,
			Phonemes: phonemes,
			Source:   source,
		})
	}
	return results
}

// lookupAllDicts tries custom, gold, and silver dictionaries (with suffix handling).
func (p *Phonemizer) lookupAllDicts(word string) (string, bool) {
	if p.custom != nil {
		if ipa, ok := p.custom.LookupWithSuffix(word); ok {
			return ipa, true
		}
	}
	if ipa, ok := p.gold.LookupWithSuffix(word); ok {
		return ipa, true
	}
	if ipa, ok := p.silver.LookupWithSuffix(word); ok {
		return ipa, true
	}
	return "", false
}

// splitPunctuation separates leading/trailing punctuation from a word.
// Punctuation that maps to Kokoro tokens is preserved; other punctuation is dropped.
func splitPunctuation(word string) (prefix, core, suffix string) {
	runes := []rune(word)
	start := 0
	end := len(runes)

	// Leading punctuation
	for start < end && isPunctuation(runes[start]) {
		start++
	}
	prefix = string(runes[:start])

	// Trailing punctuation
	for end > start && isPunctuation(runes[end-1]) {
		end--
	}
	suffix = string(runes[end:])
	core = string(runes[start:end])

	return
}

// isCapitalized returns true if the word starts with an uppercase letter.
func isCapitalized(word string) bool {
	for _, r := range word {
		return unicode.IsUpper(r)
	}
	return false
}

// isPunctuation returns true for characters that have Kokoro token mappings
// or that should be stripped from words.
func isPunctuation(r rune) bool {
	switch r {
	case '.', ',', '!', '?', ';', ':', '"', '(', ')', '\u201c', '\u201d',
		'\u2014', '\u2026', '\'', '-':
		return true
	}
	return false
}
