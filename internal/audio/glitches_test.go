//go:build unit

package audio

import (
	"math"
	"testing"
)

func TestDetectGlitches_Spike(t *testing.T) {
	// Create a steady signal with a sudden spike
	n := SampleRate // 1 second at 24kHz
	samples := make([]float32, n)

	// Quiet sine wave
	for i := range samples {
		samples[i] = 0.05 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(SampleRate)))
	}

	// Insert a spike at 0.5s
	spikeStart := n / 2
	for i := spikeStart; i < spikeStart+128 && i < n; i++ {
		samples[i] = 0.8 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(SampleRate)))
	}

	glitches := DetectGlitches(samples, SampleRate)

	found := false
	for _, g := range glitches {
		if g.Type == "spike" && g.TimeSec > 0.4 && g.TimeSec < 0.6 {
			found = true
			t.Logf("Detected spike at %.3fs, severity=%.1f", g.TimeSec, g.Severity)
		}
	}
	if !found {
		t.Error("expected to detect a spike near 0.5s")
	}
}

func TestDetectGlitches_Dropout(t *testing.T) {
	// Create a steady signal with a brief dropout then recovery
	n := SampleRate
	samples := make([]float32, n)

	for i := range samples {
		samples[i] = 0.5 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(SampleRate)))
	}

	// Insert a brief dropout at 0.5s — exactly 128 samples (one analysis frame).
	// The detector needs one fully-quiet frame with recovery in the next frame.
	dropStart := (n / 2 / 64) * 64 // align to hop boundary
	for i := dropStart; i < dropStart+128 && i < n; i++ {
		samples[i] *= 0.005
	}

	glitches := DetectGlitches(samples, SampleRate)

	found := false
	for _, g := range glitches {
		if g.Type == "dropout" && g.TimeSec > 0.4 && g.TimeSec < 0.6 {
			found = true
			t.Logf("Detected dropout at %.3fs, severity=%.1f", g.TimeSec, g.Severity)
		}
	}
	if !found {
		// Dropout detection requires drop+recovery, log what we got
		for _, g := range glitches {
			t.Logf("  got: %s at %.3fs severity=%.1f", g.Type, g.TimeSec, g.Severity)
		}
		t.Error("expected to detect a dropout near 0.5s")
	}
}

func TestDetectGlitches_DCJump(t *testing.T) {
	// Create a signal with a DC offset jump
	n := SampleRate
	samples := make([]float32, n)

	// First half: centered at 0
	for i := 0; i < n/2; i++ {
		samples[i] = 0.01 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(SampleRate)))
	}
	// Second half: centered at 0.15 (DC offset jump)
	for i := n / 2; i < n; i++ {
		samples[i] = 0.15 + 0.01*float32(math.Sin(2*math.Pi*200*float64(i)/float64(SampleRate)))
	}

	glitches := DetectGlitches(samples, SampleRate)

	found := false
	for _, g := range glitches {
		if g.Type == "dc_jump" && g.TimeSec > 0.3 && g.TimeSec < 0.7 {
			found = true
			t.Logf("Detected DC jump at %.3fs, severity=%.1f", g.TimeSec, g.Severity)
		}
	}
	if !found {
		for _, g := range glitches {
			t.Logf("  got: %s at %.3fs severity=%.1f", g.Type, g.TimeSec, g.Severity)
		}
		t.Error("expected to detect a DC jump near 0.5s")
	}
}

func TestDetectGlitches_CleanSignal(t *testing.T) {
	// A clean sine wave should produce no glitches
	n := SampleRate
	samples := make([]float32, n)
	for i := range samples {
		samples[i] = 0.5 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(SampleRate)))
	}

	glitches := DetectGlitches(samples, SampleRate)
	if len(glitches) > 0 {
		for _, g := range glitches {
			t.Logf("unexpected: %s at %.3fs severity=%.1f", g.Type, g.TimeSec, g.Severity)
		}
		t.Errorf("expected no glitches on clean signal, got %d", len(glitches))
	}
}

func TestDetectGlitches_Silence(t *testing.T) {
	// All-silence should produce no glitches
	samples := make([]float32, SampleRate)
	glitches := DetectGlitches(samples, SampleRate)
	if len(glitches) > 0 {
		t.Errorf("expected no glitches on silence, got %d", len(glitches))
	}
}

func TestDetectGlitches_TooShort(t *testing.T) {
	// Very short signals should return nil, not panic
	samples := make([]float32, 100)
	glitches := DetectGlitches(samples, SampleRate)
	if glitches != nil {
		t.Errorf("expected nil for short signal, got %d glitches", len(glitches))
	}
}
