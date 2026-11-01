# proseforge-tts-worker Makefile
# Go-based TTS worker using Kokoro ONNX model

BINARY_NAME := tts-worker
BUILD_DIR := build
GO := go
GOBIN := $(shell go env GOPATH)/bin
GOFLAGS := -tags=unit
LDFLAGS_PKG := main
# `git rev-parse --short HEAD` answers "what is HEAD", never "what did I build".
# A build from a modified tree stamped HEAD's hash with no marker at all, so
# /version claimed to be a commit that does not contain the code running —
# measured: tree had `M internal/phonemizer/homograph.go` and /version said
# {"git_hash":"4be2256"}, HEAD exactly.
#
# ⚠️ `--porcelain` with no flag INCLUDES untracked files, and that is deliberate.
# An untracked file is the case that has bitten this fleet hardest: it is
# invisible to `git diff`, gets swept by a broad `git add`, and is exactly what a
# red-proof leaves on disk. A guard that ignored it would miss the one it exists
# for. Gitignored paths (models/, build/) are ignored by --porcelain already, so
# a fresh model download does not trip it.
#
# The deploy script refuses a dirty tree outright, so a DEPLOYED binary never
# carries this suffix. It exists for private instances — a worker on a derived
# port testing uncommitted work, which is the workflow that makes refusing at
# deploy time livable in the first place.
GIT_DIRTY := $(shell git status --porcelain 2>/dev/null | grep -q . && echo -dirty)
LDFLAGS := -X '$(LDFLAGS_PKG).buildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)' \
           -X '$(LDFLAGS_PKG).gitHash=$(shell git rev-parse --short HEAD)$(GIT_DIRTY)'

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
# Quoted, because a Go -run pattern is a regex and alternation is the natural
# way to select several tests. Unquoted, `RUN='TestA|TestB'` reached /bin/sh as
# a PIPE: go test ran with a truncated pattern and its output was piped into a
# command named TestB. That exits 127 with "command not found" and ZERO "--- FAIL"
# lines — a selection that silently ran nothing, presenting as a clean run.
# Cost a whole afternoon of multi-pattern invocations that appeared to match no
# tests when they had never been passed to go test at all.
	TEST_RUN := -run '$(RUN)'
endif
ifdef VERBOSE
	TEST_VERBOSE := -v
endif

# Fixing the quoting above left the deeper hole wide open, and it took a day to see:
# a RUN pattern that matches NOTHING is indistinguishable from one where everything passed.
#
#	make dev-test-unit RUN='TestThatDoesNotExist'   →  exit 0, no FAIL lines, "ok" everywhere
#
# That is the failure mode of a RED-PROOF specifically, which is the one run whose whole
# purpose is showing a test CAN fail. A typo in the test name yields a confident green over
# an empty set — and counting "--- FAIL" lines gives zero in the healthy case too, so the
# usual tell cannot separate them.
#
# ⚠️ And the exit code cannot either. MEASURED in this repo:
#
#	                       bare go test    through make
#	genuine test failure        1               2
#	compile / vet failure       2               2      ← collapsed
#	RUN matches nothing         0               0      ← silent success
#
# ⚑ `go test` DOES distinguish a failing assertion from a broken build. `make`
# destroys that distinction by returning 2 for any failed recipe — so the signal
# exists upstream and never reaches the person reading the number, which is worse
# than it not existing. @wayland and I each measured one column and read the other
# as a contradiction; both were right about the tool they ran.
#
# So the guard cannot be a number at all. It has to be a POSITIVE assertion that the
# named tests exist: `go test -list` prints each matching test name, and prints none
# when the pattern matches nothing.
#
# ⚠️ THE PRINTED COUNT IS FUNCTIONS, AND THE RUN OUTPUT SHOWS FUNCTIONS PLUS SUBTESTS.
# Measured 2026-08-20 on RUN=TestHomograph: the guard says 13, and the verbose run
# emits 56 `=== RUN` lines — 13 top-level plus 43 t.Run subtests. Both numbers are
# correct about different things, and `go test -list` agrees with the guard at 13.
#
# 📌 Do not "fix" the guard to report 56. It exists to answer "does this name match
# anything", and -list is the only cheap oracle for that; counting subtests would mean
# running them, which is the thing being guarded. Recorded because a guard whose number
# disagrees with the output it precedes is the kind of thing someone corrects into
# being wrong.
#
# ✅ And RUN genuinely restricts execution here — verified rather than assumed after
# @vance found proseforge's dev-test-unit passing no -run at all, with its own docs
# asserting the opposite: whole suite 573 tests, RUN=<one exact name> 1 test.
define assert_run_matches
@if [ -n "$(RUN)" ]; then \
		matched=$$($(GO) test -list '$(RUN)' -tags=$(1) ./... 2>/dev/null | grep -c '^Test' || true); \
		if [ "$$matched" -eq 0 ]; then \
			echo ""; \
			echo "🛑 RUN='$(RUN)' matches NO test in any package (-tags=$(1))."; \
			echo "   Refusing to run: go test would exit 0 with no output, which reads as PASS."; \
			echo "   Check the spelling, and check the build tag — a unit test is invisible to -tags=functional."; \
			exit 1; \
		fi; \
		echo "   RUN='$(RUN)' matches $$matched test(s)"; \
	fi
