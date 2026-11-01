//go:build unit

package phonemizer

import (
	"os"
	"strings"
	"testing"
)

func loadTestPhonemizer(t *testing.T) *Phonemizer {
	t.Helper()
	cfg := Config{
		GoldDictPath:   "../../data/us_gold.json",
		SilverDictPath: "../../data/us_silver.json",
		HomographsPath: "../../data/homographs.json",
		TokenMapPath:   "../../data/token_map.json",
		ARPABETMapPath: "../../data/arpabet_to_ipa.json",
		CustomDictPath: "../../data/custom_dict.json",
	}
	for _, p := range []string{cfg.GoldDictPath, cfg.SilverDictPath, cfg.HomographsPath, cfg.TokenMapPath, cfg.ARPABETMapPath} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("data file not found: %s", p)
		}
	}
	ph, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ph
}

func TestContractionPronunciation(t *testing.T) {
	ph := loadTestPhonemizer(t)

	// Contractions should use gold dictionary values (no overrides).
	// The model was trained on these exact phoneme sequences.
	tests := []struct {
		name    string
		input   string
		wantIPA string
	}{
		{"she'd", "she'd", "ʃid"},
		{"he'd", "he'd", "hid"},
		{"we'd", "we'd", "wid"},
		{"they'd", "they'd", "ðAd"},
		{"who'd", "who'd", "hˌud"},
		{"you'd", "you'd", "jud"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ph.Phonemize(tt.input)
			if err != nil {
				t.Fatalf("Phonemize(%q): %v", tt.input, err)
			}
			if got != tt.wantIPA {
				t.Errorf("Phonemize(%q) = %q, want %q", tt.input, got, tt.wantIPA)
			}
		})
	}
}

func TestCurlyApostropheContraction(t *testing.T) {
	ph := loadTestPhonemizer(t)

	// Curly apostrophe: she\u2019d should be normalized and match gold dict
	got, err := ph.Phonemize("she\u2019d")
	if err != nil {
		t.Fatalf("Phonemize: %v", err)
	}
	if got != "ʃid" {
		t.Errorf("Phonemize(\"she\\u2019d\") = %q, want %q", got, "ʃid")
	}
}

func TestArticleA(t *testing.T) {
	ph := loadTestPhonemizer(t)

	// The article "a" should be reduced to schwa, not FACE diphthong.
	got, err := ph.Phonemize("a book")
	if err != nil {
		t.Fatalf("Phonemize: %v", err)
	}
	if got != "ə bˈʊk" {
		t.Errorf("Phonemize(\"a book\") = %q, want %q", got, "ə bˈʊk")
	}
}

func TestEdSuffix(t *testing.T) {
	ph := loadTestPhonemizer(t)

	tests := []struct {
		name    string
		input   string
		wantIPA string
	}{
		// t/d final → ᵻd (extra syllable)
		{"deleted", "deleted", "dəlˈitᵻd"},
		{"exceeded", "exceeded", "ɪksˈidᵻd"},
		// voiceless final → t
		{"indexed", "indexed", "ˈɪndˌɛkst"},
		{"stopped", "stopped", "stˈɑpt"},
		{"typed", "typed", "tˈIpt"},
		// voiced final → d
		{"leaned", "leaned", "lˈind"},
		{"carried", "carried", "kˈɛɹid"},
		// silent-e base preferred over raw stem (stare, not star)
		{"stared", "stared", "stˈɛɹd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ph.Phonemize(tt.input)
			if err != nil {
				t.Fatalf("Phonemize(%q): %v", tt.input, err)
			}
			if got != tt.wantIPA {
				t.Errorf("Phonemize(%q) = %q, want %q", tt.input, got, tt.wantIPA)
			}
		})
	}
}

func TestEdSuffixSafeWords(t *testing.T) {
	ph := loadTestPhonemizer(t)

	// Words like "bed", "red", "shed" are in the dictionary directly
	// and should NOT trigger -ed stripping.
	safeWords := []string{"bed", "red", "shed", "fed", "led", "wed"}
	for _, word := range safeWords {
		t.Run(word, func(t *testing.T) {
			got, err := ph.Phonemize(word)
			if err != nil {
				t.Fatalf("Phonemize(%q): %v", word, err)
			}
			if got == "" {
				t.Errorf("Phonemize(%q) returned empty", word)
			}
		})
	}
}

