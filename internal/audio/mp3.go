package audio

import (
	"bytes"
	"fmt"
	"os/exec"
)

// EncodeMP3 converts a WAV byte slice to MP3 using ffmpeg subprocess.
// The input must be a valid WAV file (e.g., output of EncodeWAV).
// Optional metadata is written as ID3 tags.
func EncodeMP3(wavData []byte, meta *WAVMetadata) ([]byte, error) {
	args := []string{
		"-i", "pipe:0",
		"-f", "mp3",
		"-ab", "128k",
	}

	// Add ID3 metadata tags
	if meta != nil {
		if meta.Software != "" {
			args = append(args, "-metadata", "encoded_by="+meta.Software)
		}
		if meta.Artist != "" {
			args = append(args, "-metadata", "artist="+meta.Artist)
		}
		if meta.Comment != "" {
			args = append(args, "-metadata", "comment="+meta.Comment)
		}
		if meta.Title != "" {
			args = append(args, "-metadata", "title="+meta.Title)
		}
	}

	args = append(args, "-y", "-loglevel", "error", "pipe:1")

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdin = bytes.NewReader(wavData)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg error: %v: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}
