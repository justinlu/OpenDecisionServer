"""Banking domain hierarchy and nested candidate-menu diagnostics on frozen audio."""
import argparse
import copy
import hashlib
import json
import math
from pathlib import Path
import random
import time

from banking_audio import ROOT, build_request, sha, validate_dataset, verify_media, write_json
from banking_audio_cascade import needs_business
from banking_audio_diagnostics import request_one

CREDIT = ROOT.parent/'banking-credit-audio-v1'
PREVIOUS = Path(__file__).resolve().parent/'results/2026-10-28-banking-audio-cascade/questions.json'


def specification():
    previous=json.loads(PREVIOUS.read_text())
    business=copy.deepcopy(previous['business'])
    criteria={k:v for k,v in business['criteria'].items() if k!='unknown'}
    criteria.update(credit_repay='信用卡还款',credit_bill='信用卡账单查询',credit_limit='信用卡可用额度查询',
                    credit_installment='信用卡账单分期申请',credit_apply='申请信用卡')
    criteria['unknown']=business['criteria']['unknown']
    business['criteria']=criteria
    domains=dict(cash=['withdraw','deposit'],account=['transfer','balance','statement'],
                 card=['pin','loss','replace'],digital=['phone','mobile'],
                 credit=['credit_repay','credit_bill','credit_limit','credit_installment','credit_apply'])
    domain=dict(type='choice',
        instructions='只识别当前希望办理的业务域，按主要功能归类，忽略被否定的目标。通用现金操作或卡片管理不因提及信用卡而改变业务域。不确定唯一业务域则选unknown。',
        criteria=dict(cash='现金存款或取款，不包括取款密码修改',
            account='转账汇款、银行卡余额查询、交易明细查询或流水打印；不包括信用卡专属账单和额度查询',
            card='银行卡取款密码修改或重置、挂失、补卡或换卡，包括这类通用卡片管理',
            digital='银行预留手机号修改，手机银行开通或签约',
            credit='信用卡专属的还款、账单查询、可用额度查询、账单分期申请或信用卡申请',
            unknown='无法确定唯一业务域，或不属于以上业务域'))
    return dict(gate=previous['gate'],business=business,legacy_business=previous['business'],domain=domain,domains=domains)


def domain_for(spec, leaf):
    matches=[domain for domain,leaves in spec['domains'].items() if leaf in leaves]
    if len(matches)!=1:raise ValueError('Leaf must have exactly one parent')
    return matches[0]


def menu(spec, domain):
    if domain not in spec['domains']:return None
    question=copy.deepcopy(spec['business'])
    allowed=set(spec['domains'][domain])|{'unknown'}
    question['criteria']={k:v for k,v in question['criteria'].items() if k in allowed}
    return question


def oracle_menu(spec, leaf, size, seed):
    distractors=[k for k in spec['business']['criteria'] if k not in [leaf,'unknown']]
    random.Random(hashlib.sha256(seed.encode()).hexdigest()).shuffle(distractors)
    allowed=set(distractors[:size-2])|{leaf,'unknown'}
    question=copy.deepcopy(spec['business'])
    question['criteria']={k:v for k,v in question['criteria'].items() if k in allowed}
    return question


def business_correct(spec, leaf, domain, answer):
    return bool(domain and not domain['error'] and domain['predicted']==domain_for(spec,leaf)
                and answer and not answer['error'] and answer['predicted']==leaf)


def route_correct(spec, expected, gate, domain, answer):
    if gate['error'] or gate['predicted']!=expected['mode']:return False
    if expected['mode']=='inquire':return True
    return business_correct(spec,expected['function'],domain,answer)


def samples():
    all_samples=[];manifests={}
    for origin,root in [('legacy',ROOT),('credit',CREDIT)]:
        cases,task=validate_dataset(root);manifest=verify_media(root)
        manifests[origin]=sha(root/'manifest.json')
        media={(e['case_id'],e['voice']):e for e in manifest['audio'] if e['condition']=='clean'}
        for case in cases:
            if 'core' not in case['tags']:continue
            for voice in ['none']+[v['id'] for v in task['voices']]:
                all_samples.append(dict(key=origin+'/'+case['id']+'/'+voice,origin=origin,case_id=case['id'],
                    voice=voice,modality='text' if voice=='none' else 'audio',
                    file=None if voice=='none' else media[(case['id'],voice)]['file'],case=case))
    return all_samples,manifests


