"""Recompute native ODS acceptance from persisted HTTP observations."""
import argparse
import json
from collections import defaultdict
from pathlib import Path
from benchmark_latency import metrics

parser = argparse.ArgumentParser()
parser.add_argument('--results', type=Path, required=True)
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
base = {r['id']: r for r in map(json.loads, (root / 'tests/evaluation/results/2026-10-27-chinese-decision-2b/predictions.jsonl').read_text().splitlines())}
actual = list(map(json.loads, (args.results / 'accuracy/predictions.jsonl').read_text().splitlines()))
assert len(actual) == len({r['id'] for r in actual}) == len(base) == 989
assert {r['id'] for r in actual} == set(base)
maximum, mismatches = 0., 0
for row in actual:
    old = base[row['id']]
    assert row['error'] is None and row['returned_model'] == 'chinese-decision-2b'
    assert set(row['probabilities']) == set(old['probabilities'])
    mismatches += row['predicted'] != old['predicted']
    maximum = max(maximum, max(abs(row['probabilities'][key] - p) for key, p in old['probabilities'].items()))
comparison = dict(rows=len(actual), winner_mismatches=mismatches, max_probability_delta=maximum,
                  tolerance=1e-4, passed=mismatches == 0 and maximum <= 1e-4)
assert comparison['passed']
(args.results / 'python-comparison.json').write_text(json.dumps(comparison, indent=2) + '\n')

groups = defaultdict(list)
for record in map(json.loads, (args.results / 'latency/samples.jsonl').read_text().splitlines()):
    groups[tuple(record[k] for k in ('history_pairs', 'questions', 'clients'))].append(record)
summary = json.loads((args.results / 'latency/summary.json').read_text())
assert len(groups) == len(summary) == 12
for group in summary:
    records = groups[tuple(group[k] for k in ('history_pairs', 'questions', 'clients'))]
    assert len(records) == 120
    recomputed = metrics(records, group['elapsed_seconds'], 100.)
    for key, value in recomputed.items():
        assert group[key] == value, (key, group[key], value)
    assert group['errors'] == 0
contract = json.loads((args.results / 'structured-contract.json').read_text())
assert contract['passed'] == len(contract['records']) == 24
assert all(row['max_probability_delta'] <= 1e-4 for row in contract['records'])

def counters(name):
    return {key: float(value) for key, value in
            (line.split() for line in (args.results / name).read_text().splitlines())}
before, after = counters('metrics-before.txt'), counters('metrics-after.txt')
warmup = json.loads((args.results / 'latency/metadata.json').read_text())['warmup']
expected_requests = before['ods_http_requests_total'] + len(actual) + contract['passed'] + sum(g['attempted'] + warmup for g in summary)
expected_runs = before['ods_inference_runs_total'] + len(actual) + contract['passed'] + sum((g['attempted'] + warmup) * g['questions'] for g in summary)
assert after['ods_http_requests_total'] == expected_requests == 2525
assert after['ods_inference_runs_total'] == expected_runs == 4038
assert after['ods_max_concurrent_run_calls'] == 1
for key in ('ods_active_runs', 'ods_waiting_requests', 'ods_http_errors_total',
            'ods_native_waiting_requests', 'ods_native_reserved_tokens',
            'ods_native_errors_total', 'ods_native_rejections_total'):
    assert after[key] == 0, (key, after[key])
result = dict(accuracy_parity=comparison, structured_cases=24, latency_observations=sum(map(len, groups.values())),
              latency_groups=12, latency_errors=0, checked_http_requests=int(expected_requests),
              checked_native_runs=int(expected_runs), native_concurrency=1,
              idle_queue_and_reserved_tokens_zero=True, peak_rss_bytes=int(after['ods_process_max_rss_bytes']))
(args.results / 'acceptance.json').write_text(json.dumps(result, indent=2) + '\n')
print(json.dumps(result, indent=2))
