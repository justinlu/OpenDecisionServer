# Liquid d1 GGUF evaluation

User-authorized experiment: build and run native llama.cpp first; do not integrate a new Go engine or change the default model.

## Fixed approach

- Pin llama.cpp to the merge of upstream decision support, `d05706b4f32a545814856e98ca518292585443b9`.
- Download official `d1-Omni-2B` first and `d1-2B` for comparison. Use Q8_0 language weights and F16 multimodal projectors, verify upstream SHA-256 hashes, and keep binaries/weights under ignored `dist/liquid-d1/`.
- Run the servers one at a time with Metal offload and identical parameters. Preserve the existing Chinese Decision SDK and service.
- Reuse the fixed 504 custom and 485 TokSuite cases, original labels and probability-argmax scoring. Keep every timeout and malformed response in the denominator.
- Reuse the fixed voice-assistant text latency cases, 120 observations per condition, 1/4 clients, 0/4/12 history pairs, 1/3 questions. Identify these as text inputs, not recorded audio.
- Fix a small image/audio smoke dataset before observing its outputs. Label generated speech as synthetic and keep multimedia diagnostics separate from the text benchmark.
- Record model/source/fixture hashes, commands, raw responses, p50/p95/p99, throughput, errors and process memory. Compare existing baselines as historical measurements, not simultaneous controlled reruns.

## Tasks

- [ ] Build the pinned server and verify both model bundles.
- [ ] Run text and multimedia smoke checks; establish actual upstream response semantics.
- [ ] Run both complete accuracy suites and the fixed latency matrix sequentially.
- [ ] Audit saved predictions and measurements; publish a reproducible report with limits and model-selection advice.
- [ ] Commit and push this experiment, merge into main, verify remote containment and remove the fully merged task branch.

## Review focus

No test-label or prompt tuning after predictions; no hidden score conversion; no claim that upstream question-by-question llama.cpp execution reproduces Transformers' packed-set optimization; no conflation of cached repeated-text latency with new-input latency; no production default switch from synthetic media or correlated diagnostic variants.
