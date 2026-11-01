package phonemizer

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"unicode"
)

// HomographEntry represents pronunciation rules for a homograph word.
type HomographEntry struct {
	Default        string                      `json:"default"`
	Pronunciations map[string]HomographContext `json:"pronunciations"`
	// Imperative, when set, is the reading used for a capitalized, clause-initial
	// occurrence that matched no context trigger — almost always a command, e.g.
	// "Read it again." → ɹiːd rather than the past-tense default ɹɛd.
	Imperative string `json:"imperative,omitempty"`
	// NarratedPast, when set, is the reading for a capitalised, clause-initial occurrence that
	// comes right after a plain past-tense NARRATED sentence in the same paragraph, instead of the
	// imperative: "He opened the file. Read the first page." is narration with the subject
	// dropped, not a command (#112). Opt-in per entry; it never applies to dialogue (a token
	// that opens with a quotation mark), and without a hint Resolve behaves exactly as before.
	NarratedPast string `json:"narrated_past,omitempty"`
	// SubjectName, when set, is the reading when a capitalised word that is not a function word
	// stands directly before the homograph: a name or title as its subject. "Lily read the message
	// aloud." and "The Abbot read in silence." are narration, so "red", even when a later word
	// ("aloud") pulls the other way (#124). Opt-in per entry, like NarratedPast.
	SubjectName string `json:"subject_name,omitempty"`
	// InflectedDefault, when set, is the reading for an INFLECTED occurrence that
	// matched no context trigger. It exists because a suffix can carry part of
	// speech, and part of speech can settle a homograph outright:
	//
	//	bow    bare       dominantly a NOUN — the ribbon, the weapon
	//	bowed  inflected  unavoidably a VERB — and the verb sense is bending
	//
	// You cannot bow a ribbon. The archery and ribbon senses have no verb form,
	// so -ed does not merely hint at the bending sense, it forecloses the others.
	//
	// ⚠️ Deliberately opt-in per entry, not a rule applied to all 20 homographs.
	// Smiley's scoping (2026-08-16): "leading" is a legitimate verb AND noun AND
	// adjective, so a suffix forces nothing there. Only set this where the
	// inflection genuinely removes the ambiguity.
	InflectedDefault string `json:"inflected_default,omitempty"`
}

// HomographContext contains context trigger words for a specific pronunciation.
type HomographContext struct {
	ContextBefore []string `json:"context_before"`
	ContextAfter  []string `json:"context_after"`
	// ImmediatelyBefore are words that select this pronunciation ONLY when they are the word
	// directly before the homograph, in the same clause (#112). It exists because a window trigger
	// cannot say "adjacent": "the live thing" needs "the" right before "live", but as a window
	// trigger "the" also fires in "your stories live in" and "we'll live without the fancy plaster".
	// A determiner directly before "live" can only be followed by the adjective.
	ImmediatelyBefore []string `json:"immediately_before,omitempty"`
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
	return h.ResolveWithHint(words, idx, ResolveHint{})
}

