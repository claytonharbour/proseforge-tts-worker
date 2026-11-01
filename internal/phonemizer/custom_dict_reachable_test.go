//go:build unit

package phonemizer

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Three separate proposals to fix a defect by "just banking it" were made in one
// night, and all three were unavailable — for three DIFFERENT reasons. @wayland's
// synthesis, turned into a check so the next one fails at commit time instead of
// in a conversation:
//
//	present         TOO EARLY   custom is Step 0 and the homograph resolver is
//	                            Step 1, so banking a homograph does not correct
//	                            the word — it DISABLES its context resolution and
//	                            pins one reading everywhere.
//	"off the record" WRONG UNIT lookup input is strings.Fields output, so a key
//	                            containing a space can never be matched.
//	Class III       TOO LATE    NormalizeTextWithWords runs BEFORE any dictionary,
//	                            so an acronym-expanded token no longer exists by
//	                            the time custom is consulted. A banked "iii" is
//	                            never reached: 7 words in, 7 words out, where a
//	                            hit would give 5.
//
// ⚑ What made it repeat three times: "bank it" READS as the conservative option,
// so the population measurement demanded of any rule never got run. It is only
// conservative when the defect is one surviving token resolved by dictionary
// lookup. Banking fixes PHONEMIZATION, never NORMALIZATION.
//
// All three were zero at the time this was written. This exists to keep them zero.
func TestCustomDictEntriesAreActuallyReachable(t *testing.T) {
	raw, err := os.ReadFile("../../data/custom_dict.json")
	if err != nil {
		t.Skipf("custom_dict.json not readable: %v", err)
	}
	var custom map[string]any
	if err := json.Unmarshal(raw, &custom); err != nil {
		t.Fatalf("parse custom_dict.json: %v", err)
	}

	hraw, err := os.ReadFile("../../data/homographs.json")
	if err != nil {
		t.Skipf("homographs.json not readable: %v", err)
	}
	var homos map[string]any
	if err := json.Unmarshal(hraw, &homos); err != nil {
		t.Fatalf("parse homographs.json: %v", err)
	}

	// Guard the guard: an empty bank or an empty homograph table makes every
	// assertion below vacuously true.
	if len(custom) == 0 {
		t.Fatal("custom_dict is empty — every check below passes over nothing")
	}
	if len(homos) == 0 {
		t.Fatal("homographs table is empty — the TOO EARLY check cannot fire")
	}

	homoKeys := make(map[string]bool, len(homos))
	for k := range homos {
		homoKeys[strings.ToLower(k)] = true
	}

	for key := range custom {
		lower := strings.ToLower(key)

		// WRONG UNIT — phonemizeWord looks up one strings.Fields token.
		if strings.ContainsAny(key, " \t") {
			t.Errorf("%q contains whitespace: lookup receives single tokens, so a "+
				"multi-word key can never match. Bank the words separately or accept "+
				"that the phrase is not bankable.", key)
		}

		// TOO EARLY — custom is consulted before the homograph resolver, so a
		// banked homograph is pinned to one reading in every context.
		if homoKeys[lower] {
			t.Errorf("%q is also a homographs.json key. custom is Step 0 and the "+
				"homograph resolver is Step 1, so this does not correct the word — it "+
				"removes its context resolution and gives it one reading everywhere.", key)
		}

		// TOO LATE — if the normalizer rewrites the token, the dictionary never
		// sees it. Asserted by running the real normalizer rather than by
		// pattern-matching what it is assumed to do.
		if got := NormalizeText(key); !strings.EqualFold(strings.TrimSpace(got), key) {
			t.Errorf("%q is rewritten by the normalizer to %q before any dictionary is "+
				"consulted, so this entry is unreachable. Banking fixes phonemization, "+
				"never normalization.", key, got)
		}
	}
}
