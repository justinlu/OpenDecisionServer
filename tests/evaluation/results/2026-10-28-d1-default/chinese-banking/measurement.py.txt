"""Direct authored text evaluation of d1; no ASR or audio input."""
import argparse
import copy
import hashlib
import json
import math
from pathlib import Path
import random
import time

from banking_audio import ROOT,build_request,classification,sha,validate_dataset,write_json
from banking_audio_cascade import needs_business,route_success
from banking_audio_diagnostics import request_one
from banking_audio_hierarchy import CREDIT,specification,domain_for,menu,business_correct
from evaluate import grade

DATASET=ROOT.parent/'banking-text-v1'

def dataset():
    rows=[]
    for origin,root in [('legacy',ROOT),('credit',CREDIT)]:
        cases,task=validate_dataset(root)
        for case in cases:
            rows.append(dict(id=origin+'/'+case['id'],origin=origin,case=case,
                             full_questions=task['questions']))
    return rows


def prepare():
    DATASET.mkdir(exist_ok=True)
    path=DATASET/'cases.jsonl'
    content=''.join(json.dumps(r,ensure_ascii=False)+'\n' for r in dataset())
    if path.exists() and path.read_text()!=content:raise ValueError('Refusing to replace frozen text corpus')
    path.write_text(content)
    write_json(DATASET/'manifest.json',dict(cases_sha256=sha(path),semantic_cases=len(dataset()),
        sources={origin:{f:sha(root/f) for f in ['cases.jsonl','task.json']} for origin,root in [('legacy',ROOT),('credit',CREDIT)]},
        method='Exact authored text and history copied from frozen audio semantic cases; no ASR, no rewriting, no voice duplication.'))


def load():
    rows=list(map(json.loads,(DATASET/'cases.jsonl').read_text().splitlines()))
    assert rows==dataset()
    manifest=json.loads((DATASET/'manifest.json').read_text())
    assert sha(DATASET/'cases.jsonl')==manifest['cases_sha256']
    for origin,root in [('legacy',ROOT),('credit',CREDIT)]:
        assert manifest['sources'][origin]=={f:sha(root/f) for f in ['cases.jsonl','task.json']}
    return rows


def body(row,question,model):
    return build_request(row['case'],dict(model=model,questions={'decision':question}),'text')


def quantiles(values):
    values=sorted(values)
    return {p:values[math.ceil(len(values)*q)-1] for p,q in [('p50',.5),('p95',.95),('p99',.99)]}


def summary(rows,events,routes,spec):
    result={}
    lookup={r['id']:r for r in rows}
    def event(id):return events[id] if id else None
    for origin in ['legacy','credit','all']:
        selected=[r for r in rows if origin=='all' or r['origin']==origin]
        report={}
        for node,key in [('full-mode','mode'),('full-function','function')]:
            chosen=[events[r['id']+'::'+node] for r in selected]
            records=[dict(expected={key:e['expected']},predicted={key:e['predicted']}) for e in chosen]
            report[node]=dict(cases=len(chosen),errors=sum(e['error'] is not None for e in chosen),
                **classification(records,key),latency_ms=quantiles([e['latency_ms'] for e in chosen]))
        report['full-joint']=dict(cases=len(selected),correct=sum(all(events[r['id']+'::'+n]['correct']
            for n in ['full-mode','full-function']) for r in selected))
        core=[r for r in routes if origin=='all' or lookup[r['id']]['origin']==origin]
        nav=[r for r in core if lookup[r['id']]['case']['expected']['mode']=='navigate']
        consult=[r for r in core if lookup[r['id']]['case']['expected']['mode']=='inquire']
        report['two-stage']=dict(cases=len(core),gate_correct=sum(event(r['gate'])['correct'] for r in core),
            route_correct=sum(r['correct'] for r in core),navigation_cases=len(nav),navigation_correct=sum(r['correct'] for r in nav),
            consultation_cases=len(consult),consultation_misgated=sum(needs_business(event(r['gate'])) for r in consult),
            missed_navigation=sum(not needs_business(event(r['gate'])) for r in nav),
            latency_ms=quantiles([r['elapsed_ms'] for r in core]))
        for node,key in [('flat','function'),('domain','domain')]:
            chosen=[events[r['id']+'::'+node] for r in nav]
            report[node]=dict(cases=len(chosen),**classification([dict(expected={key:e['expected']},predicted={key:e['predicted']}) for e in chosen],key))
        report['domain-then-leaf']=dict(cases=len(nav),correct=sum(business_correct(spec,lookup[r['id']]['case']['expected']['function'],
            event(r['id']+'::domain'),event(r['domain_leaf'])) for r in nav))
        result[origin]=report
    result['requests']=len(events);result['errors']=sum(e['error'] is not None for e in events.values())
    return result


