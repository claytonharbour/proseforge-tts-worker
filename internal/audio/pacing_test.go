//go:build unit

package audio

import (
	"math"
	"testing"
)

func TestSegmentClauses(t *testing.T) {
	words := []AlignedWord{
		{Word: "The", StartMs: 0, EndMs: 100},
		{Word: "quick", StartMs: 110, EndMs: 250},
		{Word: "brown", StartMs: 260, EndMs: 400},
		// 300ms gap — clause break
		{Word: "fox", StartMs: 700, EndMs: 850},
		{Word: "jumped", StartMs: 860, EndMs: 1050},
	}

	clauses := segmentClauses(words, 200)

	if len(clauses) != 2 {
		t.Fatalf("got %d clauses, want 2", len(clauses))
	}
	if len(clauses[0].words) != 3 {
		t.Errorf("clause 0: %d words, want 3", len(clauses[0].words))
	}
	if len(clauses[1].words) != 2 {
		t.Errorf("clause 1: %d words, want 2", len(clauses[1].words))
	}
	if clauses[1].startIdx != 3 {
		t.Errorf("clause 1 startIdx: %d, want 3", clauses[1].startIdx)
	}
}

func TestSegmentClauses_NoGaps(t *testing.T) {
	words := []AlignedWord{
		{Word: "one", StartMs: 0, EndMs: 100},
		{Word: "two", StartMs: 110, EndMs: 200},
		{Word: "three", StartMs: 210, EndMs: 300},
	}

	clauses := segmentClauses(words, 200)
	if len(clauses) != 1 {
		t.Fatalf("got %d clauses, want 1", len(clauses))
	}
	if len(clauses[0].words) != 3 {
		t.Errorf("got %d words, want 3", len(clauses[0].words))
	}
}

func TestClauseRate(t *testing.T) {
	c := clause{
		words: []AlignedWord{
			{Word: "The", StartMs: 0, EndMs: 100},
			{Word: "quick", StartMs: 110, EndMs: 250},
			{Word: "brown", StartMs: 260, EndMs: 400},
			{Word: "fox", StartMs: 410, EndMs: 500},
		},
	}

	rate := clauseRate(c)
	// 4 words in 0.5 seconds = 8.0 words/sec
	if math.Abs(rate-8.0) > 0.1 {
		t.Errorf("got rate %.2f, want ~8.0", rate)
	}
}

func TestClauseGapCV(t *testing.T) {
	// Uniform gaps → low CV
	uniform := clause{
		words: []AlignedWord{
			{Word: "a", StartMs: 0, EndMs: 100},
			{Word: "b", StartMs: 150, EndMs: 250},
			{Word: "c", StartMs: 300, EndMs: 400},
			{Word: "d", StartMs: 450, EndMs: 550},
		},
	}
	cvUniform := clauseGapCV(uniform)
	if cvUniform > 0.01 {
		t.Errorf("uniform gaps CV: %.3f, want ~0", cvUniform)
	}

	// Mixed gaps → higher CV
	mixed := clause{
		words: []AlignedWord{
			{Word: "a", StartMs: 0, EndMs: 100},
			{Word: "b", StartMs: 110, EndMs: 200},   // 10ms gap
			{Word: "c", StartMs: 400, EndMs: 500},   // 200ms gap
			{Word: "d", StartMs: 510, EndMs: 600},   // 10ms gap
			{Word: "e", StartMs: 790, EndMs: 900},   // 190ms gap
		},
	}
	cvMixed := clauseGapCV(mixed)
	if cvMixed < 0.5 {
		t.Errorf("mixed gaps CV: %.3f, want > 0.5", cvMixed)
	}
}

