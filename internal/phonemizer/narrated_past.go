package phonemizer

import "strings"

// ResolveHint carries what a homograph needs to know from OUTSIDE its own sentence (#112).
// The resolver otherwise sees one sentence, a ±3-word window (#47), which cannot tell
//
//	"Open the manual. Read the first page."      an instruction: "reed"
//	"He opened the file. Read the first page."   narration, subject dropped: "red"
//
// because the evidence is in the previous sentence.
type ResolveHint struct {
	// PrevSentenceNarratedPast is true when the previous sentence in the same paragraph is
	// plain past-tense narration (see narratedPastSentence).
	PrevSentenceNarratedPast bool
}

// sentenceHints computes the hint for each sentence of ONE paragraph. TokenizeText (synthesis) and
// DiagnoseText (audit) both call it, so the audit cannot drift from what is spoken (#47).
// It never looks across a paragraph boundary: the caller passes one paragraph's sentences.
func sentenceHints(sentences []string) []ResolveHint {
	hints := make([]ResolveHint, len(sentences))
	for i := 1; i < len(sentences); i++ {
		prev := sentences[i-1]
		// A run of subject-less fragments is one stretch of narration: "He picked up the page.
		// Read the first line. Read the second line." The second "Read" follows a sentence that
		// has no past verb of its own, but that sentence was itself narrated "Read" (so the first
		// one took the narrated reading), and the narration carries on through it.
		continued := hints[i-1].PrevSentenceNarratedPast && startsWithRead(prev)
		hints[i].PrevSentenceNarratedPast = narratedPastSentence(prev) || continued
	}
	return hints
}

// startsWithRead reports whether the sentence's first word is "read" (any case), the word that
// opts in to the narrated reading.
func startsWithRead(sent string) bool {
	f := strings.Fields(sent)
	return len(f) > 0 && strings.EqualFold(strings.Trim(f[0], `.,;:!?'’`), "read")
}

// narratedPastSentence reports whether sent reads as third-person past narration: it has no
// quotation marks (so it is not dialogue, and not a sentence cut out of one), no colon (a colon
// introduces reported speech or instructions: "...told the quill: this one is here. Read it
// directly." is an instruction, found on real prose as the one false positive in 11), no perfect auxiliary
// (had/has/have: "She had written you a letter. Read it first." is not an event followed by a
// fragment, and #47's own guard sentences read it as an instruction), and it contains a
// simple-past ACTION verb. Auxiliaries such as was/were/did are not evidence on their own.
// It is deliberately a small, auditable heuristic and not a tagger: a false "no" leaves today's
// imperative reading, which is the behaviour that existed before.
func narratedPastSentence(sent string) bool {
	if strings.ContainsAny(sent, "\"“”:") {
		return false
	}
	words := strings.Fields(sent)
	for _, w := range words {
		switch strings.ToLower(strings.Trim(w, `.,;:!?'’`)) {
		case "had", "has", "have", "having", "hadn't", "hasn't", "haven't":
			return false
		}
	}
	for i, w := range words {
		lw := strings.ToLower(strings.TrimFunc(w, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '\'' && r != '’'
		}))
		if lw == "" {
			continue
		}
		if lw == "read" {
			// "Maren read it twice." is past; "to read", "will read", "please read" are not.
			if i > 0 && !infinitiveMarker[strings.ToLower(strings.Trim(words[i-1], `.,;:!?'’`))] {
				return true
			}
			continue
		}
		if irregularPast[lw] {
			return true
		}
		if len(lw) >= 4 && strings.HasSuffix(lw, "ed") && !notPastEd[lw] {
			return true
		}
	}
	return false
}

// infinitiveMarker are the words after which "read" is not a past form.
var infinitiveMarker = map[string]bool{
	"to": true, "will": true, "can": true, "could": true, "should": true, "would": true,
	"shall": true, "may": true, "might": true, "must": true, "please": true, "not": true,
	"do": true, "does": true, "did": true, "don't": true, "didn't": true, "let": true,
	"and": true, "then": true, "i'll": true, "we'll": true, "you'll": true,
}

// irregularPast are common irregular past forms of ACTION verbs that narration uses. Auxiliaries
// (was, were, had, did) are left out on purpose: they do not show an event. "read" is handled
// separately because it is itself the ambiguous word.
var irregularPast = map[string]bool{
	"said": true, "went": true, "came": true,
	"took": true, "got": true, "made": true, "saw": true, "knew": true, "thought": true, "found": true,
	"gave": true, "told": true, "felt": true, "left": true, "kept": true, "stood": true, "sat": true,
	"ran": true, "began": true, "put": true, "set": true, "heard": true, "held": true, "brought": true,
	"wrote": true, "spoke": true, "broke": true, "chose": true, "fell": true, "rose": true, "drew": true,
	"threw": true, "flew": true, "grew": true, "led": true, "lay": true, "met": true, "paid": true,
	"sent": true, "spent": true, "built": true, "caught": true, "taught": true, "bought": true,
	"fought": true, "sought": true, "slept": true, "swept": true, "wept": true, "woke": true,
	"wore": true, "tore": true, "shook": true, "stole": true, "sang": true, "swam": true, "rang": true,
	"hung": true, "dug": true, "struck": true, "lit": true, "shut": true, "cut": true, "hit": true,
}

// notPastEd are words ending in "ed" that are not past-tense verbs (or are too ambiguous to count).
var notPastEd = map[string]bool{
	"need": true, "feed": true, "seed": true, "speed": true, "indeed": true, "hundred": true,
	"bleed": true, "breed": true, "creed": true, "greed": true, "proceed": true, "succeed": true,
	"exceed": true, "reed": true, "weed": true, "heed": true, "shed": true, "sled": true, "bed": true,
	"naked": true, "wicked": true, "sacred": true, "kindred": true, "hatred": true, "ragged": true,
	"tired": true, "scared": true, "bored": true, "wired": true, "red": true, "led": true,
}
