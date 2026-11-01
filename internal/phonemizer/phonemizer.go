package phonemizer

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"unicode"
)

// Phonemes written in Go rather than read from a data file are invisible to
// every scan we have. renderable_ipa_test.go walks the five data sources and
// proves each symbol has a token; it cannot open this file, so a homoglyph typed
// HERE ships silently — the tokenizer drops an unknown rune with no error.
//
// @wayland demonstrated exactly that: he planted the turned-e homoglyph of ə (U+01DD vs U+0259)
// into the two LTS-schwa literals on a copy of the tree and the ENTIRE unit
// suite stayed green. `ə` was covered only because it happens to be a value in
// arpabet_to_ipa.json — coincidentally, not causally.
//
// Naming them gives that test one place to assert, so the coverage becomes
// causal: change a constant and the test fails.
const (
	// phonemeSchwa is the reduced article "a", and the vowel appended by the
	// capitalized-final-a LTS heuristic (Lina → lˈinə).
	phonemeSchwa = "ə"
	// phonemeThe is the reduced "the". Gold has /ði/, which overpronounces it
	// before a consonant.
	phonemeThe = "ðə"
	// phonemeJoin separates the parts of a hyphenated word — "foot-path" becomes
	// fˈʊt + this + pˈæθ. It is token 16, so it renders as a word boundary rather
	// than being dropped.
	//
	// ⚑ It is here because @wayland found it after the other two were named, and
	// found it in the gap the naming was supposed to close. Both of us had reasoned
	// past it from opposite directions: I wrote that "an enumeration of call sites
	// goes stale the moment someone adds a seventh" while the seventh already
	// existed, and he had argued the hyphenated path was safe as a "pure
	// concatenation, union of already-swept sources".
	//
	// A JOIN IS NOT A CONCATENATION. It introduces a rune that neither part
	// contains, and 1,147 distinct words in the corpus carry it. He planted NBSP
	// U+00A0 at both sites and the entire unit suite stayed green.
	phonemeJoin = " "
)

// Phonemizer is the main G2P (grapheme-to-phoneme) pipeline.
type Phonemizer struct {
	custom     *MisakiDict // optional author-provided pronunciations
	gold       *MisakiDict
	silver     *MisakiDict
	homographs *HomographResolver
	lts        *LTS
	tokenizer  *Tokenizer
	emphasis   bool // inject Gemini-style emphasis pauses (see emphasis.go)
}

// Config holds paths to data files needed by the phonemizer.
type Config struct {
	GoldDictPath   string
	SilverDictPath string
	HomographsPath string
	TokenMapPath   string
	ARPABETMapPath string // needed by LTS for ARPABET→IPA conversion
	CustomDictPath string // optional — custom pronunciations for proper nouns etc.
	EmphasisPauses bool   // optional — inject Gemini-style emphasis pauses (see emphasis.go)
}

