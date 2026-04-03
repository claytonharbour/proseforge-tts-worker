package audio

import "math"

// Glitch represents a detected audio anomaly — a sudden energy discontinuity
// that sounds like a click, pop, or jitter.
type Glitch struct {
	TimeSec  float64 // position in seconds
	Sample   int     // sample index
	Severity float64 // ratio of energy change (higher = more severe)
	Type     string  // "spike", "dropout", or "dc_jump"
}

// DetectGlitches scans audio for sudden energy discontinuities.
// Returns timestamps where clicks, pops, or jitter are detected.
func DetectGlitches(samples []float32, sampleRate int) []Glitch {
	const (
		frameSize        = 128   // ~5ms at 24kHz — short enough to catch transients
		hopSize          = 64    // ~2.7ms hop for fine resolution
		spikeRatio       = 4.0   // frame-to-frame energy ratio threshold
		quietSpeechFloor = 0.005 // RMS below this is too quiet for a spike to be audible
		dropDB           = -20   // dB drop threshold for dropout detection
		dcWindow         = 512   // ~21ms for DC offset detection
		dcThresh         = 0.08  // DC offset jump threshold
	)

	n := len(samples)
	if n < frameSize*2 {
		return nil
	}

	// Compute per-frame RMS
	numFrames := (n - frameSize) / hopSize
	rms := make([]float64, numFrames)
	for i := 0; i < numFrames; i++ {
		start := i * hopSize
		var sum float64
		for j := start; j < start+frameSize; j++ {
			s := float64(samples[j])
			sum += s * s
		}
		rms[i] = math.Sqrt(sum / float64(frameSize))
	}

	var glitches []Glitch

	// Detect energy spikes and dropouts
	for i := 1; i < numFrames; i++ {
		prev := rms[i-1]
		curr := rms[i]
		sampleIdx := i * hopSize

		// Skip silent regions
		if prev < 1e-6 && curr < 1e-6 {
			continue
		}

		// Spike: sudden energy increase
		if prev > 1e-6 && curr/prev > spikeRatio {
			// Check if this is a silence→speech onset (not a real glitch).
			// If several preceding frames are near-silent, the spike is just
			// normal speech resuming after an intentional silence gap.
			if isSilenceOnset(rms, i, 4, 0.001) {
				continue
			}
			// Check if preceding frame is just very quiet speech (not silence).
			// Natural plosive onsets after quiet passages produce high ratios
			// (60-80x) but aren't audible artifacts — the preceding energy is
			// too low for the transition to sound like a pop or click.
			if prev < quietSpeechFloor {
				continue
			}
			glitches = append(glitches, Glitch{
				TimeSec:  float64(sampleIdx) / float64(sampleRate),
				Sample:   sampleIdx,
				Severity: curr / prev,
				Type:     "spike",
			})
		}

		// Dropout: sudden energy drop then recovery
		if curr > 1e-6 && prev > 1e-6 {
			prevDB := 20 * math.Log10(curr/prev)
			if prevDB < dropDB && i+1 < numFrames && rms[i+1] > curr*2 {
				glitches = append(glitches, Glitch{
					TimeSec:  float64(sampleIdx) / float64(sampleRate),
					Sample:   sampleIdx,
					Severity: -prevDB,
					Type:     "dropout",
				})
			}
		}
	}

	// Detect DC offset jumps (sample-level discontinuities)
	for i := dcWindow; i < n-dcWindow; i += dcWindow / 2 {
		// Average before and after
		var sumBefore, sumAfter float64
		for j := i - dcWindow; j < i; j++ {
			sumBefore += float64(samples[j])
		}
		for j := i; j < i+dcWindow; j++ {
			sumAfter += float64(samples[j])
		}
		avgBefore := sumBefore / float64(dcWindow)
		avgAfter := sumAfter / float64(dcWindow)

		jump := math.Abs(avgAfter - avgBefore)
		if jump > dcThresh {
			glitches = append(glitches, Glitch{
				TimeSec:  float64(i) / float64(sampleRate),
				Sample:   i,
				Severity: jump / dcThresh,
				Type:     "dc_jump",
			})
		}
	}

	return glitches
}

// isSilenceOnset checks whether frame i is a normal speech onset after silence.
// Returns true if at least minSilentFrames of the preceding frames have RMS below threshold.
func isSilenceOnset(rms []float64, i, minSilentFrames int, threshold float64) bool {
	silentCount := 0
	for j := i - 1; j >= 0 && j >= i-minSilentFrames-2; j-- {
		if rms[j] < threshold {
			silentCount++
		}
	}
	return silentCount >= minSilentFrames
}
