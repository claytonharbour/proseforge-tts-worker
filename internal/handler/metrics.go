package handler

import (
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
)

// histogram buckets matching the Python Kokoro-TTS server.
var (
	generationBuckets = []float64{0.1, 0.25, 0.5, 1, 2, 3, 5, 8, 13, 21}
	audioBuckets      = []float64{0.5, 1, 2, 3, 5, 8, 13, 21, 30, 60}
)

type voiceStatus struct {
	voice  string
	status string
}

type histogram struct {
	buckets []float64
	counts  []uint64 // one per bucket
	sum     float64
	count   uint64
}

func newHistogram(buckets []float64) *histogram {
	return &histogram{
		buckets: buckets,
		counts:  make([]uint64, len(buckets)),
	}
}

func (h *histogram) observe(v float64) {
	for i, bound := range h.buckets {
		if v <= bound {
			h.counts[i]++
		}
	}
	h.sum += v
	h.count++
}

// Metrics collects Prometheus-compatible TTS metrics without external dependencies.
type Metrics struct {
	mu              sync.Mutex
	requestsTotal   map[voiceStatus]uint64
	generationSecs  map[string]*histogram
	audioSecs       map[string]*histogram
	charactersTotal map[string]uint64
}

// NewMetrics creates an initialized Metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		requestsTotal:   make(map[voiceStatus]uint64),
		generationSecs:  make(map[string]*histogram),
		audioSecs:       make(map[string]*histogram),
		charactersTotal: make(map[string]uint64),
	}
}

// RecordRequest increments the requests counter for the given voice and status.
func (m *Metrics) RecordRequest(voice, status string) {
	m.mu.Lock()
	m.requestsTotal[voiceStatus{voice, status}]++
	m.mu.Unlock()
}

// ObserveGeneration records a TTS generation duration in seconds.
func (m *Metrics) ObserveGeneration(voice string, seconds float64) {
	m.mu.Lock()
	h, ok := m.generationSecs[voice]
	if !ok {
		h = newHistogram(generationBuckets)
		m.generationSecs[voice] = h
	}
	h.observe(seconds)
	m.mu.Unlock()
}

// ObserveAudio records the duration of generated audio in seconds.
func (m *Metrics) ObserveAudio(voice string, seconds float64) {
	m.mu.Lock()
	h, ok := m.audioSecs[voice]
	if !ok {
		h = newHistogram(audioBuckets)
		m.audioSecs[voice] = h
	}
	h.observe(seconds)
	m.mu.Unlock()
}

// AddCharacters increments the characters counter for the given voice.
func (m *Metrics) AddCharacters(voice string, count int) {
	m.mu.Lock()
	m.charactersTotal[voice] += uint64(count)
	m.mu.Unlock()
}

// WriteTo serializes all metrics in Prometheus text exposition format.
func (m *Metrics) WriteTo(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// kokoro_tts_model_loaded — always 1
	fmt.Fprintln(w, "# HELP kokoro_tts_model_loaded Whether the TTS model is loaded.")
	fmt.Fprintln(w, "# TYPE kokoro_tts_model_loaded gauge")
	fmt.Fprintln(w, "kokoro_tts_model_loaded 1")

	// kokoro_tts_requests_total
	fmt.Fprintln(w, "# HELP kokoro_tts_requests_total Total number of TTS requests.")
	fmt.Fprintln(w, "# TYPE kokoro_tts_requests_total counter")
	writeCounter(w, "kokoro_tts_requests_total", m.requestsTotal)

	// kokoro_tts_characters_total
	fmt.Fprintln(w, "# HELP kokoro_tts_characters_total Total characters processed.")
	fmt.Fprintln(w, "# TYPE kokoro_tts_characters_total counter")
	writeCounterByVoice(w, "kokoro_tts_characters_total", m.charactersTotal)

	// kokoro_tts_generation_seconds
	fmt.Fprintln(w, "# HELP kokoro_tts_generation_seconds TTS generation duration in seconds.")
	fmt.Fprintln(w, "# TYPE kokoro_tts_generation_seconds histogram")
	writeHistogram(w, "kokoro_tts_generation_seconds", m.generationSecs)

	// kokoro_tts_audio_seconds
	fmt.Fprintln(w, "# HELP kokoro_tts_audio_seconds Duration of generated audio in seconds.")
	fmt.Fprintln(w, "# TYPE kokoro_tts_audio_seconds histogram")
	writeHistogram(w, "kokoro_tts_audio_seconds", m.audioSecs)
}

func writeCounter(w io.Writer, name string, data map[voiceStatus]uint64) {
	keys := make([]voiceStatus, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].voice != keys[j].voice {
			return keys[i].voice < keys[j].voice
		}
		return keys[i].status < keys[j].status
	})
	for _, k := range keys {
		fmt.Fprintf(w, "%s{voice=%q,status=%q} %d\n", name, k.voice, k.status, data[k])
	}
}

func writeCounterByVoice(w io.Writer, name string, data map[string]uint64) {
	voices := sortedKeys(data)
	for _, v := range voices {
		fmt.Fprintf(w, "%s{voice=%q} %d\n", name, v, data[v])
	}
}

func writeHistogram(w io.Writer, name string, data map[string]*histogram) {
	voices := sortedHistogramKeys(data)
	for _, voice := range voices {
		h := data[voice]
		for i, bound := range h.buckets {
			fmt.Fprintf(w, "%s_bucket{voice=%q,le=%q} %d\n", name, voice, formatFloat(bound), h.counts[i])
		}
		fmt.Fprintf(w, "%s_bucket{voice=%q,le=\"+Inf\"} %d\n", name, voice, h.count)
		fmt.Fprintf(w, "%s_sum{voice=%q} %s\n", name, voice, formatFloat(h.sum))
		fmt.Fprintf(w, "%s_count{voice=%q} %d\n", name, voice, h.count)
	}
}

func sortedKeys(m map[string]uint64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedHistogramKeys(m map[string]*histogram) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// formatFloat formats a float for Prometheus output. Integers are rendered without
// a decimal point (e.g. "1" not "1.000000"), while fractional values keep minimal
// precision.
func formatFloat(v float64) string {
	if v == math.Trunc(v) && !math.IsInf(v, 0) {
		return fmt.Sprintf("%g", v)
	}
	return fmt.Sprintf("%g", v)
}
