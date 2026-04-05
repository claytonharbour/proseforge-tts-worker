package tts

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/claytonharbour/proseforge-tts-worker/internal/audio"
	"github.com/claytonharbour/proseforge-tts-worker/internal/inference"
	"github.com/claytonharbour/proseforge-tts-worker/internal/phonemizer"
)

const (
	// clauseSilenceSamples is 0.05 seconds of silence at 24kHz,
	// inserted at clause boundaries (commas, em dashes, semicolons).
	// Kept short because the model already produces micro-pauses at punctuation.
	clauseSilenceSamples = 1200
	// crossfadeSamples is ~20ms at 24kHz, used between segments within a sentence.
	// 20ms gives enough overlap for spectral continuity at segment boundaries.
	crossfadeSamples = 480
	// fadeSamples is 5ms at 24kHz, used at sentence/paragraph silence edges.
	fadeSamples = 120
	// clauseFadeSamples is 15ms at 24kHz, used at clause pause edges.
	// Longer than fadeSamples because clause pauses are inserted mid-sentence
	// where the model still has active speech energy that needs a smooth taper.
	clauseFadeSamples = 360
	// energySearchSamples is +/- 50ms at 24kHz. When inserting clause pauses,
	// we search this radius around the linear estimate for the local energy
	// minimum — the model's natural micro-pause at punctuation.
	energySearchSamples = 1200
)

// Engine is the top-level TTS orchestrator.
// It wires together phonemization, tokenization, inference, and audio encoding.
type Engine struct {
	phonemizer *phonemizer.Phonemizer
	inference  *inference.Engine
	voices     *inference.VoiceStore
	version    string
}

// Config holds all paths needed to initialize the TTS engine.
type Config struct {
	// Phonemizer data files
	GoldDictPath   string
	SilverDictPath string
	HomographsPath string
	TokenMapPath   string
	ARPABETMapPath string // needed by LTS for ARPABET→IPA conversion
	CustomDictPath string // optional — custom pronunciations for proper nouns etc.

	// ONNX model files
	ONNXLibraryPath string
	ModelPath       string
	VoicesPath      string

	// Version is embedded in WAV metadata (LIST INFO ISFT field).
	// Format: "ProseForge TTS v1.1 (abc1234)" or similar.
	Version string
}

// New creates a fully initialized TTS engine.
func New(cfg Config) (*Engine, error) {
	p, err := phonemizer.New(phonemizer.Config{
		GoldDictPath:   cfg.GoldDictPath,
		SilverDictPath: cfg.SilverDictPath,
		HomographsPath: cfg.HomographsPath,
		TokenMapPath:   cfg.TokenMapPath,
		ARPABETMapPath: cfg.ARPABETMapPath,
		CustomDictPath: cfg.CustomDictPath,
	})
	if err != nil {
		return nil, fmt.Errorf("init phonemizer: %w", err)
	}

	eng, err := inference.NewEngine(cfg.ONNXLibraryPath, cfg.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("init inference: %w", err)
	}

	voices, err := inference.LoadVoices(cfg.VoicesPath)
	if err != nil {
		eng.Close()
		return nil, fmt.Errorf("load voices: %w", err)
	}

	return &Engine{
		phonemizer: p,
		inference:  eng,
		voices:     voices,
		version:    cfg.Version,
	}, nil
}

