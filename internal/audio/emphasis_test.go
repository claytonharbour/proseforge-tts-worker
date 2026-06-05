//go:build unit

package audio

import (
	"fmt"
	"math"
	"testing"
)

func TestIsFunctionWord(t *testing.T) {
	// Should match (case-insensitive)
	functionWordCases := []string{
		"the", "The", "THE",
		"a", "an",
		"her", "him", "they", "we",
		"to", "of", "in", "on", "at", "by", "for", "with",
		"and", "but", "or",
		"is", "was", "were", "has", "have", "had",
		"not", "just", "yet", "that",
	}
	for _, w := range functionWordCases {
		if !IsFunctionWord(w) {
			t.Errorf("IsFunctionWord(%q) = false, want true", w)
		}
	}

	// Should not match
	contentWordCases := []string{
		"concrete", "rhythm", "clung", "echo",
		"district", "familiar", "clothes", "pocked",
		"hello", "world",
	}
	for _, w := range contentWordCases {
		if IsFunctionWord(w) {
			t.Errorf("IsFunctionWord(%q) = true, want false", w)
		}
	}
}

func TestFunctionWordListSize(t *testing.T) {
	// Plan specifies ~60 function words
	count := len(functionWords)
	if count < 55 || count > 70 {
		t.Errorf("functionWords has %d entries, expected ~60", count)
	}
}

func TestAnalyzeEmphasis_NilAlignment(t *testing.T) {
	report := AnalyzeEmphasis(nil, 24000, nil)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Anomalies) != 0 {
		t.Errorf("got %d anomalies for nil alignment, want 0", len(report.Anomalies))
	}
}

func TestAnalyzeEmphasis_EmptyAlignment(t *testing.T) {
	alignment := &Alignment{Words: []AlignedWord{}}
	report := AnalyzeEmphasis(make([]float32, 24000), 24000, alignment)
	if len(report.Anomalies) != 0 {
		t.Errorf("got %d anomalies for empty alignment, want 0", len(report.Anomalies))
	}
}

func TestAnalyzeEmphasis_DetectsFunctionWordStress(t *testing.T) {
	// Create synthetic audio: 20 content words at quiet level, one loud function word.
	// Need enough content words for the +-7 sliding window.
	sampleRate := 24000
	numWords := 20
	msPerWord := 200
	duration := float64(numWords*msPerWord) / 1000.0
	samples := make([]float32, int(duration*float64(sampleRate))+sampleRate) // +1s buffer

	words := []AlignedWord{}
	functionIdx := 10 // place "her" at index 10 (plenty of context on both sides)
	for i := 0; i < numWords; i++ {
		word := fmt.Sprintf("word%d", i)
		if i == functionIdx {
			word = "her" // function word
		}
		words = append(words, AlignedWord{
			Word:       word,
			StartMs:    i * msPerWord,
			EndMs:      (i + 1) * msPerWord,
			Confidence: 0.9,
		})
	}

	// Fill content words with quiet sine, function word with loud sine + higher pitch.
	for i, w := range words {
		startSample := w.StartMs * sampleRate / 1000
		endSample := w.EndMs * sampleRate / 1000
		if endSample > len(samples) {
			endSample = len(samples)
		}
		amplitude := float32(0.05) // quiet
		freq := 200.0
		if i == functionIdx {
			amplitude = 0.5 // 10x louder for the function word
			freq = 350.0    // much higher pitch
		}
		for j := startSample; j < endSample; j++ {
			samples[j] = amplitude * float32(math.Sin(2*math.Pi*freq*float64(j)/float64(sampleRate)))
		}
	}

	alignment := &Alignment{Words: words, Duration: duration}
	report := AnalyzeEmphasis(samples, sampleRate, alignment)

	// Should detect at least one anomaly for "her"
	found := false
	for _, a := range report.Anomalies {
		if a.Word.Word == "her" {
			found = true
			if a.Type != "function_word_stress" {
				t.Errorf("anomaly type = %q, want %q", a.Type, "function_word_stress")
			}
			if a.CompositeZ < 1.5 {
				t.Errorf("compositeZ = %.2f, want >= 1.5", a.CompositeZ)
			}
		}
	}
	if !found {
		t.Error("expected 'her' to be flagged as function_word_stress anomaly")
	}
}

