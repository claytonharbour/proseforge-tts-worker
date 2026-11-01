package audio

import (
	"fmt"
	"math"
	"strings"
)

// PacingAnomaly represents a flagged pacing issue in the audio.
type PacingAnomaly struct {
	Type     string  // "rushed_segment", "slow_segment", "unexpected_pause", "erratic_pacing"
	StartMs  int     // start of the anomalous region
	EndMs    int     // end of the anomalous region
	Rate     float64 // words/sec for this segment (rate anomalies)
	AvgRate  float64 // local average rate for comparison
	GapMs    int     // gap duration in ms (pause anomalies)
	Variance float64 // coefficient of variation (erratic pacing)
	Context  string  // surrounding words for display
	Severity string  // "low", "medium", "high"
}

// PacingReport holds the full pacing analysis result.
type PacingReport struct {
	AudioFile   string
	Duration    float64
	TotalWords  int
	OverallRate float64 // overall speaking rate in words/sec
	Clauses     int     // number of detected clauses
	Anomalies   []PacingAnomaly
}

// clause is a group of consecutive words with no large gaps between them.
type clause struct {
	words    []AlignedWord
	startIdx int // index into the original alignment
}

// AnalyzePacing detects pacing anomalies in speech using word-level alignment.
// It identifies rushed/slow segments, unexpected pauses, and erratic pacing.
func AnalyzePacing(alignment *Alignment) *PacingReport {
	if alignment == nil || len(alignment.Words) < 3 {
		return &PacingReport{}
	}

	words := alignment.Words

	// Overall speaking rate
	totalDur := float64(words[len(words)-1].EndMs-words[0].StartMs) / 1000.0
	overallRate := 0.0
	if totalDur > 0 {
		overallRate = float64(len(words)) / totalDur
	}

	// Segment words into clauses by detecting gaps > clauseGapMs
	const clauseGapMs = 200
	clauses := segmentClauses(words, clauseGapMs)

	var anomalies []PacingAnomaly

	// 1. Speaking rate anomalies — flag clauses significantly faster/slower than local avg
	anomalies = append(anomalies, detectRateAnomalies(clauses, words)...)

	// 2. Unexpected pauses — gaps after function words where no pause is expected
	anomalies = append(anomalies, detectUnexpectedPauses(words)...)

	// 3. Erratic pacing — high variance in word spacing within a clause
	anomalies = append(anomalies, detectErraticPacing(clauses, words)...)

	return &PacingReport{
		Duration:    alignment.Duration,
		TotalWords:  len(words),
		OverallRate: overallRate,
		Clauses:     len(clauses),
		Anomalies:   anomalies,
	}
}

// segmentClauses groups consecutive words into clauses split by gaps > gapMs.
func segmentClauses(words []AlignedWord, gapMs int) []clause {
	if len(words) == 0 {
		return nil
	}

	var clauses []clause
	current := clause{startIdx: 0}
	current.words = append(current.words, words[0])

	for i := 1; i < len(words); i++ {
		gap := words[i].StartMs - words[i-1].EndMs
		if gap > gapMs {
			if len(current.words) > 0 {
				clauses = append(clauses, current)
			}
			current = clause{startIdx: i}
		}
		current.words = append(current.words, words[i])
	}
	if len(current.words) > 0 {
		clauses = append(clauses, current)
	}

	return clauses
}

// clauseRate computes words/second for a clause.
func clauseRate(c clause) float64 {
	if len(c.words) < 2 {
		return 0
	}
	dur := float64(c.words[len(c.words)-1].EndMs-c.words[0].StartMs) / 1000.0
	if dur <= 0 {
		return 0
	}
	return float64(len(c.words)) / dur
}

// detectRateAnomalies flags clauses with speaking rate significantly different from local average.
func detectRateAnomalies(clauses []clause, words []AlignedWord) []PacingAnomaly {
	if len(clauses) < 3 {
		return nil
	}

	// Compute rate for each clause (only those with >= 3 words)
	type ratedClause struct {
		idx  int
		rate float64
	}
	var rated []ratedClause
	for i, c := range clauses {
		if len(c.words) >= 3 {
			r := clauseRate(c)
			if r > 0 {
				rated = append(rated, ratedClause{idx: i, rate: r})
			}
		}
	}

	if len(rated) < 3 {
		return nil
	}

	// Compute overall mean and stddev of clause rates
	var sum float64
	for _, r := range rated {
		sum += r.rate
	}
	mean := sum / float64(len(rated))

	var sqSum float64
	for _, r := range rated {
		d := r.rate - mean
		sqSum += d * d
	}
	stddev := math.Sqrt(sqSum / float64(len(rated)))

	if stddev < mean*0.05 {
		return nil // very consistent pacing, nothing to flag
	}

	var anomalies []PacingAnomaly
	for _, r := range rated {
		z := (r.rate - mean) / stddev
		c := clauses[r.idx]

		if z > 1.8 {
			severity := "low"
			if z > 3.0 {
				severity = "high"
			} else if z > 2.5 {
				severity = "medium"
			}
			anomalies = append(anomalies, PacingAnomaly{
				Type:     "rushed_segment",
				StartMs:  c.words[0].StartMs,
				EndMs:    c.words[len(c.words)-1].EndMs,
				Rate:     r.rate,
				AvgRate:  mean,
				Context:  buildClauseContext(c),
				Severity: severity,
			})
		} else if z < -1.8 {
			severity := "low"
			if z < -3.0 {
				severity = "high"
			} else if z < -2.5 {
				severity = "medium"
			}
			anomalies = append(anomalies, PacingAnomaly{
				Type:     "slow_segment",
				StartMs:  c.words[0].StartMs,
				EndMs:    c.words[len(c.words)-1].EndMs,
				Rate:     r.rate,
				AvgRate:  mean,
				Context:  buildClauseContext(c),
				Severity: severity,
			})
		}
	}

	return anomalies
}

