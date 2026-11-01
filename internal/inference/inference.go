package inference

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// Engine wraps the ONNX runtime session for Kokoro TTS inference.
type Engine struct {
	session *ort.DynamicAdvancedSession
	mu      sync.Mutex
}

// NewEngine loads the Kokoro ONNX model and returns an inference engine.
// libraryPath is the path to the ONNX Runtime shared library (libonnxruntime.so/.dylib).
// modelPath is the path to the kokoro-v1.0.onnx model file.
func NewEngine(libraryPath, modelPath string) (*Engine, error) {
	ort.SetSharedLibraryPath(libraryPath)
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("init ONNX environment: %w", err)
	}

	opts, err := sessionOptions()
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, err
	}
	if opts != nil {
		defer func() { _ = opts.Destroy() }()
	}
	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{"tokens", "style", "speed"},
		[]string{"audio"},
		opts, // nil = ONNX Runtime defaults
	)
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &Engine{session: session}, nil
}

// intraThreadsEnv names the optional override for ONNX Runtime's intra-op thread count (#113).
const intraThreadsEnv = "TTS_ORT_INTRA_THREADS"

// intraThreadsFromEnv reads the override. Unset, empty, non-numeric or below 1 all mean "use the
// ONNX Runtime default" (0), so a bad value can never make the worker slower than before.
func intraThreadsFromEnv() int {
	n, err := strconv.Atoi(os.Getenv(intraThreadsEnv))
	if err != nil || n < 1 {
		return 0
	}
	return n
}

// IntraThreads is the intra-op thread count in effect (0 = the ONNX Runtime default), for logging.
func IntraThreads() int { return intraThreadsFromEnv() }

// sessionOptions returns nil (ONNX Runtime defaults) unless TTS_ORT_INTRA_THREADS is set.
// Measured on an M2 Pro (8 performance + 4 efficiency cores), quiet machine, 268 words: default
// 5.8x real time, 8 threads 7.0x, 12 threads 5.0x, 4 threads 4.6x (#113).
//
// ⚠️ The OUTPUT DIFFERS from the default. On 210 s of narration about a third of the samples
// differ (up to roughly 5,000 to 6,800 on a 16-bit scale); loudness, the average spectrum (0.16 dB)
// and speech-to-text output matched, but nobody has listened. Each setting is repeatable against
// itself. Treat turning it on as a change to the audio, not as a free speed-up (#113 has the
// numbers and the A/B method).
//
// The best value is machine-specific, which is why this is an opt-in setting and not a new
// default: RunPod and other hardware keep today's behaviour.
func sessionOptions() (*ort.SessionOptions, error) {
	n := intraThreadsFromEnv()
	if n == 0 {
		return nil, nil
	}
	o, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("session options: %w", err)
	}
	if err := o.SetIntraOpNumThreads(n); err != nil {
		_ = o.Destroy()
		return nil, fmt.Errorf("set intra-op threads to %d: %w", n, err)
	}
	return o, nil
}

// Infer runs ONNX inference with the given token IDs, style vector, and speed.
// tokens should NOT include the padding zeros — they are added internally.
// styleVec is the 256-dim voice embedding for the sequence length.
// Returns the raw float32 waveform samples at 24kHz.
func (e *Engine) Infer(tokens []int64, styleVec []float32, speed float32) ([]float32, error) {
	// Pad tokens: [0, ...tokens, 0]
	padded := make([]int64, len(tokens)+2)
	copy(padded[1:], tokens)
	// padded[0] and padded[len(padded)-1] are already 0

	inputIDs, err := ort.NewTensor(ort.NewShape(1, int64(len(padded))), padded)
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	defer func() { _ = inputIDs.Destroy() }()

	// Style tensor: [1, 256]
	if len(styleVec) != 256 {
		return nil, fmt.Errorf("style vector must be 256 elements, got %d", len(styleVec))
	}
	styleCopy := make([]float32, 256)
	copy(styleCopy, styleVec)
	styleTensor, err := ort.NewTensor(ort.NewShape(1, 256), styleCopy)
	if err != nil {
		return nil, fmt.Errorf("create style tensor: %w", err)
	}
	defer func() { _ = styleTensor.Destroy() }()

	speedSlice := []float32{speed}
	speedTensor, err := ort.NewTensor(ort.NewShape(1), speedSlice)
	if err != nil {
		return nil, fmt.Errorf("create speed tensor: %w", err)
	}
	defer func() { _ = speedTensor.Destroy() }()

	// Run inference (mutex: onnxruntime sessions are not thread-safe)
	// Pass nil output so ONNX runtime auto-allocates the waveform tensor
	outputs := []ort.Value{nil}
	e.mu.Lock()
	err = e.session.Run(
		[]ort.Value{inputIDs, styleTensor, speedTensor},
		outputs,
	)
	e.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("inference run: %w", err)
	}

	if outputs[0] == nil {
		return nil, fmt.Errorf("no output tensor returned")
	}
	defer func() { _ = outputs[0].Destroy() }()

	// Extract float32 waveform from output
	outputTensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output type: expected *Tensor[float32]")
	}

	waveform := outputTensor.GetData()
	// Copy to avoid keeping the ONNX runtime buffer alive
	result := make([]float32, len(waveform))
	copy(result, waveform)

	return result, nil
}

// Close releases the ONNX runtime resources.
func (e *Engine) Close() {
	if e.session != nil {
		_ = e.session.Destroy()
	}
	_ = ort.DestroyEnvironment()
}