// New creates a new Phonemizer with all components loaded.
func New(cfg Config) (*Phonemizer, error) {
	gold, err := LoadMisakiDict(cfg.GoldDictPath)
	if err != nil {
		return nil, err
	}

	silver, err := LoadMisakiDict(cfg.SilverDictPath)
	if err != nil {
		return nil, err
	}

	homographs, err := LoadHomographs(cfg.HomographsPath)
	if err != nil {
		return nil, err
	}

	lts, err := NewLTS(cfg.ARPABETMapPath)
	if err != nil {
		return nil, err
	}

	tokenizer, err := LoadTokenizer(cfg.TokenMapPath)
	if err != nil {
		return nil, err
	}

	// Custom dict is optional — nil if the path is empty or the file is absent.
	//
	// ⛔ ABSENT and CORRUPT are not the same thing, and this used to discard both.
	// LoadMisakiDict returns an error for a missing file AND for malformed JSON, so
	// `custom, _ = ...` meant one stray comma in custom_dict.json silently emptied
	// the whole bank: all 100+ hand-banked readings — every name @clayton has given
	// an ear to — quietly reverting to LTS with nothing logged and nothing red.
	// A dropped bank sounds like a worse voice, not like a broken file.
	var custom *MisakiDict
	if cfg.CustomDictPath != "" {
		c, err := LoadMisakiDict(cfg.CustomDictPath)
		switch {
		case err == nil:
			custom = c
		case errors.Is(err, fs.ErrNotExist), errors.Is(err, ErrEmptyDict):
			// Genuinely optional: no file (or an empty one) means no bank, no complaint.
			// Both are STATES; only a parse failure is a defect.
		default:
			return nil, fmt.Errorf("custom dict %s is present but unusable: %w",
				cfg.CustomDictPath, err)
		}
	}

	// Every reading the resolver can serve must be expressible in the model's token
	// vocabulary. Tokenize drops an unknown rune and says nothing (#84), so this is
	// the last point at which a bad entry can still be reported rather than heard.
	//
	// 📌 At the LOADER by design. A caller-side helper only protects whoever
	// remembers it exists; the loader sees every entry by construction — @Sten's
	// argument in #84, and the reason this is not merely a test.
	for _, v := range []struct {
		name string
		err  error
	}{
		{"gold", gold.ValidateAgainstVocab(tokenizer, "gold")},
		{"silver", silver.ValidateAgainstVocab(tokenizer, "silver")},
		{"homographs", homographs.ValidateAgainstVocab(tokenizer, "homographs")},
		{"custom", custom.ValidateAgainstVocab(tokenizer, "custom_dict")},
	} {
		if v.err != nil {
			return nil, v.err
		}
	}

	return &Phonemizer{
		custom:     custom,
		gold:       gold,
		silver:     silver,
		homographs: homographs,
		lts:        lts,
		tokenizer:  tokenizer,
		emphasis:   cfg.EmphasisPauses,
	}, nil
}

// Phonemize converts text to a Kokoro IPA phoneme string.
func (p *Phonemizer) Phonemize(text string) (string, error) {
	return p.phonemizeHinted(text, ResolveHint{})
}

// phonemizeHinted is Phonemize with what the caller knows from outside the sentence (#112).
func (p *Phonemizer) phonemizeHinted(text string, hint ResolveHint) (string, error) {
	if text == "" {
		return "", nil
	}

	// Step 1: Normalize text
	normalized := NormalizeTextWithWords(text, p.isEnglishWord)

	// Step 2: Split into words and phonemize each
	words := strings.Fields(normalized)
	var result strings.Builder

	for i, word := range words {
		// Strip surrounding punctuation
		prefix, core, suffix := splitPunctuation(word)

		if prefix != "" {
			result.WriteString(prefix)
		}

		if core != "" {
			phonemes := p.phonemizeWord(words, i, core, hint)
			result.WriteString(phonemes)
		}

		if suffix != "" {
			result.WriteString(suffix)
		}

		// Add space between words
		if i < len(words)-1 {
			result.WriteString(" ")
		}
	}

	return result.String(), nil
}

// SentenceTokens holds the token sequence for one or more sentences grouped
// for a single inference call. AllTokens is used post-inference to locate
// clause boundaries and insert silence at the corresponding waveform positions.
type SentenceTokens struct {
	Segments  [][]int64 // 509-token-limited chunks for ONNX inference
	AllTokens []int64   // flat token sequence for clause boundary detection
}

