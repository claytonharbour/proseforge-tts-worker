//go:build functional

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The other arm of #88: a worker whose engine DOES load starts normally and says so.
//
// TestWorkerRefusesToStartWithoutEngine proves the no-engine paths. Without this
// one, a change that made the gauge read 0 unconditionally, or made every start
// fatal, would still pass there.
func TestWorkerWithEngineReportsModelLoaded(t *testing.T) {
	for _, p := range []string{"../../models/kokoro-v1.0.onnx", "../../models/voices-v1.0.bin"} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("model file %s not present (run make dev-models): %v", p, err)
		}
	}

	bin := buildWorker(t)
	port := freePort(t)
	cmd := exec.Command(bin)
	cmd.Env = workerEnv(port, "MODELS_DIR=../../models", "DATA_DIR=../../data")
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
	if !strings.Contains(body, "kokoro_tts_model_loaded 1") {
		t.Errorf("worker with a loaded engine should read kokoro_tts_model_loaded 1:\n%s\nstderr:\n%s", body, stderr.String())
	}
	if strings.Contains(stderr.String(), "stub") {
		t.Errorf("worker with a loaded engine mentioned the stub at startup:\n%s", stderr.String())
	}
}
