//go:build unit

package audio

import (
	"testing"
)

func TestDetectPronunciationErrors_NoErrors(t *testing.T) {
	// Identical source and alignment should produce 0 errors.
	alignment := &Alignment{
		Words: []AlignedWord{
			{Word: "Hello", StartMs: 0, EndMs: 500, Confidence: 0.95},
			{Word: "world", StartMs: 500, EndMs: 1000, Confidence: 0.90},
		},
		Duration: 1.0,
	}

	report := DetectPronunciationErrors(alignment, "Hello world")
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Errors) != 0 {
		t.Errorf("got %d errors, want 0", len(report.Errors))
		for _, e := range report.Errors {
			t.Logf("  error: type=%s expected=%q heard=%q", e.Type, e.Expected, e.Heard)
		}
	}
	if report.SourceWords != 2 {
		t.Errorf("SourceWords = %d, want 2", report.SourceWords)
	}
	if report.WhisperWords != 2 {
		t.Errorf("WhisperWords = %d, want 2", report.WhisperWords)
	}
}

func TestDetectPronunciationErrors_Substitution(t *testing.T) {
	// "parked" in source but Whisper heard "pocked" — substitution error.
	alignment := &Alignment{
		Words: []AlignedWord{
			{Word: "the", StartMs: 0, EndMs: 200, Confidence: 0.95},
			{Word: "pocked", StartMs: 200, EndMs: 600, Confidence: 0.85},
			{Word: "road", StartMs: 600, EndMs: 1000, Confidence: 0.92},
		},
		Duration: 1.0,
	}

	report := DetectPronunciationErrors(alignment, "the parked road")
	if report == nil {
		t.Fatal("expected non-nil report")
	}

	if len(report.Errors) != 1 {
		t.Fatalf("got %d errors, want 1", len(report.Errors))
	}

	err := report.Errors[0]
	if err.Type != "substitution" {
		t.Errorf("error type = %q, want %q", err.Type, "substitution")
	}
	if err.Expected != "parked" {
		t.Errorf("expected = %q, want %q", err.Expected, "parked")
	}
	if err.Heard != "pocked" {
		t.Errorf("heard = %q, want %q", err.Heard, "pocked")
	}
	if err.StartMs != 200 {
		t.Errorf("StartMs = %d, want 200", err.StartMs)
	}
	if err.EndMs != 600 {
		t.Errorf("EndMs = %d, want 600", err.EndMs)
	}
	if err.Confidence != 0.85 {
		t.Errorf("Confidence = %.2f, want 0.85", err.Confidence)
	}
	// "parked" vs "pocked": char Levenshtein = 2 (a->o, r->c), <= 3 so severity "medium"
	if err.Severity != "medium" {
		t.Errorf("Severity = %q, want %q", err.Severity, "medium")
	}
	if err.Context == "" {
		t.Error("expected non-empty context")
	}
}

func TestDetectPronunciationErrors_Insertion(t *testing.T) {
	// Whisper heard an extra word "um" not in the source.
	alignment := &Alignment{
		Words: []AlignedWord{
			{Word: "hello", StartMs: 0, EndMs: 400, Confidence: 0.95},
			{Word: "um", StartMs: 400, EndMs: 600, Confidence: 0.70},
			{Word: "world", StartMs: 600, EndMs: 1000, Confidence: 0.92},
		},
		Duration: 1.0,
	}

	report := DetectPronunciationErrors(alignment, "hello world")
	if report == nil {
		t.Fatal("expected non-nil report")
	}

	if len(report.Errors) != 1 {
		t.Fatalf("got %d errors, want 1", len(report.Errors))
	}

	err := report.Errors[0]
	if err.Type != "insertion" {
		t.Errorf("error type = %q, want %q", err.Type, "insertion")
	}
	if err.Expected != "" {
		t.Errorf("expected = %q, want empty", err.Expected)
	}
	if err.Heard != "um" {
		t.Errorf("heard = %q, want %q", err.Heard, "um")
	}
	if err.Severity != "low" {
		t.Errorf("Severity = %q, want %q", err.Severity, "low")
	}
	if err.StartMs != 400 {
		t.Errorf("StartMs = %d, want 400", err.StartMs)
	}
}

