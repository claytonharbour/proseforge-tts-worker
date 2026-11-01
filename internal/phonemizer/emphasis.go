package phonemizer

import "strings"

// Emphasis injection — Gemini-parity prosody.
//
// A dramatic narrator inserts a short beat (~330–420ms) at the boundary between a
// function word (article, preposition, auxiliary, pronoun, conjunction) and the
// following CONTENT word — i.e. right before the stressed head of a phrase.
// This pattern was reverse-engineered from Gemini's reference render of the
// section-1 excerpt: it pauses in the gap "for the [350ms] neural", "her [330ms]
// daily", "the [390ms] device", ~once per clause. See
// build/experiments/040-gemini-parity-recheck/FINDINGS.md.
//
// Kokoro has no SSML, but it responds to punctuation tokens: an ellipsis "…"
// (token 10) lengthens the beat at the boundary it sits on. InjectEmphasis places
// "…" at chosen function→content boundaries; the normalizer then spaces it into
// its own pause token and the model does the rest.
//
// 🛑 THE BEAT IS MOSTLY NOT OURS. #37 and the line that used to sit here both
// said the ellipsis "makes the model generate a real ~340ms pause". The 340ms is
// real, reproducible, and measured on Kokoro — but it is the pause's TOTAL
// length, and the model already takes most of it there unprompted. Re-measured
// 2026-08-20 against the experiment's own artifacts, which were kept:
//
//	build/experiments/040-gemini-parity-recheck/ab-pause-tokens/
//	  ab_A_plain.wav      1.17s → 1.43s   256 ms   ← NO MARKER AT ALL
//	  ab_C_ellipsis.wav   1.17s → 1.51s   341 ms   ← +85 ms
//	  ab_B_emdash.wav     1.17s → 1.33s   165 ms   ← the em-dash SHORTENS it
//
// Same start sample, same sentence, same instrument. The marker's contribution is
// +85ms of silence and +107ms of speech, not 340ms of either.
//
// ⚑ The A/B RENDERED a plain arm and left it out of its summary table, which
// compared the marker arms against our own 50ms waveform splice instead.
// A measurement taken against the wrong control reads exactly like a strong one.
//
// 📌 In flowing prose it is smaller still: #75 measures +64ms per beat with the
// silence total UNCHANGED, because the pause the marker lands on already exists
// and absorbs it. FINDINGS.md line 92 saw the deflation ("~170ms in flowing
// paragraph context, not the 340ms it gives in isolation") and #37 quoted the
// isolation figure without it.
//
// ⛔ So do not reach for "—" as a stronger marker: measured, it shortens the beat
// to 165ms and spawns a second pause later in the sentence.
//
// ✅ None of this touches placement, which IS verified — the heuristic
// independently reproduces Gemini's own beat positions on the excerpt, and
// @clayton approved the result by ear. The feature works. Its documented
// magnitude was five times its real one.

// functionWords is the closed-class set that cliticizes onto a following content
// word. A word NOT in this set (and not a number) is treated as a content word.
var functionWords = map[string]bool{
	// articles / determiners / possessives
	"the": true, "a": true, "an": true, "this": true, "that": true, "these": true,
	"those": true, "her": true, "his": true, "its": true, "their": true, "our": true,
	"my": true, "your": true, "some": true, "any": true, "no": true, "each": true,
	// prepositions
	"of": true, "for": true, "to": true, "in": true, "on": true, "at": true, "by": true,
	"with": true, "from": true, "into": true, "onto": true, "over": true, "under": true,
	"between": true, "through": true, "about": true, "against": true, "toward": true,
	// pronouns
	"it": true, "he": true, "she": true, "they": true, "we": true, "i": true, "you": true,
	"who": true, "which": true, "what": true,
	// auxiliaries / modals / copula
	"is": true, "was": true, "were": true, "are": true, "am": true, "be": true, "been": true,
	"being": true, "had": true, "has": true, "have": true, "do": true, "does": true, "did": true,
	"could": true, "would": true, "should": true, "might": true, "may": true, "can": true,
	"will": true, "shall": true, "must": true,
	// conjunctions / particles
	"and": true, "or": true, "but": true, "so": true, "yet": true, "nor": true, "as": true,
	"than": true, "if": true, "then": true, "not": true,
}

const (
	// emphasisMarker is the ellipsis pause token (Kokoro token 10). It ADDS ~85ms
	// to whatever pause already sits at that boundary — not the ~340ms this line
	// used to claim, which is the resulting pause's total. See the header.
	emphasisMarker = "…"
	// minEmphasisSentenceWords skips short sentences — no room for drama.
	minEmphasisSentenceWords = 8
	// minContentWordLen prefers weightier phrase heads (Gemini's strongest beats
	// land before longer/rarer content words).
	minContentWordLen = 5
	// minEmphasisSpacing is the minimum word gap between two injected beats, so we
	// match Gemini's ~once-per-clause cadence instead of stammering.
	minEmphasisSpacing = 5
	// edgeGuard keeps beats away from the first/last words of a sentence.
	edgeGuard = 2
)

