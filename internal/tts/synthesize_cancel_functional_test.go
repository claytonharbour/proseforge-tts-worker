//go:build functional

package tts

// Synthesis must stop when its context is cancelled (#114). Before, the engine took no context:
// a caller that disconnected or timed out left the engine finishing audio nobody would read, and
// because it runs one inference at a time, every request queued behind it waited for that work.
//
// The control is the same text with a live context: it must finish and produce audible audio, so
// the cancelled run stopping early is the cancellation and not a text that is too short to
// matter or an engine that fails anyway.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// A few paragraphs, so there are several inferences to stop between.
var cancelTestText = strings.Repeat("The ferry crossed the grey water slowly, and nobody on the deck said a word.\n\n", 6)

func TestSynthesizeContext_LiveContextFinishes(t *testing.T) {
	engine := newFunctionalEngine(t)
	wav, duration, err := engine.SynthesizeContext(context.Background(), cancelTestText, "af_sarah", "wav", 1.0, nil)
	if err != nil {
		t.Fatalf("SynthesizeContext with a live context: %v", err)
	}
	if duration < 5 {
		t.Fatalf("control synthesised only %.1fs, too short to show a cancellation", duration)
	}
	if f := decode(t, wav); len(f.Samples) == 0 {
		t.Fatal("control produced no samples")
	}
}

func TestSynthesizeContext_CancelledBeforeStartDoesNoWork(t *testing.T) {
	engine := newFunctionalEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	wav, _, err := engine.SynthesizeContext(ctx, cancelTestText, "af_sarah", "wav", 1.0, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got err=%v, want context.Canceled", err)
	}
	if len(wav) != 0 {
		t.Fatalf("a cancelled call returned %d bytes of audio", len(wav))
	}
	t.Logf("cancelled call returned in %s", time.Since(start).Round(time.Millisecond))
}

func TestSynthesizeContext_CancelledMidwayStopsEarly(t *testing.T) {
	engine := newFunctionalEngine(t)

	// How long the whole text takes, to compare against.
	full := time.Now()
	if _, _, err := engine.SynthesizeContext(context.Background(), cancelTestText, "af_sarah", "wav", 1.0, nil); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	fullTook := time.Since(full)

	ctx, cancel := context.WithTimeout(context.Background(), fullTook/4)
	defer cancel()
	start := time.Now()
	wav, _, err := engine.SynthesizeContext(ctx, cancelTestText, "af_sarah", "wav", 1.0, nil)
	took := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got err=%v, want context.DeadlineExceeded (took %s of a %s full run)", err, took, fullTook)
	}
	if len(wav) != 0 {
		t.Fatalf("a cancelled call returned %d bytes of audio", len(wav))
	}
	// It stopped well before the whole text was done. One inference may finish after the
	// deadline, so the bound is generous (3/4), not tight.
	if took > fullTook*3/4 {
		t.Fatalf("cancelled at %s but ran for %s of a %s full run: it did not stop", fullTook/4, took, fullTook)
	}
	t.Logf("full run %s; cancelled after %s, returned after %s", fullTook.Round(time.Millisecond), (fullTook / 4).Round(time.Millisecond), took.Round(time.Millisecond))
}
