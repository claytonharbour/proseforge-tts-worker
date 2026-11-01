package phonemizer

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	// markdownBoldItalicRe matches ***text*** or ___text___
	markdownBoldItalicRe = regexp.MustCompile(`\*{3}([^*]+)\*{3}|_{3}([^_]+)_{3}`)
	// markdownBoldRe matches **text** or __text__
	markdownBoldRe = regexp.MustCompile(`\*{2}([^*]+)\*{2}|_{2}([^_]+)_{2}`)
	// markdownItalicRe matches *text* or _text_
	markdownItalicRe = regexp.MustCompile(`\*([^*]+)\*|_([^_]+)_`)
	// markdownCodeRe matches inline `code` spans and ```fenced``` blocks, keeping
	// the CONTENTS and dropping only the ticks.
	//
	// Asterisks were already stripped; backticks were not, so `gobble` reached the
	// dictionary as "`gobble`" and fell to LTS — 127 inline spans across 5 books
	// (@Wayland). Markdown handling was partial, not absent, which is easy to miss
	// because the half that works is the visible one.
	//
	// ⚠️ Dropping the block rather than its markers is the tempting default and it
	// is WRONG here: the single fenced block in 350 sections is the plot device of
	// The Keeper's Door — the anomalous log entry Mara finds. Dropping it removes
	// the mystery from the audiobook and leaves the §10 callback referring to
	// something the listener never heard. Keep the contents; drop the ticks.
	markdownCodeRe = regexp.MustCompile("`+([^`]+)`+")

	// alphaNumRe matches letter+digit combos like Q3, B2, F16
	alphaNumRe = regexp.MustCompile(`\b([A-Za-z])(\d+)\b`)
	// digitLetterRe / letterDigitRe split any remaining alphanumeric run so the
	// digits survive to the number expander (#43). Applied late — see the call
	// site for why the ordering matters.
	digitLetterRe = regexp.MustCompile(`(\d)([A-Za-z])`)
	letterDigitRe = regexp.MustCompile(`([A-Za-z])(\d)`)
	// percentRe matches N% (with or without space)
	percentRe = regexp.MustCompile(`\b(\d+)%`)
	// numberRe matches standalone integers (not part of a larger word)
	numberRe = regexp.MustCompile(`\b\d+\b`)
	// decimalRe matches decimal numbers like 3.14
	decimalRe = regexp.MustCompile(`\b(\d+)\.(\d+)\b`)
	// currencyRe matches $N, $N.NN
	currencyRe = regexp.MustCompile(`\$(\d+(?:\.\d{2})?)`)
	// ordinalRe matches 1st, 2nd, 3rd, 4th, etc.
	ordinalRe = regexp.MustCompile(`\b(\d+)(st|nd|rd|th)\b`)
	// timeRe matches 10:30, 2:00, etc.
	timeRe = regexp.MustCompile(`\b(\d{1,2}):(\d{2})\b`)
	// timestampRe matches a three-part HH:MM:SS timestamp. It must run BEFORE
	// timeRe, which would otherwise match only the HH:MM prefix and leave the
	// seconds glued to a literal colon ("two seventeen:thirty three").
	//
	// The fractional part is captured HERE rather than left to decimalRe. Once
	// this rule consumes the seconds, decimalRe no longer sees "33.847" as a
	// decimal — the bare ".847" fell through to the plain number expander and
	// became "point eight hundred forty seven". Dropping or garbling data from a
	// log line is the exact failure this lane spent a night on.
	timestampRe = regexp.MustCompile(`\b(\d{1,2}):(\d{2}):(\d{2})(?:\.(\d+))?\b`)
	// yearRe matches 4-digit years (1100-2099) preceded by context words that
	// signal a year rather than a quantity. Captures: [1]=context word, [2]=year.
	yearRe = regexp.MustCompile(`(?i)\b(in|of|from|since|by|around|circa|year|before|after|until|during|early|late|mid)\s+(1[1-9]\d{2}|20\d{2})\b`)
	// decadeRe matches 1990s, 1980s, etc.
	decadeRe = regexp.MustCompile(`\b(\d{4})s\b`)
	// abbreviationDotRe matches U.S.A., Dr., etc.
	abbreviationDotRe = regexp.MustCompile(`\b([A-Z]\.){2,}`)
	// acronymRe matches 2-4 uppercase letter sequences like CEO, FTC, HR, AR
	acronymRe = regexp.MustCompile(`\b[A-Z]{2,4}\b`)
	// acronymPluralRe matches an acronym with a lowercase plural s: LLCs, IDs,
	// LEDs, SREs. The trailing s is a word character, so acronymRe's closing \b
	// never matches and these fell straight through to LTS as consonant mush
	// (LLCs → lks, SREs → sɹz — no vowel at all). 24 occurrences over 8 forms
	// in the corpus. Found by Wayland, #39.
	acronymPluralRe = regexp.MustCompile(`\b([A-Z]{2,4})s\b`)
	// romanNumeralRe matches a NUMBERING WORD followed by a Roman numeral.
	// Deliberately gated on the preceding word rather than matching numerals
	// anywhere, because the Roman alphabet is a subset of English spelling and the
	// collisions are ordinary words: MIX (1009), DID, DIM, MID, LID, CIVIC — and
	// worst, CD, which is a valid canonical numeral (400) and a common noun.
	// "Class CD" is implausible; a bare CD is not. See #57.
	//
	// {2,} excludes a bare I, which is the pronoun far more often than a numeral,
	// and excludes lone V and X, which already read correctly.
	romanNumeralRe = regexp.MustCompile(`(?i)\b(class|chapter|section|part|book|volume|act|phase|level|type|grade|tier|war|stage|appendix|article|figure|table|title|mark|round|paragraph)\s+([IVXLCDM]{2,})\b`)
	// canonicalRomanRe rejects non-canonical spellings (IIII, VX, IC) so an
	// arbitrary run of these letters is not read as a number.
	canonicalRomanRe = regexp.MustCompile(`(?i)^M{0,3}(CM|CD|D?C{0,3})(XC|XL|L?X{0,3})(IX|IV|V?I{0,3})$`)
	// stStreetRe captures the words either side of "St." so the two senses can be
	// told apart. Both groups are re-emitted by the replacement; RE2 has no
	// lookahead, so the trailing word must be consumed to be inspected.
	stStreetRe = regexp.MustCompile(`(\b[\p{L}']+)(\s+)[Ss]t\.(\s+([\p{L}']+))?`)
	// capsRunTokenRe matches all-caps words of ANY length, for detecting
	// all-caps RUNS (see expandAcronymsInContext). Deliberately wider than
	// acronymRe: in "I AM WATCHING", it is the 8-letter WATCHING — which
	// acronymRe can never match — that makes AM part of a run.
	capsRunTokenRe = regexp.MustCompile(`\b[A-Z]{2,}\b`)
	// sentenceBreakRe matches text that ends an all-caps run. A run is emphasis
	// ("STOP WAIT HELP"); consecutive abbreviations in ordinary prose are not
	// ("...the department. DA. FBI. Someone..."). Without this, that single
	// specimen — the only one in 612k words of corpus — reads DA as "duh".
	sentenceBreakRe = regexp.MustCompile(`[.!?]`)
	// slashAcronymRe matches patterns like A/B
	slashAcronymRe = regexp.MustCompile(`\b([A-Z])/([A-Z])\b`)
	// mixedCaseTermRe matches case-sensitive technical terms that acronymRe
	// (all-caps only) can never see. Without this, they fall through to LTS,
	// which sounds them out as words: PhD→"fd", pH→"f", MHz→"mz" (issue #26).
	mixedCaseTermRe = regexp.MustCompile(`\b(PhD|pH|[MkGT]Hz|Hz)\b`)
	// degreeRe matches °C / °F with optional surrounding space (21°C, 21 °C)
	degreeRe = regexp.MustCompile(`\s*°\s*([CF])\b`)
	// plusMinusRe matches ± with optional surrounding space (±5, 40 ± 2)
	plusMinusRe = regexp.MustCompile(`\s*±\s*`)
	// markdownHeadingRe matches # heading markers at line start
	markdownHeadingRe = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	// paragraphBreakRe matches blank lines (paragraph boundaries)
	paragraphBreakRe = regexp.MustCompile(`\n\s*\n`)
)