func TestDetectUnexpectedPauses(t *testing.T) {
	words := []AlignedWord{
		{Word: "grabbed", StartMs: 0, EndMs: 300},
		{Word: "the", StartMs: 310, EndMs: 400},
		// 400ms gap after function word "the" → should be flagged
		{Word: "quick", StartMs: 800, EndMs: 950},
		{Word: "brown", StartMs: 960, EndMs: 1100},
		// 50ms gap after content word → should not be flagged
		{Word: "folder", StartMs: 1150, EndMs: 1350},
	}

	anomalies := detectUnexpectedPauses(words)

	if len(anomalies) != 1 {
		t.Fatalf("got %d anomalies, want 1", len(anomalies))
	}
	if anomalies[0].Type != "unexpected_pause" {
		t.Errorf("type: %s, want unexpected_pause", anomalies[0].Type)
	}
	if anomalies[0].GapMs != 400 {
		t.Errorf("gap: %d ms, want 400", anomalies[0].GapMs)
	}
	if anomalies[0].Severity != "medium" {
		t.Errorf("severity: %s, want medium", anomalies[0].Severity)
	}
}

func TestDetectUnexpectedPauses_ContentWord(t *testing.T) {
	// Gap after content word should not be flagged
	words := []AlignedWord{
		{Word: "walked", StartMs: 0, EndMs: 300},
		// 500ms gap after content word — normal sentence pause
		{Word: "she", StartMs: 800, EndMs: 900},
		{Word: "stopped", StartMs: 910, EndMs: 1100},
	}

	anomalies := detectUnexpectedPauses(words)
	if len(anomalies) != 0 {
		t.Errorf("got %d anomalies, want 0 (gap after content word is OK)", len(anomalies))
	}
}

func TestAnalyzePacing_TooFewWords(t *testing.T) {
	alignment := &Alignment{
		Words: []AlignedWord{
			{Word: "hello", StartMs: 0, EndMs: 500},
			{Word: "world", StartMs: 510, EndMs: 1000},
		},
		Duration: 1.0,
	}

	report := AnalyzePacing(alignment)
	if report.TotalWords != 0 {
		t.Errorf("expected empty report for <3 words, got %d words", report.TotalWords)
	}
}

func TestAnalyzePacing_NilAlignment(t *testing.T) {
	report := AnalyzePacing(nil)
	if report.TotalWords != 0 {
		t.Errorf("expected empty report for nil alignment")
	}
}

func TestBuildGapContext(t *testing.T) {
	words := []AlignedWord{
		{Word: "grabbed"},
		{Word: "the"},
		{Word: "quick"},
		{Word: "brown"},
		{Word: "folder"},
	}

	ctx := buildGapContext(words, 1, 400)
	if ctx != "grabbed the [400ms] quick brown folder" {
		t.Errorf("got %q", ctx)
	}
}

func TestBuildClauseContext(t *testing.T) {
	c := clause{
		words: []AlignedWord{
			{Word: "the"},
			{Word: "quick"},
			{Word: "brown"},
		},
	}
	ctx := buildClauseContext(c)
	if ctx != "the quick brown" {
		t.Errorf("got %q, want %q", ctx, "the quick brown")
	}
}

func TestDetectRateAnomalies_ConsistentRate(t *testing.T) {
	// Build 5 clauses all at ~5 words/sec → no anomalies
	var clauses []clause
	for i := 0; i < 5; i++ {
		base := i * 2000
		clauses = append(clauses, clause{
			startIdx: i * 5,
			words: []AlignedWord{
				{Word: "word1", StartMs: base, EndMs: base + 150},
				{Word: "word2", StartMs: base + 200, EndMs: base + 350},
				{Word: "word3", StartMs: base + 400, EndMs: base + 550},
				{Word: "word4", StartMs: base + 600, EndMs: base + 750},
				{Word: "word5", StartMs: base + 800, EndMs: base + 950},
			},
		})
	}

	anomalies := detectRateAnomalies(clauses, nil)
	if len(anomalies) != 0 {
		t.Errorf("got %d anomalies, want 0 for consistent rate", len(anomalies))
	}
}
