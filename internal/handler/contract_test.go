//go:build unit

package handler

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// TestContractResponseFormat verifies the response JSON matches proseforge's
// runPodTTSResponse struct exactly.
func TestContractResponseFormat(t *testing.T) {
	h := newTestHandler()
	w := doPost(t, h, `{"input":{"text":"Hello world","voice":"af_sarah","speed":1.0,"format":"wav"}}`)

	if w.Code != 200 {
		t.Fatalf("got status %d: %s", w.Code, w.Body.String())
	}

	// Parse as generic map to check exact field names
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Must have exactly one top-level key: "output"
	if len(raw) != 1 {
		t.Errorf("expected 1 top-level key, got %d: %v", len(raw), keys(raw))
	}
	outputRaw, ok := raw["output"]
	if !ok {
		t.Fatal("missing 'output' key")
	}

	// Parse output
	var output map[string]json.RawMessage
	if err := json.Unmarshal(outputRaw, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	// Check all required fields exist
	requiredFields := []string{"audio", "format", "sample_rate", "duration_seconds", "voice"}
	for _, field := range requiredFields {
		if _, ok := output[field]; !ok {
			t.Errorf("missing required field: %s", field)
		}
	}

	// Verify no extra fields
	if len(output) != len(requiredFields) {
		t.Errorf("expected %d fields, got %d: %v", len(requiredFields), len(output), keys(output))
	}

	// Verify field types
	var resp TTSResponse
	if err := json.Unmarshal(outputRaw, &resp); err != nil {
		t.Fatalf("unmarshal TTSResponse: %v", err)
	}

	// audio must be valid base64
	if _, err := base64.StdEncoding.DecodeString(resp.Audio); err != nil {
		t.Errorf("audio is not valid base64: %v", err)
	}

	// sample_rate must be 24000
	if resp.SampleRate != 24000 {
		t.Errorf("sample_rate: got %d, want 24000", resp.SampleRate)
	}

	// duration_seconds must be positive
	if resp.DurationSeconds <= 0 {
		t.Errorf("duration_seconds: got %f, want > 0", resp.DurationSeconds)
	}

	// voice must echo back
	if resp.Voice != "af_sarah" {
		t.Errorf("voice: got %q, want %q", resp.Voice, "af_sarah")
	}

	// format must match request
	if resp.Format != "wav" {
		t.Errorf("format: got %q, want %q", resp.Format, "wav")
	}
}

// TestContractErrorFormat verifies error responses match RunPod format.
func TestContractErrorFormat(t *testing.T) {
	h := newTestHandler()
	w := doPost(t, h, `{"input":{}}`)

	if w.Code != 400 {
		t.Fatalf("got status %d, want 400", w.Code)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Must have exactly one key: "error"
	if len(raw) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(raw), keys(raw))
	}
	if _, ok := raw["error"]; !ok {
		t.Error("missing 'error' key")
	}

	var errStr string
	if err := json.Unmarshal(raw["error"], &errStr); err != nil {
		t.Errorf("error field is not a string: %v", err)
	}
	if errStr == "" {
		t.Error("error message is empty")
	}
}

func keys[K comparable, V any](m map[K]V) []K {
	k := make([]K, 0, len(m))
	for key := range m {
		k = append(k, key)
	}
	return k
}
