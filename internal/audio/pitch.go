package audio

import "math"

// PitchFrame holds pitch information for a single analysis frame.
type PitchFrame struct {
	TimeSec float64
	F0      float64 // fundamental frequency in Hz, 0 if unvoiced
	Voiced  bool
}

// PitchStats holds summary statistics for a pitch contour.
type PitchStats struct {
	MeanF0   float64
	MedianF0 float64
	StdDevF0 float64
	MinF0    float64
	MaxF0    float64
	VoicedPct float64 // percentage of frames that are voiced
	Frames   []PitchFrame
}

// ExtractPitch computes the F0 contour using the YIN algorithm.
// Returns one PitchFrame per hop (10ms).
func ExtractPitch(samples []float32, sampleRate int) *PitchStats {
	const (
		hopMs      = 10    // analysis hop in milliseconds
		windowMs   = 50    // analysis window in milliseconds
		minF0      = 60    // Hz — low end of speech range
		maxF0      = 500   // Hz — high end of speech range
		yinThresh  = 0.15  // YIN absolute threshold (lower = stricter)
	)

	hopSize := sampleRate * hopMs / 1000
	windowSize := sampleRate * windowMs / 1000
	minLag := sampleRate / maxF0
	maxLag := sampleRate / minF0

	if maxLag >= windowSize/2 {
		maxLag = windowSize/2 - 1
	}

	numFrames := 0
	if len(samples) > windowSize {
		numFrames = 1 + (len(samples)-windowSize)/hopSize
	}

	frames := make([]PitchFrame, numFrames)

	for i := 0; i < numFrames; i++ {
		offset := i * hopSize
		frame := samples[offset : offset+windowSize]
		f0 := yin(frame, sampleRate, minLag, maxLag, yinThresh)
		frames[i] = PitchFrame{
			TimeSec: float64(offset) / float64(sampleRate),
			F0:      f0,
			Voiced:  f0 > 0,
		}
	}

	return computePitchStats(frames)
}

// yin runs the YIN algorithm on a single frame.
// Returns F0 in Hz, or 0 if unvoiced.
func yin(frame []float32, sampleRate, minLag, maxLag int, threshold float64) float64 {
	n := len(frame) / 2
	if maxLag >= n {
		maxLag = n - 1
	}
	if minLag >= maxLag {
		return 0
	}

	// Step 1: Difference function
	d := make([]float64, maxLag+1)
	for tau := 1; tau <= maxLag; tau++ {
		for j := 0; j < n; j++ {
			diff := float64(frame[j] - frame[j+tau])
			d[tau] += diff * diff
		}
	}

	// Step 2: Cumulative mean normalized difference function
	dPrime := make([]float64, maxLag+1)
	dPrime[0] = 1.0
	runningSum := 0.0
	for tau := 1; tau <= maxLag; tau++ {
		runningSum += d[tau]
		if runningSum < 1e-10 {
			dPrime[tau] = 1.0
		} else {
			dPrime[tau] = d[tau] * float64(tau) / runningSum
		}
	}

	// Step 3: Absolute threshold — find first minimum below threshold
	for tau := minLag; tau <= maxLag-1; tau++ {
		if dPrime[tau] < threshold {
			// Check it's a local minimum (or the first below threshold)
			if dPrime[tau] <= dPrime[tau+1] {
				// Step 4: Parabolic interpolation
				refined := parabolicInterp(dPrime, tau, maxLag)
				if refined > 0 {
					return float64(sampleRate) / refined
				}
				return float64(sampleRate) / float64(tau)
			}
		}
	}

	return 0 // unvoiced
}

// parabolicInterp refines the lag estimate using parabolic interpolation.
func parabolicInterp(d []float64, tau, maxLag int) float64 {
	if tau < 1 || tau >= maxLag {
		return float64(tau)
	}
	a := d[tau-1]
	b := d[tau]
	c := d[tau+1]

	denom := 2.0 * (a - 2.0*b + c)
	if math.Abs(denom) < 1e-10 {
		return float64(tau)
	}
	shift := (a - c) / denom
	return float64(tau) + shift
}

func computePitchStats(frames []PitchFrame) *PitchStats {
	stats := &PitchStats{Frames: frames}
	if len(frames) == 0 {
		return stats
	}

	// Collect voiced F0 values
	var voiced []float64
	for _, f := range frames {
		if f.Voiced {
			voiced = append(voiced, f.F0)
		}
	}

	stats.VoicedPct = 100.0 * float64(len(voiced)) / float64(len(frames))

	if len(voiced) == 0 {
		return stats
	}

	// Mean
	sum := 0.0
	for _, v := range voiced {
		sum += v
	}
	stats.MeanF0 = sum / float64(len(voiced))

	// Min/Max
	stats.MinF0 = voiced[0]
	stats.MaxF0 = voiced[0]
	for _, v := range voiced {
		if v < stats.MinF0 {
			stats.MinF0 = v
		}
		if v > stats.MaxF0 {
			stats.MaxF0 = v
		}
	}

	// Std dev
	var sqDiffSum float64
	for _, v := range voiced {
		diff := v - stats.MeanF0
		sqDiffSum += diff * diff
	}
	stats.StdDevF0 = math.Sqrt(sqDiffSum / float64(len(voiced)))

	// Median (simple sort — frame counts are small)
	sorted := make([]float64, len(voiced))
	copy(sorted, voiced)
	// Insertion sort — good enough for ~5000 frames
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		stats.MedianF0 = (sorted[mid-1] + sorted[mid]) / 2
	} else {
		stats.MedianF0 = sorted[mid]
	}

	return stats
}
