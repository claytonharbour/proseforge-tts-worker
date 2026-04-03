package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/claytonharbour/proseforge-tts-worker/internal/audio"
)

// Story matches the ProseForge JSON export format.
type Story struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Author   string    `json:"author"`
	Sections []Section `json:"sections"`
}

// Section is a single chapter/section of a story.
type Section struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
	Name     string `json:"name"`
	Content  string `json:"content"`
}

func main() {
	storyPath := flag.String("story", "", "Path to story JSON file")
	serverURL := flag.String("server", "http://localhost:8099", "TTS server URL")
	voice := flag.String("voice", "af_sarah", "Voice name")
	speed := flag.Float64("speed", 0.93, "Speech speed")
	outputDir := flag.String("output", "", "Output directory (default: build/stories/<slug>/)")
	sections := flag.String("sections", "", "Comma-separated section positions to generate (default: all)")
	analyze := flag.Bool("analyze", true, "Run audio analysis on generated files")
	refAudio := flag.String("ref", "", "Reference audio file for comparison analysis")
	flag.Parse()

	if *storyPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: generate -story <story.json> [-server URL] [-voice name] [-speed N]\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Load story
	data, err := os.ReadFile(*storyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading story: %v\n", err)
		os.Exit(1)
	}

	var story Story
	if err := json.Unmarshal(data, &story); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing story JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Story: %s by %s (%d sections)\n", story.Title, story.Author, len(story.Sections))

	// Parse section filter
	sectionFilter := parseSectionFilter(*sections)

	// Resolve output directory
	outDir := *outputDir
	if outDir == "" {
		outDir = filepath.Join("build", "stories", slugify(story.Title))
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output dir: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Output: %s\n", outDir)
	fmt.Printf("Voice: %s @ %.2f speed\n", *voice, *speed)
	fmt.Println()

	// Generate each section
	type result struct {
		section  Section
		wavPath  string
		duration float64
		elapsed  time.Duration
	}
	var results []result

	for _, sec := range story.Sections {
		if sectionFilter != nil {
			if _, ok := sectionFilter[sec.Position]; !ok {
				continue
			}
		}

		fmt.Printf("Section %d: %s (%d chars)\n", sec.Position, sec.Name, len(sec.Content))

		start := time.Now()
		wavData, duration, err := synthesize(*serverURL, sec.Content, *voice, *speed)
		elapsed := time.Since(start)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR: %v\n", err)
			continue
		}

		filename := fmt.Sprintf("%02d-%s.wav", sec.Position, slugify(sec.Name))
		wavPath := filepath.Join(outDir, filename)
		if err := os.WriteFile(wavPath, wavData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR writing file: %v\n", err)
			continue
		}

		fmt.Printf("  -> %s (%.1fs, generated in %.1fs)\n", filename, duration, elapsed.Seconds())
		results = append(results, result{section: sec, wavPath: wavPath, duration: duration, elapsed: elapsed})
	}

	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "No sections generated.\n")
		os.Exit(1)
	}

	// Summary
	fmt.Printf("\n=== Generation Summary ===\n")
	var totalDuration, totalElapsed float64
	for _, r := range results {
		fmt.Printf("  Section %d: %.1fs audio, %.1fs generation\n",
			r.section.Position, r.duration, r.elapsed.Seconds())
		totalDuration += r.duration
		totalElapsed += r.elapsed.Seconds()
	}
	fmt.Printf("  Total: %.1fs audio (%.1f min), %.1fs generation (%.1f min)\n",
		totalDuration, totalDuration/60, totalElapsed, totalElapsed/60)
	fmt.Printf("  Real-time factor: %.2fx\n", totalDuration/totalElapsed)

	// Run analysis if requested
	if *analyze {
		fmt.Printf("\n=== Audio Analysis ===\n")

		var refData *audio.WAVFile
		if *refAudio != "" {
			refData, err = audio.ReadWAVFile(*refAudio)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not read ref audio: %v\n", err)
			}
		}

		reportBuf := &strings.Builder{}
		fmt.Fprintf(reportBuf, "Story: %s\n", story.Title)
		fmt.Fprintf(reportBuf, "Voice: %s @ %.2f\n", *voice, *speed)
		fmt.Fprintf(reportBuf, "Generated: %s\n\n", time.Now().Format("2006-01-02 15:04"))

		for _, r := range results {
			w, err := audio.ReadWAVFile(r.wavPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not read %s: %v\n", r.wavPath, err)
				continue
			}

			pause := audio.DetectPauses(w.Samples, w.SampleRate, -40.0, 0.1)
			pitch := audio.ExtractPitch(w.Samples, w.SampleRate)
			glitches := audio.DetectGlitches(w.Samples, w.SampleRate)

			severeCount := 0
			for _, g := range glitches {
				if g.Severity >= 35.0 {
					severeCount++
				}
			}

			header := fmt.Sprintf("Section %d: %s", r.section.Position, r.section.Name)
			fmt.Printf("\n%s\n", header)
			fmt.Printf("  Duration:    %.1fs\n", pause.TotalDuration)
			fmt.Printf("  Pauses:      %d (avg %.3fs, max %.3fs)\n", pause.PauseCount, pause.AvgPause, pause.MaxPause)
			fmt.Printf("  Pitch:       mean=%.1f median=%.1f stddev=%.1f Hz\n", pitch.MeanF0, pitch.MedianF0, pitch.StdDevF0)
			fmt.Printf("  Glitches:    %d total, %d severe\n", len(glitches), severeCount)

			// Histogram
			buckets := pauseHistogram(pause.Pauses)
			fmt.Printf("  Histogram:   <0.2s=%d  0.2-0.5s=%d  0.5-1.0s=%d  >1.0s=%d\n",
				buckets[0], buckets[1], buckets[2], buckets[3])

			// Write to report
			fmt.Fprintf(reportBuf, "%s\n", header)
			fmt.Fprintf(reportBuf, "  Duration:    %.1fs\n", pause.TotalDuration)
			fmt.Fprintf(reportBuf, "  Pauses:      %d (avg %.3fs, max %.3fs)\n", pause.PauseCount, pause.AvgPause, pause.MaxPause)
			fmt.Fprintf(reportBuf, "  Silence:     %.1f%%\n", pause.SilenceRatio*100)
			fmt.Fprintf(reportBuf, "  Pitch:       mean=%.1f median=%.1f stddev=%.1f range=%.0f-%.0f Hz\n",
				pitch.MeanF0, pitch.MedianF0, pitch.StdDevF0, pitch.MinF0, pitch.MaxF0)
			fmt.Fprintf(reportBuf, "  Glitches:    %d total, %d severe (>35x)\n", len(glitches), severeCount)
			fmt.Fprintf(reportBuf, "  Histogram:   <0.2s=%d  0.2-0.5s=%d  0.5-1.0s=%d  >1.0s=%d\n",
				buckets[0], buckets[1], buckets[2], buckets[3])
			if severeCount > 0 {
				for _, g := range glitches {
					if g.Severity >= 35.0 {
						min := int(g.TimeSec) / 60
						sec := g.TimeSec - float64(min*60)
						fmt.Fprintf(reportBuf, "    [GLITCH] %02d:%05.2f severity=%.1f type=%s\n",
							min, sec, g.Severity, g.Type)
					}
				}
			}
			fmt.Fprintln(reportBuf)
		}

		// If we have ref audio, add comparison stats
		if refData != nil {
			refPause := audio.DetectPauses(refData.Samples, refData.SampleRate, -40.0, 0.1)
			refPitch := audio.ExtractPitch(refData.Samples, refData.SampleRate)
			fmt.Fprintf(reportBuf, "Reference Audio:\n")
			fmt.Fprintf(reportBuf, "  Duration:    %.1fs\n", refPause.TotalDuration)
			fmt.Fprintf(reportBuf, "  Pitch:       mean=%.1f median=%.1f stddev=%.1f Hz\n",
				refPitch.MeanF0, refPitch.MedianF0, refPitch.StdDevF0)
			refBuckets := pauseHistogram(refPause.Pauses)
			fmt.Fprintf(reportBuf, "  Histogram:   <0.2s=%d  0.2-0.5s=%d  0.5-1.0s=%d  >1.0s=%d\n",
				refBuckets[0], refBuckets[1], refBuckets[2], refBuckets[3])
		}

		// Save report
		reportPath := filepath.Join(outDir, "report.txt")
		os.WriteFile(reportPath, []byte(reportBuf.String()), 0644)
		fmt.Printf("\nReport saved: %s\n", reportPath)
	}
}

