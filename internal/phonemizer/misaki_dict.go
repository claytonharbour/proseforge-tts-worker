package phonemizer

import (
	"encoding/json"
	"fmt"
	"os"
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

	for word, val := range raw {
		key := strings.ToLower(word)

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

	if len(d.entries) == 0 {
		return nil, fmt.Errorf("misaki dict is empty")
	}

	return d, nil
}

// Lookup returns the Kokoro IPA phoneme string for a word.
func (d *MisakiDict) Lookup(word string) (string, bool) {
	word = strings.ToLower(strings.TrimSpace(word))
	ipa, ok := d.entries[word]
	return ipa, ok
}

// LookupWithSuffix handles inflected forms: possessives, plurals, and past tense -ed.
func (d *MisakiDict) LookupWithSuffix(word string) (string, bool) {
	word = strings.ToLower(strings.TrimSpace(word))

	// Try direct lookup first
	if ipa, ok := d.entries[word]; ok {
		return ipa, true
	}

	// Try stripping possessive 's
	if strings.HasSuffix(word, "'s") {
		base := word[:len(word)-2]
		if ipa, ok := d.entries[base]; ok {
			return ipa + possessiveSuffix(ipa), true
		}
	}

	// Try stripping plural -ies → -y (memories→memory, stories→story)
	if strings.HasSuffix(word, "ies") && len(word) > 4 {
		base := word[:len(word)-3] + "y"
		if ipa, ok := d.entries[base]; ok {
			return ipa + "z", true
		}
	}

	// Try stripping plural/possessive s
	if strings.HasSuffix(word, "s") && len(word) > 1 {
		base := word[:len(word)-1]
		if ipa, ok := d.entries[base]; ok {
			return ipa + possessiveSuffix(ipa), true
		}
	}

	// Try stripping -ing (carrying→carry, narrowing→narrow, copying→copy)
	if strings.HasSuffix(word, "ing") && len(word) > 4 {
		for _, base := range ingBases(word) {
			if ipa, ok := d.entries[base]; ok {
				return ipa + "ɪŋ", true
			}
		}
	}

	// Try stripping -es (processes→process, workbenches→workbench)
	if strings.HasSuffix(word, "es") && len(word) > 3 {
		base := word[:len(word)-2]
		if ipa, ok := d.entries[base]; ok {
			return ipa + possessiveSuffix(ipa), true
		}
	}

	// Try stripping past tense -ed
	if strings.HasSuffix(word, "ed") && len(word) > 3 {
		for _, base := range edBases(word) {
			if ipa, ok := d.entries[base]; ok {
				return ipa + edSuffix(ipa), true
			}
		}
	}

	return "", false
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
		stem + "e",    // stare→stared, delete→deleted, type→typed (silent-e preferred)
		stem,          // exceed→exceeded, index→indexed, lean→leaned
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
		stem + "e",   // typing→type, narrowing→narrowe (won't match, but safe)
		stem,         // narrow→narrowing, interrupt→interrupting
	}
	// Undouble: carrying→carry (y was changed to i), copying→copy
	if strings.HasSuffix(stem, "y") {
		// carrying stem is "carry" — already handled by stem
	}
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
