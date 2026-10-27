# Multilingual decision evaluation implementation plan

> Execute inline using executing-plans; user requested a usable representative dataset and authorized designing missing coverage.

Goal: research reusable public data, add a reproducible Chinese/English/digit/pinyin decision dataset, and measure the current local model without modifying inference behavior.

Design: keep externally authored robustness questions separate from original context-grounded decisions. Original data uses 24 semantic families across eight domains, each with nine input forms and Chinese/English prompts (432 rows), plus 12 numeric families with ASCII/fullwidth/leading-zero forms and two prompt languages (72 rows). Each row has a fixed answer and explanation; transformations share family IDs. No model-generated answer labels or post-result label changes.

Files: `tests/evaluation/{seeds.json,build_dataset.py,custom.jsonl,evaluate.py,test_evaluation.py,README.md}`, source manifests and external JSONL, and `docs/research/chinese-decision-datasets.md`.

Constraints: Python standard library for building custom data and running HTTP evaluation; explicit optional external import dependency only. No inference changes. Public knowledge questions must not be pooled with context-grounded decision results. No accuracy claim for unseen traffic. Keep request errors in denominator. Score by highest option probability, never rounded expected score. Do not count correlated variants as independent evidence. Preserve pure-digit states as JSON strings.

Review focus: family semantic equivalence and negatives; pinyin/English separation; answer-order bias; malformed responses/error accounting; reproducible sources/licensing.

- [x] Task 1: research primary sources and pin any imported external data; record suitability and limits.
- [x] Task 2: write failing dataset/scoring/HTTP-error tests; author fixed semantic seeds; build and validate dataset deterministically.
- [x] Task 3: implement HTTP evaluation with per-source/form/prompt/domain/type breakdowns, confusion matrices, majority baselines, paired correctness/consistency, per-row predictions and reproducibility metadata; test fixtures before live run.
- [x] Task 4: run local model evaluation, record results and caveats, request fresh independent review, fix material issues, run Python/Go checks, commit/push/merge/verify remote main and safely remove this task's branch.

Execution notes: baseline Go suite passed before edits. Work proceeds on a clean dedicated branch in the existing checkout to reuse 600 MB ignored model assets; no other worktrees exist. User's standing Git authorization covers final integration.

Verification ledger: authored504 and external485 immutable cases; local live runs290/504 and238/485 with zero request errors. Fresh review checked all24 authored seed families, found malformed response and F1 issues; both reproduced by failing tests and fixed. Added truncated-response regression after a failing reproduction. Labels and inference code unchanged. Final checks: 13 Python tests and Go suite passed; recorded result labels and inputs match the fixed datasets. Integration uses the standing authorization and is verified against remote main in the delivery session. The upstream source card is retained byte-for-byte, including its two trailing-whitespace lines, to preserve its recorded hash.