// ResolveWithHint is Resolve with what the caller knows from outside the sentence (#112).
// A zero ResolveHint gives exactly Resolve's behaviour.
func (h *HomographResolver) ResolveWithHint(words []string, idx int, hint ResolveHint) (string, bool) {
	if idx < 0 || idx >= len(words) {
		return "", false
	}

	word := strings.ToLower(stripWordPunctuation(words[idx]))
	entry, ok := h.entries[word]

	// An inflected homograph must keep its disambiguation. Found by Clayton's
	// ear on 2026-08-16: "Percival bowed to the room" said bˈOd — "bode", the
	// ribbon/weapon vowel — because "bowed" is in no dictionary. It was resolved
	// later by stripping -ed to "bow" and taking gold's DEFAULT, which never
	// revisits context. Meanwhile homographs.json already had the rule, listing
	// "to" and "head" as triggers for the bending sense.
	//
	// So "bow to" was right and "bowed to" was wrong, and the class is wider
	// than one word: read/reading, lead/leading, wound/wounded, live/lived.
	//
	// Note this defect was invisible to BOTH of the sweeps built for #39 — the
	// word resolves cleanly from gold so it never reaches LTS, and bˈOd is
	// structurally well-formed so the -structural filter cannot see it either.
	// It is the Devi class: wrongly-known rather than unknown.
	// inflect re-attaches the suffix to whichever pronunciation wins below. It
	// must be applied at the return points rather than up front, because
	// edSuffix picks d/t/ɪd from the chosen IPA's final phoneme — and choosing
	// the IPA is the whole point of the context matching.
	inflect := func(ipa string) string { return ipa }
	inflected := false
	if !ok {
		var stem string
		var apply func(string) string
		if stem, apply, ok = inflectedHomographStem(word, h.entries); ok {
			entry, inflect, inflected = h.entries[stem], apply, true
		}
	}
	if !ok {
		return "", false
	}

	// ⚠️ An inflected form only overrides the dictionary when a context trigger
	// actually MATCHES. It must never fall through to entry.Default.
	//
	// Measured reason, not caution: all 28 pronunciations in homographs.json are
	// written WITHOUT stress marks (read → ɹɛd) while gold carries them
	// (read → ɹˈɛd). Claiming every inflected form unconditionally moved six
	// words in one probe off gold and onto unstressed homograph readings —
	// trading a wrong vowel for a lost stress on words that were already fine.
	//
	// Gating on a matched trigger makes this strictly additive: with evidence we
	// correct the vowel, without it the word takes the path it always took.
	// (The bare forms have always resolved unstressed this way. That is
	// pre-existing and worth its own look, but it is not this change's to widen.)

	// Check context window (±3 words) for trigger words.
	// context_before triggers are checked before the target word,
	// context_after triggers are checked after the target word.
	// Additionally, all triggers are checked in both directions as
	// a fallback (e.g., "yesterday" can appear after "read" in
	// "I read the book yesterday").
	windowSize := 3

	// The window must not reach past a clause boundary.
	// A trigger in a different clause is not evidence about this word, it is a coincidence of proximity.
	// At windowSize 3 that coincidence outranks the real evidence sitting next door.
	//
	// Measured by @wayland over 1,681 homograph occurrences (#60):
	//
	//	38   decided by a trigger OUTSIDE their own clause
	//	15   of those move stress, so they are audible
	//	~12  wrong today; 3 regress — the first favourable ratio this lane has produced
	//
	// The specimen is #60's: "measured in minutes, is the difference" — `is`
	// reaches back across the comma and picks the adjective mInjˈuːt, so a
	// duration is spoken as "my-NOOT". And #61 found the mirror image, a
	// trigger reaching FORWARD across a comma in "No, record it again."
	//
	// ⚠️ This is NOT the whole of #60. Two mechanisms were tangled all night:
	//
	//	cross-boundary   trigger in another clause      → fixed here
	//	bare imperative  NO trigger in range at all     → falls to entry.Default,
	//	                                                  and no window change reaches it
	//
	// "From here on, record every call." has no trigger in its clause, so
	// isolating the clause changes nothing. That class belongs to the default,
	// not the window, and it stays open.
	lo, hi := clauseBounds(words, idx, windowSize)

	// Find the best matching pronunciation by closest trigger word.
	//
	// ⚠️ Nearest-trigger alone does NOT make this deterministic, though the
	// comment here used to claim it did. It discriminates only when the
	// distances DIFFER. On a tie, `dist < bestDist` keeps whichever pronunciation
	// was seen first — and that is Go's randomised map iteration order.
	//
	// Found by @wayland (#49): the same binary on the same corpus resolved
	// differently across runs, words flipping between gold and homograph.
	// Reproduced in-process, 200 iterations over real corpus sentences:
	//
	//	wound → wuːnd   (the injury)
	//	wound → wWnd    (past tense of wind)
	//
	// That is a MEANING change in prose, and it made narration unreproducible:
	// sections render in separate passes, so s5 and s10 could disagree — which
	// breaks the identical-rendering requirement the Keeper's Door timestamp
	// work was built around.
	//
	// Two changes below: iterate in sorted key order so the traversal is stable,
	// and resolve exact ties toward entry.Default rather than by arrival. A tie
	// means context supplied no discriminating evidence, and the default is
	// precisely what we use when there is no evidence.
	prons := make([]string, 0, len(entry.Pronunciations))
	for pron := range entry.Pronunciations {
		prons = append(prons, pron)
	}
	sort.Strings(prons)

	// A word that selects a pronunciation only when directly before this one (see
	// HomographContext.ImmediatelyBefore) decides it outright: adjacency is stronger evidence than
	// any trigger further away ("where the live data goes": "the" beats "where").
	if idx > 0 && !endsClause(words[idx-1]) {
		prev := strings.ToLower(stripWordPunctuation(words[idx-1]))
		for _, pron := range prons {
			for _, w := range entry.Pronunciations[pron].ImmediatelyBefore {
				if w == prev && (!inflected || pron != entry.Default) {
					return inflect(pron), true
				}
			}
		}
	}

	// A name or title directly before decides it for an entry that opts in (#124).
	// ...unless an earlier word in the clause already makes it a base form: "Did Lily read it?",
	// "Can Mara read this?", "Let Tom read it first" (an auxiliary or modal sits before the name).
	if !inflected && entry.SubjectName != "" && precededByNameSubject(words, idx) &&
		!triggerBeforeName(entry, words, lo, idx-1) {
		return entry.SubjectName, true
	}

	bestPron := ""
	bestDist := windowSize + 1
	// better reports whether a candidate at distance d should replace the
	// incumbent: strictly nearer, or an equal-distance tie that the default wins.
	better := func(pron string, d int) bool {
		if d < bestDist {
			return true
		}
		return d == bestDist && pron == entry.Default && bestPron != entry.Default
	}

	// First pass: strict directional matching (higher confidence)
	for _, pron := range prons {
		ctx := entry.Pronunciations[pron]
		for _, trigger := range ctx.ContextBefore {
			for i := lo; i < idx; i++ {
				if strings.ToLower(stripWordPunctuation(words[i])) == trigger {
					if dist := idx - i; better(pron, dist) {
						bestDist = dist
						bestPron = pron
					}
				}
			}
		}
		for _, trigger := range ctx.ContextAfter {
			for i := idx + 1; i <= hi; i++ {
				if strings.ToLower(stripWordPunctuation(words[i])) == trigger {
					if dist := i - idx; better(pron, dist) {
						bestDist = dist
						bestPron = pron
					}
				}
			}
		}
	}

	if bestPron != "" && (!inflected || bestPron != entry.Default) {
		return inflect(bestPron), true
	}

	// A capitalized, clause-initial occurrence with an imperative reading set is
	// almost always a command (e.g. "Read it again." / "Read them slowly.") —
	// present tense. Resolve it before the broad second pass, which is
	// direction-blind and would otherwise mis-fire on an object like "it"
	// in "Read it" (the "it" trigger is meant for the past-tense "it read").
	if !inflected && entry.Imperative != "" && isImperativePosition(words, idx) {
		// ...unless it follows past-tense narration: "He opened the file. Read the first page."
		// Dialogue never takes this (a token that opens with a quote mark is speech).
		if hint.PrevSentenceNarratedPast && entry.NarratedPast != "" && !opensWithQuote(words[idx]) {
			return entry.NarratedPast, true
		}
		return entry.Imperative, true
	}

	// Second pass: check all triggers in both directions (broader matching)
	for _, pron := range prons {
		ctx := entry.Pronunciations[pron]
		// ⚠️ NOT append(ctx.ContextBefore, ctx.ContextAfter...). That writes into
		// ContextBefore's backing array when the result FITS in its capacity —
		// encoding/json grows these slices with append, so the spare capacity is
		// real. Measured over all 40 pronunciation entries:
		//
		//	6 write in place    read ɹiːd (22/32 + 6) · minute · content ·
		//	                    conduct · object əbʤˈɛkt · project
		//	                    ⚑ `read` is the corpus's most frequent homograph
		//
		// ⛔ The condition is len(before)+len(after) <= cap(before), NOT merely
		// cap > len. `record`'s verb list is 13/16 + 8 = 21, which OVERSHOOTS the
		// capacity and reallocates — it does not alias, and citing it as the
		// example (as the commit that introduced this fix did) was wrong.
		// Entries whose ContextAfter is EMPTY do not alias either, however much
		// spare capacity they have: appending nothing writes nothing. That
		// excludes read ɹɛd and object ˈɑbʤɛkt.
		//
		// The stored slice's LENGTH never changes, so nothing reads the writes
		// and no reading is currently wrong. The problem is that a
		// HomographResolver is shared by every request goroutine, so two
		// concurrent Resolve calls write the same three words into the same
		// backing array at the same time — a data race whose consequences are
		// undefined even though the values happen to agree.
		//
		// Not an observed defect. Fixed as hygiene, because the observable form
		// of this class is #49: a resolver that answers differently between runs
		// and breaks reproducible narration.
		allTriggers := make([]string, 0, len(ctx.ContextBefore)+len(ctx.ContextAfter))
		allTriggers = append(allTriggers, ctx.ContextBefore...)
		allTriggers = append(allTriggers, ctx.ContextAfter...)
		for _, trigger := range allTriggers {
			for i := lo; i <= hi; i++ {
				if i == idx {
					continue
				}
				dist := idx - i
				if dist < 0 {
					dist = -dist
				}
				if strings.ToLower(stripWordPunctuation(words[i])) == trigger {
					if better(pron, dist) {
						bestDist = dist
						bestPron = pron
					}
				}
			}
		}
	}

	if bestPron != "" && (!inflected || bestPron != entry.Default) {
		return inflect(bestPron), true
	}

	// An inflected form whose SUFFIX settles the part of speech takes that
	// reading even with no trigger. "The branches bowed under snow" matches
	// nothing in the trigger list and is still unambiguously the bending sense,
	// because -ed forces a verb and the other senses of "bow" have no verb form.
	if inflected && entry.InflectedDefault != "" {
		return inflect(entry.InflectedDefault), true
	}

	// Otherwise: no context match, or an inflected form whose context merely
	// reconfirms the default. Decline for inflected forms — the dictionary
	// layers below already produce that sense AND carry stress, so deferring to
	// them avoids trading a wrong vowel for a lost stress. Only the bare word
	// takes the default here.
	if inflected {
		return "", false
	}
	return entry.Default, true
}

