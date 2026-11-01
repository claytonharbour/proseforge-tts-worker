//go:build unit

package phonemizer

import "testing"

// `record` is the first homograph in this repo whose readings were settled by
// @clayton's ear rather than by counting, so they are pinned here — a later edit
// to homographs.json must not silently undo a decision a person made by listening.
//
// What he confirmed, on 2026-08-18 from calibration/ear-check:
//
//	"Clean driving record, no accidents."   NOUN  ɹˈɛkɔɹd   "the 1st record nailed it"
//	"They record every call."               VERB  ɹəkˈɔɹd   picked #3 of three candidates
//
// ⚑ The verb reading is gold's own VERB entry. The hand-written homograph string
// (ɹɪˈkɔɹd) lost to the dictionary's when an ear judged them — which is #63's
// thesis, confirmed on the one instance anybody has actually listened to.
//
// The noun triggers in|on|no come from variant C. @wayland measured it at 32
// fixed / 0 broken over 99 corpus occurrences ON THE DEPLOYED ENGINE — an
// earlier 33/0 predated the clause stop and was stale, which is why C was held.
func TestRecordReadings(t *testing.T) {
	p := newTestPhonemizer(t)

	const (
		noun = "ɹˈɛkɔɹd" // REK-ord
		verb = "ɹəkˈɔɹd" // ruh-KORD, gold's VERB entry
	)

	cases := []struct {
		text string
		want string
		why  string
	}{
		{"Clean driving record, no accidents.", noun,
			"@clayton by ear — the sentence the whole ticket was opened for"},
		{"They record every call.", verb,
			"@clayton by ear — he chose this of three rendered candidates"},
		{"Please record the interview.", verb, "explicit verb trigger"},
		{"No, record it again.", verb,
			"dialogue imperative. `no` is a noun trigger in the PREVIOUS clause and " +
				"must not reach it — this only passes because the clause stop (ef3e1b7) " +
				"blocks it, and it was one of C's three breaks before that landed"},
		{"Bring the mic in, record the whole thing.", verb,
			"same shape as above, with `in` across the comma"},
		{"No record of the call exists.", noun, "C's `no` trigger, same clause"},
		{"She finished in record time.", noun, "C's `in` trigger, same clause"},
		{"They went on record that this is political retaliation.", noun,
			"C's `on` trigger beats the distant `that`"},
	}

	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			if got := phonemesForWord(t, p, tc.text, "record"); got != tc.want {
				t.Errorf("%q → %q, want %q\n  %s", tc.text, got, tc.want, tc.why)
			}
		})
	}
}

// A bare imperative has no subject and no trigger in its clause, so it falls to
// entry.Default — and flipping that default to the noun is what makes it wrong.
// No trigger list reaches it, because there is no trigger to add; the clause stop
// does not reach it either, because there is nothing in the clause to block.
//
// ⛔ This asserts the CURRENT WRONG READING deliberately. It is a known limitation
// with no available fix, not an oversight, and pinning it means the day somebody
// does fix it this test fails and says so rather than passing silently.
//
// @wayland measured ZERO bare-imperative `record` in 619,583 words, so nothing in
// the shipping corpus hits it. It fails identically under every variant considered
// (P, D and C alike) — it is the irreducible cost of flipping a default.
func TestBareImperativeRecordIsKnownWrong(t *testing.T) {
	p := newTestPhonemizer(t)

	const text = "From here on, record every call."
	const wrongToday = "ɹˈɛkɔɹd" // the noun; a listener wants ɹəkˈɔɹd here

	got := phonemesForWord(t, p, text, "record")
	if got != wrongToday {
		t.Errorf("%q → %q.\n"+
			"This test pins a KNOWN-WRONG reading. If it now says ɹəkˈɔɹd the "+
			"bare-imperative class has been fixed — delete this test and close the "+
			"ticket tracking it. Any OTHER value means something unexpected moved.",
			text, got)
	}
}

