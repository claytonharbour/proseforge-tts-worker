//go:build unit

package handler

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestCounterIncrements(t *testing.T) {
	m := NewMetrics()
	m.RecordRequest("af_sarah", "ok")
	m.RecordRequest("af_sarah", "ok")
	m.RecordRequest("af_sarah", "error")
	m.RecordRequest("bf_emma", "ok")

	var buf bytes.Buffer
	if err := m.WriteMetrics(&buf); err != nil {
		t.Fatalf("WriteMetrics: %v", err)
	}
	out := buf.String()

	assertContains(t, out, `kokoro_tts_requests_total{voice="af_sarah",status="ok"} 2`)
	assertContains(t, out, `kokoro_tts_requests_total{voice="af_sarah",status="error"} 1`)
	assertContains(t, out, `kokoro_tts_requests_total{voice="bf_emma",status="ok"} 1`)
}

func TestCharactersCounter(t *testing.T) {
	m := NewMetrics()
	m.AddCharacters("af_sarah", 11)
	m.AddCharacters("af_sarah", 5)
	m.AddCharacters("bf_emma", 7)

	var buf bytes.Buffer
	if err := m.WriteMetrics(&buf); err != nil {
		t.Fatalf("WriteMetrics: %v", err)
	}
	out := buf.String()

	assertContains(t, out, `kokoro_tts_characters_total{voice="af_sarah"} 16`)
	assertContains(t, out, `kokoro_tts_characters_total{voice="bf_emma"} 7`)
}

func TestHistogramBuckets(t *testing.T) {
	m := NewMetrics()
	// Observe 0.3s generation — should land in 0.5, 1, 2, 3, 5, 8, 13, 21 buckets
	m.ObserveGeneration("af_sarah", 0.3)

	var buf bytes.Buffer
	if err := m.WriteMetrics(&buf); err != nil {
		t.Fatalf("WriteMetrics: %v", err)
	}
	out := buf.String()

	// 0.3 > 0.1, 0.3 > 0.25, so those buckets should be 0
	assertContains(t, out, `kokoro_tts_generation_seconds_bucket{voice="af_sarah",le="0.1"} 0`)
	assertContains(t, out, `kokoro_tts_generation_seconds_bucket{voice="af_sarah",le="0.25"} 0`)
	// 0.3 <= 0.5, so this and all higher buckets should be 1
	assertContains(t, out, `kokoro_tts_generation_seconds_bucket{voice="af_sarah",le="0.5"} 1`)
	assertContains(t, out, `kokoro_tts_generation_seconds_bucket{voice="af_sarah",le="1"} 1`)
	assertContains(t, out, `kokoro_tts_generation_seconds_bucket{voice="af_sarah",le="+Inf"} 1`)
	assertContains(t, out, `kokoro_tts_generation_seconds_sum{voice="af_sarah"} 0.3`)
	assertContains(t, out, `kokoro_tts_generation_seconds_count{voice="af_sarah"} 1`)
}

func TestHistogramMultipleObservations(t *testing.T) {
	m := NewMetrics()
	m.ObserveAudio("af_sarah", 1.5) // <= 2, 3, 5, 8, 13, 21, 30, 60
	m.ObserveAudio("af_sarah", 4.0) // <= 5, 8, 13, 21, 30, 60
	m.ObserveAudio("af_sarah", 100) // only +Inf

	var buf bytes.Buffer
	if err := m.WriteMetrics(&buf); err != nil {
		t.Fatalf("WriteMetrics: %v", err)
	}
	out := buf.String()

	assertContains(t, out, `kokoro_tts_audio_seconds_bucket{voice="af_sarah",le="0.5"} 0`)
	assertContains(t, out, `kokoro_tts_audio_seconds_bucket{voice="af_sarah",le="1"} 0`)
	assertContains(t, out, `kokoro_tts_audio_seconds_bucket{voice="af_sarah",le="2"} 1`)
	assertContains(t, out, `kokoro_tts_audio_seconds_bucket{voice="af_sarah",le="5"} 2`)
	assertContains(t, out, `kokoro_tts_audio_seconds_bucket{voice="af_sarah",le="60"} 2`)
	assertContains(t, out, `kokoro_tts_audio_seconds_bucket{voice="af_sarah",le="+Inf"} 3`)
	assertContains(t, out, `kokoro_tts_audio_seconds_sum{voice="af_sarah"} 105.5`)
	assertContains(t, out, `kokoro_tts_audio_seconds_count{voice="af_sarah"} 3`)
}

// The gauge reports whether the real engine initialised (#88). It used to be the
// literal 1, so a worker serving silent stub audio reported a loaded model.
func TestModelLoadedGauge(t *testing.T) {
	for _, tc := range []struct {
		loaded bool
		want   string
	}{
		{false, "kokoro_tts_model_loaded 0"},
		{true, "kokoro_tts_model_loaded 1"},
	} {
		m := NewMetrics()
		m.SetModelLoaded(tc.loaded)
		var buf bytes.Buffer
		if err := m.WriteMetrics(&buf); err != nil {
			t.Fatalf("WriteMetrics: %v", err)
		}
		out := buf.String()

		assertContains(t, out, "# TYPE kokoro_tts_model_loaded gauge")
		assertContains(t, out, tc.want)
	}
}

