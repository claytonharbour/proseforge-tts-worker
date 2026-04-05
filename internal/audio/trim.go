package audio

import "math"

// TrimSilence removes leading and trailing silence from audio samples.
// It uses an RMS-based silence detector matching the Python kokoro-onnx trim_audio()
// algorithm: frame the signal, compute per-frame RMS in dB relative to peak,
// and trim frames below -60 dB from peak.
func TrimSilence(samples []float32) []float32 {
	const (
		frameLength = 2048
		hopLength   = 512
		topDB       = 60.0
	)

	if len(samples) < frameLength {
		return samples
	}

	// Number of complete frames
	numFrames := 1 + (len(samples)-frameLength)/hopLength

	// Compute RMS per frame and track peak
	rms := make([]float64, numFrames)
	maxRMS := 0.0
	for i := 0; i < numFrames; i++ {
		start := i * hopLength
		var sum float64
		for j := start; j < start+frameLength; j++ {
			s := float64(samples[j])
			sum += s * s
		}
		rms[i] = math.Sqrt(sum / frameLength)
		if rms[i] > maxRMS {
			maxRMS = rms[i]
		}
	}

	// All silence — peak RMS is zero
	if maxRMS == 0 {
		return samples[:0]
	}

	// Find first and last non-silent frames (dB > -topDB relative to peak)
	first := -1
	last := -1
	for i := 0; i < numFrames; i++ {
		dB := 20 * math.Log10(rms[i]/maxRMS)
		if dB > -topDB {
			if first == -1 {
				first = i
			}
			last = i
		}
	}

	if first == -1 {
		return samples[:0]
	}

	// Convert frame indices to sample indices (librosa convention)
	startSample := first * hopLength
	endSample := (last + 1) * hopLength
	if endSample > len(samples) {
		endSample = len(samples)
	}

	return samples[startSample:endSample]
}
