//go:build unit

package audio

import (
	"os/exec"
	"strings"
	"testing"
)

func TestTranscribe_MissingBinary(t *testing.T) {
	// Use a non-existent model path — should fail on binary lookup or model load.
	// If whisper-cli is not installed, we expect a clear error.
	if _, err := exec.LookPath("whisper-cli"); err != nil {
		t.Skip("whisper-cli not installed, skipping")
	}

	// Empty WAV data should produce an error from whisper-cli.
	_, err := Transcribe([]byte{}, "/nonexistent/model.bin")
	if err == nil {
		t.Fatal("expected error for empty WAV data, got nil")
	}
	assertErrorIsFromWhisper(t, err)
}

func TestTranscribe_InvalidModel(t *testing.T) {
	if _, err := exec.LookPath("whisper-cli"); err != nil {
		t.Skip("whisper-cli not installed, skipping")
	}

	// Valid WAV header but invalid model path.
	wav := makeMinimalWAV()
	_, err := Transcribe(wav, "/nonexistent/model.bin")
	if err == nil {
		t.Fatal("expected error for invalid model path, got nil")
	}
	assertErrorIsFromWhisper(t, err)
}

// assertErrorIsFromWhisper checks WHICH failure occurred, not merely that one did.
//
// 🛑 Both tests above asserted only `err != nil`, and that is a null instrument:
// Transcribe can fail at temp-file creation, at the write, at the flush, or at
// whisper-cli itself, and "an error happened" is true in every one of those
// worlds. Demonstrated 2026-08-19 while adding the flush check (#73) — an
// injection making tmp.Close() ALWAYS return an error left both tests green,
// because the injected error satisfied `err != nil` exactly as well as the real
// one. The tests reached the changed line and could not see it.
//
// ⚠️ So this pins the error to whisper's own failure. If a future change breaks
// the temp-file path instead, these tests now say so rather than staying green
// on a completely different fault.
func assertErrorIsFromWhisper(t *testing.T, err error) {
	t.Helper()
	if !strings.Contains(err.Error(), "whisper-cli error") {
		t.Fatalf("expected the failure to come from whisper-cli, got %q — "+
			"Transcribe failed somewhere else (temp file, write, or flush) and "+
			"an err != nil check alone would have called this a pass", err)
	}
}

// makeMinimalWAV creates a minimal valid WAV file (silence, 24kHz, 1 channel, 0.1s).
func makeMinimalWAV() []byte {
	numSamples := 24000 / 10 // 0.1 seconds
	samples := make([]float32, numSamples)
	return EncodeWAV(samples)
}
