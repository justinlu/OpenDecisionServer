# Native Chinese Decision Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve the approved Chinese Decision 2B Q8_0 through ODS without Python at runtime.
**Architecture:** One selected model per process; model-specific parsing and response policy; a bounded serial worker exclusively owns a llama.cpp context. A darwin/arm64 cgo build links the pinned Metal libraries; ordinary builds retain Julia and fail clearly if the native engine is selected.
**Tech Stack:** Go, cgo, llama.cpp bundled with llama-cpp-python 0.3.36, Metal.
**Spec:** ../specs/2026-10-27-llama-backend-design.md

## Global Constraints

- Preserve Julia aliases, parser behavior, responses and official ONNX parity.
- Chinese Decision: 2–26 candidates, 16,384 tokens, temperature 1.074872374534607 from verified config; no truncation, sampling or batch optimization.
- String/structured prompts must match fixed Python; retain nested object order, integer precision, finite float semantics, null and empty descriptions.
- One worker/context; active Metal cancellation discards after completion. Shutdown joins before native free.
- Requests at most 64 questions / 1 MiB; at most 32 queued requests and 65,536 queued compiled tokens. Accept all questions atomically, schedule one question per request per round.
- Acceptance: 100% winner agreement and probability delta ≤1e-4 on 989 fixed cases, plus Python boundary goldens; HTTP latency matrix measured through full response.

## Review Focus

- Structured decimal/exponent numbers, Unicode controls and duplicate nested keys must not silently change prompt bytes.
- Cancellation during native work must not free or reuse native memory early, including after HTTP shutdown timeout.
- A request failing its last question’s validation must not execute its first question.
- Exhaustion, concurrent Close/Decide and canceled queued jobs must release queue/token budgets exactly once.
- Startup with wrong model/template/tokenizer/native ABI or missing Metal must fail before readiness.

### Task 1: Model policy and faithful request content
**Files:** internal/decision/{model.go,chinese.go,content.go,*_test.go,types.go,service.go}; internal/api/http/{server.go,server_test.go}; tests/evaluation/native_probe/export_golden.py; internal/decision/testdata/chinese-golden.json.
**Interfaces:** Model{ID,Aliases,Chinese}; Service.ModelInfo() Model; ParseForModel(body []byte,maxQuestions int,model Model)(Request,error); ChineseText(json.RawMessage)(string,error).
- [ ] Write tests for Chinese defaults, 26 options, raw legend, null/empty and duplicate content; Python prompt/content goldens with float/large-int/escape boundaries; Julia unchanged; /models reflects active model and readiness.
- [ ] Observe new tests fail; implement model policy, ordered Python-compatible content rendering and response fields; run decision and HTTP tests.
- [ ] Commit passing slice.

### Task 2: Serial runtime ownership and queue
**Files:** internal/engine/chinese/{engine.go,queue.go,engine_test.go,prompt.go,config.go}.
**Interfaces:** native interface Tokenize(string)([]int32,error), Logits([]int32,int)([]float32,error), Close() error; Engine.Decide/Warmup/Stop/Close; Options with model/config/library paths, threads and queue/token limits.
- [ ] Fake-native tests cover atomic compile, limits, round-robin order, canceled queued/active jobs, decode errors, concurrent closes and resource drain.
- [ ] Observe failures; implement exclusive worker, stable calibrated softmax, prompt/config checks and metrics; verify with go test -race.
- [ ] Commit passing slice.

### Task 3: Pinned native runtime and reproducible build
**Files:** internal/engine/chinese/{native_llama.go,native_stub.go,native_integration_test.go}; scripts/{prepare-llama.sh,build-llama.sh}; Makefile.
**Interfaces:** openNative(Options)(native,error), cgo build tag llama on darwin/arm64; ordinary build returns explicit unsupported error.
- [ ] Add gated real-Metal tests for golden token IDs/probabilities, A/B/A, long input chunks, cancellation and safe Close; add missing-native startup test.
- [ ] Implement C wrappers using exactly matching SDK headers/libraries, clear full hybrid memory, copy finite candidate logits, synchronize before cleanup. Prepare standalone pinned native SDK without Python runtime.
- [ ] Build both ordinary and llama-tag binaries; run gated acceptance and commit.

### Task 4: Startup, lifecycle, docs and end-to-end acceptance
**Files:** cmd/ods/{main.go,main_test.go}; README.md; tests/evaluation/results/2026-10-27-ods-native/*.
**Interfaces:** CLI -engine julia|chinese-decision; selected model descriptor used by parser/service/models; run shutdown drains workers even when HTTP deadline expires.
- [ ] Tests for selected model, invalid config, stop-before-drain and no native Close while an active request uses resources.
- [ ] Implement startup/warmup/readiness/lifecycle, runtime metrics and deployment docs; run normal/race/vet and official Julia parity.
- [ ] Run 989 HTTP accuracy cases, structured Python goldens, 18 voice workloads × clients 1/4 × 120 observations/group. Preserve errors, probabilities, latency and environment.
- [ ] Fresh whole-branch review under executing-plans, address material findings, commit and integrate main under standing user authorization.

## Execution record

User accepted the native dynamic-library approach after the design and native spike. Execute inline; do not re-request the already supplied implementation authorization. The design’s failed batch experiment remains disabled.