// Subject–auxiliary inversion puts a bare verb after a third-person pronoun, and
// the trigger list does not cover it:
//
//	"When your cooling system adjusts, does he record why?"   → ɹˈɛkɔɹd  🛑
//
// ⛔ THIS PINS A KNOWN-WRONG READING, like the bare imperative above.
//
// It exists to correct a rationale, not only to record a defect. `he`/`she`/`who`
// were pruned from the verb triggers in 9b2eef2 with two reasons given, and only
// one of them was sound:
//
//	✅ EVIDENCE   zero corpus occurrences — they were never exercised here
//	🛑 RATIONALE  "third-person agreement makes `he record` ungrammatical, so they
//	              could never fire" — WRONG. @wayland found the counterexample:
//	              in an inversion, `does` carries the agreement and the verb goes
//	              bare, so "does he record why?" is perfectly good English.
//
// ⚑ Why the distinction is worth a test rather than a comment on a closed ticket:
// a trigger dropped for being UNEXERCISED gets restored the day a corpus
// exercises it. A trigger dropped for being IMPOSSIBLE never does. The wrong
// reason was written into a commit message, where it would have blocked the
// restore forever.
//
// Restore condition: a corpus containing `does <pronoun> record` frames. The
// candidate fix is broader than the three pronouns — `does` is currently a
// trigger for NEITHER reading, so adding it to the verb's context_before would
// cover the whole inversion class at once. ⚠️ It needs its own count first:
// "Does record time matter?" is a noun with `does` adjacent, and would break.
func TestSubjectAuxiliaryInversionIsKnownWrong(t *testing.T) {
	p := newTestPhonemizer(t)

	const text = "When your cooling system adjusts, does he record why?"
	const wrongToday = "ɹˈɛkɔɹd" // the noun; a listener wants ɹəkˈɔɹd

	// The control: the same frame with a pronoun that IS still a trigger. If this
	// stops being the verb, the trigger list has moved and the pin below is
	// measuring something else entirely.
	if got := phonemesForWord(t, p, "When your cooling system adjusts, does it record why?", "record"); got != "ɹəkˈɔɹd" {
		t.Fatalf("CONTROL: `does it record` = %q, want the verb — the frame itself has changed", got)
	}

	if got := phonemesForWord(t, p, text, "record"); got != wrongToday {
		t.Errorf("`does he record` = %q.\n"+
			"This pins a KNOWN-WRONG reading. If it now says ɹəkˈɔɹd the inversion "+
			"class has been fixed — delete this test and note it on #61. Any OTHER "+
			"value means something unexpected moved.", got)
	}
}

// `present` is the second homograph whose reading was settled by @clayton's ear,
// and the second where the hand-written homograph string lost to gold's.
//
// He was asked which of two SENSE errors was worse and answered that both clips
// were bad, describing what he heard as "pree-asant" and "a-pree-asant" —
// neither of the two readings on offer. The question was about sense selection;
// the defect was in the strings.
//
//	was    pɹɪˈzɛnt  /  ˈpɹɛzənt      hand-written
//	now    pɹizˈɛnt  /  pɹˈɛzᵊnt      us_gold.json's own VERB and DEFAULT
//
// ⚑ The SENSE was already right by the time he listened — ef3e1b7's clause stop
// fixed "Not present, but the subject…", where a trigger past the comma had been
// choosing the verb. So the two halves came apart cleanly: the window decided
// which reading, and gold decided what that reading sounds like.
//
// That is #63 confirmed twice by ear now, on the only two keys anybody has
// actually listened to.
func TestPresentReadings(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct{ text, want, why string }{
		{"We can't present the murder case.", "pɹizˈɛnt",
			"verb. @clayton confirmed gold's string in present-gold.wav"},
		{"Not present, but the subject of the first ten minutes.", "pɹˈɛzᵊnt",
			"adjective. The sense here is the one ef3e1b7 fixed; the string is gold's"},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			if got := phonemesForWord(t, p, tc.text, "present"); got != tc.want {
				t.Errorf("%q → %q, want %q\n  %s", tc.text, got, tc.want, tc.why)
			}
		})
	}
}

