//go:build unit

package handler

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// shortWriter accepts only the first `limit` bytes and then reports a short
// write with no error — the case a bare `w.Write(audioData)` cannot distinguish
// from success, because it discards BOTH return values.
type shortWriter struct {
	http.ResponseWriter
	limit   int
	written int
	failErr error
}

func (s *shortWriter) Write(p []byte) (int, error) {
	if s.failErr != nil {
		return 0, s.failErr
	}
	n := len(p)
	if s.written+n > s.limit {
		n = s.limit - s.written
	}
	s.written += n
	return n, nil // short, but NOT an error — this is the silent case
}

// #73. handleSpeech returns raw audio, so a short write is TRUNCATED AUDIO at the
// caller: it decodes as a clipped file rather than as a failure. The fix checks
// the byte count as well as the error, and this pins both arms.
//
// ⚠️ The two arms are not redundant. A `w.Write` that returns (n < len, nil) is
// invisible to an error check alone — that is precisely why the count is
// checked — and a write that returns (0, err) is invisible to a count check that
// runs only when err == nil. One arm each.
func TestSpeechShortWriteIsNotSilent(t *testing.T) {
	audioLen := len("fake-audio-data")

	run := func(t *testing.T, w http.ResponseWriter) string {
		t.Helper()
		var logs bytes.Buffer
		old := log.Writer()
		log.SetOutput(&logs)
		defer log.SetOutput(old)

		h := newTestHandler()
		req := httptest.NewRequest("POST", "/v1/audio/speech",
			strings.NewReader(`{"input":"hello","voice":"af_sarah"}`))
		req.Header.Set("Content-Type", "application/json")
		h.handleSpeech(w, req)
		return logs.String()
	}

	t.Run("short write with no error", func(t *testing.T) {
		sw := &shortWriter{ResponseWriter: httptest.NewRecorder(), limit: audioLen / 2}
		got := run(t, sw)
		if !strings.Contains(got, "short write") {
			t.Fatalf("wrote %d of %d bytes and logged nothing about it — truncated audio is silent again.\nlogs: %q",
				sw.written, audioLen, got)
		}
	})

	t.Run("write returns an error", func(t *testing.T) {
		sw := &shortWriter{ResponseWriter: httptest.NewRecorder(), failErr: errors.New("connection reset by peer")}
		got := run(t, sw)
		if !strings.Contains(got, "writing audio failed") {
			t.Fatalf("Write returned an error and nothing was logged.\nlogs: %q", got)
		}
	})

	t.Run("healthy write logs nothing", func(t *testing.T) {
		// The control. Without it, a fix that logged unconditionally would pass
		// both arms above and spam a line on every successful request.
		sw := &shortWriter{ResponseWriter: httptest.NewRecorder(), limit: audioLen}
		got := run(t, sw)
		if strings.Contains(got, "short write") || strings.Contains(got, "writing audio failed") {
			t.Fatalf("a complete write logged a failure: %q", got)
		}
	})
}
