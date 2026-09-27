#!/bin/sh
set -eu

REVISION=82a2fadf8fccfccdc5fd4e1009ba8f1a265eb7a8
BASE="https://huggingface.co/SupersonicLabs/Julia-1-ONNX/resolve/$REVISION"
DEST="${1:-models/julia-1}"
mkdir -p "$DEST"

hash_file() {
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | cut -d ' ' -f 1
  else sha256sum "$1" | cut -d ' ' -f 1; fi
}

fetch() {
  name="$1"; expected="$2"; target="$DEST/$name"
  if [ -f "$target" ] && [ "$(hash_file "$target")" = "$expected" ]; then
    echo "verified $target"
    return
  fi
  curl -fL --retry 3 --silent --show-error -o "$target.tmp" "$BASE/$name?download=true"
  actual="$(hash_file "$target.tmp")"
  if [ "$actual" != "$expected" ]; then
    rm -f "$target.tmp"
    echo "checksum mismatch for $name: $actual" >&2
    exit 1
  fi
  mv "$target.tmp" "$target"
  echo "downloaded $target"
}

fetch model.onnx 97141d0cfb1da6204e9f8f24d581af72eaeb82cda21149d83eaa6df7160fbcd9
fetch model.onnx.data fd915be810d7ebfb80fb05a48dd33c9484d17ae1b6bcb9e1f544cbaaa913ded1
fetch tokenizer.json 609d8f4c067cd3950f88594c5a802616cea245823836ef5848ee4fc40aab5b6f
fetch parity-cases.json 534670f25823f3f9abf9e812500918fabc606682dfd4e2456d827889d9bdca20