// Common abbreviation expansions
var abbreviations = map[string]string{
	"dr.":     "doctor",
	"mr.":     "mister",
	"mrs.":    "misses",
	"ms.":     "miss",
	"prof.":   "professor",
	"sr.":     "senior",
	"jr.":     "junior",
	"st.":     "saint",
	"ave.":    "avenue",
	"blvd.":   "boulevard",
	"dept.":   "department",
	"govt.":   "government",
	"gen.":    "general",
	"sgt.":    "sergeant",
	"cpl.":    "corporal",
	"pvt.":    "private",
	"lt.":     "lieutenant",
	"capt.":   "captain",
	"maj.":    "major",
	"col.":    "colonel",
	"etc.":    "etcetera",
	"vs.":     "versus",
	"approx.": "approximately",
	"est.":    "established",
	"vol.":    "volume",
	"no.":     "number",
	"fig.":    "figure",
	"rev.":    "reverend",
	"esq.":    "esquire",
}

// nameSuffixAbbreviations may be followed by punctuation rather than a space,
// because they end a name: "Jester Jr., left", "Jr.'s hat", "Foot, Esq.," (#100).
// Only these get the punctuation-tolerant match. "no." also expands, and letting
// it match before a quote would read dialogue "No." as "number".
var nameSuffixAbbreviations = map[string]bool{"jr.": true, "sr.": true, "esq.": true}