// Synthesize converts text to audio.
// Returns the audio bytes (WAV or MP3), duration in seconds, and any error.
func (e *Engine) Synthesize(text, voiceName, format string, speed float64) ([]byte, float64, error) {
	if text == "" {
		return nil, 0, fmt.Errorf("text is empty")
	}

	// Resolve voice — supports blends like "af_sarah+bf_alice@20" (80% sarah, 20% alice)
	voiceName, err := e.resolveVoiceBlend(voiceName)
	if err != nil {
		return nil, 0, fmt.Errorf("voice: %w", err)
	}
	voice, err := e.voices.Get(voiceName)
	if err != nil {
		return nil, 0, fmt.Errorf("voice: %w", err)
	}

	// Phonemize and tokenize (paragraph-aware)
	paragraphs, err := e.phonemizer.TokenizeText(text)
	if err != nil {
		return nil, 0, fmt.Errorf("phonemize: %w", err)
	}

	if len(paragraphs) == 0 {
		return nil, 0, fmt.Errorf("no tokens produced from text")
	}

	// Run inference: paragraph → sentence group → segment
	var allSamples []float32
	var prevParagraphGroups int
	totalParagraphs := len(paragraphs)
	synthStart := time.Now()

	for pi, sentences := range paragraphs {
		var paragraphSamples []float32

		for si, sent := range sentences {
			// Insert sentence silence between sentence groups
			if si > 0 && len(paragraphSamples) > 0 {
				// Fade out the end of the current waveform to prevent click
				fadeOutEnd(paragraphSamples, fadeSamples)
				silenceLen := sentenceSilenceFor(sentences[si-1].AllTokens)
				silence := make([]float32, silenceLen)
				paragraphSamples = append(paragraphSamples, silence...)
			}

			// Infer each segment and crossfade within the sentence group
			var sentenceSamples []float32
			for _, tokens := range sent.Segments {
				if len(tokens) == 0 {
					continue
				}

				paddedLen := len(tokens) + 2
				style := voice.StyleForLength(paddedLen)

				waveform, err := e.inference.Infer(tokens, style, float32(speed))
				if err != nil {
					return nil, 0, fmt.Errorf("inference paragraph %d: %w", pi, err)
				}

				waveform = audio.TrimSilence(waveform)
				if len(waveform) == 0 {
					continue
				}

				if len(sentenceSamples) > 0 {
					sentenceSamples = crossfade(sentenceSamples, waveform, crossfadeSamples)
				} else {
					sentenceSamples = append(sentenceSamples, waveform...)
				}
			}

			if len(sentenceSamples) == 0 {
				continue
			}

			// Insert clause pauses post-hoc into the sentence waveform
			sentenceSamples = insertClausePauses(sentenceSamples, sent.AllTokens, clauseSilenceSamples)

			// Fade in the start of this sentence if it follows a silence gap
			if si > 0 {
				fadeInStart(sentenceSamples, fadeSamples)
			}

			paragraphSamples = append(paragraphSamples, sentenceSamples...)
		}

		if len(paragraphSamples) == 0 {
			continue
		}

		// Between paragraphs: insert silence with fade edges
		if len(allSamples) > 0 {
			fadeOutEnd(allSamples, fadeSamples)
			silenceLen := paragraphSilenceFor(prevParagraphGroups)
			silence := make([]float32, silenceLen)
			allSamples = append(allSamples, silence...)
			fadeInStart(paragraphSamples, fadeSamples)
		}
		prevParagraphGroups = len(sentences)
		allSamples = append(allSamples, paragraphSamples...)

		// Progress logging for long texts
		if totalParagraphs >= 5 {
			elapsed := time.Since(synthStart)
			audioDur := audio.Duration(len(allSamples))
			log.Printf("paragraph %d/%d done (%.1fs audio, %s elapsed)",
				pi+1, totalParagraphs, audioDur, elapsed.Round(time.Second))
		}
	}

	if len(allSamples) == 0 {
		return nil, 0, fmt.Errorf("no audio produced")
	}

	// Smooth onset — 5ms fade-in prevents clicks from trimmed fricatives
	for i := 0; i < fadeSamples && i < len(allSamples); i++ {
		allSamples[i] *= float32(i) / float32(fadeSamples)
	}

	// Expand dynamic range — accentuate natural loudness variation
	allSamples = audio.ExpandDynamicRange(allSamples)

	// Final trim — remove any leading/trailing silence from the assembled waveform.
	// Per-segment TrimSilence runs earlier but silence can accumulate from fades
	// and paragraph gaps at the edges.
	allSamples = audio.TrimSilence(allSamples)

	// Calculate duration
	duration := audio.Duration(len(allSamples))
	duration = math.Round(duration*1000) / 1000

	// Encode to WAV with version metadata
	var meta *audio.WAVMetadata
	if e.version != "" {
		meta = &audio.WAVMetadata{
			Software: e.version,
			Artist:   voiceName,
			Comment:  audio.FormatMetadataComment(speed, format),
		}
	}
	wavData := audio.EncodeWAVWithMetadata(allSamples, meta)

	// Convert to MP3 if requested
	if format == "mp3" {
		mp3Data, err := audio.EncodeMP3(wavData, meta)
		if err != nil {
			return nil, 0, fmt.Errorf("MP3 encode: %w", err)
		}
		return mp3Data, duration, nil
	}

	return wavData, duration, nil
}

