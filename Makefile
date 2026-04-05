# proseforge-tts-worker Makefile
# Go-based TTS worker using Kokoro ONNX model

BINARY_NAME := tts-worker
BUILD_DIR := build
GO := go
GOFLAGS := -tags=unit
LDFLAGS_PKG := main
LDFLAGS := -X '$(LDFLAGS_PKG).buildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)' \
           -X '$(LDFLAGS_PKG).gitHash=$(shell git rev-parse --short HEAD)'

# Model download URLs (HuggingFace, public)
MODEL_BASE_URL := https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0
MODEL_ONNX := kokoro-v1.0.onnx
MODEL_VOICES := voices-v1.0.bin
WHISPER_MODEL := models/ggml-base.en.bin
WHISPER_MODEL_URL := https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin

# Test filtering
RUN ?=
VERBOSE ?=

ifdef RUN
	TEST_RUN := -run $(RUN)
endif
ifdef VERBOSE
	TEST_VERBOSE := -v
endif

.PHONY: help dev-build dev-run dev-test dev-test-unit dev-test-functional \
        dev-models dev-whisper-model dev-install-deps dev-lint dev-format \
        dev-docker-build dev-clean dev-analyze dev-generate dev-evaluate dev-diagnose \
        sync-github sync-github-dry sync-check

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-24s\033[0m %s\n", $$1, $$2}'

dev-build: ## Build Go binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/worker/

dev-run: dev-build ## Run local HTTP server on :8090
	@echo "Starting TTS worker on :8090..."
	$(BUILD_DIR)/$(BINARY_NAME)

dev-test: dev-test-unit dev-test-functional ## Run all tests

dev-test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	$(GO) test $(TEST_VERBOSE) $(TEST_RUN) -tags=unit ./...

dev-test-functional: ## Run functional/integration tests (requires model files)
	@echo "Running functional tests..."
	$(GO) test $(TEST_VERBOSE) $(TEST_RUN) -tags=functional ./...

dev-models: ## Download ONNX model and voice files
	@echo "Downloading model files to models/..."
	@mkdir -p models
	@if [ ! -f models/$(MODEL_ONNX) ]; then \
		echo "Downloading $(MODEL_ONNX) (~310MB)..."; \
		curl -L -o models/$(MODEL_ONNX) "$(MODEL_BASE_URL)/$(MODEL_ONNX)"; \
	else \
		echo "$(MODEL_ONNX) already exists, skipping."; \
	fi
	@if [ ! -f models/$(MODEL_VOICES) ]; then \
		echo "Downloading $(MODEL_VOICES) (~13MB)..."; \
		curl -L -o models/$(MODEL_VOICES) "$(MODEL_BASE_URL)/$(MODEL_VOICES)"; \
	else \
		echo "$(MODEL_VOICES) already exists, skipping."; \
	fi
	@echo "Model files ready."

dev-whisper-model: ## Download Whisper model for transcription (~142MB)
	@mkdir -p models
	@if [ ! -f $(WHISPER_MODEL) ]; then \
		echo "Downloading ggml-base.en.bin (~142MB)..."; \
		curl -L -o $(WHISPER_MODEL) "$(WHISPER_MODEL_URL)"; \
	else \
		echo "$(WHISPER_MODEL) already exists, skipping."; \
	fi

dev-install-deps: ## Install dev dependencies (whisper-cpp, ffmpeg)
	brew install whisper-cpp ffmpeg

dev-lint: ## Run linter
	@echo "Running linter..."
	golangci-lint run ./...

dev-format: ## Format code
	@echo "Formatting..."
	gofmt -w .
	goimports -w .

dev-docker-build: ## Build RunPod Docker image
	@echo "Building Docker image..."
	docker build -t proseforge-tts-worker:latest -f deploy/docker/Dockerfile .

dev-analyze: ## Build audio analysis CLI
	@echo "Building analyze CLI..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/analyze ./cmd/analyze/

dev-generate: ## Build story generation CLI
	@echo "Building generate CLI..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/generate ./cmd/generate/

EVALUATE_WAV ?= build/experiments/031-adaptive-silence/excerpt-4para.wav
EVALUATE_TEXT ?= build/excerpt-section1.md
EVALUATE_LINES ?= 5,11p

dev-evaluate: dev-analyze ## Evaluate TTS pronunciation quality via Whisper WER
	@echo "Running WER evaluation..."
	@if [ ! -f $(WHISPER_MODEL) ]; then \
		echo "Error: Whisper model not found. Run 'make dev-whisper-model' first."; \
		exit 1; \
	fi
	@sed -n '$(EVALUATE_LINES)' $(EVALUATE_TEXT) > /tmp/evaluate-ref.txt
	@$(BUILD_DIR)/analyze \
		-test $(EVALUATE_WAV) \
		-reference-text /tmp/evaluate-ref.txt \
		-whisper-model $(WHISPER_MODEL)
	@rm -f /tmp/evaluate-ref.txt

dev-diagnose: ## Build emphasis diagnosis CLI
	@echo "Building diagnose CLI..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/diagnose ./cmd/diagnose/

dev-clean: ## Remove build artifacts
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	@echo "Clean complete. (Model files in models/ preserved — use 'rm -rf models/' to remove)"

# --- GitHub mirror sync ---

sync-github: ## Sync curated snapshot to GitHub (full push)
	scripts/sync-github.sh

sync-github-dry: ## Dry run — stage + verify, no push
	scripts/sync-github.sh --dry-run

sync-check: ## Run guardrail checks on current tree
	scripts/sync-github.sh --check
