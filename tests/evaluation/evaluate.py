"""Evaluate ODS decisions using fixed labels; Python 3.9+ standard library only."""
import argparse
from collections import Counter, defaultdict
from datetime import datetime, timezone
import hashlib
import http.client
import json
import math
from pathlib import Path
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent


def number(value):
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value):
        raise ValueError('Expected a finite numeric value')
    return float(value)


def grade(row, response):
    question = row['request']['questions']['decision']
    kind = question['type']
    if not isinstance(response, dict) or not isinstance(response.get('answers'), dict):
        raise ValueError('Response and answers must be objects')
    answer = response['answers'].get('decision')
    if not isinstance(answer, dict):
        raise ValueError('Decision answer must be an object')
    if answer.get('type') != kind or not isinstance(response.get('model'), str) or not response['model']:
        raise ValueError('Response type or model is missing/mismatched')
    if kind == 'noul':
        p = number(answer['noul'])
        if not 0 <= p <= 1:
            raise ValueError('Noul probability outside [0,1]')
        probs = {'false': 1-p, 'true': p}
        predicted = 'true' if p > .5 else 'false'
    else:
        keys = list(question['criteria']) if kind == 'choice' else [str(i) for i in range(len(question['criteria']))]
        raw = answer['probabilities']
        if not isinstance(raw, dict) or set(raw) != set(keys):
            raise ValueError('Response probability keys differ from request')
        probs = {k: number(raw[k]) for k in keys}
        if any(not 0 <= p <= 1 for p in probs.values()) or abs(sum(probs.values())-1) > 1e-4:
            raise ValueError('Invalid probability distribution')
        predicted = max(keys, key=probs.get)
        if kind == 'choice':
            selected = answer.get('choice')
            if selected not in probs or probs[selected] < max(probs.values())-1e-7:
                raise ValueError('Choice disagrees with probabilities')
            predicted = selected
        else:
            score = number(answer['score'])
            if abs(score-sum(i*probs[k] for i,k in enumerate(keys))) > 1e-4:
                raise ValueError('Score disagrees with expected rubric index')
    return dict(predicted=predicted, correct=predicted == row['expected'], probabilities=probs,
                returned_model=response['model'], error=None)


def stats(records):
    total = len(records)
    correct = sum(r['correct'] for r in records)
    matrix = defaultdict(Counter)
    for r in records:
        matrix[r['expected']][r['predicted'] if r['predicted'] is not None else '__error__'] += 1
    f1 = []
    labels = set()
    for r in records:
        criteria = r['request']['questions']['decision']['criteria']
        labels.update(criteria if isinstance(criteria, dict) else (str(i) for i in range(len(criteria))))
    for label in sorted(labels):
        tp = matrix[label][label]
        fp = sum(v[label] for k,v in matrix.items() if k != label)
        fn = sum(matrix[label].values())-tp
        f1.append(2*tp/(2*tp+fp+fn) if 2*tp+fp+fn else 0.)
    chance = sum(1/len(r['request']['questions']['decision']['criteria']) for r in records)/total if total else 0.
    return dict(total=total, correct=correct, errors=sum(r['error'] is not None for r in records),
                accuracy=correct/total if total else 0., uniform_random_baseline=chance,
                macro_f1=sum(f1)/len(f1) if f1 else 0., macro_f1_labels=sorted(labels), confusion_matrix={k:dict(v) for k,v in sorted(matrix.items())})