def run(args):
    rows=load();spec=specification();args.output.mkdir(parents=True,exist_ok=False)
    write_json(args.output/'specification.json',spec)
    write_json(args.output/'metadata.json',dict(model=args.model,url=args.url,runner_sha256=sha(Path(__file__)),
        dataset_sha256=sha(DATASET/'cases.jsonl'),purpose='Post-hoc direct authored text, no ASR; previously seen semantic cases.',
        timing='One sequential warmup excluded. Per-request HTTP latency; actual two-stage wall time includes body preparation. Domain diagnostics excluded.'))
    warmup=request_one(args.url,dict(model=args.model,state='我要取款',questions={'decision':spec['gate']}),'navigate')
    write_json(args.output/'warmup.json',warmup)
    if warmup['error']:raise RuntimeError(warmup['error'])
    events={};routes=[];jobs=copy.deepcopy(rows);random.Random(20261028).shuffle(jobs)
    with (args.output/'events.jsonl').open('w') as stream:
        def call(row,node,question,expected):
            id=row['id']+'::'+node
            assert id not in events
            payload=body(row,question,args.model)
            event=dict(id=id,row_id=row['id'],node=node,question=question,**request_one(args.url,payload,expected))
            events[id]=event;stream.write(json.dumps(event,ensure_ascii=False)+'\n');stream.flush()
            return id
        for i,row in enumerate(jobs):
            expected=row['case']['expected']
            for key in ['mode','function']:call(row,'full-'+key,row['full_questions'][key],expected[key])
            if 'core' not in row['case']['tags']:continue
            start=time.perf_counter();gate=call(row,'gate',spec['gate'],expected['mode'])
            question=spec['legacy_business'] if row['origin']=='legacy' else spec['business']
            leaf=call(row,'actual-leaf',question,expected['function']) if needs_business(events[gate]) else None
            route=dict(id=row['id'],gate=gate,leaf=leaf,correct=route_success(expected,events[gate],events[leaf] if leaf else None),elapsed_ms=(time.perf_counter()-start)*1000,domain_leaf=None)
            routes.append(route)
            # Business-only studies use true navigation membership; never repair actual route scores.
            if expected['mode']=='navigate':
                call(row,'flat',question,expected['function'])
                domain=call(row,'domain',spec['domain'],domain_for(spec,expected['function']))
                child=menu(spec,events[domain]['predicted']) if not events[domain]['error'] else None
                route['domain_leaf']=call(row,'domain-leaf',child,expected['function']) if child else None
            if i%20==0:print('semantic cases',i+1,'requests',len(events),flush=True)
    (args.output/'routes.jsonl').write_text(''.join(json.dumps(r,ensure_ascii=False)+'\n' for r in routes))
    write_json(args.output/'summary.json',summary(rows,events,routes,spec))
    print(json.dumps(json.loads((args.output/'summary.json').read_text()),ensure_ascii=False,indent=2),flush=True)
    if any(e['error'] for e in events.values()):raise SystemExit('Recorded inference errors')


