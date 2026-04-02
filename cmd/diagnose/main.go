package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

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

// qualityIssue is a unified wrapper for any detected issue across all detectors.
type qualityIssue struct {
	Category  string  // "emphasis", "pacing", "pronunciation", "intonation", "stress"
	Severity  string  // "high", "medium", "low"
	Timestamp int     // ms
	Summary   string  // one-line description
	Context   string  // surrounding text
	Score     float64 // sort key (higher = worse)
}

func main() {
	storyPath := flag.String("story", "", "Path to story JSON file (optional, for section context)")
	audioPath := flag.String("audio", "", "Audio file to analyze (WAV, MP4, M4B)")
	geminiPath := flag.String("gemini", "", "Gemini reference audio for cross-model stress comparison")
	sourceTextPath := flag.String("source-text", "", "Source text file (enables pronunciation + intonation detection)")
	whisperModel := flag.String("whisper-model", "models/ggml-base.en.bin", "Path to Whisper GGML model")
	threshold := flag.Float64("threshold", 1.5, "Z-score threshold for function word anomalies")
	contentThreshold := flag.Float64("content-threshold", 2.0, "Z-score threshold for content word anomalies")
	jsonOutput := flag.Bool("json", false, "Output as JSON for automation")
	flag.Parse()

	if *audioPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: diagnose -audio <file> [-source-text <file>] [-gemini <file>] [-story <story.json>]\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if *geminiPath != "" && *sourceTextPath == "" {
		fmt.Fprintf(os.Stderr, "Error: -gemini requires -source-text for stress comparison\n")
		os.Exit(1)
	}

	// Load source text if provided
	var sourceText string
	if *sourceTextPath != "" {
		data, err := os.ReadFile(*sourceTextPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading source text: %v\n", err)
			os.Exit(1)
		}
		sourceText = string(data)
	}

	// Determine WAV path — extract from non-WAV formats
	wavPath := *audioPath
	ext := strings.ToLower(filepath.Ext(*audioPath))
	if ext != ".wav" {
		fmt.Fprintf(os.Stderr, "Extracting WAV from %s...\n", filepath.Base(*audioPath))
		extracted, cleanup, err := audio.ExtractWAV(*audioPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error extracting WAV: %v\n", err)
			os.Exit(1)
		}
		defer cleanup()
		wavPath = extracted
	}

	// Load 24kHz audio for analysis
	analysisPath := wavPath
	if ext != ".wav" {
		fmt.Fprintf(os.Stderr, "Extracting 24kHz WAV for analysis...\n")
		tmp, err := os.CreateTemp("", "analysis-*.wav")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating temp file: %v\n", err)
			os.Exit(1)
		}
		tmp.Close()
		analysisPath = tmp.Name()
		defer os.Remove(analysisPath)

		if err := extractWAVAt(audioPath, analysisPath, 24000); err != nil {
			fmt.Fprintf(os.Stderr, "Error extracting 24kHz WAV: %v\n", err)
			os.Exit(1)
		}
	}

	wav, err := audio.ReadWAVFile(analysisPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading WAV: %v\n", err)
		os.Exit(1)
	}

	// Align words via Whisper
	fmt.Fprintf(os.Stderr, "Aligning words with Whisper...\n")
	alignment, err := audio.AlignWords(wavPath, *whisperModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error aligning words: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Aligned %d words (%.1fs)\n", len(alignment.Words), alignment.Duration)

	// === Run all detectors ===

	// 1. Emphasis
	fmt.Fprintf(os.Stderr, "Analyzing emphasis...\n")
	emphasisReport := audio.AnalyzeEmphasis(wav.Samples, wav.SampleRate, alignment)
	emphasisReport.AudioFile = *audioPath

	// Filter emphasis by threshold
	var filtered []audio.EmphasisAnomaly
	for _, a := range emphasisReport.Anomalies {
		minThreshold := *threshold
		if a.Type == "content_word_stress" {
			minThreshold = *contentThreshold
		}
		if a.CompositeZ >= minThreshold {
			filtered = append(filtered, a)
		}
	}
	emphasisReport.Anomalies = filtered

	// 2. Pacing
	fmt.Fprintf(os.Stderr, "Analyzing pacing...\n")
	pacingReport := audio.AnalyzePacing(alignment)
	pacingReport.AudioFile = *audioPath
	fmt.Fprintf(os.Stderr, "Pacing: %.1f words/sec, %d clauses, %d anomalies\n",
		pacingReport.OverallRate, pacingReport.Clauses, len(pacingReport.Anomalies))

	// 3. Pronunciation (requires source text)
	var pronReport *audio.PronunciationReport
	if sourceText != "" {
		fmt.Fprintf(os.Stderr, "Analyzing pronunciation...\n")
		pronReport = audio.DetectPronunciationErrors(alignment, sourceText)
		pronReport.AudioFile = *audioPath
		fmt.Fprintf(os.Stderr, "Pronunciation: %d errors (%d source words, %d whisper words)\n",
			len(pronReport.Errors), pronReport.SourceWords, pronReport.WhisperWords)
	}

	// 4. Intonation (requires source text + audio samples)
	var intonReport *audio.IntonationReport
	if sourceText != "" {
		fmt.Fprintf(os.Stderr, "Analyzing intonation...\n")
		intonReport = audio.AnalyzeIntonation(wav.Samples, wav.SampleRate, alignment, sourceText)
		intonReport.AudioFile = *audioPath
		fmt.Fprintf(os.Stderr, "Intonation: %d anomalies across %d sentences\n",
			len(intonReport.Anomalies), intonReport.Sentences)
	}

	// 5. Cross-model stress comparison (optional)
	var stressReport *audio.StressReport
	if *geminiPath != "" && sourceText != "" {
		stressReport = runStressComparison(*geminiPath, wav, alignment, sourceText, *whisperModel)
	}

	// Load story for context (optional)
	var story *Story
	if *storyPath != "" {
		s, err := loadStory(*storyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load story: %v\n", err)
		} else {
			story = s
		}
	}

	// === Output ===

	if *jsonOutput {
		output := map[string]interface{}{
			"emphasis": emphasisReport,
			"pacing":   pacingReport,
		}
		if pronReport != nil {
			output["pronunciation"] = pronReport
		}
		if intonReport != nil {
			output["intonation"] = intonReport
		}
		if stressReport != nil {
			output["stress"] = stressReport
		}
		output["quality"] = computeQualityScore(emphasisReport, pacingReport, pronReport, intonReport, stressReport)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(output)
		return
	}

	// Unified quality report
	printUnifiedReport(*audioPath, story, emphasisReport, pacingReport, pronReport, intonReport, stressReport)
}

