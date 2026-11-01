//go:build unit

package phonemizer

import (
	"strings"
	"testing"
)

// @wayland asked directly whether this suite asserts the `skip` path. It did not —
// zero assertions, across every idiom I could think to search. His regression corpus
// held the only pin anywhere, and a path with one pin is one move from empty.
//
// ⚠️ THE GREP THAT NEARLY ANSWERED WRONG: `skip` appears 27 times in these test
// files and 22 of them are `t.Skip` — a different feature wearing the same word.
// A grep-and-glance reports "well covered". @crispin hit the identical shape hours
// earlier searching a mobile repo for "block" and finding eight markdown
// `BlockParagraph`/`BlockCode` hits. The literal `"skip"` appeared exactly once, in
// a classifier feeding t.Logf — no assertion at all.
//
// 🛑 WHAT `skip` MEANS, from phonemizer.go: it is the INITIAL value of `source`, and
// it survives only if custom, homograph, gold, silver, hyphenated AND lts all decline
// — lts included, since it returns "" for scripts it cannot letter-to-sound. So a
// `skip` word contributes NO SOUND WHATSOEVER. It is not a fallback reading; it is
// silence.
//
// 📌 That makes this path worth pinning on consequence, not just on coverage
// arithmetic: if a regression ever moved ordinary English words into `skip`, the
// narration would quietly lose them, and every other source assertion in this package
// would still pass. Four scripts are pinned rather than one so a single change cannot
// empty it — @wayland's own singleton argument applied to the fix for it.
//
// ⛔ NOT a defect claim. Silence is defensible here: Kokoro cannot voice 日本語, and
// emitting garbage would be worse. This records the behaviour and its boundary.
func TestSkipPathIsPinnedAcrossScripts(t *testing.T) {
	p := loadTestPhonemizer(t)

	cases := []struct{ text, word, why string }{
		{"He read 日本語 aloud.", "日本語", "CJK"},
		{"The sign said Привет there.", "Привет", "Cyrillic"},
		{"It was 🙂 apparently.", "🙂", "emoji"},
		{"The word ﷺ appeared.", "ﷺ", "Arabic ligature"},
	}

	for _, c := range cases {
		var found bool
		for _, d := range p.DiagnoseText(c.text) {
			if d.Word != c.word {
				continue
			}
			found = true
			if d.Source != "skip" {
				t.Errorf("%s: %q now resolves from %q with phonemes %q, not skip — "+
					"if a layer learned to voice this, that is an improvement and this pin "+
					"should move; if it is emitting a guess, that is worse than silence",
					c.why, c.word, d.Source, d.Phonemes)
			}
			if d.Phonemes != "" {
				t.Errorf("%s: %q carries phonemes %q while sourced skip — skip means "+
					"NO sound, so these two disagree", c.why, c.word, d.Phonemes)
			}
		}
		if !found {
			t.Fatalf("%s: %q produced no diagnosis row at all — the token did not survive "+
				"tokenization, so this specimen no longer exercises the skip path", c.why, c.word)
		}
	}

	// The control. Without it this passes on an engine where EVERYTHING is skipped,
	// which is the failure mode that would matter most.
	var english int
	for _, d := range p.DiagnoseText("He read the sign aloud there.") {
		if d.Source == "skip" {
			t.Errorf("ordinary English word %q resolved to skip — narration would lose it "+
				"silently, and every other source assertion here would still pass", d.Word)
		}
		if strings.TrimSpace(d.Phonemes) != "" {
			english++
		}
	}
	if english == 0 {
		t.Fatal("no English word produced phonemes — the control did not run, so the " +
			"skip assertions above prove nothing")
	}
}
