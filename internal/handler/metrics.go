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
	modelLoaded     bool
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

// SetModelLoaded records whether the real TTS engine initialised (#88).
func (m *Metrics) SetModelLoaded(loaded bool) {
	m.mu.Lock()
	m.modelLoaded = loaded
	m.mu.Unlock()
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

// metricsWriter latches the FIRST write error and keeps going. Prometheus
// exposition is one flat text body, so there is no partial-write recovery worth
// attempting mid-stream — but the error must reach the caller rather than being
// dropped at ~15 separate Fprint sites.
//
// 📌 The println/printf helpers exist so the call sites do not each need a blank
// assignment to satisfy errcheck. A `_, _ =` per line is noise that trains the
// reader to skip exactly the lines that would carry a real error.
type metricsWriter struct {
	w   io.Writer
	err error
}

func (e *metricsWriter) println(s string) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintln(e.w, s)
}

func (e *metricsWriter) printf(format string, a ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, a...)
}

// WriteMetrics serializes all metrics in Prometheus text exposition format,
// returning the first write error encountered.
//
// ⚠️ NOT named WriteTo. It was, and `go vet`'s stdmethods flagged it: a method
// called WriteTo is expected to be io.WriterTo, which returns (int64, error).
// This one returned nothing, so it CLAIMED a standard interface it did not
// implement — a reader (or a later refactor reaching for io.Copy) would find the
// name promising semantics the body never had.
func (m *Metrics) WriteMetrics(w io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e := &metricsWriter{w: w}

	// kokoro_tts_model_loaded — 1 only when the real engine initialised (#88). It
	// was the literal 1, so a worker serving silent stub audio reported a loaded
	// model to anything scraping it.
	loaded := 0
	if m.modelLoaded {
		loaded = 1
	}
	e.println("# HELP kokoro_tts_model_loaded Whether the TTS model is loaded.")
	e.println("# TYPE kokoro_tts_model_loaded gauge")
	e.printf("kokoro_tts_model_loaded %d\n", loaded)

	// kokoro_tts_requests_total
	e.println("# HELP kokoro_tts_requests_total Total number of TTS requests.")
	e.println("# TYPE kokoro_tts_requests_total counter")
	writeCounter(e, "kokoro_tts_requests_total", m.requestsTotal)

	// kokoro_tts_characters_total
	e.println("# HELP kokoro_tts_characters_total Total characters processed.")
	e.println("# TYPE kokoro_tts_characters_total counter")
	writeCounterByVoice(e, "kokoro_tts_characters_total", m.charactersTotal)

	// kokoro_tts_generation_seconds
	e.println("# HELP kokoro_tts_generation_seconds TTS generation duration in seconds.")
	e.println("# TYPE kokoro_tts_generation_seconds histogram")
	writeHistogram(e, "kokoro_tts_generation_seconds", m.generationSecs)

	// kokoro_tts_audio_seconds
	e.println("# HELP kokoro_tts_audio_seconds Duration of generated audio in seconds.")
	e.println("# TYPE kokoro_tts_audio_seconds histogram")
	writeHistogram(e, "kokoro_tts_audio_seconds", m.audioSecs)
	return e.err
}

func writeCounter(e *metricsWriter, name string, data map[voiceStatus]uint64) {
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
		e.printf("%s{voice=%q,status=%q} %d\n", name, k.voice, k.status, data[k])
	}
}

func writeCounterByVoice(e *metricsWriter, name string, data map[string]uint64) {
	voices := sortedKeys(data)
	for _, v := range voices {
		e.printf("%s{voice=%q} %d\n", name, v, data[v])
	}
}

func writeHistogram(e *metricsWriter, name string, data map[string]*histogram) {
	voices := sortedHistogramKeys(data)
	for _, voice := range voices {
		h := data[voice]
		for i, bound := range h.buckets {
			e.printf("%s_bucket{voice=%q,le=%q} %d\n", name, voice, formatFloat(bound), h.counts[i])
		}
		e.printf("%s_bucket{voice=%q,le=\"+Inf\"} %d\n", name, voice, h.count)
		e.printf("%s_sum{voice=%q} %s\n", name, voice, formatFloat(h.sum))
		e.printf("%s_count{voice=%q} %d\n", name, voice, h.count)
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