// ⚑ THE MARKER DOES REACH THE MODEL, and `diagnose` cannot show you that.
// Measured 2026-08-20 on the shipping path:
//
//	Phonemize(injected)  → "ðə sˈɪɡnᵊl fɹʌm ðə … tˈWəɹ …"   the … survives
//	tokens               → 59 injected vs 57 plain           +2
//	DiagnoseText rows    → NO ROW for the marker at all
//
// 🛑 DiagnoseText emits rows for WORDS, and standalone punctuation is not one —
// verified across "…", "—", ";" and "...", none of which produce a row.
//
// the diagnostic is blind to exactly the token this file inserts.
//
// So a measurement of emphasis beats taken through `diagnose` would report that
// nothing was injected.
//
// That is why the count-2 measurement below walks strings.Fields over
// InjectEmphasis's own output instead of grepping the diagnostic's rows.
//
// InjectEmphasis returns the sentence with "…" pause markers inserted at
// function→content boundaries, subject to guardrails. It is deterministic and a
// no-op for short sentences or sentences with no qualifying boundary.
func InjectEmphasis(sentence string) string {
	words := strings.Fields(sentence)
	if len(words) < minEmphasisSentenceWords {
		return sentence
	}

	// A boundary is a candidate when word[i] is a function word, word[i+1] is a
	// weighty content word, and neither side already carries punctuation (the
	// model already pauses at existing punctuation).
	//
	// ⚠️ THE PUNCTUATION GUARD ONLY SEES PUNCTUATION ATTACHED TO A WORD. A
	// free-standing "…" or "—" is its own token after strings.Fields, so it does
	// NOT block the next boundary, and minEmphasisSpacing never counts it —
	// lastPick tracks injected beats only. #37 describes the guardrail as "skip
	// existing punctuation", which is broader than what this implements.
	//
	// So a beat can land 2 tokens from an existing marker despite a 5-word
	// minimum. Measured on the excerpt @clayton ear-confirmed for #37: 13 beats
	// injected, 4 of them closer to an existing marker than minEmphasisSpacing —
	// gaps of 2, 3, 3 and 4.
	//
	//	typing a … quarterly report — revenue      gap 3
	//	left ear — a … sound so                    gap 2
	//
	// ⚑ That is why this is documented rather than fixed: the condition was
	// already present in the audio he approved, so it is RISK, not defect —
	// the #46 standard. Raised as class 6 of #42 in my own words as "a defect I
	// may have shipped"; measuring it found the ear had already covered it.
	//
	// ⛔ The excerpt carries 5 free-standing em-dashes and ZERO ellipses, so the dash case is ear-covered and the ellipsis case is not.
	//
	// 🛑 AND THE SENTENCE THAT USED TO SIT HERE SIZED THAT GAP WITH THE WRONG
	// NUMBER. It read "a book dense with '…' (321 markers in one, per #42)". The
	// 321 is EM-DASHES — EMPHASIS.md, line 11, the ear-COVERED marker. I attached
	// the covered case's population to the uncovered one, which made the caveat
	// read as a sized risk. @wayland's rule, and I wrote the example:
	// a caveat reads as diligence, which is why an invented one outlives a wrong number.
	//
	// ⚑ And the line above was split across a wrap on its first draft, so the
	// findability grep for it returned 0 — the exact break CLAUDE.md warns about,
	// in the commit adding a rule about wrong numbers. Keep it on one line.
	//
	// Measured 2026-08-19 over 1,940 sentences of real prose
	// (TestEmphasisBeatNeverLandsAgainstAuthorMarker):
	//
	//	Hum of Silent Partners   718 sent · 531 beats · 98 free-standing — · 0 …
	//	Giggling Gateaux        1222 sent · 1479 beats ·  0 free-standing · 0 …
	//
	// So the unheard case has NO MEASURED POPULATION: zero free-standing ellipses
	// in either book. The exposed marker is the em-dash, and that is the one an
	// ear already cleared. Still worth a clip if a "…"-dense book ever appears —
	// but it is not a pending risk, and it was never 321 of anything.
	//
	// 📌 Gap convention differs from the four gaps listed above: those count
	// positions between markers (min 2), the test counts intervening tokens
	// (min 1). Same measurement, off by one — do not read the test's 1 as a
	// regression against this comment's 2.
	lastPick := -minEmphasisSpacing
	out := make([]string, 0, len(words)+4)

	for i := 0; i < len(words); i++ {
		out = append(out, words[i])

		// The beat goes BEFORE word[i+1] (the content head), so evaluate the gap
		// between word[i] (function) and word[i+1] (content).
		if i < edgeGuard || i >= len(words)-1-edgeGuard {
			continue
		}
		if i-lastPick < minEmphasisSpacing {
			continue
		}
		if hasEdgePunctuation(words[i]) || hasEdgePunctuation(words[i+1]) {
			continue
		}
		fw := strings.ToLower(stripWordPunctuation(words[i]))
		head := stripWordPunctuation(words[i+1])
		if !functionWords[fw] || containsDigit(head) {
			continue
		}
		if functionWords[strings.ToLower(head)] || len([]rune(head)) < minContentWordLen {
			continue
		}
		out = append(out, emphasisMarker)
		lastPick = i
	}

	return strings.Join(out, " ")
}

// hasEdgePunctuation reports whether the word begins or ends with punctuation
// that already induces (or blocks) a pause.
func hasEdgePunctuation(word string) bool {
	r := []rune(word)
	if len(r) == 0 {
		return false
	}
	return isPunctuation(r[0]) || isPunctuation(r[len(r)-1])
}

// containsDigit reports whether the string holds any ASCII digit (numbers get
// expanded by the normalizer and shouldn't anchor an emphasis beat).
func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}
