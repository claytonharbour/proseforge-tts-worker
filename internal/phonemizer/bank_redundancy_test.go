//go:build unit

package phonemizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// A banked entry the engine would produce anyway is harmless — and invisible, which is the
// problem. If LTS regresses on `marcus`, the bank silently covers it and nothing reports
// that the generator got worse. That is the same silence as #53's missing `AX`: no error,
// no signal, just a wrong thing quietly not happening.
//
// This pins which entries are currently redundant. It fails in BOTH directions, and the
// directions mean opposite things:
//
//	redundant → needed    the engine got WORSE and the bank is now hiding it   ← investigate
//	needed → redundant    the engine improved; the entry can be retired        ← good news
//
// It is the only instrument here that gets more useful as LTS improves, and it matters more
// after #53 than before: restoring the schwa took our structural detectors from 66% recall
// to 22%, so most remaining defects are now well-formed strings no filter can see.
//
// ⛔ Redundant does NOT mean deletable. See TestBankOverridesAreNotRedundant — the
// gold-resolved entries exist precisely because they disagree with gold.
//
// ⚠️ THIS COUNT IS BYTE-EQUALITY. It is 18 here and 19 in @wayland's corpus pin, and both
// are right: his normalises `ɜɹ` to `əɹ` before comparing, because #52 established that is
// a notation difference rather than a sound one — LTS writes the r-coloured schwa `ɜɹ` where
// gold writes `əɹ`, and across this population `ɜ` never occurs anywhere else (measured: 0
// bare `ɜ`). *Redundant for sound* and *redundant for bytes* are different questions.
// TestBankRedundancyDefinitionsDivergeOnlyOnSutherland pins where they part company, because
// a count whose definition is not written down invites the exchange that produced this note.
func TestBankRedundancyIsPinned(t *testing.T) {
	withBank := loadTestPhonemizer(t)
	withoutBank := loadPhonemizerWithoutBank(t)

	bank := loadBank(t)
	var redundant []string
	for word, banked := range bank {
		got, err := withoutBank.Phonemize(word)
		if err != nil {
			t.Fatalf("Phonemize(%q) without bank: %v", word, err)
		}
		if got == banked {
			redundant = append(redundant, word)
		}

		// Sanity: with the bank, every entry must produce its banked value. If this
		// fails the bank is not being consulted and the whole measurement is void.
		if got, err := withBank.Phonemize(word); err != nil || got != banked {
			t.Fatalf("%s with bank = %q (err %v), want the banked %q — "+
				"the bank is not being applied, so redundancy cannot be measured",
				word, got, err, banked)
		}
	}
	sort.Strings(redundant)

	// Measured 2026-08-16 after #53 (`AX` → `ə`). Six of these became redundant with
	// that fix — hanson, lina, magnus, mara, marcus, silas were banked only because a
	// schwa was being deleted.
	//
	// ⚠️ `forties` and `workbenches` are here because this test put them here. I had
	// counted them as overrides by asking what gold HOLDS (no exact key, so "differs")
	// instead of what the engine PRODUCES — suffix-stripping reaches forty+z and
	// workbench+ɪz, which is exactly the banked string. My published count of 16 was
	// wrong; it is 18. The component question and the pipeline question again.
	want := []string{
		"aldric", "chen", "enzo", "forties", "hanson", "joon-ho", "kai", "kofi",
		"lina", "magnus", "mara", "marcus", "multi", "noc", "non", "silas",
		"workbench", "workbenches",
	}

	if diff := setDiff(want, redundant); diff != "" {
		t.Errorf("the set of redundant bank entries moved:\n%s\n"+
			"A entry that became NEEDED means LTS regressed and the bank is now hiding it — "+
			"find out why before updating this list. An entry that became REDUNDANT means "+
			"the generator improved and the entry can be retired.", diff)
	}
}

// The counterpart, and the reason "redundant" must never be read as "deletable": these
// entries resolve from gold when the bank is removed, and they deliberately DISAGREE with
// what gold says. Deleting one silently reverts a decision someone made on purpose.
func TestBankOverridesAreNotRedundant(t *testing.T) {
	withoutBank := loadPhonemizerWithoutBank(t)
	bank := loadBank(t)

	for _, word := range []string{
		"bed",      // unstressed function-word reading
		"breathed", // gold has bɹˈɛθt (the "breathed on it" sense)
		"devi",     // gold AND silver carry dˈAvi, "Davy" — wrong for this character
		"gelada",   // gold has ʤəlˈɑdə
		"hvac",     // gold stresses it the other way
		"torres",   // #54: gold suffix-strips it to torr + z
	} {
		banked, ok := bank[word]
		if !ok {
			t.Fatalf("%s is no longer in custom_dict — if it was retired, say so here; "+
				"this test asserts it is an override, not that it exists by accident", word)
		}
		got, err := withoutBank.Phonemize(word)
		if err != nil {
			t.Fatalf("Phonemize(%q): %v", word, err)
		}
		if got == banked {
			t.Errorf("%s = %q with and without the bank — it is no longer an override. "+
				"Move it to the redundant list in TestBankRedundancyIsPinned rather than "+
				"leaving it here, where it reads as a deliberate disagreement", word, got)
		}
	}
}

