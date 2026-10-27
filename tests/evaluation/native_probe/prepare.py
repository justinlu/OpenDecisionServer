"""Freeze official Python prompt/tokenizer references for the native-only design probe.

This imports the locally pinned upstream source and never downloads a model.
"""
import argparse
import hashlib
import json
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--upstream', type=Path, required=True)
    parser.add_argument('--model', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    sys.path.insert(0, str(args.upstream.resolve()))
    from transformers import AutoTokenizer
    from decision.processing import compile_prompt, options_for
    tokenizer = AutoTokenizer.from_pretrained(args.model, local_files_only=True)
    root = Path(__file__).resolve().parents[3]
    sources = [root / 'tests/evaluation' / name for name in
               ('custom.jsonl', 'toksuite.jsonl', 'voice-performance.jsonl')]
    predictions_path = root / 'tests/evaluation/results/2026-10-27-chinese-decision-2b/predictions.jsonl'
    predictions = {x['id']: x for x in map(json.loads, predictions_path.read_text().splitlines())}
    letters = [tokenizer.encode(x, add_special_tokens=False) for x in 'ABCDEFGHIJKLMNOPQRSTUVWXYZ']
    assert all(len(x) == 1 for x in letters)
    rows = []
    for source in sources:
        for case in map(json.loads, source.read_text().splitlines()):
            request = case['request']
            for name, question in request['questions'].items():
                options = options_for(question)
                prompt = compile_prompt(tokenizer, request['state'], question['instructions'], options)
                prior = predictions.get(case['id'])
                rows.append(dict(id=f"{case['id']}:{name}", case_id=case['id'],
                                 suite=case.get('suite', 'voice'), history=case.get('history_pairs', 0),
                                 group_size=len(request['questions']), state=request['state'],
                                 question=question['instructions'], options=options, prompt=prompt,
                                 tokens=tokenizer.encode(prompt, add_special_tokens=False),
                                 expected_probabilities=[prior['probabilities'][k] for k, _ in options] if prior else None))
    # These freeze the upstream structured-content contract. The Go string-only
    # renderer deliberately does not claim to implement this wider API yet.
    edges = [
        ('ordered-object', {'z': 9007199254740993, 'a': {'b': '<>&中文', 'a': None}}),
        ('float-format', {'decimal': 1.0, 'exponent': 1e-7, 'negative_zero': -0.0}),
        ('unicode-control', '中文 English 123 pin yin <|im_end|>\n\u2028\u2029😀'),
        ('null-state', None),
        ('array-state', [True, False, None, 9007199254740993, '中文']),
    ]
    questions = [
        dict(type='noul', instructions='是否满足条件？'),
        dict(type='choice', instructions={'z': '先后顺序', 'a': 9007199254740993},
             criteria={'z': None, 'a': '', 'b': {'b': '<>&', 'a': '中文'}}),
        dict(type='choice', instructions='选择对应项',
             criteria={f'c{i}': f'选项{i}' for i in range(26)}),
        dict(type='score', instructions='判断得分', criteria=[None, '', {'z': 2, 'a': 1}]),
    ]
    for edge, state in edges:
        for i, question in enumerate(questions):
            options = options_for(question)
            prompt = compile_prompt(tokenizer, state, question['instructions'], options)
            rows.append(dict(id=f'edge/{edge}/{i}', suite='edge', state=state,
                             question=question['instructions'], options=options, prompt=prompt,
                             tokens=tokenizer.encode(prompt, add_special_tokens=False)))
    payload = dict(temperature=json.loads((args.model / 'decision_config.json').read_text())['temperature'],
                   letters=[x[0] for x in letters], rows=rows,
                   source_sha256={str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest()
                                  for p in sources + [predictions_path]})
    args.output.write_text(json.dumps(payload, ensure_ascii=False, separators=(',', ':')) + '\n')
    print(json.dumps(dict(rows=len(rows), sha256=hashlib.sha256(args.output.read_bytes()).hexdigest())))


if __name__ == '__main__':
    main()