func TestPluralIesSuffix(t *testing.T) {
	ph := loadTestPhonemizer(t)

	// "memories" should resolve via memory + z (ies→y plural)
	got, err := ph.Phonemize("memories")
	if err != nil {
		t.Fatalf("Phonemize: %v", err)
	}
	if got == "" {
		t.Fatal("Phonemize(\"memories\") returned empty")
	}
	// Should end with /z/ (plural suffix)
	if !strings.HasSuffix(got, "z") {
		t.Errorf("Phonemize(\"memories\") = %q, expected to end with 'z'", got)
	}
}

func TestProperNounCustomDict(t *testing.T) {
	ph := loadTestPhonemizer(t)

	tests := []struct {
		name    string
		input   string
		wantIPA string
	}{
		{"Lina", "Lina", "lˈinə"},
		{"ProseForge", "ProseForge", "pɹˈOzfˌɔɹʤ"},
		{"Mihret", "Mihret", "mˌiɹˈɛt"},
		{"Ekua", "Ekua", "ɛkwˈɑ"},
		{"gelada", "gelada", "ɡəlˈɑdə"},
		{"geladas", "geladas", "ɡəlˈɑdəz"},
		{"Wulfric", "Wulfric", "wˈʊlfɹɪk"},
		{"Katherine", "Katherine", "kˈæθɹɪn"},
		{"Sutherland", "Sutherland", "sˈʌðəɹlənd"},
		{"Aldric", "Aldric", "ˈɔldɹɪk"},
		{"Scholastica", "Scholastica", "skəlˈæstɪkə"},
		{"Rambald", "Rambald", "ɹˈæmbˌɔld"},
		{"Byrne", "Byrne", "bˈɜɹn"},
		{"Pruitt", "Pruitt", "pɹˈuːɪt"},
		// Word-acronyms: normalizer bypasses letter-expansion, dict supplies sound.
		{"HVAC", "HVAC", "ˌAʧvˈæk"},
		{"NOC", "NOC", "nˈɑk"},
		{"Matthias", "Matthias", "məθˈIəs"},
		{"Okafor", "Okafor", "Okˈɑfɔɹ"},
		{"Chandrasekaran", "Chandrasekaran", "ʧˌʌndɹəsˈAkəɹən"},
		{"Devi", "Devi", "dˈɛvi"},
		// Smiley-Saves-the-Multiverse pronunciation scan: Kai/Enzo locked at their
		// (correct) LTS readings; Renata/Niagara/Reynolds corrected off LTS errors.
		{"Kai", "Kai", "kˈI"},
		{"Enzo", "Enzo", "ˈɛnzO"},
		{"Renata", "Renata", "ɹɛnˈɑtə"},
		{"Niagara", "Niagara", "nIˈæɡɹə"},
		{"Reynolds", "Reynolds", "ɹˈɛnəldz"},
		{"Tomas", "Tomas", "tˈɑmæs"}, // Tom-ass (Clayton); distinct from accented tomás (toh-MAHS)
		{"Gerald", "Gerald", "ʤˈɛɹəld"},
		{"Magnus", "Magnus", "mˈæɡnəs"},
		{"Navarro", "Navarro", "nəvˈɑɹO"},
		// Edith Navarro — LTS said ˈɛdɪθ ("ED-ith"); the name is "EE-dith". A LEXICAL
		// EXCEPTION, which is what banking is for: the letter pattern genuinely favours
		// ɛ and LTS was following it, not failing at it.
		//
		// ⚠️ An earlier version of this note cited gold's Eden ˈidᵊn as the anchor and
		// ethan ˈiθən as precedent. @wayland measured the distribution: Ed- across gold
		// is ɛ 68 vs i 12, so Eden is the MINORITY reading — the evidence ran 5:1 the
		// other way — and ethan is a custom_dict entry, i.e. a previous bank cited as
		// evidence for the next. Corrected in place; the entry stands, the reasoning
		// did not. 10× in book 2.
		{"Edith", "Edith", "ˈidɪθ"},
		{"Edith possessive", "Edith's", "ˈidɪθs"},
		// #54's only LIVE victim — Torres, the other one, was already banked, which is
		// why the class went unnoticed. Suffix-stripping found the English base `dolor`
		// and made the name a plural: dˈOləɹz, "DOH-lurz". The possessive was worse and
		// fell to LTS entirely: dOlˈɔɜɹɪzz.
		//
		// ⚠️ Gold's -ores distribution cannot arbitrate this one: 4 of its 6 members are
		// Azores/underscores/paradores/rebores, and at least paradores looks like the
		// same defect, so the population is contaminated by the thing being measured.
		// This rests on the name being duh-LOR-es, with Flores flˈɔɹəs as the one clean
		// structural parallel — stated rather than dressed up as a count (cf. Edith).
		//
		// Banking it does NOT hide the class: TestSurnameSuffixStripping_KnownBad pins
		// the mechanism on unbanked non-corpus specimens for exactly this reason.
		{"Dolores", "Dolores", "dəlˈɔɹəs"},
		{"Dolores possessive", "Dolores's", "dəlˈɔɹəsɪz"},
		// #55, ear-confirmed by @clayton: "sounds like ridge-ed-dale and should be
		// Ridj-Dale". LTS gave the silent e a vowel — ɹˈɪʤˈɛdˈAl, three syllables.
		//
		// Banked rather than fixed by rule, and that is the measured answer rather than
		// the lazy one: three detectors were built and all three are net-negative
		// (@wayland's two, and my compound-seam rule over the letter→phone alignment —
		// 959 fires, 14 improved, 61 worsened). Silence is not recoverable from spelling
		// OR from alignment: ridge|dale and Corne|lius have identical alignment shapes.
		// Exposure is 2 occurrences in 1 book; a rule would cost 61 words to save 14.
		{"Ridgedale", "Ridgedale", "ɹˈɪʤdˌAl"},
		{"Ridgedale possessive", "Ridgedale's", "ɹˈɪʤdˌAlz"},
		// #51's sole survivor. #53 restored the dropped vowel in 20 of 21 un- forms;
		// this one came back as the WRONG vowel — jˈunɪɡnɹəbəl, the "uni-" of unicorn,
		// plus -nor- with no nucleus at all (ɡnɹ).
		//
		// Banked rather than ruled: @wayland sized the un-|uni- boundary at 75 corpus
		// words with a vowel-initial stem, of which 72 are dictionary-resolved and
		// correct either way. A rule separating the two prefixes has 72 words to get
		// wrong and this one to fix.
		//
		// Target grounded in the distribution, not an example: gold realises the
		// negation prefix as ˌʌn 353 times, against 102 for the jun-/jˈun- family which
		// are all genuinely uni- words (unitarian, universal, unison). Stem from gold's
		// ignore ɪɡnˈɔɹ; -əbəl matches the ear-confirmed unjammable ənʤˈæməbəl.
		//
		// No possessive row: it is an adjective.
		{"unignorable", "unignorable", "ˌʌnɪɡnˈɔɹəbəl"},
		{"Chou", "Chou", "ʧˈW"},
		{"Alvarez", "Alvarez", "ˌælvəɹˈɛz"},
		{"Kedzie", "Kedzie", "kˈɛdzi"},
		{"Mara", "Mara", "mˈɑɹə"},
		{"Herrera", "Herrera", "ɛɹˈɛɹɑ"},
		{"Tomás", "Tomás", "tˌOmˈɑs"},
		{"Eastview", "Eastview", "ˈistvju"},
		{"Grafana", "Grafana", "ɡɹəfˈɑnə"},
		{"Okonkwo", "Okonkwo", "OkˈɑŋkwO"},
		{"PagerDuty", "PagerDuty", "pˈAʤɜɹdˈuti"},
		{"Enron", "Enron", "ˈɛnɹɑn"},
		{"overridden", "overridden", "ˌOvəɹˈɪdən"},
		// Smiley bible-batch canon (Book 3 keeper roster)
		{"Adwoa", "Adwoa", "əʤwˈɑ"},
		{"Kofi", "Kofi", "kˈOfi"},
		{"Kwame", "Kwame", "kwˈami"},
		{"Osei", "Osei", "OsˈA"},
		{"Bonsu", "Bonsu", "bˈOnsu"},
		{"Tigist", "Tigist", "tˌiɡˈist"},
		{"Mwangi", "Mwangi", "mwˈɑŋɡi"},
		{"Oliveira", "Oliveira", "ˌOlivˈAɹə"},
		{"Mensah", "Mensah", "mˈɛnsɑ"},
		{"Joon-ho", "Joon-ho", "ʤˈunhO"},
		// Forge of Forgotten Scrolls first-pass (Sten audit)
		{"Adelard", "Adelard", "ˈædəlɑɹd"},
		{"Brigid", "Brigid", "bɹˈɪʤɪd"},
		{"Kubernetes", "Kubernetes", "kˌubəɹnˈɛtiz"},
		{"Nightwriter", "Nightwriter", "nˈItɹˌItəɹ"},
		// Corbin frontline cast (Sten audit)
		{"Hanson", "Hanson", "hˈænsən"},
		{"Reyes", "Reyes", "ɹˈAɛs"},
		// ⬅ MOVED ˈvæskɛz→vˈæskɛz 2026-08-23 on @clayton's ear, and this test
		// firing is the reason the change is trustworthy rather than a tidy-up.
		//
		// The stress mark now precedes the VOWEL, matching gold and silver: all
		// 15 characters that ever follow a mark across their 182,772 entries are
		// vowels, with zero exceptions. Ours put it before the onset consonant
		// (standard IPA). Both notations are legible to a human; they are NOT
		// the same input to the model.
		//
		// ⚠️ The engine does not normalise mark position — measured with a
		// fixture plus control, same phonemes and only the mark moved, and the
		// output came back unchanged. So `ˈ` (token 156) lands at a different
		// INDEX and the model receives a different token sequence.
		//
		// ⛔ It is audible. A/B rendered locally (af_sarah 0.93, "Vasquez opened
		// the file."): the two clips differ by 1,024 bytes — ~21ms — so this
		// changes the word's DURATION, not just its bytes. @clayton picked the
		// vowel-adjacent form. "Obviously equivalent" was the hypothesis; the
		// render refused it.
		//
		// 📌 Four other custom entries carry the old shape (oliveira · pâté ·
		// résumé · soufflé) and 29 of 62 strings in homographs.json do too.
		// They are deliberately NOT changed here — see #63. One ear-confirmed
		// specimen licenses the CLASS as a question worth answering, not as an
		// answer. @wayland's asymmetry is why: banking a broken name has a floor
		// (it is wrong today, a fix holds or improves), while editing a working
		// one has none.
		{"Vasquez", "Vasquez", "vˈæskɛz"},
		{"Kouri", "Kouri", "kˈuɹi"},
		{"Novak", "Novak", "nˈOvæk"},
		// Corbin tier-3 round-2 (Sten audit)
		{"Torres", "Torres", "tˈɔɹɛs"},
		{"Ruiz", "Ruiz", "ɹuˈis"},
		{"Silas", "Silas", "sˈIləs"},
		// Buildings Speak STT audit (Sten). Explicit ɔɪ, NOT the Y diphthong
		// token: same IPA, but the model renders Y as "Mora" here while the
		// decomposed form renders correctly (whisper-verified both ways).
		{"Moira", "Moira", "mˈɔɪɹə"},
		{"Moira possessive", "Moira's", "mˈɔɪɹəz"},
		// #26 unit expansion: "GHz" normalizes to this word, which LTS renders
		// with a wrong full vowel (ɡˈɪɡɑhɜɹts) — banked with the schwa.
		{"gigahertz", "gigahertz", "ɡˈɪɡəhɜɹts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ph.Phonemize(tt.input)
			if err != nil {
				t.Fatalf("Phonemize(%q): %v", tt.input, err)
			}
			if got != tt.wantIPA {
				t.Errorf("Phonemize(%q) = %q, want %q", tt.input, got, tt.wantIPA)
			}
		})
	}
}

