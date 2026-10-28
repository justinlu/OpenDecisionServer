#!/bin/sh
set -eu

REVISION=63be75e6586882defeb706ed4bda812d90256a6d
BASE="https://huggingface.co/LiquidAI/d1-2B-GGUF/resolve/$REVISION"
DEST="${1:-models/d1-2b}"
mkdir -p "$DEST"

hash_file() {
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | cut -d ' ' -f 1
  else sha256sum "$1" | cut -d ' ' -f 1; fi
}

target="$DEST/d1-2B-Q8_0.gguf"
expected=073313d92b53b0ac8193db6e906cb3c78a98183d244b15b7b8f5426115c6738c
if [ -f "$target" ] && [ "$(hash_file "$target")" = "$expected" ]; then
  echo "verified $target"
  exit 0
fi
curl -fL --retry 3 --silent --show-error -o "$target.tmp" "$BASE/d1-2B-Q8_0.gguf?download=true"
actual="$(hash_file "$target.tmp")"
if [ "$actual" != "$expected" ]; then
  rm -f "$target.tmp"
  echo "checksum mismatch for $target: $actual" >&2
  exit 1
fi
mv "$target.tmp" "$target"
echo "downloaded $target"
