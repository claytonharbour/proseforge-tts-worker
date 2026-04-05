package audio

import (
	"strings"
)

// IntonationAnomaly represents a segment flagged for unusual pitch contour.
type IntonationAnomaly struct {
	Type     string  // "monotone_segment", "wrong_question_inflection", "uptalk", "missing_pitch_reset"
	StartMs  int
	EndMs    int
	F0Range  float64 // Hz range for the segment
	F0Start  float64 // pitch at segment start (for inflection checks)
	F0End    float64 // pitch at segment end
	AvgRange float64 // local average F0 range for comparison
	Context  string  // sentence text
	Severity string  // "low", "medium", "high"
}

// IntonationReport is the full intonation analysis result.
type IntonationReport struct {
	AudioFile string
	Duration  float64
	Sentences int
	Anomalies []IntonationAnomaly
}

// sentence holds a parsed sentence from source text.
type sentence struct {
	text         string
	endsQuestion bool
	words        []string // normalized words
}

// segmentSentences splits source text into sentences on '.', '!', '?'.
// Keeps track of whether each sentence ends with a question mark.
func segmentSentences(sourceText string) []sentence {
	if strings.TrimSpace(sourceText) == "" {
		return nil
	}

	var sentences []sentence
	var current strings.Builder

	for _, r := range sourceText {
		current.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			text := strings.TrimSpace(current.String())
			if text != "" {
				words := normalizeForWER(text)
				if len(words) > 0 {
					sentences = append(sentences, sentence{
						text:         text,
						endsQuestion: r == '?',
						words:        words,
					})
				}
			}
			current.Reset()
		}
	}

	// Handle trailing text without terminal punctuation.
	trailing := strings.TrimSpace(current.String())
	if trailing != "" {
		words := normalizeForWER(trailing)
		if len(words) > 0 {
			sentences = append(sentences, sentence{
				text:         trailing,
				endsQuestion: false,
				words:        words,
			})
		}
	}

	return sentences
}

// AnalyzeIntonation checks for intonation anomalies by examining the F0 contour
// per sentence. It uses Whisper alignment to map sentences to time ranges.
func AnalyzeIntonation(samples []float32, sampleRate int, alignment *Alignment, sourceText string) *IntonationReport {
	report := &IntonationReport{}

	if alignment == nil || len(alignment.Words) == 0 {
		return report
	}

	report.Duration = alignment.Duration

	// Extract full pitch contour.
	pitchStats := ExtractPitch(samples, sampleRate)
	frames := pitchStats.Frames

	// Parse sentences from source text.
	sents := segmentSentences(sourceText)
	report.Sentences = len(sents)

	if len(sents) == 0 {
		return report
	}

	// Map each sentence to alignment timestamps by matching normalized words.
	type sentenceRange struct {
		sent    sentence
		startMs int
		endMs   int
	}

	ranges := mapSentencesToAlignment(sents, alignment.Words)

	// Compute per-sentence F0 stats and detect anomalies.
	var allRangesHz []float64

	for _, sr := range ranges {
		startSec := float64(sr.startMs) / 1000.0
		endSec := float64(sr.endMs) / 1000.0

		voicedF0 := voicedFramesInRange(frames, startSec, endSec)
		if len(voicedF0) < 2 {
			continue
		}

		f0Range := f0RangeOf(voicedF0)
		allRangesHz = append(allRangesHz, f0Range)
	}

	avgRange := 0.0
	if len(allRangesHz) > 0 {
		sum := 0.0
		for _, r := range allRangesHz {
			sum += r
		}
		avgRange = sum / float64(len(allRangesHz))
	}

	for _, sr := range ranges {
		startSec := float64(sr.startMs) / 1000.0
		endSec := float64(sr.endMs) / 1000.0

		voicedF0 := voicedFramesInRange(frames, startSec, endSec)
		if len(voicedF0) < 2 {
			continue
		}

		f0Range := f0RangeOf(voicedF0)

		// First and last 3 voiced frame F0 values.
		f0Start := avgOfFirst(voicedF0, 3)
		f0End := avgOfLast(voicedF0, 3)

		// Monotone detection: range < 20 Hz and at least 5 words.
		if f0Range < 20 && len(sr.sent.words) >= 5 {
			severity := "low"
			if f0Range < 10 {
				severity = "high"
			} else if f0Range < 15 {
				severity = "medium"
			}
			report.Anomalies = append(report.Anomalies, IntonationAnomaly{
				Type:     "monotone_segment",
				StartMs:  sr.startMs,
				EndMs:    sr.endMs,
				F0Range:  f0Range,
				F0Start:  f0Start,
				F0End:    f0End,
				AvgRange: avgRange,
				Context:  sr.sent.text,
				Severity: severity,
			})
		}

		// Question inflection: sentence ends with '?' but final F0 falls.
		if sr.sent.endsQuestion {
			if f0End < f0Start {
				report.Anomalies = append(report.Anomalies, IntonationAnomaly{
					Type:     "wrong_question_inflection",
					StartMs:  sr.startMs,
					EndMs:    sr.endMs,
					F0Range:  f0Range,
					F0Start:  f0Start,
					F0End:    f0End,
					AvgRange: avgRange,
					Context:  sr.sent.text,
					Severity: "medium",
				})
			}
		}

		// Uptalk: sentence ends with '.' (or no punctuation) but final F0 rises >15 Hz.
		if !sr.sent.endsQuestion && len(voicedF0) >= 6 {
			// Compare last 3 frames to the 3 frames before them.
			penultimate := avgOfSlice(voicedF0, len(voicedF0)-6, len(voicedF0)-3)
			final := avgOfLast(voicedF0, 3)
			if final-penultimate > 15 {
				report.Anomalies = append(report.Anomalies, IntonationAnomaly{
					Type:     "uptalk",
					StartMs:  sr.startMs,
					EndMs:    sr.endMs,
					F0Range:  f0Range,
					F0Start:  f0Start,
					F0End:    f0End,
					AvgRange: avgRange,
					Context:  sr.sent.text,
					Severity: "low",
				})
			}
		}
	}

	return report
}