// qualityScore holds the computed quality score and breakdown.
type qualityScore struct {
	Score       float64            `json:"score"`
	MaxScore    float64            `json:"max_score"`
	ByCategory  map[string]int     `json:"by_category"`
	BySeverity  map[string]int     `json:"by_severity"`
	TotalIssues int                `json:"total_issues"`
}

func computeQualityScore(
	emphasis *audio.EmphasisReport,
	pacing *audio.PacingReport,
	pron *audio.PronunciationReport,
	inton *audio.IntonationReport,
	stress *audio.StressReport,
) qualityScore {
	qs := qualityScore{
		MaxScore:   10.0,
		ByCategory: map[string]int{},
		BySeverity: map[string]int{"high": 0, "medium": 0, "low": 0},
	}

	score := 10.0
	deduct := func(severity, category string) {
		qs.TotalIssues++
		qs.ByCategory[category]++
		qs.BySeverity[severity]++
		switch severity {
		case "high":
			score -= 0.3
		case "medium":
			score -= 0.1
		case "low":
			score -= 0.03
		}
	}

	if emphasis != nil {
		for _, a := range emphasis.Anomalies {
			deduct(a.Severity, "emphasis")
		}
	}
	if pacing != nil {
		for _, a := range pacing.Anomalies {
			deduct(a.Severity, "pacing")
		}
	}
	if pron != nil {
		for _, e := range pron.Errors {
			deduct(e.Severity, "pronunciation")
		}
	}
	if inton != nil {
		for _, a := range inton.Anomalies {
			deduct(a.Severity, "intonation")
		}
	}
	if stress != nil {
		for _, m := range stress.Mismatches {
			deduct(m.Severity, "stress")
		}
	}

	// Normalize by duration — longer audio gets more lenient scoring.
	// Baseline: 60s. Audio twice as long gets half the deductions.
	duration := 60.0
	if emphasis != nil && emphasis.Duration > 0 {
		duration = emphasis.Duration
	}
	if duration > 60.0 {
		factor := 60.0 / duration
		score = 10.0 + (score-10.0)*factor
	}

	if score < 0 {
		score = 0
	}
	qs.Score = float64(int(score*10)) / 10 // round to 1 decimal
	return qs
}