// inflectedHomographStem finds a homograph stem for an inflected surface form,
// returning the stem and a function that re-attaches the suffix's phonemes.
//
// It deliberately reuses edBases/ingBases and edSuffix/possessiveSuffix from the
// dictionary layer rather than restating the rules. A second, parallel copy of
// English inflection would drift from the first, and "a tool that mirrors engine
// logic must stay synced with it" is a lesson this repo has already paid for.
//
// Only -ed, -ing and -s are handled: those are the inflections that carry a
// homograph's sense across (bowed/bowing, reading, leads). Comparative and
// derivational forms are not inflections of the same word and are left alone.
func inflectedHomographStem(word string, entries map[string]HomographEntry) (string, func(string) string, bool) {
	has := func(s string) bool { _, ok := entries[s]; return ok }

	if strings.HasSuffix(word, "ed") && len(word) > 3 {
		for _, base := range edBases(word) {
			if has(base) {
				return base, func(ipa string) string { return ipa + edSuffix(ipa) }, true
			}
		}
	}
	if strings.HasSuffix(word, "ing") && len(word) > 4 {
		for _, base := range ingBases(word) {
			if has(base) {
				return base, func(ipa string) string { return ipa + "ɪŋ" }, true
			}
		}
	}
	if strings.HasSuffix(word, "s") && len(word) > 1 {
		base := word[:len(word)-1]
		if has(base) {
			return base, func(ipa string) string { return ipa + possessiveSuffix(ipa) }, true
		}
	}
	return "", nil, false
}