func TestProperNounHeuristic(t *testing.T) {
	ph := loadTestPhonemizer(t)

	// Names ending in 'a' not in any dictionary should get final schwa from heuristic
	// "Mara" is unlikely to be in gold/silver
	got, err := ph.Phonemize("Mara")
	if err != nil {
		t.Fatalf("Phonemize: %v", err)
	}
	if !strings.HasSuffix(got, "ə") {
		t.Errorf("Phonemize(\"Mara\") = %q, expected final schwa", got)
	}
}

func TestDiagnoseText_LTSFallback(t *testing.T) {
	ph := loadTestPhonemizer(t)

	// Use the standard excerpt (first 4 content paragraphs)
	// Prefer full story for comprehensive audit, fall back to target paragraphs
	data, err := os.ReadFile("testdata/full_story_text.txt")
	if err != nil {
		data, err = os.ReadFile("testdata/lts_audit_text.txt")
		if err != nil {
			t.Skipf("testdata not found: %v", err)
		}
	}
	text := string(data)

	results := ph.DiagnoseText(text)

	// Collect unique LTS and skip words
	seen := make(map[string]bool)
	var ltsWords, skipWords []WordDiagnosis
	for _, r := range results {
		key := strings.ToLower(r.Word)
		if seen[key] {
			continue
		}
		seen[key] = true
		switch r.Source {
		case "lts":
			ltsWords = append(ltsWords, r)
		case "skip":
			skipWords = append(skipWords, r)
		}
	}

	t.Logf("Total words: %d", len(results))
	t.Logf("LTS fallback: %d unique", len(ltsWords))
	for _, w := range ltsWords {
		t.Logf("  LTS:  %-20s → %s", w.Word, w.Phonemes)
	}
	t.Logf("Skipped: %d unique", len(skipWords))
	for _, w := range skipWords {
		t.Logf("  SKIP: %q", w.Word)
	}

	// Source distribution
	counts := make(map[string]int)
	for _, r := range results {
		counts[r.Source]++
	}
	for source, count := range counts {
		t.Logf("  %s: %d", source, count)
	}
}

