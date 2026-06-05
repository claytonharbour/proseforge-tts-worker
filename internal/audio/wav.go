package audio

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	SampleRate    = 24000
	BitsPerSample = 16
	NumChannels   = 1
)

// WAVMetadata holds optional metadata to embed in the WAV LIST INFO chunk.
// All fields are optional — only non-empty fields are written.
type WAVMetadata struct {
	Software string // ISFT — software name + version (e.g. "ProseForge TTS v1.1 (abc1234)")
	Artist   string // IART — voice name
	Comment  string // ICMT — generation parameters (speed, format, etc.)
	Title    string // INAM — section/chapter title
}

// EncodeWAV converts float32 samples (-1.0 to 1.0) to a WAV file in memory.
// Returns the complete WAV file bytes (44-byte header + PCM data).
func EncodeWAV(samples []float32) []byte {
	return EncodeWAVWithMetadata(samples, nil)
}

// EncodeWAVWithMetadata converts float32 samples to a WAV file with optional
// LIST INFO metadata. The metadata is stored in a standard RIFF INFO chunk
// that all audio players handle gracefully (most ignore it, some display it).
func EncodeWAVWithMetadata(samples []float32, meta *WAVMetadata) []byte {
	numSamples := len(samples)
	dataSize := numSamples * (BitsPerSample / 8) * NumChannels

	// Build LIST INFO chunk if metadata provided
	var infoChunk []byte
	if meta != nil {
		infoChunk = buildListInfoChunk(meta)
	}

	fileSize := 44 + dataSize + len(infoChunk)
	buf := make([]byte, fileSize)

	// RIFF header
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(fileSize-8))
	copy(buf[8:12], "WAVE")

	// fmt chunk
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)                               // chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)                                // PCM format
	binary.LittleEndian.PutUint16(buf[22:24], NumChannels)                      // channels
	binary.LittleEndian.PutUint32(buf[24:28], SampleRate)                       // sample rate
	binary.LittleEndian.PutUint32(buf[28:32], SampleRate*NumChannels*BitsPerSample/8) // byte rate
	binary.LittleEndian.PutUint16(buf[32:34], NumChannels*BitsPerSample/8)      // block align
	binary.LittleEndian.PutUint16(buf[34:36], BitsPerSample)                    // bits per sample

	// data chunk
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))

	// Convert float32 samples to int16 PCM
	offset := 44
	for _, s := range samples {
		// Clamp to [-1.0, 1.0]
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		i16 := int16(s * math.MaxInt16)
		binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(i16))
		offset += 2
	}

	// Append LIST INFO chunk after audio data
	if len(infoChunk) > 0 {
		copy(buf[offset:], infoChunk)
	}

	return buf
}

// buildListInfoChunk builds a RIFF LIST INFO chunk from metadata.
func buildListInfoChunk(meta *WAVMetadata) []byte {
	type infoField struct {
		id    string
		value string
	}

	var fields []infoField
	if meta.Software != "" {
		fields = append(fields, infoField{"ISFT", meta.Software})
	}
	if meta.Artist != "" {
		fields = append(fields, infoField{"IART", meta.Artist})
	}
	if meta.Comment != "" {
		fields = append(fields, infoField{"ICMT", meta.Comment})
	}
	if meta.Title != "" {
		fields = append(fields, infoField{"INAM", meta.Title})
	}

	if len(fields) == 0 {
		return nil
	}

	// Calculate total size of INFO sub-chunks
	// Each sub-chunk: 4-byte ID + 4-byte size + string data + null terminator + optional pad byte
	infoDataSize := 4 // "INFO" type identifier
	for _, f := range fields {
		strLen := len(f.value) + 1 // +1 for null terminator
		if strLen%2 != 0 {
			strLen++ // pad to even boundary
		}
		infoDataSize += 8 + strLen // 4 (ID) + 4 (size) + padded string
	}

	chunk := make([]byte, 8+infoDataSize)
	copy(chunk[0:4], "LIST")
	binary.LittleEndian.PutUint32(chunk[4:8], uint32(infoDataSize))
	copy(chunk[8:12], "INFO")

	offset := 12
	for _, f := range fields {
		copy(chunk[offset:offset+4], f.id)
		rawLen := len(f.value) + 1 // include null terminator in chunk size
		binary.LittleEndian.PutUint32(chunk[offset+4:offset+8], uint32(rawLen))
		copy(chunk[offset+8:], f.value)
		chunk[offset+8+len(f.value)] = 0 // null terminator

		paddedLen := rawLen
		if paddedLen%2 != 0 {
			paddedLen++ // RIFF chunks must be word-aligned
		}
		offset += 8 + paddedLen
	}

	return chunk
}

// FormatMetadataComment creates a standard comment string from generation parameters.
func FormatMetadataComment(speed float64, format string) string {
	return fmt.Sprintf("speed=%.2f format=%s", speed, format)
}

// Duration returns the duration in seconds for the given number of samples.
func Duration(numSamples int) float64 {
	return float64(numSamples) / float64(SampleRate)
}
