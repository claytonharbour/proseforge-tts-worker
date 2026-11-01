//go:build unit

package phonemizer

import "testing"

// A homograph must be decided by evidence in its OWN clause. A trigger past a
// comma is a coincidence of proximity, and at windowSize 3 it outranks the real
// evidence sitting next door.
//
// #60's specimen, and the reason this exists:
//
//	"The difference, measured in minutes, is the difference between…"
//	                          ^^^^^^^                    ^^^^^^^^^^
//	`difference` is a mInjˈuːt trigger three words later, across the comma, so a
//	DURATION was spoken as the adjective — "my-NOOTS". @wayland measured 38 of
//	1,681 occurrences decided outside their own clause, 15 of them stress-moving.
//
// ⚠️ Red-proof these TOGETHER. Restoring the full window must turn the four
// boundary cases red while "trigger inside the clause still decides" stays
// GREEN — that arm is the control, and it is what would catch bounds so tight
// they disable the feature rather than narrow it.
//
// A batch where an arm stays green under injection is the finding, not a pass:
// the last case here originally used `every` as its antecedent, which is itself
// a noun trigger, so it asserted the same reading with and without the bounds.
// It survived one round of red-proofing looking exactly like a guard.
func TestHomographWindowStopsAtClauseBoundary(t *testing.T) {
	r, err := LoadHomographs("../../data/homographs.json")
	if err != nil {
		t.Fatalf("load homographs: %v", err)
	}

	const (
		adjective = "mInjˈuːt" // "my-NOOT" — tiny
		noun      = "mˈɪnɪt"   // "MIN-it"  — the duration
	)

	cases := []struct {
		name string
		// want is the required reading. Empty means "the resolver must DECLINE",
		// which is a correct outcome for an inflected form: the existing rule is
		// that an inflected homograph overrides the dictionary only when a trigger
		// actually matches, never by falling through to entry.Default. With the
		// boundary respected nothing matches, so gold answers instead — which is
		// where mˈɪnəts (MIN-its, + built -s) comes from.
		want      string
		words     []string
		idx       int
		wantNever string
		why       string
	}{{
		name:      "#60 verbatim: plural, trigger past the comma",
		words:     []string{"measured", "in", "minutes,", "is", "the", "difference"},
		idx:       2,
		want:      "", // declines — see above
		wantNever: adjective,
		why: "#60's actual sentence. `difference` is in the NEXT clause. Whether the " +
			"resolver declines or returns the noun, the one thing it must never do " +
			"is return the adjective — that is the my-NOOTS defect",
	}, {
		name:      "singular: trigger past the comma does not reach back",
		words:     []string{"measured", "in", "minute,", "is", "the", "difference"},
		idx:       2,
		want:      noun,
		wantNever: adjective,
		why: "same shape, uninflected, so the resolver stays engaged and must " +
			"return the duration outright rather than declining",
	}, {
		name:      "trigger inside the clause still decides",
		words:     []string{"the", "minute", "details", "matter"},
		idx:       1,
		want:      adjective,
		wantNever: noun,
		why:       "control: `details` is same-clause evidence and must still win",
	}, {
		name:      "trigger before a comma does not reach forward",
		words:     []string{"Wait,", "minute", "details", "matter"},
		idx:       1,
		want:      adjective,
		wantNever: noun,
		why: "`wait` is a noun trigger in the previous clause. Blocked, `details` " +
			"wins alone. Unblocked it ties and the tie falls to the default (#49), " +
			"so this case discriminates in BOTH directions",
	}, {
		name:      "a boundary on the target word cuts what follows",
		words:     []string{"the", "minute,", "details", "emerged"},
		idx:       1,
		want:      noun,
		wantNever: adjective,
		why: "`details` is past the target's OWN comma, so it must not decide. " +
			"⚠️ The antecedent is deliberately `the` — a non-trigger. An earlier " +
			"version used `every`, which IS a noun trigger, so with the bounds " +
			"removed it tied with `details` and the tie fell to the noun default " +
			"(#49) — the assertion passed either way and guarded nothing. Caught " +
			"by red-proofing the batch, which is the whole reason the batch exists",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := r.Resolve(tc.words, tc.idx)
			if ok && tc.wantNever != "" && got == tc.wantNever {
				t.Errorf("Resolve(%v, %d) = %q — the reading it must NEVER return\n  %s",
					tc.words, tc.idx, got, tc.why)
			}
			if tc.want == "" {
				if ok {
					t.Errorf("Resolve(%v, %d) = %q, expected the resolver to DECLINE\n  %s",
						tc.words, tc.idx, got, tc.why)
				}
				return
			}
			if !ok {
				t.Fatalf("Resolve(%q) declined; want %q\n  %s", tc.words[tc.idx], tc.want, tc.why)
			}
			if got != tc.want {
				t.Errorf("Resolve(%v, %d) = %q, want %q\n  %s",
					tc.words, tc.idx, got, tc.want, tc.why)
			}
		})
	}
}

// clauseBounds is the mechanism; assert it directly so a failure above can be
// localised to the bounds rather than to trigger data that may change.
func TestClauseBoundsNarrowsToTheClause(t *testing.T) {
	cases := []struct {
		name           string
		words          []string
		idx            int
		wantLo, wantHi int
	}{
		{"no punctuation uses the full window",
			[]string{"a", "b", "c", "d", "e", "f", "g"}, 3, 0, 6},
		{"comma before cuts the left edge",
			[]string{"a", "b,", "c", "d", "e", "f", "g"}, 3, 2, 6},
		{"comma after cuts the right edge",
			[]string{"a", "b", "c", "d,", "e", "f", "g"}, 3, 0, 3},
		{"quote after a comma is still a boundary",
			[]string{"a", "b", "c", `said,"`, "e", "f", "g"}, 2, 0, 3},
		{"window still clamps at the slice edges",
			[]string{"a", "b"}, 0, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lo, hi := clauseBounds(tc.words, tc.idx, 3)
			if lo != tc.wantLo || hi != tc.wantHi {
				t.Errorf("clauseBounds(%v, %d) = (%d,%d), want (%d,%d)",
					tc.words, tc.idx, lo, hi, tc.wantLo, tc.wantHi)
			}
		})
	}
}