def summary(spec, samples_by_key, events, routes, studies):
    def answer(id):return events[id] if id else None
    report={}
    for origin in ['legacy','credit','all']:
        for modality in ['text','audio']:
            selected=[r for r in routes if (origin=='all' or samples_by_key[r['key']]['origin']==origin)
                      and samples_by_key[r['key']]['modality']==modality]
            study=[r for r in studies if (origin=='all' or samples_by_key[r['key']]['origin']==origin)
                   and samples_by_key[r['key']]['modality']==modality]
            nav=[r for r in selected if samples_by_key[r['key']]['case']['expected']['mode']=='navigate']
            consult=[r for r in selected if r not in nav]
            timings=sorted(r['elapsed_ms'] for r in selected)
            out=dict(route_cases=len(selected),route_correct=0,navigation_cases=len(nav),navigation_correct=0,
                consultation_cases=len(consult),consultation_misgated=0,consultation_known_leaf=0,
                gate_correct=0,business_study_cases=len(study),diagnostics={})
            for row in selected:
                expected=samples_by_key[row['key']]['case']['expected']
                gate,domain,leaf=map(answer,[row['gate'],row['domain'],row['leaf']])
                correct=route_correct(spec,expected,gate,domain,leaf)
                out['gate_correct']+=gate['correct'];out['route_correct']+=correct
                if expected['mode']=='navigate':out['navigation_correct']+=correct
                else:
                    out['consultation_misgated']+=needs_business(gate)
                    out['consultation_known_leaf']+=bool(leaf and not leaf['error'] and leaf['predicted']!='unknown')
            out['route_accuracy']=out['route_correct']/len(selected)
            out['route_latency_ms']=dict(p50=timings[math.ceil(len(timings)*.5)-1],p95=timings[math.ceil(len(timings)*.95)-1])
            for condition in ['flat11','flat16','domain','tree','oracle_domain_leaf','oracle3','oracle5']:
                eligible=[r for r in study if condition!='flat11' or r.get('flat11')]
                correct=0
                for row in eligible:
                    expected=samples_by_key[row['key']]['case']['expected']['function']
                    if condition=='tree':correct+=business_correct(spec,expected,answer(row['domain']),answer(row['leaf']))
                    else:correct+=answer(row[condition])['correct']
                out['diagnostics'][condition]=dict(cases=len(eligible),correct=correct,
                    accuracy=correct/len(eligible) if eligible else None)
            report[origin+'/'+modality]=out
    report['inference_requests']=len(events)
    report['errors']=sum(e['error'] is not None for e in events.values())
    return report


def run(output,url):
    spec=specification();all_samples,manifests=samples()
    _,task=validate_dataset(ROOT)
    output.mkdir(parents=True,exist_ok=False)
    write_json(output/'specification.json',spec)
    write_json(output/'metadata.json',dict(post_hoc=True,manifests=manifests,runner_sha256=sha(Path(__file__)),url=url,
        oracle_warning='Oracle menus use gold to restrict candidates. Domain-oracle and mode-oracle never repair actual routes.',
        leaf_control='All leaf conditions use identical instructions, descriptions and relative candidate order. Only membership changes.'))
    lookup={s['key']:s for s in all_samples};events={};routes=[];studies=[]
    log=(output/'classifications.jsonl').open('w')
    def call(sample,node,question,expected):
        id=sample['key']+'::'+node
        if id in events:raise ValueError('Duplicate inference event')
        root=ROOT if sample['origin']=='legacy' else CREDIT
        body=build_request(sample['case'],dict(task,questions={'decision':question}),sample['modality'],
                           root/sample['file'] if sample['file'] else None)
        event=dict(id=id,key=sample['key'],node=node,question=question,**request_one(url,body,expected))
        events[id]=event;log.write(json.dumps(event,ensure_ascii=False)+'\n');log.flush()
        if len(events)%64==0:print('inference',len(events),node,flush=True)
        return id
    def domain_and_leaf(sample,phase):
        target=sample['case']['expected']['function']
        domain=call(sample,'domain-'+phase,spec['domain'],domain_for(spec,target))
        question=menu(spec,events[domain]['predicted']) if not events[domain]['error'] else None
        leaf=call(sample,'leaf-'+phase,question,target) if question else None
        return domain,leaf
    jobs=list(all_samples);random.Random(20261028).shuffle(jobs)
    with (output/'routes.jsonl').open('w') as stream:
        for sample in jobs:
            start=time.perf_counter();expected=sample['case']['expected']
            gate=call(sample,'gate',spec['gate'],expected['mode'])
            domain,leaf=domain_and_leaf(sample,'actual') if needs_business(events[gate]) else (None,None)
            row=dict(key=sample['key'],gate=gate,domain=domain,leaf=leaf,elapsed_ms=(time.perf_counter()-start)*1000)
            routes.append(row);stream.write(json.dumps(row)+'\n');stream.flush()
    actual={r['key']:r for r in routes}
    # Actual routes are already on disk. Every subsequent call is a diagnostic.
    with (output/'business-study.jsonl').open('w') as stream:
        for sample in jobs:
            expected=sample['case']['expected']
            if expected['mode']!='navigate':continue
            target=expected['function'];parent=domain_for(spec,target)
            route=actual[sample['key']]
            if route['domain']:domain,leaf=route['domain'],route['leaf']
            else:domain,leaf=domain_and_leaf(sample,'mode-oracle')
            oracle_leaf=leaf if events[domain]['predicted']==parent and leaf else call(sample,'leaf-domain-oracle',menu(spec,parent),target)
            row=dict(key=sample['key'],domain=domain,leaf=leaf,oracle_domain_leaf=oracle_leaf,
                flat16=call(sample,'flat16',spec['business'],target),
                flat11=call(sample,'flat11',spec['legacy_business'],target) if sample['origin']=='legacy' else None,
                oracle3=call(sample,'oracle3',oracle_menu(spec,target,3,sample['origin']+'/'+sample['case_id']),target),
                oracle5=call(sample,'oracle5',oracle_menu(spec,target,5,sample['origin']+'/'+sample['case_id']),target))
            studies.append(row);stream.write(json.dumps(row)+'\n');stream.flush()
    log.close()
    report=summary(spec,lookup,events,routes,studies);write_json(output/'summary.json',report)
    print(json.dumps(report,indent=2),flush=True)
    if report['errors']:raise SystemExit('Finished with recorded inference errors')