// loadPhonemizerWithoutBank builds the real pipeline against an empty custom dictionary,
// so what it returns is what the engine would say if the entry had never been banked.
// Wayland's method: strip from a copy and run the whole engine, rather than calling a
// layer directly and reporting the component's answer as the pipeline's.
func loadPhonemizerWithoutBank(t *testing.T) *Phonemizer {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "custom_dict.json")
	if err := os.WriteFile(empty, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write empty custom dict: %v", err)
	}
	p, err := New(Config{
		GoldDictPath:   "../../data/us_gold.json",
		SilverDictPath: "../../data/us_silver.json",
		HomographsPath: "../../data/homographs.json",
		TokenMapPath:   "../../data/token_map.json",
		ARPABETMapPath: "../../data/arpabet_to_ipa.json",
		CustomDictPath: empty,
	})
	if err != nil {
		t.Fatalf("New without bank: %v", err)
	}
	return p
}

func loadBank(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("../../data/custom_dict.json")
	if err != nil {
		t.Fatalf("read custom_dict: %v", err)
	}
	var bank map[string]string
	if err := json.Unmarshal(raw, &bank); err != nil {
		t.Fatalf("parse custom_dict: %v", err)
	}
	if len(bank) == 0 {
		t.Fatal("custom_dict is empty — every entry would look redundant")
	}
	return bank
}

// setDiff reports entries present in one set and not the other, labelled by what the
// change means rather than as a bare +/- list.
func setDiff(want, got []string) string {
	in := func(xs []string, x string) bool {
		for _, y := range xs {
			if x == y {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	for _, w := range want {
		if !in(got, w) {
			b.WriteString("  NOW NEEDED (was redundant): " + w + "\n")
		}
	}
	for _, g := range got {
		if !in(want, g) {
			b.WriteString("  NOW REDUNDANT (was needed): " + g + "\n")
		}
	}
	return b.String()
}

// Byte-equality and sound-equality give 18 and 19, and the whole difference is one entry.
// Pinning the seam rather than picking a side: whichever definition a future reader assumes,
// this says what the other one gives and which key separates them.
//
// It also pins the invariant the normalisation rests on — that LTS's `ɜ` only ever appears
// before `ɹ`. If a bare `ɜ` shows up, `ɜɹ ≡ əɹ` stops being a safe rewrite and @wayland's
// count silently starts merging things that are genuinely different.
func TestBankRedundancyDefinitionsDivergeOnlyOnSutherland(t *testing.T) {
	p := loadPhonemizerWithoutBank(t)
	bank := loadBank(t)

	// #52: LTS writes ɜɹ where gold writes əɹ for the same r-coloured schwa.
	norm := func(s string) string { return strings.ReplaceAll(s, "ɜɹ", "əɹ") }

	var onlyNormalised []string
	strict, loose := 0, 0
	for word, banked := range bank {
		got, err := p.Phonemize(word)
		if err != nil {
			t.Fatalf("Phonemize(%q): %v", word, err)
		}
		if got == banked {
			strict++
		}
		if norm(got) == norm(banked) {
			loose++
			if got != banked {
				onlyNormalised = append(onlyNormalised, word)
			}
		}

		// The invariant: a bare ɜ would make the rewrite above unsound.
		for i, r := range []rune(got) {
			if r != 'ɜ' {
				continue
			}
			rest := []rune(got)[i+1:]
			if len(rest) == 0 || rest[0] != 'ɹ' {
				t.Errorf("%s = %q has a ɜ that is not before ɹ — `ɜɹ ≡ əɹ` is no longer a "+
					"safe normalisation and the 19 count would merge distinct sounds", word, got)
			}
		}
	}
	sort.Strings(onlyNormalised)

	if strict != 18 || loose != 19 {
		t.Errorf("byte-equal = %d (want 18), sound-equal = %d (want 19) — if a fix moved these, "+
			"update both numbers together and say which definition changed", strict, loose)
	}
	if want := []string{"sutherland"}; !reflect.DeepEqual(onlyNormalised, want) {
		t.Errorf("entries redundant by sound but not by bytes = %v, want %v — a new one means "+
			"LTS and the bank have diverged on notation somewhere else", onlyNormalised, want)
	}
}
