//go:build unit

package audio

import (
	"os/exec"
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
}

// makeMinimalWAV creates a minimal valid WAV file (silence, 24kHz, 1 channel, 0.1s).
func makeMinimalWAV() []byte {
	numSamples := 24000 / 10 // 0.1 seconds
	samples := make([]float32, numSamples)
	return EncodeWAV(samples)
}
