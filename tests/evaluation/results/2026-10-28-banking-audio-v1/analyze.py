"""Recompute supplemental readouts and audit every post-hoc response without inference."""
from collections import defaultdict
import hashlib
import json
from pathlib import Path
import sys

RESULTS = Path(__file__).resolve().parent
EVALUATION = RESULTS.parents[1]
sys.path.insert(0, str(EVALUATION))
from banking_audio import ROOT, build_request, summarize, validate_dataset, verify_media, write_json
from evaluate import grade


def main():
    cases, task = validate_dataset(ROOT)
    manifest = verify_media(ROOT)
    lookup = {c['id']:c for c in cases}
    rows = [json.loads(line) for line in (RESULTS/'predictions.jsonl').read_text().splitlines()]
    diagnostics = [json.loads(line) for line in (RESULTS/'diagnostics/predictions.jsonl').read_text().splitlines()]
    core = {c['id'] for c in cases if 'core' in c['tags']}
    voices = {v['id'] for v in task['voices']}
    sentence_cases = {f'{topic}-02-{mode}' for topic in ['withdraw','deposit','transfer','balance','statement','pin']
                      for mode in ['navigate','inquire']}
    expected_jobs = {('binary_text',c,'none') for c in core}
    expected_jobs.update(('binary_audio',c,v) for c in core for v in voices)
    expected_jobs.update(('sentence_audio',c,v) for c in sentence_cases for v in voices)
    actual = [(r['group'],r['case_id'],r['voice']) for r in diagnostics]
    assert len(actual) == len(expected_jobs) and set(actual) == expected_jobs
    media = {e['file']:e for e in manifest['audio']}
    for row in diagnostics:
        case = lookup[row['case_id']]
        if row['group'].startswith('binary'):
            assert set(row['question']['criteria']) == {'navigate','inquire'}
            expected = case['expected']['mode']
        else:
            criteria = row['question']['criteria']
            assert len(criteria) == 4 and len(set(criteria.values())) == 4
            matches = [k for k,v in criteria.items() if v == case['transcript']]
            assert len(matches) == 1
            expected = matches[0]
        assert row['expected'] == expected
        if row['file']:
            entry = media[row['file']]
            assert entry['case_id'] == case['id'] and entry['voice'] == row['voice'] and entry['condition'] == 'clean'
        variant = dict(task, questions={'decision':row['question']})
        body = build_request(case,variant,'audio' if row['file'] else 'text',ROOT/row['file'] if row['file'] else None)
        assert row['payload_sha256'] == hashlib.sha256(json.dumps(body,ensure_ascii=False).encode()).hexdigest()
        if row['error']:
            assert not row['correct']
            continue
        assert row['http_status'] == 200
        payload = json.loads(row['response_body'])
        assert payload['model'] == task['model']
        result = grade(dict(expected=expected,request={'questions':body['questions']}),payload)
        assert all(row[k] == result[k] for k in ['predicted','correct','probabilities'])
    extra = dict(post_hoc_responses_audited=len(diagnostics), post_hoc_request_errors=sum(r['error'] is not None for r in diagnostics))
    for group in ['text','clean']:
        selected = [r for r in rows if r['group']==group and 'core' in r['tags']]
        extra[group+'_core_original_six_labels'] = summarize(selected)
    navigation = {}
    for group in ['text','clean','noise15']:
        selected = [r for r in rows if r['group']==group and r['expected']['mode']=='navigate']
        chosen = [r for r in selected if r['predicted']['mode']=='navigate']
        navigation[group] = dict(expected_navigation=len(selected),
            correct_mode_and_destination=sum(r['predicted']==r['expected'] for r in selected),
            missed_mode=len(selected)-len(chosen),
            selected_mode_but_unknown_destination=sum(r['predicted']['function']=='unknown' for r in chosen),
            selected_mode_but_wrong_known_destination=sum(r['predicted']['function'] not in ['unknown',r['expected']['function']] for r in chosen))
    extra['navigation_readout'] = navigation
    tags = defaultdict(list)
    for row in rows:
        if row['group']=='clean':
            for tag in row['tags']:
                tags[tag].append(row)
    extra['clean_by_tag'] = {tag:summarize(selected) for tag,selected in tags.items()}
    write_json(RESULTS/'additional-analysis.json',extra)
    print('Audited',len(diagnostics),'post-hoc responses; supplemental readouts recomputed.')


if __name__ == '__main__':
    main()
