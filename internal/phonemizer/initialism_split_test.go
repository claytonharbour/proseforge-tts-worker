//go:build unit

package phonemizer

import (
	"reflect"
	"testing"
)

// A period inside a dotted initialism is not a sentence end.
//
// #67, and the third instance of one class — the other two are guards on the
// same line of SplitSentences:
//
//	#17  isAbbreviationDot   Dr. Mr. Mrs.     a word list
//	#59  isDecimalPoint      $2.8  0.001%     a digit either side
//	#67  isInitialismDot     a.m.  A.R.       a SINGLE letter either side
//
// The cost was prosodic rather than phonetic: "It was 9 a.m. sharp." split into
// THREE fragments, each tokenized as its own sentence, so a boundary landed
// inside a time.
//
// ⚑ Built on @wayland's count, not on the two specimens I happened to find:
// 156 period-flanked single-letter runs, 51 defensibly mid-sentence (a lowercase
// word or comma/colon follows, and no English sentence ends that way). 69 more
// are ambiguous — a capital follows — and those are LEFT SPLITTING on purpose,
// because a capital is as consistent with a new sentence as with a continuation.
func TestInitialismDotDoesNotSplitSentences(t *testing.T) {
	cases := []struct {
		in   string
		want []string
		why  string
	}{
		{"It was 9 a.m. sharp.", []string{"It was 9 a.m. sharp."},
			"#67's specimen — was three fragments"},
		{"The meeting ran to 5 p.m. and everyone left.",
			[]string{"The meeting ran to 5 p.m. and everyone left."},
			"p.m. mid-sentence, lowercase follows"},
		{"7:43 p.m.: Nadine Osei sends the file.",
			[]string{"7:43 p.m.: Nadine Osei sends the file."},
			"@wayland's most frequent corpus shape, 10 occurrences — colon follows"},

		// ⚠️ CONTROLS. Each of these MUST still split, and they are what stops
		// the guard being "never split at a period".
		// ⚠️ NOT a pure control, and the red-proof is what said so. It is the control
		// for isInitialismTrailingDot — which must NOT fire here, because a capital
		// follows and that is as consistent with a new sentence as a continuation —
		// but it DEPENDS on isInitialismDot, since bypassing that one splits `9 a.`
		// from `m.` and yields three fragments rather than two. It went red under one
		// injection and green under the other; a "control" that only ever stays green
		// would not have shown which guard owns which behaviour.
		{"The meeting is at 9 a.m. Then we left.",
			[]string{"The meeting is at 9 a.m.", "Then we left."},
			"a.m. ending a real sentence — a CAPITAL follows, so the trailing dot splits"},
		{"She grew up in Ohio. And then she left.",
			[]string{"She grew up in Ohio.", "And then she left."},
			"CONTROL: `o` in Ohio is preceded by a letter, so the run is a word"},
		{"He stopped. She did not.",
			[]string{"He stopped.", "She did not."},
			"CONTROL: an ordinary sentence end"},
		{"Dr. Goss arrived.", []string{"Dr. Goss arrived."},
			"CONTROL: #17's guard still holds"},
		{"It cost $2.8 million.", []string{"It cost $2.8 million."},
			"CONTROL: #59's guard still holds"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := SplitSentences(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitSentences(%q)\n  got  %q\n  want %q\n  %s", tc.in, got, tc.want, tc.why)
			}
		})
	}
}

// The whole point was audio, so assert the phonemes rather than only the split.
//
// Before the guard, `a.m.` reached phonemizeWord as two words and the article
// rule claimed the first: "uh em".
func TestInitialismReachesPhonemizerWhole(t *testing.T) {
	p := newTestPhonemizer(t)

	var words []string
	for _, d := range p.DiagnoseText("It was 9 a.m. sharp.") {
		words = append(words, d.Word)
	}
	// ⚠️ Asserting the WORD LIST, not a phoneme: the defect was one token
	// becoming two, and a phoneme assertion on either half would pass while the
	// split survived.
	want := []string{"It", "was", "nine", "a.m", "sharp"}
	if !reflect.DeepEqual(words, want) {
		t.Errorf("words = %q, want %q\n  a bare `a` here means the split is back, "+
			"and the article rule will read it as \"uh\"", words, want)
	}
}

