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
// asciiFold maps non-ASCII Latin letters to the closest a-z spelling LTS can
// read. The decision tree is trained on a-z only, and Phonemize below iterates
// BYTES, skipping anything outside that range — so a two-byte å was skipped
// twice and vanished. That is issue #25:
//
//	Þórsson → ɹsn      Vågå → vɡ      Müller → mlɜɹ      Møller → mlɜɹ
//
// The last pair is the clearest statement of the bug: two different names
// producing one wrong output.
//
// ⚠️ This makes a name SPEAKABLE, not correct. Vågå folds to "vaga" ≈ "VAH-gah"
// where Norwegian is nearer "VAW-gaw". Getting a specific name right is still a
// canon call and a custom_dict entry — which wins anyway, because folding
// happens here in LTS, after every dictionary layer has already missed.
// Silence is never what an author intended, so an approximation strictly beats
// dropping the letter.
//
// Multi-letter expansions are conventional transliterations, written out rather
// than hidden inside a normalisation library so they can be argued with.
var asciiFold = map[rune]string{
	'á': "a", 'à': "a", 'â': "a", 'ä': "a", 'ã': "a", 'å': "a", 'ā': "a", 'ă': "a", 'ą': "a",
	'é': "e", 'è': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ĕ': "e", 'ė': "e", 'ę': "e", 'ě': "e",
	'í': "i", 'ì': "i", 'î': "i", 'ï': "i", 'ī': "i", 'ĭ': "i", 'į': "i", 'ı': "i",
	'ó': "o", 'ò': "o", 'ô': "o", 'ö': "o", 'õ': "o", 'ō': "o", 'ŏ': "o", 'ő': "o", 'ø': "o",
	'ú': "u", 'ù': "u", 'û': "u", 'ü': "u", 'ū': "u", 'ŭ': "u", 'ů': "u", 'ű': "u", 'ų': "u",
	'ý': "y", 'ÿ': "y",
	'ñ': "n", 'ń': "n", 'ň': "n", 'ņ': "n",
	'ç': "c", 'ć': "c", 'č': "c", 'ĉ': "c", 'ċ': "c",
	'š': "s", 'ś': "s", 'ş': "s", 'ŝ': "s",
	'ž': "z", 'ź': "z", 'ż': "z",
	'ł': "l", 'ď': "d", 'đ': "d", 'ř': "r", 'ŕ': "r", 'ť': "t", 'ţ': "t",
	'ğ': "g", 'ĝ': "g", 'ġ': "g", 'ģ': "g", 'ĥ': "h", 'ħ': "h", 'ĵ': "j", 'ķ': "k", 'ŵ': "w",
	// Non-decomposable letters — these are transliteration choices, not folds.
	'þ': "th", 'ð': "d", 'æ': "ae", 'œ': "oe", 'ß': "ss",
}

// foldToASCII rewrites non-ASCII Latin letters to their closest a-z spelling.
// Runes with no mapping are left alone; Phonemize skips them as before, so an
// unmapped script degrades exactly as it does today rather than worse.
func foldToASCII(word string) string {
	if isASCIILower(word) {
		return word
	}
	var b strings.Builder
	b.Grow(len(word))
	for _, r := range word {
		if s, ok := asciiFold[r]; ok {
			b.WriteString(s)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isASCIILower(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func (l *LTS) Phonemize(word string) string {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return ""
	}
	// Fold before the byte loop below, which cannot see multi-byte letters.
	word = foldToASCII(word)

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
		switch stress {
		case 1:
			result.WriteString("ˈ")
		case 2:
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
