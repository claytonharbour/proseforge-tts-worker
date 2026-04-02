//go:build unit

package runpod

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// stubTTS returns fake audio data and a fixed duration.
func stubTTS(text, voice, format string, speed float64) ([]byte, float64, error) {
	return []byte("fake-audio-data"), 1.5, nil
}

// failTTS always returns an error.
func failTTS(text, voice, format string, speed float64) ([]byte, float64, error) {
	return nil, 0, fmt.Errorf("synthesis failed")
}

// mockRunPod simulates RunPod's polling endpoints.
type mockRunPod struct {
	mu        sync.Mutex
	jobs      []job
	results   []postedResult
	pingCount int
}

type postedResult struct {
	Path string
	Body map[string]json.RawMessage
}

func (m *mockRunPod) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/get-job", m.handleGetJob)
	mux.HandleFunc("/post-output/", m.handlePostOutput)
	mux.HandleFunc("/ping", m.handlePing)
	return mux
}

func (m *mockRunPod) handlePing(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pingCount++
	w.WriteHeader(http.StatusOK)
}

func (m *mockRunPod) getPingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pingCount
}

func (m *mockRunPod) handleGetJob(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.jobs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	j := m.jobs[0]
	m.jobs = m.jobs[1:]
	json.NewEncoder(w).Encode(j)
}

func (m *mockRunPod) handlePostOutput(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var body map[string]json.RawMessage
	json.NewDecoder(r.Body).Decode(&body)
	m.results = append(m.results, postedResult{
		Path: r.URL.Path,
		Body: body,
	})
	w.WriteHeader(http.StatusOK)
}

func (m *mockRunPod) getResults() []postedResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]postedResult, len(m.results))
	copy(cp, m.results)
	return cp
}