func synthesize(serverURL, text, voice string, speed float64) ([]byte, float64, error) {
	reqBody := map[string]any{
		"input": map[string]any{
			"text":   text,
			"voice":  voice,
			"speed":  speed,
			"format": "wav",
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Post(serverURL, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Output struct {
			Audio           string  `json:"audio"`
			DurationSeconds float64 `json:"duration_seconds"`
		} `json:"output"`
		Error string `json:"error"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, 0, fmt.Errorf("parse response: %w", err)
	}
	if result.Error != "" {
		return nil, 0, fmt.Errorf("server error: %s", result.Error)
	}

	wavData, err := base64.StdEncoding.DecodeString(result.Output.Audio)
	if err != nil {
		return nil, 0, fmt.Errorf("decode audio: %w", err)
	}

	return wavData, result.Output.DurationSeconds, nil
}

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteRune('-')
		}
	}
	// Collapse multiple dashes
	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}

func parseSectionFilter(s string) map[int]struct{} {
	if s == "" {
		return nil
	}
	filter := make(map[int]struct{})
	for _, part := range strings.Split(s, ",") {
		var pos int
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &pos); err == nil {
			filter[pos] = struct{}{}
		}
	}
	return filter
}

func pauseHistogram(pauses []audio.Pause) [4]int {
	var buckets [4]int
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
	return buckets
}