// a.m. and p.m. are letter names, and p.m. had NO VOWEL AT ALL (#68).
//
// 🛑 This is a regression I introduced in #67 and shipped, found by @wayland
// within the hour. Keeping the whole token together is right — that is what
// stopped the sentence splitting — but it handed LTS a string it cannot read:
//
//	before #67   p → pˈi  ·  m → ˈɛm      "pee em"  ✅ right reading, THREE sentences
//	after  #67   p.m → pm                 🛑 /p/ /m/, an unpronounceable cluster
//	now          p.m → ˌpiˈɛm             ✅ right reading, ONE sentence
//
// ⚠️ So the fix traded a prosodic defect for a phonetic one and I did not check
// the reading, only the split. `a.m` went sideways in the same move — "uh em"
// to "am" rhyming with "ham" — but `p.m` went from CORRECT to malformed, and per
// @wayland's #67 count the "7:43 p.m.:" shape is 10 of the 15 occurrences, so
// the majority case was the broken one.
//
// 📌 The lesson is narrower than "test more": when a fix changes what a TOKEN
// is, every downstream layer sees a string it has never seen before. The split
// assertion and the reading assertion are different assertions, and #67 only
// had the first. TestInitialismReachesPhonemizerWhole above checks the word
// list; it passed throughout, because the tokenization was right and the
// pronunciation was not.
//
// ⛔ Banked rather than ear-checked first, which is a deliberate exception to
// the standing rule that data changes ship on @clayton's ear: a word with zero
// vowels is malformed by definition and needs no judgement, and this one was
// live on :8090. The CHOICE of reading is still open to his correction.
//
// ✅ AND HE CORRECTED IT, 2026-08-22. That caveat was load-bearing after all.
// He heard `ˌpiˈɛm` as "five [ah] PM" — a faint vowel onset before the P — and
// said `a.m` was perfect. The leading ˌ was the whole cause:
//
//	a.m   ˌAˈɛm    ˌ lands on a VOWEL      ✅ clean
//	p.m   ˌpiˈɛm   ˌ lands on a CONSONANT  🛑 renders as an "ah" onset
//	p.m   piˈɛm    no leading ˌ            ✅ "nailed it"
//
// 🛑 A/B'd all three against his ear, same voice, speed and binary, one dict
// entry apart. My FIRST hypothesis was wrong and he killed it: I thought the
// i→ɛ hiatus needed a /j/ glide, rendered `ˌpiˈjɛm`, and he reported the breath
// unchanged. The glide was never the mechanism; the leading mark was.
//
// ⚑ THE CONVENTION WAS IN THE DATA THE WHOLE TIME, and this is the durable part:
//
//	182,872 entries across custom + gold + silver
//	 12,012 begin with ˌ
//	 12,011 are followed by a VOWEL          ← Misaki's rule, never stated anywhere
//	      1 was followed by a consonant      ← this entry. Hand-written. Mine.
//
// So a hand-written entry violated a convention with twelve thousand examples
// behind it, and the only instrument that caught it was a human ear on 6 seconds
// of audio. **Grep the corpus for the shape before inventing a pronunciation:**
// 12,011 agreeing entries is a stronger prior than my judgement about phonetics.
func TestAMPMReadAsLetterNames(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct{ text, word, want, why string }{
		{"It was 9 a.m. sharp.", "a.m", "ˌAˈɛm", "\"ay-EM\" — was ˈæm, the word \"am\""},
		{"The meeting ran to 5 p.m. and everyone left.", "p.m", "piˈɛm", "\"pee-EM\" — NO leading ˌ, see below"},
		{"7:43 p.m.: Nadine Osei sends the file.", "p.m", "piˈɛm", "@wayland's ×10 majority shape"},
	}
	for _, tc := range cases {
		got := phonemesForWord(t, p, tc.text, tc.word)
		if got != tc.want {
			t.Errorf("%q in %q = %q, want %q\n  %s", tc.word, tc.text, got, tc.want, tc.why)
		}
		if src := sourceForWord(t, p, tc.text, tc.word); src != "custom" {
			t.Errorf("%q resolved from %q, not custom — the bank is not being reached",
				tc.word, src)
		}
	}
}

