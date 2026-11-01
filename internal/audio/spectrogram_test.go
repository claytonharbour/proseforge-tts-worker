//go:build unit

package audio

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderSpectrogram_Sine440(t *testing.T) {
	const (
		sampleRate = 24000
		duration   = 0.5
		freq       = 440.0
	)
	n := int(duration * sampleRate)
	samples := make([]float32, n)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * freq * float64(i) / sampleRate))
	}

	cfg := SpectrogramConfig{FFTSize: 2048, HopSize: 512, MaxFreqHz: 8000}
	img := RenderSpectrogram(samples, sampleRate, cfg, nil)

	// Check image dimensions
	expectedW := 1 + (n-cfg.FFTSize)/cfg.HopSize
	expectedH := int(cfg.MaxFreqHz * float64(cfg.FFTSize) / float64(sampleRate))
	if img.Bounds().Dx() != expectedW {
		t.Errorf("width = %d, want %d", img.Bounds().Dx(), expectedW)
	}
	if img.Bounds().Dy() != expectedH {
		t.Errorf("height = %d, want %d", img.Bounds().Dy(), expectedH)
	}
}

func TestRenderSpectrogram_WritePNG(t *testing.T) {
	const sampleRate = 24000
	// Generate a chirp: frequency sweeps from 200 to 2000 Hz over 1 second
	n := sampleRate
	samples := make([]float32, n)
	for i := range samples {
		frac := float64(i) / float64(n)
		freq := 200.0 + 1800.0*frac
		phase := 2 * math.Pi * freq * float64(i) / float64(sampleRate)
		samples[i] = float32(math.Sin(phase))
	}

	cfg := SpectrogramConfig{}
	img := RenderSpectrogram(samples, sampleRate, cfg, nil)

	// Write to temp file and verify it's a valid PNG
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	if err := SaveSpectrogram(img, path); err != nil {
		t.Fatalf("SaveSpectrogram: %v", err)
	}

	// Decode it back
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	decoded, err := png.Decode(f)
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	if decoded.Bounds().Dx() != img.Bounds().Dx() || decoded.Bounds().Dy() != img.Bounds().Dy() {
		t.Errorf("decoded size %v != original %v", decoded.Bounds(), img.Bounds())
	}
}

func TestRenderSpectrogram_WithPitchOverlay(t *testing.T) {
	const sampleRate = 24000
	samples := make([]float32, sampleRate) // 1 second
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 300 * float64(i) / sampleRate))
	}

	pitchFrames := []PitchFrame{
		{TimeSec: 0.1, F0: 300, Voiced: true},
		{TimeSec: 0.5, F0: 300, Voiced: true},
		{TimeSec: 0.9, F0: 300, Voiced: true},
	}

	cfg := SpectrogramConfig{}
	img := RenderSpectrogram(samples, sampleRate, cfg, pitchFrames)

	if img == nil {
		t.Fatal("RenderSpectrogram returned nil")
	}
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Error("image has zero dimensions")
	}
}
