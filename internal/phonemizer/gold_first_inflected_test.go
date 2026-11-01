//go:build unit

package phonemizer

import (
	"os"
	"strings"
	"testing"
)

// newTestPhonemizer builds the real pipeline against the real data files, so the
// readings below are what the engine actually produces rather than what a single
// layer would answer if called directly.
//
// ⚠️ CustomDictPath is not optional here, however optional the field is.
// It was omitted originally, so every test using this helper ran without the author bank — a configuration the worker never uses, since cmd/worker/main.go:97 always passes it.
//
// Nothing in the assertions was wrong: a custom key may not be a homographs.json
// key, which custom_dict_reachable_test enforces, so the pinned `record` and
// `present` readings could not have moved either way. But the helper was
// answering a question about a pipeline that does not ship, which is the exact
// failure this repo already paid for once — DiagnoseText tokenized whole files
// while TokenizeText did one sentence, and a 2,760-call census was built on the
// difference.
//
// Found by measuring a banked word through this helper and getting the UNBANKED
// reading twice — with the entry present and with it deleted. Two identical
// answers where one had to differ.
// ⚑ THE TWO-AXIS NULL-DATA CHECK — how to tell whether a test in this package
// is measuring the layer it names. Empty ONE data file at a time and see which
// tests go red:
//
//	                                    {} custom_dict   {} homographs
//	TestAMPMReadAsLetterNames               RED             green      → bank
//	TestAccentedLoanwordsAreBanked          RED               —        → bank
//	TestLiveTriggerDisambiguation          green             RED       → homographs
//	TestRecordReadings / TestPresent…         —              RED       → homographs
//	TestGoldWholeWordBeatsInflected…       green             RED       → homographs
//
// Measured 2026-08-19: 63 assertion failures on an empty bank, 57 on empty
// homographs, and every test above goes red on exactly the file it names.
//
// ⚠️ A test green on BOTH axes claims to test a dictionary and does not.
// ⛔ And green on ONE axis proves nothing by itself — I read
// "TestLiveTriggerDisambiguation passed with an empty bank" as a null instrument
// before checking that it targets homographs.json, where it correctly goes red.
// The probe needs both axes to be interpretable, exactly like a red-proof needs
// its controls.
//
// This is the suite-level form of the bug fixed below: a helper that silently
// omits a data source makes every test using it answer about a pipeline that
// does not ship, and nothing goes red to say so.
//
// MISAKI_DATA_DIR repoints every test built on this helper at a scratch copy of
// data/, defaulting to the real one when unset.
//
// ⛔ It exists so these tests can be RED-PROOFED WITHOUT MUTATING data/. Several
// benches read that directory; breaking an entry there to falsify an assertion
// corrupts whatever they measure in the same minute — recoverable for you, invisible
// to them. With hardcoded paths the only way to make one of these fail was to edit
// the shared file, so in practice nobody but the author ever did.
//
// ⚠ That is not hypothetical. @Wayland could not falsify
// TestEveryPOSEntryHasAUsableDefault at all until e3b76e5 added this same override
// there; he verified it by reading the branch and running it green, and said so
// rather than letting "verified" cover both.
//
// ⚑ A test only its author can make fail is one only its author can trust.
//
// misaki_default_key_test.go had this and this helper did not, so the override
// covered one test while 19 call sites across 8 files stayed unfalsifiable.
//
//	cp data/*.json /tmp/fixture/ && <break one entry in /tmp/fixture/>
//	MISAKI_DATA_DIR=/tmp/fixture make dev-test-unit RUN=TestSplitInitialism VERBOSE=1
func newTestPhonemizer(t *testing.T) *Phonemizer {
	t.Helper()
	dir := os.Getenv("MISAKI_DATA_DIR")
	if dir == "" {
		dir = "../../data"
	}
	p, err := New(Config{
		GoldDictPath:   dir + "/us_gold.json",
		SilverDictPath: dir + "/us_silver.json",
		HomographsPath: dir + "/homographs.json",
		TokenMapPath:   dir + "/token_map.json",
		ARPABETMapPath: dir + "/arpabet_to_ipa.json",
		CustomDictPath: dir + "/custom_dict.json",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

// phonemesForWord runs a whole sentence through the diagnostic and returns the
// reading for one word. Going through the sentence rather than calling the
// resolver directly is deliberate: context is what decides a homograph, and a
// direct call would discard it.
func phonemesForWord(t *testing.T, p *Phonemizer, text, word string) string {
	t.Helper()
	var hits []WordDiagnosis
	for _, d := range p.DiagnoseText(text) {
		if strings.EqualFold(d.Word, word) {
			hits = append(hits, d)
		}
	}
	if len(hits) == 0 {
		t.Fatalf("%q never appeared in the diagnosis of %q", word, text)
	}
	if len(hits) > 1 {
		// Guard the guard: with several occurrences, "the reading" is ambiguous
		// and a silent first-match would hide a disagreement between them.
		t.Fatalf("%q appears %d times in %q — the probe must isolate one",
			word, len(hits), text)
	}
	return hits[0].Phonemes
}

// sourceForWord returns which pipeline LAYER answered a word — custom, homograph,
// gold, silver, lts or skip.
//
// ⚑ @gordon's species, applied here: an arm can assert the right VALUE and still
// be testing nothing, because an upstream layer produced that value and the layer
// the test is named after was never reached. His case was a stranger 404'd a
// hundred lines before the branch his test named; the injection at that branch
// left the arm green.
//
// This pipeline is six layers deep and ordered, so it has exactly that shape.
// `closing` currently resolves from gold — which is what the gold-first tests
// claim — but a `closing` entry in custom_dict would satisfy every one of those
// assertions while gold-first was completely broken, and nothing would say so.
// That became newly possible when this helper started loading custom_dict.
func sourceForWord(t *testing.T, p *Phonemizer, text, word string) string {
	t.Helper()
	for _, d := range p.DiagnoseText(text) {
		if strings.EqualFold(d.Word, word) {
			return d.Source
		}
	}
	t.Fatalf("%q never appeared in the diagnosis of %q", word, text)
	return ""
}

// A whole-word dictionary entry beats a stem-plus-suffix reconstruction.
//
// The homograph resolver's inflected path strips a suffix, resolves the STEM as
// a homograph and re-attaches. It ran ahead of the gold lookup, so "closing" was
// rebuilt out of "close" and came back klOsɪŋ — unstressed, and with /s/ where
// the word takes /z/ — while gold held klˈOzɪŋ the whole time.
//
// @wayland's corpus count (#64): 25 words / 1,150 occurrences are inflected AND
// present whole in gold; 25 of 25 favour gold; 842 occurrences (73%) arrived
// with no stress mark at all — klOst 347x, ɹiːdɪŋ 272x, lIvɪŋ 115x.
//
// ⚠️ THE CONTROLS ARE THE POINT. Two things must NOT change, and they are what
// separates this from "stop using the homograph resolver":
//
//	bowed          gold does not have it, so the inflected path must still answer.
//	               This is @clayton's original ear finding — "Percival bowed to the
//	               room" said bˈOd, the ribbon vowel — and it is the whole reason
//	               the inflected path exists.
//	close (bare)   context must still decide it. A bare homograph is not inflected
//	               and gold cannot disambiguate it, so it must never be diverted.
//
// Red-proof these TOGETHER: reverting the gold-first check turns the first group
// red and must leave BOTH controls green. A control that goes red means the
// change reached further than intended.
func TestGoldWholeWordBeatsInflectedReconstruction(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct {
		name       string
		text       string
		word       string
		want       string
		why        string
		wantSource string // which LAYER must answer — see sourceForWord
		guard      bool   // true = a control that must hold regardless
	}{{
		name: "closing takes gold's stressed voiced form",
		text: "He heard a car door closing.",
		word: "closing", want: "klˈOzɪŋ", wantSource: "gold",
		why: "rebuilt from the `close` stem it was klOsɪŋ — no stress, wrong sibilant. " +
			"One of the two regressions ef3e1b7 appeared to cause (#60)",
	}, {
		name: "recording takes gold's stressed form",
		text: "It was a chapter, a recording, nothing more.",
		word: "recording", want: "ɹəkˈɔɹdɪŋ", wantSource: "gold",
		why: "reconstruction gave ˈɹɛkɔɹdɪŋ from the noun stem",
	}, {
		name: "closed — 347 occurrences, the highest-recurrence item",
		text: "It was a closed case.",
		word: "closed", want: "klˈOzd", wantSource: "gold",
		why: "`a` fires the klOs (near/adjective) trigger, so the resolver engages and " +
			"rebuilds klOst — \"a CLOST case\". ⚠️ TWO earlier sentences here guarded " +
			"nothing: \"The shop was closed\" (no trigger fires) and \"He closed the door\" " +
			"(the winner equals entry.Default, and the inflected path declines on that). " +
			"Both passed with the fix reverted. Only a NON-default trigger discriminates",
	}, {
		name: "reading — 272 occurrences",
		text: "He was reading about the case.",
		word: "reading", want: "ɹˈidɪŋ", wantSource: "gold",
		why: "`about` fires the ɹiːd trigger so the resolver engages. Same correction as\n\t\t\t" +
			"the case above — the first sentence had no trigger and discriminated nothing",
	}, {
		name: "CONTROL: bowed is not in gold, so the inflected path still answers",
		text: "Percival bowed to the room.",
		word: "bowed", want: "bˈWd", wantSource: "homograph", guard: true,
		why: "@clayton's original finding. `bowed` is in NO dictionary — if this " +
			"goes red the change has broken the case the inflected path exists for. " +
			"⬅ want MOVED bWd→bˈWd 2026-08-22: this control FIRED on the #63 notation " +
			"landing, correctly — `bow` adopted gold's bˈW, so its reconstruction " +
			"inherits the mark. Kept as an adoption, not a silenced control, because " +
			"gold marks every comparable inflection it holds: bows bˈOz · closed " +
			"klˈOzd · reads ɹˈidz · tears tˈɪɹz · winds wˈɪndz. UNMARKED bWd was the " +
			"anomaly (gold leaves 124 of 90,940 strings unmarked, 0.136%); the mark " +
			"brings reconstruction INTO line rather than reaching past the 8 keys",
	}, {
		name: "CONTROL: a bare homograph is still decided by context (verb)",
		text: "He will close the door.",
		word: "close", want: "klˈOz", wantSource: "homograph", guard: true,
		why: "`close` is a bare homograph key. Gold cannot disambiguate it; only " +
			"context can, so it must never be diverted to gold",
	}, {
		name: "CONTROL: a bare homograph is still decided by context (adjective)",
		text: "The door was close to the wall.",
		word: "close", want: "klˈOs", wantSource: "homograph", guard: true,
		why: "the same word resolving the OTHER way proves context still operates — " +
			"a single-sense control would pass even if the resolver were bypassed",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if src := sourceForWord(t, p, tc.text, tc.word); src != tc.wantSource {
				t.Errorf("%q in %q was answered by %q, not %q — the assertion below "+
					"may hold for the wrong reason. Ask which gate produced the value, "+
					"not just whether the value is right.",
					tc.word, tc.text, src, tc.wantSource)
			}
			got := phonemesForWord(t, p, tc.text, tc.word)
			if got != tc.want {
				kind := "assertion"
				if tc.guard {
					kind = "CONTROL — the change reached further than intended"
				}
				t.Errorf("%s: %q in %q = %q, want %q\n  %s",
					kind, tc.word, tc.text, got, tc.want, tc.why)
			}
		})
	}
}

// goldOwnsWord is the predicate; assert it directly so a failure above
// can be localised to it rather than to dictionary contents that may change.
func TestGoldOwnsInflectedFormPredicate(t *testing.T) {
	p := newTestPhonemizer(t)

	cases := []struct {
		word string
		want bool
		why  string
	}{
		{"closing", true, "inflected, and gold holds it whole"},
		{"closed", true, "inflected, and gold holds it whole"},
		{"reading", true, "inflected, and gold holds it whole"},
		{"bowed", false, "inflected but ABSENT from gold — the resolver must keep it"},
		{"close", false, "a BARE homograph — context owns it, never gold"},
		{"read", false, "a BARE homograph"},
		{"record", false, "a BARE homograph"},
		{"table", true, "not a homograph and gold holds it — true, and skipping the " +
			"resolver is a no-op since Resolve would decline. The predicate does not " +
			"test inflection; an earlier name claimed it did"},
	}
	for _, tc := range cases {
		if got := p.goldOwnsWord(tc.word); got != tc.want {
			t.Errorf("goldOwnsWord(%q) = %v, want %v — %s",
				tc.word, got, tc.want, tc.why)
		}
	}
}
