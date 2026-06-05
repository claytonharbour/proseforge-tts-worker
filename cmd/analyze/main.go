package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/claytonharbour/proseforge-tts-worker/internal/audio"
)

func main() {
	refPath := flag.String("ref", "", "Reference WAV file (e.g., Gemini output)")
	testPath := flag.String("test", "", "Test WAV file (e.g., Kokoro output)")
	thresholdDB := flag.Float64("threshold", -40.0, "Silence threshold in dB (relative to peak)")
	minPause := flag.Float64("min-pause", 0.1, "Minimum pause duration in seconds")
	spectrogramDir := flag.String("spectrogram", "", "Output directory for spectrogram PNGs (creates experiment folder)")
	transcribe := flag.Bool("transcribe", false, "Transcribe WAV files using whisper-cpp")
	whisperModel := flag.String("whisper-model", "models/ggml-base.en.bin", "Path to Whisper GGML model file")
	referenceText := flag.String("reference-text", "", "Reference text file for WER computation (implies -transcribe)")
	flag.Parse()

	// -reference-text implies -transcribe
	if *referenceText != "" {
		*transcribe = true
	}

	if *refPath == "" && *testPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: analyze -ref <file.wav> [-test <file.wav>] [-spectrogram <dir>]\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Tee all output to a buffer so we can save to analysis.txt
	var outputBuf bytes.Buffer
	out := io.MultiWriter(os.Stdout, &outputBuf)

	type wavData struct {
		wav      *audio.WAVFile
		pause    *audio.PauseAnalysis
		pitch    *audio.PitchStats
		glitches []audio.Glitch
	}

	var ref, test *wavData

	if *refPath != "" {
		w, err := audio.ReadWAVFile(*refPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading ref: %v\n", err)
			os.Exit(1)
		}
		ref = &wavData{
			wav:      w,
			pause:    audio.DetectPauses(w.Samples, w.SampleRate, *thresholdDB, *minPause),
			pitch:    audio.ExtractPitch(w.Samples, w.SampleRate),
			glitches: audio.DetectGlitches(w.Samples, w.SampleRate),
		}
		fmt.Fprintf(out, "Reference: %s\n", *refPath)
		fprintAnalysis(out, ref.pause)
		fprintPitch(out, ref.pitch)
		fprintGlitches(out, "Ref", ref.glitches)
	}

	if *testPath != "" {
		w, err := audio.ReadWAVFile(*testPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading test: %v\n", err)
			os.Exit(1)
		}
		test = &wavData{
			wav:      w,
			pause:    audio.DetectPauses(w.Samples, w.SampleRate, *thresholdDB, *minPause),
			pitch:    audio.ExtractPitch(w.Samples, w.SampleRate),
			glitches: audio.DetectGlitches(w.Samples, w.SampleRate),
		}
		fmt.Fprintf(out, "\nTest: %s\n", *testPath)
		fprintAnalysis(out, test.pause)
		fprintPitch(out, test.pitch)
		fprintGlitches(out, "Test", test.glitches)
	}

	if ref != nil && test != nil {
		fmt.Fprintf(out, "\n--- Pause Comparison ---\n")
		fmt.Fprintf(out, "%-24s %10s %10s %10s\n", "Metric", "Ref", "Test", "Diff")
		fmt.Fprintf(out, "%-24s %10s %10s %10s\n", "------", "---", "----", "----")
		fprintRow(out, "Total Duration (s)", ref.pause.TotalDuration, test.pause.TotalDuration)
		fprintRow(out, "Speech Duration (s)", ref.pause.SpeechDuration, test.pause.SpeechDuration)
		fprintRow(out, "Silence Duration (s)", ref.pause.SilenceDuration, test.pause.SilenceDuration)
		fprintRow(out, "Silence Ratio", ref.pause.SilenceRatio, test.pause.SilenceRatio)
		fprintRowInt(out, "Pause Count", ref.pause.PauseCount, test.pause.PauseCount)
		fprintRow(out, "Avg Pause (s)", ref.pause.AvgPause, test.pause.AvgPause)
		fprintRow(out, "Max Pause (s)", ref.pause.MaxPause, test.pause.MaxPause)

		fmt.Fprintf(out, "\n--- Pause Duration Histogram ---\n")
		fprintHistogram(out, "Ref", ref.pause.Pauses)
		fprintHistogram(out, "Test", test.pause.Pauses)

		fmt.Fprintf(out, "\n--- Pitch Comparison ---\n")
		fmt.Fprintf(out, "%-24s %10s %10s %10s\n", "Metric", "Ref", "Test", "Diff")
		fmt.Fprintf(out, "%-24s %10s %10s %10s\n", "------", "---", "----", "----")
		fprintRow(out, "Mean F0 (Hz)", ref.pitch.MeanF0, test.pitch.MeanF0)
		fprintRow(out, "Median F0 (Hz)", ref.pitch.MedianF0, test.pitch.MedianF0)
		fprintRow(out, "StdDev F0 (Hz)", ref.pitch.StdDevF0, test.pitch.StdDevF0)
		fprintRow(out, "Min F0 (Hz)", ref.pitch.MinF0, test.pitch.MinF0)
		fprintRow(out, "Max F0 (Hz)", ref.pitch.MaxF0, test.pitch.MaxF0)
		fprintRow(out, "Voiced (%)", ref.pitch.VoicedPct, test.pitch.VoicedPct)
		fprintRow(out, "F0 Range (Hz)", ref.pitch.MaxF0-ref.pitch.MinF0, test.pitch.MaxF0-test.pitch.MinF0)
	}

	// Create output directory early so transcription can write files too.
	if *spectrogramDir != "" {
		if err := os.MkdirAll(*spectrogramDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output dir: %v\n", err)
			os.Exit(1)
		}
	}

	// Transcribe WAV files if requested
	if *transcribe {
		fmt.Fprintf(out, "\n--- Transcription ---\n")
		if ref != nil {
			refWAV, err := os.ReadFile(*refPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading ref WAV for transcription: %v\n", err)
				os.Exit(1)
			}
			text, err := audio.Transcribe(refWAV, *whisperModel)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error transcribing ref: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(out, "Ref:  %s\n", text)
			if *spectrogramDir != "" {
				os.WriteFile(filepath.Join(*spectrogramDir, "ref-transcript.txt"), []byte(text), 0644)
			}
		}
		if test != nil {
			testWAV, err := os.ReadFile(*testPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading test WAV for transcription: %v\n", err)
				os.Exit(1)
			}
			text, err := audio.Transcribe(testWAV, *whisperModel)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error transcribing test: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(out, "Test: %s\n", text)
			if *spectrogramDir != "" {
				os.WriteFile(filepath.Join(*spectrogramDir, "test-transcript.txt"), []byte(text), 0644)
			}
		}
	}

	// Compute WER if reference text provided
	if *referenceText != "" && *testPath != "" {
		refTextData, err := os.ReadFile(*referenceText)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading reference text: %v\n", err)
			os.Exit(1)
		}
		refText := string(refTextData)

		testWAVForWER, err := os.ReadFile(*testPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading test WAV for WER: %v\n", err)
			os.Exit(1)
		}
		hypText, err := audio.Transcribe(testWAVForWER, *whisperModel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error transcribing for WER: %v\n", err)
			os.Exit(1)
		}

		result := audio.ComputeWER(refText, hypText)
		fmt.Fprintf(out, "\n--- Word Error Rate ---\n")
		fmt.Fprintf(out, "  Reference words: %d\n", result.TotalRef)
		fmt.Fprintf(out, "  Hypothesis words: %d\n", result.TotalHyp)
		fmt.Fprintf(out, "  WER: %.1f%%\n", result.WER*100)
		fmt.Fprintf(out, "  Substitutions: %d  Insertions: %d  Deletions: %d\n",
			result.Substitutions, result.Insertions, result.Deletions)

		if len(result.Errors) > 0 {
			fmt.Fprintf(out, "\n  Errors:\n")
			maxErrors := 30
			for i, e := range result.Errors {
				if i >= maxErrors {
					fmt.Fprintf(out, "  ... and %d more\n", len(result.Errors)-maxErrors)
					break
				}
				switch e.Type {
				case "sub":
					fmt.Fprintf(out, "    [SUB] \"%s\" → \"%s\" (pos %d)\n", e.RefWord, e.HypWord, e.RefPos)
				case "del":
					fmt.Fprintf(out, "    [DEL] \"%s\" (pos %d)\n", e.RefWord, e.RefPos)
				case "ins":
					fmt.Fprintf(out, "    [INS] \"%s\"\n", e.HypWord)
				}
			}
		}

		if *spectrogramDir != "" {
			os.WriteFile(filepath.Join(*spectrogramDir, "wer-transcript.txt"), []byte(hypText), 0644)
		}
	}

	// Generate spectrograms if requested
	if *spectrogramDir != "" {
		cfg := audio.SpectrogramConfig{FFTSize: 2048, HopSize: 512, MaxFreqHz: 8000}

		if ref != nil {
			pngPath := filepath.Join(*spectrogramDir, "ref-spectrogram.png")
			img := audio.RenderSpectrogram(ref.wav.Samples, ref.wav.SampleRate, cfg, ref.pitch.Frames)
			if err := audio.SaveSpectrogram(img, pngPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving ref spectrogram: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(out, "\nRef spectrogram: %s\n", pngPath)
		}

		if test != nil {
			pngPath := filepath.Join(*spectrogramDir, "test-spectrogram.png")
			img := audio.RenderSpectrogram(test.wav.Samples, test.wav.SampleRate, cfg, test.pitch.Frames)
			if err := audio.SaveSpectrogram(img, pngPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving test spectrogram: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(out, "\nTest spectrogram: %s\n", pngPath)
		}

		// Save analysis text
		analysisPath := filepath.Join(*spectrogramDir, "analysis.txt")
		if err := os.WriteFile(analysisPath, outputBuf.Bytes(), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving analysis: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Analysis saved: %s\n", analysisPath)
	}
}

func fprintAnalysis(w io.Writer, a *audio.PauseAnalysis) {
	fmt.Fprintf(w, "  Total Duration:   %.2fs\n", a.TotalDuration)
	fmt.Fprintf(w, "  Speech Duration:  %.2fs\n", a.SpeechDuration)
	fmt.Fprintf(w, "  Silence Duration: %.2fs\n", a.SilenceDuration)
	fmt.Fprintf(w, "  Silence Ratio:    %.1f%%\n", a.SilenceRatio*100)
	fmt.Fprintf(w, "  Pause Count:      %d\n", a.PauseCount)
	fmt.Fprintf(w, "  Avg Pause:        %.3fs\n", a.AvgPause)
	fmt.Fprintf(w, "  Max Pause:        %.3fs\n", a.MaxPause)
}

func fprintRow(w io.Writer, label string, ref, test float64) {
	diff := test - ref
	sign := "+"
	if diff < 0 {
		sign = ""
	}
	fmt.Fprintf(w, "%-24s %10.3f %10.3f %9s%.3f\n", label, ref, test, sign, diff)
}

func fprintRowInt(w io.Writer, label string, ref, test int) {
	diff := test - ref
	sign := "+"
	if diff < 0 {
		sign = ""
	}
	fmt.Fprintf(w, "%-24s %10d %10d %9s%d\n", label, ref, test, sign, diff)
}

func fprintPitch(w io.Writer, p *audio.PitchStats) {
	fmt.Fprintf(w, "  Mean F0:          %.1f Hz\n", p.MeanF0)
	fmt.Fprintf(w, "  Median F0:        %.1f Hz\n", p.MedianF0)
	fmt.Fprintf(w, "  StdDev F0:        %.1f Hz\n", p.StdDevF0)
	fmt.Fprintf(w, "  F0 Range:         %.0f-%.0f Hz\n", p.MinF0, p.MaxF0)
	fmt.Fprintf(w, "  Voiced:           %.1f%%\n", p.VoicedPct)
}

func fprintGlitches(w io.Writer, label string, glitches []audio.Glitch) {
	if len(glitches) == 0 {
		fmt.Fprintf(w, "  Glitches:         none\n")
		return
	}

	// Only show glitches above severity threshold that indicate real artifacts.
	// Natural speech regularly produces 4-30x energy jumps at plosives/onsets.
	const severeThreshold = 35.0

	var severe []audio.Glitch
	for _, g := range glitches {
		if g.Severity >= severeThreshold {
			severe = append(severe, g)
		}
	}

	fmt.Fprintf(w, "  Glitches:         %d total, %d severe (>%.0fx)\n",
		len(glitches), len(severe), severeThreshold)
	for _, g := range severe {
		min := int(g.TimeSec) / 60
		sec := g.TimeSec - float64(min*60)
		fmt.Fprintf(w, "    [%s] %02d:%05.2f (sample %d) severity=%.1f type=%s\n",
			label, min, sec, g.Sample, g.Severity, g.Type)
	}
}

func fprintHistogram(w io.Writer, label string, pauses []audio.Pause) {
	buckets := [4]int{}
	for _, p := range pauses {
		switch {
		case p.Duration < 0.2:
			buckets[0]++
		case p.Duration < 0.5:
			buckets[1]++
		case p.Duration < 1.0:
			buckets[2]++
		default:
			buckets[3]++
		}
	}
	fmt.Fprintf(w, "  %s: <0.2s=%d  0.2-0.5s=%d  0.5-1.0s=%d  >1.0s=%d\n",
		label, buckets[0], buckets[1], buckets[2], buckets[3])
}
