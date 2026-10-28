#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
case "${1:-d1-omni-2b}" in
 d1-omni-2b) alias=d1-omni-2b; name=d1-Omni-2B ;;
 d1-2b) alias=d1-2b; name=d1-2B ;;
 *) echo 'Usage: bash scripts/run-liquid-d1.sh [d1-omni-2b|d1-2b] [port]' >&2; exit 2 ;;
esac
exec dist/liquid-d1/build/bin/llama-server \
 --model "dist/liquid-d1/models/$name-Q8_0.gguf" \
 --mmproj "dist/liquid-d1/models/mmproj-$name-F16.gguf" \
 --alias "$alias" --host 127.0.0.1 --port "${2:-18084}" \
 --n-gpu-layers 99 --ctx-size 8192 --batch-size 8192 --ubatch-size 8192 \
 --parallel 1 --threads 4 --threads-batch 4 --cache-ram 0 \
 --no-context-shift --metrics --no-webui
