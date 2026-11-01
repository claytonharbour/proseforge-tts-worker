//go:build unit

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/claytonharbour/proseforge-tts-worker/internal/audio"
)

// TestPauseListingDiscriminatesRendersTheSummaryCannot pins the reason -pauses
// exists, as a pair rather than as an assertion about formatting.
//
// The two analyses below are the A/B shape that produced #37's wrong 340ms: same
// pause COUNT, same AVERAGE, same MAXIMUM, and a pause that sits somewhere else.
// The summary block is identical for both — that is not a bug in the summary, it
// is what a summary is — so a reader comparing two renders through it concludes
// "no difference" and is looking at the wrong thing.
//
// ⚑ The first assertion is the positive control and it must PASS as an equality.
// Without it this test could be satisfied by a printer that emits anything at
// all, and a listing that differs on two inputs that also differ everywhere else
// demonstrates nothing. The equality is what makes the inequality mean something.
func TestPauseListingDiscriminatesRendersTheSummaryCannot(t *testing.T) {
	// Same durations (310ms, 100ms) — only the first pause's POSITION moves.
	early := &audio.PauseAnalysis{
		TotalDuration: 3.0, SpeechDuration: 2.59, SilenceDuration: 0.41,
		SilenceRatio: 0.41 / 3.0, PauseCount: 2, AvgPause: 0.205, MaxPause: 0.31,
		Pauses: []audio.Pause{
			{StartSec: 0.50, EndSec: 0.81, Duration: 0.31},
			{StartSec: 2.00, EndSec: 2.10, Duration: 0.10},
		},
	}
	late := &audio.PauseAnalysis{
		TotalDuration: 3.0, SpeechDuration: 2.59, SilenceDuration: 0.41,
		SilenceRatio: 0.41 / 3.0, PauseCount: 2, AvgPause: 0.205, MaxPause: 0.31,
		Pauses: []audio.Pause{
			{StartSec: 1.19, EndSec: 1.50, Duration: 0.31},
			{StartSec: 2.00, EndSec: 2.10, Duration: 0.10},
		},
	}

	render := func(f func(*bytes.Buffer, *audio.PauseAnalysis), a *audio.PauseAnalysis) string {
		var b bytes.Buffer
		f(&b, a)
		return b.String()
	}
	summary := func(b *bytes.Buffer, a *audio.PauseAnalysis) { fprintAnalysis(b, a) }
	listing := func(b *bytes.Buffer, a *audio.PauseAnalysis) { fprintPauses(b, a) }

	// POSITIVE CONTROL: the summary genuinely cannot see the difference.
	if s1, s2 := render(summary, early), render(summary, late); s1 != s2 {
		t.Fatalf("the summary distinguishes these two renders, so this test's premise "+
			"is gone and its inequality below proves nothing:\n--- early ---\n%s--- late ---\n%s", s1, s2)
	}

	// THE FIX: the listing does.
	l1, l2 := render(listing, early), render(listing, late)
	if l1 == l2 {
		t.Errorf("-pauses renders two differently-positioned pause sets identically, so it "+
			"restores none of the discrimination it was added for:\n%s", l1)
	}
	if !strings.Contains(l2, "1.19") || !strings.Contains(l2, "1.50") {
		t.Errorf("-pauses omits the start/end times that align an A/B pair; got:\n%s", l2)
	}

	// A render with no pause at this threshold must say so rather than print an
	// empty block, which reads as "not measured" instead of "measured, none".
	none := render(listing, &audio.PauseAnalysis{TotalDuration: 3.0, SpeechDuration: 3.0})
	if !strings.Contains(none, "none") {
		t.Errorf("a pause-free analysis printed %q, which is indistinguishable from the "+
			"listing not having run", none)
	}
}
