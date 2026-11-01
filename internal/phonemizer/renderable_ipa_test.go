//go:build unit

package phonemizer

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

// Every IPA symbol we ship must have a token, because the tokenizer discards the
// ones that do not:
//
//	for _, r := range phonemes {
//	    if id, ok := t.vocab[r]; ok { tokens = append(tokens, id) }
//	}
//
// No error, no warning — the phoneme simply vanishes and the word is spoken
// wrong with nothing anywhere reporting it. ⚑ The realistic way in is a
// homoglyph: IPA uses ɡ (U+0261 LATIN SMALL LETTER SCRIPT G), not the ASCII g on
// every keyboard, and they are indistinguishable in most fonts. I typed the
// wrong one myself while hand-building a suffix rule and nearly filed the result
// as an engine defect.
//
// ✅ All five sources measure clean today — custom_dict 85, homographs 62,
// us_gold 90,940, us_silver 93,361, arpabet_to_ipa 43 strings, zero unrenderable
// symbols. This exists to keep it that way: it is green on arrival and goes red
// the first time somebody banks a pronunciation with a symbol Kokoro has no
// token for.
//
// ⛔ Not a hypothetical guard against nothing: `custom_dict.json` is the file
// authors edit by hand to fix a name, which is precisely where a pasted or
// mistyped symbol arrives.
func TestEveryShippedIPASymbolHasAToken(t *testing.T) {
	raw, err := os.ReadFile("../../data/token_map.json")
	if err != nil {
		t.Fatalf("read token map: %v", err)
	}
	var tm struct {
		Vocab map[string]int64 `json:"vocab"`
	}
	if err := json.Unmarshal(raw, &tm); err != nil {
		t.Fatalf("parse token map: %v", err)
	}
	if len(tm.Vocab) == 0 {
		t.Fatal("token vocab is empty — every check below would pass over nothing")
	}
	has := func(r rune) bool { _, ok := tm.Vocab[string(r)]; return ok }

	// Each source is checked separately so a failure names the file to edit.
	for _, src := range []struct {
		path string
		// pull returns every IPA string in the file. A null value is NOT a
		// string and must be skipped rather than stringified — my first pass
		// rendered JSON nulls as the text "None" and reported its N as 67
		// dropped phonemes in gold. The data was clean; the instrument was not.
		pull func(map[string]json.RawMessage) []string
	}{
		{"../../data/custom_dict.json", pullFlat},
		{"../../data/us_gold.json", pullFlat},
		{"../../data/us_silver.json", pullFlat},
		{"../../data/homographs.json", pullHomograph},
		// ⚑ LTS phonemes exist in no file — they are generated at call time, so a
		// file scan cannot reach them. @wayland measured 653 LTS rows from his
		// corpus and found them clean, and was precise that this is a SAMPLE of
		// what one corpus made the generator emit, not the generator's output
		// space.
		//
		// It can be bounded instead of sampled. arpabetToIPA writes exactly three
		// things and nothing else:
		//
		//	"ˈ" on stress 1 · "ˌ" on stress 2 · l.arpToIPA[base]
		//
		// An unmapped base is skipped outright. So every rune LTS can EVER emit
		// is either a stress mark or a value in this map's `combined` — checking
		// it here turns his sample into a proof about the generator.
		{"../../data/arpabet_to_ipa.json", pullARPABET},
	} {
		t.Run(src.path, func(t *testing.T) {
			b, err := os.ReadFile(src.path)
			if err != nil {
				t.Skipf("not readable: %v", err)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("parse: %v", err)
			}
			strs := src.pull(m)
			if len(strs) == 0 {
				t.Fatal("no IPA strings extracted — the check would pass over nothing")
			}
			bad := 0
			for _, s := range strs {
				for _, r := range s {
					if !has(r) {
						bad++
						if bad <= 10 {
							t.Errorf("%q contains %q (U+%04X), which has no token — "+
								"the tokenizer will DROP it silently. If this is a "+
								"homoglyph, IPA uses ɡ U+0261 rather than ASCII g.",
								s, r, r)
						}
					}
				}
			}
			if bad > 10 {
				t.Errorf("... and %d more unrenderable symbols", bad-10)
			}
			t.Logf("%d strings checked, %d unrenderable", len(strs), bad)
		})
	}
}

