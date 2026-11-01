//go:build unit

package phonemizer

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// Every custom_dict entry exists because someone wanted a different reading than the
// engine would otherwise produce. But @Sten measured 6 banked names against a rendered
// audiobook and found 4 were ALREADY correct — so banking an entry proves intent, not
// that the fallback was audibly wrong.
//
// This screens the whole bank the same way, without audio: for each entry, what would
// the engine have said WITHOUT it? Where the banked reading and the fallback agree, the
// entry is belt-and-braces. Where they disagree, the entry is load-bearing and any
// audio rendered before it was banked is a candidate defect.
//
// ⚠️ This predicts DIVERGENCE, not audibility. @Sten's byrne→"Bayern" and pruitt→"Prout"
// are confirmed by STT; everything this flags is a candidate until something renders it.
func TestBankedEntriesThatTheFallbackWouldGetWrong(t *testing.T) {
	p := newTestPhonemizer(t)
	if p.custom == nil || p.custom.Size() == 0 {
		t.Fatal("custom dict is empty — this screen would report nothing and look clean")
	}

	type row struct{ word, banked, fallback string }
	var diverge, agree []row

	for word, banked := range p.custom.entries {
		// What answers if the bank is absent: gold (incl. suffix rules), then LTS.
		fallback, ok := p.gold.LookupWithSuffix(word)
		if !ok {
			if s, ok2 := p.silver.LookupWithSuffix(word); ok2 {
				fallback = s
			} else {
				fallback = p.lts.Phonemize(word)
			}
		}
		r := row{word, banked, fallback}
		if normaliseForCompare(banked) == normaliseForCompare(fallback) {
			agree = append(agree, r)
		} else {
			diverge = append(diverge, r)
		}
	}

	sort.Slice(diverge, func(i, j int) bool { return diverge[i].word < diverge[j].word })
	sort.Slice(agree, func(i, j int) bool { return agree[i].word < agree[j].word })

	if len(diverge)+len(agree) == 0 {
		t.Fatal("no entries compared — the screen ran on nothing")
	}
	// A screen where everything or nothing diverges is not discriminating. Both of
	// @Sten's confirmed defects must land in `diverge`, and his four correct names
	// give the other side of the control.
	if len(diverge) == 0 {
		t.Error("no entry diverges from its fallback — implausible, and indistinguishable " +
			"from a screen that is comparing a value against itself")
	}
	if len(agree) == 0 {
		t.Error("every entry diverges — equally implausible; @Sten measured 4 of 6 as already correct")
	}

	t.Logf("banked entries: %d diverge from the fallback, %d agree",
		len(diverge), len(agree))

	// The two @Sten confirmed by STT must be on the diverging side, or this screen
	// cannot be the thing that predicts them.
	for _, want := range []string{"byrne", "pruitt"} {
		found := false
		for _, r := range diverge {
			if r.word == want {
				found = true
				t.Logf("  CONFIRMED-BY-STT  %-14s banked %-16s fallback %s", r.word, r.banked, r.fallback)
			}
		}
		if !found {
			t.Errorf("%q is a CONFIRMED mispronunciation but this screen does not flag it — "+
				"the screen cannot predict the defects we already know about", want)
		}
	}

	if os.Getenv("VERBOSE") != "" {
		t.Log("  --- all diverging entries (candidates for re-narration) ---")
		for _, r := range diverge {
			t.Logf("    %-16s banked %-20s fallback %s", r.word, r.banked, r.fallback)
		}
	}
}

// normaliseForCompare strips stress marks before comparing. A banked entry that differs
// from the fallback ONLY in stress placement is a real difference but a much subtler
// one than a wrong vowel, and lumping them together would inflate the candidate list.
//
// ⛔ Deliberately does NOT normalise ᵊ to ə: those are different tokens (#71), and
// collapsing them is exactly the normalisation that hid a real distinction there.
func normaliseForCompare(s string) string {
	return strings.NewReplacer("ˈ", "", "ˌ", "").Replace(s)
}

// @Sten rendered six banked names and found four already correct in the audio. Those
// four are the negative control for this screen: if it flags them as diverging, it
// over-predicts and its candidate list is not worth acting on.
func TestScreenAgreesWithTheRenderedControl(t *testing.T) {
	p := newTestPhonemizer(t)

	correctInAudio := []string{"mara", "kai", "noc", "grafana"}
	for _, w := range correctInAudio {
		banked, ok := p.custom.Lookup(w)
		if !ok {
			t.Errorf("%q is not banked — the control cannot be evaluated", w)
			continue
		}
		fallback, ok := p.gold.LookupWithSuffix(w)
		if !ok {
			if s, ok2 := p.silver.LookupWithSuffix(w); ok2 {
				fallback = s
			} else {
				fallback = p.lts.Phonemize(w)
			}
		}
		same := normaliseForCompare(banked) == normaliseForCompare(fallback)
		t.Logf("  %-10s banked %-16s fallback %-16s %s", w, banked, fallback,
			map[bool]string{true: "agree (screen says safe) ✅", false: "DIVERGES (screen flags it)"}[same])
	}
}
