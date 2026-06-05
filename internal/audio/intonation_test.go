//go:build unit

package audio

import (
	"math"
	"testing"
)

func TestSegmentSentences(t *testing.T) {
	sents := segmentSentences("Hello world. How are you? Fine.")

	if len(sents) != 3 {
		t.Fatalf("got %d sentences, want 3", len(sents))
	}

	// First sentence: "Hello world."
	if sents[0].text != "Hello world." {
		t.Errorf("sentence[0].text = %q, want %q", sents[0].text, "Hello world.")
	}
	if sents[0].endsQuestion {
		t.Error("sentence[0] should not be a question")
	}
	if len(sents[0].words) != 2 {
		t.Errorf("sentence[0] has %d words, want 2", len(sents[0].words))
	}

	// Second sentence: "How are you?" — should be a question
	if sents[1].text != "How are you?" {
		t.Errorf("sentence[1].text = %q, want %q", sents[1].text, "How are you?")
	}
	if !sents[1].endsQuestion {
		t.Error("sentence[1] should be a question")
	}
	if len(sents[1].words) != 3 {
		t.Errorf("sentence[1] has %d words, want 3", len(sents[1].words))
	}

	// Third sentence: "Fine."
	if sents[2].text != "Fine." {
		t.Errorf("sentence[2].text = %q, want %q", sents[2].text, "Fine.")
	}
	if sents[2].endsQuestion {
		t.Error("sentence[2] should not be a question")
	}
}

func TestSegmentSentences_TrailingText(t *testing.T) {
	sents := segmentSentences("Hello world")
	if len(sents) != 1 {
		t.Fatalf("got %d sentences, want 1", len(sents))
	}
	if sents[0].endsQuestion {
		t.Error("trailing text should not be a question")
	}
}

func TestSegmentSentences_Empty(t *testing.T) {
	sents := segmentSentences("")
	if len(sents) != 0 {
		t.Errorf("got %d sentences for empty string, want 0", len(sents))
	}

	sents = segmentSentences("   ")
	if len(sents) != 0 {
		t.Errorf("got %d sentences for whitespace, want 0", len(sents))
	}
}

func TestSegmentSentences_Exclamation(t *testing.T) {
	sents := segmentSentences("Wow! Amazing.")
	if len(sents) != 2 {
		t.Fatalf("got %d sentences, want 2", len(sents))
	}
	if sents[0].text != "Wow!" {
		t.Errorf("sentence[0].text = %q, want %q", sents[0].text, "Wow!")
	}
	if sents[0].endsQuestion {
		t.Error("exclamation should not be a question")
	}
}

func TestAnalyzeIntonation_NilAlignment(t *testing.T) {
	report := AnalyzeIntonation(nil, 24000, nil, "Hello world.")
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Anomalies) != 0 {
		t.Errorf("got %d anomalies for nil alignment, want 0", len(report.Anomalies))
	}
	if report.Sentences != 0 {
		t.Errorf("got %d sentences for nil alignment, want 0", report.Sentences)
	}
}

func TestAnalyzeIntonation_Monotone(t *testing.T) {
	// Create a synthetic signal with constant pitch (200 Hz) across a long sentence.
	// This should be flagged as monotone since the F0 range will be ~0 Hz.
	sampleRate := 24000
	durationMs := 3000 // 3 seconds for a long sentence
	numSamples := durationMs * sampleRate / 1000
	samples := make([]float32, numSamples)

	// Generate a pure 200 Hz sine wave (constant pitch = monotone).
	freq := 200.0
	for i := range samples {
		samples[i] = 0.3 * float32(math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate)))
	}

	// Create alignment words for a 7-word sentence.
	sourceText := "The quick brown fox jumps over lazily."
	words := []AlignedWord{
		{Word: "The", StartMs: 0, EndMs: 300, Confidence: 0.9},
		{Word: "quick", StartMs: 300, EndMs: 600, Confidence: 0.9},
		{Word: "brown", StartMs: 600, EndMs: 900, Confidence: 0.9},
		{Word: "fox", StartMs: 900, EndMs: 1200, Confidence: 0.9},
		{Word: "jumps", StartMs: 1200, EndMs: 1500, Confidence: 0.9},
		{Word: "over", StartMs: 1500, EndMs: 2000, Confidence: 0.9},
		{Word: "lazily", StartMs: 2000, EndMs: 2800, Confidence: 0.9},
	}

	alignment := &Alignment{Words: words, Duration: 2.8}
	report := AnalyzeIntonation(samples, sampleRate, alignment, sourceText)

	if report.Sentences != 1 {
		t.Fatalf("got %d sentences, want 1", report.Sentences)
	}

	// Should detect monotone.
	found := false
	for _, a := range report.Anomalies {
		if a.Type == "monotone_segment" {
			found = true
			if a.F0Range >= 20 {
				t.Errorf("monotone F0Range = %.1f, expected < 20", a.F0Range)
			}
			if a.Severity != "high" && a.Severity != "medium" {
				t.Errorf("monotone severity = %q, expected high or medium", a.Severity)
			}
		}
	}
	if !found {
		t.Error("expected monotone_segment anomaly for constant-pitch signal")
		for _, a := range report.Anomalies {
			t.Logf("  anomaly: type=%q range=%.1f severity=%q", a.Type, a.F0Range, a.Severity)
		}
	}
}