// pullFlat handles a dictionary of word → IPA, or word → {POS: IPA}. A POS whose
// value is null means "no entry for this part of speech" and yields nothing.
func pullFlat(m map[string]json.RawMessage) []string {
	var out []string
	for k, v := range m {
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			out = append(out, s)
			continue
		}
		var byPOS map[string]*string
		if json.Unmarshal(v, &byPOS) == nil {
			for _, p := range byPOS {
				if p != nil {
					out = append(out, *p)
				}
			}
		}
	}
	return out
}

// pullHomograph collects the default plus every pronunciation KEY, since the
// keys are themselves the IPA.
func pullHomograph(m map[string]json.RawMessage) []string {
	var out []string
	for k, v := range m {
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		var e struct {
			Default          string                     `json:"default"`
			Imperative       string                     `json:"imperative"`
			InflectedDefault string                     `json:"inflected_default"`
			Pronunciations   map[string]json.RawMessage `json:"pronunciations"`
		}
		if json.Unmarshal(v, &e) != nil {
			continue
		}
		for _, s := range []string{e.Default, e.Imperative, e.InflectedDefault} {
			if s != "" {
				out = append(out, s)
			}
		}
		for pron := range e.Pronunciations {
			out = append(out, pron)
		}
	}
	return out
}

// pullARPABET returns everything arpabetToIPA can write: the two stress marks it
// emits directly, plus every value of the `combined` map it looks bases up in.
func pullARPABET(m map[string]json.RawMessage) []string {
	out := []string{"\u02c8", "\u02cc"} // ˈ ˌ — written by arpabetToIPA itself
	var combined map[string]string
	if raw, ok := m["combined"]; ok {
		if err := json.Unmarshal(raw, &combined); err == nil {
			for _, v := range combined {
				out = append(out, v)
			}
		}
	}
	return out
}

// Phonemes written in Go rather than read from a data file are invisible to the
// scan above — it walks five JSON sources and cannot open a .go file.
//
// @wayland proved the gap rather than arguing it: on a copy of the tree he
// planted the turned-e homoglyph into the two LTS-schwa literals and the ENTIRE unit suite
// stayed green, 10.6s. `ə` was covered only because it is coincidentally a value
// in arpabet_to_ipa.json — change the literal and nothing failed.
//
// ⛔ A source scan cannot fix that. `normalizer.go` legitimately contains ≈, §
// and – in string literals, none of which are in the vocab and none of which
// need to be: they are INPUT characters the normalizer replaces (≈ →
// " approximately ", – → —). An "every non-ASCII literal must be renderable"
// check would flag all three. Input-side and output-side literals are not
// mechanically distinguishable.
//
// So the literals are named instead, and named constants can be asserted.
func TestGoLiteralPhonemesAreRenderable(t *testing.T) {
	raw, err := os.ReadFile("../../data/token_map.json")
	if err != nil {
		t.Fatalf("read token map: %v", err)
	}
	var tm struct {
		Vocab map[string]int64 `json:"vocab"`
	}
	if err := json.Unmarshal(raw, &tm); err != nil {
		t.Fatalf("parse token map: %v", err)
	}
	if len(tm.Vocab) == 0 {
		t.Fatal("token vocab is empty — the checks below would pass over nothing")
	}

	for _, c := range []struct{ name, ipa string }{
		{"phonemeSchwa", phonemeSchwa},
		{"phonemeThe", phonemeThe},
		{"phonemeJoin", phonemeJoin},
	} {
		if c.ipa == "" {
			t.Errorf("%s is empty — a phoneme constant that emits nothing is a "+
				"silent deletion, which is the defect this guards", c.name)
			continue
		}
		for _, r := range c.ipa {
			if _, ok := tm.Vocab[string(r)]; !ok {
				// ⚠️ Name the character and stop. An earlier version of this
				// message also asserted "ǝ U+01DD is the homoglyph of ə U+0259
				// and is the realistic way this arrives" — true for the schwa
				// constants and irrelevant to the join separator, where the
				// realistic arrival is NBSP. A hint that names a cause the check
				// cannot know is the error-string species: it describes more
				// than the code does, and sends the reader somewhere specific
				// and wrong.
				t.Errorf("%s = %q contains %q (U+%04X), which has no token — the "+
					"tokenizer DROPS it silently, with no error anywhere. Look for a "+
					"homoglyph or an invisible character: IPA ə is U+0259 (not ǝ U+01DD), "+
					"and a separator must be U+0020 (not NBSP U+00A0).",
					c.name, c.ipa, r, r)
			}
		}
	}
}

