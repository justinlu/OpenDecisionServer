"""Check the pinned structured-content goldens through a live ODS endpoint."""
import argparse
import json
import math
import urllib.request
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('--url', required=True)
parser.add_argument('--output', type=Path, required=True)
parser.add_argument('--goldens', type=Path, default=Path(__file__).resolve().parents[2] / 'internal/engine/chinese/testdata/prompt-goldens.json')
args = parser.parse_args()
records = []
for index, case in enumerate(json.loads(args.goldens.read_text())):
    request = urllib.request.Request(args.url, json.dumps(case['Request'], ensure_ascii=False).encode(), {'Content-Type': 'application/json'})
    with urllib.request.urlopen(request, timeout=120) as response:
        actual = json.loads(response.read())
    question = case['Request']['questions']['q']
    answer = actual['answers']['q']
    assert actual['model'] == 'chinese-decision-2b'
    assert actual['usage'] == dict(input_tokens=len(case['Tokens']), output_tokens=0)
    assert answer['type'] == question['type']
    keys = (list(question['criteria']) if question['type'] == 'choice' else
            [str(i) for i in range(len(question['criteria']))] if question['type'] == 'score' else ['false', 'true'])
    probabilities = answer['probabilities']
    assert set(probabilities) == set(keys)
    expected = case['Probabilities']
    values = [probabilities[key] for key in keys]
    assert all(math.isfinite(x) and 0 <= x <= 1 for x in values)
    assert abs(sum(values) - 1) <= 1e-12
    delta = max(abs(a - b) for a, b in zip(values, expected))
    assert delta <= 1e-4
    winner = max(range(len(values)), key=values.__getitem__)
    expected_winner = max(range(len(expected)), key=expected.__getitem__)
    assert winner == expected_winner
    assert abs(answer['confidence'] - (len(keys) * values[winner] - 1) / (len(keys) - 1)) <= 1e-12
    if question['type'] == 'noul':
        assert answer['noul'] == probabilities['true']
    elif question['type'] == 'choice':
        assert answer['choice'] == keys[winner]
    else:
        assert abs(answer['score'] - sum(i * p for i, p in enumerate(values))) <= 1e-12
        assert answer['legend'] == {str(i): v for i, v in enumerate(question['criteria'])}
    records.append(dict(index=index, max_probability_delta=delta, input_tokens=len(case['Tokens']), response=actual))
args.output.write_text(json.dumps(dict(passed=len(records), records=records), ensure_ascii=False, indent=2) + '\n')
print(f'Passed {len(records)} structured API goldens')