// isImperativePosition reports whether words[idx] is a capitalized word at the
// start of a sentence or quoted clause — the position where an imperative verb
// like "Read it again." sits. It requires BOTH a leading capital AND a preceding
// clause boundary, so stylized ALL-CAPS mid-sentence text ("THE WARDS READ
// THEM") does not falsely trigger the imperative reading.
// endsClause reports whether a token terminates the clause it sits in — i.e.
// whether the NEXT token begins a new one. The punctuation is read off the raw
// token because stripWordPunctuation is applied only to the trigger comparison,
// which is exactly why "no," matches the trigger "no" today.
//
// Sentence terminators are included even though SplitSentences normally removes
// them: Resolve takes an arbitrary []string and must not depend on who built it.
func endsClause(word string) bool {
	r := []rune(word)
	for i := len(r) - 1; i >= 0; i-- {
		switch r[i] {
		case ',', ';', ':', '—', '.', '!', '?':
			return true
		// Trailing quotes and brackets close around the punctuation rather than
		// replacing it — `said,"` is still a boundary — so look past them.
		case '"', '\'', ')', ']', '}', '”', '’':
			continue
		default:
			return false
		}
	}
	return false
}

// clauseBounds narrows the ±windowSize context window to the clause containing
// idx, returning inclusive scan bounds. A trigger beyond a comma, semicolon,
// colon or dash is in a different clause and is not evidence about this word.
func clauseBounds(words []string, idx, windowSize int) (lo, hi int) {
	lo = max(0, idx-windowSize)
	hi = min(len(words)-1, idx+windowSize)

	for j := idx - 1; j >= lo; j-- {
		if endsClause(words[j]) {
			lo = j + 1
			break
		}
	}
	// Start at idx itself: "record," ends the clause, so nothing after it counts.
	for j := idx; j < hi; j++ {
		if endsClause(words[j]) {
			hi = j
			break
		}
	}
	return lo, hi
}

