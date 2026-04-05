package audio

import (
	"math"
	"testing"
)

func TestComputeWER(t *testing.T) {
	tests := []struct {
		name   string
		ref    string
		hyp    string
		wantWER float64
		wantSub int
		wantIns int
		wantDel int
	}{
		{
			name:    "perfect match",
			ref:     "hello world",
			hyp:     "hello world",
			wantWER: 0.0,
		},
		{
			name:    "one substitution",
			ref:     "hello world",
			hyp:     "hello earth",
			wantWER: 0.5,
			wantSub: 1,
		},
		{
			name:    "one deletion",
			ref:     "the big dog",
			hyp:     "the dog",
			wantWER: 1.0 / 3.0,
			wantDel: 1,
		},
		{
			name:    "one insertion",
			ref:     "the dog",
			hyp:     "the big dog",
			wantWER: 0.5,
			wantIns: 1,
		},
		{
			name:    "punctuation ignored",
			ref:     "Hello, world!",
			hyp:     "hello world",
			wantWER: 0.0,
		},
		{
			name:    "case ignored",
			ref:     "The Quick Brown Fox",
			hyp:     "the quick brown fox",
			wantWER: 0.0,
		},
		{
			name:    "empty reference",
			ref:     "",
			hyp:     "extra words",
			wantWER: 2.0, // 2 insertions / 0 ref words → technically infinite, but we use m
			wantIns: 2,
		},
		{
			name:    "empty hypothesis",
			ref:     "three missing words",
			hyp:     "",
			wantWER: 1.0,
			wantDel: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeWER(tt.ref, tt.hyp)
			if math.Abs(result.WER-tt.wantWER) > 0.001 {
				t.Errorf("WER = %.3f, want %.3f", result.WER, tt.wantWER)
			}
			if result.Substitutions != tt.wantSub {
				t.Errorf("Substitutions = %d, want %d", result.Substitutions, tt.wantSub)
			}
			if result.Insertions != tt.wantIns {
				t.Errorf("Insertions = %d, want %d", result.Insertions, tt.wantIns)
			}
			if result.Deletions != tt.wantDel {
				t.Errorf("Deletions = %d, want %d", result.Deletions, tt.wantDel)
			}
		})
	}
}

func TestNormalizeForWER(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"Hello, World!", []string{"hello", "world"}},
		{"it's a test", []string{"its", "a", "test"}},
		{"multiple   spaces", []string{"multiple", "spaces"}},
		{`"quoted text"`, []string{"quoted", "text"}},
		{"", nil},
	}

	for _, tt := range tests {
		got := normalizeForWER(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("normalizeForWER(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("normalizeForWER(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}