def audit(output):
    spec=json.loads((output/'specification.json').read_text());all_samples,manifests=samples()
    metadata=json.loads((output/'metadata.json').read_text())
    assert metadata['manifests']==manifests
    assert metadata['runner_sha256']==sha(output/'measurement.py.txt')
    assert spec==specification()
    lookup={s['key']:s for s in all_samples};_,task=validate_dataset(ROOT)
    rows=[json.loads(line) for line in (output/'classifications.jsonl').read_text().splitlines()]
    events={e['id']:e for e in rows};assert len(events)==len(rows)
    from evaluate import grade
    for event in rows:
        sample=lookup[event['key']];root=ROOT if sample['origin']=='legacy' else CREDIT
        body=build_request(sample['case'],dict(task,questions={'decision':event['question']}),sample['modality'],
                           root/sample['file'] if sample['file'] else None)
        assert hashlib.sha256(json.dumps(body,ensure_ascii=False).encode()).hexdigest()==event['payload_sha256']
        assert event['error'] is None and event['http_status']==200
        payload=json.loads(event['response_body']);assert payload['model']==task['model']
        result=grade(dict(expected=event['expected'],request={'questions':body['questions']}),payload)
        assert all(result[k]==event[k] for k in ['predicted','correct','probabilities'])
    routes=[json.loads(line) for line in (output/'routes.jsonl').read_text().splitlines()]
    studies=[json.loads(line) for line in (output/'business-study.jsonl').read_text().splitlines()]
    assert len(routes)==len(lookup) and {r['key'] for r in routes}==set(lookup)
    by_route={r['key']:r for r in routes}
    expected_studies={s['key'] for s in all_samples if s['case']['expected']['mode']=='navigate'}
    assert len(studies)==len(expected_studies) and {r['key'] for r in studies}==expected_studies
    used=set()
    def check(id,key,question,expected):
        event=events[id];used.add(id)
        assert event['key']==key and event['question']==question and event['expected']==expected
        return event
    for row in routes:
        sample=lookup[row['key']];expected=sample['case']['expected']
        gate=check(row['gate'],row['key'],spec['gate'],expected['mode'])
        assert bool(row['domain'])==needs_business(gate)
        if row['domain']:
            domain=check(row['domain'],row['key'],spec['domain'],domain_for(spec,expected['function']))
            question=menu(spec,domain['predicted'])
            assert bool(row['leaf'])==bool(question)
            if question:check(row['leaf'],row['key'],question,expected['function'])
        else:assert row['leaf'] is None
        assert math.isfinite(row['elapsed_ms']) and row['elapsed_ms']>=0
    for row in studies:
        sample=lookup[row['key']];target=sample['case']['expected']['function'];parent=domain_for(spec,target)
        actual=by_route[row['key']]
        if actual['domain']:assert (row['domain'],row['leaf'])==(actual['domain'],actual['leaf'])
        domain=check(row['domain'],row['key'],spec['domain'],parent);question=menu(spec,domain['predicted'])
        assert bool(row['leaf'])==bool(question)
        if question:check(row['leaf'],row['key'],question,target)
        if domain['predicted']==parent:assert row['oracle_domain_leaf']==row['leaf']
        check(row['oracle_domain_leaf'],row['key'],menu(spec,parent),target)
        check(row['flat16'],row['key'],spec['business'],target)
        assert bool(row['flat11'])==(sample['origin']=='legacy')
        if row['flat11']:check(row['flat11'],row['key'],spec['legacy_business'],target)
        for count in [3,5]:check(row['oracle'+str(count)],row['key'],oracle_menu(spec,target,count,sample['origin']+'/'+sample['case_id']),target)
    assert used==set(events)
    assert summary(spec,lookup,events,routes,studies)==json.loads((output/'summary.json').read_text())
    print('Audited',len(routes),'actual routes,',len(studies),'business studies and',len(events),'native requests')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command',choices=['run','audit'])
    parser.add_argument('--output',type=Path,required=True)
    parser.add_argument('--url',default='http://127.0.0.1:18084/v1/systemone')
    args=parser.parse_args()
    run(args.output,args.url) if args.command=='run' else audit(args.output)


if __name__=='__main__':main()
