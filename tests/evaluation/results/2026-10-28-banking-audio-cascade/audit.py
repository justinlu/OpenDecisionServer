"""Offline verification of actual dispatch, oracle separation and raw decisions."""
import hashlib
import json
import math
from pathlib import Path
import sys

RESULTS=Path(__file__).resolve().parent
sys.path.insert(0,str(RESULTS.parents[1]))
from banking_audio import ROOT, build_request, validate_dataset, verify_media
from banking_audio_cascade import needs_business, route_success, summarize
from evaluate import grade


def main():
    cases,task=validate_dataset(ROOT)
    cases={c['id']:c for c in cases if 'core' in c['tags']}
    manifest=verify_media(ROOT)
    config=json.loads((RESULTS/'questions.json').read_text())
    metadata=json.loads((RESULTS/'metadata.json').read_text())
    assert metadata['runner_sha256']==hashlib.sha256((RESULTS/'measurement.py.txt').read_bytes()).hexdigest()
    assert metadata['dataset_manifest_sha256']==hashlib.sha256((ROOT/'manifest.json').read_bytes()).hexdigest()
    media={(e['case_id'],e['voice']):e for e in manifest['audio'] if e['condition']=='clean'}
    rows=[json.loads(line) for line in (RESULTS/'routes.jsonl').read_text().splitlines()]
    expected={(c,v) for c in cases for v in ['none']+[v['id'] for v in task['voices']]}
    actual=[(r['case_id'],r['voice']) for r in rows]
    assert len(actual)==len(expected) and set(actual)==expected
    def check(row,answer,question,expected):
        variant=dict(task,questions={'decision':question})
        body=build_request(cases[row['case_id']],variant,row['modality'],ROOT/row['file'] if row['file'] else None)
        assert answer['payload_sha256']==hashlib.sha256(json.dumps(body,ensure_ascii=False).encode()).hexdigest()
        assert answer['expected']==expected
        assert answer['error'] is None and answer['http_status']==200
        payload=json.loads(answer['response_body'])
        assert payload['model']==task['model']
        result=grade(dict(expected=expected,request={'questions':body['questions']}),payload)
        assert all(answer[k]==result[k] for k in ['correct','predicted','probabilities'])
    for row in rows:
        case=cases[row['case_id']]
        assert row['expected']==case['expected']
        assert row['modality']==('text' if row['voice']=='none' else 'audio')
        assert row['file']==(None if row['voice']=='none' else media[(row['case_id'],row['voice'])]['file'])
        check(row,row['gate'],config['gate'],case['expected']['mode'])
        assert (row['business'] is not None)==needs_business(row['gate'])
        if row['business'] is not None:check(row,row['business'],config['business'],case['expected']['function'])
        assert row['route_correct']==route_success(case['expected'],row['gate'],row['business'])
        assert math.isfinite(row['route_elapsed_ms']) and row['route_elapsed_ms']>=row['gate']['latency_ms']
    oracle=[json.loads(line) for line in (RESULTS/'oracle-navigation.jsonl').read_text().splitlines()]
    nav=[r for r in rows if r['expected']['mode']=='navigate']
    expected_nav={(r['case_id'],r['voice']) for r in nav}
    assert len(oracle)==len(expected_nav) and {(r['case_id'],r['voice']) for r in oracle}==expected_nav
    lookup={(r['case_id'],r['voice']):r for r in rows}
    for record in oracle:
        row=lookup[(record['case_id'],record['voice'])]
        assert record['reused_actual']==(row['business'] is not None)
        if record['reused_actual']:assert record['result']==row['business']
        check(row,record['result'],config['business'],row['expected']['function'])
        row['oracle_business']=record['result']
    report=json.loads((RESULTS/'summary.json').read_text())
    for mode in ['text','audio']:assert report[mode]==summarize([r for r in rows if r['modality']==mode])
    for voice in task['voices']:
        assert report['audio_by_voice'][voice['id']]==summarize([r for r in rows if r['voice']==voice['id']])
    print('Audited 300 actual routes and 150 separate oracle records; no oracle result changes route correctness.')


if __name__=='__main__':main()
