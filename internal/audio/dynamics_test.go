//go:build unit

package audio

import (
	"math"
	"testing"
)

func TestExpandDynamicRange_LoudGetsLouder(t *testing.T) {
	// Create a signal with a loud section and a quiet section.
	// After expansion, the ratio between them should increase.
	n := SampleRate // 1 second
	samples := make([]float32, n)

	// First half: loud sine (amplitude 0.8)
	for i := 0; i < n/2; i++ {
		samples[i] = 0.8 * float32(math.Sin(2*math.Pi*440*float64(i)/float64(SampleRate)))
	}
	// Second half: quiet sine (amplitude 0.2)
	for i := n / 2; i < n; i++ {
		samples[i] = 0.2 * float32(math.Sin(2*math.Pi*440*float64(i)/float64(SampleRate)))
	}

	// Measure RMS of each half before expansion
	loudRMSBefore := rmsOf(samples[:n/2])
	quietRMSBefore := rmsOf(samples[n/2:])
	ratioBefore := loudRMSBefore / quietRMSBefore

	// Expand
	expanded := ExpandDynamicRange(samples)

	// Measure RMS of each half after expansion
	loudRMSAfter := rmsOf(expanded[:n/2])
	quietRMSAfter := rmsOf(expanded[n/2:])
	ratioAfter := loudRMSAfter / quietRMSAfter

	t.Logf("Before: loud=%.4f quiet=%.4f ratio=%.2f", loudRMSBefore, quietRMSBefore, ratioBefore)
	t.Logf("After:  loud=%.4f quiet=%.4f ratio=%.2f", loudRMSAfter, quietRMSAfter, ratioAfter)

	if ratioAfter <= ratioBefore {
		t.Errorf("expected ratio to increase after expansion: before=%.2f after=%.2f", ratioBefore, ratioAfter)
	}
}

func TestExpandDynamicRange_SilenceUntouched(t *testing.T) {
	// Silence should remain silence
	samples := make([]float32, SampleRate)
	expanded := ExpandDynamicRange(samples)

	for i, s := range expanded {
		if s != 0 {
			t.Fatalf("sample[%d] = %f, want 0", i, s)
		}
	}
}

func TestExpandDynamicRange_NoPeakClipping(t *testing.T) {
	// After expansion, peak should not exceed original peak
	n := SampleRate
	samples := make([]float32, n)
	for i := 0; i < n; i++ {
		samples[i] = 0.9 * float32(math.Sin(2*math.Pi*440*float64(i)/float64(SampleRate)))
	}

	var origPeak float32
	for _, s := range samples {
		if abs := float32(math.Abs(float64(s))); abs > origPeak {
			origPeak = abs
		}
	}

	expanded := ExpandDynamicRange(samples)

	var newPeak float32
	for _, s := range expanded {
		if abs := float32(math.Abs(float64(s))); abs > newPeak {
			newPeak = abs
		}
	}

	if newPeak > origPeak*1.01 { // 1% tolerance for float rounding
		t.Errorf("peak increased after expansion: orig=%.4f new=%.4f", origPeak, newPeak)
	}
}

func TestExpandDynamicRange_ShortSignal(t *testing.T) {
	// Should not panic on very short signals
	for _, length := range []int{0, 1, 10, 100} {
		samples := make([]float32, length)
		for i := range samples {
			samples[i] = 0.5
		}
		expanded := ExpandDynamicRange(samples)
		if len(expanded) != length {
			t.Errorf("length %d: got %d samples, want %d", length, len(expanded), length)
		}
	}
}

func TestExpandDynamicRange_MixedSilenceAndSpeech(t *testing.T) {
	// Simulate TTS output: speech, silence gap, speech
	n := SampleRate * 2 // 2 seconds
	samples := make([]float32, n)

	// 0-0.5s: speech
	for i := 0; i < n/4; i++ {
		samples[i] = 0.5 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(SampleRate)))
	}
	// 0.5-1.0s: silence (inserted pause gap)
	// 1.0-2.0s: speech
	for i := n / 2; i < n; i++ {
		samples[i] = 0.5 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(SampleRate)))
	}

	expanded := ExpandDynamicRange(samples)

	// Silence region should still be effectively silent
	silenceRMS := rmsOf(expanded[n/4+1000 : n/2-1000]) // avoid edges
	if silenceRMS > 0.001 {
		t.Errorf("silence region RMS = %.6f, expected ~0", silenceRMS)
	}
}

func rmsOf(samples []float32) float64 {
	var sum float64
	for _, s := range samples {
		v := float64(s)
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(samples)))
}
