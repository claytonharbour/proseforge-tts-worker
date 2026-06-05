package runpod

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/claytonharbour/proseforge-tts-worker/internal/handler"
)

// Config holds RunPod webhook URLs parsed from env vars.
type Config struct {
	GetJobURL     string        // RUNPOD_WEBHOOK_GET_JOB
	PostOutputURL string        // RUNPOD_WEBHOOK_POST_OUTPUT (contains $ID)
	PingURL       string        // RUNPOD_WEBHOOK_PING
	PingInterval  time.Duration // RUNPOD_PING_INTERVAL (ms, default 10s)
	PodID         string        // RUNPOD_POD_ID
	APIKey        string        // RUNPOD_AI_API_KEY
}

const defaultPingInterval = 10 * time.Second

// DetectConfig reads RunPod env vars. Returns nil if not running on RunPod.
func DetectConfig() *Config {
	getJob := os.Getenv("RUNPOD_WEBHOOK_GET_JOB")
	if getJob == "" {
		return nil
	}

	pingInterval := defaultPingInterval
	if ms, err := strconv.Atoi(os.Getenv("RUNPOD_PING_INTERVAL")); err == nil && ms > 0 {
		pingInterval = time.Duration(ms) * time.Millisecond
	}

	return &Config{
		GetJobURL:     getJob,
		PostOutputURL: os.Getenv("RUNPOD_WEBHOOK_POST_OUTPUT"),
		PingURL:       os.Getenv("RUNPOD_WEBHOOK_PING"),
		PingInterval:  pingInterval,
		PodID:         os.Getenv("RUNPOD_POD_ID"),
		APIKey:        os.Getenv("RUNPOD_AI_API_KEY"),
	}
}

// job represents a RunPod job received from the polling endpoint.
type job struct {
	ID    string          `json:"id"`
	Input json.RawMessage `json:"input"`
}

// ttsInput is the expected input payload for a TTS job.
type ttsInput struct {
	Text   string  `json:"text"`
	Voice  string  `json:"voice"`
	Speed  float64 `json:"speed"`
	Format string  `json:"format"`
}

// ttsOutput is the response payload for a successful TTS job.
type ttsOutput struct {
	Audio           string  `json:"audio"`
	Format          string  `json:"format"`
	SampleRate      int     `json:"sample_rate"`
	DurationSeconds float64 `json:"duration_seconds"`
	Voice           string  `json:"voice"`
}

const (
	defaultVoice      = "af_sarah"
	defaultSpeed      = 1.0
	defaultFormat     = "wav"
	defaultSampleRate = 24000
	pollTimeout       = 10 * time.Second
	postTimeout       = 120 * time.Second
	retryDelay        = 250 * time.Millisecond
)

// Worker polls RunPod for jobs and processes them.
type Worker struct {
	tts          handler.TTSFunc
	config       *Config
	client       *http.Client // polling + pings (short timeout)
	postClient   *http.Client // result POST (long timeout for large payloads)
	currentJobID string       // tracked so heartbeat pings include it
}

// New creates a new RunPod polling worker.
func New(tts handler.TTSFunc, config *Config) *Worker {
	return &Worker{
		tts:        tts,
		config:     config,
		client:     &http.Client{Timeout: pollTimeout},
		postClient: &http.Client{Timeout: postTimeout},
	}
}

// Run enters the polling loop. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		j, err := w.getJob(ctx)
		if err != nil {
			log.Printf("poll error: %v", err)
			sleep(ctx, retryDelay)
			continue
		}
		if j == nil {
			// 204 No Content — no jobs available
			sleep(ctx, retryDelay)
			continue
		}

		log.Printf("processing job %s", j.ID)
		w.processJob(j)
	}
}

// getJob long-polls RunPod for a job. Returns nil for 204 No Content.
func (w *Worker) getJob(ctx context.Context) (*job, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.config.GetJobURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if w.config.APIKey != "" {
		req.Header.Set("Authorization", w.config.APIKey)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var j job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, fmt.Errorf("decode job: %w", err)
	}
	return &j, nil
}

