package inference

import (
	"fmt"
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

	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{"tokens", "style", "speed"},
		[]string{"audio"},
		nil, // default session options
	)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &Engine{session: session}, nil
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
	defer inputIDs.Destroy()

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
	defer styleTensor.Destroy()

	speedSlice := []float32{speed}
	speedTensor, err := ort.NewTensor(ort.NewShape(1), speedSlice)
	if err != nil {
		return nil, fmt.Errorf("create speed tensor: %w", err)
	}
	defer speedTensor.Destroy()

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
	defer outputs[0].Destroy()

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
		e.session.Destroy()
	}
	ort.DestroyEnvironment()
}