// `minute`'s adjective reading no longer triggers on `most`.
//
// @wayland's regression corpus surfaced it: `most` fired on "most under four
// minutes" and "the most critical minutes", where it quantifies the DURATION and
// has nothing to do with the tiny sense. 2 corpus occurrences fixed, 0 corpus
// regressions.
//
// ⚑ It cost nothing because in ordinary adjective use the FOLLOWING noun already
// carries the reading — "most minute detail" fires on `detail`, "most minute
// difference" on `difference` — and even a trailing form like "the change was
// most minute" survives, because the direction-blind second pass matches `change`
// behind the target (#65's mechanism doing useful work for once).
//
// ⛔ KNOWN LIMITATION, pinned below rather than left implicit. An elliptical
// "the most minute" with no noun anywhere loses its only trigger and falls to the
// noun default. That shape is absent from the corpus — @wayland measured 0
// regressions across all occurrences — and it is stilted English, but it is a
// real cost and it should be visible to whoever revisits this.
func TestMinuteMostTriggerRemoved(t *testing.T) {
	p := newTestPhonemizer(t)

	const (
		adjective = "mInjˈuːt" // tiny
		nounPl    = "mˈɪnəts"  // durations, from gold
	)

	for _, tc := range []struct{ text, word, want, why string }{
		{"It took most under four minutes.", "minutes", nounPl,
			"`most` quantifies the duration — this is why the trigger went"},
		{"The most critical minutes were lost.", "minutes", nounPl,
			"same shape, second corpus occurrence"},
		{"He described the most minute detail of the room.", "minute", adjective,
			"CONTROL: genuine adjective use survives, carried by `detail` after it"},
		{"The change was most minute.", "minute", adjective,
			"CONTROL: carried by `change` BEHIND it, via the direction-blind pass 2"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			if got := phonemesForWord(t, p, tc.text, tc.word); got != tc.want {
				t.Errorf("%q → %q, want %q\n  %s", tc.text, got, tc.want, tc.why)
			}
		})
	}
}

// ⛔ PINS A KNOWN-WRONG READING, like the bare imperative above.
//
// An elliptical "the most minute" has no noun after it and no adjective trigger
// behind it, so removing `most` leaves nothing to fire and it falls to the noun
// default. Zero occurrences in the corpus; stilted but real English.
//
// If this ever says mInjˈuːt the class has been fixed — delete this test.
func TestEllipticalMostMinuteIsKnownWrong(t *testing.T) {
	p := newTestPhonemizer(t)

	// Control: the same word one sentence over, where a trigger DOES exist.
	if got := phonemesForWord(t, p, "He described the most minute detail.", "minute"); got != "mInjˈuːt" {
		t.Fatalf("CONTROL: a triggered adjective gave %q — the frame itself has moved", got)
	}
	if got := phonemesForWord(t, p, "He spoke of the most minute.", "minute"); got != "mˈɪnɪt" {
		t.Errorf("elliptical `the most minute` = %q. This pins a KNOWN-WRONG reading; "+
			"if it now says mInjˈuːt the class is fixed — delete this test.", got)
	}
}