// TokenizeText runs the full pipeline: text → paragraphs → sentences → phonemes → token IDs.
// Consecutive sentences whose combined tokens fit under 509 are grouped into a
// single inference call so the model produces natural cross-sentence prosody.
// Returns [paragraph][group]SentenceTokens.
// emphasisOverride, when non-nil, overrides the phonemizer's configured default
// (p.emphasis) for this call only — enabling per-request emphasis control.
func (p *Phonemizer) TokenizeText(text string, emphasisOverride *bool) ([][]SentenceTokens, error) {
	paragraphs := SplitParagraphs(text)
	if len(paragraphs) == 0 {
		return nil, nil
	}

	useEmphasis := p.emphasis
	if emphasisOverride != nil {
		useEmphasis = *emphasisOverride
	}

	const spaceToken int64 = 16

	result := make([][]SentenceTokens, 0, len(paragraphs))
	for _, para := range paragraphs {
		sentences := SplitSentences(para)
		hints := sentenceHints(sentences) // computed on the raw sentences, before emphasis changes them

		// Tokenize each sentence individually
		var sentTokens [][]int64
		for si, sent := range sentences {
			if useEmphasis {
				sent = InjectEmphasis(sent)
			}
			phonemes, err := p.phonemizeHinted(sent, hints[si])
			if err != nil {
				return nil, err
			}
			tokens := p.tokenizer.Tokenize(phonemes)
			if len(tokens) > 0 {
				sentTokens = append(sentTokens, tokens)
			}
		}

		// Group consecutive sentences that fit under MaxTokensPerSegment
		var paraGroups []SentenceTokens
		i := 0
		for i < len(sentTokens) {
			allTokens := append([]int64{}, sentTokens[i]...)
			j := i + 1
			for j < len(sentTokens) {
				// +1 for space token between sentences
				if len(allTokens)+1+len(sentTokens[j]) > MaxTokensPerSegment {
					break
				}
				allTokens = append(allTokens, spaceToken)
				allTokens = append(allTokens, sentTokens[j]...)
				j++
			}

			segments := p.tokenizer.SplitTokens(allTokens)
			paraGroups = append(paraGroups, SentenceTokens{
				Segments:  segments,
				AllTokens: allTokens,
			})
			i = j
		}

		if len(paraGroups) > 0 {
			result = append(result, paraGroups)
		}
	}
	return result, nil
}

// goldOwnsWord reports whether the gold dictionary holds this exact word and the
// word is not itself a bare homograph key — in which case gold answers it and the
// homograph resolver is not consulted.
//
// ⚠️ The motivating case is INFLECTED forms, but the predicate does not test for
// inflection and must not claim to. For a word that is neither inflected nor a
// homograph ("table") it returns true and skipping the resolver is a harmless
// no-op, because Resolve would have declined anyway. An earlier name said
// "InflectedForm" and was simply the wrong noun for what it computes.
//
// A whole-word dictionary entry beats a stem-plus-suffix reconstruction.
// The resolver's inflected path strips a suffix, resolves the STEM as a
// homograph and re-attaches — so "closing" was rebuilt from "close" and came
// back klOsɪŋ, unstressed and with the wrong sibilant, while gold held klˈOzɪŋ
// the whole time.
//
// Measured by @wayland over the corpus (#64):
//
//	25 words / 1,150 occurrences are inflected AND present whole in gold
//	25 of 25 favour gold — he could not construct a case where the
//	                       reconstruction wins
//	842 occurrences (73%) arrived with NO STRESS MARK AT ALL: klOst 347x,
//	                       ɹiːdɪŋ 272x, lIvɪŋ 115x
//
// Two independent grounds. The reconstruction is unstressed (the #53 family,
// via #63's finding that 18 of 40 homograph strings carry no stress), and for
// producing/refusing/objecting the noun stem is wrong BY CONSTRUCTION — those
// forms have no noun reading, so ɹˈɛfjuːsɪŋ is not a context misfire but a word
// that does not exist.
//
// ⛔ This must not disturb the case the inflected path was built for. @clayton's
// ear caught "Percival bowed to the room" saying bˈOd, the ribbon vowel, because
// `bowed` is in NO dictionary. Nine such forms exist, `bowed` among them — none
// is in gold, so none reaches this check. The path keeps working exactly where
// it is load-bearing.
//
// ⚠️ A BARE homograph is never diverted: `close`, `read` and `record` still go
// to the resolver, because context is the only thing that can decide them.
func (p *Phonemizer) goldOwnsWord(lower string) bool {
	if p.gold == nil || p.homographs == nil {
		return false
	}
	// A bare homograph belongs to the resolver — context decides it, not gold.
	if p.homographs.IsHomograph(lower) {
		return false
	}
	// An empty rule means a DIRECT hit: gold holds this exact word, not a
	// suffix-stripped relative of it.
	_, rule, ok := p.gold.LookupWithSuffixRule(lower)
	return ok && rule == ""
}

