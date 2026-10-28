# Native Liquid d1 experiment setup

Run from the repository root on macOS ARM64 with CMake, Apple Command Line Tools, curl and Python 3.9+. No Python ML runtime is needed: Python is only the HTTP evaluation client and fixture generator.

```sh
bash scripts/prepare-liquid-d1.sh
bash scripts/run-liquid-d1.sh d1-omni-2b 18084
```

This downloads the pinned upstream source, Q8_0 language models and F16 projectors/encoders into `dist/liquid-d1/`. The source revision is `d05706b4f32a545814856e98ca518292585443b9`, the merge of [llama.cpp #31275](https://github.com/ggml-org/llama.cpp/pull/31275). Official weight revisions, byte sizes and SHA-256 values are pinned in [liquid-d1-models.json](../../liquid-d1-models.json); measured checks are in `download-verification.json`. Large binaries and models are intentionally excluded from Git.

The startup script uses 99 requested GPU layers, 8192 context tokens, 8192 batch and microbatch tokens, one slot, four CPU threads and zero RAM prompt cache. The server listens only on localhost. The existing ODS server/SDK and default engine are unchanged. One d1 server runs at a time.

In a second terminal:

```sh
python3 tests/evaluation/evaluate.py \
 --url http://127.0.0.1:18084/v1/systemone --model d1-omni-2b \
 --score-readout median --timeout 120 --output /tmp/d1-omni-accuracy
python3 tests/evaluation/benchmark_latency.py \
 --url http://127.0.0.1:18084/v1/systemone --model d1-omni-2b \
 --score-readout median --samples 120 --clients 1 4 --warmup 6 \
 --budget-ms 100 --timeout 120 --output /tmp/d1-omni-latency
```

Stop this server, then start `bash scripts/run-liquid-d1.sh d1-2b 18084`. Replace the model name and output directories in both evaluation commands. Output directories must not already exist.

For the small media diagnostic:

```sh
mkdir -p dist/liquid-d1/media
curl -fL http://images.cocodataset.org/val2017/000000039769.jpg \
 -o dist/liquid-d1/media/cats.jpg
# Expected JPEG SHA-256: dea9e7ef97386345f7cff32f9055da4982da5471c48d575146c796ab4563b04e
python3 tests/evaluation/liquid_d1_media.py --prepare
python3 tests/evaluation/liquid_d1_media.py \
 --url http://127.0.0.1:18084/v1/systemone --model d1-omni-2b \
 --output /tmp/d1-omni-media
python3 tests/evaluation/liquid_d1_media.py \
 --url http://127.0.0.1:18084/v1/systemone --model d1-omni-2b \
 --repeats 20 --output /tmp/d1-omni-media-repeated
```

The official example uses HTTP for this COCO image. HTTPS for the image host failed hostname verification during this run; certificate checks were not disabled. Verify the JPEG hash above. The image comes from the [COCO 2017 validation set](https://cocodataset.org/) and is not redistributed here. Generated WAVs remain in ignored `dist/liquid-d1/media/`; `media-manifest.json` freezes the actual recording hashes, durations and macOS voices. Regenerating with another OS/voice version may change audio bytes and predictions.

The `d1-2b` image diagnostic uses the same command with `--model d1-2b --image-only` and a new output directory. It does not support audio.

Timing includes client HTTP send, response read, JSON parsing and response validation. Audio timing starts with an existing recording; it excludes capture, TTS generation and any live endpointing delay. The timed path contains no ASR. Repeated diagnostic recordings are not new accuracy examples.

`python3 tests/evaluation/audit_liquid_d1.py` verifies saved accuracy responses and every latency response against frozen fixtures and recomputes the comparison. It expects the final result directories from this experiment. The initial Omni run is retained separately because its older client did not preserve raw responses; final results use the corrected client.
