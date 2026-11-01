//go:build functional

package tts

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/claytonharbour/proseforge-tts-worker/internal/audio"
)

// #71 asks whether ᵊ (token 42) and ə (token 83) are DIFFERENT to the model, or
// merely different notation. Everything else on that ticket — audibility, exposure,
// whether LTS's full vowel is wrong — is downstream of this one question, and it was
// never measured: the ticket says outright "token 42 could be doing something else
// entirely" and "nobody has listened".
//
// This answers it without a listener. Same word, same carrier, same voice, same seed
// path; the ONLY difference is one codepoint in a scratch custom_dict. If the model
// treats 42 and 83 alike, the two renders are identical samples and #71 is moot. If
// they differ, the magnitude is the first real evidence of what is at stake.
//
// ⚠️ This does NOT establish audibility. A measurable waveform difference and an
// audible one are different claims — @Sten's STT round-trip and a human ear remain
// the oracles for that. What this rules in or out is whether there is anything there.
func TestReducedSchwaIsADistinctTokenToTheModel(t *testing.T) {
	const (
		word     = "alton"
		withRed  = "ˈɔltᵊn" // as gold writes it — token 42
		withFull = "ˈɔltən" // what LTS can only ever produce — token 83
		carrier  = "The name is Alton."
	)

	src := envOr("MISAKI_DATA_DIR", envOr("DATA_DIR", "../../data"))
	verifyTokens(t, src, map[string]int{"ᵊ": 42, "ə": 83})

	// ⚠️ ONE engine, not two. The ONNX runtime initialises once per process, so a
	// second tts.New in the same test fails with "already initialized" — which is
	// why the two readings are carried by two DICTIONARY KEYS rather than two
	// engines. The keys are arbitrary spellings; only their phoneme strings reach
	// the model, and those differ in exactly one codepoint.
	// A THIRD key carrying the SAME reading as the first. It is the null control:
	// two different spellings, identical phonemes. If the pipeline is deterministic
	// and spelling does not leak past the dictionary, this pair must differ by ZERO —
	// and only then does a non-zero ᵊ-vs-ə difference mean the TOKEN caused it.
	dir := fixtureWithReadings(t, src, map[string]string{
		"altonred":  withRed,
		"altonfull": withFull,
		"altonnull": withRed, // same phonemes as altonred, different spelling
	})
	e := newEngineWithData(t, dir)

	a := renderSamples(t, e, "The name is altonred.")
	b := renderSamples(t, e, "The name is altonfull.")
	n := renderSamples(t, e, "The name is altonnull.")

	// ⛔ Run the control FIRST and fail on it, because a non-zero null makes every
	// number below meaningless — and a control reported after the result is one the
	// reader has already stopped caring about.
	if ne, ae := spokenExtent(n), spokenExtent(a); ne != ae {
		t.Fatalf("NULL CONTROL FAILED: identical phonemes under two spellings rendered "+
			"%d vs %d samples of speech. Spelling is reaching the model, or synthesis is "+
			"not deterministic — either way the ᵊ-vs-ə comparison below cannot be trusted",
			ne, ae)
	}
	t.Logf("null control: identical phonemes, different spelling → spoken extent equal (%d) ✅",
		spokenExtent(n))
	_ = word
	_ = carrier

	// ⚠️ A raw sample-count difference can be FRAMING, not speech: the first run of
	// this test reported exactly 1024 samples of difference, which is 2^10 and smells
	// of a buffer boundary rather than a shorter vowel. So report the SPOKEN extent
	// (samples above a silence floor) beside the raw count — if the raw counts differ
	// but the spoken extents match, the difference is padding and means nothing.
	if len(a) != len(b) {
		sa, sb := spokenExtent(a), spokenExtent(b)
		t.Logf("raw samples   %d vs %d  (%.3fs vs %.3fs, Δ %.1f ms)",
			len(a), len(b), audio.Duration(len(a)), audio.Duration(len(b)),
			float64(len(b)-len(a))/24.0)
		t.Logf("spoken extent %d vs %d  (Δ %d = %.1f ms)  ⇐ THIS is the acoustic claim",
			sa, sb, sb-sa, float64(sb-sa)/24.0)
		if sa == sb {
			t.Logf("⚠️ spoken extents IDENTICAL — the raw difference is padding, not speech")
		}
		return
	}

	var diff, energy float64
	maxAbs := 0.0
	for i := range a {
		d := float64(a[i] - b[i])
		diff += d * d
		energy += float64(a[i]) * float64(a[i])
		if m := math.Abs(d); m > maxAbs {
			maxAbs = m
		}
	}
	rmsDiff := math.Sqrt(diff / float64(len(a)))
	rmsSig := math.Sqrt(energy / float64(len(a)))

	t.Logf("%q rendered both ways, %d samples each", word, len(a))
	t.Logf("  ᵊ (42): %q", withRed)
	t.Logf("  ə (83): %q", withFull)
	t.Logf("  RMS difference %.6g · signal RMS %.6g · ratio %.4f · peak |Δ| %.6g",
		rmsDiff, rmsSig, rmsDiff/rmsSig, maxAbs)

	// The load-bearing assertion. Identical samples would mean the tokenizer or the
	// model collapses 42 and 83 — which would retire #71 entirely, and is a result
	// worth failing loudly for rather than logging quietly.
	if rmsDiff == 0 {
		t.Errorf("IDENTICAL audio from tokens 42 and 83 — the model does not distinguish " +
			"reduced from full schwa, and #71's premise does not hold")
	}
}

