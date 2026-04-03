//go:build unit

package tts

import (
	"math"
	"testing"
)

func TestCrossfade(t *testing.T) {
	a := []float32{1.0, 1.0, 1.0, 1.0, 1.0}
	b := []float32{0.0, 0.0, 0.0, 0.0, 0.0}
	fadeLen := 3

	result := crossfade(a, b, fadeLen)

	// Expected length: 5 + 5 - 3 = 7
	expectedLen := len(a) + len(b) - fadeLen
	if len(result) != expectedLen {
		t.Fatalf("crossfade length: got %d, want %d", len(result), expectedLen)
	}

	// Non-overlapping part of a: [1.0, 1.0]
	for i := 0; i < 2; i++ {
		if result[i] != 1.0 {
			t.Errorf("result[%d] = %f, want 1.0", i, result[i])
		}
	}

	// Overlap region blends using Hann window: w = 0.5*(1-cos(π*t))
	// At i=0: t=0/3, w=0.0, result = 1.0*(1-0) + 0.0*0 = 1.0
	// At i=1: t=1/3, w=0.25, result = 1.0*0.75 + 0.0*0.25 = 0.75
	// At i=2: t=2/3, w=0.75, result = 1.0*0.25 + 0.0*0.75 = 0.25
	if math.Abs(float64(result[2]-1.0)) > 0.01 {
		t.Errorf("overlap[0]: got %f, want ~1.0", result[2])
	}
	if math.Abs(float64(result[3]-0.75)) > 0.01 {
		t.Errorf("overlap[1]: got %f, want ~0.75", result[3])
	}
	if math.Abs(float64(result[4]-0.25)) > 0.01 {
		t.Errorf("overlap[2]: got %f, want ~0.25", result[4])
	}

	// Non-overlapping part of b: [0.0, 0.0]
	for i := 5; i < 7; i++ {
		if result[i] != 0.0 {
			t.Errorf("result[%d] = %f, want 0.0", i, result[i])
		}
	}
}

func TestCrossfadeTooShort(t *testing.T) {
	a := []float32{1.0}
	b := []float32{0.0}

	// fadeLen > len(a), should just concatenate
	result := crossfade(a, b, 5)
	if len(result) != 2 {
		t.Errorf("got length %d, want 2", len(result))
	}
}

func TestCrossfadeZeroFade(t *testing.T) {
	a := []float32{1.0, 2.0}
	b := []float32{3.0, 4.0}

	result := crossfade(a, b, 0)
	if len(result) != 4 {
		t.Errorf("got length %d, want 4", len(result))
	}
}

func TestFindClausePausePositions(t *testing.T) {
	// Token IDs: comma=3, semicolon=1, em dash=9, space=16
	// Need ≥4 words (≥3 space tokens) before a clause boundary to qualify

	tests := []struct {
		name   string
		tokens []int64
		want   []int // expected token indices
	}{
		{
			"comma after 4 words",
			// word space word space word space word comma
			[]int64{44, 16, 44, 16, 44, 16, 44, 3},
			[]int{7}, // comma at index 7
		},
		{
			"comma after too few words",
			// word space word comma
			[]int64{44, 16, 44, 3},
			nil,
		},
		{
			"em dash after 4 words",
			[]int64{44, 16, 44, 16, 44, 16, 44, 9},
			[]int{7},
		},
		{
			"semicolon after 5 words",
			[]int64{44, 16, 44, 16, 44, 16, 44, 16, 44, 1},
			[]int{9},
		},
		{
			"two qualifying boundaries",
			// 4 words + comma + 4 words + semicolon
			[]int64{44, 16, 44, 16, 44, 16, 44, 3, 44, 16, 44, 16, 44, 16, 44, 1},
			[]int{7, 15},
		},
		{
			"no punctuation",
			[]int64{44, 16, 44, 16, 44, 16, 44},
			nil,
		},
		{
			"space count resets after qualifying boundary",
			// 4 words + comma + 2 words + comma (should not qualify)
			[]int64{44, 16, 44, 16, 44, 16, 44, 3, 44, 16, 44, 3},
			[]int{7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findClausePausePositions(tt.tokens)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("position[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestInsertSilenceAt_FadeEdges(t *testing.T) {
	// Create a waveform of constant 1.0 values
	waveform := make([]float32, 1000)
	for i := range waveform {
		waveform[i] = 1.0
	}

	silenceLen := 100
	fadeLen := 10
	pos := 500

	result := insertSilenceAt(waveform, pos, silenceLen, fadeLen)

	// Check total length
	wantLen := len(waveform) + silenceLen
	if len(result) != wantLen {
		t.Fatalf("length: got %d, want %d", len(result), wantLen)
	}

	// Before fade-out region: should be untouched
	if result[pos-fadeLen-1] != 1.0 {
		t.Errorf("before fade-out: got %f, want 1.0", result[pos-fadeLen-1])
	}

	// End of fade-out (last sample before silence): should be near zero
	// At i=fadeLen-1, t=(fadeLen-1)/fadeLen=0.9, value = 1.0 * 0.1 = 0.1
	if result[pos-1] > 0.15 {
		t.Errorf("fade-out end: got %f, want ~0.0", result[pos-1])
	}

	// Check silence region is zero
	for i := pos; i < pos+silenceLen; i++ {
		if result[i] != 0 {
			t.Errorf("silence at %d: got %f, want 0", i, result[i])
			break
		}
	}

	// Check fade-in: samples after silence should rise from 0
	if result[pos+silenceLen] > 0.15 {
		t.Errorf("fade-in start: got %f, want ~0.0", result[pos+silenceLen])
	}
	// After fade-in, should be back to 1.0
	if result[pos+silenceLen+fadeLen] != 1.0 {
		t.Errorf("after fade-in: got %f, want 1.0", result[pos+silenceLen+fadeLen])
	}
}

func TestInsertClausePauses_NoPositions(t *testing.T) {
	waveform := []float32{1.0, 2.0, 3.0, 4.0, 5.0}
	// Tokens with no qualifying clause boundaries (too few words)
	tokens := []int64{44, 16, 44, 3} // only 2 words before comma

	result := insertClausePauses(waveform, tokens, 100)

	if len(result) != len(waveform) {
		t.Fatalf("length changed: got %d, want %d", len(result), len(waveform))
	}
	for i := range waveform {
		if result[i] != waveform[i] {
			t.Errorf("sample[%d] changed: got %f, want %f", i, result[i], waveform[i])
		}
	}
}

func TestInsertClausePauses_InsertsCorrectSilence(t *testing.T) {
	// Create a waveform
	waveform := make([]float32, 2400)
	for i := range waveform {
		waveform[i] = 0.5
	}

	// Tokens: 4 words + comma — one qualifying boundary
	tokens := []int64{44, 16, 44, 16, 44, 16, 44, 3, 44, 16, 44}
	silenceLen := 360

	result := insertClausePauses(waveform, tokens, silenceLen)

	// Waveform should grow by exactly silenceLen
	wantLen := len(waveform) + silenceLen
	if len(result) != wantLen {
		t.Errorf("length: got %d, want %d", len(result), wantLen)
	}
}
