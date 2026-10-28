#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
[[ -f lib/darwin-arm64/d1/lib/libods_d1.dylib ]] || { echo 'Run bash scripts/prepare-d1-native.sh' >&2; exit 1; }
# Chinese remains an explicit selectable cgo engine compiled against its pinned
# headers. Only the selected engine's DSO is loaded in a process.
bash scripts/build-llama.sh
