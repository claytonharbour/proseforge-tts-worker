//go:build unit

package phonemizer

import (
	"encoding/json"
	"os"
	"testing"
)

// TestEveryPOSEntryHasAUsableDefault pins an assumption the loader makes
// silently and does not check.
//
// `LoadMisakiDict` stores POS-tagged entries like this (misaki_dict.go):
//
//	if def, ok := posMap["DEFAULT"]; ok && def != nil {
//	    d.entries[key] = *def
//	}
//
// 🛑 There is no `else`. An entry whose POS dict lacks `DEFAULT`, or whose
// DEFAULT is null, is **dropped from the dictionary entirely** — the word does
// not fall back to another POS value, it simply stops existing in gold and falls
// through to LTS on the next lookup. No error, no log line, no count.
//
// Measured 2026-08-21: 0 such entries in any of the four data files, so the omission
// costs nothing today. ⚑ That is exactly why it needs pinning rather than
// fixing — `data/` moves, these files are regenerated from upstream Misaki, and
// the failure mode of an update that introduces one is a word that quietly
// leaves the dictionary. A dropped word is the hardest defect class this repo
// has: #25, #43 and half of #42 were all "silently absent rather than garbled".
//
// 📌 Context, @wayland 2026-08-21: every non-DEFAULT value in these files is
// unreachable by construction — `entries` is `map[string]string`, one reading
// per word, and no shipping code reads "VERB", "NOUN" or any other POS key. So
// gold's 671 two-reading entries offer the ENGINE exactly one reading each. This
// test does not change that and is not an argument about it; it only guarantees
// that the one reading gold does supply cannot go missing without a red.
func TestEveryPOSEntryHasAUsableDefault(t *testing.T) {
	// ⚑ Overridable so this test can be RED-PROOFED without mutating the shared
	// `data/` other benches measure against. @wayland could not red-test the
	// previous version for exactly that reason and said so rather than letting
	// "verified" cover a read-and-run-green. **A test only its author can make
	// fail is one only its author can trust.**
	dir := os.Getenv("MISAKI_DATA_DIR")
	if dir == "" {
		dir = "../../data"
	}
	for _, name := range []string{"us_gold.json", "us_silver.json"} {
		raw, err := os.ReadFile(dir + "/" + name)
		if err != nil {
			t.Skipf("%s absent: %v", name, err)
		}
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		posEntries, dropped, silent := 0, 0, 0
		for word, val := range entries {
			var s string
			if json.Unmarshal(val, &s) == nil {
				continue // bare string — the loader keeps it
			}
			var pos map[string]*string
			if json.Unmarshal(val, &pos) != nil {
				continue
			}
			posEntries++
			// ⚠️ THREE unusable states, and the loader treats them as TWO.
			//
			//	no DEFAULT      → word DROPPED      (falls to LTS)
			//	DEFAULT null    → word DROPPED      (falls to LTS)
			//	DEFAULT ""      → word KEPT, silent (does NOT fall to LTS)
			//
			// 🛑 The first version of this test mirrored the loader's own condition
			// (`!ok || def == nil`) and was therefore blind to the third — @wayland,
			// 2026-08-21. **A test written to mirror the code under test inherits the
			// code's blind spots**: it asked "does the data satisfy what the loader
			// CHECKS" instead of "what the loader NEEDS".
			//
			// The empty case is the worse one. A dropped word still gets a
			// pronunciation from LTS — wrong, perhaps, but audible. An empty reading
			// is accepted as authoritative and the word says nothing at all.
			def, ok := pos["DEFAULT"]
			switch {
			case !ok || def == nil:
				dropped++
				t.Errorf("%s: %q is a POS entry with no usable DEFAULT, so LoadMisakiDict "+
					"drops it from the dictionary entirely and the word falls through to LTS "+
					"with no error. Keys present: %v", name, word, keysOf(pos))
			case *def == "":
				// ⚠️ NOT dropped — counted separately because it is the opposite
				// outcome. @wayland: the first version incremented `dropped` here,
				// two lines under a comment saying the loader KEEPS it. The
				// per-entry message was right and the aggregate contradicted it —
				// and being non-zero only on failure, it would have been wrong
				// exactly when someone first read it.
				silent++
				t.Errorf("%s: %q has an EMPTY DEFAULT. The loader keeps it (present and "+
					"non-nil), so the word resolves from gold to no phonemes at all and is "+
					"SILENT — it does not even fall through to LTS. Keys present: %v",
					name, word, keysOf(pos))
			}
		}

		// ⚠️ The denominator matters: if a future format change stopped producing
		// POS dicts at all, every assertion above would vacuously pass. us_gold
		// has 790 today and us_silver has none, which is itself the shape of the
		// file rather than an accident.
		t.Logf("%s: %d POS entries checked · %d would be DROPPED (fall to LTS) · %d would be KEPT AND SILENT",
			name, posEntries, dropped, silent)
		if name == "us_gold.json" && posEntries == 0 {
			t.Errorf("us_gold.json has no POS-tagged entries at all — the loader's " +
				"POS branch is now dead code and this test checks nothing. Confirm the " +
				"format change was intended before deleting either.")
		}
	}
}

func keysOf(m map[string]*string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
