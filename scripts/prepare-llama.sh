#!/usr/bin/env bash
# Build a standalone Metal SDK from the exact source used by the parity probe.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(uname -s)/$(uname -m)" == Darwin/arm64 ]] || { echo "Metal SDK requires Darwin/arm64" >&2; exit 1; }
sdk="${LLAMA_SDK:-$PWD/lib/darwin-arm64/llama}"
archive="${LLAMA_SOURCE_ARCHIVE:-$PWD/dist/llama_cpp_python-0.3.36.tar.gz}"
source_dir="$PWD/dist/llama-source-0.3.36"
build_dir="$PWD/dist/llama-build-0.3.36"
sha=832db0699007f1be95a7e41ef12e88926b02ba836461e36a36372db2760c1a2e
mkdir -p "$(dirname "$archive")" "$sdk"
if [[ ! -f "$archive" ]]; then
 curl -fL --retry 3 "https://files.pythonhosted.org/packages/source/l/llama-cpp-python/llama_cpp_python-0.3.36.tar.gz" -o "$archive"
fi
printf '%s  %s\n' "$sha" "$archive" | shasum -a 256 -c -
mkdir -p "$source_dir"
tar -xzf "$archive" --strip-components=1 -C "$source_dir"
cmake -S "$source_dir/vendor/llama.cpp" -B "$build_dir" \
 -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=ON \
 -DCMAKE_INSTALL_RPATH='@loader_path' -DCMAKE_BUILD_WITH_INSTALL_RPATH=ON \
 -DLLAMA_BUILD_COMMON=OFF -DLLAMA_BUILD_TESTS=OFF -DLLAMA_BUILD_TOOLS=OFF \
 -DLLAMA_BUILD_EXAMPLES=OFF -DLLAMA_BUILD_SERVER=OFF -DLLAMA_BUILD_APP=OFF \
 -DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON -DGGML_BLAS=ON \
 -DGGML_OPENMP=OFF -DLLAMA_OPENSSL=OFF
cmake --build "$build_dir" --config Release -j "${LLAMA_BUILD_JOBS:-4}"
cmake --install "$build_dir" --prefix "$sdk"
printf '%s\n' "$sha" > "$sdk/source.sha256"
(cd "$sdk/lib"; shasum -a 256 ./*.dylib > SHA256SUMS)
cp "$source_dir/vendor/llama.cpp/LICENSE" "$sdk/LICENSE-llama.cpp"
cp "$source_dir/LICENSE.md" "$sdk/LICENSE-llama-cpp-python"
echo "Standalone Metal SDK: $sdk"