// VoiceNames returns all available voice names.
func (e *Engine) VoiceNames() []string {
	return e.voices.Names()
}

// Close releases all resources.
func (e *Engine) Close() {
	if e.inference != nil {
		e.inference.Close()
	}
}

// resolveVoiceBlend parses blend syntax "voiceA+voiceB@N" where N is the
// percentage of voiceB (0-100). If the name doesn't contain "+", it's returned as-is.
// Blended voices are created once and cached in the voice store.
func (e *Engine) resolveVoiceBlend(name string) (string, error) {
	if !strings.Contains(name, "+") {
		return name, nil
	}

	// Already created?
	if _, err := e.voices.Get(name); err == nil {
		return name, nil
	}

	// Parse "voiceA+voiceB@N"
	parts := strings.SplitN(name, "+", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid blend syntax: %s", name)
	}
	voiceA := parts[0]
	rest := parts[1]

	// Split voiceB@N
	atParts := strings.SplitN(rest, "@", 2)
	voiceB := atParts[0]
	blend := float32(0.2) // default 20%
	if len(atParts) == 2 {
		pct, err := strconv.ParseFloat(atParts[1], 32)
		if err != nil {
			return "", fmt.Errorf("invalid blend percentage: %s", atParts[1])
		}
		blend = float32(pct) / 100.0
	}

	blendName, err := e.voices.CreateBlend(voiceA, voiceB, blend)
	if err != nil {
		return "", err
	}
	return blendName, nil
}

// crossfade blends the end of segment a with the beginning of segment b
// using a Hann window for smooth spectral transitions at segment boundaries.
// Returns a single combined slice.
func crossfade(a, b []float32, fadeLen int) []float32 {
	if fadeLen <= 0 || len(a) < fadeLen || len(b) < fadeLen {
		// Can't crossfade — just concatenate
		return append(a, b...)
	}

	// Overlap region: last fadeLen of a blended with first fadeLen of b
	result := make([]float32, len(a)+len(b)-fadeLen)

	// Copy non-overlapping part of a
	copy(result, a[:len(a)-fadeLen])

	// Blend overlap region using Hann window (raised cosine)
	// Hann has zero derivative at boundaries → smoother spectral transitions
	for i := 0; i < fadeLen; i++ {
		t := float64(i) / float64(fadeLen)
		w := float32(0.5 * (1 - math.Cos(math.Pi*t)))
		aIdx := len(a) - fadeLen + i
		bIdx := i
		result[len(a)-fadeLen+i] = a[aIdx]*(1-w) + b[bIdx]*w
	}

	// Copy non-overlapping part of b
	copy(result[len(a):], b[fadeLen:])

	return result
}

