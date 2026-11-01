package phonemizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"
)

// MisakiDict holds Misaki word→phoneme mappings (native Kokoro IPA).
type MisakiDict struct {
	entries map[string]string // lowercase word → Kokoro IPA phoneme string
}

// LoadMisakiDict loads a Misaki dictionary JSON file (us_gold.json or us_silver.json).
// The JSON format is a flat object where values are either:
//   - string: single pronunciation (e.g., "hello": "hɛlˈO")
//   - object: POS-tagged pronunciations (e.g., "read": {"VBD": "ɹɛd", "DEFAULT": "ɹˈiːd"})
//
// For POS-tagged entries, the "DEFAULT" key is used as the pronunciation.
// ErrEmptyDict reports a dictionary file that parsed cleanly but holds no entries.
//
// ⚠️ Distinguished from a parse failure ON PURPOSE. `{}` is well-formed JSON meaning
// "no entries", which is a legitimate state for the OPTIONAL custom dict — a
// deployment with no bank, and the fixture bank_redundancy_test writes to measure
// what the engine produces without one. Collapsing it into "unusable" made that
// guard fail, which is how this was caught.
//
// It is NOT legitimate for gold or silver, so those still refuse it.
var ErrEmptyDict = errors.New("dictionary has no entries")

func LoadMisakiDict(path string) (*MisakiDict, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read misaki dict: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse misaki dict: %w", err)
	}

	d := &MisakiDict{
		entries: make(map[string]string, len(raw)),
	}

	// ⚠️ Keys are stored in their ORIGINAL case, and lowercase is a fallback
	// written only if nothing claims it.
	//
	// This used to be `key := strings.ToLower(word)`, which collapsed 831 pairs
	// that gold deliberately differentiates -- CALM/calm, Polish/polish,
	// August/august, DRAM/dram, 1,673 entries in all. Both wrote the same key and
	// the winner was whichever `range raw` reached last, i.e. Go's randomised map
	// order, DECIDED FRESHLY IN EVERY PROCESS:
	//
	//	6 separate runs   calm = kˈɑlm x5, kˈɑm x1
	//	                  polish = pˈɑlɪʃ x3, pˈOlɪʃ x3
	//	                  dram = dɹˈæm x4, dˈiɹˌæm x2
	//
	// Narration renders section-by-section in separate processes, so the same
	// word could be pronounced differently in s5 and s10 of one book -- the exact
	// failure the Keeper's Door timestamp work was built to prevent, arriving
	// through the dictionary rather than the text.
	//
	// Found by @wayland's 831-pair census (#49 follow-up); the collapse was mine
	// to find and I had blamed my own measurement for it twice.
	for word, val := range raw {
		key := word

		// Try as string first
		var str string
		if err := json.Unmarshal(val, &str); err == nil {
			d.entries[key] = str
			continue
		}

		// Try as POS-tagged object
		var posMap map[string]*string
		if err := json.Unmarshal(val, &posMap); err == nil {
			if def, ok := posMap["DEFAULT"]; ok && def != nil {
				d.entries[key] = *def
			}
		}
	}

	// Lowercase fallbacks, added only where nothing already claims the key, so a
	// genuine lowercase entry always beats a folded uppercase one. Sorted so the
	// choice among several uppercase variants is stable rather than map-ordered.
	keys := make([]string, 0, len(d.entries))
	for k := range d.entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		lower := strings.ToLower(k)
		if _, taken := d.entries[lower]; !taken {
			d.entries[lower] = d.entries[k]
		}
	}

	if len(d.entries) == 0 {
		return nil, fmt.Errorf("%s: %w", path, ErrEmptyDict)
	}

	return d, nil
}

// Lookup returns the Kokoro IPA phoneme string for a word.
//
// Exact case first, then lowercase. Gold differentiates 831 pairs on case and
// the distinction is real -- August the month vs august the adjective, DRAM the
// memory vs dram the measure -- so folding the query threw that away before it
// could be used.
func (d *MisakiDict) Lookup(word string) (string, bool) {
	word = strings.TrimSpace(word)
	if ipa, ok := d.entries[word]; ok {
		return ipa, true
	}
	word = strings.ToLower(word)
	ipa, ok := d.entries[word]
	return ipa, ok
}

// LookupWithSuffix handles inflected forms: possessives, plurals, and past tense -ed.
func (d *MisakiDict) LookupWithSuffix(word string) (string, bool) {
	ipa, _, ok := d.LookupWithSuffixRule(word)
	return ipa, ok
}

