//go:build unit

package phonemizer

import (
	"os"
	"strings"
	"testing"
)

// ⚑ THE DETECTOR MUST BE SHOWN TO FIND SOMETHING BEFORE ITS SILENCE MEANS ANYTHING.
//
// This is the whole point of #84. The obvious audit — tokenize a dictionary string
// and look for an UNK id — comes back clean for EVERY input, because Tokenize has no
// UNK: it drops the rune and returns a shorter slice. @Sten's first pass reported
// "533 strings, 0 UNK" and that number could not have been anything else.
//
// So the known-bad cases below run FIRST and are not decoration. If LossyRunes ever
// stops flagging "§", the corpus sweep further down becomes a green that cannot go
// red — and it would keep printing 0 while a bank entry was being silently gutted.
func TestLossyRunesFindsPlantedRunes(t *testing.T) {
	tk := newTestTokenizer(t)

	cases := []struct {
		in   string
		want string // the runes expected to be dropped, concatenated
		note string
	}{
		{"QZ§", "Z§", "@Sten's repro from #84 — two of three characters vanish"},
		{"ɹˈɛd", "", "real IPA: the detector must NOT cry wolf"},
		{"ɹˈɛd§", "§", "one planted rune inside otherwise-valid IPA"},
		{"building", "g", "⛔ ASCII 'g' U+0067 is the ONE letter absent from the vocab; " +
			"IPA wants 'ɡ' U+0261. Near-identical glyphs, so this is the typo that ships"},
		{"ɡ", "", "the SCRIPT g is present — proving the row above is about the codepoint, " +
			"not about the letter g being unrepresentable"},
		{"", "", "empty string is not lossy"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := string(tk.LossyRunes(tc.in))
			if got != tc.want {
				t.Errorf("LossyRunes(%q) = %q, want %q — %s", tc.in, got, tc.want, tc.note)
			}
		})
	}
}

// Deduplicated, in order of first appearance — so an error message names each bad
// rune once rather than once per occurrence.
func TestLossyRunesDeduplicatesInOrder(t *testing.T) {
	tk := newTestTokenizer(t)
	if got := string(tk.LossyRunes("§g§g§")); got != "§g" {
		t.Errorf("LossyRunes = %q, want %q", got, "§g")
	}
}

// The shipped data must be clean. This assertion is only worth reading because the
// test above proves the instrument can go red.
func TestShippedDictionariesAreFullyTokenizable(t *testing.T) {
	p := newTestPhonemizer(t)
	tk := newTestTokenizer(t)

	for _, c := range []struct {
		name  string
		check func() error
	}{
		{"gold", func() error { return p.gold.ValidateAgainstVocab(tk, "gold") }},
		{"silver", func() error { return p.silver.ValidateAgainstVocab(tk, "silver") }},
		{"custom_dict", func() error { return p.custom.ValidateAgainstVocab(tk, "custom_dict") }},
		{"homographs", func() error { return p.homographs.ValidateAgainstVocab(tk, "homographs") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := c.check(); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// ⚠️ THE COUNT IS THE ASSERTION, not the absence of an error.
//
// homographs.json keeps IPA in three places and one of them is a map KEY
// (Pronunciations). A validator walking only the struct's string fields sees 22 of
// 62 readings and reports the other 40 as checked — silently, with no symptom.
//
// I wrote that validator. It was caught only because #63 and @Sten had each
// published the number 62 independently, and mine said 22. Pinning the count here
// is what makes the next such omission fail instead of pass quietly.
func TestHomographValidationReachesEveryReading(t *testing.T) {
	p := newTestPhonemizer(t)

	seen := 0
	for _, e := range p.homographs.entries {
		for _, r := range []string{e.Default, e.Imperative, e.InflectedDefault} {
			if r != "" {
				seen++
			}
		}
		seen += len(e.Pronunciations)
	}

	// 62 until #112 added `lives` (a default and two readings).
	const want = 65
	if seen != want {
		t.Errorf("validation walks %d homograph readings, want %d.\n"+
			"If readings were legitimately added or removed, update this number AND "+
			"confirm ValidateAgainstVocab still reaches the Pronunciations MAP KEYS — "+
			"that is the half a struct-field walk silently misses (#84, #63)", seen, want)
	}
}

// A corrupt custom_dict must be an ERROR, not an empty bank.
//
// `custom, _ = LoadMisakiDict(...)` discarded parse errors as well as missing-file
// errors, so one stray comma emptied the bank in silence. Every hand-banked reading
// would revert to LTS — which sounds like a worse voice, not like a broken file, so
// nobody would look at the JSON.
func TestCorruptCustomDictIsAnErrorNotAnEmptyBank(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/custom_dict.json", "{\"karen\": \"kˈæɹən\",}") // trailing comma

	cfg := testConfig()
	cfg.CustomDictPath = dir + "/custom_dict.json"
	_, err := New(cfg)
	if err == nil {
		t.Fatal("New succeeded with a corrupt custom_dict — the bank was silently " +
			"emptied and every banked reading would fall through to LTS")
	}
	if !strings.Contains(err.Error(), "present but unusable") {
		t.Errorf("error = %v, want it to say the file is present but unusable", err)
	}

	// CONTROLS: the two NON-defects must both still load. Without these the fix above
	// is indistinguishable from "custom_dict is now mandatory and must be non-empty".
	//
	// ⚠️ The empty case is not hypothetical. Collapsing `{}` into "unusable" broke
	// bank_redundancy_test, which writes exactly that fixture to measure what the
	// engine produces with no bank. A well-formed file saying "no entries" is a
	// STATE; only a parse failure is a defect.
	for _, c := range []struct{ name, path, content string }{
		{"absent", dir + "/does_not_exist.json", ""},
		{"empty", dir + "/empty.json", "{}"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.content != "" {
				writeFile(t, c.path, c.content)
			}
			cfg := testConfig()
			cfg.CustomDictPath = c.path
			if _, err := New(cfg); err != nil {
				t.Errorf("New failed on an %s custom dict: %v — that is a state, not a defect", c.name, err)
			}
		})
	}
}

// --- helpers ---------------------------------------------------------------

// testDataDir honours MISAKI_DATA_DIR for the same reason newTestPhonemizer does:
// red-proofing a data-dependent assertion must not require editing the shared
// data/ that other benches are reading in the same minute.
func testDataDir() string {
	if d := os.Getenv("MISAKI_DATA_DIR"); d != "" {
		return d
	}
	return "../../data"
}

func testConfig() Config {
	d := testDataDir()
	return Config{
		GoldDictPath:   d + "/us_gold.json",
		SilverDictPath: d + "/us_silver.json",
		HomographsPath: d + "/homographs.json",
		TokenMapPath:   d + "/token_map.json",
		ARPABETMapPath: d + "/arpabet_to_ipa.json",
		CustomDictPath: d + "/custom_dict.json",
	}
}

func newTestTokenizer(t *testing.T) *Tokenizer {
	t.Helper()
	tk, err := LoadTokenizer(testDataDir() + "/token_map.json")
	if err != nil {
		t.Fatalf("LoadTokenizer: %v", err)
	}
	return tk
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