func TestDiagnoseSegmentSizes(t *testing.T) {
	ph := loadTestPhonemizer(t)

	data, err := os.ReadFile("testdata/full_story_text.txt")
	if err != nil {
		t.Skipf("testdata not found: %v", err)
	}

	paragraphs, err := ph.TokenizeText(string(data), nil)
	if err != nil {
		t.Fatalf("TokenizeText: %v", err)
	}

	totalSegments := 0
	totalGroups := 0
	var segSizes []int
	multiSegGroups := 0

	for pi, groups := range paragraphs {
		for gi, group := range groups {
			totalGroups++
			if len(group.Segments) > 1 {
				multiSegGroups++
				t.Logf("Paragraph %d, Group %d: %d tokens split into %d segments",
					pi, gi, len(group.AllTokens), len(group.Segments))
				for si, seg := range group.Segments {
					t.Logf("  Segment %d: %d tokens", si, len(seg))
				}
			}
			for _, seg := range group.Segments {
				totalSegments++
				segSizes = append(segSizes, len(seg))
			}
		}
	}

	// Stats
	var sum int
	minSeg, maxSeg := segSizes[0], segSizes[0]
	for _, s := range segSizes {
		sum += s
		if s < minSeg {
			minSeg = s
		}
		if s > maxSeg {
			maxSeg = s
		}
	}

	t.Logf("Total paragraphs: %d", len(paragraphs))
	t.Logf("Total groups: %d (%d needed splitting)", totalGroups, multiSegGroups)
	t.Logf("Total segments: %d", totalSegments)
	t.Logf("Segment sizes: min=%d max=%d avg=%d", minSeg, maxSeg, sum/len(segSizes))
}

