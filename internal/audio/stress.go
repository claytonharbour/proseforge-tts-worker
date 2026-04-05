package audio

import (
	"math"
	"strings"
	"unicode"
)

// StressProfile holds the energy shape of a word divided into syllable-sized slices.
type StressProfile struct {
	SliceRMS     []float64
	PeakIndex    int
	PeakPosition float64 // 0.0 = word start, 1.0 = word end
}

// StressMismatch records a word where Kokoro and Gemini stress different syllables.
type StressMismatch struct {
	Word          string  `json:"word"`
	StartMs       int     `json:"start_ms"`
	KokoroPeak    float64 `json:"kokoro_peak"`    // 0.0-1.0 peak position
	GeminiPeak    float64 `json:"gemini_peak"`     // 0.0-1.0 peak position
	SyllableCount int     `json:"syllable_count"`
	Context       string  `json:"context"`
	Severity      string  `json:"severity"` // "low", "medium", "high"
}

// StressReport is the full cross-model stress comparison result.
type StressReport struct {
	TotalWords int               `json:"total_words"`
	Matched    int               `json:"matched"`
	Compared   int               `json:"compared"`   // multi-syllable words actually compared
	Mismatches []StressMismatch  `json:"mismatches"`
}

// matchedWord pairs a word across Kokoro and Gemini alignments.
type matchedWord struct {
	text       string
	kokoroWord AlignedWord
	geminiWord AlignedWord
	sourceIdx  int // position in source text words
}

// ExtractWordStress divides a word's audio into syllable-count slices and finds
// where the energy peak falls. Returns a StressProfile with per-slice RMS and
// the normalized peak position (0.0 = word start, 1.0 = word end).
func ExtractWordStress(samples []float32, sampleRate int, word AlignedWord, syllables int) StressProfile {
	if syllables < 2 {
		syllables = 2
	}

	startSample := msToSample(word.StartMs, sampleRate)
	endSample := msToSample(word.EndMs, sampleRate)
	if startSample < 0 {
		startSample = 0
	}
	if endSample > len(samples) {
		endSample = len(samples)
	}
	if startSample >= endSample {
		return StressProfile{SliceRMS: make([]float64, syllables)}
	}

	totalSamples := endSample - startSample
	sliceSize := totalSamples / syllables
	if sliceSize < 1 {
		sliceSize = 1
	}

	rmsSlices := make([]float64, syllables)
	peakIdx := 0
	peakRMS := -1.0

	for i := 0; i < syllables; i++ {
		sliceStart := startSample + i*sliceSize
		sliceEnd := sliceStart + sliceSize
		if i == syllables-1 {
			sliceEnd = endSample // last slice gets remainder
		}
		if sliceEnd > endSample {
			sliceEnd = endSample
		}

		rms := computeWordRMS(samples, sliceStart, sliceEnd)
		rmsSlices[i] = rms
		if rms > peakRMS {
			peakRMS = rms
			peakIdx = i
		}
	}

	// Normalize peak position to 0.0-1.0
	peakPos := 0.0
	if syllables > 1 {
		peakPos = (float64(peakIdx) + 0.5) / float64(syllables)
	}

	return StressProfile{
		SliceRMS:     rmsSlices,
		PeakIndex:    peakIdx,
		PeakPosition: peakPos,
	}
}

// CompareStress matches words between Kokoro and Gemini alignments, extracts
// per-word energy profiles, and flags words where the stress peak falls in a
// different position. sourceText is the original text used to anchor word matching.
func CompareStress(
	kokoroSamples, geminiSamples []float32,
	kokoroRate, geminiRate int,
	kokoroAlign, geminiAlign *Alignment,
	sourceText string,
) *StressReport {
	if kokoroAlign == nil || geminiAlign == nil {
		return &StressReport{}
	}

	matched := matchWordsByText(kokoroAlign, geminiAlign, sourceText)

	report := &StressReport{
		TotalWords: len(normalizeForWER(sourceText)),
		Matched:    len(matched),
	}

	for _, mw := range matched {
		syllables := EstimateSyllables(mw.text)
		if syllables < 2 {
			continue // single-syllable words can't have stress errors
		}

		report.Compared++

		kokoroProfile := ExtractWordStress(kokoroSamples, kokoroRate, mw.kokoroWord, syllables)
		geminiProfile := ExtractWordStress(geminiSamples, geminiRate, mw.geminiWord, syllables)

		// Check if peak positions differ significantly
		diff := math.Abs(kokoroProfile.PeakPosition - geminiProfile.PeakPosition)

		// Threshold scales inversely with syllable count:
		// 2 syllables → 0.3, 3+ syllables → 0.25
		threshold := 0.3
		if syllables >= 3 {
			threshold = 0.25
		}

		if diff > threshold {
			severity := "low"
			if diff > 0.5 {
				severity = "high"
			} else if diff > 0.35 {
				severity = "medium"
			}

			report.Mismatches = append(report.Mismatches, StressMismatch{
				Word:          mw.text,
				StartMs:       mw.kokoroWord.StartMs,
				KokoroPeak:    kokoroProfile.PeakPosition,
				GeminiPeak:    geminiProfile.PeakPosition,
				SyllableCount: syllables,
				Context:       buildContextFromSource(sourceText, mw.sourceIdx),
				Severity:      severity,
			})
		}
	}

	return report
}

