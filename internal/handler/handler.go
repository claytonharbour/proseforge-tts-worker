package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// TTSRequest is the inner payload from RunPod's {"input": ...} wrapper.
type TTSRequest struct {
	Text   string  `json:"text"`
	Voice  string  `json:"voice"`
	Speed  float64 `json:"speed"`
	Format string  `json:"format"`
	// Emphasis, when set, overrides the worker's default emphasis-pause behavior
	// for this request (pointer so an omitted field falls back to the default).
	Emphasis *bool `json:"emphasis,omitempty"`
}

// TTSResponse is the inner payload wrapped in RunPod's {"output": ...}.
type TTSResponse struct {
	Audio           string  `json:"audio"`
	Format          string  `json:"format"`
	SampleRate      int     `json:"sample_rate"`
	DurationSeconds float64 `json:"duration_seconds"`
	Voice           string  `json:"voice"`
}

// runPodRequest is the outer RunPod envelope.
type runPodRequest struct {
	Input TTSRequest `json:"input"`
}

// runPodResponse is the outer RunPod envelope for success.
type runPodResponse struct {
	Output TTSResponse `json:"output"`
}

// errorResponse is the RunPod error envelope.
type errorResponse struct {
	Error string `json:"error"`
}

const (
	DefaultVoice      = "af_sarah"
	DefaultSpeed      = 1.0
	DefaultFormat     = "wav"
	DefaultSampleRate = 24000
)

// TTSFunc synthesizes text to audio. Returns raw audio bytes and duration in seconds.
type TTSFunc func(ctx context.Context, text, voice, format string, speed float64, emphasis *bool) (audioData []byte, durationSeconds float64, err error)

// openAISpeechRequest is the OpenAI-compatible TTS request format.
type openAISpeechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float64 `json:"speed"`
	// Emphasis is a ProseForge extension to the OpenAI contract (#658),
	// mirroring TTSRequest.Emphasis on the RunPod-envelope path: nil = use
	// the worker default (TTS_EMPHASIS), true/false = per-request override.
	Emphasis *bool `json:"emphasis,omitempty"`
}

// Handler holds dependencies for the HTTP handler.
type Handler struct {
	tts        TTSFunc
	voiceNames []string
	metrics    *Metrics
}

// New creates a new Handler with the given TTS function and available voice names.
func New(tts TTSFunc, voiceNames []string) *Handler {
	return &Handler{tts: tts, voiceNames: voiceNames, metrics: NewMetrics()}
}

// SetModelLoaded records whether the real TTS engine initialised, for /metrics (#88).
func (h *Handler) SetModelLoaded(loaded bool) {
	h.metrics.SetModelLoaded(loaded)
}

// RegisterRoutes registers HTTP routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// RunPod contract
	mux.HandleFunc("POST /", h.handleTTS)
	mux.HandleFunc("GET /health", h.handleHealth)

	// OpenAI-compatible endpoints
	mux.HandleFunc("GET /v1/audio/voices", h.handleVoices)
	mux.HandleFunc("POST /v1/audio/speech", h.handleSpeech)

	// Prometheus metrics
	mux.HandleFunc("GET /metrics", h.handleMetrics)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	// Headers are already sent, so there is no status code left to change — but a
	// scrape that dies halfway produces a TRUNCATED exposition body, which
	// Prometheus reports as a parse error against us with nothing on our side
	// saying why. Log it.
	if err := h.metrics.WriteMetrics(w); err != nil {
		log.Printf("metrics: write failed mid-body: %v", err)
	}
}

func (h *Handler) handleTTS(w http.ResponseWriter, r *http.Request) {
	var req runPodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	input := req.Input

	if input.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	// Apply defaults
	if input.Voice == "" {
		input.Voice = DefaultVoice
	}
	if input.Speed <= 0 {
		input.Speed = DefaultSpeed
	}
	if input.Format == "" {
		input.Format = DefaultFormat
	}
	if input.Format != "wav" && input.Format != "mp3" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported format: %q (must be \"wav\" or \"mp3\")", input.Format))
		return
	}

	h.metrics.AddCharacters(input.Voice, len([]rune(input.Text)))

	start := time.Now()
	audioData, duration, err := h.tts(r.Context(), input.Text, input.Voice, input.Format, input.Speed, input.Emphasis)
	elapsed := time.Since(start).Seconds()

	if err != nil {
		h.metrics.RecordRequest(input.Voice, "error")
		log.Printf("TTS error: %v", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("TTS synthesis failed: %v", err))
		return
	}

	h.metrics.RecordRequest(input.Voice, "ok")
	h.metrics.ObserveGeneration(input.Voice, elapsed)
	h.metrics.ObserveAudio(input.Voice, duration)

	resp := runPodResponse{
		Output: TTSResponse{
			Audio:           base64.StdEncoding.EncodeToString(audioData),
			Format:          input.Format,
			SampleRate:      DefaultSampleRate,
			DurationSeconds: duration,
			Voice:           input.Voice,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	// Status and headers are already committed, so a failure here cannot become an
	// HTTP error — the client just receives truncated JSON. Log it rather than
	// discard it: a caller reporting "invalid JSON from the worker" is otherwise
	// unattributable from our side.
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("tts: encoding response failed: %v", err)
	}
}