// findClausePausePositions scans tokens for clause boundaries (comma, em dash,
// semicolon) that have at least minWords space tokens since the last boundary.
// Returns token indices where silence should be inserted (after the boundary token).
func findClausePausePositions(tokens []int64) []int {
	const (
		commaToken     int64 = 3
		semicolonToken int64 = 1
		emDashToken    int64 = 9
		spaceToken     int64 = 16
		minWords             = 4
	)

	var positions []int
	spaceCount := 0

	for i, tok := range tokens {
		if tok == spaceToken {
			spaceCount++
			continue
		}
		if (tok == commaToken || tok == semicolonToken || tok == emDashToken) && spaceCount >= minWords-1 {
			positions = append(positions, i)
			spaceCount = 0
		}
	}

	return positions
}

// insertClausePauses finds clause boundaries in the token sequence and inserts
// silence at the corresponding waveform positions. Token-to-sample mapping uses
// linear interpolation as an initial estimate, then snaps to the nearest energy
// minimum within a search window. The model naturally produces micro-pauses at
// punctuation — snapping to these troughs makes inserted pauses sound seamless.
func insertClausePauses(waveform []float32, allTokens []int64, silenceLen int) []float32 {
	positions := findClausePausePositions(allTokens)
	if len(positions) == 0 {
		return waveform
	}

	totalTokens := len(allTokens)
	totalSamples := len(waveform)

	// Process rightmost positions first so earlier indices aren't invalidated
	for i := len(positions) - 1; i >= 0; i-- {
		// Map token position (after the boundary token) to sample position
		tokenPos := positions[i] + 1
		samplePos := (tokenPos * totalSamples) / totalTokens
		if samplePos > totalSamples {
			samplePos = totalSamples
		}
		// Snap to the nearest energy minimum within +/- 50ms
		samplePos = findEnergyMinimum(waveform, samplePos, energySearchSamples)
		waveform = insertSilenceAt(waveform, samplePos, silenceLen, clauseFadeSamples)
	}

	return waveform
}

// findEnergyMinimum searches for the local RMS energy minimum near pos.
// Returns the sample position at the center of the lowest-energy frame
// within [pos-searchRadius, pos+searchRadius].
func findEnergyMinimum(samples []float32, pos, searchRadius int) int {
	const frameSize = 240 // 10ms at 24kHz — wide enough for stable RMS

	n := len(samples)
	start := pos - searchRadius
	if start < 0 {
		start = 0
	}
	end := pos + searchRadius
	if end > n-frameSize {
		end = n - frameSize
	}
	if start >= end {
		return pos
	}

	bestPos := pos
	bestRMS := float32(1e30)

	// Slide a frame across the search window, stepping by half-frame for resolution
	for p := start; p <= end; p += frameSize / 2 {
		var sum float32
		limit := p + frameSize
		if limit > n {
			limit = n
		}
		for j := p; j < limit; j++ {
			s := samples[j]
			sum += s * s
		}
		rms := sum / float32(limit-p)
		if rms < bestRMS {
			bestRMS = rms
			bestPos = p + frameSize/2 // center of the frame
		}
	}

	if bestPos >= n {
		bestPos = n - 1
	}
	return bestPos
}

// fadeOutEnd applies a fade-out to the last fadeLen samples of a waveform.
func fadeOutEnd(samples []float32, fadeLen int) {
	n := len(samples)
	if fadeLen > n {
		fadeLen = n
	}
	for i := 0; i < fadeLen; i++ {
		t := float32(i) / float32(fadeLen)
		samples[n-fadeLen+i] *= (1 - t)
	}
}

// fadeInStart applies a fade-in to the first fadeLen samples of a waveform.
func fadeInStart(samples []float32, fadeLen int) {
	n := len(samples)
	if fadeLen > n {
		fadeLen = n
	}
	for i := 0; i < fadeLen; i++ {
		t := float32(i) / float32(fadeLen)
		samples[i] *= t
	}
}