func TestAnalyzeEmphasis_NoFalsePositivesOnUniformAudio(t *testing.T) {
	// Uniform audio with equal energy on all words should produce no anomalies.
	sampleRate := 24000
	duration := 2.0
	samples := make([]float32, int(duration*float64(sampleRate)))

	words := []AlignedWord{}
	msPerWord := 180
	wordList := []string{"the", "quick", "brown", "fox", "jumps", "over", "the", "lazy", "dog", "now"}
	for i, w := range wordList {
		words = append(words, AlignedWord{
			Word:       w,
			StartMs:    i * msPerWord,
			EndMs:      (i + 1) * msPerWord,
			Confidence: 0.9,
		})
	}

	// Fill all words with same amplitude/pitch
	for _, w := range words {
		startSample := w.StartMs * sampleRate / 1000
		endSample := w.EndMs * sampleRate / 1000
		if endSample > len(samples) {
			endSample = len(samples)
		}
		for j := startSample; j < endSample; j++ {
			samples[j] = 0.1 * float32(math.Sin(2*math.Pi*200.0*float64(j)/float64(sampleRate)))
		}
	}

	alignment := &Alignment{Words: words, Duration: duration}
	report := AnalyzeEmphasis(samples, sampleRate, alignment)

	if len(report.Anomalies) > 0 {
		t.Errorf("got %d anomalies on uniform audio, want 0", len(report.Anomalies))
		for _, a := range report.Anomalies {
			t.Logf("  anomaly: %q z=%.2f", a.Word.Word, a.CompositeZ)
		}
	}
}

func TestComputeWordRMS(t *testing.T) {
	// 1.0 amplitude square wave → RMS should be 1.0
	samples := make([]float32, 100)
	for i := range samples {
		samples[i] = 1.0
	}
	rms := computeWordRMS(samples, 0, 100)
	if math.Abs(rms-1.0) > 0.01 {
		t.Errorf("RMS of all-1.0 samples = %.3f, want 1.0", rms)
	}

	// Silence → RMS should be 0
	silence := make([]float32, 100)
	rms = computeWordRMS(silence, 0, 100)
	if rms > 0.001 {
		t.Errorf("RMS of silence = %.3f, want ~0", rms)
	}

	// Edge cases
	if computeWordRMS(samples, 50, 50) != 0 {
		t.Error("RMS of zero-length range should be 0")
	}
	if computeWordRMS(samples, -1, 10) != 0 {
		t.Error("RMS of negative start should be 0")
	}
}

func TestComputeWordPitch(t *testing.T) {
	frames := []PitchFrame{
		{TimeSec: 0.0, F0: 200, Voiced: true},
		{TimeSec: 0.01, F0: 210, Voiced: true},
		{TimeSec: 0.02, F0: 0, Voiced: false},
		{TimeSec: 0.03, F0: 220, Voiced: true},
		{TimeSec: 0.04, F0: 230, Voiced: true},
	}

	// Window [0.0, 0.03) should include first 3 frames, but only voiced ones count
	meanF0 := computeWordPitch(frames, 0.0, 0.03)
	expected := (200.0 + 210.0) / 2.0
	if math.Abs(meanF0-expected) > 0.1 {
		t.Errorf("meanF0 = %.1f, want %.1f", meanF0, expected)
	}

	// No voiced frames in range
	meanF0 = computeWordPitch(frames, 0.02, 0.03)
	if meanF0 != 0 {
		t.Errorf("meanF0 for unvoiced range = %.1f, want 0", meanF0)
	}
}

func TestBuildContext(t *testing.T) {
	words := []AlignedWord{
		{Word: "the"}, {Word: "quick"}, {Word: "brown"}, {Word: "fox"},
		{Word: "jumps"}, {Word: "over"}, {Word: "the"}, {Word: "lazy"}, {Word: "dog"},
	}

	// Target word in the middle
	ctx := buildContext(words, 4)
	if ctx != "the quick brown fox JUMPS over the lazy dog" {
		t.Errorf("context = %q", ctx)
	}

	// Target word near start
	ctx = buildContext(words, 1)
	if ctx != "the QUICK brown fox jumps over..." {
		t.Errorf("context near start = %q", ctx)
	}

	// Target word near end
	ctx = buildContext(words, 7)
	if ctx != "...fox jumps over the LAZY dog" {
		t.Errorf("context near end = %q", ctx)
	}
}
