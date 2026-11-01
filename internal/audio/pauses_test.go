//go:build unit

package audio

import (
	"math"
	"testing"
)

func generateTone(sampleRate int, durationSec float64, freq float64, amplitude float32) []float32 {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float32, n)
	for i := range samples {
		samples[i] = amplitude * float32(math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate)))
	}
	return samples
}

func generateSilence(sampleRate int, durationSec float64) []float32 {
	return make([]float32, int(durationSec*float64(sampleRate)))
}

func TestDetectPauses_PureSilence(t *testing.T) {
	samples := generateSilence(24000, 1.0)
	result := DetectPauses(samples, 24000, -40.0, 0.1)
	if result.PauseCount != 1 {
		t.Errorf("expected 1 pause for pure silence, got %d", result.PauseCount)
	}
	if result.SilenceRatio < 0.99 {
		t.Errorf("expected ~1.0 silence ratio, got %.2f", result.SilenceRatio)
	}
}

func TestDetectPauses_NoPauses(t *testing.T) {
	samples := generateTone(24000, 1.0, 440.0, 0.8)
	result := DetectPauses(samples, 24000, -40.0, 0.1)
	if result.PauseCount != 0 {
		t.Errorf("expected 0 pauses for continuous tone, got %d", result.PauseCount)
	}
	if result.SilenceRatio > 0.01 {
		t.Errorf("expected ~0 silence ratio, got %.2f", result.SilenceRatio)
	}
}

func TestDetectPauses_ToneSilenceTone(t *testing.T) {
	sr := 24000
	tone1 := generateTone(sr, 0.5, 440.0, 0.8)
	silence := generateSilence(sr, 0.3)
	tone2 := generateTone(sr, 0.5, 440.0, 0.8)

	samples := make([]float32, 0, len(tone1)+len(silence)+len(tone2))
	samples = append(samples, tone1...)
	samples = append(samples, silence...)
	samples = append(samples, tone2...)

	result := DetectPauses(samples, sr, -40.0, 0.1)
	if result.PauseCount != 1 {
		t.Errorf("expected 1 pause, got %d", result.PauseCount)
	}
	if result.PauseCount > 0 {
		p := result.Pauses[0]
		if p.Duration < 0.2 || p.Duration > 0.5 {
			t.Errorf("expected pause ~0.3s, got %.3fs", p.Duration)
		}
	}
}

func TestDetectPauses_MultiplePauses(t *testing.T) {
	sr := 24000
	var samples []float32
	for i := 0; i < 3; i++ {
		samples = append(samples, generateTone(sr, 0.3, 440.0, 0.8)...)
		samples = append(samples, generateSilence(sr, 0.2)...)
	}
	samples = append(samples, generateTone(sr, 0.3, 440.0, 0.8)...)

	result := DetectPauses(samples, sr, -40.0, 0.1)
	if result.PauseCount != 3 {
		t.Errorf("expected 3 pauses, got %d", result.PauseCount)
	}
}

func TestDetectPauses_MinDurationFilter(t *testing.T) {
	sr := 24000
	tone := generateTone(sr, 0.5, 440.0, 0.8)
	shortSilence := generateSilence(sr, 0.05) // below 0.1s threshold
	tone2 := generateTone(sr, 0.5, 440.0, 0.8)

	samples := make([]float32, 0)
	samples = append(samples, tone...)
	samples = append(samples, shortSilence...)
	samples = append(samples, tone2...)

	result := DetectPauses(samples, sr, -40.0, 0.1)
	if result.PauseCount != 0 {
		t.Errorf("expected 0 pauses (below min duration), got %d", result.PauseCount)
	}
}

func TestDetectPauses_ShortInput(t *testing.T) {
	samples := make([]float32, 100) // way below frameLength
	result := DetectPauses(samples, 24000, -40.0, 0.1)
	if result.PauseCount != 0 {
		t.Errorf("expected 0 pauses for short input, got %d", result.PauseCount)
	}
}
