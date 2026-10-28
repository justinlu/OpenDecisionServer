# Open Decision Server

Open Decision Server serves typed probabilistic decisions through a Jev-compatible System One HTTP API. The first engine runs Julia-1 ONNX on ONNX Runtime CPU. Request encoding and tokenization are implemented in Go.

Chinese Decision 2B Q8_0 is also available on Apple Silicon through an optional cgo/llama.cpp Metal build. It runs in the ODS process with a standalone native SDK and separate model files; Python is only used to export reference tests, never by the running service.

## Supported platforms

- macOS ARM64: native development and primary acceptance platform.
- Linux x86-64: container deployment and CI target.

The service needs Go 1.26 or newer with cgo and a C compiler to build, plus the ONNX Runtime native library at runtime. The model graph, external weights, tokenizer, and ORT library are downloaded separately from source control.

## Run on macOS ARM64

```sh
make deps
make build
./bin/ods -http 127.0.0.1:8080
```

`make deps` verifies SHA-256 checksums for the pinned Julia-1 ONNX snapshot and ONNX Runtime 1.30.0. Model files go to `models/julia-1/`; the native library goes to `lib/darwin-arm64/`. Both are ignored by Git.

Check readiness and ask a question:

```sh
curl http://127.0.0.1:8080/health/ready
curl -sS http://127.0.0.1:8080/v1/systemone \
  -H 'Content-Type: application/json' \
  -d '{"model":"jev-latest","state":"The customer was charged twice.","questions":{"refund":{"type":"noul","instructions":"Is the customer asking for a refund?"},"team":{"type":"choice","instructions":"Which team should handle this?","criteria":{"billing":"Payments and refunds","support":"General help"}}}}'
```

With the official Python SDK, configure only the API root and use any nonempty local API key:

```python
from typesafe_sdk import Noul, TypeSafeClient

with TypeSafeClient(api_key="local", base_url="http://127.0.0.1:8080") as client:
    response = client.system_one(
        state="The customer was charged twice.",
        questions={"refund": Noul(instructions="Is a refund requested?")},
    )
    print(response.nouls["refund"].noul)
```

## Chinese Decision on Apple Silicon

Install Go, a C/C++ compiler, CMake and curl, then build the pinned Metal SDK and fetch the pinned model:

```sh
bash scripts/prepare-llama.sh
bash scripts/download-chinese-model.sh
bash scripts/build-llama.sh
./bin/ods-metal -engine chinese-decision -http 127.0.0.1:18082
```

The SDK is built directly from the native source bundled with llama-cpp-python 0.3.36 (verified SHA-256); no Python installation is required. Dynamic-library dependencies resolve relative to their own directory. Distribute `bin/ods-metal`, the SDK `lib/` directory, its adjacent `source.sha256` marker, `SHA256SUMS`, native licenses and model/config files together. `LLAMA_SDK` selects another SDK build/install directory; pass its `lib/libllama.dylib` through `-llama-lib` at runtime.

For the isolated LFM3/d1 candidate SDK and Chinese Decision numerical compatibility patch, see the [compatibility report](tests/evaluation/results/2026-10-28-chinese-parity-fix/README.md). The [unified native routing design](docs/superpowers/specs/2026-10-28-unified-native-model-routing-design.md) describes the planned Go+cgo integration; production model routing and the SDK above have not yet changed.

Use `-llama-model`, `-llama-config` and `-llama-lib` for files outside the default directories. Startup verifies model/config hashes, the SDK source marker and library manifest, candidate token IDs, Metal availability and warmup. A wrong model, missing library or unavailable GPU fails startup; there is no CPU fallback. Ordinary `make build` still defaults to Julia and does not require native SDK headers.

Requests select `"model":"chinese-decision-2b"`. Its choice/score limit is 2–26 candidates, and full prompts may contain at most 16,384 tokens. It preserves ordered structured JSON, large integers, finite Python-style floats, null and empty candidate descriptions. Missing/null instructions are rejected; noul descriptions default to “否/是”. All three answer types include calibrated probabilities and confidence. Score legends preserve original criteria JSON. Duplicate JSON keys and invalid Unicode are rejected.

One worker owns one native context, processing independent questions serially. Multiple requests rotate one question at a time; GPU batching is disabled after the [batch experiment](tests/evaluation/results/2026-10-27-native-validation/README.md). Defaults are 32 waiting requests and 65,536 reserved compiled tokens, including in-flight requests. Change `-llama-max-waiting`, `-llama-max-tokens` and `-llama-threads`; the Julia `-max-runs` setting does not add native workers. A full budget returns 429; prompts are never silently truncated.

Canceling active Metal work stops the caller's wait; the worker retains the context until the GPU finishes. Shutdown stops admission and drains native work before freeing resources, even if HTTP shutdown times out. The process may therefore outlive that HTTP deadline while draining.

`/metrics` includes native queue depth, reserved tokens, runs, rejects, cancellations, errors, compilation/queue/inference seconds, HTTP handler duration, process peak RSS and Metal device memory. The device free/total counters describe the recommended Metal working-set allowance and its unused portion, rather than physical RAM; startup logs include the native library SHA-256. Text content is excluded from default logs. These metrics and the [end-to-end acceptance report](tests/evaluation/results/2026-10-27-ods-native/README.md) describe measured behavior; the 100ms voice target is not met for every history/question/concurrency combination.

