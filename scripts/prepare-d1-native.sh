#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
# Re-extract and verify the fixed upstream archive and reviewed compatibility patch.
bash scripts/prepare-liquid-d1-compat.sh
cmake -S native/d1 -B dist/d1-native/build -DLLAMA_SOURCE_DIR="$PWD/dist/chinese-parity-fix/source"
cmake --build dist/d1-native/build --target ods_d1 ods_d1_error_test -j "${LLAMA_BUILD_JOBS:-6}"
dist/d1-native/build/bin/ods_d1_error_test
python3 - <<'PY'
from pathlib import Path
import shutil,hashlib
sdk=Path('lib/darwin-arm64/d1');(sdk/'lib').mkdir(parents=True,exist_ok=True)
for library in Path('dist/d1-native/build/bin').glob('*.dylib'):
 shutil.copy2(library,sdk/'lib'/library.name)
for name in ['source.sha256','patch.sha256','LICENSE-llama.cpp']:
 shutil.copy2(Path('dist/chinese-parity-fix/sdk')/name,sdk/name)
(sdk/'bridge.sha256').write_text(hashlib.sha256(Path('native/d1/bridge.cpp').read_bytes()+Path('native/d1/bridge.h').read_bytes()+Path('native/d1/CMakeLists.txt').read_bytes()).hexdigest()+'\n')
(sdk/'lib/SHA256SUMS').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in sorted((sdk/'lib').glob('*.dylib'))))
PY