func printUnifiedReport(
	audioFile string,
	story *Story,
	emphasis *audio.EmphasisReport,
	pacing *audio.PacingReport,
	pron *audio.PronunciationReport,
	inton *audio.IntonationReport,
	stress *audio.StressReport,
) {
	qs := computeQualityScore(emphasis, pacing, pron, inton, stress)

	fmt.Printf("=== Audio Quality Report ===\n")
	fmt.Printf("Audio: %s (%.1fs, %d words)\n", audioFile, emphasis.Duration, emphasis.TotalWords)
	if story != nil {
		fmt.Printf("Story: %s by %s\n", story.Title, story.Author)
	}
	fmt.Printf("Quality Score: %.1f/10\n\n", qs.Score)

	// Category summary
	categories := []string{"emphasis", "pacing", "pronunciation", "intonation", "stress"}
	for _, cat := range categories {
		if c, ok := qs.ByCategory[cat]; ok && c > 0 {
			fmt.Printf("  %-15s %d issues\n", cat+":", c)
		}
	}
	fmt.Printf("  %-15s %d high, %d medium, %d low\n\n",
		"total:", qs.BySeverity["high"], qs.BySeverity["medium"], qs.BySeverity["low"])

	// Collect all issues into unified list
	var issues []qualityIssue

	if emphasis != nil {
		for _, a := range emphasis.Anomalies {
			issues = append(issues, qualityIssue{
				Category:  "emphasis",
				Severity:  a.Severity,
				Timestamp: a.Word.StartMs,
				Summary:   fmt.Sprintf("Unexpected emphasis on \"%s\" (z=%.2f)", a.Word.Word, a.CompositeZ),
				Context:   a.Context,
				Score:     a.CompositeZ,
			})
		}
	}

	if pacing != nil {
		for _, a := range pacing.Anomalies {
			var summary string
			switch a.Type {
			case "rushed_segment":
				summary = fmt.Sprintf("Rushed segment (%.1f w/s, avg %.1f)", a.Rate, a.AvgRate)
			case "slow_segment":
				summary = fmt.Sprintf("Slow segment (%.1f w/s, avg %.1f)", a.Rate, a.AvgRate)
			case "unexpected_pause":
				summary = fmt.Sprintf("Unexpected %dms pause after function word", a.GapMs)
			case "erratic_pacing":
				summary = fmt.Sprintf("Erratic pacing (CV=%.2f)", a.Variance)
			}
			score := 1.0
			if a.Severity == "high" {
				score = 3.0
			} else if a.Severity == "medium" {
				score = 2.0
			}
			issues = append(issues, qualityIssue{
				Category:  "pacing",
				Severity:  a.Severity,
				Timestamp: a.StartMs,
				Summary:   summary,
				Context:   a.Context,
				Score:     score,
			})
		}
	}

	if pron != nil {
		for _, e := range pron.Errors {
			var summary string
			switch e.Type {
			case "substitution":
				summary = fmt.Sprintf("Expected \"%s\" → heard \"%s\"", e.Expected, e.Heard)
			case "insertion":
				summary = fmt.Sprintf("Unexpected word \"%s\"", e.Heard)
			case "deletion":
				summary = fmt.Sprintf("Missing word \"%s\"", e.Expected)
			}
			score := 1.0
			if e.Severity == "high" {
				score = 3.0
			} else if e.Severity == "medium" {
				score = 2.0
			}
			issues = append(issues, qualityIssue{
				Category:  "pronunciation",
				Severity:  e.Severity,
				Timestamp: e.StartMs,
				Summary:   summary,
				Context:   e.Context,
				Score:     score,
			})
		}
	}

	if inton != nil {
		for _, a := range inton.Anomalies {
			var summary string
			switch a.Type {
			case "monotone_segment":
				summary = fmt.Sprintf("Monotone delivery (F0 range %.0f Hz, avg %.0f)", a.F0Range, a.AvgRange)
			case "wrong_question_inflection":
				summary = fmt.Sprintf("Question with falling pitch (%.0f→%.0f Hz)", a.F0Start, a.F0End)
			case "uptalk":
				summary = fmt.Sprintf("Statement with rising pitch (%.0f→%.0f Hz)", a.F0Start, a.F0End)
			}
			score := 1.0
			if a.Severity == "high" {
				score = 3.0
			} else if a.Severity == "medium" {
				score = 2.0
			}
			issues = append(issues, qualityIssue{
				Category:  "intonation",
				Severity:  a.Severity,
				Timestamp: a.StartMs,
				Summary:   summary,
				Context:   a.Context,
				Score:     score,
			})
		}
	}

	if stress != nil {
		for _, m := range stress.Mismatches {
			score := 1.0
			if m.Severity == "high" {
				score = 3.0
			} else if m.Severity == "medium" {
				score = 2.0
			}
			issues = append(issues, qualityIssue{
				Category:  "stress",
				Severity:  m.Severity,
				Timestamp: m.StartMs,
				Summary:   fmt.Sprintf("Syllable stress mismatch on \"%s\"", m.Word),
				Context:   m.Context,
				Score:     score,
			})
		}
	}

	if len(issues) == 0 {
		fmt.Printf("No issues detected.\n")
		return
	}

	// Sort by severity (high first), then by timestamp
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Score != issues[j].Score {
			return issues[i].Score > issues[j].Score
		}
		return issues[i].Timestamp < issues[j].Timestamp
	})

	// Print top issues (limit to 30 for readability)
	limit := len(issues)
	if limit > 30 {
		limit = 30
	}
	fmt.Printf("Top Issues:\n")
	for i := 0; i < limit; i++ {
		iss := issues[i]
		ts := formatTimestamp(iss.Timestamp)
		fmt.Printf("  %2d. [%s] [%s] %s at %s\n",
			i+1, strings.ToUpper(iss.Severity), iss.Category, iss.Summary, ts)
		if iss.Context != "" {
			fmt.Printf("      \"%s\"\n", iss.Context)
		}
	}
	if len(issues) > limit {
		fmt.Printf("  ... and %d more issues\n", len(issues)-limit)
	}
}

