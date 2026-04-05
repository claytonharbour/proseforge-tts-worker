//go:build unit

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubTTS returns a small fake audio payload.
func stubTTS(text, voice, format string, speed float64) ([]byte, float64, error) {
	return []byte("fake-audio-data"), 1.5, nil
}

var testVoiceNames = []string{"af_sarah", "bf_emma"}

func newTestHandler() *Handler {
	return New(stubTTS, testVoiceNames)
}

func doPost(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestHealthCheck(t *testing.T) {
	h := newTestHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("health check: got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestValidRequest(t *testing.T) {
	h := newTestHandler()
	w := doPost(t, h, `{"input":{"text":"Hello world","voice":"af_sarah","speed":1.0,"format":"wav"}}`)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp runPodResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Output.Audio == "" {
		t.Error("audio field is empty")
	}
	if resp.Output.Format != "wav" {
		t.Errorf("format: got %q, want %q", resp.Output.Format, "wav")
	}
	if resp.Output.SampleRate != DefaultSampleRate {
		t.Errorf("sample_rate: got %d, want %d", resp.Output.SampleRate, DefaultSampleRate)
	}
	if resp.Output.DurationSeconds != 1.5 {
		t.Errorf("duration_seconds: got %f, want %f", resp.Output.DurationSeconds, 1.5)
	}
	if resp.Output.Voice != "af_sarah" {
		t.Errorf("voice: got %q, want %q", resp.Output.Voice, "af_sarah")
	}
}

func TestDefaultValues(t *testing.T) {
	h := newTestHandler()
	// Only text provided — voice, speed, format should use defaults
	w := doPost(t, h, `{"input":{"text":"Hello"}}`)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp runPodResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Output.Voice != DefaultVoice {
		t.Errorf("default voice: got %q, want %q", resp.Output.Voice, DefaultVoice)
	}
	if resp.Output.Format != DefaultFormat {
		t.Errorf("default format: got %q, want %q", resp.Output.Format, DefaultFormat)
	}
}

func TestMissingText(t *testing.T) {
	h := newTestHandler()
	w := doPost(t, h, `{"input":{}}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp errorResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Error("expected error message, got empty")
	}
}

func TestEmptyText(t *testing.T) {
	h := newTestHandler()
	w := doPost(t, h, `{"input":{"text":""}}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestInvalidJSON(t *testing.T) {
	h := newTestHandler()
	w := doPost(t, h, `not json`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUnsupportedFormat(t *testing.T) {
	h := newTestHandler()
	w := doPost(t, h, `{"input":{"text":"Hello","format":"ogg"}}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMP3Format(t *testing.T) {
	h := newTestHandler()
	w := doPost(t, h, `{"input":{"text":"Hello","format":"mp3"}}`)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp runPodResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Output.Format != "mp3" {
		t.Errorf("format: got %q, want %q", resp.Output.Format, "mp3")
	}
}

func TestNegativeSpeed(t *testing.T) {
	h := newTestHandler()
	w := doPost(t, h, `{"input":{"text":"Hello","speed":-1}}`)

	if w.Code != http.StatusOK {
		t.Fatalf("negative speed should default to 1.0, got status %d", w.Code)
	}
}

// --- OpenAI-compatible endpoint tests ---

func doGet(t *testing.T, h *Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func doPostPath(t *testing.T, h *Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestVoicesEndpoint(t *testing.T) {
	h := newTestHandler()
	w := doGet(t, h, "/v1/audio/voices")

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Voices []struct {
			Name string `json:"name"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Voices) != 2 {
		t.Errorf("expected 2 voices, got %d: %v", len(resp.Voices), resp.Voices)
	}
	if resp.Voices[0].Name != "af_sarah" || resp.Voices[1].Name != "bf_emma" {
		t.Errorf("unexpected voices: %v", resp.Voices)
	}
}

func TestSpeechEndpoint(t *testing.T) {
	h := newTestHandler()
	w := doPostPath(t, h, "/v1/audio/speech", `{"model":"kokoro-v1.0","input":"Hello world","voice":"af_sarah","response_format":"wav"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Response should be raw audio bytes, not JSON
	ct := w.Header().Get("Content-Type")
	if ct != "audio/wav" {
		t.Errorf("Content-Type: got %q, want %q", ct, "audio/wav")
	}

	// Body should be the raw audio (from stub: "fake-audio-data")
	if w.Body.Len() == 0 {
		t.Error("response body is empty")
	}
}

func TestSpeechEndpointMP3(t *testing.T) {
	h := newTestHandler()
	w := doPostPath(t, h, "/v1/audio/speech", `{"model":"kokoro-v1.0","input":"Hello","response_format":"mp3"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "audio/mpeg" {
		t.Errorf("Content-Type: got %q, want %q", ct, "audio/mpeg")
	}
}

func TestSpeechEndpointMissingInput(t *testing.T) {
	h := newTestHandler()
	w := doPostPath(t, h, "/v1/audio/speech", `{"model":"kokoro-v1.0"}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSpeechEndpointDefaults(t *testing.T) {
	h := newTestHandler()
	// Only input provided — voice, format, speed should use defaults
	w := doPostPath(t, h, "/v1/audio/speech", `{"input":"Hello"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "audio/wav" {
		t.Errorf("Content-Type: got %q, want %q (default format)", ct, "audio/wav")
	}
}

// --- Metrics endpoint tests ---

func TestMetricsEndpoint(t *testing.T) {
	h := newTestHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Make a RunPod TTS request.
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"input":{"text":"Hello world","voice":"af_sarah"}}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(httptest.NewRecorder(), req)

	// Make an OpenAI speech request.
	req = httptest.NewRequest("POST", "/v1/audio/speech", bytes.NewBufferString(`{"input":"Testing","voice":"bf_emma"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(httptest.NewRecorder(), req)

	// Fetch metrics.
	w := httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/metrics", nil)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("metrics: got status %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Errorf("Content-Type: got %q, want prometheus text format", ct)
	}

	out := w.Body.String()

	// Verify both requests were counted.
	if !strings.Contains(out, `kokoro_tts_requests_total{voice="af_sarah",status="ok"} 1`) {
		t.Errorf("missing af_sarah request count in:\n%s", out)
	}
	if !strings.Contains(out, `kokoro_tts_requests_total{voice="bf_emma",status="ok"} 1`) {
		t.Errorf("missing bf_emma request count in:\n%s", out)
	}

	// Verify characters were counted ("Hello world" = 11 runes, "Testing" = 7 runes).
	if !strings.Contains(out, `kokoro_tts_characters_total{voice="af_sarah"} 11`) {
		t.Errorf("missing af_sarah character count in:\n%s", out)
	}
	if !strings.Contains(out, `kokoro_tts_characters_total{voice="bf_emma"} 7`) {
		t.Errorf("missing bf_emma character count in:\n%s", out)
	}

	// Verify histogram data exists.
	if !strings.Contains(out, "kokoro_tts_generation_seconds_count") {
		t.Errorf("missing generation histogram in:\n%s", out)
	}
	if !strings.Contains(out, "kokoro_tts_audio_seconds_count") {
		t.Errorf("missing audio histogram in:\n%s", out)
	}

	// Verify model loaded gauge.
	if !strings.Contains(out, "kokoro_tts_model_loaded 1") {
		t.Errorf("missing model_loaded gauge in:\n%s", out)
	}
}
