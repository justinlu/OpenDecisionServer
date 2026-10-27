"""Export raw request / prompt / token / probability goldens from pinned Python."""
import argparse
import json
import sys
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('--upstream', type=Path, required=True)
parser.add_argument('--model', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
args = parser.parse_args()
sys.path.insert(0, str(args.upstream.resolve()))
from decision.processing import options_for, compile_prompt
from decision.serving.backends import GGUFBackend, candidate_probabilities

backend = GGUFBackend(args.model, args.model / 'chinese-decision-2b-Q8_0.gguf', threads=4)
temperature = json.loads((args.model / 'decision_config.json').read_text())['temperature']
states = [
    {'z': 9007199254740993, 'a': {'b': '<>&中文', 'a': None}},
    {'decimal': 1.0, 'exponent': 1e-7, 'negative_zero': -0.0},
    '中文 English 123 pin yin <|im_end|>\n\u2028\u2029😀',
    None,
    [True, False, None, 9007199254740993, '中文'],
    '用户要求修改地址。' * 100,  # crosses native 512-token decode chunks
]
questions = [
    dict(type='noul', instructions='是否满足条件？'),
    dict(type='choice', instructions={'z': '先后顺序', 'a': 9007199254740993},
         criteria={'z': None, 'a': '', 'b': {'b': '<>&', 'a': '中文'}}),
    dict(type='choice', instructions='选择对应项',
         criteria={f'c{i}': f'选项{i}' for i in range(26)}),
    dict(type='score', instructions='判断得分', criteria=[None, '', {'z': 2, 'a': 1}]),
]
rows = []
try:
    for state in states:
        for question in questions:
            request = dict(model='chinese-decision-2b', state=state, questions={'q': question})
            options = options_for(question)
            prompt = compile_prompt(backend.tokenizer, state, question['instructions'], options)
            ids = backend.tokenizer.encode(prompt, add_special_tokens=False)
            backend.validate_tokens(prompt, ids)
            probabilities = candidate_probabilities(backend.logits(ids, len(options)), temperature)
            rows.append(dict(Request=request, Prompt=prompt, Tokens=ids, Probabilities=probabilities))
finally:
    backend.close()
args.output.parent.mkdir(parents=True, exist_ok=True)
args.output.write_text(json.dumps(rows, ensure_ascii=False, indent=2) + '\n')
print(f'Exported {len(rows)} goldens')
