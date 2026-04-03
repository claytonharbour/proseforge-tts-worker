package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"

	"github.com/claytonharbour/proseforge-tts-worker/internal/audio"
	"github.com/claytonharbour/proseforge-tts-worker/internal/handler"
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

	ttsFn, voiceNames := initTTS()

	// RunPod serverless mode — poll for jobs
	if rpConfig := runpod.DetectConfig(); rpConfig != nil {
		log.Printf("RunPod mode: polling %s", rpConfig.GetJobURL)

		// Start HTTP server in background for health checks
		go func() {
			h := handler.New(ttsFn, voiceNames)
			mux := http.NewServeMux()
			h.RegisterRoutes(mux)
			handler.RegisterVersion(mux, buildTime, gitHash)
			addr := fmt.Sprintf(":%s", port)
			log.Printf("Health check server on %s", addr)
			http.ListenAndServe(addr, mux)
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
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	handler.RegisterVersion(mux, buildTime, gitHash)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("TTS worker listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// stubVoiceNames is the default voice list when using stub TTS.
var stubVoiceNames = []string{"af_sarah"}

// initTTS tries to initialize the full TTS engine. Falls back to stub if
// model files or ONNX runtime are not available.
func initTTS() (handler.TTSFunc, []string) {
	modelsDir := envOrDefault("MODELS_DIR", "models")
	dataDir := envOrDefault("DATA_DIR", "data")
	onnxLib := envOrDefault("ONNX_LIBRARY_PATH", findONNXLibrary())

	modelPath := envOrDefault("MODEL_PATH", filepath.Join(modelsDir, "kokoro-v1.0.onnx"))
	voicesPath := filepath.Join(modelsDir, "voices-v1.0.bin")

	// Check if model files exist
	if _, err := os.Stat(modelPath); err != nil {
		log.Printf("Model files not found (%s), using stub TTS", modelPath)
		return stubTTS, stubVoiceNames
	}
	if onnxLib == "" {
		log.Printf("ONNX Runtime library not found, using stub TTS")
		return stubTTS, stubVoiceNames
	}

	engine, err := tts.New(tts.Config{
		GoldDictPath:    filepath.Join(dataDir, "us_gold.json"),
		SilverDictPath:  filepath.Join(dataDir, "us_silver.json"),
		HomographsPath:  filepath.Join(dataDir, "homographs.json"),
		TokenMapPath:    filepath.Join(dataDir, "token_map.json"),
		ARPABETMapPath:  filepath.Join(dataDir, "arpabet_to_ipa.json"),
		CustomDictPath:  filepath.Join(dataDir, "custom_dict.json"),
		ONNXLibraryPath: onnxLib,
		ModelPath:       modelPath,
		VoicesPath:      voicesPath,
		Version:         fmt.Sprintf("ProseForge TTS %s", gitHash),
	})
	if err != nil {
		log.Printf("Failed to initialize TTS engine: %v, using stub", err)
		return stubTTS, stubVoiceNames
	}

	log.Printf("TTS engine initialized (model: %s)", modelPath)
	return engine.Synthesize, engine.VoiceNames()
}

// stubTTS generates a silent WAV of estimated duration.
// Used when model files or ONNX runtime are not available.
func stubTTS(text, voice, format string, speed float64) ([]byte, float64, error) {
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
