//go:build unit

package audio

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestEncodeWAV_Header(t *testing.T) {
	samples := make([]float32, SampleRate) // 1 second of silence
	wav := EncodeWAV(samples)

	// Must be at least 44 bytes (header)
	if len(wav) < 44 {
		t.Fatalf("WAV too short: %d bytes", len(wav))
	}

	// RIFF header
	if string(wav[0:4]) != "RIFF" {
		t.Errorf("missing RIFF marker, got %q", wav[0:4])
	}

	expectedFileSize := uint32(len(wav) - 8)
	gotFileSize := binary.LittleEndian.Uint32(wav[4:8])
	if gotFileSize != expectedFileSize {
		t.Errorf("file size: got %d, want %d", gotFileSize, expectedFileSize)
	}

	if string(wav[8:12]) != "WAVE" {
		t.Errorf("missing WAVE marker, got %q", wav[8:12])
	}

	// fmt chunk
	if string(wav[12:16]) != "fmt " {
		t.Errorf("missing fmt marker, got %q", wav[12:16])
	}

	fmtSize := binary.LittleEndian.Uint32(wav[16:20])
	if fmtSize != 16 {
		t.Errorf("fmt chunk size: got %d, want 16", fmtSize)
	}

	audioFormat := binary.LittleEndian.Uint16(wav[20:22])
	if audioFormat != 1 {
		t.Errorf("audio format: got %d, want 1 (PCM)", audioFormat)
	}

	channels := binary.LittleEndian.Uint16(wav[22:24])
	if channels != 1 {
		t.Errorf("channels: got %d, want 1 (mono)", channels)
	}

	sampleRate := binary.LittleEndian.Uint32(wav[24:28])
	if sampleRate != SampleRate {
		t.Errorf("sample rate: got %d, want %d", sampleRate, SampleRate)
	}

	byteRate := binary.LittleEndian.Uint32(wav[28:32])
	expectedByteRate := uint32(SampleRate * NumChannels * BitsPerSample / 8)
	if byteRate != expectedByteRate {
		t.Errorf("byte rate: got %d, want %d", byteRate, expectedByteRate)
	}

	blockAlign := binary.LittleEndian.Uint16(wav[32:34])
	expectedBlockAlign := uint16(NumChannels * BitsPerSample / 8)
	if blockAlign != expectedBlockAlign {
		t.Errorf("block align: got %d, want %d", blockAlign, expectedBlockAlign)
	}

	bitsPerSample := binary.LittleEndian.Uint16(wav[34:36])
	if bitsPerSample != BitsPerSample {
		t.Errorf("bits per sample: got %d, want %d", bitsPerSample, BitsPerSample)
	}

	// data chunk
	if string(wav[36:40]) != "data" {
		t.Errorf("missing data marker, got %q", wav[36:40])
	}

	dataSize := binary.LittleEndian.Uint32(wav[40:44])
	expectedDataSize := uint32(len(samples) * 2) // 16-bit = 2 bytes per sample
	if dataSize != expectedDataSize {
		t.Errorf("data size: got %d, want %d", dataSize, expectedDataSize)
	}
}

func TestEncodeWAV_TotalSize(t *testing.T) {
	samples := make([]float32, 100)
	wav := EncodeWAV(samples)

	expectedSize := 44 + len(samples)*2
	if len(wav) != expectedSize {
		t.Errorf("total size: got %d, want %d", len(wav), expectedSize)
	}
}

func TestEncodeWAV_EmptySamples(t *testing.T) {
	wav := EncodeWAV(nil)
	if len(wav) != 44 {
		t.Errorf("empty WAV should be 44 bytes, got %d", len(wav))
	}
}

func TestEncodeWAV_SampleConversion(t *testing.T) {
	// Test that samples are correctly converted to int16 PCM
	samples := []float32{0.0, 1.0, -1.0, 0.5, -0.5}
	wav := EncodeWAV(samples)

	for i, expected := range []int16{0, math.MaxInt16, -math.MaxInt16, math.MaxInt16 / 2, -(math.MaxInt16 / 2)} {
		offset := 44 + i*2
		got := int16(binary.LittleEndian.Uint16(wav[offset : offset+2]))
		// Allow ±1 for rounding
		diff := got - expected
		if diff < -1 || diff > 1 {
			t.Errorf("sample[%d]: got %d, want %d (±1)", i, got, expected)
		}
	}
}

func TestEncodeWAV_Clamping(t *testing.T) {
	// Values outside [-1, 1] should be clamped
	samples := []float32{2.0, -3.0}
	wav := EncodeWAV(samples)

	s0 := int16(binary.LittleEndian.Uint16(wav[44:46]))
	s1 := int16(binary.LittleEndian.Uint16(wav[46:48]))

	if s0 != math.MaxInt16 {
		t.Errorf("clamped +2.0: got %d, want %d", s0, int16(math.MaxInt16))
	}
	if s1 != -math.MaxInt16 {
		t.Errorf("clamped -3.0: got %d, want %d", s1, int16(-math.MaxInt16))
	}
}

func TestDuration(t *testing.T) {
	// 24000 samples at 24kHz = exactly 1 second
	d := Duration(SampleRate)
	if d != 1.0 {
		t.Errorf("Duration(24000): got %f, want 1.0", d)
	}

	// 48000 samples = 2 seconds
	d = Duration(SampleRate * 2)
	if d != 2.0 {
		t.Errorf("Duration(48000): got %f, want 2.0", d)
	}

	// 0 samples = 0 seconds
	d = Duration(0)
	if d != 0.0 {
		t.Errorf("Duration(0): got %f, want 0.0", d)
	}
}
