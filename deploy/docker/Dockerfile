# Multi-stage build: Go binary + minimal runtime for RunPod serverless.
# Kokoro-82M runs at 35-100x realtime on CPU — no GPU needed for inference.

# Stage 1: Build
FROM golang:1.25 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=1 go build \
    -ldflags "-X 'main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)' -X 'main.gitHash=$(git rev-parse --short HEAD 2>/dev/null || echo docker)'" \
    -o /tts-worker ./cmd/worker/

# Stage 2: Runtime
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl ffmpeg && \
    rm -rf /var/lib/apt/lists/*

# ONNX Runtime (CPU — Kokoro-82M is tiny, GPU not needed)
RUN curl -fSL -o /tmp/ort.tgz \
    https://github.com/microsoft/onnxruntime/releases/download/v1.24.2/onnxruntime-linux-x64-1.24.2.tgz && \
    tar xzf /tmp/ort.tgz -C /usr/local --strip-components=1 && \
    rm /tmp/ort.tgz && ldconfig

WORKDIR /app
COPY --from=builder /tts-worker .
COPY data/ data/

# Bake model files in (FlashBoot optimization — avoid runtime download)
RUN mkdir -p models && \
    curl -fSL -o models/kokoro-v1.0.onnx \
      https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/kokoro-v1.0.onnx && \
    curl -fSL -o models/voices-v1.0.bin \
      https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/voices-v1.0.bin

ENV PORT=8090 MODELS_DIR=models DATA_DIR=data
EXPOSE 8090
CMD ["./tts-worker"]