func formatTimestamp(ms int) string {
	totalSec := ms / 1000
	remainMs := ms % 1000
	min := totalSec / 60
	sec := totalSec % 60
	return fmt.Sprintf("%02d:%02d.%03d", min, sec, remainMs)
}

func loadStory(path string) (*Story, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var story Story
	if err := json.Unmarshal(data, &story); err != nil {
		return nil, err
	}
	return &story, nil
}

func runStressComparison(geminiPath string, wav *audio.WAVFile, alignment *audio.Alignment, sourceText, whisperModel string) *audio.StressReport {
	geminiWavPath := geminiPath
	geminiExt := strings.ToLower(filepath.Ext(geminiPath))
	if geminiExt != ".wav" {
		fmt.Fprintf(os.Stderr, "Extracting WAV from Gemini audio %s...\n", filepath.Base(geminiPath))
		extracted, cleanup, err := audio.ExtractWAV(geminiPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error extracting Gemini WAV: %v\n", err)
			return nil
		}
		defer cleanup()
		geminiWavPath = extracted
	}

	geminiAnalysisPath := geminiWavPath
	if geminiExt != ".wav" {
		fmt.Fprintf(os.Stderr, "Extracting 24kHz WAV for Gemini analysis...\n")
		tmp, err := os.CreateTemp("", "gemini-analysis-*.wav")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating temp file: %v\n", err)
			return nil
		}
		tmp.Close()
		geminiAnalysisPath = tmp.Name()
		defer os.Remove(geminiAnalysisPath)

		if err := extractWAVAt(&geminiPath, geminiAnalysisPath, 24000); err != nil {
			fmt.Fprintf(os.Stderr, "Error extracting Gemini 24kHz WAV: %v\n", err)
			return nil
		}
	}

	geminiWav, err := audio.ReadWAVFile(geminiAnalysisPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading Gemini WAV: %v\n", err)
		return nil
	}

	fmt.Fprintf(os.Stderr, "Aligning Gemini words with Whisper...\n")
	geminiAlignment, err := audio.AlignWords(geminiWavPath, whisperModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error aligning Gemini words: %v\n", err)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Aligned %d Gemini words (%.1fs)\n",
		len(geminiAlignment.Words), geminiAlignment.Duration)

	fmt.Fprintf(os.Stderr, "Comparing syllable stress patterns...\n")
	report := audio.CompareStress(
		wav.Samples, geminiWav.Samples,
		wav.SampleRate, geminiWav.SampleRate,
		alignment, geminiAlignment,
		sourceText,
	)
	fmt.Fprintf(os.Stderr, "Compared %d multi-syllable words, found %d mismatches\n",
		report.Compared, len(report.Mismatches))
	return report
}

// extractWAVAt uses ffmpeg to extract audio at a specific sample rate.
func extractWAVAt(inputPath *string, outputPath string, sampleRate int) error {
	cmd := exec.Command("ffmpeg",
		"-i", *inputPath,
		"-ar", fmt.Sprintf("%d", sampleRate),
		"-ac", "1",
		"-y",
		"-loglevel", "error",
		outputPath,
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg error: %v: %s", err, stderr.String())
	}
	return nil
}
