//go:build unit

package handler

import (
	"bytes"
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
	m.WriteTo(&buf)
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
	m.WriteTo(&buf)
	out := buf.String()

	assertContains(t, out, `kokoro_tts_characters_total{voice="af_sarah"} 16`)
	assertContains(t, out, `kokoro_tts_characters_total{voice="bf_emma"} 7`)
}

func TestHistogramBuckets(t *testing.T) {
	m := NewMetrics()
	// Observe 0.3s generation — should land in 0.5, 1, 2, 3, 5, 8, 13, 21 buckets
	m.ObserveGeneration("af_sarah", 0.3)

	var buf bytes.Buffer
	m.WriteTo(&buf)
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
	m.WriteTo(&buf)
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

func TestModelLoadedGauge(t *testing.T) {
	m := NewMetrics()
	var buf bytes.Buffer
	m.WriteTo(&buf)
	out := buf.String()

	assertContains(t, out, "# TYPE kokoro_tts_model_loaded gauge")
	assertContains(t, out, "kokoro_tts_model_loaded 1")
}

func TestEmptyMetricsOutput(t *testing.T) {
	m := NewMetrics()
	var buf bytes.Buffer
	m.WriteTo(&buf)
	out := buf.String()

	// Should have HELP/TYPE headers and model_loaded even with no observations.
	assertContains(t, out, "# HELP kokoro_tts_requests_total")
	assertContains(t, out, "# TYPE kokoro_tts_requests_total counter")
	assertContains(t, out, "# HELP kokoro_tts_generation_seconds")
	assertContains(t, out, "# TYPE kokoro_tts_generation_seconds histogram")
	assertContains(t, out, "kokoro_tts_model_loaded 1")

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
	m.WriteTo(&buf)
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
	m.WriteTo(&buf)
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
