//go:build unit

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A worker whose engine cannot load refuses to start (#88).
//
// It used to fall back to stubTTS on a missing model, a missing ONNX runtime or a
// failed engine init, answer /health 200, and return silent audio. On RunPod every
// one of those requests is billed. The stub is now opt-in via TTS_ALLOW_STUB=1, and
// /metrics reports the engine state instead of a literal 1.
//
// This runs the real binary, because the failure being fixed is what the PROCESS
// does. A missing model path is the cheapest of the three init failures to force:
// it needs no model files, so the test runs anywhere.
func TestWorkerRefusesToStartWithoutEngine(t *testing.T) {
	bin := buildWorker(t)
	missing := filepath.Join(t.TempDir(), "no-such-model.onnx")

	t.Run("no flag: exits nonzero", func(t *testing.T) {
		cmd := exec.Command(bin)
		cmd.Env = workerEnv(freePort(t), "MODEL_PATH="+missing)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		go func() { done <- cmd.Wait() }()

		select {
		case err := <-done:
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() == 0 {
				t.Fatalf("worker exited with %v, want a nonzero exit; stderr:\n%s", err, stderr.String())
			}
			if !strings.Contains(stderr.String(), "refusing to start") {
				t.Errorf("stderr does not say why it stopped:\n%s", stderr.String())
			}
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatalf("worker was still running after 10s with no engine; stderr:\n%s", stderr.String())
		}
	})

	t.Run("TTS_ALLOW_STUB=1: serves, gauge reads 0", func(t *testing.T) {
		port := freePort(t)
		cmd := exec.Command(bin)
		cmd.Env = workerEnv(port, "MODEL_PATH="+missing, "TTS_ALLOW_STUB=1")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})

		body := getWhenUp(t, fmt.Sprintf("http://127.0.0.1:%d/metrics", port), &stderr)
		if !strings.Contains(body, "kokoro_tts_model_loaded 0") {
			t.Errorf("stub worker's /metrics should read kokoro_tts_model_loaded 0:\n%s", body)
		}
		if !strings.Contains(stderr.String(), "SILENT stub") {
			t.Errorf("stub mode should be announced loudly at startup; stderr:\n%s", stderr.String())
		}
	})
}
