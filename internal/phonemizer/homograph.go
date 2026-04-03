package phonemizer

import (
	"encoding/json"
	"os"
	"strings"
)

// HomographEntry represents pronunciation rules for a homograph word.
type HomographEntry struct {
	Default        string                         `json:"default"`
	Pronunciations map[string]HomographContext     `json:"pronunciations"`
}

// HomographContext contains context trigger words for a specific pronunciation.
type HomographContext struct {
	ContextBefore []string `json:"context_before"`
	ContextAfter  []string `json:"context_after"`
}

// HomographResolver disambiguates homograph pronunciations using context.
type HomographResolver struct {
	entries map[string]HomographEntry
}

// LoadHomographs loads homograph rules from a JSON file.
func LoadHomographs(path string) (*HomographResolver, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse the JSON, ignoring underscore-prefixed metadata fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	entries := make(map[string]HomographEntry)
	for key, val := range raw {
		if strings.HasPrefix(key, "_") {
			continue
		}
		var entry HomographEntry
		if err := json.Unmarshal(val, &entry); err != nil {
			return nil, err
		}
		entries[key] = entry
	}

	return &HomographResolver{entries: entries}, nil
}

// Resolve returns the pronunciation for a word given its surrounding context.
// words is the full word list, idx is the position of the word to resolve.
// Returns the IPA pronunciation and true if the word is a known homograph,
// or empty string and false otherwise.
func (h *HomographResolver) Resolve(words []string, idx int) (string, bool) {
	if idx < 0 || idx >= len(words) {
		return "", false
	}

	word := strings.ToLower(stripWordPunctuation(words[idx]))
	entry, ok := h.entries[word]
	if !ok {
		return "", false
	}

	// Check context window (±3 words) for trigger words.
	// context_before triggers are checked before the target word,
	// context_after triggers are checked after the target word.
	// Additionally, all triggers are checked in both directions as
	// a fallback (e.g., "yesterday" can appear after "read" in
	// "I read the book yesterday").
	windowSize := 3

	// Find the best matching pronunciation by closest trigger word.
	// When multiple pronunciations match, the one with the nearest
	// context trigger wins (avoids non-determinism from map iteration).
	bestPron := ""
	bestDist := windowSize + 1

	// First pass: strict directional matching (higher confidence)
	for pron, ctx := range entry.Pronunciations {
		for _, trigger := range ctx.ContextBefore {
			for i := max(0, idx-windowSize); i < idx; i++ {
				if strings.ToLower(stripWordPunctuation(words[i])) == trigger {
					if dist := idx - i; dist < bestDist {
						bestDist = dist
						bestPron = pron
					}
				}
			}
		}
		for _, trigger := range ctx.ContextAfter {
			for i := idx + 1; i <= min(len(words)-1, idx+windowSize); i++ {
				if strings.ToLower(stripWordPunctuation(words[i])) == trigger {
					if dist := i - idx; dist < bestDist {
						bestDist = dist
						bestPron = pron
					}
				}
			}
		}
	}

	if bestPron != "" {
		return bestPron, true
	}

	// Second pass: check all triggers in both directions (broader matching)
	for pron, ctx := range entry.Pronunciations {
		allTriggers := append(ctx.ContextBefore, ctx.ContextAfter...)
		for _, trigger := range allTriggers {
			for i := max(0, idx-windowSize); i <= min(len(words)-1, idx+windowSize); i++ {
				if i == idx {
					continue
				}
				dist := idx - i
				if dist < 0 {
					dist = -dist
				}
				if strings.ToLower(stripWordPunctuation(words[i])) == trigger {
					if dist < bestDist {
						bestDist = dist
						bestPron = pron
					}
				}
			}
		}
	}

	if bestPron != "" {
		return bestPron, true
	}

	// No context match — use default pronunciation
	return entry.Default, true
}

// stripWordPunctuation removes leading and trailing punctuation from a word.
func stripWordPunctuation(word string) string {
	return strings.TrimFunc(word, func(r rune) bool {
		switch r {
		case '.', ',', '!', '?', ';', ':', '"', '(', ')', '\'', '-',
			'\u201c', '\u201d', '\u2014', '\u2026':
			return true
		}
		return false
	})
}

// IsHomograph returns true if the word is a known homograph.
func (h *HomographResolver) IsHomograph(word string) bool {
	_, ok := h.entries[strings.ToLower(word)]
	return ok
}

// Count returns the number of homograph entries.
func (h *HomographResolver) Count() int {
	return len(h.entries)
}
