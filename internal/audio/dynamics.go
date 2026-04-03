package audio

import "math"

// ExpandDynamicRange widens the loudness variation of a waveform.
// Passages louder than the overall average get boosted; quieter passages
// get attenuated. This makes speech sound more expressive without
// changing pitch or timing.
//
// Parameters are chosen for narration TTS at 24kHz:
//   - 300ms RMS window tracks phrase-level energy (not syllables)
//   - ratio 1.3 gives moderate expansion
//   - gain clamped to [0.8, 1.4] to prevent extreme swings
//   - silence regions (below -50 dB) get gain=1.0 (untouched)
func ExpandDynamicRange(samples []float32) []float32 {
	const (
		windowMs       = 300   // RMS averaging window in milliseconds
		ratio          = 1.5   // expansion ratio (1.0 = no change)
		minGain        = 0.7   // minimum gain multiplier
		maxGain        = 1.6   // maximum gain multiplier
		silenceDBFloor = -50.0 // below this, treat as silence (gain=1.0)
		smoothMs       = 100   // gain smoothing window in milliseconds
	)

	n := len(samples)
	if n == 0 {
		return samples
	}

	windowSamples := SampleRate * windowMs / 1000 // 7200 at 24kHz
	if windowSamples > n {
		windowSamples = n
	}
	smoothSamples := SampleRate * smoothMs / 1000 // 2400 at 24kHz

	// Step 1: Compute per-sample RMS envelope using a sliding window.
	// Use running sum-of-squares for O(n) efficiency.
	rmsEnv := computeRMSEnvelope(samples, windowSamples)

	// Step 2: Compute reference level (overall RMS of non-silent regions).
	refRMS := computeReferenceRMS(samples)
	if refRMS < 1e-10 {
		return samples // all silence
	}

	refDB := 20 * math.Log10(refRMS)

	// Step 3: Compute raw gain curve.
	gain := make([]float64, n)
	for i := 0; i < n; i++ {
		localRMS := rmsEnv[i]
		if localRMS < 1e-10 {
			gain[i] = 1.0
			continue
		}

		localDB := 20 * math.Log10(localRMS)
		if localDB < refDB+silenceDBFloor {
			gain[i] = 1.0
			continue
		}

		// gain = (localRMS / refRMS) ^ (ratio - 1)
		g := math.Pow(localRMS/refRMS, ratio-1)
		if g < minGain {
			g = minGain
		} else if g > maxGain {
			g = maxGain
		}
		gain[i] = g
	}

	// Step 3b: Blend gain edges from 1.0 to prevent artifacts from partial
	// RMS windows at the start and end of the waveform.
	halfWindow := windowSamples / 2
	for i := 0; i < halfWindow && i < n; i++ {
		t := float64(i) / float64(halfWindow)
		gain[i] = 1.0 + (gain[i]-1.0)*t
	}
	for i := n - halfWindow; i < n; i++ {
		if i < 0 {
			continue
		}
		t := float64(n-1-i) / float64(halfWindow)
		gain[i] = 1.0 + (gain[i]-1.0)*t
	}

	// Step 4: Smooth the gain curve to prevent abrupt changes.
	smoothGain(gain, smoothSamples)

	// Step 5: Apply gain and find peak for normalization.
	out := make([]float32, n)
	var peak float64
	for i := 0; i < n; i++ {
		v := float64(samples[i]) * gain[i]
		out[i] = float32(v)
		if abs := math.Abs(v); abs > peak {
			peak = abs
		}
	}

	// Step 6: Normalize to prevent clipping (preserve original peak level).
	var origPeak float64
	for _, s := range samples {
		if abs := math.Abs(float64(s)); abs > origPeak {
			origPeak = abs
		}
	}
	if peak > 0 && origPeak > 0 {
		scale := float32(origPeak / peak)
		for i := range out {
			out[i] *= scale
		}
	}

	return out
}

// computeRMSEnvelope computes a per-sample RMS envelope using a centered
// sliding window with running sum-of-squares.
func computeRMSEnvelope(samples []float32, windowSamples int) []float64 {
	n := len(samples)
	rms := make([]float64, n)

	half := windowSamples / 2

	// Initial sum for the first window position
	var sumSq float64
	end := half
	if end > n {
		end = n
	}
	for i := 0; i < end; i++ {
		s := float64(samples[i])
		sumSq += s * s
	}
	count := end

	for i := 0; i < n; i++ {
		// Add sample entering the right side of the window
		addIdx := i + half
		if addIdx < n {
			s := float64(samples[addIdx])
			sumSq += s * s
			count++
		}
		// Remove sample leaving the left side of the window
		removeIdx := i - half - 1
		if removeIdx >= 0 {
			s := float64(samples[removeIdx])
			sumSq -= s * s
			count--
		}
		// Clamp floating point drift
		if sumSq < 0 {
			sumSq = 0
		}
		if count > 0 {
			rms[i] = math.Sqrt(sumSq / float64(count))
		}
	}

	return rms
}

// computeReferenceRMS computes the RMS of all non-silent samples.
func computeReferenceRMS(samples []float32) float64 {
	var sumSq float64
	var count int
	for _, s := range samples {
		v := float64(s)
		if math.Abs(v) > 1e-8 {
			sumSq += v * v
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return math.Sqrt(sumSq / float64(count))
}

// smoothGain applies a simple moving average to the gain curve.
func smoothGain(gain []float64, windowSamples int) {
	if windowSamples <= 1 || len(gain) <= windowSamples {
		return
	}

	n := len(gain)
	smoothed := make([]float64, n)
	half := windowSamples / 2

	var sum float64
	var count int

	// Initialize window
	end := half
	if end > n {
		end = n
	}
	for i := 0; i < end; i++ {
		sum += gain[i]
		count++
	}

	for i := 0; i < n; i++ {
		addIdx := i + half
		if addIdx < n {
			sum += gain[addIdx]
			count++
		}
		removeIdx := i - half - 1
		if removeIdx >= 0 {
			sum -= gain[removeIdx]
			count--
		}
		if count > 0 {
			smoothed[i] = sum / float64(count)
		} else {
			smoothed[i] = 1.0
		}
	}

	copy(gain, smoothed)
}
