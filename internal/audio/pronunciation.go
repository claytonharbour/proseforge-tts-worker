package audio

// PronunciationError represents a word-level mispronunciation or diff error.
type PronunciationError struct {
	Type       string  // "substitution", "insertion", "deletion"
	Expected   string  // source text word (empty for insertions)
	Heard      string  // Whisper transcription (empty for deletions)
	StartMs    int     // timestamp from Whisper alignment
	EndMs      int
	Confidence float64 // Whisper confidence (lower = less certain)
	Context    string  // surrounding words
	Severity   string  // "low", "medium", "high"
}

// PronunciationReport holds the full pronunciation error analysis.
type PronunciationReport struct {
	AudioFile    string
	Duration     float64
	SourceWords  int
	WhisperWords int
	Errors       []PronunciationError
}

// pronEditOp represents a single edit operation from the Levenshtein backtrace.
type pronEditOp struct {
	errType  string // "substitution", "insertion", "deletion"
	srcIdx   int    // index into srcWords (-1 for insertions)
	whispIdx int    // index into whisperWords/alignment.Words (-1 for deletions)
}

// DetectPronunciationErrors compares Whisper alignment output against source text
// to identify mispronunciations, insertions, and deletions.
// It normalizes both inputs using normalizeForWER, runs Levenshtein edit distance,
// and maps errors back to Whisper timestamps and confidence scores.
func DetectPronunciationErrors(alignment *Alignment, sourceText string) *PronunciationReport {
	if alignment == nil {
		return &PronunciationReport{}
	}

	srcWords := normalizeForWER(sourceText)
	whisperWords := make([]string, len(alignment.Words))
	for i, w := range alignment.Words {
		normed := normalizeForWER(w.Word)
		if len(normed) > 0 {
			whisperWords[i] = normed[0]
		}
	}

	report := &PronunciationReport{
		Duration:     alignment.Duration,
		SourceWords:  len(srcWords),
		WhisperWords: len(whisperWords),
	}

	if len(srcWords) == 0 {
		return report
	}

	// Run Levenshtein edit distance on word sequences, preserving backtrace
	// with indices into both source and whisper word arrays.
	n := len(srcWords)
	m := len(whisperWords)

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
			if srcWords[i-1] == whisperWords[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				sub := dp[i-1][j-1] + 1
				del := dp[i-1][j] + 1
				ins := dp[i][j-1] + 1
				dp[i][j] = min(sub, min(del, ins))
			}
		}
	}

	// Backtrace to collect errors with position indices
	var edits []pronEditOp
	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && srcWords[i-1] == whisperWords[j-1] {
			// Match
			i--
			j--
		} else if i > 0 && j > 0 && dp[i][j] == dp[i-1][j-1]+1 {
			// Substitution
			edits = append(edits, pronEditOp{
				errType:  "substitution",
				srcIdx:   i - 1,
				whispIdx: j - 1,
			})
			i--
			j--
		} else if i > 0 && dp[i][j] == dp[i-1][j]+1 {
			// Deletion (word in source but not in whisper)
			edits = append(edits, pronEditOp{
				errType:  "deletion",
				srcIdx:   i - 1,
				whispIdx: -1,
			})
			i--
		} else {
			// Insertion (word in whisper but not in source)
			edits = append(edits, pronEditOp{
				errType:  "insertion",
				srcIdx:   -1,
				whispIdx: j - 1,
			})
			j--
		}
	}

	// Reverse edits (backtrace gives them in reverse order)
	for left, right := 0, len(edits)-1; left < right; left, right = left+1, right-1 {
		edits[left], edits[right] = edits[right], edits[left]
	}

	// Convert source words to AlignedWord for buildContext reuse
	srcAligned := make([]AlignedWord, len(srcWords))
	for idx, w := range srcWords {
		srcAligned[idx] = AlignedWord{Word: w}
	}

	// Build PronunciationErrors with timestamps, confidence, context, and severity
	for _, e := range edits {
		pe := PronunciationError{
			Type: e.errType,
		}

		switch e.errType {
		case "substitution":
			pe.Expected = srcWords[e.srcIdx]
			pe.Heard = whisperWords[e.whispIdx]
			pe.StartMs = alignment.Words[e.whispIdx].StartMs
			pe.EndMs = alignment.Words[e.whispIdx].EndMs
			pe.Confidence = alignment.Words[e.whispIdx].Confidence
			pe.Context = buildContext(srcAligned, e.srcIdx)

			// Severity: "high" if character-level Levenshtein > 3, else "medium"
			pe.Severity = "medium"
			if charLevenshtein(pe.Expected, pe.Heard) > 3 {
				pe.Severity = "high"
			}

			// Filter: very short word substitutions are often Whisper errors
			if len(pe.Expected) <= 2 || len(pe.Heard) <= 2 {
				pe.Severity = "low"
			}

		case "insertion":
			pe.Heard = whisperWords[e.whispIdx]
			pe.StartMs = alignment.Words[e.whispIdx].StartMs
			pe.EndMs = alignment.Words[e.whispIdx].EndMs
			pe.Confidence = alignment.Words[e.whispIdx].Confidence
			pe.Severity = "low"
			// Context: use nearest source word position
			contextIdx := findNearestSrcIdx(e.whispIdx, edits, len(srcWords))
			pe.Context = buildContext(srcAligned, contextIdx)

		case "deletion":
			pe.Expected = srcWords[e.srcIdx]
			pe.Context = buildContext(srcAligned, e.srcIdx)
			pe.Severity = "medium"
			// No whisper word to get timestamps from; find nearest whisper word
			nearIdx := findNearestWhisperIdx(e.srcIdx, edits, alignment)
			if nearIdx >= 0 && nearIdx < len(alignment.Words) {
				pe.StartMs = alignment.Words[nearIdx].StartMs
				pe.EndMs = alignment.Words[nearIdx].EndMs
			}
		}

		// Low confidence words get severity "low" regardless of error type
		if pe.Confidence > 0 && pe.Confidence < 0.5 {
			pe.Severity = "low"
		}

		report.Errors = append(report.Errors, pe)
	}

	return report
}