func TestSentenceGrouping(t *testing.T) {
	ph := loadTestPhonemizer(t)

	// Two short sentences should be grouped into one SentenceTokens
	paragraphs, err := ph.TokenizeText("Hello world. Goodbye world.", nil)
	if err != nil {
		t.Fatalf("TokenizeText: %v", err)
	}
	if len(paragraphs) != 1 {
		t.Fatalf("expected 1 paragraph, got %d", len(paragraphs))
	}

	// Short sentences combined into a single group
	if len(paragraphs[0]) != 1 {
		t.Fatalf("expected 1 group (combined), got %d", len(paragraphs[0]))
	}

	group := paragraphs[0][0]
	if len(group.AllTokens) == 0 {
		t.Error("AllTokens is empty")
	}
	if len(group.Segments) == 0 {
		t.Error("Segments is empty")
	}
}

func TestSentenceGroupingParagraphs(t *testing.T) {
	ph := loadTestPhonemizer(t)

	// Two paragraphs should produce two separate entries
	paragraphs, err := ph.TokenizeText("Hello world.\n\nGoodbye world.", nil)
	if err != nil {
		t.Fatalf("TokenizeText: %v", err)
	}
	if len(paragraphs) != 2 {
		t.Fatalf("expected 2 paragraphs, got %d", len(paragraphs))
	}
}
