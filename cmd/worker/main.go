package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"

	"github.com/claytonharbour/proseforge-tts-worker/internal/audio"
	"github.com/claytonharbour/proseforge-tts-worker/internal/handler"
	"github.com/claytonharbour/proseforge-tts-worker/internal/inference"
	"github.com/claytonharbour/proseforge-tts-worker/internal/runpod"
	"github.com/claytonharbour/proseforge-tts-worker/internal/tts"
)

// Set via -ldflags at build time.
var (
	buildTime = "unknown"
	gitHash   = "unknown"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	logCPUBudget(inference.IntraThreads())
	ttsFn, voiceNames, dataFingerprint, modelLoaded := initTTS()

	// RunPod serverless mode — poll for jobs
	if rpConfig := runpod.DetectConfig(); rpConfig != nil {
		log.Printf("RunPod mode: polling %s", rpConfig.GetJobURL)

		// Start HTTP server in background for health checks
		go func() {
			h := handler.New(ttsFn, voiceNames)
			h.SetModelLoaded(modelLoaded)
			mux := http.NewServeMux()
			h.RegisterRoutes(mux)
			handler.RegisterVersion(mux, buildTime, gitHash, dataFingerprint)
			addr := fmt.Sprintf(":%s", port)
			log.Printf("Health check server on %s", addr)
			// NOT Fatalf: the polling loop below is the worker's actual job, and
			// it runs fine without health checks. But the error must be VISIBLE —
			// the Printf above announces the server before the bind, so a port
			// conflict used to produce "Health check server on :8080" and then
			// silence, with RunPod's probes failing for no logged reason.
			// Local mode 20 lines down always checked this; only the path that
			// runs in production discarded it.
			//
			// 🛑 TO RE-TEST THIS, THE PORT MUST BE UNBINDABLE BY CONSTRUCTION —
			// use PORT=99999 ("invalid port"). Three ways of "occupying" a port
			// all failed to induce any error here, and each looked like a clean
			// null result because BOTH arms printed nothing:
			//
			//	blocker with SO_REUSEADDR   → macOS lets the second bind through
			//	blocker on 0.0.0.0 (IPv4)   → Go binds the IPv6 wildcard, no clash
			//	PORT=80 as non-root         → macOS permits :80 unprivileged
			//
			// ⚠️ A specimen that fails to fail is a broken harness, not a passing system.
			// Symmetric arms (0 and 0) read as "the fix changes nothing";
			// they actually meant the failure was never induced. Verify the
			// FAILURE lands, not just that the code under test ran.
			//
			// Red-proof 2026-08-19, PORT=99999: fix present → "health check
			// server stopped: listen tcp: address 99999: invalid port", 12 polls
			// continued. Fix stashed → nothing logged, 13 polls. Binary checked
			// with `strings` on each arm, because the on-disk binary after a
			// stash/build cycle belongs to whichever arm built last.
			if err := http.ListenAndServe(addr, mux); err != nil {
				log.Printf("health check server stopped: %v", err)
			}
		}()

		// Block on polling loop
		w := runpod.New(ttsFn, rpConfig)
		if err := w.Run(context.Background()); err != nil {
			log.Fatalf("runpod worker: %v", err)
		}
		return
	}

	// Local HTTP server mode
	h := handler.New(ttsFn, voiceNames)
	h.SetModelLoaded(modelLoaded)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	handler.RegisterVersion(mux, buildTime, gitHash, dataFingerprint)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("TTS worker listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// stubVoiceNames is the default voice list when using stub TTS.
var stubVoiceNames = []string{"af_sarah"}

// initTTS loads the real TTS engine, or refuses to start (#88).
//
// It used to fall back to stubTTS on any init failure. A stub worker answers
// /health 200 and returns silent audio, and on RunPod every request it serves is
// billed, so a broken image cost money while looking healthy. TTS_ALLOW_STUB=1
// opts back into the silent stub, for local dev without the model files.
//
// The bool reports whether the real engine is live, for /metrics.
func initTTS() (handler.TTSFunc, []string, any, bool) {
	engine, err := loadEngine()
	if err != nil {
		if os.Getenv("TTS_ALLOW_STUB") != "1" {
			log.Fatalf("TTS engine unavailable, refusing to start: %v (set TTS_ALLOW_STUB=1 to serve silent stub audio for local dev)", err)
		}
		log.Printf("TTS engine unavailable: %v; serving SILENT stub audio because TTS_ALLOW_STUB=1", err)
		return stubTTS, stubVoiceNames, nil, false
	}
	return engine.SynthesizeContext, engine.VoiceNames(), engine.DataFingerprint(), true
}

// loadEngine builds the real engine from the model, ONNX runtime and data files,
// and says which of them is missing when it cannot.
func loadEngine() (*tts.Engine, error) {
	modelsDir := envOrDefault("MODELS_DIR", "models")
	dataDir := envOrDefault("DATA_DIR", "data")
	onnxLib := envOrDefault("ONNX_LIBRARY_PATH", findONNXLibrary())

	modelPath := envOrDefault("MODEL_PATH", filepath.Join(modelsDir, "kokoro-v1.0.onnx"))
	voicesPath := filepath.Join(modelsDir, "voices-v1.0.bin")

	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model files not found (%s)", modelPath)
	}
	if onnxLib == "" {
		return nil, errors.New("ONNX Runtime library not found")
	}

	engine, err := tts.New(tts.Config{
		GoldDictPath:    filepath.Join(dataDir, "us_gold.json"),
		SilverDictPath:  filepath.Join(dataDir, "us_silver.json"),
		HomographsPath:  filepath.Join(dataDir, "homographs.json"),
		TokenMapPath:    filepath.Join(dataDir, "token_map.json"),
		ARPABETMapPath:  filepath.Join(dataDir, "arpabet_to_ipa.json"),
		CustomDictPath:  filepath.Join(dataDir, "custom_dict.json"),
		EmphasisPauses:  os.Getenv("TTS_EMPHASIS") != "0", // Gemini-style emphasis pauses; default on, set TTS_EMPHASIS=0 to disable
		ONNXLibraryPath: onnxLib,
		ModelPath:       modelPath,
		VoicesPath:      voicesPath,
		Version:         fmt.Sprintf("ProseForge TTS %s", gitHash),
	})
	if err != nil {
		return nil, fmt.Errorf("engine init: %w", err)
	}

	// Logged as well as served: the fingerprint at startup is what lets a reader
	// of the logs tell WHICH dictionary this process parsed, after the fact.
	fp := engine.DataFingerprint()
	log.Printf("TTS engine initialized (model: %s) gold=%d/%s silver=%d/%s custom=%d/%s homographs=%d/%s vocab=%d/%s",
		modelPath,
		fp.Gold.Entries, fp.Gold.Digest, fp.Silver.Entries, fp.Silver.Digest,
		fp.Custom.Entries, fp.Custom.Digest, fp.Homographs.Entries, fp.Homographs.Digest,
		fp.Vocab.Entries, fp.Vocab.Digest)
	return engine, nil
}

// stubTTS generates a silent WAV of estimated duration.
// Used only when TTS_ALLOW_STUB=1 and the real engine could not load.
func stubTTS(_ context.Context, text, voice, format string, speed float64, _ *bool) ([]byte, float64, error) {
	wordCount := 1
	for _, b := range text {
		if b == ' ' {
			wordCount++
		}
	}
	duration := float64(wordCount) * 0.08 / speed

	numSamples := int(duration * float64(audio.SampleRate))
	if numSamples == 0 {
		numSamples = audio.SampleRate / 10
	}

	samples := make([]float32, numSamples)
	wavData := audio.EncodeWAV(samples)

	if format == "mp3" {
		mp3Data, err := audio.EncodeMP3(wavData, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("MP3 encoding: %w", err)
		}
		duration = audio.Duration(numSamples)
		duration = math.Round(duration*1000) / 1000
		return mp3Data, duration, nil
	}

	duration = audio.Duration(numSamples)
	duration = math.Round(duration*1000) / 1000
	return wavData, duration, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// findONNXLibrary searches common paths for the ONNX Runtime shared library.
func findONNXLibrary() string {
	candidates := []string{
		// macOS (Homebrew)
		"/opt/homebrew/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.dylib",
		// Linux
		"/usr/lib/libonnxruntime.so",
		"/usr/local/lib/libonnxruntime.so",
		"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