// subjectNotFunctionWords are capitalised words that can stand right before "read" without being
// its subject: a sentence-initial adverb, auxiliary, conjunction or the like ("Then read it.",
// "Please read aloud.", "Do read this."). Lower-cased.
var subjectNotFunctionWords = map[string]bool{
	"please": true, "then": true, "now": true, "just": true, "always": true, "never": true, "ever": true,
	"first": true, "next": true, "also": true, "finally": true, "once": true, "again": true, "and": true,
	"but": true, "so": true, "or": true, "if": true, "when": true, "as": true, "while": true, "do": true,
	"does": true, "did": true, "don't": true, "don\u2019t": true, "let": true, "can": true, "could": true,
	"will": true, "would": true, "should": true, "shall": true, "may": true, "might": true, "must": true,
	"not": true, "to": true, "i": true, "we": true, "you": true, "they": true, "he": true, "she": true,
	"it": true, "who": true, "that": true, "which": true, "how": true, "what": true, "why": true,
	"where": true, "whoever": true, "everyone": true, "someone": true, "anyone": true, "people": true,
}

// triggerBeforeName reports whether any context_before trigger of a pronunciation OTHER than the
// subject-name reading occurs in words[lo:nameIdx]: the words before the name in the same clause.
func triggerBeforeName(entry HomographEntry, words []string, lo, nameIdx int) bool {
	for pron, ctx := range entry.Pronunciations {
		if pron == entry.SubjectName {
			continue
		}
		for _, trigger := range ctx.ContextBefore {
			for i := lo; i < nameIdx; i++ {
				if strings.ToLower(stripWordPunctuation(words[i])) == trigger {
					return true
				}
			}
		}
	}
	return false
}

// pluralDeterminers are the words that stand right before a plural noun and never before a bare name.
var pluralDeterminers = map[string]bool{
	"the": true, "these": true, "those": true, "our": true, "your": true, "their": true, "its": true,
	"all": true, "some": true, "many": true, "my": true, "his": true, "her": true, "both": true, "few": true,
}

// precededByNameSubject reports whether the word directly before words[idx] is a capitalised word
// that is not a function word, in the same clause: "Lily read", "the Abbot read".
func precededByNameSubject(words []string, idx int) bool {
	if idx == 0 || endsClause(words[idx-1]) {
		return false
	}
	prev := stripWordPunctuation(words[idx-1])
	r := []rune(prev)
	if len(r) == 0 || !unicode.IsUpper(r[0]) {
		return false
	}
	// A contraction is a pronoun or auxiliary ("I'll read it tomorrow", "We've read"), not a name.
	if strings.ContainsAny(prev, "'\u2019") {
		return false
	}
	// A capitalised word ending in a single "s" with a determiner right before it is a plural noun
	// ("the Troubadours read your story aloud", "The Monks read the texts"): present tense as often
	// as past, so no subject evidence. A bare name ending in s ("Thomas read the message aloud",
	// Silas, Marcus, Agnes) never has a determiner and keeps the rule (Rambald's review of #126). The
	// determiner must be in the same clause: "He looked at her. Thomas read the letter aloud." has
	// "her." before "Thomas", which ends the sentence and is not its determiner.
	if lower := strings.ToLower(prev); strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") && idx >= 2 &&
		!endsClause(words[idx-2]) && pluralDeterminers[strings.ToLower(stripWordPunctuation(words[idx-2]))] {
		return false
	}
	return !subjectNotFunctionWords[strings.ToLower(prev)]
}

func isImperativePosition(words []string, idx int) bool {
	s := stripWordPunctuation(words[idx])
	if s == "" || !unicode.IsUpper([]rune(s)[0]) {
		return false
	}
	if idx == 0 {
		return true
	}
	prev := []rune(words[idx-1])
	switch prev[len(prev)-1] {
	case '.', '!', '?', ':', ';', '"', '”', '’', ')', '—':
		return true
	}
	return false
}

// stripWordPunctuation removes leading and trailing punctuation from a word.
func stripWordPunctuation(word string) string {
	return strings.TrimFunc(word, func(r rune) bool {
		switch r {
		case '.', ',', '!', '?', ';', ':', '"', '(', ')', '\'', '-',
			'\u201c', '\u201d', '\u2014', '\u2026',
			// Brackets, matching isPunctuation. Without them a bracketed trigger
			// word ("[to]") never matches its context rule.
			'[', ']', '{', '}':
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

// opensWithQuote reports whether a raw token begins with a quotation mark, so it opens speech.
func opensWithQuote(word string) bool {
	return strings.HasPrefix(word, "\"") || strings.HasPrefix(word, "\u201c")
}
