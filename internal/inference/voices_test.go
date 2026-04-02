//go:build unit

package inference

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// makeNPY builds a minimal NPY v1.0 file with float32 data and given shape.
func makeNPY(rows, cols int, data []float32) []byte {
	header := fmt.Sprintf("{'descr': '<f4', 'fortran_order': False, 'shape': (%d, %d), }", rows, cols)
	// Pad header to 64-byte alignment (magic=6 + version=2 + headerLen=2 + header + padding)
	// Total preamble before data must be multiple of 64
	preamble := 10 + len(header) + 1 // +1 for newline
	padding := 0
	if preamble%64 != 0 {
		padding = 64 - (preamble % 64)
	}

	var buf bytes.Buffer
	// Magic
	buf.Write([]byte{0x93, 'N', 'U', 'M', 'P', 'Y'})
	// Version 1.0
	buf.Write([]byte{1, 0})
	// Header length (header + padding + newline)
	headerLen := uint16(len(header) + padding + 1)
	binary.Write(&buf, binary.LittleEndian, headerLen)
	// Header string
	buf.WriteString(header)
	// Padding
	for range padding {
		buf.WriteByte(' ')
	}
	buf.WriteByte('\n')
	// Data
	binary.Write(&buf, binary.LittleEndian, data)
	return buf.Bytes()
}