// phonemizeWord converts a single word to IPA phonemes.
func (p *Phonemizer) phonemizeWord(words []string, idx int, word string, hint ResolveHint) string {
	lower := strings.ToLower(word)

	// The article "a" is always reduced to schwa in connected speech.
	// The gold dictionary has A (FACE diphthong /eɪ/) which overpronounces it.
	if lower == "a" {
		return phonemeSchwa
	}

	// The article "the" is reduced to /ðə/ in connected speech.
	// The gold dictionary has /ði/ ("thee") which sounds unnatural before consonants.
	if lower == "the" {
		return phonemeThe
	}

	// Step 0: Custom dictionary (author-provided pronunciations for proper nouns etc.)
	if p.custom != nil {
		if ipa, ok := p.custom.LookupWithSuffix(lower); ok {
			return ipa
		}
	}

	// Step 1: Check homograph resolver
	if !p.goldOwnsWord(lower) {
		if ipa, ok := p.homographs.ResolveWithHint(words, idx, hint); ok {
			return ipa
		}
	}

	// Step 2: Gold dictionary lookup (with suffix handling)
	if ipa, ok := p.gold.LookupWithSuffix(lower); ok {
		return ipa
	}

	// Step 3: Silver dictionary lookup (with suffix handling)
	if ipa, ok := p.silver.LookupWithSuffix(lower); ok {
		return ipa
	}

	// Step 4: Handle hyphenated words — try both dicts for each part
	if strings.Contains(lower, "-") {
		parts := strings.Split(lower, "-")
		var combined strings.Builder
		allFound := true
		for j, part := range parts {
			ipa, ok := p.lookupAllDicts(part)
			if !ok {
				allFound = false
				break
			}
			if j > 0 {
				combined.WriteString(phonemeJoin)
			}
			combined.WriteString(ipa)
		}
		if allFound && combined.Len() > 0 {
			return combined.String()
		}
	}

	// Step 5: Flite LTS fallback
	if ipa, _ := p.ltsWord(word); ipa != "" {
		return ipa
	}

	// Step 6: Skip word
	return ""
}

// ltsWord is the LTS fallback for a word no dictionary knows, returning the
// IPA and the suffix rule applied ("", "'s" or "'ll").
//
// A possessive is phonemized as stem + possessive suffix, the same way
// LookupWithSuffix builds it for a dictionary stem (#97). Sending the whole
// token to LTS let the trailing letters change the guess for the stem itself,
// so a character's name was pronounced one way bare and another way possessive:
// Lila lˈilə, Lila's lˈIlˈæz. The 'll contraction has the same shape (#99).
func (p *Phonemizer) ltsWord(word string) (string, string) {
	if stem, ok := strings.CutSuffix(word, "'s"); ok && stem != "" {
		if ipa := p.ltsStem(stem); ipa != "" {
			return ipa + possessiveSuffix(ipa), "'s"
		}
	}
	if stem, ok := strings.CutSuffix(word, "'ll"); ok && stem != "" {
		if ipa := p.ltsStem(stem); ipa != "" {
			return ipa + willSuffix(ipa), "'ll"
		}
	}
	return p.ltsStem(word), ""
}

// ltsStem runs LTS on a bare word. Capitalized words ending in 'a' get a final
// schwa (Lina→lˈinə, Sara→sˈɛɹə); LTS drops the final vowel.
func (p *Phonemizer) ltsStem(word string) string {
	ipa := p.lts.Phonemize(word)
	if ipa != "" && isCapitalized(word) && strings.HasSuffix(strings.ToLower(word), "a") && !strings.HasSuffix(ipa, phonemeSchwa) {
		ipa += phonemeSchwa
	}
	return ipa
}

// WordDiagnosis holds the result of diagnosing a single word's phonemization path.
type WordDiagnosis struct {
	Word     string // original word (after punctuation stripping)
	Phonemes string // IPA output
	Source   string // "custom", "homograph", "gold", "silver", "lts", or "skip"

	// Rule is "" when the word was found in the dictionary as written, and
	// otherwise the suffix that was stripped to reach a base ("'s", "'ll",
	// "-ies", "-s", "-ing", "-es", "-ed").
	//
	// ⚠️ Source alone does NOT tell you the word is in that dictionary — it
	// names where the BASE came from. `dresses` reports Source "gold" while
	// not being a gold key at all. Anyone asking "is this word listed?" wants
	// Rule == "", and reading Source as membership is how @wayland came one
	// post from refuting #56.
	Rule string
}