// LookupWithSuffixRule is LookupWithSuffix plus which rule produced the answer:
// "" for a direct hit, otherwise the suffix stripped ("'s", "'ll", "-ies", "-s",
// "-ing", "-es", "-ed").
//
// The distinction is not cosmetic. A direct hit is what the dictionary says; anything
// else is a form this code BUILT, and the two can disagree — #56 is exactly that, a
// built plural using ɪz where every listed one uses ᵻz. @wayland came one post from
// refuting #56 because the diagnose source label says "gold" for both halves of the
// pair: it names where the BASE came from, which is a true statement about provenance
// and not the membership claim it reads as.
func (d *MisakiDict) LookupWithSuffixRule(word string) (string, string, bool) {
	word = strings.ToLower(strings.TrimSpace(word))

	// Try direct lookup first
	if ipa, ok := d.entries[word]; ok {
		return ipa, "", true
	}

	// Try stripping possessive 's
	if strings.HasSuffix(word, "'s") {
		base := word[:len(word)-2]
		if ipa, ok := d.entries[base]; ok {
			return ipa + possessiveSuffix(ipa), "'s", true
		}
	}

	// Try stripping the 'll contraction (#99). Gold banks only the pronouns
	// (he'll, it'll); every other stem sent the whole token to LTS, which read
	// Mary'll as "Mary ill" and devoiced There'll.
	if strings.HasSuffix(word, "'ll") {
		// The stem may itself be inflected (slates'll), so resolve it with the
		// suffix rules rather than a bare entry lookup.
		if ipa, _, ok := d.LookupWithSuffixRule(word[:len(word)-3]); ok {
			return ipa + willSuffix(ipa), "'ll", true
		}
	}

	// Try stripping plural -ies → -y (memories→memory, stories→story)
	if strings.HasSuffix(word, "ies") && len(word) > 4 {
		base := word[:len(word)-3] + "y"
		if ipa, ok := d.entries[base]; ok {
			return ipa + "z", "-ies", true
		}
	}

	// Try stripping plural/possessive s
	if strings.HasSuffix(word, "s") && len(word) > 1 {
		base := word[:len(word)-1]
		if ipa, ok := d.entries[base]; ok {
			return ipa + possessiveSuffix(ipa), "-s", true
		}
	}

	// Try stripping -ing (carrying→carry, narrowing→narrow, copying→copy)
	if strings.HasSuffix(word, "ing") && len(word) > 4 {
		for _, base := range ingBases(word) {
			if ipa, ok := d.entries[base]; ok {
				return ipa + "ɪŋ", "-ing", true
			}
		}
	}

	// Try stripping -es (processes→process, workbenches→workbench)
	if strings.HasSuffix(word, "es") && len(word) > 3 {
		base := word[:len(word)-2]
		if ipa, ok := d.entries[base]; ok {
			return ipa + possessiveSuffix(ipa), "-es", true
		}
	}

	// Try stripping past tense -ed
	if strings.HasSuffix(word, "ed") && len(word) > 3 {
		for _, base := range edBases(word) {
			if ipa, ok := d.entries[base]; ok {
				return ipa + edSuffix(ipa), "-ed", true
			}
		}
	}

	return "", "", false
}

// Size returns the number of entries in the dictionary.
func (d *MisakiDict) Size() int {
	return len(d.entries)
}

// possessiveSuffix returns the appropriate suffix phoneme for possessives.
func possessiveSuffix(ipa string) string {
	if len(ipa) == 0 {
		return "z"
	}
	// After sibilants (s, z, ʃ, ʒ, ʧ, ʤ) → /ɪz/
	lastRune := lastChar(ipa)
	switch lastRune {
	case 's', 'z', 'ʃ', 'ʒ', 'ʧ', 'ʤ':
		return "ɪz"
	}
	// After voiceless consonants (p, t, k, f, θ) → /s/
	switch lastRune {
	case 'p', 't', 'k', 'f', 'θ':
		return "s"
	}
	// After everything else → /z/
	return "z"
}

// willSuffix returns the contracted "will" for a base IPA, following gold's
// own 'll entries: l after a vowel (he'll hil, they'll ðAl), ᵊl after a
// consonant (it'll ˈɪɾᵊl).
func willSuffix(ipa string) string {
	if strings.ContainsRune("aeiouɑæɛɪʊʌɔəᵊɜɐAIOWY", lastChar(ipa)) {
		return "l"
	}
	return "ᵊl"
}

// edSuffix returns the phonetically correct past tense suffix for a base IPA.
func edSuffix(ipa string) string {
	last := lastChar(ipa)
	switch last {
	case 't', 'd':
		return "ᵻd"
	case 'p', 'k', 'f', 'θ', 's', 'ʃ', 'ʧ':
		return "t"
	default:
		return "d"
	}
}

// edBases returns candidate base forms for a word ending in -ed.
func edBases(word string) []string {
	stem := word[:len(word)-2] // strip "ed"
	bases := []string{
		stem + "e", // stare→stared, delete→deleted, type→typed (silent-e preferred)
		stem,       // exceed→exceeded, index→indexed, lean→leaned
	}
	// Undouble: stopped→stop, hummed→hum
	if len(stem) >= 2 && stem[len(stem)-1] == stem[len(stem)-2] {
		bases = append(bases, stem[:len(stem)-1])
	}
	// -ied→-y: carried→carry, worried→worry
	if strings.HasSuffix(word, "ied") && len(word) > 4 {
		bases = append(bases, word[:len(word)-3]+"y")
	}
	return bases
}

// ingBases returns candidate base forms for a word ending in -ing.
func ingBases(word string) []string {
	stem := word[:len(word)-3] // strip "ing"
	bases := []string{
		stem + "e", // typing→type, narrowing→narrowe (won't match, but safe)
		stem,       // narrow→narrowing, interrupt→interrupting
	}
	// Undouble: carrying→carry (y was changed to i), copying→copy — needs no
	// branch here, because "carry" is already in bases as `stem`. This was an
	// empty `if strings.HasSuffix(stem, "y") {}` carrying the same note inside,
	// which staticcheck reads as a discarded HasSuffix (SA4017) and a reader
	// reads as a missing check. It was neither.
	// -ying → -y: carrying → carr + ying → carry
	if strings.HasSuffix(word, "ying") && len(word) > 5 {
		bases = append(bases, word[:len(word)-4]+"y")
	}
	// Undouble consonant: stopping→stop, humming→hum
	if len(stem) >= 2 && stem[len(stem)-1] == stem[len(stem)-2] {
		bases = append(bases, stem[:len(stem)-1])
	}
	return bases
}

func lastChar(s string) rune {
	for i := len(s) - 1; i >= 0; i-- {
		r := rune(s[i])
		if r >= 128 {
			// Multi-byte: decode properly
			runes := []rune(s)
			if len(runes) > 0 {
				return runes[len(runes)-1]
			}
		}
		if unicode.IsLetter(r) || unicode.IsSymbol(r) {
			return r
		}
	}
	return 0
}
