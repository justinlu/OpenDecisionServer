#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
sdk="${LLAMA_SDK:-$PWD/lib/darwin-arm64/llama}"
[[ -f "$sdk/include/llama.h" && -f "$sdk/lib/SHA256SUMS" ]] || { echo "Run scripts/prepare-llama.sh first" >&2; exit 1; }
[[ "$(cat "$sdk/source.sha256")" == 832db0699007f1be95a7e41ef12e88926b02ba836461e36a36372db2760c1a2e ]] || { echo "Wrong SDK source" >&2; exit 1; }
(cd "$sdk/lib"; shasum -a 256 -c SHA256SUMS)
mkdir -p bin
CGO_ENABLED=1 CGO_CFLAGS="-I$sdk/include ${CGO_CFLAGS:-}" \
 go build -tags llama -o bin/ods-metal ./cmd/ods
echo "Built bin/ods-metal; distribute it with $sdk/lib and the pinned model/config files."
