//go:build unit

package audio

import (
	"math"
	"testing"
)

// sineWave generates a sine wave at the given frequency for numSamples samples.
func sineWave(freq float64, numSamples int) []float32 {
	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * freq * float64(i) / float64(SampleRate)))
	}
	return samples
}

func TestTrimSilence_AllSilence(t *testing.T) {
	// All zeros should return empty slice
	silence := make([]float32, SampleRate) // 1 second of silence
	result := TrimSilence(silence)
	if len(result) != 0 {
		t.Errorf("all silence: got %d samples, want 0", len(result))
	}
}

func TestTrimSilence_NoSilence(t *testing.T) {
	// Pure tone — should preserve nearly all samples.
	// Frame-based analysis can't cover the final partial frame, so allow
	// up to (frameLength + hopLength) samples lost at the tail.
	tone := sineWave(440, SampleRate) // 1 second of 440Hz
	result := TrimSilence(tone)

	maxLoss := 2048 + 512 // frameLength + hopLength
	if len(result) < len(tone)-maxLoss || len(result) > len(tone) {
		t.Errorf("no silence: got %d samples, want %d (±%d)", len(result), len(tone), maxLoss)
	}
}

func TestTrimSilence_LeadingSilence(t *testing.T) {
	// 0.5s silence + 0.5s tone — should trim the leading silence
	silence := make([]float32, SampleRate/2)
	tone := sineWave(440, SampleRate/2)
	input := append(silence, tone...)

	result := TrimSilence(input)

	// Result should be significantly shorter than input (leading silence removed)
	if len(result) >= len(input) {
		t.Errorf("leading silence not trimmed: result %d >= input %d", len(result), len(input))
	}
	// Result should contain most of the tone
	if len(result) < len(tone)/2 {
		t.Errorf("too much trimmed: result %d < half tone %d", len(result), len(tone)/2)
	}
}

func TestTrimSilence_TrailingSilence(t *testing.T) {
	// 0.5s tone + 0.5s silence — should trim the trailing silence
	tone := sineWave(440, SampleRate/2)
	silence := make([]float32, SampleRate/2)
	input := append(tone, silence...)

	result := TrimSilence(input)

	if len(result) >= len(input) {
		t.Errorf("trailing silence not trimmed: result %d >= input %d", len(result), len(input))
	}
	if len(result) < len(tone)/2 {
		t.Errorf("too much trimmed: result %d < half tone %d", len(result), len(tone)/2)
	}
}

func TestTrimSilence_BothSides(t *testing.T) {
	// 0.5s silence + 0.5s tone + 0.5s silence — should trim both sides
	silence := make([]float32, SampleRate/2)
	tone := sineWave(440, SampleRate/2)

	input := make([]float32, 0, len(silence)*2+len(tone))
	input = append(input, silence...)
	input = append(input, tone...)
	input = append(input, silence...)

	result := TrimSilence(input)

	// Should be close to tone length, definitely shorter than input
	if len(result) >= len(input) {
		t.Errorf("silence not trimmed: result %d >= input %d", len(result), len(input))
	}
	// Should have removed roughly 1 second of silence total
	removed := len(input) - len(result)
	expectedRemoved := SampleRate // ~1 second of silence
	tolerance := SampleRate / 4   // Allow 0.25s tolerance (frame boundaries)
	if removed < expectedRemoved-tolerance {
		t.Errorf("too little removed: %d samples, expected ~%d (±%d)", removed, expectedRemoved, tolerance)
	}
}

func TestTrimSilence_ShortInput(t *testing.T) {
	// Input shorter than one frame should be returned as-is
	short := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	result := TrimSilence(short)
	if len(result) != len(short) {
		t.Errorf("short input: got %d samples, want %d", len(result), len(short))
	}
}

func TestTrimSilence_Empty(t *testing.T) {
	result := TrimSilence(nil)
	if len(result) != 0 {
		t.Errorf("nil input: got %d samples, want 0", len(result))
	}

	result = TrimSilence([]float32{})
	if len(result) != 0 {
		t.Errorf("empty input: got %d samples, want 0", len(result))
	}
}