endef

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

# ⚠️ `go test ./...` BUFFERS OUTPUT PER PACKAGE, so the last `=== RUN` in a stalled
# log is the last line FLUSHED — not the test that is running. @Gordon filed a hang
# against a test that takes 0.53s and closed it invalid: the package holding it was
# simply the slowest to complete, and its whole buffer arrives at the end.
#
# 🛑 So "which test is hanging" cannot be read off the tail of the output. Use
# `-v` (per-test flush) or `go test -timeout` and read the panic trace, which names
# the actual goroutine. Guessing from the last RUN line names the wrong test with
# complete confidence.
dev-test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	$(call assert_run_matches,unit)
	$(GO) test $(TEST_VERBOSE) $(TEST_RUN) -tags=unit ./...

# assert_functional_only refuses to run a functional suite that contains no test the
# unit suite does not already have.
#
# ⛔ This is the #91 defect as a gate. An UNTAGGED file compiles under every tag set,
# so `-tags=functional` used to resolve to four string/arithmetic tests that also ran
# under `-tags=unit` — functional-ONLY tests: 0. The suite exited 0 whether or not the
# TTS engine worked, because nothing it ran could observe the engine.
#
# assert_run_matches catches a zero-match RUN. It cannot catch a zero-coverage SUITE,
# because the suite was never empty — it was full of the wrong tests.
define assert_functional_only
@fnl=$$(mktemp); unl=$$(mktemp); \
	$(GO) test -list '.*' -tags=functional ./... 2>/dev/null | grep '^Test' | sort -u > $$fnl; \
	$(GO) test -list '.*' -tags=unit ./... 2>/dev/null | grep '^Test' | sort -u > $$unl; \
	only=$$(comm -23 $$fnl $$unl | grep -c . || true); \
	rm -f $$fnl $$unl; \
	if [ "$$only" -eq 0 ]; then \
		echo ""; \
		echo "🛑 The functional suite contains NO test that the unit suite lacks."; \
		echo "   Every test here also runs under -tags=unit, so this target proves nothing"; \
		echo "   the unit suite has not already proven. That is #91: a green that cannot go red."; \
		echo "   Fix: give at least one real end-to-end test a //go:build functional tag."; \
		exit 1; \
	fi; \
	echo "   functional-only tests: $$only"
endef

dev-test-functional: ## End-to-end tests over the REAL model and engine (requires `make dev-models`)
	@echo "Running functional tests..."
	@# -count=1 disables Go's test cache. Go keys the cache on the test binary and
	@# the files it reads, so it cannot see that the ONNX model, the voice pack or a
	@# rebuilt worker changed underneath it — a cached PASS would be a replay of an
	@# earlier run against different state, reported as fresh evidence.
	@# Unit tests deliberately keep the cache: they are hermetic and Go's
	@# invalidation is correct for them (changing data/custom_dict.json does re-run
	@# the phonemizer package). See proseforge#891.
	$(call assert_functional_only)
	$(call assert_run_matches,functional)
	$(GO) test -count=1 $(TEST_VERBOSE) $(TEST_RUN) -tags=functional ./...

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