// DiagnoseText runs the phonemization pipeline and reports which dictionary
// each word resolved from. Useful for auditing LTS fallback coverage.
//
// 🛑 It reports PER SENTENCE, because that is the window synthesis uses.
//
// This used to run strings.Fields over the whole text. Phonemize applies Fields
// to whatever it is handed, so the code was identical and the WINDOW was not:
// TokenizeText hands it one sentence at a time, and this handed it the file.
// The only consumer of that slice is homographs.Resolve(words, idx) — a ±3-word
// context lookup — so a homograph could be resolved by a trigger in a NEIGHBOURING
// SENTENCE that synthesis would never have shown it (#47):
//
//	"She had written you a letter. Read it first."
//	  whole text   → ɹɛd     the reading this tool used to report
//	  per sentence → ɹiːd    the reading a listener actually gets
//
// That misreported 27 of 1,380 homograph occurrences on real corpus text — 2.0%,
// 22 of them in the wrong direction. @wayland and I both measured it and both
// published it as an engine defect; it was this function all along. The comment
// four lines below ("Mirror phonemizeWord resolution order") is the root cause
// stated out loud: a hand-maintained model of the engine drifts from the engine,
// which is the thing this lane says about hand-maintained lists everywhere else.
//
// ⚠️ Blast radius is exactly the homograph set. Every other resolution step is
// per-word, so LTS, gold, silver, custom and skip numbers were never affected.
func (p *Phonemizer) DiagnoseText(text string) []WordDiagnosis {
	var results []WordDiagnosis
	for _, para := range SplitParagraphs(text) {
		sents := SplitSentences(para)
		hints := sentenceHints(sents)
		for si, sent := range sents {
			results = append(results, p.diagnoseSentence(sent, hints[si])...)
		}
	}
	return results
}

// diagnoseSentence is DiagnoseText's body for a single sentence — the unit
// TokenizeText phonemizes.
func (p *Phonemizer) diagnoseSentence(text string, hint ResolveHint) []WordDiagnosis {
	normalized := NormalizeTextWithWords(text, p.isEnglishWord)
	words := strings.Fields(normalized)
	var results []WordDiagnosis

	for i, word := range words {
		_, core, _ := splitPunctuation(word)
		if core == "" {
			continue
		}
		lower := strings.ToLower(core)

		// Mirror phonemizeWord resolution order
		source := "skip"
		phonemes := ""
		rule := ""

		if lower == "a" {
			source, phonemes = "builtin", phonemeSchwa
		} else if lower == "the" {
			source, phonemes = "builtin", phonemeThe
		} else if p.custom != nil {
			if ipa, r, ok := p.custom.LookupWithSuffixRule(lower); ok {
				source, phonemes, rule = "custom", ipa, r
			}
		}

		if source == "skip" && !p.goldOwnsWord(lower) {
			if ipa, ok := p.homographs.ResolveWithHint(words, i, hint); ok {
				source, phonemes = "homograph", ipa
			}
		}
		if source == "skip" {
			if ipa, r, ok := p.gold.LookupWithSuffixRule(lower); ok {
				source, phonemes, rule = "gold", ipa, r
			}
		}
		if source == "skip" {
			if ipa, r, ok := p.silver.LookupWithSuffixRule(lower); ok {
				source, phonemes, rule = "silver", ipa, r
			}
		}
		if source == "skip" && strings.Contains(lower, "-") {
			parts := strings.Split(lower, "-")
			allFound := true
			var combined strings.Builder
			for j, part := range parts {
				ipa, ok := p.lookupAllDicts(part)
				if !ok {
					allFound = false
					break
				}
				if j > 0 {
					combined.WriteString(phonemeJoin)
				}
				combined.WriteString(ipa)
			}
			if allFound && combined.Len() > 0 {
				source, phonemes = "hyphenated", combined.String()
			}
		}
		if source == "skip" {
			// Shared with phonemizeWord so the diagnosis reports exactly what
			// the engine emits.
			if ipa, r := p.ltsWord(core); ipa != "" {
				source, phonemes, rule = "lts", ipa, r
			}
		}

		results = append(results, WordDiagnosis{
			Word:     core,
			Phonemes: phonemes,
			Source:   source,
			Rule:     rule,
		})
	}
	return results
}

