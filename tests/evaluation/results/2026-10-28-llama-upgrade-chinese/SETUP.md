# Chinese Decision / llama.cpp upgrade reproduction

This is a native Go/cgo experiment, not a production SDK replacement. The same source is compiled twice with matching headers and libraries. Never point an old-header binary at a new library or load both Metal SDK versions into the same test process.

The frozen [reference.json](reference.json) contains 1,069 rows: the original 989 accuracy cases, 36 voice-assistant text questions, 20 native edge inputs and 24 production structured/long goldens. It records prompts, token IDs, option order, temperature and historical probabilities. There is no audio in this compatibility test.

Required local artifacts:

- Existing pinned Chinese Decision 2B Q8_0 and configuration under `dist/chinese-decision-2b-q8/`.
- Old standalone SDK under `lib/darwin-arm64/llama/`, built from the source pinned by `scripts/prepare-llama.sh`.
- New d1-compatible build under `dist/liquid-d1/`, produced by `scripts/prepare-liquid-d1.sh`, fixed to commit `d05706b4f32a545814856e98ca518292585443b9`.

From the repository root:

```sh
mkdir -p dist/llama-upgrade-compat
CGO_ENABLED=1 \
 CGO_CFLAGS="-I$PWD/lib/darwin-arm64/llama/include" \
 CGO_LDFLAGS="-L$PWD/lib/darwin-arm64/llama/lib -Wl,-rpath,$PWD/lib/darwin-arm64/llama/lib -lllama" \
 go build -tags llamavalidation -o dist/llama-upgrade-compat/probe-old ./tests/evaluation/native_probe

CGO_ENABLED=1 \
 CGO_CFLAGS="-I$PWD/dist/liquid-d1/source/include -I$PWD/dist/liquid-d1/source/ggml/include" \
 CGO_LDFLAGS="-L$PWD/dist/liquid-d1/build/bin -Wl,-rpath,$PWD/dist/liquid-d1/build/bin -lllama" \
 go build -tags llamavalidation -o dist/llama-upgrade-compat/probe-new ./tests/evaluation/native_probe

# Run one at a time. Each output directory must not exist.
dist/llama-upgrade-compat/probe-new -serial-only \
 -reference tests/evaluation/results/2026-10-28-llama-upgrade-chinese/reference.json \
 -model dist/chinese-decision-2b-q8/chinese-decision-2b-Q8_0.gguf \
 -output /tmp/chinese-llama-new -samples 120
dist/llama-upgrade-compat/probe-old -serial-only \
 -reference tests/evaluation/results/2026-10-28-llama-upgrade-chinese/reference.json \
 -model dist/chinese-decision-2b-q8/chinese-decision-2b-Q8_0.gguf \
 -output /tmp/chinese-llama-old -samples 120
```

The measured source is also frozen as `measurement.go.txt`. The `-serial-only` path excludes the previously rejected multi-sequence batching experiment; the default probe retains its previous serial+batch behavior.

Context parameters are 16,384 tokens, one sequence, batch/microbatch 512, four CPU threads, full Metal offload, flash attention disabled. Every independent question clears attention and recurrent state. Timing includes clear/decode/synchronous logits copying and calibrated softmax, excluding model load, prompt compilation, tokenization, HTTP and scheduling. Six groups rotate the existing three templates each, with six warmups and 120 formal observations per group.

The original 18083 ODS process remained resident but was not load-tested concurrently. Our separate 18084 Omni reference process was stopped during measurement and restored afterward. No model, temperature or production SDK manifest was changed.

`environment.json` records file hashes, compiler input identities and commands; `old-loaded-libraries.txt` / `new-loaded-libraries.txt` record actual mapped native libraries. `abi.c` and `abi-old.json` / `abi-new.json` compare public C struct layout using each version's own headers.

To re-audit the checked-in measurements without inference:

```sh
python3 tests/evaluation/native_probe/audit_upgrade.py \
 --results tests/evaluation/results/2026-10-28-llama-upgrade-chinese
```

The audit validates artifacts and writes `audit.json`; a zero audit exit means the saved evidence is internally consistent. Read `strict_upgrade_pass` separately: a failed predeclared migration gate is retained as a valid research result and is not converted into a pass by the audit.