// insertSilenceAt splices silence into a waveform at the given position.
// Applies fade-out before and fade-in after the silence to prevent click artifacts.
func insertSilenceAt(waveform []float32, pos, silenceLen, fadeLen int) []float32 {
	if pos < 0 || pos > len(waveform) {
		return waveform
	}

	// Clamp fade length to available samples
	fadeOut := fadeLen
	if fadeOut > pos {
		fadeOut = pos
	}
	fadeIn := fadeLen
	if fadeIn > len(waveform)-pos {
		fadeIn = len(waveform) - pos
	}

	// Create new waveform with silence inserted
	result := make([]float32, len(waveform)+silenceLen)
	copy(result, waveform[:pos])
	// silence region is zero-valued (already initialized)
	copy(result[pos+silenceLen:], waveform[pos:])

	// Fade-out before silence
	for i := 0; i < fadeOut; i++ {
		t := float32(i) / float32(fadeOut)
		result[pos-fadeOut+i] *= (1 - t)
	}

	// Fade-in after silence
	for i := 0; i < fadeIn; i++ {
		t := float32(i) / float32(fadeIn)
		result[pos+silenceLen+i] *= t
	}

	return result
}

// sentenceSilenceFor computes adaptive sentence silence based on the preceding
// sentence group's terminal punctuation and length. Returns sample count at 24kHz.
// Range: 0.20s (4800) to 0.35s (8400), centered on 0.25s (6000).
func sentenceSilenceFor(prevTokens []int64) int {
	const sampleRate = 24000

	// Base silence by terminal punctuation type
	base := 0.25 // period (default)
	if punct := terminalPunctuation(prevTokens); punct != 0 {
		switch punct {
		case 5: // !
			base = 0.32
		case 6: // ?
			base = 0.30
		case 10: // …
			base = 0.35
		}
	}

	// Length adjustment: short sentences get slightly longer pauses (dramatic),
	// long sentences get slightly shorter (listener wants to move on).
	n := len(prevTokens)
	switch {
	case n <= 50:
		base += 0.03
	case n >= 200:
		base -= 0.03
	}

	// Clamp to range
	if base < 0.20 {
		base = 0.20
	}
	if base > 0.35 {
		base = 0.35
	}

	return int(base * float64(sampleRate))
}

// paragraphSilenceFor computes adaptive paragraph silence based on the preceding
// paragraph's sentence group count. Returns sample count at 24kHz.
// Range: 0.70s (16800) to 0.85s (20400), centered on 0.80s (19200).
// Short paragraphs (1-2 groups) are dramatic → longer pause.
// Long paragraphs (4+ groups) are expository → shorter pause.
func paragraphSilenceFor(prevGroupCount int) int {
	const sampleRate = 24000

	switch {
	case prevGroupCount <= 1:
		return int(0.85 * float64(sampleRate))
	case prevGroupCount <= 2:
		return int(0.85 * float64(sampleRate))
	case prevGroupCount <= 3:
		return int(0.80 * float64(sampleRate))
	default:
		return int(0.70 * float64(sampleRate))
	}
}

// terminalPunctuation scans backward from the end of a token sequence to find
// the sentence-ending punctuation token (., !, ?, …). Skips closing quotes.
func terminalPunctuation(tokens []int64) int64 {
	const (
		periodToken     int64 = 4
		exclaimToken    int64 = 5
		questionToken   int64 = 6
		ellipsisToken   int64 = 10
		dblQuoteToken   int64 = 11
		rCurlyQuote     int64 = 15
		closeParenToken int64 = 13
	)

	for i := len(tokens) - 1; i >= 0 && i >= len(tokens)-5; i-- {
		tok := tokens[i]
		// Skip trailing quotes/parens
		if tok == dblQuoteToken || tok == rCurlyQuote || tok == closeParenToken {
			continue
		}
		if tok == periodToken || tok == exclaimToken || tok == questionToken || tok == ellipsisToken {
			return tok
		}
		break // stop at first non-quote, non-punctuation token
	}
	return 0
}
