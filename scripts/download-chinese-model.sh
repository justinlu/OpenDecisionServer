#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
model_dir="${CHINESE_MODEL_DIR:-$PWD/models/chinese-decision-2b}"
url=https://huggingface.co/quincyqiang/chinese-decision-2b-gguf/resolve/2357aa9aad05cbd87bcfc11a1b74e5705e3b5bbc
mkdir -p "$model_dir"
while read -r sha name; do
 if [[ ! -f "$model_dir/$name" ]]; then
  curl -fL --retry 3 "$url/$name" -o "$model_dir/$name.partial"
  printf '%s  %s\n' "$sha" "$model_dir/$name.partial" | shasum -a 256 -c -
  mv "$model_dir/$name.partial" "$model_dir/$name"
 fi
 printf '%s  %s\n' "$sha" "$model_dir/$name" | shasum -a 256 -c -
done <<'FILES'
4f902f73781f87cc8388617bc7e2e99eb14fdeb8551f7ebb48ffbde69394cc02 chinese-decision-2b-Q8_0.gguf
7e5dacec4c914dfb13c6582d23f423272a24bc631788d591f6585a5a7f16b7d9 decision_config.json
273d8e0e683b885071fb17e08d71e5f2a5ddfb5309756181681de4f5a1822d80 chat_template.jinja
a5cd9732badce41de57e6efce8302930ded1c1188c5f81feb2bd6c24c4a1941f tokenizer.json
fb6a894aa514307c68e32d1481080013ad389c038bd1c8781720deae114eccb8 tokenizer_config.json
50cbab8a892c5f2993b8c7351a99182507472def3b1374558308605d99b86b32 LICENSE
4e178cb46a1aab584fb4b94ce7d5369a004d2e7e7ab979647e48797768fc7372 THIRD_PARTY_NOTICES.md
f20aacf5315a261685288ba058ca4d32a6cc6fc5d138b357153e68f5ec63ecce README.md
FILES
