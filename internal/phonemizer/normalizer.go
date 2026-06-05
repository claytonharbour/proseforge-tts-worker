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

	// alphaNumRe matches letter+digit combos like Q3, B2, F16
	alphaNumRe = regexp.MustCompile(`\b([A-Za-z])(\d+)\b`)
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
	// yearRe matches 4-digit years (1100-2099) preceded by context words that
	// signal a year rather than a quantity. Captures: [1]=context word, [2]=year.
	yearRe = regexp.MustCompile(`(?i)\b(in|of|from|since|by|around|circa|year|before|after|until|during|early|late|mid)\s+(1[1-9]\d{2}|20\d{2})\b`)
	// decadeRe matches 1990s, 1980s, etc.
	decadeRe = regexp.MustCompile(`\b(\d{4})s\b`)
	// abbreviationDotRe matches U.S.A., Dr., etc.
	abbreviationDotRe = regexp.MustCompile(`\b([A-Z]\.){2,}`)
	// acronymRe matches 2-4 uppercase letter sequences like CEO, FTC, HR, AR
	acronymRe = regexp.MustCompile(`\b[A-Z]{2,4}\b`)
	// slashAcronymRe matches patterns like A/B
	slashAcronymRe = regexp.MustCompile(`\b([A-Z])/([A-Z])\b`)
	// markdownHeadingRe matches # heading markers at line start
	markdownHeadingRe = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	// paragraphBreakRe matches blank lines (paragraph boundaries)
	paragraphBreakRe = regexp.MustCompile(`\n\s*\n`)
)

// Common abbreviation expansions
var abbreviations = map[string]string{
	"dr.":   "doctor",
	"mr.":   "mister",
	"mrs.":  "misses",
	"ms.":   "miss",
	"prof.": "professor",
	"sr.":   "senior",
	"jr.":   "junior",
	"st.":   "saint",
	"ave.":  "avenue",
	"blvd.": "boulevard",
	"dept.": "department",
	"govt.": "government",
	"gen.":  "general",
	"sgt.":  "sergeant",
	"cpl.":  "corporal",
	"pvt.":  "private",
	"lt.":   "lieutenant",
	"capt.": "captain",
	"maj.":  "major",
	"col.":  "colonel",
	"etc.":  "etcetera",
	"vs.":   "versus",
	"approx.": "approximately",
	"est.":  "established",
	"vol.":  "volume",
	"no.":   "number",
	"fig.":  "figure",
	"rev.":  "reverend",
}

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
func NormalizeText(text string) string {
	// Strip markdown formatting (must happen before sentence splitting to avoid
	// false sentence boundaries inside *italic* or **bold** blocks)
	text = markdownHeadingRe.ReplaceAllString(text, "")
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
	text = expandAbbreviations(text)

	// Expand dotted abbreviations like U.S.A. → U S A
	text = abbreviationDotRe.ReplaceAllStringFunc(text, func(s string) string {
		return strings.ReplaceAll(strings.TrimSuffix(s, "."), ".", " ")
	})

	// Expand slash patterns like A/B → A B
	text = slashAcronymRe.ReplaceAllString(text, "${1} ${2}")

	// Expand uppercase acronyms (CEO → C E O, FTC → F T C)
	// Must come after abbreviation dot handling to avoid double-expansion
	text = acronymRe.ReplaceAllStringFunc(text, expandAcronym)

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

// letterNames maps uppercase letters to their spoken names for acronym expansion.
var letterNames = map[rune]string{
	'A': "ay", 'B': "bee", 'C': "see", 'D': "dee", 'E': "ee",
	'F': "eff", 'G': "jee", 'H': "aitch", 'I': "eye", 'J': "jay",
	'K': "kay", 'L': "ell", 'M': "em", 'N': "en", 'O': "oh",
	'P': "pee", 'Q': "cue", 'R': "ar", 'S': "ess", 'T': "tee",
	'U': "you", 'V': "vee", 'W': "double you", 'X': "ex", 'Y': "why",
	'Z': "zee",
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

func expandAbbreviations(text string) string {
	words := strings.Fields(text)
	for i, w := range words {
		lower := strings.ToLower(w)
		if expanded, ok := abbreviations[lower]; ok {
			words[i] = expanded
		}
	}
	return strings.Join(words, " ")
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
