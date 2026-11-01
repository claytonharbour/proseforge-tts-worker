//go:build unit

package phonemizer

import "testing"

// `live` was wrong in BOTH directions on the same trigger (#70).
//
//	"gone live at 22:17"          lɪv   gone@1 (lIv) TIED at@1 (lɪv) → default
//	"gone on to live thirty…"     lIv   gone@3 (lIv) imposed across three words
//	"We go live in the studio."   lɪv   go@1 TIED in@1 → default          ← unfiled until now
//
// `lɪv` is the DEFAULT and its before-list was EMPTY, so every competing case
// was decided by whichever after-trigger happened to sit at distance 1.
//
// Two edits, and they are only correct TOGETHER:
//
//	A  drop `at`/`in` from the default's after-list   fixes defects 1 and 3
//	B  add `to` to the default's before-list          fixes defect 2
//
// ⚠️ A ALONE BREAKS FIVE SENTENCES — "They went to live at the coast" → lIv.
// My reasoning for A was that a trigger on the default is redundant, since
// absence falls back to the same pronunciation. That is true only when it is the
// ONLY trigger; where it defeats a competing one — its entire purpose — removing
// it flips the result. I wrote that sentence before measuring and still had to be
// shown the five.
//
// ⚑ B repairs every one of them, and not by luck: all five are `went|gone|going
// to live at|in …`, so B's `to` fires at distance 1 and beats the distant lIv
// trigger that A exposed. Two edits that are individually wrong and jointly
// right — which is why they must never be split, and why the five stay in this
// file as arms rather than being deleted once green.
//
// @wayland's corpus (2026-08-06): live 47 occ, 45 correct, 2 wrong — both fixed
// here. A's regression set 0, B's regression set 0. ⛔ His own caveat, and it is
// the reason the pins below exist: "my 0 does not refute your 5 — unpaid here is
// not safe." Those sentences are ordinary English that this corpus happens not
// to contain.
func TestLiveTriggerDisambiguation(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct {
		text, want, why string
		knownWrong      bool
	}{
		// --- the three defects ---
		{text: "The feed had gone live at 22:17.", want: "lˈIv",
			why: "DEFECT 1 — gone@1 tied at@1 and lost to the default"},
		{text: "He had gone on to live thirty more years.", want: "lˈɪv",
			why: "DEFECT 2 — gone@3 imposed the broadcast sense across three words"},
		{text: "We go live in the studio.", want: "lˈIv",
			why: "DEFECT 3 — go@1 tied in@1. Unfiled until the matrix was built"},

		// --- A's five, which B rescues. NOT decoration: A alone turns every one lIv ---
		{text: "They went to live at the coast.", want: "lˈɪv", why: "A-alone → lIv"},
		{text: "She has gone to live in Spain.", want: "lˈɪv", why: "A-alone → lIv"},
		{text: "He is going to live at home again.", want: "lˈɪv", why: "A-alone → lIv"},
		{text: "We go to live in the country each summer.", want: "lˈɪv", why: "A-alone → lIv"},
		{text: "They went on to live at the lake.", want: "lˈɪv", why: "A-alone → lIv"},

		// --- controls: must not move ---
		{text: "They watched the live broadcast.", want: "lˈIv", why: "CONTROL after-trigger"},
		{text: "She wanted to live abroad.", want: "lˈɪv", why: "CONTROL infinitive"},
		{text: "They live at the edge of town.", want: "lˈɪv", why: "CONTROL — `at` case A removed"},
		{text: "I live in Chicago.", want: "lˈɪv", why: "CONTROL — `in` case A removed"},
		{text: "He will live here alone.", want: "lˈɪv", why: "CONTROL untouched trigger"},
		{text: "The live stream started.", want: "lˈIv", why: "CONTROL"},

		// --- KNOWN-WRONG: `to live <noun>`, the broadcast sense after `to` ---
		{text: "We go to live pictures now.", want: "lˈɪv", knownWrong: true,
			why: "B's cost. `to`@1 (lɪv) beats go@2 (lIv). @wayland: 0 in corpus"},
		{text: "We cut to live coverage of the fire.", want: "lˈɪv", knownWrong: true,
			why: "ALREADY wrong before #70 — no lIv trigger fires at all, so this is " +
				"pre-existing rather than a cost of B. Pinned to keep the two distinct"},
	}

	for _, tc := range cases {
		got := phonemesForWord(t, p, tc.text, "live")
		if got == tc.want {
			continue
		}
		if tc.knownWrong {
			t.Errorf("%q → %q, was %q.\nThis pinned a KNOWN-WRONG reading. If it now says "+
				"lIv the `to live <noun>` class has been fixed — delete this arm. Any "+
				"OTHER value means something unexpected moved.\n  %s", tc.text, got, tc.want, tc.why)
			continue
		}
		t.Errorf("%q = %q, want %q\n  %s", tc.text, got, tc.want, tc.why)
	}
}
