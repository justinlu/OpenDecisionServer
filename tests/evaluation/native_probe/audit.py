"""Independently audit persisted native outputs, without running inference again."""
import argparse
import json
import math
from collections import Counter, defaultdict
from pathlib import Path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--reference', type=Path, required=True)
    parser.add_argument('--results', type=Path, required=True)
    args = parser.parse_args()
    reference = json.loads(args.reference.read_text())
    rows = {r['id']: r for r in reference['rows']}
    phases = defaultdict(dict)
    batch_sizes = Counter()
    for record in map(json.loads, (args.results / 'parity.jsonl').read_text().splitlines()):
        phase, name = record['phase'], record['id']
        assert name in rows and name not in phases[phase], (phase, name)
        probabilities, logits = record['probabilities'], record['logits']
        assert len(probabilities) == len(logits) == len(rows[name]['options'])
        assert all(math.isfinite(x) for x in logits)
        assert all(math.isfinite(p) and 0 <= p <= 1 for p in probabilities)
        assert abs(sum(probabilities) - 1) <= 1e-12
        phases[phase][name] = record
        if phase == 'batch':
            batch_sizes[record['batch_size']] += 1
    assert set(phases) == {'serial', 'batch'}
    assert all(set(records) == set(rows) for records in phases.values())
    summary = json.loads((args.results / 'summary.json').read_text())
    # Predeclared tolerance is unchanged; include A/B/A explicitly rather than
    # relying on an aggregate flag from the native measurement program.
    baseline_path = Path(__file__).resolve().parents[1] / 'results/2026-10-27-chinese-decision-2b/predictions.jsonl'
    baseline = {r['id']: r for r in map(json.loads, baseline_path.read_text().splitlines())}
    grades = defaultdict(lambda: Counter(total=0, correct=0, winner_mismatches=0))
    differences = Counter()
    winner = lambda values: max(range(len(values)), key=values.__getitem__)
    for name, row in rows.items():
        serial = phases['serial'][name]['probabilities']
        batch = phases['batch'][name]['probabilities']
        differences['batch_max_probability_delta'] = max(differences['batch_max_probability_delta'],
                                                        max(abs(a - b) for a, b in zip(serial, batch)))
        differences['batch_winner_mismatches'] += winner(serial) != winner(batch)
        if row.get('expected_probabilities') is not None:
            expected = row['expected_probabilities']
            differences['python_max_probability_delta'] = max(differences['python_max_probability_delta'],
                                                              max(abs(a - b) for a, b in zip(serial, expected)))
            differences['python_winner_mismatches'] += winner(serial) != winner(expected)
            old = baseline[row['case_id']]
            for phase in ('serial', 'batch'):
                probability = phases[phase][name]['probabilities']
                predicted = row['options'][winner(probability)][0]
                grade = grades[f"{phase}/{row['suite']}"]
                grade['total'] += 1
                grade['correct'] += predicted == old['expected']
                grade['winner_mismatches'] += predicted != old['predicted']
    for key, value in differences.items():
        assert abs(summary[key] - value) <= 1e-12, (key, summary[key], value)
    serial_pass = differences['python_winner_mismatches'] == 0
    serial_pass &= differences['python_max_probability_delta'] <= 1e-4
    serial_pass &= summary['serial_aba_max_probability_delta'] <= 1e-4
    batch_pass = differences['batch_winner_mismatches'] == 0
    batch_pass &= differences['batch_max_probability_delta'] <= 1e-4
    batch_pass &= summary['repeated_permutation_max_probability_delta'] <= 1e-4
    parity_pass = serial_pass and batch_pass
    assert parity_pass == summary['parity_pass']
    latency = defaultdict(list)
    for record in map(json.loads, (args.results / 'latency.jsonl').read_text().splitlines()):
        value = record['latency_ms']
        assert math.isfinite(value) and value >= 0
        latency[f"{record['mode']}/h{record['history']}/q{record['questions']}"].append(value)
    assert set(latency) == set(summary['latency'])
    for group, values in latency.items():
        assert len(values) == summary['samples_per_latency_group']
        values.sort()
        assert values[math.ceil(.95 * len(values)) - 1] == summary['latency'][group]['p95_ms']
    result = dict(parity_pass=bool(parity_pass), serial_parity_pass=bool(serial_pass),
                  batch_parity_pass=bool(batch_pass), finite_logits_checked=True,
                  exact_row_coverage=len(rows), observed_batch_sizes=dict(batch_sizes),
                  latency_samples=sum(map(len, latency.values())), grades=dict(grades),
                  recomputed_differences=dict(differences))
    (args.results / 'audit.json').write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(result, indent=2))
    if not parity_pass:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