## Linux container (Julia)

Download the model before building the image:

```sh
./scripts/download-model.sh
docker build --platform linux/amd64 -t open-decision-server .
docker run --rm -p 8080:8080 open-decision-server
```

The Docker build downloads the pinned Linux x86-64 ONNX Runtime CPU archive and checks its SHA-256 digest. The image includes the model files and native ORT library. It does not require Python, PyTorch, CUDA, Node.js, or Rust at runtime.

## API

`POST /v1/systemone` takes `model`, `state`, and named `questions`. Supported types are `noul`, `choice`, and `score`. The response contains `model`, `answers`, and `usage`. `GET /v1/models` lists accepted model names; `GET /health/live` and `GET /health/ready` serve health checks. `GET /metrics` exposes the active inference runs, queue depth, total runs, and maximum observed overlapping `Session.Run` calls in Prometheus text format.

The initial aliases `jev-latest`, `jev-1.13.0`, and `typesafe/jev-1.13` all resolve to `julia-1`. A response identifies the actual model as `julia-1`. `usage.input_tokens` counts encoded inputs; `usage.output_tokens` is zero because Julia does not generate tokens.

Choice accepts 2–20 options; score accepts 2–10 levels. Requests outside those limits, with oversized strict encodings, or more than 64 questions receive 422. Malformed JSON receives 400, unknown models receive 404, and a full inference queue receives 429. State, instructions, and descriptions accept strings or structured JSON content.

Authentication defaults to `none`, so use a trusted local network or put an authenticating reverse proxy in front of a remote deployment. Bearer headers sent by existing SDKs are accepted.

## Concurrency

The process loads one model and creates one ONNX Runtime Session. Multiple goroutines may call `Session.Run` concurrently; each inference owns its input and output tensors. Defaults are four active runs and 128 waiting requests. Change `-max-runs`, `-max-waiting`, and `-ort-threads` based on measured CPU throughput and tail latency. Canceled requests leave the queue before inference.

Same-request questions run as one ONNX batch. Cross-request micro batching is not in this release.

## Verification

```sh
make test
make race
make parity
./bin/odsbench -url http://127.0.0.1:8080/v1/systemone -clients 64 -duration 10m
```

The pure-Go encoder has a fixed official token-ID golden test. `make parity` runs the official 100 Julia-1 ONNX cases in batches of four and compares logits and selected answers against the published reference. `odsbench` checks the wire response while measuring QPS and p50/p95/p99 at 64 concurrent clients. The default load test uses a 30-second per-request timeout; latency goals should be set from measurements on the target hardware and request mix.

macOS ARM64 acceptance on a 10-logical-CPU, 16 GB Mac (`-max-runs 4 -ort-threads 4`): 64 clients for 10 minutes, 72,328 successful requests, zero completed-request errors, 120.5 QPS, p50 493 ms, p95 732 ms, p99 893 ms. The process RSS stabilized near 2.69 GB during the run. A later 30-second run with concurrency metrics recorded four overlapping calls to the same `Session.Run`. These are baseline measurements for the included short/medium request mix, not latency guarantees.

## Language decision evaluation

The [Chinese, English, numeric and pinyin evaluation suite](tests/evaluation/README.md) includes fixed context-grounded diagnostics and a pinned external TokSuite robustness dataset. It reports accuracy and errors separately by source, input form, prompt language, task and domain, and retains per-case predictions. It is an accuracy evaluation, separate from the throughput benchmark above.

For voice assistants, the [Apple Silicon latency comparison](tests/evaluation/results/2026-10-27-voice-latency/README.md) measures Julia CPU and Chinese Decision 2B Metal against a 100 ms p95 budget across history lengths, question counts and concurrency.

The [banking audio intent dataset](tests/evaluation/datasets/banking-audio-v1/README.md) contains 128 Mandarin semantic cases and 324 frozen synthetic WAV files for kiosk navigation versus inquiry, with an [offline listening page](tests/evaluation/datasets/banking-audio-v1/index.html). The [d1-Omni evaluation report](tests/evaluation/results/2026-10-28-banking-audio-v1/README.md) separates text, direct audio, synthetic noise and post-hoc diagnostics; these results do not establish production speech accuracy.

## Configuration

`ods -h` lists all flags. Select `-engine julia|chinese-decision`; the default is Julia. Julia options include `-model`, `-tokenizer`, `-ort-lib`, `-ort-threads`, `-max-runs`, `-max-waiting`, `-max-length` and `-head-length`. Native options are described above. Shared options include `-http`, `-max-questions` and `-log-requests`. Structured request logging omits state, instructions, and criteria.

## Upstream references

- [Julia-1 ONNX model and parity fixtures](https://huggingface.co/SupersonicLabs/Julia-1-ONNX)
- [TypeSafe System One OpenAPI](https://api.typesafe.ai/openapi.json)
- [Official ONNX Runtime Go binding](https://github.com/microsoft/onnxruntime/tree/main/go)

Licensed under Apache-2.0. Julia-1 model assets and ONNX Runtime retain their upstream licenses.

The [two-stage banking routing experiment](tests/evaluation/results/2026-10-28-banking-audio-cascade/README.md) evaluates consultation versus handling first, then business selection only on the handling branch, with separate oracle diagnostics and a fair routing baseline.