func TestEmptyMetricsOutput(t *testing.T) {
	m := NewMetrics()
	var buf bytes.Buffer
	if err := m.WriteMetrics(&buf); err != nil {
		t.Fatalf("WriteMetrics: %v", err)
	}
	out := buf.String()

	// Should have HELP/TYPE headers and model_loaded even with no observations.
	assertContains(t, out, "# HELP kokoro_tts_requests_total")
	assertContains(t, out, "# TYPE kokoro_tts_requests_total counter")
	assertContains(t, out, "# HELP kokoro_tts_generation_seconds")
	assertContains(t, out, "# TYPE kokoro_tts_generation_seconds histogram")
	// Nothing has said the engine loaded, so the honest default is 0 (#88).
	assertContains(t, out, "kokoro_tts_model_loaded 0")

	// No data lines for counters/histograms.
	if strings.Contains(out, `voice="`) {
		t.Errorf("empty metrics should have no voice labels, got:\n%s", out)
	}
}

func TestMultiVoiceSorted(t *testing.T) {
	m := NewMetrics()
	m.RecordRequest("zz_voice", "ok")
	m.RecordRequest("aa_voice", "ok")
	m.AddCharacters("zz_voice", 10)
	m.AddCharacters("aa_voice", 5)

	var buf bytes.Buffer
	if err := m.WriteMetrics(&buf); err != nil {
		t.Fatalf("WriteMetrics: %v", err)
	}
	out := buf.String()

	// aa_voice should appear before zz_voice.
	aaIdx := strings.Index(out, `voice="aa_voice"`)
	zzIdx := strings.Index(out, `voice="zz_voice"`)
	if aaIdx == -1 || zzIdx == -1 {
		t.Fatalf("expected both voices in output:\n%s", out)
	}
	if aaIdx > zzIdx {
		t.Errorf("voices not sorted: aa_voice at %d, zz_voice at %d", aaIdx, zzIdx)
	}
}

func TestConcurrencySafety(t *testing.T) {
	m := NewMetrics()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.RecordRequest("af_sarah", "ok")
			m.AddCharacters("af_sarah", 5)
			m.ObserveGeneration("af_sarah", 0.5)
			m.ObserveAudio("af_sarah", 2.0)
		}()
	}
	wg.Wait()

	var buf bytes.Buffer
	if err := m.WriteMetrics(&buf); err != nil {
		t.Fatalf("WriteMetrics: %v", err)
	}
	out := buf.String()

	assertContains(t, out, `kokoro_tts_requests_total{voice="af_sarah",status="ok"} 100`)
	assertContains(t, out, `kokoro_tts_characters_total{voice="af_sarah"} 500`)
	assertContains(t, out, `kokoro_tts_generation_seconds_count{voice="af_sarah"} 100`)
	assertContains(t, out, `kokoro_tts_audio_seconds_count{voice="af_sarah"} 100`)
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("output missing expected line %q\n\nfull output:\n%s", needle, haystack)
	}
}

// failingWriter fails after n successful bytes, to exercise the path where a
// scrape dies mid-body.
type failingWriter struct {
	remaining int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.remaining <= 0 {
		return 0, errors.New("connection reset by peer")
	}
	if len(p) > f.remaining {
		n := f.remaining
		f.remaining = 0
		return n, errors.New("connection reset by peer")
	}
	f.remaining -= len(p)
	return len(p), nil
}

// #73. WriteMetrics grew an error return specifically so a truncated scrape stops
// being silent. An error path with no test is the null instrument this ticket is
// about, so this asserts BOTH directions: a healthy writer must return nil, and a
// writer that dies mid-body must surface the error.
//
// ⚠️ The nil arm is not padding. Latching the first error means a bug that sets
// e.err spuriously would make every scrape report failure, and only the healthy
// arm can see that.
func TestWriteMetricsSurfacesWriteFailure(t *testing.T) {
	build := func() *Metrics {
		m := NewMetrics()
		m.RecordRequest("af_sarah", "ok")
		m.AddCharacters("af_sarah", 120)
		m.ObserveGeneration("af_sarah", 1.5)
		m.ObserveAudio("af_sarah", 4.0)
		return m
	}

	var good bytes.Buffer
	if err := build().WriteMetrics(&good); err != nil {
		t.Fatalf("healthy writer: want nil, got %v", err)
	}
	if good.Len() == 0 {
		t.Fatal("healthy writer produced no body")
	}

	// Fail partway through, not at byte 0 — a writer that refuses everything can
	// be caught by the first line and proves less.
	fw := &failingWriter{remaining: good.Len() / 2}
	err := build().WriteMetrics(fw)
	if err == nil {
		t.Fatalf("writer died after %d of %d bytes but WriteMetrics returned nil — "+
			"a truncated scrape is silent again", good.Len()/2, good.Len())
	}
}
