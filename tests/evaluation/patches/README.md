# d05706b Qwen3.5 compatibility patch

`llama-d05706b-qwen35-late-crop.patch` restores the two row-selection conditions
from the pinned llama-cpp-python 0.3.36 Qwen3.5 graph. With default context flags,
the last FFN and output normalization process all rows before selecting logits
rows. Upstream d05706b selects rows before the last FFN, changing the Metal
matrix shape and slightly changing decision logits.

The patch only changes `src/models/qwen35.cpp`. It does not change LFM3 graphs,
weights, tokenizer, temperature, batch sizes, or the public C ABI. This is our
local compatibility patch, not an upstream release or an upstream-endorsed fix.

Build an isolated candidate with `bash scripts/prepare-liquid-d1-compat.sh`.
The archive is pinned by revision and SHA-256. `build-inputs.json` and the SDK's
`patch.sha256` record the patch identity; distribute the patch together with the
pinned source license. Candidate output is under `dist/chinese-parity-fix/`;
the production SDK and its runtime identity guard are unchanged.

The probe's `-full-hidden` flag is a diagnostic only: it calls a pinned C++
staging API to force late cropping and additionally copies hidden states. The
candidate patch requires neither that API nor that output allocation. Upgrade
auditing rejects measurements taken with the diagnostic flag enabled.
