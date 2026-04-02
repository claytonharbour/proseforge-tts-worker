package audio

import (
	"regexp"
	"strings"
)

// WERResult holds the result of a word error rate computation.
type WERResult struct {
	WER          float64  // word error rate (0.0 = perfect, 1.0 = all wrong)
	Substitutions int
	Insertions    int
	Deletions     int
	TotalRef      int     // total words in reference
	TotalHyp      int     // total words in hypothesis
	Errors       []WERError // individual error details
}

// WERError describes a single word-level error.
type WERError struct {
	Type    string // "sub", "ins", "del"
	RefWord string // reference word (empty for insertions)
	HypWord string // hypothesis word (empty for deletions)
	RefPos  int    // position in reference (-1 for insertions)
}

// ComputeWER calculates word error rate between reference and hypothesis text.
// Both inputs are normalized (lowercased, punctuation stripped) before comparison.
// Uses the standard Levenshtein edit distance algorithm on word sequences.
func ComputeWER(reference, hypothesis string) WERResult {
	refWords := normalizeForWER(reference)
	hypWords := normalizeForWER(hypothesis)

	n := len(refWords)
	m := len(hypWords)

	if n == 0 {
		return WERResult{
			WER:        float64(m),
			Insertions: m,
			TotalHyp:   m,
		}
	}

	// Dynamic programming table for edit distance
	// dp[i][j] = min edits to transform refWords[:i] into hypWords[:j]
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
			if refWords[i-1] == hypWords[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				sub := dp[i-1][j-1] + 1
				del := dp[i-1][j] + 1
				ins := dp[i][j-1] + 1
				dp[i][j] = min(sub, min(del, ins))
			}
		}
	}

	// Backtrace to identify specific errors
	var errors []WERError
	subs, ins, dels := 0, 0, 0
	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && refWords[i-1] == hypWords[j-1] {
			i--
			j--
		} else if i > 0 && j > 0 && dp[i][j] == dp[i-1][j-1]+1 {
			// Substitution
			errors = append(errors, WERError{
				Type:    "sub",
				RefWord: refWords[i-1],
				HypWord: hypWords[j-1],
				RefPos:  i - 1,
			})
			subs++
			i--
			j--
		} else if i > 0 && dp[i][j] == dp[i-1][j]+1 {
			// Deletion (word in ref but not in hyp)
			errors = append(errors, WERError{
				Type:    "del",
				RefWord: refWords[i-1],
				RefPos:  i - 1,
			})
			dels++
			i--
		} else {
			// Insertion (word in hyp but not in ref)
			errors = append(errors, WERError{
				Type:    "ins",
				HypWord: hypWords[j-1],
				RefPos:  -1,
			})
			ins++
			j--
		}
	}

	// Reverse errors (backtrace gives them in reverse order)
	for left, right := 0, len(errors)-1; left < right; left, right = left+1, right-1 {
		errors[left], errors[right] = errors[right], errors[left]
	}

	wer := float64(subs+ins+dels) / float64(n)

	return WERResult{
		WER:           wer,
		Substitutions: subs,
		Insertions:    ins,
		Deletions:     dels,
		TotalRef:      n,
		TotalHyp:      m,
		Errors:        errors,
	}
}

var werPunctRe = regexp.MustCompile(`[^\w\s]`)

// normalizeForWER lowercases, strips punctuation, and splits into words.
func normalizeForWER(text string) []string {
	text = strings.ToLower(text)
	text = werPunctRe.ReplaceAllString(text, "")
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return nil
	}
	return strings.Fields(text)
}