// detectUnexpectedPauses flags gaps after function words where no pause is expected.
// A pause between "the" and the next word (e.g., "the [400ms] quick") sounds unnatural.
func detectUnexpectedPauses(words []AlignedWord) []PacingAnomaly {
	const minGapMs = 150 // gaps shorter than this are normal articulation

	var anomalies []PacingAnomaly
	for i := 0; i < len(words)-1; i++ {
		gap := words[i+1].StartMs - words[i].EndMs
		if gap < minGapMs {
			continue
		}

		// Only flag if the word before the gap is a function word
		// (function words should flow into the next word without pause)
		if !IsFunctionWord(words[i].Word) {
			continue
		}

		// Build context showing the gap
		ctx := buildGapContext(words, i, gap)

		severity := "low"
		if gap > 500 {
			severity = "high"
		} else if gap > 300 {
			severity = "medium"
		}

		anomalies = append(anomalies, PacingAnomaly{
			Type:     "unexpected_pause",
			StartMs:  words[i].EndMs,
			EndMs:    words[i+1].StartMs,
			GapMs:    gap,
			Context:  ctx,
			Severity: severity,
		})
	}

	return anomalies
}

// detectErraticPacing flags clauses where word spacing has unusually high variance.
// A clause where some words run together and others have big gaps sounds robotic.
func detectErraticPacing(clauses []clause, words []AlignedWord) []PacingAnomaly {
	if len(clauses) < 3 {
		return nil
	}

	// Compute coefficient of variation (CV) for each clause's inter-word gaps
	type cvClause struct {
		idx int
		cv  float64
	}
	var cvs []cvClause
	for i, c := range clauses {
		cv := clauseGapCV(c)
		if cv > 0 && len(c.words) >= 4 {
			cvs = append(cvs, cvClause{idx: i, cv: cv})
		}
	}

	if len(cvs) < 3 {
		return nil
	}

	// Compute mean and stddev of CVs
	var sum float64
	for _, c := range cvs {
		sum += c.cv
	}
	mean := sum / float64(len(cvs))

	var sqSum float64
	for _, c := range cvs {
		d := c.cv - mean
		sqSum += d * d
	}
	stddev := math.Sqrt(sqSum / float64(len(cvs)))

	if stddev < 0.01 {
		return nil
	}

	var anomalies []PacingAnomaly
	for _, c := range cvs {
		z := (c.cv - mean) / stddev
		if z > 1.8 {
			cl := clauses[c.idx]
			severity := "low"
			if z > 3.0 {
				severity = "high"
			} else if z > 2.5 {
				severity = "medium"
			}
			anomalies = append(anomalies, PacingAnomaly{
				Type:     "erratic_pacing",
				StartMs:  cl.words[0].StartMs,
				EndMs:    cl.words[len(cl.words)-1].EndMs,
				Variance: c.cv,
				Context:  buildClauseContext(cl),
				Severity: severity,
			})
		}
	}

	return anomalies
}

// clauseGapCV computes the coefficient of variation of inter-word gaps within a clause.
func clauseGapCV(c clause) float64 {
	if len(c.words) < 3 {
		return 0
	}

	gaps := make([]float64, len(c.words)-1)
	for i := 1; i < len(c.words); i++ {
		gaps[i-1] = float64(c.words[i].StartMs - c.words[i-1].EndMs)
	}

	var sum float64
	for _, g := range gaps {
		sum += g
	}
	mean := sum / float64(len(gaps))

	if mean <= 0 {
		return 0
	}

	var sqSum float64
	for _, g := range gaps {
		d := g - mean
		sqSum += d * d
	}
	stddev := math.Sqrt(sqSum / float64(len(gaps)))

	return stddev / mean // coefficient of variation
}

// buildClauseContext returns the first ~10 words of a clause for display.
func buildClauseContext(c clause) string {
	limit := len(c.words)
	if limit > 10 {
		limit = 10
	}
	var parts []string
	for _, w := range c.words[:limit] {
		parts = append(parts, w.Word)
	}
	ctx := strings.Join(parts, " ")
	if limit < len(c.words) {
		ctx += "..."
	}
	return ctx
}

// buildGapContext shows words around a gap with the gap duration marked.
func buildGapContext(words []AlignedWord, gapAfterIdx int, gapMs int) string {
	start := gapAfterIdx - 2
	if start < 0 {
		start = 0
	}
	end := gapAfterIdx + 4
	if end > len(words) {
		end = len(words)
	}

	var parts []string
	for i := start; i < end; i++ {
		parts = append(parts, words[i].Word)
		if i == gapAfterIdx {
			parts = append(parts, fmt.Sprintf("[%dms]", gapMs))
		}
	}

	ctx := strings.Join(parts, " ")
	if start > 0 {
		ctx = "..." + ctx
	}
	if end < len(words) {
		ctx += "..."
	}
	return ctx
}
