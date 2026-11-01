package audio

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Transcribe converts WAV audio bytes to text using whisper-cli (whisper-cpp).
// Requires: brew install whisper-cpp, and a downloaded GGML model file.
// whisper-cpp handles resampling internally, so 24kHz WAV input works directly.
func Transcribe(wavData []byte, modelPath string) (string, error) {
	if _, err := exec.LookPath("whisper-cli"); err != nil {
		return "", fmt.Errorf("whisper-cli not found: install with 'brew install whisper-cpp'")
	}

	// whisper-cli requires a file path — write WAV to a temp file.
	tmp, err := os.CreateTemp("", "transcribe-*.wav")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	// Best-effort cleanup of a temp file we are done with; a failure here
	// cannot change the transcription that was already produced.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(wavData); err != nil {
		_ = tmp.Close() // already failing; this error cannot improve the report
		return "", fmt.Errorf("writing temp file: %w", err)
	}
	// 🛑 THE ONE Close IN THIS PACKAGE THAT IS NOT CLEANUP. Write() above can
	// succeed into a buffer and still fail at flush, so a dropped error here
	// leaves a TRUNCATED WAV on disk. whisper-cli then transcribes a corrupt
	// file and returns garbage, or an error that reads as whisper's fault --
	// the failure surfaces one process away from its cause.
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("flushing temp file: %w", err)
	}

	cmd := exec.Command("whisper-cli",
		"--model", modelPath,
		"--language", "en",
		"--no-timestamps",
		"--no-prints",
		"--file", tmp.Name(),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper-cli error: %v: %s", err, stderr.String())
	}

	text := strings.TrimSpace(stdout.String())
	return text, nil
}