// SplitParagraphs splits text at blank lines (\n\n). Returns trimmed non-empty paragraphs.
func SplitParagraphs(text string) []string {
	parts := paragraphBreakRe.Split(text, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// NormalizeText converts numbers, abbreviations, and special punctuation to speakable text.
//
// Acronym handling has no dictionary available here, so every 2-4 letter
// all-caps token letter-expands unless it is on the hand-written skip list.
// Callers holding the Misaki dictionaries should use NormalizeTextWithWords,
// which can tell SHOUTED PROSE from an acronym.
func NormalizeText(text string) string {
	return normalizeText(text, nil)
}

// NormalizeTextWithWords is NormalizeText with an English-word oracle, used to
// stop ALL-CAPS prose being spelled out letter by letter (#41).
//
// isWord reports whether a lowercased token is an ordinary English word — in
// practice a gold/silver dictionary lookup. It is consulted ONLY for tokens
// inside an all-caps run, never in isolation, because the dictionary answers
// "is this a known token", not "is this a word rather than an acronym": AM, PM,
// DA, ER, PA, AG, OS and SEC are all in it and all currently letter-expand
// correctly.
func NormalizeTextWithWords(text string, isWord func(string) bool) string {
	return normalizeText(text, isWord)
}

func normalizeText(text string, isWord func(string) bool) string {
	// Strip markdown formatting (must happen before sentence splitting to avoid
	// false sentence boundaries inside *italic* or **bold** blocks)
	text = markdownHeadingRe.ReplaceAllString(text, "")
	text = markdownCodeRe.ReplaceAllString(text, "${1}")
	text = markdownBoldItalicRe.ReplaceAllString(text, "${1}${2}")
	text = markdownBoldRe.ReplaceAllString(text, "${1}${2}")
	text = markdownItalicRe.ReplaceAllString(text, "${1}${2}")

	// Normalize curly apostrophes to straight (must happen before dictionary lookups)
	text = strings.ReplaceAll(text, "\u2019", "'") // right single quotation mark '
	text = strings.ReplaceAll(text, "\u2018", "'") // left single quotation mark '

	// Paragraph breaks → sentence-ending pause (before whitespace normalization)
	text = paragraphBreakRe.ReplaceAllString(text, ".\n")
	text = strings.ReplaceAll(text, "\n", " ")

	// Normalize unicode whitespace
	text = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, text)

	// Expand number-adjacent symbols LTS can't speak: 21°C → 21 degrees Celsius,
	// ±5 → plus or minus 5. Before space-collapse so the padding normalizes away.
	text = degreeRe.ReplaceAllStringFunc(text, func(s string) string {
		if strings.HasSuffix(s, "F") {
			return " degrees Fahrenheit"
		}
		return " degrees Celsius"
	})
	text = plusMinusRe.ReplaceAllString(text, " plus or minus ")

	// Silent symbols (#42): these produce NO phonemes at all — the reader hears
	// nothing where the author wrote something. Only the three whose reading is
	// unambiguous are expanded here.
	//
	// ≈ is the one that changes meaning rather than merely losing information:
	// "≈ four hundred strong" spoken as "four hundred strong" turns a hedge into
	// a precise claim.
	//
	// & runs before the acronym pass below, so the letters either side become
	// space-bounded and reach that pass at all.
	//
	// ⚠️ That is NOT sufficient for every case, and AT&T is the counter-example:
	// it becomes "at and T" — "AT" is space-bounded by then, hits the
	// common-word skip list as the preposition "at", and the lone "T" is below
	// acronymRe's {2,4}. Pre-existing, surfaced by writing the test rather than
	// caused by this change, and left alone: shortening the skip list to rescue
	// it would reintroduce the AM bug for every ordinary "at".
	//
	// ⚠️ DELIBERATELY NOT expanded, because each needs a decision rather than a
	// default — silence is wrong, but so is guessing:
	//   →  "to" reads well between places (London → Zurich) and wrongly between
	//      abstractions (cause → effect wants "leads to"). Meaning-changing, so
	//      the wrong guess is worse than the current silence is.
	//   ×  "three BY four metres" vs "three TIMES four" — same glyph, different
	//      words, decided by context we do not parse.
	//   | • ↓  layout and trend marks. These probably want a PAUSE rather than a
	//      word, which is a different mechanism (see the em-dash token), not a
	//      missing entry in this list.
	// Separators that FUSE rather than drop. Distinct from the silent symbols
	// below: `=` and `_` are already silent (LTS skips anything outside a-z), so
	// they contribute no sound — but they glue the words either side into one
	// token that LTS then pronounces as a single mangled word:
	//
	//	source=unknown           → sˈɔɹsɪˈʌnnˈOn
	//	event_type=unclassified  → ˈivˈɛnttˈIpˈʌnklˈæsfˈId   (four primary stresses)
	//
	// ⚑ Separating is unambiguous and does NOT decide how `=` reads. Replacing it
	// with a space preserves exactly the silence it already has and removes only
	// the fusing. Whether `=` should be spoken ("equals") is still open — see the
	// undecided list below — and this change neither answers nor forecloses it.
	//
	// Same argument as the digit split in #43: preserve the words without
	// choosing the symbol's reading.
	text = strings.ReplaceAll(text, "=", " ")
	text = strings.ReplaceAll(text, "_", " ")

	text = strings.ReplaceAll(text, "&", " and ")
	text = strings.ReplaceAll(text, "≈", " approximately ")
	text = strings.ReplaceAll(text, "§", " section ")

	// Collapse multiple spaces
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	// Preserve em dash and en dash as dedicated pause tokens (token ID 9)
	text = strings.ReplaceAll(text, "—", " — ")
	text = strings.ReplaceAll(text, "–", " — ")

	// Preserve ellipsis as dedicated pause token (token ID 10)
	text = strings.ReplaceAll(text, "...", " … ")
	text = strings.ReplaceAll(text, "…", " … ")

	// Expand abbreviations (case-insensitive match)
	text = expandStreetSt(text)
	text = expandAbbreviations(text)

	// Expand dotted abbreviations like U.S.A. → U S A
	text = abbreviationDotRe.ReplaceAllStringFunc(text, func(s string) string {
		return strings.ReplaceAll(strings.TrimSuffix(s, "."), ".", " ")
	})

	// Expand slash patterns like A/B → A B
	text = slashAcronymRe.ReplaceAllString(text, "${1} ${2}")

	// Expand mixed-case technical terms (case-sensitive, so prose words like
	// "phase" are untouched). Letters-vs-word renderings are canon (issue #26).
	text = mixedCaseTermRe.ReplaceAllStringFunc(text, func(s string) string {
		return mixedCaseTerms[s]
	})

	// Expand uppercase acronyms (CEO → C E O, FTC → F T C)
	// Must come after abbreviation dot handling to avoid double-expansion
	// Plurals first: acronymRe cannot match "LLCs" (the lowercase s blocks its
	// closing \b), so this would otherwise reach LTS as unpronounceable
	// consonants. Running it first also keeps it clear of the ALL-CAPS run rule
	// below — a lowercase s means the token is never part of a shouting run.
	text = acronymPluralRe.ReplaceAllStringFunc(text, func(s string) string {
		return expandAcronymPlural(strings.TrimSuffix(s, "s"))
	})
	text = expandRomanNumerals(text, isWord)
	text = expandAcronymsInContext(text, isWord)

	// Expand decades: 1990s → nineteen nineties (must run before year expansion)
	text = decadeRe.ReplaceAllStringFunc(text, expandDecade)

	// Expand years with context: "in 1987" → "in nineteen eighty seven"
	text = yearRe.ReplaceAllStringFunc(text, func(s string) string {
		matches := yearRe.FindStringSubmatch(s)
		if len(matches) < 3 {
			return s
		}
		return matches[1] + " " + expandYear(matches[2])
	})

	// Expand a three-part timestamp before the HH:MM rule, which would match only
	// the prefix and leave "two seventeen:thirty three" — seconds glued to a
	// literal colon. See expandTimestamp for why digits and not clock form.
	text = timestampRe.ReplaceAllStringFunc(text, expandTimestamp)

	// Expand time: 10:30 → ten thirty
	text = timeRe.ReplaceAllStringFunc(text, expandTime)

	// Expand ordinals: 1st → first
	text = ordinalRe.ReplaceAllStringFunc(text, expandOrdinal)

	// Expand currency: $100 → one hundred dollars
	text = currencyRe.ReplaceAllStringFunc(text, expandCurrency)

	// Expand percent: 14% → fourteen percent
	text = percentRe.ReplaceAllStringFunc(text, func(s string) string {
		matches := percentRe.FindStringSubmatch(s)
		if len(matches) < 2 {
			return s
		}
		n := parseNumber(matches[1])
		if n < 0 {
			return s
		}
		return numberToWords(n) + " percent"
	})

	// Expand alphanumeric: Q3 → Q three, B2 → B two
	text = alphaNumRe.ReplaceAllStringFunc(text, func(s string) string {
		matches := alphaNumRe.FindStringSubmatch(s)
		if len(matches) < 3 {
			return s
		}
		n := parseNumber(matches[2])
		if n < 0 {
			return s
		}
		return strings.ToUpper(matches[1]) + " " + numberToWords(n)
	})

	// Separate any remaining digit/letter run so the number expander below can
	// see the digits at all (#43).
	//
	// alphaNumRe above covers exactly one shape — a SINGLE letter then digits at
	// a word boundary (Q3, B2). Everything else reached LTS, which skips any
	// character outside a-z and therefore DELETED the digits outright — the same
	// mechanism as the diacritic bug in #25:
	//
	//	Room 3B → "room b"        14M → "em"        H2O → "ho"        mp3 → "mp"
	//
	// The room number, the amount and the 2 in water were simply gone.
	//
	// Runs after every number-FORMAT rule (ordinals, decades, years, time,
	// currency, percent) because those need their digits and letters adjacent:
	// splitting first would turn 1st into "1 st" and break expandOrdinal.
	//
	// ⚠️ This preserves the digits; it does not settle how the letter reads.
	// "14M" in a funding sentence is fourteen MILLION, not fourteen EM, and
	// nothing here knows which — that needs the same decision → and × are
	// waiting on. Preserving information is unambiguous, so it goes in now.
	text = digitLetterRe.ReplaceAllString(text, "$1 $2")
	text = letterDigitRe.ReplaceAllString(text, "$1 $2")

	// Expand decimals: 3.14 → three point one four
	text = decimalRe.ReplaceAllStringFunc(text, expandDecimal)

	// Expand remaining numbers
	text = numberRe.ReplaceAllStringFunc(text, func(s string) string {
		n := parseNumber(s)
		if n < 0 {
			return s
		}
		return numberToWords(n)
	})

	// Clean up double periods from paragraph break insertion (e.g., ".\n\n" → "..")
	text = strings.ReplaceAll(text, "..", ".")

	// Clean up multiple spaces again
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	return text
}

// mixedCaseTerms maps the terms matched by mixedCaseTermRe to spoken forms.
// PhD/pH spell out (letter names match letterNames spellings); Hz units
// expand to the unit word.
var mixedCaseTerms = map[string]string{
	"PhD": "pee aitch dee",
	"pH":  "pee aitch",
	"MHz": "megahertz",
	"kHz": "kilohertz",
	"GHz": "gigahertz",
	"THz": "terahertz",
	"Hz":  "hertz",
}

// letterNames maps uppercase letters to their spoken names for acronym expansion.
var letterNames = map[rune]string{
	'A': "ay", 'B': "bee", 'C': "see", 'D': "dee", 'E': "ee",
	'F': "eff", 'G': "jee", 'H': "aitch", 'I': "eye", 'J': "jay",
	'K': "kay", 'L': "ell", 'M': "em", 'N': "en", 'O': "oh",
	'P': "pee", 'Q': "cue", 'R': "ar", 'S': "ess", 'T': "tee",
	'U': "you", 'V': "vee", 'W': "double you", 'X': "ex", 'Y': "why",
	'Z': "zee",
}

// expandAcronymsInContext expands all-caps tokens, but first rescues ALL-CAPS
// PROSE from being spelled out letter by letter (#41).
//
// Two factors, and both are required:
//
//  1. Is the token inside an all-caps RUN — two or more consecutive all-caps
//     tokens, uninterrupted by a lowercase word or by . ! ?
//  2. If so, is its lowercase form an ordinary English word?
//
// Only tokens satisfying both are read as words. Everything else keeps the
// existing behaviour exactly, which is why isWord==nil is a no-op.
//
// Factor 1 is what makes this safe, and AM is the clearest case:
//
//	"7 AM"           digit neighbour, no run  → "ay em"    ← the meridiem
//	"I AM WATCHING"  caps neighbour, a run    → the verb   ← Corvus's chronicle
//
// The old skip list had to choose one of those globally and chose letters,
// which is exactly why it forbade lowercasing AM at all. Context gets both right.
//
// Measured by @Wayland across 612k words: 0 of the 33 dictionary-word acronyms
// regress under this rule, provided . ! ? break a run.
func expandAcronymsInContext(text string, isWord func(string) bool) string {
	if isWord == nil {
		return acronymRe.ReplaceAllStringFunc(text, expandAcronym)
	}

	// Run detection uses capsRunTokenRe (any length), not acronymRe (2-4), so
	// that a long neighbour like WATCHING can anchor a run it can never be
	// expanded as.
	runToks := capsRunTokenRe.FindAllStringIndex(text, -1)
	inRun := make(map[int]bool, len(runToks))
	for i := 1; i < len(runToks); i++ {
		gap := text[runToks[i-1][1]:runToks[i][0]]
		// Adjacent when nothing but non-letter, non-sentence-ending characters
		// separates them. A lowercase word between two caps tokens means
		// ordinary prose, not a shouted passage.
		if strings.ContainsAny(gap, "abcdefghijklmnopqrstuvwxyz") || sentenceBreakRe.MatchString(gap) {
			continue
		}
		inRun[runToks[i-1][0]] = true
		inRun[runToks[i][0]] = true
	}

	return replaceAllStringFuncIndex(text, acronymRe, func(start int, s string) string {
		lower := strings.ToLower(s)
		if inRun[start] && isWord(lower) {
			return lower
		}
		// A LONE all-caps token of 4+ letters that is a dictionary word is spoken
		// rather than spelled: WARN, INFO, RICO, OSHA. Smiley's ruling (#42) —
		// "spell out initialisms; speak truncations" — WARN is a truncation of
		// warning and has no expansion.
		//
		// ⚠️ His proposed test was dictionary membership ALONE, and measuring it
		// first is why this is length-gated. Membership alone changes 151 tokens
		// over 476 occurrences, led by DA ×70 — District Attorney, and `da` is a
		// dictionary word. That is the regression list from #41 comment 8853
		// exactly, which killed the first version of the rule above.
		//
		// Length is the discriminator because initialisms cluster at 2-3 letters
		// (DA, ER, PA, AG, OS, SEC) — that is how much people abbreviate — while
		// an acronym pronounced as a word needs enough letters to be
		// pronounceable. Of the 33 known regressions, exactly ONE is 4+ letters:
		// FOIA, which is wrong either way (gold gives fˈYə "FY-uh"; "eff oh eye
		// ay" is no better), so it is a wash rather than a regression.
		//
		// ⚑ AM/PM are excluded by LENGTH, structurally, rather than by a
		// deny-list someone has to remember — which is what line 267's comment
		// asked for.
		//
		// Validated three ways: my corpus measurement, Wayland's independent run
		// over all 38 snapshots (439 tokens changed, zero regressions found —
		// the 8 genuine initialisms at 4+ letters are all acronyms-pronounced-
		// as-words), and the historical 33-token list.
		if len(s) >= 4 && isWord(lower) {
			return lower
		}
		return expandAcronym(s)
	})
}

// replaceAllStringFuncIndex is ReplaceAllStringFunc with the match offset, which
// the stdlib does not provide and the run lookup needs.
func replaceAllStringFuncIndex(s string, re *regexp.Regexp, f func(start int, match string) string) string {
	locs := re.FindAllStringIndex(s, -1)
	if locs == nil {
		return s
	}
	var b strings.Builder
	prev := 0
	for _, loc := range locs {
		b.WriteString(s[prev:loc[0]])
		b.WriteString(f(loc[0], s[loc[0]:loc[1]]))
		prev = loc[1]
	}
	b.WriteString(s[prev:])
	return b.String()
}

// expandAcronym expands uppercase letter sequences to spoken letter names.
// CEO → "see ee oh", DNA → "dee en ay".
// Common short words that happen to be uppercase are lowercased instead.
func expandAcronym(s string) string {
	// Skip common uppercase words that aren't acronyms.
	// NOT "AM"/"PM": all-caps AM is the meridiem (or AM radio) and must letter-
	// expand to "ay em"/"pee em" — lowercasing it to the verb "am" makes
	// "7 AM" → "seven əm" (the AM bug). The lowercase verb "am" is never matched
	// by acronymRe (\b[A-Z]{2,4}\b requires all-caps), so it stays untouched.
	switch s {
	case "OK", "IT", "IS", "IN", "ON", "OR", "AN", "AS",
		"AT", "BE", "BY", "DO", "GO", "HE", "IF", "ME", "MY", "NO",
		"OF", "SO", "TO", "UP", "US", "WE":
		return strings.ToLower(s)
	}
	// Acronyms pronounced as words, not spelled out letter-by-letter. Pass them
	// through lowercased so the dictionary supplies the pronunciation
	// (custom_dict: hvac→"H-vac", noc→"knock"). Letter-by-letter is wrong here.
	switch s {
	case "HVAC", "NOC":
		return strings.ToLower(s)
	}
	// Expand each letter to its spoken name
	var result strings.Builder
	for i, r := range s {
		if i > 0 {
			result.WriteRune(' ')
		}
		if name, ok := letterNames[r]; ok {
			result.WriteString(name)
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// expandAcronymPlural expands "LLCs" to "ell ell sees" — the letters spoken,
// with the plural attaching to the LAST letter name rather than trailing as a
// bare consonant.
//
// It routes through expandAcronym so the word-acronym cases keep working:
// NOC → "noc" means NOCs → "nocs", not "en oh sees".
func expandAcronymPlural(base string) string {
	expanded := expandAcronym(base)
	i := strings.LastIndex(expanded, " ")
	return expanded[:i+1] + pluralize(expanded[i+1:])
}

// pluralize applies English plural spelling to a spoken letter name: "see" →
// "sees", but sibilant endings take -es ("ess" → "esses", "ex" → "exes",
// "aitch" → "aitches"). Without this, S and X pluralise to "esss" and "exs",
// which LTS cannot say.
func pluralize(w string) string {
	for _, suf := range []string{"s", "z", "x", "ch", "sh"} {
		if strings.HasSuffix(w, suf) {
			return w + "es"
		}
	}
	return w + "s"
}

func expandAbbreviations(text string) string {
	words := strings.Fields(text)
	for i, w := range words {
		lower := strings.ToLower(w)
		if expanded, ok := abbreviations[lower]; ok {
			words[i] = expanded
			continue
		}
		words[i] = expandNameSuffix(w)
	}
	return strings.Join(words, " ")
}

// expandNameSuffix expands a name-suffix abbreviation wrapped in punctuation,
// keeping the punctuation and any possessive: `"Jr.,` → `"junior,`,
// `Jr.'s` → `junior's`. Anything else is returned unchanged.
func expandNameSuffix(w string) string {
	dot := strings.IndexByte(w, '.')
	if dot < 0 {
		return w
	}
	start := 0
	for start < dot && strings.ContainsRune(`"'([`, rune(w[start])) {
		start++
	}
	key := strings.ToLower(w[start : dot+1])
	if !nameSuffixAbbreviations[key] {
		return w
	}
	rest := w[dot+1:]
	tail := strings.TrimPrefix(rest, "'s")
	if strings.Trim(tail, `,;:!?"')]`) != "" {
		return w
	}
	return w[:start] + abbreviations[key] + rest
}

// expandYear converts 4-digit years to natural speech:
// 1987 → "nineteen eighty seven", 2024 → "twenty twenty four",
// 1900 → "nineteen hundred", 2000 → "two thousand", 2005 → "two thousand five".
func expandYear(s string) string {
	n := parseNumber(s)
	if n < 1000 || n > 2099 {
		return s
	}

	century := n / 100
	remainder := n % 100

	// 2000-2009: "two thousand", "two thousand five"
	if century == 20 && remainder < 10 {
		if remainder == 0 {
			return "two thousand"
		}
		return "two thousand " + numberToWords(remainder)
	}

	// 1900, 1800, etc.: "nineteen hundred"
	if remainder == 0 {
		return numberToWords(century) + " hundred"
	}

	// 1901-1909, etc.: "nineteen oh one"
	if remainder < 10 {
		return numberToWords(century) + " oh " + numberToWords(remainder)
	}

	// 1987 → "nineteen eighty seven"
	return numberToWords(century) + " " + numberToWords(remainder)
}

func expandDecade(s string) string {
	matches := decadeRe.FindStringSubmatch(s)
	if len(matches) < 2 {
		return s
	}
	year := parseNumber(matches[1])
	if year < 0 {
		return s
	}
	// 1990s → nineteen nineties
	century := year / 100
	decade := year % 100
	if decade == 0 {
		return numberToWords(century) + " hundreds"
	}
	decadeWord := numberToWords(decade)
	// Make decade plural: twenty→twenties, thirty→thirties, etc.
	if strings.HasSuffix(decadeWord, "y") {
		decadeWord = decadeWord[:len(decadeWord)-1] + "ies"
	} else {
		decadeWord += "s"
	}
	return numberToWords(century) + " " + decadeWord
}

// expandTimestamp reads HH:MM:SS as machine output — "oh two seventeen thirty
// three" — rather than as a clock time.
//
// Smiley's call (2026-08-16), and the reasoning is about the book rather than
// the format. The `02:17:33` in The Keeper's Door appears four times across two
// sections, and THREE of the four are prose, not the log line: it is a motif
// with its payoff five sections later in s10, not a timestamp format question.
//
// 🛑 The load-bearing requirement is therefore that all four render IDENTICALLY.
// A listener who hears one thing in s5 and another in s10 cannot connect them,
// and connecting them is the point of the passage.
//
// Digits over clock form because the prose itself says "timestamp": it is
// machine output in all four, clock form ("two seventeen and thirty-three
// seconds") reads as time of day and softens what makes the entry eerie, and
// digits survive four repetitions without sounding laboured.
//
// A leading zero is spoken ("oh two"), because dropping it is how a log line
// stops looking like a log line.
func expandTimestamp(s string) string {
	m := timestampRe.FindStringSubmatch(s)
	if len(m) < 5 {
		return s
	}
	parts := make([]string, 0, 4)
	for _, comp := range m[1:4] {
		n := parseNumber(comp)
		if n < 0 {
			return s
		}
		if len(comp) == 2 && comp[0] == '0' {
			parts = append(parts, "oh "+numberToWords(n))
			continue
		}
		parts = append(parts, numberToWords(n))
	}
	// Milliseconds digit by digit: "point eight four seven". They appear only in
	// the fenced occurrence, so the shared prefix above lands first and the match
	// with the three prose occurrences still registers.
	if frac := m[4]; frac != "" {
		parts = append(parts, "point")
		for _, d := range frac {
			parts = append(parts, numberToWords(int(d-'0')))
		}
	}
	return strings.Join(parts, " ")
}

func expandTime(s string) string {
	matches := timeRe.FindStringSubmatch(s)
	if len(matches) < 3 {
		return s
	}
	hour := parseNumber(matches[1])
	minute := parseNumber(matches[2])
	if hour < 0 || minute < 0 {
		return s
	}
	if minute == 0 {
		return numberToWords(hour) + " o'clock"
	}
	if minute < 10 {
		return numberToWords(hour) + " oh " + numberToWords(minute)
	}
	return numberToWords(hour) + " " + numberToWords(minute)
}

func expandOrdinal(s string) string {
	matches := ordinalRe.FindStringSubmatch(s)
	if len(matches) < 2 {
		return s
	}
	n := parseNumber(matches[1])
	if n < 0 {
		return s
	}
	return numberToOrdinal(n)
}

func expandCurrency(s string) string {
	matches := currencyRe.FindStringSubmatch(s)
	if len(matches) < 2 {
		return s
	}
	// Handle cents
	if strings.Contains(matches[1], ".") {
		parts := strings.Split(matches[1], ".")
		dollars := parseNumber(parts[0])
		cents := parseNumber(parts[1])
		if dollars < 0 || cents < 0 {
			return s
		}
		result := numberToWords(dollars) + " dollar"
		if dollars != 1 {
			result += "s"
		}
		if cents > 0 {
			result += " and " + numberToWords(cents) + " cent"
			if cents != 1 {
				result += "s"
			}
		}
		return result
	}
	n := parseNumber(matches[1])
	if n < 0 {
		return s
	}
	result := numberToWords(n) + " dollar"
	if n != 1 {
		result += "s"
	}
	return result
}

func expandDecimal(s string) string {
	matches := decimalRe.FindStringSubmatch(s)
	if len(matches) < 3 {
		return s
	}
	whole := parseNumber(matches[1])
	if whole < 0 {
		return s
	}
	result := numberToWords(whole) + " point"
	// Read decimal digits individually
	for _, d := range matches[2] {
		digit := int(d - '0')
		result += " " + numberToWords(digit)
	}
	return result
}

func parseNumber(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
		if n > 999999999 { // don't handle huge numbers
			return -1
		}
	}
	return n
}

var ones = []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
	"ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
var tens = []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}

func numberToWords(n int) string {
	if n < 0 {
		return "minus " + numberToWords(-n)
	}
	if n < 20 {
		return ones[n]
	}
	if n < 100 {
		result := tens[n/10]
		if n%10 != 0 {
			result += " " + ones[n%10]
		}
		return result
	}
	if n < 1000 {
		result := ones[n/100] + " hundred"
		if n%100 != 0 {
			result += " " + numberToWords(n%100)
		}
		return result
	}
	if n < 1000000 {
		result := numberToWords(n/1000) + " thousand"
		if n%1000 != 0 {
			result += " " + numberToWords(n%1000)
		}
		return result
	}
	result := numberToWords(n/1000000) + " million"
	if n%1000000 != 0 {
		result += " " + numberToWords(n%1000000)
	}
	return result
}

var ordinals = map[int]string{
	1: "first", 2: "second", 3: "third", 4: "fourth", 5: "fifth",
	6: "sixth", 7: "seventh", 8: "eighth", 9: "ninth", 10: "tenth",
	11: "eleventh", 12: "twelfth", 13: "thirteenth", 14: "fourteenth", 15: "fifteenth",
	16: "sixteenth", 17: "seventeenth", 18: "eighteenth", 19: "nineteenth",
	20: "twentieth", 30: "thirtieth", 40: "fortieth", 50: "fiftieth",
	60: "sixtieth", 70: "seventieth", 80: "eightieth", 90: "ninetieth",
}

func numberToOrdinal(n int) string {
	if w, ok := ordinals[n]; ok {
		return w
	}
	if n < 100 {
		o := n % 10
		if o > 0 {
			return tens[n/10] + " " + ordinals[o]
		}
	}
	// Fallback: just use the number word + "th"
	w := numberToWords(n)
	if strings.HasSuffix(w, "y") {
		return w[:len(w)-1] + "ieth"
	}
	if strings.HasSuffix(w, "e") {
		return w[:len(w)-1] + "th"
	}
	if strings.HasSuffix(w, "t") {
		return w + "h"
	}
	return fmt.Sprintf("%s%s", w, "th")
}

// expandRomanNumerals rewrites "Class III" to "Class three" (#57).
//
// @wayland's corpus population is 6 occurrences across 4 books and every one is
// `Class III` in spoken dialogue — an institutional term, not a heading — so the
// reading is unambiguously CARDINAL. Regnal ordinals ("Louis III" → "the Third")
// are a different rule and there are none in the corpus; this leaves them
// untouched rather than guessing, because `Louis` is not a numbering word.
//
// ⚠️ THREE guards, each load-bearing:
//
//	trigger word     the Roman alphabet is a SUBSET of English spelling. Matching
//	                 numerals anywhere converts CD (400, and a compact disc), MIX
//	                 (1009), DID, DIM, MID, LID. CD is the dangerous one — valid,
//	                 canonical, and an ordinary noun.
//	canonical form   rejects IIII, VX, IC — letter runs that are not numbers
//	dictionary word  rejects a real word even after a trigger, so the trigger list
//	                 can be widened later without re-auditing every collision
//
// ⛔ Not ear-confirmed. That it reaches audio is proven — the normalizer runs
// ahead of phonemization — but "Class three" versus "Class the third" is
// @clayton's call, and the clip is queued.
func expandRomanNumerals(text string, isWord func(string) bool) string {
	return romanNumeralRe.ReplaceAllStringFunc(text, func(s string) string {
		m := romanNumeralRe.FindStringSubmatch(s)
		if m == nil {
			return s
		}
		word, numeral := m[1], m[2]
		if !canonicalRomanRe.MatchString(numeral) {
			return s
		}
		if isWord != nil && isWord(strings.ToLower(numeral)) {
			return s
		}
		n := romanToInt(numeral)
		if n <= 0 {
			return s
		}
		return word + " " + numberToWords(n)
	})
}

// romanToInt converts a canonical Roman numeral. Callers must check
// canonicalRomanRe first; this does the arithmetic, not the validation.
func romanToInt(s string) int {
	vals := map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
	up := strings.ToUpper(s)
	total := 0
	for i := 0; i < len(up); i++ {
		v, ok := vals[up[i]]
		if !ok {
			return 0
		}
		// Subtractive pair: a smaller value before a larger one is negated.
		if i+1 < len(up) && v < vals[up[i+1]] {
			total -= v
			continue
		}
		total += v
	}
	return total
}

// expandStreetSt rewrites "Delaney St." to "Delaney street" (#44).
//
// `st.` is in the abbreviations map as "saint" unconditionally, and @wayland
// measured BOTH senses live in the corpus — 27 occurrences over 10 books — so
// one of them is always wrong and no dictionary entry can fix it. The token is
// identical; only position separates them.
//
// ⚠️ NEITHER positional signal works alone, because a sentence boundary
// capitalises a word that carries no information about the sense:
//
//	"next word is capitalised → Saint"   fails on  "Baker St. The door was red."
//	"previous word is capitalised → St"  fails on  "On St. James Street the …"
//
// In the first, `The` is capitalised only because a sentence starts; in the
// second, `On` is. Each rule alone reads its own counterexample backwards.
//
// So BOTH must agree, and this converts only the confident case:
//
//	prev CAPITALISED and next lowercase/absent   → street   ✅ converted here
//	prev lowercase and next CAPITALISED          → saint    ✅ the map already does this
//	both capitalised, or both lowercase          → AMBIGUOUS, left on the default
//
// ⛔ The ambiguous bucket is deliberately untouched, and it is not empty:
// "Baker St. The door" is a street and "On St. James Street" is a saint, both
// land there, and they go OPPOSITE ways. There is nothing to guess with. Same
// posture as #67's 69 ambiguous splits, which were left splitting.
//
// ⚑ Safe whatever the corpus ratio turns out to be: it only ever changes a case
// where both signals agree, so it cannot make an occurrence worse than the
// unconditional "saint" it replaces. @wayland has the split; the fix did not
// need to wait for it.
func expandStreetSt(text string) string {
	return stStreetRe.ReplaceAllStringFunc(text, func(m string) string {
		g := stStreetRe.FindStringSubmatch(m)
		if g == nil {
			return m
		}
		prev, gap, tail, next := g[1], g[2], g[3], g[4]
		if !startsUpper(prev) {
			return m // prev lowercase → Saint, or ambiguous; leave to the map
		}
		if next != "" && startsUpper(next) {
			return m // both capitalised → ambiguous; leave to the map
		}
		return prev + gap + "street" + tail
	})
}

// startsUpper reports whether a word begins with an uppercase letter.
func startsUpper(w string) bool {
	for _, r := range w {
		return unicode.IsUpper(r)
	}
	return false
}