// Two triggers that DISAGREE at equal distance fall to the default (#65).
//
//	"A minute difference in temperature."
//	 a@1 (time sense) ─┬─ tie ─┬─ difference@1 (tiny sense)
//	                   └─ entry.Default wins → mˈɪnɪt, the WRONG sense
//
// ⚠️ This is a consequence of #49's determinism fix, not a regression. Go's
// randomised map order made `wound` resolve differently between runs and break
// reproducible narration; resolving ties toward entry.Default fixed that and was
// correct. The cost is that a tie between DISAGREEING triggers — conflicting
// evidence — is handled identically to a word with no triggers near it at all,
// which is absent evidence. Those are different states.
//
// ⛔ NO FIX IS PINNED HERE because every obvious one is worse:
//
//	prefer the more specific trigger   nothing in the data ranks specificity;
//	                                   `a` vs `difference` is asymmetric only to a human
//	prefer after over before on a tie  arbitrary, and it breaks "a last minute
//	                                   change" — measured, see the control below
//	widen the window / weight by rarity  a scoring change to a resolver whose
//	                                   determinism was hard-won, with its own tie cases
//
// ⚑ MEASURED 2026-08-18, and the answer is that the tie-break is CORRECT:
//
//	corpus, @wayland   30 equidistant conflicts · 30/30 → default · 29 CORRECT · 1 wrong
//	my grid            45 disagreeing pairs     · 45/45 → default ·  1 correct
//
// 🛑 THOSE TWO LOOK IDENTICAL AND ANSWER OPPOSITE QUESTIONS, and mine is the
// misleading one. My 45 were every combination of `minute`'s before- and
// after-triggers — a grid of the MECHANISM, most of whose cells are sentences
// nobody writes. "1 of 45 correct" is therefore a property of the grid, not a
// defect rate. Where these ties actually occur, the default is right 29 times
// in 30, and 18 of the 30 are the single idiom `had read about`.
//
// 📌 So this test pins the mechanism and MUST NOT be read as evidence the
// mechanism is wrong. A constructed grid measures what a rule DOES; only the
// corpus measures what it COSTS. I made the same substitution on #69 the same
// afternoon — picking specimens that demonstrate a mechanism cleanly selects
// against the cases that decide the fix.
//
// ⛔ #65 was closed on this: fixing the tie-break would break 29 to fix 1. The
// one genuine defect is `live`, which is a trigger-configuration problem in that
// entry rather than anything about ties, and has its own ticket.
func TestEquidistantDisagreementResolvesToDefault(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct {
		text       string
		want       string
		knownWrong bool
		why        string
	}{{
		text: "A minute difference in temperature.", want: "mˈɪnɪt", knownWrong: true,
		why: "a@1 and difference@1 tie; the default takes it and a listener wants mInjˈuːt",
	}, {
		// ⚠️ THE CONTROL THAT FORBIDS THE OBVIOUS FIX, and #65 named the wrong
		// sentence for this job. `last`@1 (time) and `change`@1 (tiny) disagree
		// at equal distance, and here the default is RIGHT — "a change made at
		// the last minute". Disabling the tie-break turns this into "a last
		// MY-newt change". MEASURED, not reasoned: it is the only one of the 45
		// disagreeing pairs where mˈɪnɪt is the correct English reading.
		text: "There was a last minute change to the schedule.", want: "mˈɪnɪt",
		why: "`prefer after on a tie` would fix `a minute difference` and BREAK this",
	}, {
		// ⛔ DECORATIVE, and kept labelled rather than deleted. #65 cited this as
		// the forbidding control — "same a@1 tie" — but `a` and `ago` BOTH point
		// to mˈɪnɪt, so the triggers AGREE and the tie-break never runs. It does
		// not move under any injection, which is how it was caught: the pin fired
		// on one arm and this stayed green when it should have been the arm that
		// proved the point.
		//
		// 📌 A tie is not a conflict. #65's conclusion was right and its evidence
		// was not, which is the harder kind to notice — nothing about a passing
		// control says whether it could ever fail.
		text: "A minute ago the lights went out.", want: "mˈɪnɪt",
		why: "an AGREEING tie — both triggers give mˈɪnɪt, so this discriminates nothing",
	}, {
		text: "Their minute difference in temperature.", want: "mInjˈuːt",
		why: "no before-trigger, so no tie — the after-trigger simply wins",
	}, {
		text: "A truly minute difference in temperature.", want: "mInjˈuːt",
		why: "a@2 vs difference@1 — DISTANCE decides, not direction. This specimen is " +
			"why the ticket is not titled 'before beats after'; three others supported " +
			"that reading and it is wrong",
	}, {
		text: "A minute but real difference.", want: "mˈɪnɪt",
		why: "a@1 vs difference@3 — before is genuinely closer, correctly",
	}}

	for _, tc := range cases {
		got := phonemesForWord(t, p, tc.text, "minute")
		if got == tc.want {
			continue
		}
		if tc.knownWrong {
			t.Errorf("%q → %q, was %q.\nThis pinned a KNOWN-WRONG reading. If it now "+
				"says mInjˈuːt the equidistant-conflict class has been fixed — delete "+
				"this arm and close #65. Any OTHER value means something unexpected moved.\n  %s",
				tc.text, got, tc.want, tc.why)
			continue
		}
		t.Errorf("%q → %q, want %q\n  %s", tc.text, got, tc.want, tc.why)
	}
}