# Runs every check required before a commit, as one command that can fail.
#
# This gate CHECKS formatting; it never rewrites it. `dev-format` is `gofmt -w .` and
# rewrites every .go file in the tree, so a hook that ran it on each commit would change
# files the committer never touched, and the commit would differ from what was staged.
#
# `make install-hooks` is a convenience only: hooks are per-clone, so this target is the
# durable gate.
dev-precommit: ## Run every check CLAUDE.md requires before a commit (checks only — never rewrites)
	@echo "→ format check (staged files only, no rewrite)"
	@unformatted=$$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$$' | xargs -r gofmt -l 2>/dev/null); \
	 if [ -n "$$unformatted" ]; then \
	   echo "🛑 staged Go files are not gofmt-clean:"; echo "$$unformatted" | sed 's/^/     /'; \
	   echo "   run 'make dev-format' yourself — this gate will not rewrite the tree for you"; \
	   exit 1; \
	 fi
	@echo "→ red-proof markers (staged content only)"
	@# 🛑 LINT DOES NOT CATCH A RED-PROOF INJECTION. Measured 2026-08-21 against the
	@# exact shape that reached @Aldric's master: `if true { return "" }` COMPILES and
	@# `golangci-lint run ./...` reports 0 issues. Every other arm of this gate passed
	@# it too, so without this the gate is silent on the one defect it most needs to
	@# see — I inject markers into this tree several times a day to red-proof things.
	@#
	@# ⚠️ Marker set is deliberately TIGHT, and it got tighter after testing it.
	@# @Wayland measured `return ""` as a false positive across 55 commits; `XXX` and
	@# `TEMP` are ordinary English. I also had to DROP `RED-?PROOF`: running the check
	@# over 24h of history flagged e3b76e5, which is a COMMENT saying a test "can be
	@# RED-PROOFED without mutating the shared dir" — legitimate documentation, and I
	@# write that prose constantly.
	@#
	@# 🛑 A gate that refuses honest commits gets bypassed, which is the "red that is
	@# always red" defect I deliberately avoided when scoping check-docs. What remains
	@# is identifier-shaped: things nobody types except while injecting.
	@# ⚠️ SCOPED TO STAGED .go FILES, and that is not laziness. The first version
	@# scanned the whole staged diff and refused THIS commit — the Makefile contains
	@# the marker strings, in the comment above and in the grep itself. A literal
	@# true positive and a practical false one. Injections in this fleet are Go
	@# (@Aldric's was, all of mine are), so .go is where the guard belongs.
	@#
	@# 🛑 The hole is real and stated rather than hidden: an injection into a shell
	@# script or a Makefile is NOT caught. If that ever happens, widen this and solve
	@# the self-reference properly — do not quietly assume it is covered.
	@staged=$$(git diff --cached -U0 -- '*.go' | grep '^+' | grep -v '^+++'); \
	 hits=$$(printf '%s\n' "$$staged" | grep -nE 'if true \{|DO NOT COMMIT|redProofProbe|ForRedProof|badlyFormatted' || true); \
	 if [ -n "$$hits" ]; then \
	   echo "🛑 red-proof marker in staged content — refusing:"; \
	   printf '%s\n' "$$hits" | sed 's/^/     /'; \
	   exit 1; \
	 fi
	@echo "→ lint";       $(MAKE) --no-print-directory dev-lint
	@echo "→ build";      $(MAKE) --no-print-directory dev-build
	@echo "→ unit tests"; $(MAKE) --no-print-directory dev-test-unit
	@echo "→ docs";       $(MAKE) --no-print-directory check-docs
	@echo "✅ pre-commit checks passed"

install-hooks: ## Install .git/hooks/pre-commit calling dev-precommit (per-clone, not checked in)
	@mkdir -p .git/hooks
	@printf '#!/usr/bin/env bash\n# installed by `make install-hooks` (#78) — re-run after a fresh clone\nexec make dev-precommit\n' > .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "✅ .git/hooks/pre-commit installed → make dev-precommit"
	@echo "⚠️  per-clone only. The durable check is 'make dev-precommit'; a fresh checkout has no hook."

# ⚠️ ENFORCED ON THE RULE FILES ONLY, deliberately. The repo-wide sweep reports 13
# further files (ear-check write-ups, a design doc, a blog post) — prose, where a
# wrapped bold costs nothing because nobody greps it. Wiring those into a gate would
# make this target red on day one, and a red that is always red trains people to
# ignore the next one. Run the sweep by hand when you want the population:
#     bash scripts/check-unbroken-emphasis.sh
check-docs: ## Verify no memorable rule is split across a line break (CLAUDE.md, AGENTS.md)
	@bash scripts/check-unbroken-emphasis.sh CLAUDE.md AGENTS.md

# ⚠️ DELIBERATELY NOT IN dev-precommit, for the same reason check-docs is scoped to
# two files: this target is RED ON DAY ONE and honestly so — every clip in
# calibration/ear-check/ was rendered 40-85 commits ago, so all nine with a recorded
# sha are stale and two record no sha at all (#94). Wiring that into the commit gate
# would refuse every honest commit until someone re-renders eleven clips, and "a red
# that is always red trains people to ignore the next one".
#
# ⇒ Run it BEFORE offering a clip to a listener. That is the moment the answer
# matters: a stale clip asks someone to judge audio the engine no longer produces,
# and their verdict then attaches to a rendering that no longer exists.
check-ear-clips: ## Report which ear-check clips predate changes to data/ or internal/phonemizer/
	@bash scripts/check-ear-clips.sh

dev-format: ## Format code
	@echo "Formatting..."
	gofmt -w .
	@# Invoke goimports by absolute path rather than relying on PATH. GOPATH/bin
	@# is not on PATH for several of us, so a bare `goimports` made this target
	@# exit 2 — harmless alone, but it silently skipped anything chained behind
	@# it with && and cost a night of measuring against a binary that was never
	@# rebuilt. Gordon's pattern from proseforge (47e47fbe): a Makefile should not
	@# depend on PATH when it can compute the absolute path one line away, and
	@# the remedy belongs here rather than in ten people's shell profiles.
	@test -x "$(GOBIN)/goimports" || (echo "goimports not found at $(GOBIN)/goimports — run: go install golang.org/x/tools/cmd/goimports@latest" >&2; exit 2)
	$(GOBIN)/goimports -w .

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

.PHONY: verify-ear-queue
verify-ear-queue: ## Check every queued ear clip cites a real, not-newer commit
	bash scripts/verify-ear-queue.sh