func (h *Handler) handleVoices(w http.ResponseWriter, r *http.Request) {
	type voice struct {
		Name string `json:"name"`
	}
	voices := make([]voice, len(h.voiceNames))
	for i, name := range h.voiceNames {
		voices[i] = voice{Name: name}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string][]voice{"voices": voices}); err != nil {
		log.Printf("voices: encoding response failed: %v", err)
	}
}

func (h *Handler) handleSpeech(w http.ResponseWriter, r *http.Request) {
	var req openAISpeechRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if req.Input == "" {
		writeError(w, http.StatusBadRequest, "input is required")
		return
	}

	// Apply defaults
	voice := req.Voice
	if voice == "" {
		voice = DefaultVoice
	}
	speed := req.Speed
	if speed <= 0 {
		speed = DefaultSpeed
	}
	format := req.ResponseFormat
	if format == "" {
		format = DefaultFormat
	}
	if format != "wav" && format != "mp3" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported response_format: %q (must be \"wav\" or \"mp3\")", format))
		return
	}

	h.metrics.AddCharacters(voice, len([]rune(req.Input)))

	start := time.Now()
	audioData, duration, err := h.tts(r.Context(), req.Input, voice, format, speed, req.Emphasis)
	elapsed := time.Since(start).Seconds()

	if err != nil {
		h.metrics.RecordRequest(voice, "error")
		log.Printf("TTS error: %v", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("TTS synthesis failed: %v", err))
		return
	}

	h.metrics.RecordRequest(voice, "ok")
	h.metrics.ObserveGeneration(voice, elapsed)
	h.metrics.ObserveAudio(voice, duration)

	// Return raw audio bytes (OpenAI-compatible — no JSON, no base64)
	switch format {
	case "mp3":
		w.Header().Set("Content-Type", "audio/mpeg")
	default:
		w.Header().Set("Content-Type", "audio/wav")
	}
	// A short write here is TRUNCATED AUDIO reaching the caller, which decodes as
	// a clipped file rather than as an error. The byte count is the only thing
	// that distinguishes it, so check both.
	if n, err := w.Write(audioData); err != nil {
		log.Printf("speech: writing audio failed after %d of %d bytes: %v", n, len(audioData), err)
	} else if n < len(audioData) {
		log.Printf("speech: short write, %d of %d bytes", n, len(audioData))
	}
}

// VersionInfo is the GET /version payload.
//
// Data carries a fingerprint of the dictionaries the worker actually LOADED. It is
// the only value here that can move without a rebuild, which is the whole point: a
// data-only deploy replaces JSON and restarts the process, so BuildTime and GitHash
// answer identically before and after the thing that changed (#92). Typed as any so
// this package stays free of a phonemizer dependency, and omitted entirely when the
// worker is running the stub — an absent fingerprint is itself the signal that no
// dictionary is loaded (#88).
type VersionInfo struct {
	BuildTime string `json:"build_time"`
	GitHash   string `json:"git_hash"`
	Data      any    `json:"data,omitempty"`
}

// RegisterVersion adds a GET /version endpoint to the given mux.
func RegisterVersion(mux *http.ServeMux, buildTime, gitHash string, data any) {
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(VersionInfo{
			BuildTime: buildTime,
			GitHash:   gitHash,
			Data:      data,
		}); err != nil {
			log.Printf("version: encoding response failed: %v", err)
		}
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// ⚠️ This is the one place where swallowing the error is worst. If the body
	// fails to write, the client sees the status code with no explanation — so
	// the reason we carefully built into msg is exactly what goes missing, and
	// the caller reports "the worker 400s with an empty body" instead.
	if err := json.NewEncoder(w).Encode(errorResponse{Error: msg}); err != nil {
		log.Printf("writeError: encoding %d response failed, client got a bare status: %v", status, err)
	}
}
