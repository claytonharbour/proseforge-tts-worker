//go:build unit

package audio

import (
	"math"
	"testing"
)

// genSine creates a sine wave at the given frequency.
func genSine(freq float64, sampleRate int, durationSec float64) []float32 {
	n := int(float64(sampleRate) * durationSec)
	samples := make([]float32, n)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate)))
	}
	return samples
}

func TestExtractPitch_Sine200Hz(t *testing.T) {
	samples := genSine(200.0, 24000, 0.5)
	stats := ExtractPitch(samples, 24000)

	if stats.VoicedPct < 80 {
		t.Errorf("voiced%% = %.1f%%, want >= 80%%", stats.VoicedPct)
	}

	// YIN should find ~200Hz (allow ±5Hz tolerance)
	if math.Abs(stats.MeanF0-200.0) > 5.0 {
		t.Errorf("mean F0 = %.1f Hz, want ~200 Hz", stats.MeanF0)
	}
	if math.Abs(stats.MedianF0-200.0) > 5.0 {
		t.Errorf("median F0 = %.1f Hz, want ~200 Hz", stats.MedianF0)
	}
}

func TestExtractPitch_Sine100Hz(t *testing.T) {
	samples := genSine(100.0, 24000, 0.5)
	stats := ExtractPitch(samples, 24000)

	if math.Abs(stats.MeanF0-100.0) > 5.0 {
		t.Errorf("mean F0 = %.1f Hz, want ~100 Hz", stats.MeanF0)
	}
}

func TestExtractPitch_Sine400Hz(t *testing.T) {
	samples := genSine(400.0, 24000, 0.5)
	stats := ExtractPitch(samples, 24000)

	if math.Abs(stats.MeanF0-400.0) > 10.0 {
		t.Errorf("mean F0 = %.1f Hz, want ~400 Hz", stats.MeanF0)
	}
}

func TestExtractPitch_Silence(t *testing.T) {
	samples := make([]float32, 24000) // 1 second of silence
	stats := ExtractPitch(samples, 24000)

	if stats.VoicedPct > 5 {
		t.Errorf("voiced%% = %.1f%%, want < 5%% for silence", stats.VoicedPct)
	}
}

func TestExtractPitch_TooShort(t *testing.T) {
	samples := make([]float32, 100) // too short for analysis
	stats := ExtractPitch(samples, 24000)

	if len(stats.Frames) != 0 {
		t.Errorf("got %d frames for tiny input, want 0", len(stats.Frames))
	}
}
