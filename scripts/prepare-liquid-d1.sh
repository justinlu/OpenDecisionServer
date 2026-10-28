#!/usr/bin/env bash
# Standalone experiment; does not replace the SDK used by ods-metal.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(uname -s)/$(uname -m)" == Darwin/arm64 ]] || { echo 'This build targets Apple Silicon Metal.' >&2; exit 1; }
python3 - <<'PY'
import hashlib
import json
from pathlib import Path
import subprocess

manifest = json.loads(Path('tests/evaluation/liquid-d1-models.json').read_text())
root = Path('dist/liquid-d1')
(root/'models').mkdir(parents=True, exist_ok=True)

def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024*1024), b''):
            h.update(block)
    return h.hexdigest()

def fetch(url, path, expected):
    if path.exists() and digest(path) == expected:
        print('Verified:', path, flush=True)
        return
    subprocess.run(['curl', '-fL', '--retry', '4', '-C', '-', url, '-o', str(path)], check=True)
    if digest(path) != expected:
        raise SystemExit('SHA-256 mismatch: '+str(path))

source = manifest['llama_cpp']
archive = root/'source.tar.gz'
fetch('https://codeload.github.com/ggml-org/llama.cpp/tar.gz/'+source['revision'], archive, source['archive_sha256'])
src = root/'source'
src.mkdir(exist_ok=True)
subprocess.run(['tar', '-xzf', str(archive), '--strip-components=1', '-C', str(src)], check=True)
for model in manifest['models']:
    for file in model['files']:
        fetch('https://huggingface.co/'+model['repository']+'/resolve/'+model['revision']+'/'+file['name'],
              root/'models'/file['name'], file['sha256'])
    for name in ('LICENSE', 'README.md'):
        subprocess.run(['curl', '-fL', '--retry', '3', 'https://huggingface.co/'+model['repository']+'/raw/'+model['revision']+'/'+name,
                        '-o', str(root/'models'/(model['id']+'-'+name))], check=True)
PY
cmake -S dist/liquid-d1/source -B dist/liquid-d1/build \
 -DLLAMA_BUILD_COMMIT=d05706b4f32a545814856e98ca518292585443b9 -DLLAMA_BUILD_NUMBER=0 \
 -DCMAKE_BUILD_TYPE=Release -DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON \
 -DGGML_BLAS=ON -DGGML_OPENMP=OFF -DLLAMA_BUILD_TESTS=OFF \
 -DLLAMA_BUILD_EXAMPLES=OFF -DLLAMA_BUILD_SERVER=ON -DLLAMA_OPENSSL=OFF
cmake --build dist/liquid-d1/build --target llama-server -j "${LLAMA_BUILD_JOBS:-6}"
