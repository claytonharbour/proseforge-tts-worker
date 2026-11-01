package phonemizer

import (
	"fmt"
	"sort"
	"strings"
)

// LossyRunes returns the runes of s that the vocabulary cannot represent, in
// order of first appearance, deduplicated.
//
// ⚑ THIS IS THE ONLY WAY TO ASK THE QUESTION. Tokenize drops an unknown rune and
// returns, with no UNK id, no error and no count mismatch — so its result for a
// lossy string is indistinguishable from a clean tokenization of a shorter one
// (#84, @Sten):
//
//	Tokenize("QZ§")  ->  1 token, no error.  Two of three characters vanished.
//
// The natural audit — tokenize every dictionary string and look for UNK — is
// therefore unfalsifiable: it returns clean for every possible input, including
// garbage. @Sten's first pass reported "533 strings, 0 UNK" and it meant nothing.
func (t *Tokenizer) LossyRunes(s string) []rune {
	var out []rune
	var seen map[rune]bool
	for _, r := range s {
		if _, ok := t.vocab[r]; ok {
			continue
		}
		if seen == nil {
			seen = make(map[rune]bool)
		}
		if seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// vocabViolation is one dictionary entry carrying a rune the model cannot speak.
type vocabViolation struct {
	word    string
	reading string
	runes   []rune
}

// describeViolations renders at most `limit` violations, naming the word, the
// reading and each offending rune with its codepoint.
//
// ⚠️ The codepoint is not decoration. The likeliest defect here is INVISIBLE in a
// diff: ASCII "g" (U+0067) is the ONE letter of the alphabet absent from the
// vocabulary, while IPA uses "ɡ" (U+0261, LATIN SMALL LETTER SCRIPT G). The two
// glyphs are near-identical in most fonts, so a message that printed only the
// character would show you `g` and leave you none the wiser.
//
// Measured over the shipped data: 25 of 26 ASCII letters ARE in the vocabulary.
// `g` is the sole exception, which is exactly why it is the one that will be typed.
func describeViolations(source string, vs []vocabViolation, limit int) string {
	sort.Slice(vs, func(i, j int) bool { return vs[i].word < vs[j].word })

	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d entry/entries contain runes absent from the token vocabulary, "+
		"which Tokenize would DROP SILENTLY", source, len(vs))
	for i, v := range vs {
		if i == limit {
			fmt.Fprintf(&b, "\n  ... and %d more", len(vs)-limit)
			break
		}
		fmt.Fprintf(&b, "\n  %q = %q dropped:", v.word, v.reading)
		for _, r := range v.runes {
			fmt.Fprintf(&b, " %q (U+%04X)", r, r)
		}
	}
	return b.String()
}

// validateReadings collects every entry whose reading carries an unspeakable rune.
func validateReadings(t *Tokenizer, source string, readings func(yield func(word, reading string))) error {
	var vs []vocabViolation
	readings(func(word, reading string) {
		if bad := t.LossyRunes(reading); len(bad) > 0 {
			vs = append(vs, vocabViolation{word: word, reading: reading, runes: bad})
		}
	})
	if len(vs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", describeViolations(source, vs, 10))
}

// ValidateAgainstVocab reports entries the model cannot pronounce as written.
//
// It checks the readings the resolver will actually SERVE — for a POS-tagged gold
// entry that is the DEFAULT, because that is the only one LoadMisakiDict keeps.
// Validating a reading nothing can reach would manufacture failures.
func (d *MisakiDict) ValidateAgainstVocab(t *Tokenizer, source string) error {
	if d == nil {
		return nil
	}
	return validateReadings(t, source, func(yield func(word, reading string)) {
		for word, reading := range d.entries {
			yield(word, reading)
		}
	})
}

// ValidateAgainstVocab reports homograph readings the model cannot pronounce.
//
// ⚠️ IPA lives in THREE places in this structure, and the third is a map KEY:
//
//	Default / Imperative / InflectedDefault   plain string fields
//	Pronunciations                            map[IPA]context — the KEY is the reading
//
// A validator that walked only the struct's string fields would cover 22 of the 62
// readings in homographs.json and report the other 40 as checked. I wrote exactly
// that on the first pass; it was caught only because #63 and @Sten had each
// independently published the number 62 and my instrument said 22.
func (h *HomographResolver) ValidateAgainstVocab(t *Tokenizer, source string) error {
	if h == nil {
		return nil
	}
	return validateReadings(t, source, func(yield func(word, reading string)) {
		for word, e := range h.entries {
			for _, reading := range []string{e.Default, e.Imperative, e.InflectedDefault} {
				if reading != "" {
					yield(word, reading)
				}
			}
			for reading := range e.Pronunciations {
				yield(word, reading)
			}
		}
	})
}
