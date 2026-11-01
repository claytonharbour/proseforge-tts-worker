//go:build unit

package main

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/claytonharbour/proseforge-tts-worker/internal/phonemizer"
)

// This tool emits FOUR streams, not one: default stdout, -json, -sources, and
// -json -sources. Twice now I have verified byte-stability on default stdout,
// announced "output unchanged", and moved a stream somebody else parses:
//
//	190e1c1   changed what `> f.json 2>&1` captured for @wayland's regress.sh
//	19586f4   added WordDiagnosis.Rule — default stdout byte-identical, and
//	          every -json inventory row silently gained "Rule": ""
//
// Both times the claim was true about the stream I checked and irrelevant to
// the stream that mattered. A diff proves nothing about the streams it did not
// read, so the field list is pinned here rather than left to whoever remembers
// to diff the right one.
//
// If this test fails you have changed the -json contract. That is allowed —
// but ANNOUNCE IT to the people parsing it before merging, because their
// baselines will move and a schema change reads to them as a stale binary.
func TestJSONInventorySchemaIsPinned(t *testing.T) {
	// Marshal the actual struct the inventory arrays are built from, so this
	// tracks the emitted shape rather than a copy of it that can drift.
	blob, err := json.Marshal(phonemizer.WordDiagnosis{})
	if err != nil {
		t.Fatalf("marshal WordDiagnosis: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(blob, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := make([]string, 0, len(fields))
	for k := range fields {
		got = append(got, k)
	}
	sort.Strings(got)

	want := []string{"Phonemes", "Rule", "Source", "Word"}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("-json inventory row fields changed:\n  got  %v\n  want %v\n\n"+
			"Consumers parse this stream. Announce the change to them — @wayland's "+
			"regress.sh and recall tooling both read it — then update `want` here. "+
			"Do not update `want` first and call the suite green.", got, want)
	}
}

// A guard over an empty struct passes trivially forever. If WordDiagnosis ever
// loses every field, the test above would still be comparing two empty lists.
func TestJSONSchemaGuardHasSomethingToGuard(t *testing.T) {
	blob, err := json.Marshal(phonemizer.WordDiagnosis{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(blob, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("WordDiagnosis marshals to zero fields — the schema pin above is " +
			"comparing nothing to nothing and cannot fail")
	}
}

// The pin above covers the FIELDS of a row. @wayland found the gap it leaves:
// it says nothing about WHICH INVENTORIES APPEAR.
//
// His gate reads gold/custom/homograph out of `-sources` output. Drop one of
// those keys upstream and every surviving row still has all four fields — so
// the field pin stays green while his coverage narrows from 92.3% to 10.5%.
// A green that means NOT COVERED is the same shape as the hole it guards.
func TestJSONTopLevelKeysArePinned(t *testing.T) {
	empty := map[string][]phonemizer.WordDiagnosis{}

	// Base payload: what every -json run emits regardless of flags.
	base := buildJSONOutput(nil, map[string]int{}, empty, "", "data", "6 tables, deadbeef")
	gotBase := sortedKeys(base)
	// ⚑ data_dir + data_fingerprint added 2026-08-18, ANNOUNCED to @wayland in the
	// room before merge, per this test's own instruction. Strictly ADDITIVE — no
	// field removed or renamed — so an existing consumer keeps working.
	//
	// They exist because the digest was on STDERR only, so anything piping stdout
	// or parsing -json could not tell WHICH dictionaries produced the numbers.
	// data/ changed EIGHT times on 2026-08-18; @wayland compared a count taken
	// before five of those commits against one taken after and published the
	// difference three times as an instrument property. Banking moves words
	// between sources (lts 7,568→7,559, custom 5,762→5,786), so a stale
	// comparison shows a gap that never existed. The fingerprint makes two
	// non-comparable runs visibly non-comparable.
	wantBase := []string{"counts", "data_dir", "data_fingerprint", "lts_inventory", "silver_inventory", "skip_inventory", "total_words"}
	if strings.Join(gotBase, ",") != strings.Join(wantBase, ",") {
		t.Errorf("-json base keys changed:\n  got  %v\n  want %v", gotBase, wantBase)
	}

	// The contract @wayland's coverage depends on: asking for these three
	// produces these three inventories, by these exact names.
	with := buildJSONOutput(nil, map[string]int{}, empty, "gold,custom,homograph", "data", "6 tables, deadbeef")
	for _, k := range []string{"gold_inventory", "custom_inventory", "homograph_inventory"} {
		if _, ok := with[k]; !ok {
			t.Errorf("-sources gold,custom,homograph did not emit %q — a consumer "+
				"reading that key gets nothing and its coverage silently narrows "+
				"instead of failing", k)
		}
	}
}

// A key-set assertion over an always-empty payload would pass even if
// buildJSONOutput returned a constant. Prove the extra sources actually move it.
func TestJSONTopLevelKeyGuardDiscriminates(t *testing.T) {
	empty := map[string][]phonemizer.WordDiagnosis{}
	base := len(buildJSONOutput(nil, map[string]int{}, empty, "", "data", "6 tables, deadbeef"))
	with := len(buildJSONOutput(nil, map[string]int{}, empty, "gold,custom,homograph", "data", "6 tables, deadbeef"))
	if with != base+3 {
		t.Fatalf("asking for 3 extra sources changed the key count by %d, want 3 — "+
			"the test above cannot detect a dropped inventory if the payload "+
			"does not respond to -sources at all", with-base)
	}
}

func sortedKeys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// @wayland measured that our two guards INTERLOCK rather than overlap: mine
// catches an inventory key that goes MISSING, his catches one that is PRESENT
// BUT EMPTY. His half is a shell script in his tree, which means half the
// interlock lives somewhere this repo's suite cannot see — and a guard that
// depends on someone else's private script is one machine away from absent.
//
// So: this repo detects the present-but-empty case on its own.
func TestJSONInventoriesCarryTheirRows(t *testing.T) {
	populated := map[string][]phonemizer.WordDiagnosis{
		"gold":      {{Word: "glasses", Phonemes: "ɡlˈæsᵻz", Source: "gold"}},
		"custom":    {{Word: "Ruiz's", Phonemes: "ɹuˈisɪz", Source: "custom", Rule: "'s"}},
		"homograph": {{Word: "read", Phonemes: "ɹˈɛd", Source: "homograph"}},
		"lts":       {{Word: "Whitfield", Phonemes: "wˈɪtfˈild", Source: "lts"}},
	}

	out := buildJSONOutput(nil, map[string]int{}, populated, "gold,custom,homograph", "data", "6 tables, deadbeef")

	for _, k := range []string{"gold_inventory", "custom_inventory", "homograph_inventory", "lts_inventory"} {
		rows, ok := out[k].([]phonemizer.WordDiagnosis)
		if !ok {
			t.Errorf("%s is not a row slice (%T) — a consumer ranging over it gets nothing", k, out[k])
			continue
		}
		if len(rows) == 0 {
			t.Errorf("%s is present but EMPTY while its source has rows. The key-set "+
				"assertion above passes on this, which is why it is not enough: a "+
				"consumer's coverage silently narrows instead of failing.", k)
		}
	}
}
