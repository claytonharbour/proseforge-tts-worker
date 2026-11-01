//go:build unit

package phonemizer

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// #42 class 6, count 2 (@wayland asked; measured here instead). The punctuation
// guard in InjectEmphasis only sees punctuation ATTACHED to a word (43b761a), so
// a free-standing "…" or "—" is its own token after strings.Fields and does not
// block the next boundary. The question that decides whether that is a defect is
// not "how many markers are free-standing" but "how close does an injected beat
// actually land to one".
//
// ⚠️ THE TWO MARKERS ARE NOT INTERCHANGEABLE HERE. The excerpt @clayton
// ear-confirmed for #37 carries free-standing em-dashes and ZERO ellipses, so the
// dash case is covered by an ear and the ellipsis case is not. A count that pools
// them reports the covered case as though it settled the uncovered one.
//
// Corpus files are gitignored, so this skips rather than fails when absent — the
// pin below is only meaningful on a run that found prose.
var emphasisCorpusFiles = []string{
	"../../build/The-Hum-of-Silent-Partners.md",
	"../../build/The-Case-of-the-Giggling-Gteaus-Grand-Getaway/The-Case-of-the-Giggling-Gteaus-Grand-Getaway.md",
	"../../build/excerpt-section1.md",
}

func isFreeStandingMarker(tok string) bool {
	return tok == "…" || tok == "—"
}

// injectedBeatGaps returns, for one sentence, the distance in input tokens from
// each INJECTED beat to the nearest pre-existing free-standing marker.
// Distance 0 means the beat landed directly against the author's own marker.
func injectedBeatGaps(sentence string) (gaps []int, injected, freeStanding int) {
	in := strings.Fields(sentence)
	out := strings.Fields(InjectEmphasis(sentence))

	var markerAt []int
	for i, w := range in {
		if isFreeStandingMarker(w) {
			markerAt = append(markerAt, i)
			freeStanding++
		}
	}

	// InjectEmphasis only INSERTS, so walking both in step identifies the
	// insertions — an output token that does not consume an input token.
	// A pre-existing "…" is spelled identically to the marker, which is exactly
	// why this walks positions rather than counting occurrences.
	ii := 0
	for _, w := range out {
		if ii < len(in) && w == in[ii] {
			ii++
			continue
		}
		injected++
		// The beat sits between input tokens ii-1 and ii.
		if len(markerAt) == 0 {
			continue
		}
		best := -1
		for _, m := range markerAt {
			d := m - ii
			if d < 0 {
				d = ii - 1 - m
			}
			if d < 0 {
				d = 0
			}
			if best < 0 || d < best {
				best = d
			}
		}
		gaps = append(gaps, best)
	}
	return gaps, injected, freeStanding
}

// TestEmphasisBeatNeverLandsAgainstAuthorMarker pins the one claim EMPHASIS.md
// makes to @clayton in his own words: "the engine never puts its pause directly
// against the dash". That sentence is the reason the class was accepted as RISK
// rather than fixed, so it is the sentence that has to stay true.
//
// 🛑 A gap of 0 is the worst case and would make EMPHASIS.md wrong — it was shown
// to him as grounds for a verdict, so a regression here retroactively invalidates
// an answer already given, not just a future render.
func TestEmphasisBeatNeverLandsAgainstAuthorMarker(t *testing.T) {
	type row struct {
		file                     string
		sentences, beats, marker int
		dashes, ellipses         int
		hist                     map[int]int
		worst                    []string
	}
	var rows []row
	found := 0

	for _, f := range emphasisCorpusFiles {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		found++
		r := row{file: f, hist: map[int]int{}}
		for _, para := range strings.Split(string(raw), "\n\n") {
			for _, sent := range SplitSentences(para) {
				for _, tok := range strings.Fields(sent) {
					switch tok {
					case "—":
						r.dashes++
					case "…":
						r.ellipses++
					}
				}
				gaps, inj, free := injectedBeatGaps(sent)
				r.sentences++
				r.beats += inj
				r.marker += free
				for _, g := range gaps {
					r.hist[g]++
					if g == 0 {
						r.worst = append(r.worst, strings.TrimSpace(sent))
					}
				}
			}
		}
		rows = append(rows, r)
	}

	if found == 0 {
		t.Skipf("no corpus files present (all %d gitignored paths absent)", len(emphasisCorpusFiles))
	}

	for _, r := range rows {
		keys := make([]int, 0, len(r.hist))
		for k := range r.hist {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		var b strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&b, " %d:%d", k, r.hist[k])
		}
		t.Logf("%s\n  sentences %d · beats %d · free-standing markers %d (— %d, … %d)\n  gap histogram:%s",
			r.file, r.sentences, r.beats, r.marker, r.dashes, r.ellipses, b.String())

		if n := r.hist[0]; n > 0 {
			t.Errorf("%s: %d beat(s) landed DIRECTLY against a free-standing marker — "+
				"EMPHASIS.md tells @clayton this never happens.\n  e.g. %q",
				r.file, n, r.worst[0])
		}
	}
}
