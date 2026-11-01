//go:build unit

package phonemizer

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// #63, re-measured 2026-08-19. The ticket's own numbers were partly wrong and I
// wrote them, so they are corrected here and pinned.
//
//	claim                             filed         measured
//	keys disagreeing with gold        20 of 20      19 of 20 — `present` agrees
//	strings with no stress mark       18 of 40      18 of 40 ✓
//	strings using ː                    8            8 ✓
//	gold entries using ː              0 of 91,007   0 of 90,201 (entry count wrong)
//
// 🛑 The finding that changes the fix:
// gold already carries POS-tagged homograph readings for 19 of the 20 keys.
//
// ⚑ That sentence is on ONE line deliberately. Its first draft wrapped after
// "HOMOGRAPH", and the findability grep for it ALSO used the wrong case — the
// line reads "gold", the grep said "GOLD". A wrong grep and an absent phrase
// return the identical 0, which is why CLAUDE.md requires the negative control.
// Third findability miss tonight, third distinct mechanism: markdown emphasis,
// a line wrap, and now case.
//
//	gold["record"]  = {DEFAULT: ɹˈɛkəɹd, VERB: ɹəkˈɔɹd}
//	gold["present"] = {DEFAULT: pɹˈɛzᵊnt, VERB: pɹizˈɛnt}
//
// So homographs.json is not merely notated differently — it is a SECOND COPY of
// distinctions gold already makes. The fix is therefore not "invent normalised
// strings", which would be a guess about 40 pronunciations; it is "adopt the
// readings gold already holds", which is mechanical.
//
// ⚑ And the path is already proven end-to-end: `present` matches gold on BOTH
// readings (5312bc2, "@clayton's ear, second key to do so") and `record` matches
// on its VERB (5ce0b0b). Two keys were normalised by ear and neither regressed.
//
// ⚠️ The divergence reaches the MODEL, not just the file. Verified through the
// real path with `diagnose -json -sources homograph`:
//
//	read    → ɹiːd       gold ɹˈid    — length mark ADDED, stress mark LOST
//	produce → pɹəˈduːs   gold pɹədˈus
//
// ː is token-bearing, so the model receives a symbol it never sees from a
// dictionary of 90,201 entries. Whether that is audible is an ear question and
// @clayton has four clips waiting; this test asserts only what is measurable.
func TestHomographsDivergeFromGold(t *testing.T) {
	hRaw, err := os.ReadFile("../../data/homographs.json")
	if err != nil {
		t.Skipf("homographs.json absent: %v", err)
	}
	gRaw, err := os.ReadFile("../../data/us_gold.json")
	if err != nil {
		t.Skipf("us_gold.json absent: %v", err)
	}

	var homo map[string]json.RawMessage
	if err := json.Unmarshal(hRaw, &homo); err != nil {
		t.Fatalf("homographs.json: %v", err)
	}
	var gold map[string]json.RawMessage
	if err := json.Unmarshal(gRaw, &gold); err != nil {
		t.Fatalf("us_gold.json: %v", err)
	}

	// goldReadings returns every reading gold holds, string entry or POS dict.
	goldReadings := func(word string) map[string]bool {
		out := map[string]bool{}
		raw, ok := gold[word]
		if !ok {
			return out
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			out[s] = true
			return out
		}
		var m map[string]string
		if json.Unmarshal(raw, &m) == nil {
			for _, v := range m {
				out[v] = true
			}
		}
		return out
	}

	var keys []string
	for k := range homo {
		// ⚠️ `_description` is metadata, not a homograph. Counting it gives 21
		// keys and I did exactly that once tonight — the same error shape as
		// counting ˈ among the phonemes.
		if !strings.HasPrefix(k, "_") {
			keys = append(keys, k)
		}
	}
	// 20 until #112 added `lives` (the verb was always said as the plural of "life").
	if len(keys) != 21 {
		t.Fatalf("expected 21 homograph keys, got %d — #63's denominator moved", len(keys))
	}

	var totalStrings, matching, noStress, lengthMark, keysWithGoldVariants int
	for _, k := range keys {
		var entry struct {
			Pronunciations map[string]json.RawMessage `json:"pronunciations"`
		}
		if err := json.Unmarshal(homo[k], &entry); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		gr := goldReadings(k)
		if len(gr) > 1 {
			keysWithGoldVariants++
		}
		for pron := range entry.Pronunciations {
			totalStrings++
			if gr[pron] {
				matching++
			}
			if !strings.ContainsAny(pron, "ˈˌ") {
				noStress++
			}
			if strings.Contains(pron, "ː") {
				lengthMark++
			}
		}
	}

	for _, c := range []struct {
		name      string
		got, want int
	}{
		// +2 by #112: `lives` adds a verb and a noun reading. Both already match gold (its POS variants).
		{"pronunciation strings", totalStrings, 42},
		// ⬅ MOVED 2026-08-22 by the notation landing — 8 keys adopted gold's forms
		// (bass bow close live read tear wind wound) after @Clayton's ear on
		// readclose-gold.wav. All three deltas are the INTENDED direction:
		//
		//	matching a gold reading   3 → 19   +16
		//	no stress mark           18 →  2   −16
		//	using ː                   8 →  6   −2
		//
		// 🛑 The two survivors of each are the untouched buckets, and that is the
		// check: `lead` (no gold counterpart) keeps liːd/lɛd, and the ː pair that
		// remains is `lead` + `minute` — both deliberately excluded. If these ever
		// reach 0 without a ticket saying so, something adopted more than was decided.
		{"strings matching a gold reading", matching, 27}, // 19 until #112 added `lives`; 21 until #120 put the stress marks of 10 words on their vowels (6 more strings now equal gold's)
		{"strings with no stress mark", noStress, 2},
		{"strings using ː", lengthMark, 6},
		{"keys where gold holds POS variants", keysWithGoldVariants, 20}, // 19 until #112 added `lives`
	} {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d — #63's measurement has moved. "+
				"If this is a normalisation landing, that is the intended direction: "+
				"update the ticket with the new counts rather than only this number.",
				c.name, c.got, c.want)
		}
	}

	// gold's own notation, the control for "ː is foreign here"
	var goldLength int
	for _, raw := range gold {
		if strings.Contains(string(raw), "ː") {
			goldLength++
		}
	}
	if goldLength != 0 {
		t.Errorf("gold now uses ː in %d entries — the premise that it is foreign "+
			"to gold's notation no longer holds, and #63's argument rests on it", goldLength)
	}
}