// isEnglishWord reports whether word is an ordinary English word, used by the
// ALL-CAPS run rule (#41) to tell shouted prose from an acronym.
//
// Deliberately gold/silver only — NOT custom_dict. custom_dict holds banked
// proper nouns and pronounced-as-word acronyms (hvac, noc); consulting it here
// would let a banked acronym masquerade as an English word.
//
// Exact Lookup, not LookupWithSuffix: suffix handling exists to resolve
// possessives and plurals, and letting it fire here would widen membership on a
// question that needs an exact answer.
func (p *Phonemizer) isEnglishWord(word string) bool {
	if p.gold != nil {
		if _, ok := p.gold.Lookup(word); ok {
			return true
		}
	}
	if p.silver != nil {
		if _, ok := p.silver.Lookup(word); ok {
			return true
		}
	}
	return false
}

// lookupAllDicts tries custom, gold, and silver dictionaries (with suffix handling).
func (p *Phonemizer) lookupAllDicts(word string) (string, bool) {
	if p.custom != nil {
		if ipa, ok := p.custom.LookupWithSuffix(word); ok {
			return ipa, true
		}
	}
	if ipa, ok := p.gold.LookupWithSuffix(word); ok {
		return ipa, true
	}
	if ipa, ok := p.silver.LookupWithSuffix(word); ok {
		return ipa, true
	}
	return "", false
}

// splitPunctuation separates leading/trailing punctuation from a word.
// Punctuation that maps to Kokoro tokens is preserved; other punctuation is dropped.
func splitPunctuation(word string) (prefix, core, suffix string) {
	runes := []rune(word)
	start := 0
	end := len(runes)

	// Leading punctuation
	for start < end && isPunctuation(runes[start]) {
		start++
	}
	prefix = string(runes[:start])

	// Trailing punctuation
	for end > start && isPunctuation(runes[end-1]) {
		end--
	}
	suffix = string(runes[end:])
	core = string(runes[start:end])

	return
}

// isCapitalized returns true if the word starts with an uppercase letter.
func isCapitalized(word string) bool {
	for _, r := range word {
		return unicode.IsUpper(r)
	}
	return false
}

// isPunctuation returns true for characters that have Kokoro token mappings
// or that should be stripped from words.
func isPunctuation(r rune) bool {
	switch r {
	case '.', ',', '!', '?', ';', ':', '"', '(', ')', '\u201c', '\u201d',
		'\u2014', '\u2026', '\'', '-',
		// Square and curly brackets were absent while parentheses were present,
		// so a bracket stayed glued to its word and LTS pronounced the pair as
		// one: "She read [the note] aloud" gave "note]" \u2192 n\u02c8Ot\u02c8\u025b, "NOH-teh".
		// Ordinary prose, not just the log line that surfaced it.
		'[', ']', '{', '}',
		// Markdown emphasis. Same failure as the brackets above, found a second
		// time because that list was never finished: "*I" missed the dictionary
		// and LTS said \u02c8i \u2014 "EE am here" for "*I am here." (#58). Also "*I'm" \u2192
		// \u02c8\u026am and "*is" \u2192 \u02c8\u026as. 209 markers in one book, which uses *emphasis*
		// for its recurring motif.
		//
		// \u26a0\ufe0f Only '*'. '_' and '`' are the other two inline markers and both
		// occur ZERO times in that corpus \u2014 adding them would be a rule with no
		// measured population, which is the thing this lane refuses everywhere
		// else. Add them when someone counts them.
		//
		// Leading/trailing only, so arithmetic like 2*3 is untouched; the strip
		// loop handles stacked punctuation, which is what "*right*," needs.
		'*':
		return true
	}
	return false
}
