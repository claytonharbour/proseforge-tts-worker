# ProseForge TTS Worker

Go-based text-to-speech worker using the [Kokoro-82M](https://huggingface.co/hexgrad/Kokoro-82M) ONNX model. Designed for [RunPod](https://www.runpod.io/) serverless deployment, but runs locally as a plain HTTP server.

## Features

- **Pure Go phonemization** — Misaki dictionaries + Flite LTS rules, no Python dependencies
- **Context-aware homograph disambiguation** — "read" vs "read", "live" vs "live"
- **Kokoro-82M ONNX inference** — ~310MB float32 model via onnxruntime-go
- **RunPod serverless** — zero cold start, heartbeat pings, polling worker
- **WAV + MP3 output** — native WAV encoding, MP3 via ffmpeg
- **Audio quality tools** — pitch analysis, pause detection, glitch detection, spectrograms

## Quick Start

```bash
make dev-models     # Download ONNX model + voice files (~323MB)
make dev-build      # Build Go binary
make dev-run        # Start local HTTP server on :8090
make dev-test       # Run all tests
```

Test with curl:
```bash
curl -s -X POST http://localhost:8090 \
  -H "Content-Type: application/json" \
  -d '{"input":{"text":"Hello world","voice":"af_sarah","speed":1.0}}' \
  | jq '{format: .output.format, sample_rate: .output.sample_rate, duration: .output.duration_seconds}'
```

## Architecture

```
Text → Normalization → Phonemization → Token IDs → ONNX Inference → WAV → optional MP3
```

1. **Text normalization** — numbers, abbreviations, punctuation to speakable text
2. **Phonemization** — homograph disambiguation, Misaki dictionary lookup (gold/silver), Flite LTS fallback
3. **Tokenization** — phoneme sequences to Kokoro token IDs (max 509 per segment)
4. **ONNX inference** — token IDs + voice embedding + speed → raw float32 waveform
5. **Audio encoding** — waveform → WAV (native Go) or MP3 (ffmpeg)

## API

**Request** (RunPod wraps the payload in `{"input": ...}`):
```json
{
  "input": {
    "text": "Hello world",
    "voice": "af_sarah",
    "speed": 1.0,
    "format": "wav"
  }
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `text` | string | yes | — | Text to synthesize |
| `voice` | string | no | `af_sarah` | Voice name |
| `speed` | float | no | `1.0` | Playback speed multiplier |
| `format` | string | no | `wav` | Output format: `wav` or `mp3` |

**Response:**
```json
{
  "output": {
    "audio": "<base64-encoded audio>",
    "format": "wav",
    "sample_rate": 24000,
    "duration_seconds": 1.234,
    "voice": "af_sarah"
  }
}
```

**Health check:** `GET /health` → `200 OK`

## Prerequisites

- Go 1.24+
- [ONNX Runtime](https://github.com/microsoft/onnxruntime) shared library
  - macOS: `brew install onnxruntime`
  - Linux: download from [ONNX Runtime releases](https://github.com/microsoft/onnxruntime/releases)
- ffmpeg (optional, for MP3 encoding)

## License

Apache 2.0. See [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) for dependency attribution.

The Kokoro-82M model is Apache 2.0 licensed by [hexgrad](https://huggingface.co/hexgrad/Kokoro-82M).
Misaki phonemization dictionaries are Apache 2.0 licensed.
Flite LTS rules are BSD licensed.
