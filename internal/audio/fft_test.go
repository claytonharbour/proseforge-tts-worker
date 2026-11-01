//go:build unit

package audio

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestFFT_SinePeak(t *testing.T) {
	// 1024-point FFT of 440 Hz sine at 24 kHz sample rate.
	// Expected peak bin: 440 * 1024 / 24000 ≈ 18.77 → bin 18 or 19.
	const (
		n          = 1024
		sampleRate = 24000
		freq       = 440.0
	)

	x := make([]complex128, n)
	for i := range x {
		x[i] = complex(math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate)), 0)
	}

	result := FFT(x)

	// Find peak bin in first half
	peakBin := 0
	peakMag := 0.0
	for i := 1; i < n/2; i++ {
		mag := cmplx.Abs(result[i])
		if mag > peakMag {
			peakMag = mag
			peakBin = i
		}
	}

	expectedBin := float64(freq) * float64(n) / float64(sampleRate)
	if math.Abs(float64(peakBin)-expectedBin) > 1.5 {
		t.Errorf("peak at bin %d, expected ~%.1f", peakBin, expectedBin)
	}
}

func TestFFT_ZeroInput(t *testing.T) {
	x := make([]complex128, 64)
	result := FFT(x)
	for i, v := range result {
		if cmplx.Abs(v) > 1e-10 {
			t.Errorf("bin %d = %v, want 0", i, v)
		}
	}
}

func TestFFT_SingleSample(t *testing.T) {
	x := []complex128{complex(42, 0)}
	result := FFT(x)
	if cmplx.Abs(result[0]-complex(42, 0)) > 1e-10 {
		t.Errorf("single sample FFT = %v, want 42+0i", result[0])
	}
}

func TestHannWindow(t *testing.T) {
	w := HannWindow(8)
	if len(w) != 8 {
		t.Fatalf("len = %d, want 8", len(w))
	}
	// Hann window starts and ends at 0, peaks at center
	if w[0] > 1e-10 {
		t.Errorf("w[0] = %f, want ~0", w[0])
	}
	if w[len(w)-1] > 1e-10 {
		t.Errorf("w[last] = %f, want ~0", w[len(w)-1])
	}
	// Middle values should be close to 1
	mid := len(w) / 2
	if w[mid] < 0.9 {
		t.Errorf("w[mid] = %f, want close to 1.0", w[mid])
	}
}