// sentenceRange holds a sentence mapped to alignment timestamps.
type sentenceRange struct {
	sent    sentence
	startMs int
	endMs   int
}

// mapSentencesToAlignment matches normalized sentence words against alignment words
// sequentially to determine time boundaries for each sentence.
func mapSentencesToAlignment(sents []sentence, words []AlignedWord) []sentenceRange {
	var ranges []sentenceRange

	alignIdx := 0
	for _, s := range sents {
		if alignIdx >= len(words) {
			break
		}

		// Try to match the first word of this sentence in the alignment.
		matchStart := -1
		for i := alignIdx; i < len(words); i++ {
			normWord := strings.ToLower(words[i].Word)
			normWord = werPunctRe.ReplaceAllString(normWord, "")
			if normWord == s.words[0] {
				matchStart = i
				break
			}
		}

		if matchStart < 0 {
			continue
		}

		// Match remaining sentence words sequentially.
		matchEnd := matchStart
		sentWordIdx := 0
		for i := matchStart; i < len(words) && sentWordIdx < len(s.words); i++ {
			normWord := strings.ToLower(words[i].Word)
			normWord = werPunctRe.ReplaceAllString(normWord, "")
			if normWord == s.words[sentWordIdx] {
				matchEnd = i
				sentWordIdx++
			}
		}

		ranges = append(ranges, sentenceRange{
			sent:    s,
			startMs: words[matchStart].StartMs,
			endMs:   words[matchEnd].EndMs,
		})

		alignIdx = matchEnd + 1
	}

	return ranges
}

// voicedFramesInRange returns the F0 values of voiced frames within a time range.
func voicedFramesInRange(frames []PitchFrame, startSec, endSec float64) []float64 {
	var f0s []float64
	for _, f := range frames {
		if f.TimeSec >= startSec && f.TimeSec < endSec && f.Voiced {
			f0s = append(f0s, f.F0)
		}
	}
	return f0s
}

// f0RangeOf returns max - min of a slice of F0 values.
func f0RangeOf(f0s []float64) float64 {
	if len(f0s) == 0 {
		return 0
	}
	min, max := f0s[0], f0s[0]
	for _, v := range f0s[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return max - min
}

// avgOfFirst returns the average of the first n values (or all if fewer).
func avgOfFirst(vals []float64, n int) float64 {
	if len(vals) == 0 {
		return 0
	}
	if n > len(vals) {
		n = len(vals)
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += vals[i]
	}
	return sum / float64(n)
}

// avgOfLast returns the average of the last n values (or all if fewer).
func avgOfLast(vals []float64, n int) float64 {
	if len(vals) == 0 {
		return 0
	}
	if n > len(vals) {
		n = len(vals)
	}
	sum := 0.0
	start := len(vals) - n
	for i := start; i < len(vals); i++ {
		sum += vals[i]
	}
	return sum / float64(n)
}

// avgOfSlice returns the average of vals[start:end].
func avgOfSlice(vals []float64, start, end int) float64 {
	if start < 0 {
		start = 0
	}
	if end > len(vals) {
		end = len(vals)
	}
	if start >= end {
		return 0
	}
	sum := 0.0
	for i := start; i < end; i++ {
		sum += vals[i]
	}
	return sum / float64(end - start)
}
