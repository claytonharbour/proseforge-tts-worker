package phonemizer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// LTS implements the Flite letter-to-sound engine.
// It converts English words to ARPABET phonemes using decision trees,
// then converts ARPABET to Kokoro IPA.
type LTS struct {
	arpToIPA map[string]string // ARPABET phoneme → Kokoro IPA
}

// NewLTS creates a new LTS engine with the given ARPABET→IPA mapping file.
func NewLTS(arpabetMapPath string) (*LTS, error) {
	arpToIPA, err := loadARPABETMap(arpabetMapPath)
	if err != nil {
		return nil, fmt.Errorf("load ARPABET map for LTS: %w", err)
	}
	return &LTS{arpToIPA: arpToIPA}, nil
}

// contextWindowSize is the number of characters on each side of the
// current letter in the feature buffer. The Flite CMU LTS rules use
// a window of 4 left + 4 right = 8 context positions (feat indices 0-7,
// though only 1-6 are used by the trained model).
const contextWindowSize = 4

// Phonemize converts a word to Kokoro IPA using the LTS decision trees.
// Returns empty string if the word contains no letters.
func (l *LTS) Phonemize(word string) string {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return ""
	}

	// Build padded buffer: "000#word#000"
	// Padding uses '0' chars, boundaries use '#'
	var buf strings.Builder
	for i := 0; i < contextWindowSize-1; i++ {
		buf.WriteByte('0')
	}
	buf.WriteByte('#')
	buf.WriteString(word)
	buf.WriteByte('#')
	for i := 0; i < contextWindowSize-1; i++ {
		buf.WriteByte('0')
	}
	full := buf.String()

	// Process each letter (iterate forward, collect phones in order)
	var phones []string
	wordStart := contextWindowSize // first letter of word in full buffer
	wordEnd := wordStart + len(word) - 1

	for pos := wordStart; pos <= wordEnd; pos++ {
		ch := full[pos]
		if ch < 'a' || ch > 'z' {
			continue // skip non-letter characters
		}

		// Build feature vector: 4 left + 4 right context chars
		var fval [2 * contextWindowSize]byte
		for i := 0; i < contextWindowSize; i++ {
			fval[i] = full[pos-contextWindowSize+i]
		}
		for i := 0; i < contextWindowSize; i++ {
			fval[contextWindowSize+i] = full[pos+1+i]
		}

		// Look up the decision tree for this letter
		letterIdx := int(ch - 'a')
		startNode := ltsLetterIndex[letterIdx]

		phone := applyModel(fval[:], startNode)
		phoneName := ltsPhoneTable[phone]

		if phoneName == "epsilon" {
			continue
		}

		// Split compound phones (e.g., "t-s" → "t", "s")
		if idx := strings.IndexByte(phoneName, '-'); idx >= 0 {
			phones = append(phones, phoneName[:idx], phoneName[idx+1:])
		} else {
			phones = append(phones, phoneName)
		}
	}

	if len(phones) == 0 {
		return ""
	}

	// Convert ARPABET phones to Kokoro IPA
	return l.arpabetToIPA(phones)
}

// applyModel walks the decision tree starting at the given node.
// Returns the phone table index.
func applyModel(fval []byte, start uint16) uint8 {
	node := ltsRules[start]
	for node.feat != 255 {
		if fval[node.feat] == node.val {
			node = ltsRules[node.yes]
		} else {
			node = ltsRules[node.no]
		}
	}
	return node.val
}

// arpabetToIPA converts a sequence of ARPABET phones (with stress) to Kokoro IPA.
// Phone format: "phoneme" + optional stress digit, e.g., "eh1", "ax0", "b".
func (l *LTS) arpabetToIPA(phones []string) string {
	var result strings.Builder
	for _, p := range phones {
		base, stress := splitARPABETStress(p)

		// Special case: AH with 0 stress → schwa
		if base == "AH" && stress == 0 {
			base = "AH0"
		}

		ipa, ok := l.arpToIPA[base]
		if !ok {
			continue
		}

		// Add stress marker before the vowel
		if stress == 1 {
			result.WriteString("ˈ")
		} else if stress == 2 {
			result.WriteString("ˌ")
		}

		result.WriteString(ipa)
	}
	return result.String()
}

// splitARPABETStress separates the stress digit from an ARPABET phone string.
// Flite phones use lowercase with trailing digit: "eh1", "ax0", "b".
// Returns the uppercase base phoneme and stress level (0, 1, 2, or -1 for consonants).
func splitARPABETStress(phone string) (string, int) {
	if len(phone) == 0 {
		return phone, -1
	}
	last := phone[len(phone)-1]
	if last >= '0' && last <= '2' {
		return strings.ToUpper(phone[:len(phone)-1]), int(last - '0')
	}
	return strings.ToUpper(phone), -1
}

// loadARPABETMap loads the ARPABET→IPA mapping from a JSON file.
func loadARPABETMap(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Combined map[string]string `json:"combined"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ARPABET map: %w", err)
	}

	if len(raw.Combined) == 0 {
		return nil, fmt.Errorf("ARPABET map 'combined' field is empty")
	}

	return raw.Combined, nil
}
