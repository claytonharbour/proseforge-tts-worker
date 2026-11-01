//go:build functional

package tts

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Regression test: on 2026-09-05 a production audiobook lost both its intro and its
// outro. The branding TTS call (99 characters) timed out on a serverless endpoint that
// had just completed 27 narration segments of ~2,900 characters each. The shortest
// request of the run was the only one that failed, and the worker had never been
// exercised on branding text at all.
//
// Branding text is not narration text. It is the only string in an audiobook run that
// contains "ProseForge" (a coined proper noun in no dictionary, so it reaches the LTS
// fallback), and it carries the story title and author, which are arbitrary user input.
//
// A pass here does not clear the worker in production: it exercises this engine on this
// host, not a cold serverless container. What it can do is falsify "the worker hangs on
// branding text".
func TestBrandingTemplatesSynthesizeWithoutHanging(t *testing.T) {
	engine := newFunctionalEngine(t)

	// The production defaults, from config.FeatureConfig.AudiobookBranding{Intro,Outro}Template.
	// Copied literally: a paraphrase would test a string prod never sends.
	const (
		introTemplate = "From ProseForge, this is the audiobook presentation of {title}, written by {author}."
		outroTemplate = "This has been a ProseForge audiobook presentation of {title} by {author}. " +
			"We hope you enjoyed the story."
	)

	// Titles chosen for the properties branding text has and narration text does not:
	// arbitrary user input in the middle of a sentence, including the shapes most
	// likely to defeat a phonemizer.
	cases := []struct{ name, title, author string }{
		{"plain", "The Quiet Hour", "Clayton Harbour"},
		{"numeral", "Chapter 7", "A. B. Cummings"},
		{"punctuation", "Don't Look Back!", "J.R.R. O'Neill"},
		{"initialism", "The NASA Files", "R. L. Byrne"},
		{"unicode", "Café Noir", "Renée Dupré"},
		{"empty-author", "Untitled", ""},
	}

	for _, tc := range cases {
		for _, tmpl := range []struct{ kind, text string }{
			{"intro", introTemplate}, {"outro", outroTemplate},
		} {
			t.Run(tc.name+"/"+tmpl.kind, func(t *testing.T) {
				text := strings.NewReplacer("{title}", tc.title, "{author}", tc.author).Replace(tmpl.text)

				start := time.Now()
				wav, duration, err := engine.Synthesize(text, "am_michael", "wav", 1.0, nil)
				elapsed := time.Since(start)
				if err != nil {
					t.Fatalf("branding %s for %q FAILED: %v\n  text: %q",
						tmpl.kind, tc.title, err, text)
				}

				// ⇒ THE assertion this test exists for. The prod failure was a job that
				// never returned, so a wall-clock bound is the only thing that can
				// distinguish "synthesised correctly" from "synthesised eventually".
				// A ~100-character string is seconds on any host that can run the model;
				// 30s is loose enough not to go red on a slow laptop and tight enough
				// that a hang cannot pass.
				if elapsed > hangBound() {
					t.Errorf("branding %s took %v (bound %v) for %d characters — a hang, not slow synthesis.\n  text: %q",
						tmpl.kind, elapsed, hangBound(), len(text), text)
				}

				f := decode(t, wav)
				if e := rms(f.Samples); e < 1e-6 {
					t.Errorf("branding %s is SILENT (energy %.3g) — the listener hears nothing "+
						"where the credits should be.\n  text: %q", tmpl.kind, e, text)
				}
				t.Logf("  %-12s %-6s %3d chars -> %.2fs audio in %v", tc.name, tmpl.kind,
					len(text), duration, elapsed.Round(time.Millisecond))
			})
		}
	}
}

// hangBound is the wall-clock ceiling for one branding synthesis, overridable so that
// anyone can drive this test red without editing it. The repo has already learned that
// a test only its author can falsify is one only its author can trust:
//
//	BRANDING_HANG_BOUND_MS=1 make dev-test-functional RUN=TestBrandingTemplates... VERBOSE=1
//
// must report every subtest failing on the timing arm. If it does not, the arm is dead.
func hangBound() time.Duration {
	if v := os.Getenv("BRANDING_HANG_BOUND_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 30 * time.Second
}
