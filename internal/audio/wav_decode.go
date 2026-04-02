package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// WAVFile holds decoded WAV audio data.
type WAVFile struct {
	SampleRate int
	Channels   int
	Samples    []float32 // normalized [-1.0, 1.0]
	Duration   float64
}

// ReadWAVFile reads a WAV file and returns decoded PCM samples as float32.
// Supports 16-bit PCM mono/stereo WAV files.
func ReadWAVFile(path string) (*WAVFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	if len(data) < 44 {
		return nil, fmt.Errorf("file too small for WAV header: %d bytes", len(data))
	}

	// Validate RIFF header
	if string(data[0:4]) != "RIFF" {
		return nil, fmt.Errorf("not a RIFF file")
	}
	if string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a WAVE file")
	}

	// Parse fmt chunk — find it by scanning chunks
	fmtOffset := -1
	dataOffset := -1
	dataSize := 0
	pos := 12
	for pos+8 <= len(data) {
		chunkID := string(data[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		if chunkID == "fmt " {
			fmtOffset = pos + 8
		} else if chunkID == "data" {
			dataOffset = pos + 8
			dataSize = chunkSize
		}
		pos += 8 + chunkSize
		// Chunks are word-aligned
		if chunkSize%2 != 0 {
			pos++
		}
	}

	if fmtOffset < 0 {
		return nil, fmt.Errorf("fmt chunk not found")
	}
	if dataOffset < 0 {
		return nil, fmt.Errorf("data chunk not found")
	}

	audioFormat := binary.LittleEndian.Uint16(data[fmtOffset : fmtOffset+2])
	if audioFormat != 1 {
		return nil, fmt.Errorf("unsupported audio format %d (only PCM=1 supported)", audioFormat)
	}

	channels := int(binary.LittleEndian.Uint16(data[fmtOffset+2 : fmtOffset+4]))
	sampleRate := int(binary.LittleEndian.Uint32(data[fmtOffset+4 : fmtOffset+8]))
	bitsPerSample := int(binary.LittleEndian.Uint16(data[fmtOffset+14 : fmtOffset+16]))

	if bitsPerSample != 16 {
		return nil, fmt.Errorf("unsupported bits per sample %d (only 16-bit supported)", bitsPerSample)
	}

	// Clamp dataSize to available data
	if dataOffset+dataSize > len(data) {
		dataSize = len(data) - dataOffset
	}

	numSamples := dataSize / (bitsPerSample / 8) / channels
	samples := make([]float32, numSamples)

	for i := 0; i < numSamples; i++ {
		offset := dataOffset + i*channels*(bitsPerSample/8)
		// Read first channel only (mono mixdown for stereo)
		sample := int16(binary.LittleEndian.Uint16(data[offset : offset+2]))
		samples[i] = float32(sample) / math.MaxInt16
	}

	duration := float64(numSamples) / float64(sampleRate)

	return &WAVFile{
		SampleRate: sampleRate,
		Channels:   channels,
		Samples:    samples,
		Duration:   duration,
	}, nil
}
