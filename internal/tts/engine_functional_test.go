//go:build functional

// This file is the ONLY functional-only test in the repo, and that is deliberate
// (#91). Before it existed, `-tags=functional` resolved to four untagged
// string/arithmetic tests that also ran under `-tags=unit` — so
// `make dev-test-functional` exited 0 whether or not the TTS engine worked, and
// four places in the docs claimed it required model files. It did not.
//
// ⚠️ The tag matters as much as the assertions. An untagged file compiles under
// EVERY tag set, so a "functional" test in an untagged file is a unit test with a
// label. The Makefile guard asserts that the functional suite contains at least one
// test the unit suite does NOT, which is exactly what this file provides.
package tts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/claytonharbour/proseforge-tts-worker/internal/audio"
)

// envOr mirrors the worker's own resolution so this test reads the same files a
// running worker would, and can be pointed elsewhere without editing the shared tree.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func findONNX(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("ONNX_LIBRARY_PATH"); p != "" {
		return p
	}
	for _, p := range []string{
		"/opt/homebrew/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.dylib",
		"/usr/lib/libonnxruntime.so",
		"/usr/local/lib/libonnxruntime.so",
		"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// newFunctionalEngine builds a REAL engine over the real model, or skips loudly.
//
// ⛔ The skip message names the missing path. A silent skip is how a suite comes to
// report ok while covering nothing, which is the defect this file exists to fix — so
// an absent model must be visible in the output, not merely absent from it.
func newFunctionalEngine(t *testing.T) *Engine {
	t.Helper()

	modelsDir := envOr("MODELS_DIR", "../../models")
	dataDir := envOr("MISAKI_DATA_DIR", envOr("DATA_DIR", "../../data"))
	modelPath := envOr("MODEL_PATH", filepath.Join(modelsDir, "kokoro-v1.0.onnx"))
	voicesPath := filepath.Join(modelsDir, "voices-v1.0.bin")

	for _, p := range []string{modelPath, voicesPath} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("SKIPPING a real engine test: %s is absent (%v). "+
				"Run `make dev-models`. This suite covers NOTHING without it.", p, err)
		}
	}
	onnx := findONNX(t)
	if onnx == "" {
		t.Skip("SKIPPING a real engine test: no ONNX Runtime library found. " +
			"Set ONNX_LIBRARY_PATH. This suite covers NOTHING without it.")
	}

	engine, err := New(Config{
		GoldDictPath:    filepath.Join(dataDir, "us_gold.json"),
		SilverDictPath:  filepath.Join(dataDir, "us_silver.json"),
		HomographsPath:  filepath.Join(dataDir, "homographs.json"),
		TokenMapPath:    filepath.Join(dataDir, "token_map.json"),
		ARPABETMapPath:  filepath.Join(dataDir, "arpabet_to_ipa.json"),
		CustomDictPath:  filepath.Join(dataDir, "custom_dict.json"),
		ONNXLibraryPath: onnx,
		ModelPath:       modelPath,
		VoicesPath:      voicesPath,
		Version:         "functional-test",
	})
	if err != nil {
		t.Fatalf("building a real engine failed: %v", err)
	}
	t.Cleanup(engine.Close)
	return engine
}

// decode writes the WAV bytes to a temp file and reads them back through the
// repo's own decoder, rather than parsing a header by hand here.
func decode(t *testing.T, wav []byte) *audio.WAVFile {
	t.Helper()
	p := filepath.Join(t.TempDir(), "out.wav")
	if err := os.WriteFile(p, wav, 0o600); err != nil {
		t.Fatalf("write temp wav: %v", err)
	}
	f, err := audio.ReadWAVFile(p)
	if err != nil {
		t.Fatalf("the engine produced bytes the repo's own decoder rejects: %v", err)
	}
	return f
}

// rms reports signal energy. This is the assertion that separates a working engine
// from the stub, and it is the whole point of the file.
func rms(samples []float32) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	return sum / float64(len(samples))
}