// EstimateSyllables counts syllables in a word using a vowel-group heuristic.
// Counts runs of vowel letters (a, e, i, o, u, y) and adjusts for silent-e.
func EstimateSyllables(word string) int {
	word = strings.ToLower(word)

	// Strip non-letter characters
	var letters []rune
	for _, r := range word {
		if unicode.IsLetter(r) {
			letters = append(letters, r)
		}
	}
	if len(letters) == 0 {
		return 1
	}
	word = string(letters)

	vowels := "aeiouy"
	count := 0
	inVowelGroup := false

	for _, ch := range word {
		isVowel := strings.ContainsRune(vowels, ch)
		if isVowel && !inVowelGroup {
			count++
			inVowelGroup = true
		} else if !isVowel {
			inVowelGroup = false
		}
	}

	// Adjust for silent-e: if word ends in 'e' (not "le") and we counted >1, subtract 1
	if count > 1 && len(word) > 2 {
		if word[len(word)-1] == 'e' && word[len(word)-2] != 'l' {
			count--
		}
	}

	if count < 1 {
		count = 1
	}
	return count
}

// matchWordsByText pairs words from Kokoro and Gemini alignments using the
// source text as an anchor. Uses edit-distance alignment against the source
// word list to handle transcription differences.
func matchWordsByText(kokoroAlign, geminiAlign *Alignment, sourceText string) []matchedWord {
	sourceWords := normalizeForWER(sourceText)
	kokoroWords := alignedToNormalized(kokoroAlign.Words)
	geminiWords := alignedToNormalized(geminiAlign.Words)

	// Map each source word to its best Kokoro and Gemini aligned word
	kokoroMap := mapAlignmentToSource(kokoroWords, kokoroAlign.Words, sourceWords)
	geminiMap := mapAlignmentToSource(geminiWords, geminiAlign.Words, sourceWords)

	var result []matchedWord
	for srcIdx, srcWord := range sourceWords {
		kw, kOk := kokoroMap[srcIdx]
		gw, gOk := geminiMap[srcIdx]
		if kOk && gOk {
			result = append(result, matchedWord{
				text:       srcWord,
				kokoroWord: kw,
				geminiWord: gw,
				sourceIdx:  srcIdx,
			})
		}
	}
	return result
}

// mapAlignmentToSource uses edit distance to map aligned words back to source
// word positions. Returns a map from source index to the AlignedWord.
func mapAlignmentToSource(normWords []string, alignedWords []AlignedWord, sourceWords []string) map[int]AlignedWord {
	n := len(sourceWords)
	m := len(normWords)

	// DP table
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
		dp[i][0] = i
	}
	for j := 0; j <= m; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if sourceWords[i-1] == normWords[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				sub := dp[i-1][j-1] + 1
				del := dp[i-1][j] + 1
				ins := dp[i][j-1] + 1
				dp[i][j] = min(sub, min(del, ins))
			}
		}
	}

	// Backtrace to find matches
	result := make(map[int]AlignedWord)
	i, j := n, m
	for i > 0 && j > 0 {
		if sourceWords[i-1] == normWords[j-1] {
			result[i-1] = alignedWords[j-1]
			i--
			j--
		} else if dp[i][j] == dp[i-1][j-1]+1 {
			i--
			j--
		} else if dp[i][j] == dp[i-1][j]+1 {
			i--
		} else {
			j--
		}
	}
	return result
}

// alignedToNormalized extracts and normalizes word text from aligned words.
func alignedToNormalized(words []AlignedWord) []string {
	result := make([]string, len(words))
	for i, w := range words {
		result[i] = strings.ToLower(strings.TrimFunc(w.Word, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}))
	}
	return result
}

// buildContextFromSource returns a snippet of source text around word at srcIdx.
func buildContextFromSource(sourceText string, srcIdx int) string {
	words := strings.Fields(sourceText)
	start := srcIdx - 4
	if start < 0 {
		start = 0
	}
	end := srcIdx + 5
	if end > len(words) {
		end = len(words)
	}

	var parts []string
	for i := start; i < end; i++ {
		if i == srcIdx {
			parts = append(parts, strings.ToUpper(words[i]))
		} else {
			parts = append(parts, words[i])
		}
	}

	ctx := strings.Join(parts, " ")
	if start > 0 {
		ctx = "..." + ctx
	}
	if end < len(words) {
		ctx = ctx + "..."
	}
	return ctx
}
