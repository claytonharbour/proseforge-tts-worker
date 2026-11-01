//go:build unit

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// The gold dictionary is the closest thing we have to ground truth: 90k words
// with reference pronunciations that are correct by definition. Running the
// filter over it measures its false-positive rate against known-good data —
// the denominator that a hand-picked control set of 8 names cannot provide.
//
// This exists because Wayland reported the filter flagging 51% of the 215 LTS
// names (#39) and neither of us could say whether that rate was a real property
// of LTS or an artifact of the filter. A control set proves a filter doesn't
// fire on cases you thought of; this measures what it does on 90k you didn't.

// Silver is the lower-confidence tier and is where the Devi defect lives
// (devi → dˈAvi, "Davy"). Words resolving from silver are currently treated as
// safe by the LTS sweep — they never reach LTS, so they are invisible to it.
//
// This measures whether that assumption holds structurally, by running the same
// filter over silver and comparing against gold's rate. It does NOT find Devi:
// dˈAvi is structurally clean and wrong, which is the documented blind spot. A
// higher rate here would mean silver-resolved words carry risk the sweep cannot
// see; a comparable rate means silver is at least not malformed, and Devi-class
// errors need an instrument nobody has built.
func TestStructuralFlags_SilverIsNotWorseThanGold(t *testing.T) {
	gold := loadDictionary(t, "us_gold.json")
	silver := loadDictionary(t, "us_silver.json")

	// Guard the denominators. An empty dictionary gives 0/0 = NaN, and EVERY
	// comparison against NaN is false — so `s > g*5 && s > 1.0` is false and this
	// test PASSES on a dictionary that loaded nothing. The function whose job is
	// detecting a problem reports none, from a population that does not exist.
	//
	// @wayland found the Python form of this in his calibration (`max(n, 1)`, which
	// turned an empty gold into a reassuring 0.0% error rate). Go's version is
	// quieter still: no sentinel value, just a comparison that silently stops
	// meaning anything. The sibling test below guards `len(gold) < 1000`; I wrote
	// that guard and not this one, because I was writing a threshold there and a
	// ratio here.
	rate := func(t *testing.T, name string, d map[string]string) float64 {
		t.Helper()
		if len(d) == 0 {
			t.Fatalf("%s yielded 0 pronunciations — a rate over an empty population "+
				"is NaN, and NaN passes every comparison in this test", name)
		}
		n := 0
		for _, ipa := range d {
			if len(structuralFlags(ipa)) > 0 {
				n++
			}
		}
		return float64(n) / float64(len(d)) * 100
	}
	g, s := rate(t, "gold", gold), rate(t, "silver", silver)
	t.Logf("gold   %6d entries   %.2f%% flagged", len(gold), g)
	t.Logf("silver %6d entries   %.2f%% flagged", len(silver), s)

	// Silver being several times worse would mean the sweep's "resolved from a
	// dictionary, therefore safe" assumption is unsound for a whole tier.
	if s > g*5 && s > 1.0 {
		t.Errorf("silver flags %.2f%% vs gold's %.2f%% — silver-resolved words carry "+
			"structural risk the LTS sweep cannot see, because they never reach LTS", s, g)
	}
}

func TestStructuralFlags_FalsePositiveRateOnGoldDictionary(t *testing.T) {
	gold := loadDictionary(t, "us_gold.json")
	if len(gold) < 1000 {
		t.Fatalf("gold dictionary yielded only %d pronunciations — wrong file?", len(gold))
	}

	byFlag := map[string]int{}
	var flagged int
	examples := map[string][]string{}
	for word, ipa := range gold {
		fl := structuralFlags(ipa)
		if len(fl) == 0 {
			continue
		}
		flagged++
		for _, f := range fl {
			key := f
			if len(f) > 13 && f[:13] == "NO-VOWEL-TAIL" {
				key = "NO-VOWEL-TAIL"
			}
			byFlag[key]++
			if len(examples[key]) < 8 {
				examples[key] = append(examples[key], word+" "+ipa)
			}
		}
	}

	rate := float64(flagged) / float64(len(gold)) * 100
	t.Logf("gold entries: %d", len(gold))
	t.Logf("flagged:      %d (%.2f%%)", flagged, rate)
	keys := make([]string, 0, len(byFlag))
	for k := range byFlag {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("  %-14s %6d   e.g. %v", k, byFlag[k], examples[k])
	}

	// Threshold set from the measured rate, not from taste. If a change to the
	// filter pushes this up, it is trading false positives for coverage and that
	// trade must be deliberate — re-measure and move the number consciously.
	// Measured 0.05% (41 of 90,940) after excluding two legal-coda classes the
	// gold dictionary itself revealed: ʔ+nasal (button, cotton, fatten) and
	// l+nasal (realm, kiln, microfilm). 0.5% leaves 10x headroom for dictionary
	// updates while still failing loudly if the filter goes broad.
	const maxRate = 0.5
	if rate > maxRate {
		t.Errorf("false-positive rate on known-good words is %.2f%%, above the %.1f%% ceiling — "+
			"the filter is flagging correct English, so any hit rate it reports on LTS "+
			"output is inflated by this much", rate, maxRate)
	}
}

// loadDictionary reads a Misaki dictionary, flattening the POS-keyed entries
// ({"DEFAULT": "...", "NOUN": null}) that a map[string]string decode chokes on.
func loadDictionary(t *testing.T, name string) map[string]string {
	t.Helper()
	path := filepath.Join("..", "..", "data", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("dictionary not available at %s: %v", path, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	out := map[string]string{}
	for word, v := range decoded {
		switch tv := v.(type) {
		case string:
			out[word] = tv
		case map[string]any:
			for pos, pv := range tv {
				if s, ok := pv.(string); ok {
					out[word+"/"+pos] = s
				}
			}
		}
	}
	return out
}
