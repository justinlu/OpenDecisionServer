"""Recompute the saved d1 results from original responses and frozen labels."""
import argparse
from collections import defaultdict
import json
from pathlib import Path

from benchmark_latency import metrics, validate_response
from evaluate import grade, load_rows, summarize

ROOT = Path(__file__).resolve().parent


def read_rows(path):
    return [json.loads(line) for line in path.read_text().splitlines() if line.strip()]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--results', type=Path, default=ROOT/'results/2026-10-28-liquid-d1-gguf')
    args = parser.parse_args()
    frozen = load_rows([ROOT/'custom.jsonl', ROOT/'toksuite.jsonl'])
    voice = {row['id']: row for row in read_rows(ROOT/'voice-performance.jsonl')}
    report = {'models': {}, 'paired': {}, 'notes': [
        'Accuracy variants are correlated; paired counts are descriptive, not independent significance tests.',
        'Historical baseline speed is not a simultaneous controlled rerun.',
        'Media fixtures use synthetic speech; only unique cases count toward diagnostic accuracy.']}
    all_predictions = {}
    for prefix, model in [('omni', 'd1-omni-2b'), ('d1', 'd1-2b')]:
        records = read_rows(args.results/(prefix+'-accuracy')/'predictions.jsonl')
        assert len(records) == len(frozen) == 989
        for row, case in zip(records, frozen):
            assert all(row[key] == value for key, value in case.items()), row['id']
            assert row['http_status'] == 200, row['id']
            payload = json.loads(row['response_body'])
            assert payload['model'] == model and payload['usage']['output_tokens'] == 0
            verified = grade(case, payload, 'median')
            assert all(row[key] == value for key, value in verified.items()), row['id']
        summary = json.loads((args.results/(prefix+'-accuracy')/'summary.json').read_text())
        recomputed = summarize(records)
        assert all(summary[key] == value for key, value in recomputed.items())
        all_predictions[model] = {row['id']: row for row in records}
        samples = read_rows(args.results/(prefix+'-latency')/'samples.jsonl')
        latency = json.loads((args.results/(prefix+'-latency')/'summary.json').read_text())
        assert len(samples) == 1440 and len(latency) == 12
        groups = defaultdict(list)
        for sample in samples:
            case = voice[sample['case_id']]
            assert case['history_pairs'] == sample['history_pairs']
            assert len(case['request']['questions']) == sample['questions']
            assert sample['http_status'] == 200 and sample['error'] is None
            validate_response(case['request'], json.loads(sample['response_body']), model, 'median')
            groups[(sample['history_pairs'], sample['questions'], sample['clients'])].append(sample)
        for measured in latency:
            selected = groups[(measured['history_pairs'], measured['questions'], measured['clients'])]
            assert len(selected) == 120
            verified = metrics(selected, measured['elapsed_seconds'], measured['budget_ms'])
            assert all(measured[key] == value for key, value in verified.items())
        resources = read_rows(args.results/(prefix+'-resources.jsonl'))
        report['models'][model] = dict(accuracy=summary['breakdowns'], latency=latency,
                                      accuracy_errors=summary['overall']['errors'],
                                      peak_sampled_rss_mib=max(row['rss_kib'] for row in resources)/1024,
                                      accuracy_rows_verified=len(records), latency_rows_verified=len(samples))
    for model, path in [
        ('julia-1-historical', ROOT/'results/2026-10-27/predictions.jsonl'),
        ('chinese-decision-2b-historical', ROOT/'results/2026-10-27-ods-native/accuracy/predictions.jsonl')]:
        records = read_rows(path)
        assert {row['id'] for row in records} == {row['id'] for row in frozen}
        all_predictions[model] = {row['id']: row for row in records}
        report['models'][model] = dict(accuracy=summarize(records)['breakdowns'], historical=True,
                                      predictions_path=str(path.relative_to(ROOT)))
    for candidate in ['d1-omni-2b', 'd1-2b']:
        for baseline in ['julia-1-historical', 'chinese-decision-2b-historical']:
            pairs = {}
            for suite in ['custom', 'toksuite']:
                ids = [row['id'] for row in frozen if row['suite'] == suite]
                pairs[suite] = dict(corrected=sum(not all_predictions[baseline][id]['correct'] and all_predictions[candidate][id]['correct'] for id in ids),
                                    regressed=sum(all_predictions[baseline][id]['correct'] and not all_predictions[candidate][id]['correct'] for id in ids))
            report['paired'][candidate+' vs '+baseline] = pairs
    for prefix in ['omni', 'd1']:
        initial = args.results/(prefix+'-accuracy-initial')/'predictions.jsonl'
        if initial.exists():
            original = read_rows(initial)
            final = all_predictions['d1-omni-2b' if prefix == 'omni' else 'd1-2b']
            report[prefix+'_accuracy_repeat'] = dict(rows=len(original), changed_predictions=sum(row['predicted'] != final[row['id']]['predicted'] for row in original))
    (args.results/'comparison.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
    print('Verified 1,978 accuracy rows and 2,880 latency rows against raw responses and fixed fixtures.')


if __name__ == '__main__':
    main()