// charLevenshtein computes the character-level Levenshtein distance between two strings.
func charLevenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	n := len(ra)
	m := len(rb)

	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}

	// Use single-row DP for space efficiency
	prev := make([]int, m+1)
	curr := make([]int, m+1)
	for j := 0; j <= m; j++ {
		prev[j] = j
	}

	for i := 1; i <= n; i++ {
		curr[0] = i
		for j := 1; j <= m; j++ {
			if ra[i-1] == rb[j-1] {
				curr[j] = prev[j-1]
			} else {
				curr[j] = 1 + min(prev[j-1], min(prev[j], curr[j-1]))
			}
		}
		prev, curr = curr, prev
	}

	return prev[m]
}

// findNearestSrcIdx finds the nearest source word index for an insertion error.
// It looks through other errors to find a nearby one that has a srcIdx, falling
// back to index 0 if no neighbor is found.
func findNearestSrcIdx(whispIdx int, edits []pronEditOp, srcLen int) int {
	bestIdx := 0
	bestDist := whispIdx + srcLen // large initial distance
	for _, e := range edits {
		if e.srcIdx >= 0 && e.whispIdx >= 0 {
			dist := whispIdx - e.whispIdx
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist = dist
				bestIdx = e.srcIdx
			}
		}
	}
	if bestIdx >= srcLen {
		bestIdx = srcLen - 1
	}
	if bestIdx < 0 {
		bestIdx = 0
	}
	return bestIdx
}

// findNearestWhisperIdx finds the nearest whisper word index for a deletion error.
// It looks for nearby edit operations with valid whisper indices, falling back to
// a position ratio estimate.
func findNearestWhisperIdx(srcIdx int, edits []pronEditOp, alignment *Alignment) int {
	if alignment == nil || len(alignment.Words) == 0 {
		return -1
	}

	bestIdx := -1
	bestDist := srcIdx + len(alignment.Words)
	for _, e := range edits {
		if e.whispIdx >= 0 && e.srcIdx >= 0 {
			dist := srcIdx - e.srcIdx
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist = dist
				bestIdx = e.whispIdx
			}
		}
	}

	if bestIdx < 0 {
		// Fallback: estimate from position ratio
		ratio := float64(srcIdx) / float64(srcIdx+1)
		bestIdx = int(ratio * float64(len(alignment.Words)))
	}

	if bestIdx >= len(alignment.Words) {
		bestIdx = len(alignment.Words) - 1
	}
	if bestIdx < 0 {
		bestIdx = 0
	}
	return bestIdx
}
