//go:build unit

package handler

// The HTTP request's context must reach the TTS function (#114). The worker runs one inference
// at a time, so work kept going for a caller that has gone away delays every request queued
// behind it. Cancelling a request is only useful if the synthesis function can see it.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestContextReachesTTS(t *testing.T) {
	routes := []struct {
		name, path, body string
	}{
		{"runpod envelope", "/", `{"input":{"text":"Hello world","voice":"af_sarah"}}`},
		{"openai speech", "/v1/audio/speech", `{"input":"Hello world","voice":"af_sarah"}`},
	}
	for _, rt := range routes {
		t.Run(rt.name, func(t *testing.T) {
			var seen context.Context
			h := New(func(ctx context.Context, text, voice, format string, speed float64, _ *bool) ([]byte, float64, error) {
				seen = ctx
				return nil, 0, ctx.Err()
			}, testVoiceNames)
			mux := http.NewServeMux()
			h.RegisterRoutes(mux)

			// Control: a live request is handed a live context, so a context that is
			// "cancelled" below is the request's own and not one the handler made up.
			req := httptest.NewRequest("POST", rt.path, bytes.NewBufferString(rt.body))
			req.Header.Set("Content-Type", "application/json")
			mux.ServeHTTP(httptest.NewRecorder(), req)
			if seen == nil {
				t.Fatal("the TTS function was never called")
			}
			if seen.Err() != nil {
				t.Fatalf("a live request reached the TTS function with a dead context: %v", seen.Err())
			}

			// A request whose client has gone: the TTS function must see it as done.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			req = httptest.NewRequest("POST", rt.path, bytes.NewBufferString(rt.body)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			seen = nil
			mux.ServeHTTP(httptest.NewRecorder(), req)
			if seen == nil || seen.Err() == nil {
				t.Fatalf("a cancelled request's context did not reach the TTS function (%v)", seen)
			}
		})
	}
}