def summarize(records):
    report = {'overall': stats(records), 'breakdowns': {}}
    for fields in [('suite',), ('suite','form'), ('suite','prompt_language'), ('suite','type'),
                   ('suite','domain'), ('suite','form','prompt_language')]:
        groups = defaultdict(list)
        for r in records:
            groups['/'.join(str(r[f]) for f in fields)].append(r)
        report['breakdowns']['+'.join(fields)] = {k:stats(v) for k,v in sorted(groups.items())}
    # Custom families have exactly 18 or 6 rows; never count missing/error rows as complete.
    families = defaultdict(list)
    for r in records:
        if r['suite'] == 'custom':
            families[r['family']].append(r)
    complete = [rs for rs in families.values() if len(rs) == rs[0].get('family_size',18) and all(r['error'] is None for r in rs)]
    report['paired'] = dict(families_observed=len(families), families_complete=len(complete),
        families_all_correct=sum(all(r['correct'] for r in rs) for rs in complete),
        families_prediction_consistent=sum(len({r['predicted'] for r in rs}) == 1 for rs in complete))
    # Match each input variant to the Chinese state with the same prompt language.
    refs = {(r['family'],r['prompt_language']): r for r in records if r['suite']=='custom' and r['form']=='zh'}
    pairs = defaultdict(list)
    for r in records:
        ref = refs.get((r['family'],r['prompt_language'])) if r['suite']=='custom' else None
        if ref and r['form'] != 'zh':
            pairs[r['form']+'/'+r['prompt_language']].append((ref,r))
    report['paired_vs_chinese'] = {k:dict(total=len(ps), baseline_correct=sum(a['correct'] for a,b in ps),
        variant_correct=sum(b['correct'] for a,b in ps), regressions=sum(a['correct'] and not b['correct'] for a,b in ps),
        recoveries=sum(not a['correct'] and b['correct'] for a,b in ps),
        successful_pairs=sum(a['error'] is None and b['error'] is None for a,b in ps),
        same_prediction=sum(a['predicted'] == b['predicted'] for a,b in ps if a['error'] is None and b['error'] is None)) for k,ps in sorted(pairs.items())}
    # Majority is a dummy fixed-label reference within suite/type, not a trained classifier.
    report['majority_label_baselines'] = {}
    for key, values in report['breakdowns']['suite+type'].items():
        selected = [r for r in records if r['suite']+'/'+r['type'] == key]
        label, count = Counter(r['expected'] for r in selected).most_common(1)[0]
        report['majority_label_baselines'][key] = dict(label=label, accuracy=count/len(selected))
    return report


def load_rows(paths):
    rows = []
    for path in paths:
        rows.extend(json.loads(line) for line in path.read_text().splitlines() if line.strip())
    if not rows or len({r['id'] for r in rows}) != len(rows):
        raise ValueError('Dataset is empty or contains duplicate IDs')
    for r in rows:
        question = r['request']['questions']['decision']
        criteria = question['criteria']
        labels = list(criteria) if isinstance(criteria,dict) else [str(i) for i in range(len(criteria))]
        if r['expected'] not in labels or not isinstance(r['request']['state'], str):
            raise ValueError('Invalid expected label or state in '+r['id'])
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dataset', nargs='+', type=Path, default=[ROOT/'custom.jsonl', ROOT/'toksuite.jsonl'])
    parser.add_argument('--url', default='http://127.0.0.1:8080/v1/systemone')
    parser.add_argument('--output', type=Path, required=True, help='New directory; existing output is never overwritten')
    parser.add_argument('--timeout', type=float, default=30.)
    parser.add_argument('--model', default='julia-1')
    args = parser.parse_args()
    if args.timeout <= 0:
        parser.error('--timeout must be positive')
    rows = load_rows(args.dataset)
    args.output.mkdir(parents=True, exist_ok=False)
    metadata = dict(started_at=datetime.now(timezone.utc).isoformat(), url=args.url, requested_model=args.model,
                    datasets={str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in args.dataset},
                    scoring='argmax probabilities; noul >0.5; errors count as incorrect',
                    warning='Variants are correlated; overall pooling is descriptive only. Interpret source-specific results.')
    (args.output/'metadata.json').write_text(json.dumps(metadata,ensure_ascii=False,indent=2)+'\n')
    records = []
    with (args.output/'predictions.jsonl').open('w') as stream:
        for index, row in enumerate(rows):
            request = dict(row['request'], model=args.model)
            req = urllib.request.Request(args.url, data=json.dumps(request,ensure_ascii=False).encode(), headers={'Content-Type':'application/json'})
            started = time.monotonic()
            try:
                with urllib.request.urlopen(req, timeout=args.timeout) as response:
                    payload = json.load(response)
                result = grade(row, payload)
            except (urllib.error.URLError, http.client.HTTPException, OSError, ValueError, KeyError, TypeError, IndexError) as error:
                result = dict(predicted=None, correct=False, error=type(error).__name__+': '+str(error))
            record = dict(row, **result, latency_ms=(time.monotonic()-started)*1000)
            records.append(record)
            stream.write(json.dumps(record,ensure_ascii=False,allow_nan=False)+'\n')
            stream.flush()
            if (index+1) % 50 == 0:
                print(f'{index+1}/{len(rows)} evaluated', flush=True)
    report = summarize(records)
    report['metadata'] = metadata
    report['returned_models'] = dict(Counter(r.get('returned_model','__error__') for r in records))
    (args.output/'summary.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    for source, values in report['breakdowns']['suite'].items():
        print(f"{source}: {values['correct']}/{values['total']} ({values['accuracy']:.2%}); errors={values['errors']}")
    if report['overall']['errors']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
