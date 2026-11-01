//go:build functional

package tts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/claytonharbour/proseforge-tts-worker/internal/audio"
)

// #465 / #71: 78 banked pronunciations differ from what the engine says without them,
// but a phoneme difference is not necessarily an audible one — @Sten measured 4 of 6
// banked names as ALREADY correct in a published audiobook.
//
// This closes that gap locally. It rebuilds the PRE-BANK state by removing an entry
// from a fixture custom_dict, synthesises the word in a carrier sentence, and asks
// whisper what it hears. A word the pre-bank engine renders unrecognisably is a
// CONFIRMED defect in any audio rendered before that entry was banked — the same
// finding as @Sten's chapter round-trip, without prod audio or the export tier gate.
//
// ⚠️ STT under-detects by design: whisper recovered "grafana" from a wrong-vowel render
// because language modelling repairs a near-miss a listener would still notice. So a
// PASS here means "still the right word", never "sounds right".
func TestPreBankRenderingIsStillRecognisable(t *testing.T) {
	// @Sten's six, with the answers he measured from the real audiobook. Running the
	// known set first is the point: if this harness disagrees with his rendered result,
	// the harness is wrong, not the audiobook.
	// ⚠️ grafana was `true` here until 2026-08-31 and that was WRONG. @Sten first read
	// it as correct in the April audio; the phoneme strings say otherwise. April's LTS
	// rendered it ɡɹˈAfˈæn, and capital A is /eɪ/ — verified against gold's own day/dˈA,
	// name/nˈAm, grey/ɡɹˈA — so the audio said "GRAY-fan". Whisper's language model
	// repaired it into the right spelling and he reported the repair as ground truth.
	//
	// ⛔ The lesson is the ground truth itself: STT measures whether a language model can
	// RECOVER the intended word, not whether the word was said correctly. Those come
	// apart exactly where the model is confident — low-prior surnames are detected,
	// well-known product names are silently repaired.
	probes := []probe{
		{"mara", true}, {"kai", true}, {"noc", true}, {"grafana", false},
		{"byrne", false}, {"pruitt", false},
	}
	if w := os.Getenv("PREBANK_WORDS"); w != "" {
		probes = nil
		for _, s := range strings.Split(w, ",") {
			probes = append(probes, probe{strings.TrimSpace(s), true})
		}
	}

	dataDir := envOr("MISAKI_DATA_DIR", envOr("DATA_DIR", "../../data"))
	fixture := t.TempDir()
	stripped := stripBank(t, dataDir, fixture, probes)
	t.Logf("fixture custom_dict: %d entries removed", stripped)

	prebank := newEngineWithData(t, fixture)
	model := filepath.Join(envOr("MODELS_DIR", "../../models"), "ggml-base.en.bin")
	if _, err := os.Stat(model); err != nil {
		t.Skipf("SKIPPING: whisper model absent at %s — this test covers NOTHING without it", model)
	}

	agree, disagree := 0, 0
	for _, p := range probes {
		// A carrier sentence, because a bare word gives whisper no language context and
		// would make everything look broken. PREBANK_CARRIER overrides it with %s for the
		// word, so a probe can be run in the sentence it actually appears in — an
		// artificial carrier and the book's own line are not the same test.
		sentence := "The letter from " + p.word + " arrived on Tuesday."
		if c := os.Getenv("PREBANK_CARRIER"); c != "" {
			sentence = strings.ReplaceAll(c, "%s", p.word)
		}
		wav, _, err := prebank.Synthesize(sentence, "af_sarah", "wav", 1.0, nil)
		if err != nil {
			t.Fatalf("synthesising %q: %v", p.word, err)
		}
		heard, err := audio.Transcribe(wav, model)
		if err != nil {
			t.Fatalf("transcribing %q: %v — INSTRUMENT FAILED, not a result", p.word, err)
		}
		held := strings.Contains(strings.ToLower(heard), strings.ToLower(p.word))

		mark := "held"
		if !held {
			mark = "LOST"
		}
		t.Logf("  %-10s pre-bank render -> %-4s  heard: %q", p.word, mark, strings.TrimSpace(heard))

		if os.Getenv("PREBANK_WORDS") == "" {
			if held == p.stenSaysHeld {
				agree++
			} else {
				disagree++
				t.Errorf("%q: harness held=%v, corrected ground truth says held=%v",
					p.word, held, p.stenSaysHeld)
			}
		}
	}

	if os.Getenv("PREBANK_WORDS") == "" {
		t.Logf("agreement with @Sten's rendered result: %d/%d", agree, agree+disagree)
		// 6/6 once the ground truth was corrected. It was recorded as 5/6 for an hour,
		// with grafana logged as an unexplained disagreement — the harness was right and
		// the control was wrong, which is not a possibility I had allowed for.
		if agree < 6 {
			t.Errorf("agreement with the corrected control dropped to %d/%d — it was 6/6",
				agree, agree+disagree)
		}
	}
}

// stripBank writes a fixture data dir identical to the real one except that the probed
// words are removed from custom_dict.json, reproducing the state the engine was in
// before each was banked.
// probe is a word plus the answer @Sten measured from the real audiobook.
type probe struct {
	word         string
	stenSaysHeld bool // did the pre-bank audio still say the right word?
}

func stripBank(t *testing.T, src, dst string, probes []probe) int {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	removed := 0
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		if e.Name() == "custom_dict.json" {
			var d map[string]string
			if err := json.Unmarshal(b, &d); err != nil {
				t.Fatalf("parsing custom_dict: %v", err)
			}
			for _, p := range probes {
				if _, ok := d[p.word]; ok {
					delete(d, p.word)
					removed++
				}
			}
			if removed == 0 {
				t.Fatal("removed nothing — the fixture is identical to the real dict and this " +
					"test would compare the banked reading against itself")
			}
			if b, err = json.Marshal(d); err != nil {
				t.Fatalf("re-encoding custom_dict: %v", err)
			}
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o600); err != nil {
			t.Fatalf("writing fixture %s: %v", e.Name(), err)
		}
	}
	return removed
}

func newEngineWithData(t *testing.T, dataDir string) *Engine {
	t.Helper()
	modelsDir := envOr("MODELS_DIR", "../../models")
	modelPath := envOr("MODEL_PATH", filepath.Join(modelsDir, "kokoro-v1.0.onnx"))
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("SKIPPING: %s absent — run `make dev-models`. This covers NOTHING without it.", modelPath)
	}
	onnx := findONNX(t)
	if onnx == "" {
		t.Skip("SKIPPING: no ONNX Runtime library found. This covers NOTHING without it.")
	}
	e, err := New(Config{
		GoldDictPath:    filepath.Join(dataDir, "us_gold.json"),
		SilverDictPath:  filepath.Join(dataDir, "us_silver.json"),
		HomographsPath:  filepath.Join(dataDir, "homographs.json"),
		TokenMapPath:    filepath.Join(dataDir, "token_map.json"),
		ARPABETMapPath:  filepath.Join(dataDir, "arpabet_to_ipa.json"),
		CustomDictPath:  filepath.Join(dataDir, "custom_dict.json"),
		ONNXLibraryPath: onnx,
		ModelPath:       modelPath,
		VoicesPath:      filepath.Join(modelsDir, "voices-v1.0.bin"),
		Version:         "prebank-stt-test",
	})
	if err != nil {
		t.Fatalf("building engine on %s: %v", dataDir, err)
	}
	t.Cleanup(e.Close)
	return e
}