// makeNPZ creates a temporary NPZ file with the given voices.
func makeNPZ(t *testing.T, voices map[string][]float32, rows, cols int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "voices.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, data := range voices {
		fw, err := w.Create(name + ".npy")
		if err != nil {
			t.Fatal(err)
		}
		npy := makeNPY(rows, cols, data)
		if _, err := fw.Write(npy); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadVoices(t *testing.T) {
	rows, cols := 4, 3 // small shape for testing
	voiceData := make([]float32, rows*cols)
	for i := range voiceData {
		voiceData[i] = float32(i) * 0.1
	}

	path := makeNPZ(t, map[string][]float32{
		"af_sarah":   voiceData,
		"am_michael": voiceData,
	}, rows, cols)

	store, err := LoadVoices(path)
	if err != nil {
		t.Fatalf("LoadVoices: %v", err)
	}

	if store.Count() != 2 {
		t.Errorf("count: got %d, want 2", store.Count())
	}

	v, err := store.Get("af_sarah")
	if err != nil {
		t.Fatalf("Get af_sarah: %v", err)
	}
	if v.Rows != rows || v.Cols != cols {
		t.Errorf("shape: got [%d,%d], want [%d,%d]", v.Rows, v.Cols, rows, cols)
	}
	if v.Data[0] != 0.0 {
		t.Errorf("data[0]: got %f, want 0.0", v.Data[0])
	}
}

func TestVoiceNotFound(t *testing.T) {
	rows, cols := 2, 3
	voiceData := make([]float32, rows*cols)

	path := makeNPZ(t, map[string][]float32{
		"af_sarah": voiceData,
	}, rows, cols)

	store, err := LoadVoices(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent voice")
	}
}

func TestStyleForLength(t *testing.T) {
	rows, cols := 4, 3
	data := make([]float32, rows*cols)
	for i := range data {
		data[i] = float32(i)
	}

	v := &Voice{Name: "test", Data: data, Rows: rows, Cols: cols}

	tests := []struct {
		tokenLen int
		wantRow  int
	}{
		{0, 0},
		{1, 1},
		{3, 3},
		{4, 3},  // clamped to max row
		{100, 3}, // clamped
		{-1, 0},  // clamped to 0
	}

	for _, tt := range tests {
		style := v.StyleForLength(tt.tokenLen)
		if len(style) != cols {
			t.Errorf("tokenLen=%d: got len %d, want %d", tt.tokenLen, len(style), cols)
			continue
		}
		expectedFirst := float32(tt.wantRow * cols)
		if style[0] != expectedFirst {
			t.Errorf("tokenLen=%d: first value got %f, want %f", tt.tokenLen, style[0], expectedFirst)
		}
	}
}

// makeNPY3D builds a minimal NPY v1.0 file with float32 data and 3D shape.
func makeNPY3D(d0, d1, d2 int, data []float32) []byte {
	header := fmt.Sprintf("{'descr': '<f4', 'fortran_order': False, 'shape': (%d, %d, %d), }", d0, d1, d2)
	preamble := 10 + len(header) + 1
	padding := 0
	if preamble%64 != 0 {
		padding = 64 - (preamble % 64)
	}

	var buf bytes.Buffer
	buf.Write([]byte{0x93, 'N', 'U', 'M', 'P', 'Y'})
	buf.Write([]byte{1, 0})
	headerLen := uint16(len(header) + padding + 1)
	binary.Write(&buf, binary.LittleEndian, headerLen)
	buf.WriteString(header)
	for range padding {
		buf.WriteByte(' ')
	}
	buf.WriteByte('\n')
	binary.Write(&buf, binary.LittleEndian, data)
	return buf.Bytes()
}

// makeNPZ3D creates a temporary NPZ file with 3D-shaped voices (d0, 1, d2).
func makeNPZ3D(t *testing.T, voices map[string][]float32, d0, d2 int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "voices.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, data := range voices {
		fw, err := w.Create(name + ".npy")
		if err != nil {
			t.Fatal(err)
		}
		npy := makeNPY3D(d0, 1, d2, data)
		if _, err := fw.Write(npy); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadVoices3DSqueeze(t *testing.T) {
	rows, cols := 4, 3 // 3D shape: (4, 1, 3) squeezed to (4, 3)
	voiceData := make([]float32, rows*cols)
	for i := range voiceData {
		voiceData[i] = float32(i) * 0.1
	}

	path := makeNPZ3D(t, map[string][]float32{
		"af_sarah": voiceData,
	}, rows, cols)

	store, err := LoadVoices(path)
	if err != nil {
		t.Fatalf("LoadVoices (3D): %v", err)
	}

	v, err := store.Get("af_sarah")
	if err != nil {
		t.Fatalf("Get af_sarah: %v", err)
	}
	if v.Rows != rows || v.Cols != cols {
		t.Errorf("shape: got [%d,%d], want [%d,%d]", v.Rows, v.Cols, rows, cols)
	}
	if v.Data[0] != 0.0 {
		t.Errorf("data[0]: got %f, want 0.0", v.Data[0])
	}
}

func TestCreateBlend(t *testing.T) {
	rows, cols := 4, 3

	dataA := make([]float32, rows*cols)
	dataB := make([]float32, rows*cols)
	for i := range dataA {
		dataA[i] = 0.0
		dataB[i] = 1.0
	}

	path := makeNPZ(t, map[string][]float32{
		"voice_a": dataA,
		"voice_b": dataB,
	}, rows, cols)

	store, err := LoadVoices(path)
	if err != nil {
		t.Fatal(err)
	}

	// 50/50 blend
	name, err := store.CreateBlend("voice_a", "voice_b", 0.5)
	if err != nil {
		t.Fatalf("CreateBlend: %v", err)
	}

	v, err := store.Get(name)
	if err != nil {
		t.Fatalf("Get blend: %v", err)
	}

	// All values should be 0.5 (midpoint of 0.0 and 1.0)
	for i, val := range v.Data {
		if val < 0.49 || val > 0.51 {
			t.Errorf("data[%d] = %f, want ~0.5", i, val)
		}
	}

	// 80/20 blend
	name2, err := store.CreateBlend("voice_a", "voice_b", 0.2)
	if err != nil {
		t.Fatalf("CreateBlend 80/20: %v", err)
	}

	v2, err := store.Get(name2)
	if err != nil {
		t.Fatalf("Get blend 80/20: %v", err)
	}

	// All values should be 0.2 (80% of 0.0 + 20% of 1.0)
	for i, val := range v2.Data {
		if val < 0.19 || val > 0.21 {
			t.Errorf("data[%d] = %f, want ~0.2", i, val)
		}
	}

	// Verify the store now has 4 voices (2 original + 2 blends)
	if store.Count() != 4 {
		t.Errorf("count: got %d, want 4", store.Count())
	}
}

func TestCreateBlendMissingVoice(t *testing.T) {
	path := makeNPZ(t, map[string][]float32{
		"voice_a": make([]float32, 12),
	}, 4, 3)

	store, err := LoadVoices(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.CreateBlend("voice_a", "nonexistent", 0.5)
	if err == nil {
		t.Error("expected error for missing voice")
	}
}

func TestLoadVoicesEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.bin")
	// Create a valid but empty zip
	f, _ := os.Create(path)
	w := zip.NewWriter(f)
	w.Close()
	f.Close()

	_, err := LoadVoices(path)
	if err == nil {
		t.Error("expected error for empty NPZ")
	}
}

func TestLoadVoicesInvalidPath(t *testing.T) {
	_, err := LoadVoices("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
