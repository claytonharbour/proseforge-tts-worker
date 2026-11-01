package audio

import (
	"math"
	"strings"
)

// WordProminence holds acoustic measurements for a single aligned word.
type WordProminence struct {
	Word           AlignedWord
	RMS            float64
	MeanF0         float64
	Duration       float64 // seconds
	IsFunctionWord bool
}

// EmphasisAnomaly represents a word flagged for unusual emphasis.
type EmphasisAnomaly struct {
	Word       AlignedWord
	Type       string  // "function_word_stress" or "content_word_stress"
	EnergyZ    float64 // z-score for RMS energy
	PitchZ     float64 // z-score for mean F0
	DurationZ  float64 // z-score for duration
	CompositeZ float64 // weighted composite z-score
	Context    string  // surrounding words for display
	Severity   string  // "low", "medium", "high"
	AvgRMS     float64 // local average RMS for context
	AvgF0      float64 // local average F0 for context
}

// EmphasisReport is the full analysis result.
type EmphasisReport struct {
	AudioFile  string
	Duration   float64
	TotalWords int
	Anomalies  []EmphasisAnomaly
}

// functionWords is the set of ~60 common English function words that should
// typically be unstressed in natural speech.
var functionWords = map[string]bool{
	// Articles
	"a": true, "an": true, "the": true,
	// Pronouns
	"i": true, "me": true, "my": true, "you": true, "your": true,
	"he": true, "him": true, "his": true, "she": true, "her": true,
	"it": true, "its": true, "we": true, "us": true, "our": true,
	"they": true, "them": true, "their": true,
	// Prepositions
	"to": true, "of": true, "in": true, "on": true, "at": true,
	"by": true, "for": true, "with": true, "from": true, "into": true,
	"through": true, "about": true, "over": true, "under": true, "between": true,
	// Conjunctions
	"and": true, "but": true, "or": true, "so": true, "nor": true,
	"as": true, "if": true, "than": true,
	// Auxiliaries
	"is": true, "am": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "has": true, "have": true, "had": true,
	"do": true, "does": true, "did": true, "can": true, "could": true,
	"would": true, "should": true, "will": true, "shall": true,
	"may": true, "might": true, "must": true,
	// Other
	"not": true, "just": true, "yet": true, "that": true,
}

// IsFunctionWord checks whether a word is in the function word set.
// Comparison is case-insensitive.
func IsFunctionWord(word string) bool {
	return functionWords[strings.ToLower(word)]
}

// AnalyzeEmphasis computes per-word prominence and flags anomalous emphasis patterns.
// samples must be the audio corresponding to alignment. sampleRate is typically 24000.
func AnalyzeEmphasis(samples []float32, sampleRate int, alignment *Alignment) *EmphasisReport {
	if alignment == nil || len(alignment.Words) == 0 {
		return &EmphasisReport{}
	}

	// Pre-compute full pitch contour (10ms hop)
	pitchStats := ExtractPitch(samples, sampleRate)
	frames := pitchStats.Frames

	// Compute per-word prominence
	prominences := make([]WordProminence, len(alignment.Words))
	for i, w := range alignment.Words {
		startSample := msToSample(w.StartMs, sampleRate)
		endSample := msToSample(w.EndMs, sampleRate)
		if endSample > len(samples) {
			endSample = len(samples)
		}
		if startSample >= endSample {
			startSample = endSample
		}

		prominences[i] = WordProminence{
			Word:           w,
			RMS:            computeWordRMS(samples, startSample, endSample),
			MeanF0:         computeWordPitch(frames, float64(w.StartMs)/1000.0, float64(w.EndMs)/1000.0),
			Duration:       float64(w.EndMs-w.StartMs) / 1000.0,
			IsFunctionWord: IsFunctionWord(w.Word),
		}
	}

	// Flag anomalies using sliding window z-scores
	var anomalies []EmphasisAnomaly
	const windowHalf = 7 // +-7 content words

	for i, prom := range prominences {
		// Compute local stats from surrounding content words
		localRMS, localF0, localDur := localContentStats(prominences, i, windowHalf)

		if localRMS.stddev < 1e-9 && localF0.stddev < 1e-9 {
			continue // no variation at all — can't compute z-scores
		}

		// Floor stddev to avoid division by zero on individual axes.
		// Use 5% of mean as minimum stddev (conservative estimate).
		if localRMS.stddev < localRMS.mean*0.05 {
			localRMS.stddev = localRMS.mean * 0.05
		}
		if localF0.stddev < localF0.mean*0.05 {
			localF0.stddev = localF0.mean * 0.05
		}
		if localDur.stddev < localDur.mean*0.05 {
			localDur.stddev = localDur.mean * 0.05
		}

		energyZ := (prom.RMS - localRMS.mean) / localRMS.stddev
		pitchZ := 0.0
		if prom.MeanF0 > 0 && localF0.mean > 0 {
			pitchZ = (prom.MeanF0 - localF0.mean) / localF0.stddev
		}
		durationZ := 0.0
		if localDur.stddev > 1e-9 {
			durationZ = (prom.Duration - localDur.mean) / localDur.stddev
		}

		compositeZ := 0.4*energyZ + 0.4*pitchZ + 0.2*durationZ

		var anomalyType string
		var threshold float64

		if prom.IsFunctionWord {
			anomalyType = "function_word_stress"
			threshold = 1.5
		} else {
			anomalyType = "content_word_stress"
			threshold = 2.0
		}

		if compositeZ >= threshold {
			severity := "low"
			if prom.IsFunctionWord {
				if compositeZ >= 2.5 {
					severity = "high"
				} else if compositeZ >= 2.0 {
					severity = "medium"
				}
			} else {
				if compositeZ >= 3.0 {
					severity = "high"
				} else if compositeZ >= 2.5 {
					severity = "medium"
				}
			}

			anomalies = append(anomalies, EmphasisAnomaly{
				Word:       prom.Word,
				Type:       anomalyType,
				EnergyZ:    energyZ,
				PitchZ:     pitchZ,
				DurationZ:  durationZ,
				CompositeZ: compositeZ,
				Context:    buildContext(alignment.Words, i),
				Severity:   severity,
				AvgRMS:     localRMS.mean,
				AvgF0:      localF0.mean,
			})
		}
	}

	return &EmphasisReport{
		Duration:   alignment.Duration,
		TotalWords: len(alignment.Words),
		Anomalies:  anomalies,
	}
}