func TestProcessJobSuccess(t *testing.T) {
	inputJSON, _ := json.Marshal(ttsInput{
		Text:  "Hello world",
		Voice: "af_sarah",
		Speed: 1.0,
	})

	mock := &mockRunPod{
		jobs: []job{
			{ID: "job-123", Input: inputJSON},
		},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	cfg := &Config{
		GetJobURL:     srv.URL + "/get-job",
		PostOutputURL: srv.URL + "/post-output/$ID",
		PodID:         "test-pod",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w := New(stubTTS, cfg)
	// Run will process the job then get 204 and keep looping until ctx cancels
	go w.Run(ctx)

	// Wait for the result to appear
	var results []postedResult
	deadline := time.After(2 * time.Second)
	for {
		results = mock.getResults()
		if len(results) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for result")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Verify the POST path has $ID replaced
	if results[0].Path != "/post-output/job-123" {
		t.Errorf("path: got %q, want %q", results[0].Path, "/post-output/job-123")
	}

	// Verify output envelope
	outputRaw, ok := results[0].Body["output"]
	if !ok {
		t.Fatal("missing 'output' key in result")
	}

	var output ttsOutput
	if err := json.Unmarshal(outputRaw, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	// Verify base64 audio
	decoded, err := base64.StdEncoding.DecodeString(output.Audio)
	if err != nil {
		t.Fatalf("audio is not valid base64: %v", err)
	}
	if string(decoded) != "fake-audio-data" {
		t.Errorf("audio: got %q, want %q", string(decoded), "fake-audio-data")
	}

	if output.Format != "wav" {
		t.Errorf("format: got %q, want %q", output.Format, "wav")
	}
	if output.SampleRate != defaultSampleRate {
		t.Errorf("sample_rate: got %d, want %d", output.SampleRate, defaultSampleRate)
	}
	if output.DurationSeconds != 1.5 {
		t.Errorf("duration: got %f, want %f", output.DurationSeconds, 1.5)
	}
	if output.Voice != "af_sarah" {
		t.Errorf("voice: got %q, want %q", output.Voice, "af_sarah")
	}
}

func TestProcessJobDefaults(t *testing.T) {
	// Only text provided — voice, speed, format should use defaults
	inputJSON, _ := json.Marshal(map[string]string{"text": "Hello"})

	mock := &mockRunPod{
		jobs: []job{
			{ID: "job-defaults", Input: inputJSON},
		},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	cfg := &Config{
		GetJobURL:     srv.URL + "/get-job",
		PostOutputURL: srv.URL + "/post-output/$ID",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w := New(stubTTS, cfg)
	go w.Run(ctx)

	var results []postedResult
	deadline := time.After(2 * time.Second)
	for {
		results = mock.getResults()
		if len(results) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for result")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	outputRaw := results[0].Body["output"]
	var output ttsOutput
	json.Unmarshal(outputRaw, &output)

	if output.Voice != defaultVoice {
		t.Errorf("default voice: got %q, want %q", output.Voice, defaultVoice)
	}
	if output.Format != defaultFormat {
		t.Errorf("default format: got %q, want %q", output.Format, defaultFormat)
	}
}

func TestProcessJobMissingText(t *testing.T) {
	inputJSON, _ := json.Marshal(map[string]string{})

	mock := &mockRunPod{
		jobs: []job{
			{ID: "job-notext", Input: inputJSON},
		},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	cfg := &Config{
		GetJobURL:     srv.URL + "/get-job",
		PostOutputURL: srv.URL + "/post-output/$ID",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w := New(stubTTS, cfg)
	go w.Run(ctx)

	var results []postedResult
	deadline := time.After(2 * time.Second)
	for {
		results = mock.getResults()
		if len(results) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for result")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Should post an error, not output
	if _, ok := results[0].Body["error"]; !ok {
		t.Error("expected 'error' key in result for missing text")
	}
	if _, ok := results[0].Body["output"]; ok {
		t.Error("should not have 'output' key for missing text")
	}
}

func TestProcessJobTTSError(t *testing.T) {
	inputJSON, _ := json.Marshal(ttsInput{Text: "Hello"})

	mock := &mockRunPod{
		jobs: []job{
			{ID: "job-fail", Input: inputJSON},
		},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	cfg := &Config{
		GetJobURL:     srv.URL + "/get-job",
		PostOutputURL: srv.URL + "/post-output/$ID",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w := New(failTTS, cfg)
	go w.Run(ctx)

	var results []postedResult
	deadline := time.After(2 * time.Second)
	for {
		results = mock.getResults()
		if len(results) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for result")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Should post an error
	errRaw, ok := results[0].Body["error"]
	if !ok {
		t.Fatal("expected 'error' key in result for TTS failure")
	}
	var errMsg string
	json.Unmarshal(errRaw, &errMsg)
	if errMsg == "" {
		t.Error("error message should not be empty")
	}
}

func TestProcessJobUnsupportedFormat(t *testing.T) {
	inputJSON, _ := json.Marshal(ttsInput{Text: "Hello", Format: "ogg"})

	mock := &mockRunPod{
		jobs: []job{
			{ID: "job-badformat", Input: inputJSON},
		},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	cfg := &Config{
		GetJobURL:     srv.URL + "/get-job",
		PostOutputURL: srv.URL + "/post-output/$ID",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w := New(stubTTS, cfg)
	go w.Run(ctx)

	var results []postedResult
	deadline := time.After(2 * time.Second)
	for {
		results = mock.getResults()
		if len(results) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for result")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if _, ok := results[0].Body["error"]; !ok {
		t.Error("expected 'error' for unsupported format")
	}
}

func TestMultipleJobs(t *testing.T) {
	input1, _ := json.Marshal(ttsInput{Text: "First"})
	input2, _ := json.Marshal(ttsInput{Text: "Second"})

	mock := &mockRunPod{
		jobs: []job{
			{ID: "job-1", Input: input1},
			{ID: "job-2", Input: input2},
		},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	cfg := &Config{
		GetJobURL:     srv.URL + "/get-job",
		PostOutputURL: srv.URL + "/post-output/$ID",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	w := New(stubTTS, cfg)
	go w.Run(ctx)

	var results []postedResult
	deadline := time.After(3 * time.Second)
	for {
		results = mock.getResults()
		if len(results) >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: got %d results, want 2", len(results))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if results[0].Path != "/post-output/job-1" {
		t.Errorf("first result path: got %q, want %q", results[0].Path, "/post-output/job-1")
	}
	if results[1].Path != "/post-output/job-2" {
		t.Errorf("second result path: got %q, want %q", results[1].Path, "/post-output/job-2")
	}
}

func TestDetectConfigPresent(t *testing.T) {
	t.Setenv("RUNPOD_WEBHOOK_GET_JOB", "http://example.com/get")
	t.Setenv("RUNPOD_WEBHOOK_POST_OUTPUT", "http://example.com/post/$ID")
	t.Setenv("RUNPOD_WEBHOOK_PING", "http://example.com/ping")
	t.Setenv("RUNPOD_POD_ID", "pod-abc")

	cfg := DetectConfig()
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.GetJobURL != "http://example.com/get" {
		t.Errorf("GetJobURL: got %q", cfg.GetJobURL)
	}
	if cfg.PostOutputURL != "http://example.com/post/$ID" {
		t.Errorf("PostOutputURL: got %q", cfg.PostOutputURL)
	}
	if cfg.PingURL != "http://example.com/ping" {
		t.Errorf("PingURL: got %q", cfg.PingURL)
	}
	if cfg.PodID != "pod-abc" {
		t.Errorf("PodID: got %q", cfg.PodID)
	}
}

func TestDetectConfigAbsent(t *testing.T) {
	t.Setenv("RUNPOD_WEBHOOK_GET_JOB", "")

	cfg := DetectConfig()
	if cfg != nil {
		t.Errorf("expected nil config when env var absent, got %+v", cfg)
	}
}

func TestDetectConfigPingInterval(t *testing.T) {
	t.Setenv("RUNPOD_WEBHOOK_GET_JOB", "http://example.com/get")
	t.Setenv("RUNPOD_PING_INTERVAL", "4000")

	cfg := DetectConfig()
	if cfg.PingInterval != 4*time.Second {
		t.Errorf("PingInterval: got %v, want %v", cfg.PingInterval, 4*time.Second)
	}
}

func TestDetectConfigPingIntervalDefault(t *testing.T) {
	t.Setenv("RUNPOD_WEBHOOK_GET_JOB", "http://example.com/get")
	// RUNPOD_PING_INTERVAL not set

	cfg := DetectConfig()
	if cfg.PingInterval != defaultPingInterval {
		t.Errorf("PingInterval: got %v, want %v", cfg.PingInterval, defaultPingInterval)
	}
}

func TestHeartbeatPingsDuringSlowJob(t *testing.T) {
	// slowTTS takes 300ms, giving the 50ms heartbeat time to fire multiple pings.
	slowTTS := func(text, voice, format string, speed float64) ([]byte, float64, error) {
		time.Sleep(300 * time.Millisecond)
		return []byte("audio"), 1.0, nil
	}

	inputJSON, _ := json.Marshal(ttsInput{Text: "Hello"})
	mock := &mockRunPod{
		jobs: []job{{ID: "job-slow", Input: inputJSON}},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	cfg := &Config{
		GetJobURL:     srv.URL + "/get-job",
		PostOutputURL: srv.URL + "/post-output/$ID",
		PingURL:       srv.URL + "/ping",
		PingInterval:  50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w := New(slowTTS, cfg)
	go w.Run(ctx)

	// Wait for the result
	deadline := time.After(2 * time.Second)
	for {
		results := mock.getResults()
		if len(results) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for result")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	pings := mock.getPingCount()
	if pings < 2 {
		t.Errorf("expected at least 2 heartbeat pings during 300ms job (at 50ms interval), got %d", pings)
	}
}

func TestNoPingsWithoutPingURL(t *testing.T) {
	// When PingURL is empty, no pings should be sent (local dev mode).
	slowTTS := func(text, voice, format string, speed float64) ([]byte, float64, error) {
		time.Sleep(100 * time.Millisecond)
		return []byte("audio"), 1.0, nil
	}

	inputJSON, _ := json.Marshal(ttsInput{Text: "Hello"})
	mock := &mockRunPod{
		jobs: []job{{ID: "job-noping", Input: inputJSON}},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	cfg := &Config{
		GetJobURL:     srv.URL + "/get-job",
		PostOutputURL: srv.URL + "/post-output/$ID",
		// PingURL intentionally empty
		PingInterval: 20 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w := New(slowTTS, cfg)
	go w.Run(ctx)

	deadline := time.After(2 * time.Second)
	for {
		results := mock.getResults()
		if len(results) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for result")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if pings := mock.getPingCount(); pings != 0 {
		t.Errorf("expected 0 pings when PingURL is empty, got %d", pings)
	}
}

func TestContextCancellation(t *testing.T) {
	mock := &mockRunPod{} // no jobs — will always return 204
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	cfg := &Config{
		GetJobURL:     srv.URL + "/get-job",
		PostOutputURL: srv.URL + "/post-output/$ID",
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := New(stubTTS, cfg)

	done := make(chan error, 1)
	go func() {
		done <- w.Run(ctx)
	}()

	// Cancel after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancellation")
	}
}