// The table in TestGoLiteralPhonemesAreRenderable is hand-listed, which is the
// failure it exists to prevent, one level up. `1a34473` named the two phoneme
// literals it had found and asserted over exactly those two — and a THIRD already
// existed at two sites, invisible to a test whose subject was a list someone
// wrote out.
//
// @wayland found it and named the better fix on #66: derive the table from the
// constants, so a fourth literal cannot repeat this.
//
// Go cannot enumerate package constants at runtime, so this reads the const block
// from source instead. ⚠️ Deliberately NOT a scan of every string literal in the
// package — normalizer.go legitimately holds ≈, § and – as INPUT characters that
// must not be renderable, so a whole-file rule flags three correct lines. The
// const block is a narrow declared region, which is what makes scanning it safe.
func TestEveryPhonemeConstantIsCovered(t *testing.T) {
	src, err := os.ReadFile("phonemizer.go")
	if err != nil {
		t.Fatalf("read phonemizer.go: %v", err)
	}
	decl := regexp.MustCompile(`(?m)^\s*(phoneme[A-Za-z0-9_]*)\s*=`)
	found := decl.FindAllStringSubmatch(string(src), -1)
	if len(found) == 0 {
		t.Fatal("no phoneme* constants found — the regex has drifted from the " +
			"declaration style, and this check would pass over nothing")
	}

	// Names the table asserts. Kept beside the table it mirrors.
	covered := map[string]bool{
		"phonemeSchwa": true,
		"phonemeThe":   true,
		"phonemeJoin":  true,
	}
	for _, m := range found {
		if !covered[m[1]] {
			t.Errorf("%s is declared in phonemizer.go but TestGoLiteralPhonemesAreRenderable "+
				"does not assert it. A phoneme written in Go is invisible to every data-file "+
				"scan, so an unasserted constant can carry a homoglyph or an invisible "+
				"character and ship silently. Add it to that table and to `covered` here.",
				m[1])
		}
	}
	// ⚠️ A count check, because len(found)==0 only catches TOTAL drift.
	//
	// Measured: rewriting one declaration as a grouped `a, b = x, y` — valid Go,
	// compiles, usages unchanged — made the regex find 2 where 3 are declared.
	// The new constant was unasserted and this test PASSED, logging "2 declared,
	// 3 covered" as though that were a sensible thing to print. A partial regex
	// miss is invisible to an emptiness guard, which is the same species this
	// whole file exists to catch, one level further in.
	if len(found) != len(covered) {
		t.Errorf("regex found %d phoneme constants but %d are asserted — these must "+
			"match. Either a declaration style changed and the regex no longer sees "+
			"it (check for a grouped `a, b = x, y`), or `covered` lists a name that "+
			"no longer exists. A silently SMALLER match is the dangerous direction: "+
			"it passes.", len(found), len(covered))
	}
	t.Logf("%d phoneme constants declared, %d covered", len(found), len(covered))
}
