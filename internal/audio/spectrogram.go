package audio

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"math/cmplx"
	"os"
)

// SpectrogramConfig controls spectrogram rendering parameters.
type SpectrogramConfig struct {
	FFTSize   int     // default 2048
	HopSize   int     // default 512 (75% overlap)
	MaxFreqHz float64 // default 8000 (speech range)
}

func (c *SpectrogramConfig) defaults() {
	if c.FFTSize == 0 {
		c.FFTSize = 2048
	}
	if c.HopSize == 0 {
		c.HopSize = 512
	}
	if c.MaxFreqHz == 0 {
		c.MaxFreqHz = 8000
	}
}

// RenderSpectrogram generates a spectrogram image from audio samples.
// The image has time on the x-axis and frequency on the y-axis (low at bottom).
// If pitchFrames is non-nil, a cyan pitch contour is overlaid.
func RenderSpectrogram(samples []float32, sampleRate int, cfg SpectrogramConfig, pitchFrames []PitchFrame) *image.RGBA {
	cfg.defaults()

	window := HannWindow(cfg.FFTSize)

	// Number of time frames
	numFrames := 0
	if len(samples) > cfg.FFTSize {
		numFrames = 1 + (len(samples)-cfg.FFTSize)/cfg.HopSize
	}
	if numFrames == 0 {
		numFrames = 1
	}

	// Number of frequency bins up to MaxFreqHz
	maxBin := int(cfg.MaxFreqHz * float64(cfg.FFTSize) / float64(sampleRate))
	if maxBin > cfg.FFTSize/2 {
		maxBin = cfg.FFTSize / 2
	}

	// Compute STFT magnitude matrix [numFrames][maxBin] in dB
	magnitudes := make([][]float64, numFrames)
	globalMax := -math.MaxFloat64

	for f := 0; f < numFrames; f++ {
		offset := f * cfg.HopSize
		fftBuf := make([]complex128, cfg.FFTSize)
		for i := 0; i < cfg.FFTSize; i++ {
			idx := offset + i
			if idx < len(samples) {
				fftBuf[i] = complex(float64(samples[idx])*window[i], 0)
			}
		}
		FFT(fftBuf)

		magnitudes[f] = make([]float64, maxBin)
		for b := 0; b < maxBin; b++ {
			mag := cmplx.Abs(fftBuf[b])
			if mag < 1e-10 {
				mag = 1e-10
			}
			dB := 20 * math.Log10(mag)
			magnitudes[f][b] = dB
			if dB > globalMax {
				globalMax = dB
			}
		}
	}

	// Normalize: shift so max = 0, clamp to [-80, 0]
	for f := range magnitudes {
		for b := range magnitudes[f] {
			magnitudes[f][b] -= globalMax
			if magnitudes[f][b] < -80 {
				magnitudes[f][b] = -80
			}
		}
	}

	// Render image
	imgW := numFrames
	imgH := maxBin
	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

	for x := 0; x < imgW; x++ {
		for y := 0; y < imgH; y++ {
			// y=0 is top of image → highest frequency
			bin := imgH - 1 - y
			dB := magnitudes[x][bin]
			// Map [-80, 0] → [0, 1]
			t := (dB + 80) / 80
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			img.SetRGBA(x, y, hotColor(t))
		}
	}

	// Overlay pitch contour
	if len(pitchFrames) > 0 {
		cyan := color.RGBA{R: 0, G: 255, B: 255, A: 255}
		totalDur := float64(len(samples)) / float64(sampleRate)
		for _, pf := range pitchFrames {
			if !pf.Voiced || pf.F0 <= 0 {
				continue
			}
			// Map time to x pixel
			x := int(pf.TimeSec / totalDur * float64(imgW))
			if x >= imgW {
				x = imgW - 1
			}
			// Map frequency to y pixel (y=0 is top = max freq)
			freqRatio := pf.F0 / cfg.MaxFreqHz
			if freqRatio > 1 {
				continue
			}
			y := imgH - 1 - int(freqRatio*float64(imgH-1))
			if y < 0 {
				y = 0
			}
			if y >= imgH {
				y = imgH - 1
			}
			// Draw a 3-pixel-tall mark for visibility
			for dy := -1; dy <= 1; dy++ {
				py := y + dy
				if py >= 0 && py < imgH {
					img.SetRGBA(x, py, cyan)
				}
			}
		}
	}

	return img
}

// SaveSpectrogram writes an RGBA image to a PNG file.
func SaveSpectrogram(img *image.RGBA, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// hotColor maps a value t in [0,1] to a hot colormap:
// black → purple → red → orange → yellow → white
func hotColor(t float64) color.RGBA {
	var r, g, b float64
	switch {
	case t < 0.2:
		// black → purple
		s := t / 0.2
		r = 0.5 * s
		g = 0
		b = s
	case t < 0.4:
		// purple → red
		s := (t - 0.2) / 0.2
		r = 0.5 + 0.5*s
		g = 0
		b = 1.0 - s
	case t < 0.6:
		// red → orange
		s := (t - 0.4) / 0.2
		r = 1.0
		g = 0.5 * s
		b = 0
	case t < 0.8:
		// orange → yellow
		s := (t - 0.6) / 0.2
		r = 1.0
		g = 0.5 + 0.5*s
		b = 0
	default:
		// yellow → white
		s := (t - 0.8) / 0.2
		r = 1.0
		g = 1.0
		b = s
	}
	return color.RGBA{
		R: uint8(clamp01(r) * 255),
		G: uint8(clamp01(g) * 255),
		B: uint8(clamp01(b) * 255),
		A: 255,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
