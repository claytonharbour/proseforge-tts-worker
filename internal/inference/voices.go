package inference

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

// Voice holds the style embeddings for a single voice.
// Shape: [510][256] float32 — row index = token sequence length.
type Voice struct {
	Name string
	// Data is [510][256] stored row-major: Data[row*256 + col]
	Data []float32
	Rows int
	Cols int
}

// StyleForLength returns the 256-dim style vector for the given token sequence length.
// The index is clamped to [0, Rows-1].
func (v *Voice) StyleForLength(tokenLen int) []float32 {
	row := tokenLen
	if row < 0 {
		row = 0
	}
	if row >= v.Rows {
		row = v.Rows - 1
	}
	start := row * v.Cols
	return v.Data[start : start+v.Cols]
}

// VoiceStore holds all loaded voice embeddings.
type VoiceStore struct {
	voices map[string]*Voice
}

// LoadVoices reads voice embeddings from an NPZ file (zip of .npy arrays).
func LoadVoices(path string) (*VoiceStore, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open voices file: %w", err)
	}
	defer r.Close()

	store := &VoiceStore{voices: make(map[string]*Voice)}

	for _, f := range r.File {
		if !strings.HasSuffix(f.Name, ".npy") {
			continue
		}

		name := strings.TrimSuffix(f.Name, ".npy")

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}

		voice, err := parseNPY(name, rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f.Name, err)
		}

		store.voices[name] = voice
	}

	if len(store.voices) == 0 {
		return nil, fmt.Errorf("no voice embeddings found in %s", path)
	}

	return store, nil
}

// Get returns the voice with the given name, or an error if not found.
func (vs *VoiceStore) Get(name string) (*Voice, error) {
	v, ok := vs.voices[name]
	if !ok {
		return nil, fmt.Errorf("voice %q not found (available: %s)", name, vs.ListNames())
	}
	return v, nil
}

// ListNames returns all available voice names as a comma-separated string.
func (vs *VoiceStore) ListNames() string {
	return strings.Join(vs.Names(), ", ")
}

