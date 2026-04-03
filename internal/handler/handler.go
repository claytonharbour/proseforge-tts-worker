package handler

import (
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
type TTSFunc func(text, voice, format string, speed float64) (audioData []byte, durationSeconds float64, err error)

// openAISpeechRequest is the OpenAI-compatible TTS request format.
type openAISpeechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float64 `json:"speed"`
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
	h.metrics.WriteTo(w)
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
	audioData, duration, err := h.tts(input.Text, input.Voice, input.Format, input.Speed)
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
	json.NewEncoder(w).Encode(resp)
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
	json.NewEncoder(w).Encode(map[string][]voice{"voices": voices})
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
	audioData, duration, err := h.tts(req.Input, voice, format, speed)
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
	w.Write(audioData)
}

// RegisterVersion adds a GET /version endpoint to the given mux.
func RegisterVersion(mux *http.ServeMux, buildTime, gitHash string) {
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"build_time": buildTime,
			"git_hash":   gitHash,
		})
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Error: msg})
}