func TestDetectPronunciationErrors_Deletion(t *testing.T) {
	// Source has "the big red ball" but Whisper only heard "the big ball" — "red" deleted.
	alignment := &Alignment{
		Words: []AlignedWord{
			{Word: "the", StartMs: 0, EndMs: 200, Confidence: 0.95},
			{Word: "big", StartMs: 200, EndMs: 500, Confidence: 0.90},
			{Word: "ball", StartMs: 500, EndMs: 900, Confidence: 0.88},
		},
		Duration: 0.9,
	}

	report := DetectPronunciationErrors(alignment, "the big red ball")
	if report == nil {
		t.Fatal("expected non-nil report")
	}

	if len(report.Errors) != 1 {
		t.Fatalf("got %d errors, want 1", len(report.Errors))
	}

	err := report.Errors[0]
	if err.Type != "deletion" {
		t.Errorf("error type = %q, want %q", err.Type, "deletion")
	}
	if err.Expected != "red" {
		t.Errorf("expected = %q, want %q", err.Expected, "red")
	}
	if err.Heard != "" {
		t.Errorf("heard = %q, want empty", err.Heard)
	}
	if err.Severity != "medium" {
		t.Errorf("Severity = %q, want %q", err.Severity, "medium")
	}
	if err.Context == "" {
		t.Error("expected non-empty context")
	}
}

func TestDetectPronunciationErrors_LowConfidence(t *testing.T) {
	// Substitution with low confidence (<0.5) should get severity "low".
	alignment := &Alignment{
		Words: []AlignedWord{
			{Word: "the", StartMs: 0, EndMs: 200, Confidence: 0.95},
			{Word: "walking", StartMs: 200, EndMs: 600, Confidence: 0.35},
			{Word: "path", StartMs: 600, EndMs: 1000, Confidence: 0.92},
		},
		Duration: 1.0,
	}

	report := DetectPronunciationErrors(alignment, "the winding path")
	if report == nil {
		t.Fatal("expected non-nil report")
	}

	if len(report.Errors) != 1 {
		t.Fatalf("got %d errors, want 1", len(report.Errors))
	}

	err := report.Errors[0]
	if err.Type != "substitution" {
		t.Errorf("error type = %q, want %q", err.Type, "substitution")
	}
	if err.Expected != "winding" {
		t.Errorf("expected = %q, want %q", err.Expected, "winding")
	}
	if err.Heard != "walking" {
		t.Errorf("heard = %q, want %q", err.Heard, "walking")
	}
	// Low confidence should override to "low" severity
	if err.Severity != "low" {
		t.Errorf("Severity = %q, want %q (low confidence should force low severity)", err.Severity, "low")
	}
	if err.Confidence != 0.35 {
		t.Errorf("Confidence = %.2f, want 0.35", err.Confidence)
	}
}

func TestDetectPronunciationErrors_NilAlignment(t *testing.T) {
	report := DetectPronunciationErrors(nil, "hello world")
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Errors) != 0 {
		t.Errorf("got %d errors for nil alignment, want 0", len(report.Errors))
	}
}

func TestDetectPronunciationErrors_EmptySource(t *testing.T) {
	alignment := &Alignment{
		Words: []AlignedWord{
			{Word: "hello", StartMs: 0, EndMs: 500, Confidence: 0.95},
		},
		Duration: 0.5,
	}

	report := DetectPronunciationErrors(alignment, "")
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.SourceWords != 0 {
		t.Errorf("SourceWords = %d, want 0", report.SourceWords)
	}
}

func TestDetectPronunciationErrors_HighSeveritySubstitution(t *testing.T) {
	// Large character-level distance (>3) should produce "high" severity.
	alignment := &Alignment{
		Words: []AlignedWord{
			{Word: "the", StartMs: 0, EndMs: 200, Confidence: 0.95},
			{Word: "butterfly", StartMs: 200, EndMs: 700, Confidence: 0.88},
			{Word: "flew", StartMs: 700, EndMs: 1000, Confidence: 0.90},
		},
		Duration: 1.0,
	}

	report := DetectPronunciationErrors(alignment, "the catastrophe flew")
	if report == nil {
		t.Fatal("expected non-nil report")
	}

	if len(report.Errors) != 1 {
		t.Fatalf("got %d errors, want 1", len(report.Errors))
	}

	err := report.Errors[0]
	if err.Type != "substitution" {
		t.Errorf("error type = %q, want %q", err.Type, "substitution")
	}
	// "catastrophe" vs "butterfly": char distance > 3 → high severity
	if err.Severity != "high" {
		t.Errorf("Severity = %q, want %q", err.Severity, "high")
	}
}

func TestCharLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "xyz", 3},
		{"abc", "abc", 0},
		{"parked", "pocked", 2},          // a->o, r->c
		{"catastrophe", "butterfly", 9},   // very different
		{"hello", "hallo", 1},            // single substitution
		{"kitten", "sitting", 3},         // classic example
	}

	for _, tt := range tests {
		got := charLevenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("charLevenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
