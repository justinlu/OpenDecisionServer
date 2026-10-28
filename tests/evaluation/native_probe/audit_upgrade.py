"""Audit old/new serial llama.cpp compatibility from persisted logits and probabilities."""
import argparse
from collections import Counter, defaultdict
import json
import math
from pathlib import Path
import struct


def load_rows(path):
    return [json.loads(line) for line in path.read_text().splitlines() if line.strip()]


def winner(values):
    return max(range(len(values)), key=values.__getitem__)


def delta(a, b):
    assert len(a) == len(b)
    return max(abs(x-y) for x, y in zip(a, b))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--results', type=Path, required=True)
    parser.add_argument('--require-pass', action='store_true', help='exit nonzero if the original strict migration gate fails')
    args = parser.parse_args()
    root = args.results
    # A rejected rerun must not leave a previous successful certificate behind.
    for name in ['audit.json', 'differences.jsonl']:
        (root/name).unlink(missing_ok=True)
    reference = json.loads((root/'reference.json').read_text())
    expected = {row['id']: row for row in reference['rows']}
    assert len(expected) == len(reference['rows']) == 1069
    datasets = Path(__file__).resolve().parents[1]
    cases = {row['id']: row for name in ['custom.jsonl', 'toksuite.jsonl'] for row in load_rows(datasets/name)}
    assert len(cases) == 989
    report = {'probability_tolerance': 1e-4, 'rows': len(expected), 'versions': {}}
    outputs = {}
    for version in ['old', 'new']:
        phases = defaultdict(dict)
        for record in load_rows(root/version/'parity.jsonl'):
            phase, id = record['phase'], record['id']
            assert id in expected and id not in phases[phase]
            # Go serializes []float32 using shortest float32-roundtripping decimals.
            # Recover their original binary32 values before float64 softmax auditing.
            record['logits'] = [struct.unpack('<f', struct.pack('<f', x))[0] for x in record['logits']]
            probs, logits = record['probabilities'], record['logits']
            assert len(probs) == len(logits) == len(expected[id]['options'])
            assert all(math.isfinite(x) for x in logits)
            assert all(math.isfinite(p) and 0 <= p <= 1 for p in probs)
            assert abs(sum(probs)-1) <= 1e-12
            values = [float(x)/reference['temperature'] for x in logits]
            peak = max(values)
            values = [math.exp(x-peak) for x in values]
            total = sum(values)
            assert delta([x/total for x in values], probs) <= 1e-12
            phases[phase][id] = record
        assert set(phases) == {'serial', 'aba-between', 'aba-repeat'}
        assert set(phases['serial']) == set(expected)
        assert len(phases['aba-between']) == len(phases['aba-repeat']) == 1
        first, last = reference['rows'][0]['id'], reference['rows'][-1]['id']
        assert set(phases['aba-repeat']) == {first} and set(phases['aba-between']) == {last}
        aba = delta(phases['serial'][first]['probabilities'], phases['aba-repeat'][first]['probabilities'])
        outputs[version] = phases['serial']
        summary = json.loads((root/version/'summary.json').read_text())
        assert summary.get('full_hidden', False) is False, 'Diagnostic full-hidden mode cannot certify an SDK upgrade'
        historical_deltas = [delta(expected[id]['expected_probabilities'], r['probabilities'])
                             for id, r in phases['serial'].items() if expected[id].get('expected_probabilities')]
        historical_flips = sum(winner(expected[id]['expected_probabilities']) != winner(r['probabilities'])
                               for id, r in phases['serial'].items() if expected[id].get('expected_probabilities'))
        assert summary['tokenizer_rows'] == len(expected)
        assert summary['python_reference_rows'] == len(historical_deltas) == 1013
        assert summary['python_max_probability_delta'] == max(historical_deltas)
        assert summary['python_winner_mismatches'] == historical_flips
        assert summary['serial_aba_max_probability_delta'] == aba
        assert summary['serial_only']
        assert summary['parity_pass'] == (historical_flips == 0 and max(historical_deltas) <= 1e-4 and aba <= 1e-4)
        assert summary['serial_context'] == dict(n_ctx=16384, n_ctx_seq=16384, n_batch=512, n_ubatch=512, n_seq_max=1)
        grades = defaultdict(Counter)
        for id, record in phases['serial'].items():
            case = cases.get(expected[id].get('case_id'))
            if case:
                selected = expected[id]['options'][winner(record['probabilities'])][0]
                grades[case['suite']]['total'] += 1
                grades[case['suite']]['correct'] += selected == case['expected']
        assert sum(g['total'] for g in grades.values()) == 989
        timings = defaultdict(list)
        for sample in load_rows(root/version/'latency.jsonl'):
            assert sample['mode'] == 'serial' and math.isfinite(sample['latency_ms']) and sample['latency_ms'] >= 0
            timings[f"serial/h{sample['history']}/q{sample['questions']}"].append(sample['latency_ms'])
        assert set(timings) == set(summary['latency']) and len(timings) == 6
        latency = {}
        for name, values in timings.items():
            assert len(values) == summary['samples_per_latency_group'] == 120
            values.sort()
            latency[name] = dict(samples=len(values), p50_ms=values[59], p95_ms=values[113], p99_ms=values[118], max_ms=values[-1])
            assert all(summary['latency'][name][k] == v for k, v in latency[name].items() if k != 'p99_ms')
        report['versions'][version] = dict(full_hidden=summary.get('full_hidden', False), grades={k:dict(v) for k,v in grades.items()},
                                          tokenizer_rows=len(expected), historical_probability_max=max(historical_deltas),
                                          historical_winner_changes=historical_flips, aba_delta=aba, latency=latency,
                                          recorded_strict_gate_pass=summary['parity_pass'])
    differences = []
    for id, row in expected.items():
        old, new = outputs['old'][id], outputs['new'][id]
        d = delta(old['probabilities'], new['probabilities'])
        old_index, new_index = winner(old['probabilities']), winner(new['probabilities'])
        differences.append(dict(id=id, suite=row['suite'], case_id=row.get('case_id', id), max_probability_delta=d,
                                max_logit_delta=delta(old['logits'], new['logits']), winner_changed=old_index != new_index,
                                old_winner=row['options'][old_index][0], new_winner=row['options'][new_index][0],
                                old_probabilities=old['probabilities'], new_probabilities=new['probabilities']))
    corpus = [r for r in differences if r['case_id'] in cases]
    goldens = [r for r in differences if r['suite'] == 'production-golden']
    assert len(corpus) == 989 and len(goldens) == 24
    report['comparison'] = dict(max_probability_delta=max(r['max_probability_delta'] for r in differences),
                                max_logit_delta=max(r['max_logit_delta'] for r in differences),
                                corpus_max_probability_delta=max(r['max_probability_delta'] for r in corpus),
                                golden_max_probability_delta=max(r['max_probability_delta'] for r in goldens),
                                rows_above_tolerance=sum(r['max_probability_delta'] > 1e-4 for r in differences),
                                winner_changes=sum(r['winner_changed'] for r in differences),
                                corpus_winner_changes=sum(r['winner_changed'] for r in corpus),
                                changed_rows=[r for r in differences if r['winner_changed']])
    report['strict_upgrade_pass'] = report['comparison']['winner_changes'] == 0
    report['strict_upgrade_pass'] &= report['comparison']['max_probability_delta'] <= 1e-4
    report['strict_upgrade_pass'] &= all(r['aba_delta'] <= 1e-4 for r in report['versions'].values())
    report['performance_ratios_new_over_old'] = {name:report['versions']['new']['latency'][name]['p95_ms']/old['p95_ms']
                                                for name,old in report['versions']['old']['latency'].items()}
    replay_ref = root/'replay-reference.json'
    if replay_ref.exists():
        replay_expected = {r['id']:r for r in json.loads(replay_ref.read_text())['rows']}
        report['repeat_diagnostics'] = {}
        for version in ['old', 'new']:
            replay = load_rows(root/('replay-'+version)/'parity.jsonl')
            serial = {r['id']:r for r in replay if r['phase'] == 'serial'}
            assert set(serial) == set(replay_expected) and len(serial) == 66
            groups = defaultdict(list)
            for id, r in serial.items():
                if id.startswith('diagnostic-repeat/'):
                    original = id.split('/', 2)[2]
                    assert replay_expected[id]['tokens'] == expected[original]['tokens']
                    assert delta(r['probabilities'], outputs[version][original]['probabilities']) == 0
                    groups[original].append(r['probabilities'])
            assert len(groups) == 3 and all(len(v) == 10 for v in groups.values())
            report['repeat_diagnostics'][version] = {id:dict(repeats=len(values),
                max_probability_drift=max(delta(v, values[0]) for v in values),
                winner=expected[id]['options'][winner(values[0])][0]) for id, values in groups.items()}
    (root/'differences.jsonl').write_text(''.join(json.dumps(r,ensure_ascii=False)+'\n' for r in differences))
    (root/'audit.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({k:v for k,v in report.items() if k!='versions'},ensure_ascii=False,indent=2))
    # This is an audit of evidence, not a command that makes a failed migration gate disappear.
    # A recorded strict failure is a valid research result; callers must read strict_upgrade_pass.
    if args.require_pass and not report['strict_upgrade_pass']:
        raise SystemExit('Strict migration gate failed; see audit.json.')


if __name__ == '__main__':
    main()
