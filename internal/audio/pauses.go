package audio

import "math"

// Pause represents a detected silence segment.
type Pause struct {
	StartSec float64
	EndSec   float64
	Duration float64
}

// PauseAnalysis holds the results of silence detection.
type PauseAnalysis struct {
	TotalDuration   float64
	SpeechDuration  float64
	SilenceDuration float64
	SilenceRatio    float64
	PauseCount      int
	Pauses          []Pause
	AvgPause        float64
	MaxPause        float64
}

// DetectPauses finds all silence segments in the audio using RMS-based detection.
// thresholdDB is the silence threshold in negative dB relative to peak (e.g., -40.0).
// minPauseSec is the minimum duration to count as a pause.
func DetectPauses(samples []float32, sampleRate int, thresholdDB float64, minPauseSec float64) *PauseAnalysis {
	const (
		frameLength = 2048
		hopLength   = 512
	)

	totalDuration := float64(len(samples)) / float64(sampleRate)

	if len(samples) < frameLength {
		return &PauseAnalysis{TotalDuration: totalDuration, SpeechDuration: totalDuration}
	}

	numFrames := 1 + (len(samples)-frameLength)/hopLength

	// Compute RMS per frame
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

	if maxRMS == 0 {
		return &PauseAnalysis{
			TotalDuration:   totalDuration,
			SilenceDuration: totalDuration,
			SilenceRatio:    1.0,
			PauseCount:      1,
			Pauses:          []Pause{{StartSec: 0, EndSec: totalDuration, Duration: totalDuration}},
			AvgPause:        totalDuration,
			MaxPause:        totalDuration,
		}
	}

	// Classify each frame as silent or not
	silent := make([]bool, numFrames)
	for i := 0; i < numFrames; i++ {
		dB := 20 * math.Log10(rms[i]/maxRMS)
		silent[i] = dB <= thresholdDB
	}

	// Find contiguous silence regions
	var pauses []Pause
	inSilence := false
	silenceStart := 0
	for i := 0; i < numFrames; i++ {
		if silent[i] && !inSilence {
			silenceStart = i
			inSilence = true
		} else if !silent[i] && inSilence {
			startSec := float64(silenceStart*hopLength) / float64(sampleRate)
			endSec := float64(i*hopLength) / float64(sampleRate)
			dur := endSec - startSec
			if dur >= minPauseSec {
				pauses = append(pauses, Pause{StartSec: startSec, EndSec: endSec, Duration: dur})
			}
			inSilence = false
		}
	}
	// Handle trailing silence
	if inSilence {
		startSec := float64(silenceStart*hopLength) / float64(sampleRate)
		endSec := totalDuration
		dur := endSec - startSec
		if dur >= minPauseSec {
			pauses = append(pauses, Pause{StartSec: startSec, EndSec: endSec, Duration: dur})
		}
	}

	var silenceDuration float64
	var maxPause float64
	for _, p := range pauses {
		silenceDuration += p.Duration
		if p.Duration > maxPause {
			maxPause = p.Duration
		}
	}

	avgPause := 0.0
	if len(pauses) > 0 {
		avgPause = silenceDuration / float64(len(pauses))
	}

	return &PauseAnalysis{
		TotalDuration:   totalDuration,
		SpeechDuration:  totalDuration - silenceDuration,
		SilenceDuration: silenceDuration,
		SilenceRatio:    silenceDuration / totalDuration,
		PauseCount:      len(pauses),
		Pauses:          pauses,
		AvgPause:        avgPause,
		MaxPause:        maxPause,
	}
}
