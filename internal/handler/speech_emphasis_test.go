//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSpeechEmphasisPassthrough pins the #658 fix: /v1/audio/speech must
// thread the optional emphasis field into the TTS func exactly like the
// RunPod-envelope path — nil when omitted (worker default applies), and the
// caller's explicit true/false otherwise. Before the fix this handler
// hardcoded nil, silently dropping the per-request override.
func TestSpeechEmphasisPassthrough(t *testing.T) {
	cases := []struct {
		name string
		body string
		want *bool // nil = expect nil passed through
	}{
		{name: "OmittedIsNil", body: `{"input": "hello", "voice": "af_sarah"}`, want: nil},
		{name: "ExplicitTrue", body: `{"input": "hello", "voice": "af_sarah", "emphasis": true}`, want: boolPtr(true)},
		{name: "ExplicitFalse", body: `{"input": "hello", "voice": "af_sarah", "emphasis": false}`, want: boolPtr(false)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got *bool
			captured := false
			capturingTTS := func(_ context.Context, text, voice, format string, speed float64, emphasis *bool) ([]byte, float64, error) {
				got = emphasis
				captured = true
				return []byte("fake-audio-data"), 1.5, nil
			}
			h := New(capturingTTS, testVoiceNames)

			mux := http.NewServeMux()
			h.RegisterRoutes(mux)
			req := httptest.NewRequest("POST", "/v1/audio/speech", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			if !captured {
				t.Fatal("TTS func was never called")
			}
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("expected nil emphasis, got %v", *got)
			case tc.want != nil && got == nil:
				t.Errorf("expected emphasis=%v, got nil", *tc.want)
			case tc.want != nil && got != nil && *tc.want != *got:
				t.Errorf("expected emphasis=%v, got %v", *tc.want, *got)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }
