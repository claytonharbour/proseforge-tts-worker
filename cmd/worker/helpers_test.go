//go:build unit || functional

package main

// Shared by the unit and functional worker tests. It has no Test functions, so
// it adds no test to either suite.

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildWorker compiles this package once into a temp dir.
func buildWorker(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "tts-worker")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// workerEnv is the parent environment minus anything that would change which
// path the worker takes, plus the given overrides. RUNPOD_* would switch it to
// polling mode, and an inherited TTS_ALLOW_STUB would turn the no-flag arm green
// for the wrong reason.
func workerEnv(port int, extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "RUNPOD_") || strings.HasPrefix(kv, "TTS_ALLOW_STUB=") ||
			strings.HasPrefix(kv, "MODEL_PATH=") || strings.HasPrefix(kv, "PORT=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, append([]string{fmt.Sprintf("PORT=%d", port)}, extra...)...)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func getWhenUp(t *testing.T, url string, stderr *bytes.Buffer) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return string(b)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("worker never answered %s; stderr:\n%s", url, stderr.String())
	return ""
}