// TestHomographKeysPartitionByFixCost sizes #63's fix, measured 2026-08-20.
//
// "20 of 20 keys disagree with gold" is true and it is not actionable — it says
// nothing about what fixing them COSTS. Pairing each homograph reading with a
// DISTINCT gold value after stripping ˈ ˌ ː splits the twenty into five very
// different jobs:
//
// ⚠️ BOTH WORDS IN THAT SENTENCE ARE LOAD-BEARING, and this header named only one
// of them until @wayland reproduced the figure and got 13:
//
// The ruler gets you from 17 to 13 and injectivity gets you from 13 to 12, so a figure quoted without both conditions is not reproducible.
//
// `pairsLoosely` below measures the gap and the test pins it at exactly one key
// (`permit`), so the condition is now checkable rather than merely stated.
//
//	12 keys  NOTATION ONLY        every reading has a gold value with the same
//	                              phonemes; gold just writes stress and never ː
//	 4 keys  REDUCTION ONLY       one systematic disagreement, not four —
//	                              homographs.json spells an unstressed vowel in
//	                              full where gold reduces it to ə
//	 1 key   CONTENTION           permit — both readings are segmentally pɜɹmɪt,
//	                              so gold's two values cannot supply two distinct
//	                              strings. Adopting COLLAPSES the distinction.
//	 2 keys  GENUINELY DIFFERENT  minute  mIˈnjuːt vs mInˈut   (the /j/)
//	                              produce ˈpɹɑduːs vs pɹˈOdˌus (ɑ vs O)
//	 1 key   NO COUNTERPART       lead — gold holds only lˈid, so homographs.json
//	                              is the ONLY source for lead-the-metal
//
// 🛑 CONTENTION WAS A FIFTH BUCKET FOR A DAY BEFORE IT WAS A FIELD. `permit` was
// ruled its own case on 2026-08-20 — the ruling reached #63's title and its prose
// and NOT the bucket it was moved out of, so it sat in `reductionOnly` as well and
// the ticket's own title summed to 21 against a population of 20. @wayland caught
// it on the addition.
//
// ⚑ And this test was green throughout, because three of its six assertions
// compared a literal map to the count of its own literals — `len(reductionOnly)`
// against 5, where the 5 was chosen by counting that map. A bucket count that
// cannot disagree with its bucket is not a measurement, and it is why a key living
// in two buckets was invisible here.
//
// ✅ So the counts below are now DERIVED against the ruler: the four hand maps must
// be pairwise DISJOINT and their union must be exactly the set that fails the
// ruler. Double-membership fails on disjointness; a miscount fails on the union.
//
// ⚑ The 12 are the ones worth doing first, and the reason is stronger than
// "mechanical": gold's reading is what these words ALREADY get in every
// non-triggered context. The homograph layer only overrides on a context
// trigger, so adopting gold makes triggered-`read` sound like the `read` that
// has been shipping all along. That is not a new sound to audition, it is the
// removal of a second one.
//
// 🛑 It is still a token-stream change: ɹiːd → ɹˈid drops ː (token-bearing, and
// gold uses it in 0 of 90,201 entries) and adds ˈ. Expected to help, not proven
// to — @clayton normalised `present` and `record` by ear and neither regressed,
// which is 2 data points, not 12.
//
// ⚠️ The 4-vs-1-vs-2 split is a PHONETIC JUDGEMENT and is written out by hand below
// rather than derived, for the same reason #25's partition is: no mechanical
// rule separates "gold reduced the vowel" from "these are different words", and
// a rule that looked like it did would be the more dangerous artifact.
func TestHomographKeysPartitionByFixCost(t *testing.T) {
	hRaw, err := os.ReadFile("../../data/homographs.json")
	if err != nil {
		t.Skipf("homographs.json absent: %v", err)
	}
	gRaw, err := os.ReadFile("../../data/us_gold.json")
	if err != nil {
		t.Skipf("us_gold.json absent: %v", err)
	}
	var homo map[string]json.RawMessage
	if err := json.Unmarshal(hRaw, &homo); err != nil {
		t.Fatalf("homographs.json: %v", err)
	}
	var gold map[string]json.RawMessage
	if err := json.Unmarshal(gRaw, &gold); err != nil {
		t.Fatalf("us_gold.json: %v", err)
	}

	// By hand — see the comment above. ⛔ `permit` is NOT in reductionOnly: it is
	// contention, and it lived in both until 2026-08-21.
	reductionOnly := map[string]bool{
		"desert": true, "object": true, "record": true, "refuse": true,
	}
	contention := map[string]bool{"permit": true}
	genuinelyDifferent := map[string]bool{"minute": true, "produce": true}
	noCounterpart := map[string]bool{"lead": true}

	// The buckets must not overlap. This is the check that was missing: every
	// assertion below counted memberships, and a key in two buckets is counted
	// twice by all of them and caught by none.
	handBuckets := []struct {
		name string
		set  map[string]bool
	}{
		{"reduction", reductionOnly},
		{"contention", contention},
		{"genuinely different", genuinelyDifferent},
		{"no counterpart", noCounterpart},
	}
	for i := range handBuckets {
		for j := i + 1; j < len(handBuckets); j++ {
			for k := range handBuckets[i].set {
				if handBuckets[j].set[k] {
					t.Errorf("%q is in BOTH the %q and %q buckets. Any total built from "+
						"these counts it twice — #63's title summed to 21 against a "+
						"population of 20 for exactly this reason. Pick one.",
						k, handBuckets[i].name, handBuckets[j].name)
				}
			}
		}
	}

	// ⚑ The ruler, and the second ruler that has to agree with it. @wayland
	// reproduced this partition and got 15 notation to my 12, attributing the gap
	// to his classifier also stripping ə — "blind to a full vowel standing where
	// gold reduces". Measured 2026-08-20: stripping ə as well changes NOTHING,
	// still 12, the same twelve keys. So that is not where our numbers diverge,
	// and a plausible explanation for a discrepancy is not a measurement of it.
	//
	// Both rulers are asserted below so the 12 cannot be an artifact of one.
	// (A third, far cruder ruler — strip every one of ˈˌːəɪɛɔɜ — gives 17, so the
	// figure is not ruler-independent in general. It is stable across the two
	// rulers that preserve vowel identity, which is the property that matters.)
	stripBy := func(marks string) func(string) string {
		return func(s string) string {
			return strings.Map(func(r rune) rune {
				if strings.ContainsRune(marks, r) {
					return -1
				}
				return r
			}, s)
		}
	}
	strip := stripBy("ˈˌː")
	goldValues := func(word string) []string {
		raw, ok := gold[word]
		if !ok {
			return nil
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return []string{s}
		}
		var m map[string]string
		if json.Unmarshal(raw, &m) != nil {
			return nil
		}
		seen := map[string]bool{}
		var out []string
		for _, v := range m {
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
		return out
	}

	// pairsCleanly reports whether every homograph reading for k finds a distinct
	// gold value equal under st — i.e. the key differs from gold in notation only.
	pairsCleanly := func(k string, st func(string) string) bool {
		var entry struct {
			Pronunciations map[string]json.RawMessage `json:"pronunciations"`
		}
		if err := json.Unmarshal(homo[k], &entry); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		pool := goldValues(k)
		for pron := range entry.Pronunciations {
			matched := -1
			for i, gv := range pool {
				if st(gv) == st(pron) {
					matched = i
					break
				}
			}
			if matched < 0 {
				return false
			}
			pool = append(pool[:matched], pool[matched+1:]...)
		}
		return true
	}

	// pairsLoosely is the SAME test with the consumption removed: every reading
	// must match some gold value, but two readings may match the SAME one. It
	// exists only to be counted — nothing classifies with it.
	//
	// ⚑ @wayland's finding, 2026-08-21: the 12 rests on TWO conditions, and the
	// header above named only one. A vowel-preserving ruler gets you from 17 to 13;
	// injective matching gets you from 13 to 12. Drop either and the number moves.
	//
	// 🛑 The thirteenth key is `permit`, and this is what makes the original error
	// legible: `pɜɹˈmɪt` and `pˈɜɹmɪt` both strip to `pɜɹmɪt`, which matches gold's
	// `pɜɹmɪt` — twice, the same value. Putting `permit` in the notation-only 12 was
	// never a misreading of the key. It was the non-injective ruler applied
	// CONSISTENTLY, which is why it stayed coherent through twenty comments.
	//
	// 📌 So the contention ruling is now one key of arithmetic: anyone who computes
	// 13 knows exactly which key and which condition they dropped.
	pairsLoosely := func(k string, st func(string) string) bool {
		var entry struct {
			Pronunciations map[string]json.RawMessage `json:"pronunciations"`
		}
		if err := json.Unmarshal(homo[k], &entry); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		pool := goldValues(k)
		for pron := range entry.Pronunciations {
			found := false
			for _, gv := range pool {
				if st(gv) == st(pron) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}

	var notationOnly, needsJudgement, notationUnderSchwaRuler int
	var looseOnly []string
	var notationKeys []string
	for k := range homo {
		if strings.HasPrefix(k, "_") {
			continue
		}
		if pairsCleanly(k, stripBy("ˈˌːə")) {
			notationUnderSchwaRuler++
		}
		// Keys the loose ruler would call notation-only but the injective one does
		// not — i.e. the keys whose classification injectivity alone decides.
		if pairsLoosely(k, strip) && !pairsCleanly(k, strip) {
			looseOnly = append(looseOnly, k)
		}
		unpaired := 0
		if !pairsCleanly(k, strip) {
			unpaired = 1
		}

		byHand := reductionOnly[k] || contention[k] || genuinelyDifferent[k] || noCounterpart[k]
		if unpaired == 0 {
			notationOnly++
			notationKeys = append(notationKeys, k)
			if byHand {
				t.Errorf("%q pairs cleanly with gold now, but is still listed by hand as "+
					"needing a judgement — the hand-written split above is stale", k)
			}
			continue
		}
		needsJudgement++
		if !byHand {
			t.Errorf("%q no longer pairs with gold and is in none of the hand-written "+
				"buckets, so #63's decomposition does not describe it. Classify it on the "+
				"ticket before changing this test", k)
		}
	}

	// ⚑ Print the MEMBERS, not just the counts. This test answers "is it still 12?"
	// and until 2026-08-21 could not answer "which twelve?" — so every time that
	// question came up (four times in one night on #63) I wrote a throwaway script
	// instead of running the instrument that already knew.
	//
	// 📌 @vance's shape exactly: he authored the guard for a question, then eight
	// hours later reached for grep and got the wrong number. An instrument that
	// asserts but does not REPORT is one you cannot consult, only obey.
	t.Logf("notation-only (%d): %s", len(notationKeys), strings.Join(sorted(notationKeys), " "))
	t.Logf("reduction (%d):     %s", len(reductionOnly), strings.Join(sorted(keysOfSet(reductionOnly)), " "))
	t.Logf("contention (%d):    %s", len(contention), strings.Join(sorted(keysOfSet(contention)), " "))
	t.Logf("genuinely diff (%d): %s", len(genuinelyDifferent), strings.Join(sorted(keysOfSet(genuinelyDifferent)), " "))
	t.Logf("no counterpart (%d): %s", len(noCounterpart), strings.Join(sorted(keysOfSet(noCounterpart)), " "))

	// ⚑ The whole point of this total: it is built from the hand maps and compared
	// against a number DERIVED from the ruler. len(map)==literal cannot go red;
	// this can, and it is the assertion #63's title needed and did not have.
	handTotal := len(reductionOnly) + len(contention) + len(genuinelyDifferent) + len(noCounterpart)
	t.Logf("hand-classified total %d vs %d keys that fail the ruler · population %d",
		handTotal, needsJudgement, notationOnly+needsJudgement)
	t.Logf("injectivity decides %d key(s): %s — without it notation-only would be %d",
		len(looseOnly), strings.Join(sorted(looseOnly), " "), notationOnly+len(looseOnly))

	// ⛔ Name the key, not just the count. "13 vs 12" tells a reader their number is
	// wrong; "permit, and injectivity is why" tells them where to look. A count that
	// cannot say WHICH is the reporting failure this test already had once.
	if len(looseOnly) != 1 || looseOnly[0] != "permit" {
		t.Errorf("injectivity should decide exactly `permit`, got %v — either a key was "+
			"normalised, or pairsCleanly stopped consuming its matches. If the latter, "+
			"notation-only silently becomes 13 and `permit` rejoins the 12 it was ruled "+
			"out of.", sorted(looseOnly))
	}

	for _, c := range []struct {
		name      string
		got, want int
	}{
		// 12 until #112 added `lives`, which differs from gold only in notation, like `live`.
		{"keys needing only gold's notation", notationOnly, 13},
		{"same, under the ə-stripping ruler too", notationUnderSchwaRuler, 13},
		{"keys needing a phonetic judgement", needsJudgement, 8},
		{"hand buckets must account for exactly those", handTotal, needsJudgement},

		// ⚠️ THE FOUR BELOW ARE TRIPWIRES, NOT MEASUREMENTS, and must not be read as
		// evidence of anything. Each compares a literal map to the count of its own
		// literals — `len(contention)` against 1, where the 1 was chosen by counting
		// that map. No input makes them fail; they only catch an editor who changes a
		// bucket and not its number.
		//
		// 🛑 Three of them in this exact shape are why `permit` sat in two buckets for a
		// day and #63's title summed to 21 against a population of 20. Keeping them is a
		// deliberate call — the real coverage is the disjointness loop above and
		// handTotal-vs-needsJudgement, both of which are derived and red-proofed.
		{"hand-classified: reduction only (tripwire)", len(reductionOnly), 4},
		{"hand-classified: contention (tripwire)", len(contention), 1},
		{"hand-classified: genuinely different (tripwire)", len(genuinelyDifferent), 2},
		{"hand-classified: no gold counterpart (tripwire)", len(noCounterpart), 1},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d — #63's fix sizing has moved. If keys were "+
				"normalised, that is the intended direction: update the ticket with the new "+
				"partition rather than only this number.", c.name, c.got, c.want)
		}
	}
}

// TestHomographTopLevelReadingsAreAlsoPronunciationKeys guards #63's fix against
// the mirror image of the bug @wayland just found in his own extractor.
//
// He read the top-level strings and skipped `pronunciations`; I read
// `pronunciations` and skipped the top-level strings. Measured 2026-08-20, my
// half loses nothing — all 22 top-level readings are also pronunciation keys, and
// `pronunciations` holds 40 strings to the top level's 22, so it is a strict
// superset. His half loses 18 of 40, and 📌 a partial read of this file does not
// fail randomly: every top-level string IS the default, which is the reading most
// likely to match a dictionary default, so it fails TOWARD agreement.
//
// 🛑 But the engine reads BOTH halves and compares them BY STRING VALUE:
//
//	:203  d == bestDist && pron == entry.Default && …    tie-break toward default
//	:231  !inflected || bestPron != entry.Default        inflected guard, pass 1 (directional)
//	:296  !inflected || bestPron != entry.Default        inflected guard, pass 2 (direction-blind)
//
// ⚠️ THREE sites, not the two I first published on #63 and in the room. I read
// the file, found two, and reported a count from a read rather than from a
// search — @wayland's rule ("grep for comparisons BETWEEN the copies") found the
// third on the very next application of it. So a desync disables the inflected
// guard on BOTH resolution passes, and the hazard is broader than I stated.
//
// 📌 Same sweep over every other data file — us_gold, us_silver, custom_dict,
// arpabet_to_ipa, token_map — finds no comparison of one copy against another.
// homographs.json is the only file we hold with this shape.
//
// So normalising a `pronunciations` key toward gold (ɹiːd → ɹˈid) WITHOUT
// rewriting `default` to the identical new string silently disables both: the
// tie-break toward the default stops firing because nothing equals Default any
// more, and the inflected guard stops guarding on both passes for the same reason.
//
// ⚠️ Neither failure raises anything. No missing key, no error, no changed
// output on the cases anyone would spot-check — just two behaviours quietly off,
// one of which exists because @clayton found the bug it prevents.
//
// ⛔ That is a worse outcome than the wrong count that prompted this check: his
// bug produced a number we caught within the hour; mine would have produced
// working code with two disabled guards. Same bug class, opposite halves.
func TestHomographTopLevelReadingsAreAlsoPronunciationKeys(t *testing.T) {
	raw, err := os.ReadFile("../../data/homographs.json")
	if err != nil {
		t.Skipf("homographs.json absent: %v", err)
	}
	// ⚠️ Two levels, not one: `_description` is a bare string at the top level, so
	// unmarshalling straight into map[string]map[…] fails on it. That is the same
	// metadata key that has already produced a 21-vs-20 denominator error twice.
	var homo map[string]json.RawMessage
	if err := json.Unmarshal(raw, &homo); err != nil {
		t.Fatalf("homographs.json: %v", err)
	}

	topLevel := 0
	for word, rawEntry := range homo {
		if strings.HasPrefix(word, "_") {
			continue
		}
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			t.Fatalf("%s: %v", word, err)
		}
		var prons map[string]json.RawMessage
		if p, ok := entry["pronunciations"]; ok {
			if err := json.Unmarshal(p, &prons); err != nil {
				t.Fatalf("%s.pronunciations: %v", word, err)
			}
		}
		for field, rawVal := range entry {
			var s string
			if json.Unmarshal(rawVal, &s) != nil {
				continue // the pronunciations object itself, not a reading
			}
			topLevel++
			if _, ok := prons[s]; !ok {
				t.Errorf("%s.%s = %q is not a key of %s.pronunciations.\n"+
					"The engine compares these by string value at THREE sites (homograph.go "+
					":203 tie-break, :231 and :296 inflected guard on each resolution pass), "+
					"so a reading that exists in only one half disables "+
					"both silently. If this is #63 normalisation landing, rewrite EVERY field "+
					"holding the old string in the same edit.", word, field, s, word)
			}
		}
	}

	// The denominator, so a future edit that deletes half the fields cannot pass
	// this test by having nothing left to check.
	// 22 until #112 added `lives`, 24 once #112 added narrated_past to `read`, 25 once #124
	// added subject_name to `read`.
	if topLevel != 25 {
		t.Errorf("top-level reading strings: got %d, want 25 — the shape of "+
			"homographs.json changed and this guard may no longer cover it", topLevel)
	}
}

// keysOfSet returns the keys of a set-shaped map, for reporting membership rather
// than only its size.
func keysOfSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// sorted returns a sorted copy, so the reported membership is stable across runs
// and a diff between two runs means a real change rather than map ordering.
func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
