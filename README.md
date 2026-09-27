# Open Decision Server

Open Decision Server serves typed probabilistic decisions through a Jev-compatible System One HTTP API. The first engine runs Julia-1 ONNX on ONNX Runtime CPU. Request encoding and tokenization are implemented in Go; there is no Rust build or runtime dependency.

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

## Linux container

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

## Configuration

`ods -h` lists all flags. The main options are `-http`, `-model`, `-tokenizer`, `-ort-lib`, `-ort-threads`, `-max-runs`, `-max-waiting`, `-max-questions`, `-max-length`, `-head-length`, and `-log-requests`. Structured request logging omits state, instructions, and criteria.

## Upstream references

- [Julia-1 ONNX model and parity fixtures](https://huggingface.co/SupersonicLabs/Julia-1-ONNX)
- [TypeSafe System One OpenAPI](https://api.typesafe.ai/openapi.json)
- [Official ONNX Runtime Go binding](https://github.com/microsoft/onnxruntime/tree/main/go)

Licensed under Apache-2.0. Julia-1 model assets and ONNX Runtime retain their upstream licenses.