def audit(output):
    rows=load();lookup={r['id']:r for r in rows};spec=specification()
    metadata=json.loads((output/'metadata.json').read_text())
    assert metadata['dataset_sha256']==sha(DATASET/'cases.jsonl')
    assert metadata['runner_sha256']==sha(output/'measurement.py.txt')
    assert spec==json.loads((output/'specification.json').read_text())
    records=list(map(json.loads,(output/'events.jsonl').read_text().splitlines()));events={e['id']:e for e in records}
    assert len(records)==len(events)
    for e in records:
        row=lookup[e['row_id']];expected=row['case']['expected'];node=e['node']
        assert e['id']==row['id']+'::'+node
        if node.startswith('full-'):
            key=node.removeprefix('full-')
            question=row['full_questions'][key];target=expected[key]
        else:
            assert 'core' in row['case']['tags']
            question=spec['legacy_business'] if row['origin']=='legacy' else spec['business']
            target=expected['function']
            if node=='gate':question=spec['gate'];target=expected['mode']
            elif node=='actual-leaf':assert needs_business(events[row['id']+'::gate'])
            elif node in ['flat','domain','domain-leaf']:
                assert expected['mode']=='navigate'
                if node=='domain':question=spec['domain'];target=domain_for(spec,expected['function'])
                if node=='domain-leaf':question=menu(spec,events[row['id']+'::domain']['predicted'])
            else:raise AssertionError('Unexpected inference node')
        assert e['question']==question and e['expected']==target
        payload=body(row,question,metadata['model'])
        assert 'files' not in payload and payload['state'].endswith(row['case']['transcript'])
        assert e['payload_sha256']==hashlib.sha256(json.dumps(payload,ensure_ascii=False).encode()).hexdigest()
        if e['error']:continue
        raw=json.loads(e['response_body']);assert raw['model']==metadata['model']
        graded=grade(dict(expected=e['expected'],request={'questions':payload['questions']}),raw)
        assert all(e[k]==graded[k] for k in ['predicted','probabilities','correct'])
    routes=list(map(json.loads,(output/'routes.jsonl').read_text().splitlines()))
    assert {r['id'] for r in routes}=={r['id'] for r in rows if 'core' in r['case']['tags']}
    assert len(routes)==130
    required={r['id']+'::'+node for r in rows for node in ['full-mode','full-function']}
    for r in rows:
        for node in ['full-mode','full-function']:assert r['id']+'::'+node in events
    for r in routes:
        required.add(r['id']+'::gate')
        assert r['gate']==r['id']+'::gate'
        expected=lookup[r['id']]['case']['expected'];gate=events[r['gate']];leaf=events[r['leaf']] if r['leaf'] else None
        assert bool(leaf)==needs_business(gate)
        if leaf:
            assert r['leaf']==r['id']+'::actual-leaf';required.add(r['leaf'])
        if expected['mode']=='navigate':
            required.update(r['id']+'::'+node for node in ['flat','domain'])
            d=events[r['id']+'::domain']
            has_child=not d['error'] and menu(spec,d['predicted']) is not None
            assert bool(r['domain_leaf'])==has_child
            if has_child:
                assert r['domain_leaf']==r['id']+'::domain-leaf';required.add(r['domain_leaf'])
        else:assert r['domain_leaf'] is None
        assert r['correct']==route_success(expected,gate,leaf)
    assert set(events)==required
    assert summary(rows,events,routes,spec)==json.loads((output/'summary.json').read_text())
    print('Audited',len(rows),'text cases and',len(events),'requests')


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('command',choices=['prepare','run','audit'])
    p.add_argument('--model',default='d1-2b');p.add_argument('--url',default='http://127.0.0.1:18085/v1/systemone');p.add_argument('--output',type=Path)
    args=p.parse_args()
    if args.command=='prepare':prepare()
    elif args.command=='run':run(args)
    else:audit(args.output)

if __name__=='__main__':main()
