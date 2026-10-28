#!/usr/bin/env bash
# Isolated candidate SDK; never replaces the SDK used by ods-metal.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(uname -s)/$(uname -m)" == Darwin/arm64 ]] || { echo 'Requires Apple Silicon Metal.' >&2; exit 1; }
python3 - <<'PY'
import hashlib
import json
from pathlib import Path
import subprocess

source = json.loads(Path('tests/evaluation/liquid-d1-models.json').read_text())['llama_cpp']
archive = Path('dist/liquid-d1/source.tar.gz')
if not archive.is_file():
    archive.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(['curl', '-fL', '--retry', '3',
        'https://codeload.github.com/ggml-org/llama.cpp/tar.gz/'+source['revision'],
        '-o', str(archive)], check=True)
if hashlib.sha256(archive.read_bytes()).hexdigest() != source['archive_sha256']:
    raise SystemExit('Pinned source archive SHA-256 mismatch.')
root = Path('dist/chinese-parity-fix')
src = root/'source'
src.mkdir(parents=True, exist_ok=True)
subprocess.run(['tar', '-xzf', str(archive), '--strip-components=1', '-C', str(src)], check=True)
patch = Path('tests/evaluation/patches/llama-d05706b-qwen35-late-crop.patch').resolve()
subprocess.run(['patch', '--batch', '--fuzz=0', '-p1', '-i', str(patch)], cwd=src, check=True)
(root/'build-inputs.json').write_text(json.dumps(dict(source=source,
    patch_sha256=hashlib.sha256(patch.read_bytes()).hexdigest()), indent=2)+'\n')
PY
cmake -S dist/chinese-parity-fix/source -B dist/chinese-parity-fix/build \
 -DLLAMA_BUILD_COMMIT=d05706b4f32a545814856e98ca518292585443b9 -DLLAMA_BUILD_NUMBER=0 \
 -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=ON \
 -DCMAKE_INSTALL_RPATH='@loader_path' -DCMAKE_BUILD_WITH_INSTALL_RPATH=ON \
 -DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON -DGGML_BLAS=ON \
 -DGGML_OPENMP=OFF -DLLAMA_BUILD_TESTS=OFF -DLLAMA_BUILD_EXAMPLES=OFF \
 -DLLAMA_BUILD_SERVER=ON -DLLAMA_BUILD_TOOLS=ON -DLLAMA_BUILD_APP=OFF -DLLAMA_OPENSSL=OFF
cmake --build dist/chinese-parity-fix/build --target llama-server -j "${LLAMA_BUILD_JOBS:-6}"
python3 - <<'PY'
import hashlib
import json
from pathlib import Path
import shutil

root = Path('dist/chinese-parity-fix')
sdk = root/'sdk'
for name in ('include', 'lib'):
    (sdk/name).mkdir(parents=True, exist_ok=True)
for directory in ('include', 'ggml/include'):
    for header in (root/'source'/directory).glob('*.h'):
        shutil.copy2(header, sdk/'include'/header.name)
for name in ('mtmd.h', 'mtmd-helper.h'):
    shutil.copy2(root/'source/tools/mtmd'/name, sdk/'include'/name)
for lib in (root/'build/bin').glob('*.dylib'):
    shutil.copy2(lib, sdk/'lib'/lib.name)
inputs = json.loads((root/'build-inputs.json').read_text())
(sdk/'source.sha256').write_text(inputs['source']['archive_sha256']+'\n')
(sdk/'patch.sha256').write_text(inputs['patch_sha256']+'\n')
shutil.copy2(root/'source/LICENSE', sdk/'LICENSE-llama.cpp')
(sdk/'lib/SHA256SUMS').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n'
    for p in sorted((sdk/'lib').glob('*.dylib'))))
PY
echo 'Candidate SDK: dist/chinese-parity-fix/sdk; production SDK unchanged.'