// ⚠️ A vowel-free reading is malformed no matter which word produces it, so
// assert the PROPERTY rather than only the two strings above. This is what
// would have caught #68 without @wayland reading the output by eye.
func TestNoBankedInitialismIsVowelFree(t *testing.T) {
	p := newTestPhonemizer(t)
	const vowels = "iɪeɛæaɑɔoʊuʌəɜAOIWY"

	for _, text := range []string{
		"It was 9 a.m. sharp.",
		"The meeting ran to 5 p.m. and everyone left.",
		"7:43 p.m.: Nadine Osei sends the file.",
	} {
		for _, d := range p.DiagnoseText(text) {
			if d.Phonemes == "" {
				continue
			}
			hasVowel := false
			for _, r := range d.Phonemes {
				for _, v := range vowels {
					if r == v {
						hasVowel = true
					}
				}
			}
			if !hasVowel {
				t.Errorf("%q → %q has NO VOWEL — unpronounceable, from %q (source %s)",
					d.Word, d.Phonemes, text, d.Source)
			}
		}
	}
}

// #68's fix is CASE-INCOMPLETE and I shipped it. Found 2026-08-19 while working
// #62, by running its specimen instead of reading its analysis.
//
//	a.m.  → ˌAˈɛm    custom      ✅ "ay-EM"
//	A.M.  → A + M    ə + ˈɛm     🛑 "uh em" — the exact defect #68 was filed for
//	P.M.  → P + M    pˈi + ˈɛm   ⚠️ "pee em", correct only by luck: P has no
//	                              article rule, so nothing corrupts it
//
// The cause is normalizer.go:289, and it runs BEFORE any dictionary is consulted:
//
//	abbreviationDotRe = `\b([A-Z]\.){2,}`     ← UPPERCASE ONLY
//	"A.M." → TrimSuffix "." → "A.M" → dots to spaces → "A M"
//
// So the banked `a.m` entry can never fire for the uppercase form — the token it
// would match is destroyed two steps earlier. Lowercase survives only because
// the regex does not match it.
//
// ⚑ Confirmed by experiment, not inference: banking `a.r` did NOT fix `A.R.`.
// If the split had been a lookup miss, that would have fixed it. It changed
// nothing, which is what sent me upstream to the normalizer.
//
// 📌 This is also the missing half of #62. @wayland concluded there that the
// period "does not survive to where the rule would run" and the fix must live in
// or before the normalizer. He was right about the location and the mechanism is
// this line — an expansion nobody had connected to either ticket.
//
// ⛔ NOT FIXED HERE. The fix needs a custom-dictionary oracle inside
// NormalizeTextWithWords, whose isEnglishWord consults gold and silver only —
// a signature change on a shared entry point. And the denominator is unmeasured:
// @wayland counted 7 LOWERCASE a.m.; the uppercase population is unknown, and a
// rule is worth building when the population is exposed.
func TestUppercaseDottedInitialismLosesItsBankedReading(t *testing.T) {
	p := loadTestPhonemizer(t)

	read := func(text, want string) string {
		t.Helper()
		for _, d := range p.DiagnoseText(text) {
			if d.Word == want {
				return d.Phonemes
			}
		}
		return ""
	}

	// The control: lowercase still works. If this breaks, the bank regressed and
	// the assertion below is measuring something else entirely.
	if got := read("It was 9 a.m. sharp.", "a.m"); got != "ˌAˈɛm" {
		t.Fatalf("lowercase a.m regressed: got %q, want ˌAˈɛm — fix that before "+
			"reading anything into the uppercase arm", got)
	}

	// The gap. `A` must survive as its own token carrying the ARTICLE reading for
	// this defect to be present.
	if got := read("It was 9 A.M. sharp.", "A"); got != "ə" {
		t.Errorf("A.M. no longer yields a standalone A reading ə (got %q) — if the "+
			"normalizer now preserves banked dotted forms, that is the intended fix: "+
			"update this pin and close the #68 gap on the ticket", got)
	}
}