// TestEngineSynthesizesAudibleSpeech is the end-to-end check the repo did not have.
//
// 🛑 The energy assertion is load-bearing, NOT decoration. cmd/worker falls back to
// stubTTS when the model or ONNX runtime is missing, and stubTTS emits a SILENT WAV
// of estimated duration. So a test asserting only "duration > 0" or "bytes > 44"
// passes against the stub — it would confirm the fallback, report ok, and cover
// nothing. Non-zero energy is what the stub cannot fake.
func TestEngineSynthesizesAudibleSpeech(t *testing.T) {
	engine := newFunctionalEngine(t)

	const phrase = "The quick brown fox jumps over the lazy dog."
	wav, duration, err := engine.Synthesize(phrase, "af_sarah", "wav", 1.0, nil)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	f := decode(t, wav)

	if f.SampleRate != 24000 {
		t.Errorf("sample rate = %d, want 24000", f.SampleRate)
	}
	// A nine-word sentence is seconds, not milliseconds and not a minute. The bounds
	// are deliberately wide: this is a smoke test for "speech happened", and a tight
	// bound would go red on ordinary prosody variation rather than on a defect.
	if duration < 1.0 || duration > 10.0 {
		t.Errorf("duration = %.2fs, want 1-10s for %q", duration, phrase)
	}
	// Reported duration and actual samples must agree, or one of them is invented.
	fromSamples := audio.Duration(len(f.Samples))
	if diff := fromSamples - duration; diff > 0.05 || diff < -0.05 {
		t.Errorf("reported duration %.3fs but the samples say %.3fs — these should agree",
			duration, fromSamples)
	}

	// ⇒ THE assertion. The stub returns silence; a working engine cannot.
	const silenceFloor = 1e-6
	if e := rms(f.Samples); e < silenceFloor {
		t.Errorf("mean square energy %.3g is below the silence floor %.3g — "+
			"this is what a STUB worker produces. The engine did not synthesise speech.",
			e, silenceFloor)
	}
}

// TestEngineIsDeterministicForTheSameInput pins that two identical requests produce
// identical audio. Without this, a change that made synthesis subtly random would
// leave every other assertion here green.
func TestEngineIsDeterministicForTheSameInput(t *testing.T) {
	engine := newFunctionalEngine(t)

	const phrase = "Determinism is a property worth pinning."
	a, _, err := engine.Synthesize(phrase, "af_sarah", "wav", 1.0, nil)
	if err != nil {
		t.Fatalf("first Synthesize: %v", err)
	}
	b, _, err := engine.Synthesize(phrase, "af_sarah", "wav", 1.0, nil)
	if err != nil {
		t.Fatalf("second Synthesize: %v", err)
	}

	if len(a) != len(b) {
		t.Fatalf("same input produced different byte counts: %d vs %d", len(a), len(b))
	}
	// Compare SAMPLES, not the raw bytes: the WAV metadata carries a version string
	// and would mask or manufacture a difference that has nothing to do with audio.
	sa, sb := decode(t, a).Samples, decode(t, b).Samples
	for i := range sa {
		if sa[i] != sb[i] {
			t.Fatalf("same input diverged at sample %d of %d: %v vs %v", i, len(sa), sa[i], sb[i])
		}
	}
}

// TestSpeedChangesDuration exercises the speed parameter through the real engine.
// It is the one request field a caller can set that must visibly change the output,
// so it is the cheapest proof that parameters reach the model at all.
func TestSpeedChangesDuration(t *testing.T) {
	engine := newFunctionalEngine(t)

	const phrase = "Speed should change how long this takes to say."
	_, slow, err := engine.Synthesize(phrase, "af_sarah", "wav", 0.8, nil)
	if err != nil {
		t.Fatalf("slow Synthesize: %v", err)
	}
	_, fast, err := engine.Synthesize(phrase, "af_sarah", "wav", 1.3, nil)
	if err != nil {
		t.Fatalf("fast Synthesize: %v", err)
	}

	if slow <= fast {
		t.Errorf("speed 0.8 gave %.2fs and speed 1.3 gave %.2fs — "+
			"a higher speed must produce SHORTER audio, so the parameter is not reaching the model",
			slow, fast)
	}
}
