//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// TestMarkdownCodeSpansAreStripped pins that backticks lose their ticks and keep
// their words (#42). Asterisks were already handled; backticks were not, so
// `gobble` reached the dictionary as "`gobble`" and fell to LTS — 127 inline
// spans across 5 books.
//
// Each case carries a CONTROL: the same sentence without ticks. The two must
// produce IDENTICAL output. Asserting only "no backtick survives" would pass
// against a rule that deleted the word along with its ticks.
func TestMarkdownCodeSpansAreStripped(t *testing.T) {
	cases := []struct{ name, ticked, control string }{
		{"inline span", "He said `gobble` and nothing else.", "He said gobble and nothing else."},
		{"span mid-sentence", "The `correlation_id` was none.", "The correlation_id was none."},
		{"fenced block keeps its CONTENTS",
			"A single entry:\n```\n02:17:33.847 source=unknown\n```\n",
			"A single entry:\n\n02:17:33.847 source=unknown\n\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeText(c.ticked)
			want := NormalizeText(c.control)
			if strings.Contains(got, "`") {
				t.Errorf("backtick survived: %q", got)
			}
			if got != want {
				t.Errorf("ticked and control diverge\n  ticked : %q\n  control: %q", got, want)
			}
		})
	}
}

// TestFencedBlockContentSurvives is the case @Smiley flagged as the trap: the
// single fenced block in 350 sections IS the plot device of The Keeper's Door.
// Dropping the block instead of its markers removes the mystery from the
// audiobook and leaves the §10 callback dangling.
func TestFencedBlockContentSurvives(t *testing.T) {
	in := "```\n02:17:33.847 [WARN] source=unknown message=\"gobble\"\n```"
	got := NormalizeText(in)
	for _, must := range []string{"gobble", "unknown"} {
		if !strings.Contains(got, must) {
			t.Errorf("fence content %q was DROPPED, not unwrapped: %q", must, got)
		}
	}
	if strings.Contains(got, "`") {
		t.Errorf("fence markers survived: %q", got)
	}
}
