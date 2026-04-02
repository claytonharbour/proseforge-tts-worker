package audio

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AlignedWord represents a single word with its time boundaries from Whisper alignment.
type AlignedWord struct {
	Word       string
	StartMs    int
	EndMs      int
	Confidence float64
}

// Alignment holds the full word-level alignment for an audio file.
type Alignment struct {
	Words    []AlignedWord
	Duration float64 // total audio duration in seconds
}

// whisperFullJSON is the top-level structure of whisper-cli --output-json-full output.
type whisperFullJSON struct {
	Transcription []whisperSegment `json:"transcription"`
}

type whisperSegment struct {
	Tokens []whisperToken `json:"tokens"`
}

type whisperToken struct {
	Text string  `json:"text"`
	T0   int     `json:"offsets>from"` // won't work with nested — parse manually
	T1   int     `json:"offsets>to"`
	P    float64 `json:"p"`
}

// whisperTokenRaw is used for manual JSON parsing of the nested offsets structure.
type whisperTokenRaw struct {
	Text    string  `json:"text"`
	Offsets struct {
		From int `json:"from"`
		To   int `json:"to"`
	} `json:"offsets"`
	P float64 `json:"p"`
}

type whisperSegmentRaw struct {
	Tokens []whisperTokenRaw `json:"tokens"`
}

type whisperFullJSONRaw struct {
	Transcription []whisperSegmentRaw `json:"transcription"`
}

// specialTokens are whisper control tokens that should be skipped during word reconstruction.
var specialTokens = map[string]bool{
	"[_BEG_]":   true,
	"[_TT_0]":   true,
	"[_TT_1]":   true,
	"[_TT_2]":   true,
	"[_EOT_]":   true,
	"[_SOT_]":   true,
	"[_PREV_]":  true,
	"[_NOT_]":   true,
	"[_SOLM_]":  true,
	"[BLANK_AUDIO]": true,
}

// AlignWords runs whisper-cli with --output-json-full and reconstructs word-level alignment.
// wavPath must be a WAV file. modelPath is the path to a Whisper GGML model.
func AlignWords(wavPath, modelPath string) (*Alignment, error) {
	if _, err := exec.LookPath("whisper-cli"); err != nil {
		return nil, fmt.Errorf("whisper-cli not found: install with 'brew install whisper-cpp'")
	}

	// whisper-cli writes JSON to <input>.json when using --output-json-full
	// Use a temp directory to avoid polluting the source directory.
	tmpDir, err := os.MkdirTemp("", "align-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Copy or symlink the WAV to temp dir so output lands there.
	tmpWav := filepath.Join(tmpDir, "audio.wav")
	wavData, err := os.ReadFile(wavPath)
	if err != nil {
		return nil, fmt.Errorf("reading WAV file: %w", err)
	}
	if err := os.WriteFile(tmpWav, wavData, 0644); err != nil {
		return nil, fmt.Errorf("writing temp WAV: %w", err)
	}

	cmd := exec.Command("whisper-cli",
		"--model", modelPath,
		"--language", "en",
		"--output-json-full",
		"--no-prints",
		"--file", tmpWav,
	)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("whisper-cli error: %v: %s", err, stderr.String())
	}

	// whisper-cli writes output to <input>.json
	jsonPath := tmpWav + ".json"
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("reading whisper JSON output: %w", err)
	}

	return parseWhisperJSON(jsonData)
}

// parseWhisperJSON reconstructs words from whisper token-level JSON.
// Exported for testing.
func parseWhisperJSON(data []byte) (*Alignment, error) {
	var raw whisperFullJSONRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing whisper JSON: %w", err)
	}

	// Collect all tokens across segments
	var tokens []whisperTokenRaw
	for _, seg := range raw.Transcription {
		tokens = append(tokens, seg.Tokens...)
	}

	// Reconstruct words from tokens.
	// Token with leading space = new word start.
	// Subsequent tokens without leading space are continuations.
	var words []AlignedWord
	var currentWord string
	var wordStartMs, wordEndMs int
	var wordConfSum float64
	var wordTokenCount int
	inWord := false

	for _, tok := range tokens {
		text := tok.Text

		// Skip special tokens
		if specialTokens[strings.TrimSpace(text)] {
			continue
		}

		// Skip low-confidence noise tokens
		if tok.P < 0.3 {
			continue
		}

		startsNew := strings.HasPrefix(text, " ")
		cleanText := strings.TrimSpace(text)
		if cleanText == "" {
			continue
		}

		if startsNew && inWord {
			// Flush previous word
			words = append(words, AlignedWord{
				Word:       currentWord,
				StartMs:    wordStartMs,
				EndMs:      wordEndMs,
				Confidence: wordConfSum / float64(wordTokenCount),
			})
			inWord = false
		}

		if !inWord {
			// Start new word
			currentWord = cleanText
			wordStartMs = tok.Offsets.From
			wordEndMs = tok.Offsets.To
			wordConfSum = tok.P
			wordTokenCount = 1
			inWord = true
		} else {
			// Continue current word
			currentWord += cleanText
			wordEndMs = tok.Offsets.To
			wordConfSum += tok.P
			wordTokenCount++
		}
	}

	// Flush last word
	if inWord {
		words = append(words, AlignedWord{
			Word:       currentWord,
			StartMs:    wordStartMs,
			EndMs:      wordEndMs,
			Confidence: wordConfSum / float64(wordTokenCount),
		})
	}

	// Gap-fill: if EndMs <= StartMs, extend to next word or +300ms
	for i := range words {
		if words[i].EndMs <= words[i].StartMs {
			if i+1 < len(words) {
				words[i].EndMs = min(words[i+1].StartMs, words[i].StartMs+300)
			} else {
				words[i].EndMs = words[i].StartMs + 300
			}
		}
	}

	// Compute total duration from last word end
	var duration float64
	if len(words) > 0 {
		duration = float64(words[len(words)-1].EndMs) / 1000.0
	}

	return &Alignment{
		Words:    words,
		Duration: duration,
	}, nil
}

// ExtractWAV converts an audio file (MP4, MP3, etc.) to 16kHz mono WAV using ffmpeg.
// Returns the temp WAV path and a cleanup function that removes it.
func ExtractWAV(inputPath string) (wavPath string, cleanup func(), err error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", nil, fmt.Errorf("ffmpeg not found: install with 'brew install ffmpeg'")
	}

	tmp, err := os.CreateTemp("", "extract-*.wav")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmp.Close()

	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-ar", "16000",
		"-ac", "1",
		"-y",
		"-loglevel", "error",
		tmp.Name(),
	)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("ffmpeg error: %v: %s", err, stderr.String())
	}

	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
}