// Names returns all available voice names as a sorted slice.
func (vs *VoiceStore) Names() []string {
	names := make([]string, 0, len(vs.voices))
	for k := range vs.voices {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Count returns the number of loaded voices.
func (vs *VoiceStore) Count() int {
	return len(vs.voices)
}

// CreateBlend creates a new voice by linearly interpolating between two existing voices.
// blend=0.0 gives 100% voiceA, blend=1.0 gives 100% voiceB.
// The blended voice is registered in the store and its name is returned.
func (vs *VoiceStore) CreateBlend(nameA, nameB string, blend float32) (string, error) {
	a, err := vs.Get(nameA)
	if err != nil {
		return "", err
	}
	b, err := vs.Get(nameB)
	if err != nil {
		return "", err
	}
	if a.Rows != b.Rows || a.Cols != b.Cols {
		return "", fmt.Errorf("voice shape mismatch: %s(%d,%d) vs %s(%d,%d)",
			nameA, a.Rows, a.Cols, nameB, b.Rows, b.Cols)
	}

	data := make([]float32, len(a.Data))
	for i := range data {
		data[i] = a.Data[i]*(1-blend) + b.Data[i]*blend
	}

	blendName := fmt.Sprintf("%s+%s@%.0f", nameA, nameB, blend*100)
	vs.voices[blendName] = &Voice{
		Name: blendName,
		Data: data,
		Rows: a.Rows,
		Cols: a.Cols,
	}
	return blendName, nil
}

// parseNPY reads a numpy .npy file and returns a Voice.
// Supports NPY format version 1.0 and 2.0, float32 dtype only.
func parseNPY(name string, r io.Reader) (*Voice, error) {
	// Read magic: \x93NUMPY
	magic := make([]byte, 6)
	if _, err := io.ReadFull(r, magic); err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}
	if magic[0] != 0x93 || string(magic[1:6]) != "NUMPY" {
		return nil, fmt.Errorf("invalid NPY magic: %x", magic)
	}

	// Read version
	ver := make([]byte, 2)
	if _, err := io.ReadFull(r, ver); err != nil {
		return nil, fmt.Errorf("read version: %w", err)
	}
	major := ver[0]

	// Read header length
	var headerLen uint32
	switch major {
	case 1:
		var hl uint16
		if err := binary.Read(r, binary.LittleEndian, &hl); err != nil {
			return nil, fmt.Errorf("read header length: %w", err)
		}
		headerLen = uint32(hl)
	case 2:
		if err := binary.Read(r, binary.LittleEndian, &headerLen); err != nil {
			return nil, fmt.Errorf("read header length: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported NPY version: %d", major)
	}

	// Read header string
	headerBytes := make([]byte, headerLen)
	if _, err := io.ReadFull(r, headerBytes); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	header := string(headerBytes)

	// Parse dtype
	dtype, err := parseField(header, "descr")
	if err != nil {
		return nil, err
	}
	if dtype != "'<f4'" && dtype != "\"<f4\"" {
		return nil, fmt.Errorf("unsupported dtype %s, need '<f4' (float32 little-endian)", dtype)
	}

	// Parse shape
	rows, cols, err := parseShape(header)
	if err != nil {
		return nil, err
	}

	// Read raw float32 data
	totalElements := rows * cols
	data := make([]float32, totalElements)
	if err := binary.Read(r, binary.LittleEndian, data); err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	// Validate: no NaN or Inf
	for i, v := range data {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, fmt.Errorf("invalid value at index %d: %f", i, v)
		}
	}

	return &Voice{
		Name: name,
		Data: data,
		Rows: rows,
		Cols: cols,
	}, nil
}

// parseField extracts a field value from the NPY header dict string.
func parseField(header, field string) (string, error) {
	// Look for 'field': value or "field": value
	for _, quote := range []string{"'", "\""} {
		key := quote + field + quote + ":"
		idx := strings.Index(header, key)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(header[idx+len(key):])
		// Find the end: comma or }
		end := strings.IndexAny(rest, ",}")
		if end < 0 {
			return strings.TrimSpace(rest), nil
		}
		return strings.TrimSpace(rest[:end]), nil
	}
	return "", fmt.Errorf("field %q not found in header: %s", field, header)
}

// parseShape extracts (rows, cols) from the NPY header shape tuple.
func parseShape(header string) (int, int, error) {
	// Find the shape tuple directly: 'shape': (N, M) or "shape": (N, M)
	// Can't use parseField because the tuple contains commas
	idx := strings.Index(header, "'shape':")
	if idx < 0 {
		idx = strings.Index(header, "\"shape\":")
	}
	if idx < 0 {
		return 0, 0, fmt.Errorf("shape not found in header: %s", header)
	}

	rest := header[idx:]
	openParen := strings.Index(rest, "(")
	closeParen := strings.Index(rest, ")")
	if openParen < 0 || closeParen < 0 || closeParen <= openParen {
		return 0, 0, fmt.Errorf("malformed shape in header: %s", rest)
	}

	shapeStr := rest[openParen+1 : closeParen]
	parts := strings.Split(shapeStr, ",")

	// Parse all dimensions, ignoring trailing empty parts from trailing commas.
	var dims []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var d int
		if _, err := fmt.Sscanf(p, "%d", &d); err != nil {
			return 0, 0, fmt.Errorf("parse shape dim %q: %w", p, err)
		}
		dims = append(dims, d)
	}

	switch len(dims) {
	case 2:
		return dims[0], dims[1], nil
	case 3:
		// Squeeze a size-1 middle dimension: (510, 1, 256) → (510, 256)
		if dims[1] == 1 {
			return dims[0], dims[2], nil
		}
		return 0, 0, fmt.Errorf("3D shape (%s) has non-unit middle dim %d", shapeStr, dims[1])
	default:
		return 0, 0, fmt.Errorf("expected 2D or 3D shape, got %dD (%s)", len(dims), shapeStr)
	}
}