// processJob runs TTS for a job and posts the result.
func (w *Worker) processJob(j *job) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track current job so heartbeat pings include the job ID.
	w.currentJobID = j.ID
	defer func() { w.currentJobID = "" }()

	// Start heartbeat pings so RunPod knows we're alive and which job we're processing.
	if w.config.PingURL != "" {
		log.Printf("job %s: heartbeat started (interval=%s)", j.ID, w.config.PingInterval)
		go w.heartbeat(ctx)
	}

	var input ttsInput
	if err := json.Unmarshal(j.Input, &input); err != nil {
		w.postError(j.ID, fmt.Sprintf("invalid input: %v", err))
		return
	}

	if input.Text == "" {
		w.postError(j.ID, "text is required")
		return
	}
	if input.Voice == "" {
		input.Voice = defaultVoice
	}
	if input.Speed <= 0 {
		input.Speed = defaultSpeed
	}
	if input.Format == "" {
		input.Format = defaultFormat
	}
	if input.Format != "wav" && input.Format != "mp3" {
		w.postError(j.ID, fmt.Sprintf("unsupported format: %q (must be \"wav\" or \"mp3\")", input.Format))
		return
	}

	audioData, duration, err := w.tts(input.Text, input.Voice, input.Format, input.Speed)
	if err != nil {
		log.Printf("TTS error for job %s: %v", j.ID, err)
		w.postError(j.ID, fmt.Sprintf("TTS synthesis failed: %v", err))
		return
	}

	b64Audio := base64.StdEncoding.EncodeToString(audioData)
	log.Printf("job %s: audio=%d bytes, base64=%d bytes, duration=%.1fs",
		j.ID, len(audioData), len(b64Audio), duration)

	output := ttsOutput{
		Audio:           b64Audio,
		Format:          input.Format,
		SampleRate:      defaultSampleRate,
		DurationSeconds: duration,
		Voice:           input.Voice,
	}
	w.postOutput(j.ID, output)
}

// heartbeat sends periodic pings to RunPod until ctx is cancelled.
func (w *Worker) heartbeat(ctx context.Context) {
	interval := w.config.PingInterval
	if interval <= 0 {
		interval = defaultPingInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.ping()
		}
	}
}

// ping sends a single heartbeat GET to RunPod's ping webhook.
// Includes the current job ID so RunPod knows which job is in progress.
func (w *Worker) ping() {
	pingURL := w.config.PingURL
	if jobID := w.currentJobID; jobID != "" {
		sep := "?"
		if strings.Contains(pingURL, "?") {
			sep = "&"
		}
		pingURL += sep + "job_id=" + jobID
	}

	req, err := http.NewRequest(http.MethodGet, pingURL, nil)
	if err != nil {
		log.Printf("ping: build request: %v", err)
		return
	}
	if w.config.APIKey != "" {
		req.Header.Set("Authorization", w.config.APIKey)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		log.Printf("ping: %v", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}

// postOutput sends a successful result to RunPod.
func (w *Worker) postOutput(jobID string, output ttsOutput) {
	body, _ := json.Marshal(map[string]any{"output": output})
	w.postResult(jobID, body)
}

// postError sends an error result to RunPod.
func (w *Worker) postError(jobID string, msg string) {
	log.Printf("job %s error: %s", jobID, msg)
	body, _ := json.Marshal(map[string]string{"error": msg})
	w.postResult(jobID, body)
}

// postResult POSTs a result payload to RunPod, replacing $ID in the URL.
// Appends isStream=false to match the RunPod Python SDK protocol.
func (w *Worker) postResult(jobID string, body []byte) {
	base := strings.Replace(w.config.PostOutputURL, "$ID", jobID, 1)
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	url := base + sep + "isStream=false"

	payloadMB := float64(len(body)) / (1024 * 1024)
	log.Printf("POST result for job %s: payload=%.2f MB", jobID, payloadMB)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("POST result for job %s: build request: %v", jobID, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if w.config.APIKey != "" {
		req.Header.Set("Authorization", w.config.APIKey)
	}

	start := time.Now()
	resp, err := w.postClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		log.Printf("POST result for job %s: %v (after %s)", jobID, err, elapsed.Round(time.Millisecond))
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK {
		log.Printf("POST result for job %s: status %d, body: %s (%.2f MB, %s)",
			jobID, resp.StatusCode, string(respBody), payloadMB, elapsed.Round(time.Millisecond))
	} else {
		log.Printf("POST result for job %s: %d OK, body: %s (%.2f MB, %s)",
			jobID, resp.StatusCode, string(respBody), payloadMB, elapsed.Round(time.Millisecond))
	}
}

// sleep waits for the given duration or until ctx is cancelled.
func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