type statPair struct {
	mean   float64
	stddev float64
}

// localContentStats computes mean/stddev of RMS, F0, and duration for content words
// within a window of +-windowHalf around index idx.
func localContentStats(proms []WordProminence, idx, windowHalf int) (rms, f0, dur statPair) {
	var rmsVals, f0Vals, durVals []float64

	// Collect content words in window (skip the target word itself)
	contentSeen := 0
	// Search backwards
	for j := idx - 1; j >= 0 && contentSeen < windowHalf; j-- {
		if !proms[j].IsFunctionWord {
			rmsVals = append(rmsVals, proms[j].RMS)
			if proms[j].MeanF0 > 0 {
				f0Vals = append(f0Vals, proms[j].MeanF0)
			}
			durVals = append(durVals, proms[j].Duration)
			contentSeen++
		}
	}
	// Search forwards
	contentSeen = 0
	for j := idx + 1; j < len(proms) && contentSeen < windowHalf; j++ {
		if !proms[j].IsFunctionWord {
			rmsVals = append(rmsVals, proms[j].RMS)
			if proms[j].MeanF0 > 0 {
				f0Vals = append(f0Vals, proms[j].MeanF0)
			}
			durVals = append(durVals, proms[j].Duration)
			contentSeen++
		}
	}

	rms = computeStatPair(rmsVals)
	f0 = computeStatPair(f0Vals)
	dur = computeStatPair(durVals)
	return
}

func computeStatPair(vals []float64) statPair {
	if len(vals) < 2 {
		return statPair{}
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean := sum / float64(len(vals))
	var sqSum float64
	for _, v := range vals {
		d := v - mean
		sqSum += d * d
	}
	stddev := math.Sqrt(sqSum / float64(len(vals)))
	return statPair{mean: mean, stddev: stddev}
}

func msToSample(ms, sampleRate int) int {
	return ms * sampleRate / 1000
}

func computeWordRMS(samples []float32, start, end int) float64 {
	if start >= end || start < 0 {
		return 0
	}
	if end > len(samples) {
		end = len(samples)
	}
	var sum float64
	n := end - start
	for i := start; i < end; i++ {
		s := float64(samples[i])
		sum += s * s
	}
	return math.Sqrt(sum / float64(n))
}

func computeWordPitch(frames []PitchFrame, startSec, endSec float64) float64 {
	var sum float64
	var count int
	for _, f := range frames {
		if f.TimeSec >= startSec && f.TimeSec < endSec && f.Voiced {
			sum += f.F0
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// buildContext returns a snippet of surrounding words with the target word uppercased.
func buildContext(words []AlignedWord, idx int) string {
	start := idx - 4
	if start < 0 {
		start = 0
	}
	end := idx + 5
	if end > len(words) {
		end = len(words)
	}

	var parts []string
	for i := start; i < end; i++ {
		if i == idx {
			parts = append(parts, strings.ToUpper(words[i].Word))
		} else {
			parts = append(parts, words[i].Word)
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