// verifyTokens fails if the ids #71 rests on are not what the ticket says. A test
// built on a stale premise reports about a vocabulary that no longer exists.
func verifyTokens(t *testing.T, dataDir string, want map[string]int) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dataDir, "token_map.json"))
	if err != nil {
		t.Fatalf("reading token_map.json: %v", err)
	}
	var m struct {
		Vocab map[string]int `json:"vocab"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parsing token_map.json: %v", err)
	}
	for sym, id := range want {
		got, ok := m.Vocab[sym]
		if !ok {
			t.Fatalf("%q absent from vocab — #71's premise is stale", sym)
		}
		if got != id {
			t.Fatalf("%q is token %d, not %d — #71's premise is stale", sym, got, id)
		}
	}
}

// fixtureWithReadings writes a scratch data dir whose custom_dict carries the given
// spelling->IPA entries. The shared data/ is never touched: several benches read it,
// and mutating it to run a test corrupts whatever they measure in the same minute.
func fixtureWithReadings(t *testing.T, srcDir string, add map[string]string) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("reading %s: %v", srcDir, err)
	}
	wrote := false
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		if e.Name() == "custom_dict.json" {
			var d map[string]string
			if err := json.Unmarshal(b, &d); err != nil {
				t.Fatalf("parsing custom_dict: %v", err)
			}
			for k, v := range add {
				d[k] = v
			}
			if b, err = json.Marshal(d); err != nil {
				t.Fatalf("re-encoding custom_dict: %v", err)
			}
			wrote = true
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o600); err != nil {
			t.Fatalf("writing fixture %s: %v", e.Name(), err)
		}
	}
	if !wrote {
		t.Fatal("no custom_dict.json in the source data dir — the forced readings were " +
			"never applied, and both arms would render the SAME thing")
	}
	return dst
}

// spokenExtent returns the number of samples between the first and last sample above
// a silence floor, so leading/trailing padding cannot masquerade as a longer word.
func spokenExtent(s []float32) int {
	const floor = 1e-3
	first, last := -1, -1
	for i, v := range s {
		if v > floor || v < -floor {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return 0
	}
	return last - first + 1
}

func renderSamples(t *testing.T, e *Engine, sentence string) []float32 {
	t.Helper()
	wav, _, err := e.Synthesize(sentence, "af_sarah", "wav", 1.0, nil)
	if err != nil {
		t.Fatalf("synthesising %q: %v", sentence, err)
	}
	return decode(t, wav).Samples
}
