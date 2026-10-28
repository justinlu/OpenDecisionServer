"""Two-stage banking routing on the existing frozen corpus; architecture diagnostic only."""
import argparse
import copy
import json
from pathlib import Path
import random
import time

from banking_audio import ROOT, build_request, sha, validate_dataset, verify_media, write_json
from banking_audio_diagnostics import request_one


def needs_business(gate):
    return gate['error'] is None and gate['predicted']=='navigate'


def route_success(expected, gate, business):
    if gate['error'] or gate['predicted']!=expected['mode']:
        return False
    if expected['mode']=='inquire':
        return True
    return business is not None and business['error'] is None and business['predicted']==expected['function']


def questions(task):
    first=copy.deepcopy(task['questions']['mode'])
    first['instructions']='只区分最新用户现在是要进入、打开、办理或执行查询功能，还是只咨询业务规则、费用、含义、步骤。礼貌问句不等于咨询，以真正诉求为准。'
    first['criteria']={k:first['criteria'][k] for k in ['navigate','inquire']}
    second=dict(type='choice',
        instructions='当前处于银行自助机具的办理分支。只识别用户希望进入的具体办理业务，不再输出咨询或办理类别。以当前诉求为准，忽略已否定的目标；无法确定唯一办理业务时选unknown。只选择业务，不执行交易。',
        criteria=dict(withdraw='现金取款',deposit='现金存款',transfer='转账汇款',balance='查询银行卡余额',
            statement='查询交易明细或打印银行流水',pin='修改或重置银行卡取款密码',loss='银行卡挂失',
            replace='补办或更换银行卡',phone='修改银行预留手机号',mobile='开通或签约手机银行',
            unknown='无法确定唯一的办理业务，或不在以上候选功能内'))
    return first, second


def summarize(rows):
    gates=[r['gate'] for r in rows]
    navigation=[r for r in rows if r['expected']['mode']=='navigate']
    consultation=[r for r in rows if r['expected']['mode']=='inquire']
    dispatched=[r for r in rows if r['business'] is not None]
    latencies=sorted(r['route_elapsed_ms'] for r in rows)
    return dict(cases=len(rows),gate_correct=sum(g['correct'] for g in gates),
        gate_accuracy=sum(g['correct'] for g in gates)/len(rows),
        routing_correct=sum(r['route_correct'] for r in rows),
        routing_accuracy=sum(r['route_correct'] for r in rows)/len(rows),
        navigation_cases=len(navigation),correct_navigation=sum(r['route_correct'] for r in navigation),
        consultation_cases=len(consultation),consultation_misgated=sum(needs_business(r['gate']) for r in consultation),
        consultation_entered_known_business=sum(needs_business(r['gate']) and r['business']['predicted'] not in [None,'unknown']
            and not r['business']['error'] for r in consultation),
        missed_navigation=sum(not needs_business(r['gate']) for r in navigation),
        business_calls=len(dispatched),business_label_correct_on_dispatched=sum(r['business']['correct'] for r in dispatched),
        oracle_navigation_business_correct=sum(r['oracle_business']['correct'] for r in navigation),
        oracle_navigation_business_accuracy=sum(r['oracle_business']['correct'] for r in navigation)/len(navigation),
        gate_errors=sum(g['error'] is not None for g in gates),
        business_errors=sum(r['business']['error'] is not None for r in dispatched),
        oracle_errors=sum(r['oracle_business']['error'] is not None for r in navigation),
        route_latency_ms=dict(p50=latencies[(len(rows)-1)//2],p95=latencies[__import__('math').ceil(len(rows)*.95)-1]))


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',type=Path,required=True)
    parser.add_argument('--dataset',type=Path,default=ROOT)
    parser.add_argument('--url',default='http://127.0.0.1:18084/v1/systemone')
    args=parser.parse_args()
    cases,task=validate_dataset(args.dataset)
    cases=[c for c in cases if 'core' in c['tags']]
    manifest=verify_media(args.dataset)
    media={ (e['case_id'],e['voice']):e for e in manifest['audio'] if e['condition']=='clean'}
    first,second=questions(task)
    args.output.mkdir(parents=True,exist_ok=False)
    write_json(args.output/'questions.json',dict(gate=first,business=second))
    write_json(args.output/'metadata.json',dict(post_hoc=True,model=task['model'],url=args.url,
        runner_sha256=sha(Path(__file__)),dataset_manifest_sha256=sha(args.dataset/'manifest.json'),
        dispatch_rule='Only predicted navigate and no gate error calls stage two',
        oracle_rule='After actual routing, missed true-navigation cases get a separate business-only diagnostic; never affects route score',
        timing='route_elapsed_ms includes request construction and actual stages, excludes later oracle diagnosis'))
    jobs=[]
    for case in cases:
        jobs.append((case,None))
        jobs.extend((case,media[(case['id'],v['id'])]) for v in task['voices'])
    random.Random(task['seed']).shuffle(jobs)
    def call(case,entry,question,expected):
        variant=dict(task,questions={'decision':question})
        body=build_request(case,variant,'audio' if entry else 'text',args.dataset/entry['file'] if entry else None)
        return request_one(args.url,body,expected)
    rows=[]
    with (args.output/'routes.jsonl').open('w') as stream:
        for index,(case,entry) in enumerate(jobs):
            start=time.perf_counter()
            gate=call(case,entry,first,case['expected']['mode'])
            business=call(case,entry,second,case['expected']['function']) if needs_business(gate) else None
            elapsed=(time.perf_counter()-start)*1000
            row=dict(case_id=case['id'],modality='audio' if entry else 'text',voice=entry['voice'] if entry else 'none',
                file=entry['file'] if entry else None,expected=case['expected'],gate=gate,business=business,
                route_correct=route_success(case['expected'],gate,business),route_elapsed_ms=elapsed)
            rows.append(row)
            stream.write(json.dumps(row,ensure_ascii=False)+'\n');stream.flush()
            if index%32==0:print(index+1,'actual routes',flush=True)
    # Genuine routing is finished. Oracle calls are diagnostic, recorded separately.
    lookup={c['id']:c for c in cases}
    with (args.output/'oracle-navigation.jsonl').open('w') as stream:
        for row in rows:
            if row['expected']['mode']!='navigate':continue
            entry=media[(row['case_id'],row['voice'])] if row['file'] else None
            oracle=row['business'] if row['business'] is not None else call(lookup[row['case_id']],entry,second,row['expected']['function'])
            row['oracle_business']=oracle
            stream.write(json.dumps(dict(case_id=row['case_id'],voice=row['voice'],
                reused_actual=row['business'] is not None,result=oracle),ensure_ascii=False)+'\n')
    summary={m:summarize([r for r in rows if r['modality']==m]) for m in ['text','audio']}
    summary['audio_by_voice']={v['id']:summarize([r for r in rows if r['voice']==v['id']]) for v in task['voices']}
    old_root=Path(__file__).resolve().parent/'results/2026-10-28-banking-audio-v1'
    old=[json.loads(line) for line in (old_root/'predictions.jsonl').read_text().splitlines()]
    summary['previous_setting_same_routing_rubric']={}
    for group in ['text','clean']:
        selected=[r for r in old if r['group']==group and 'core' in r['tags']]
        success=sum(r['predicted']['mode']=='inquire' if r['expected']['mode']=='inquire' else
                    r['predicted']==r['expected'] for r in selected)
        summary['previous_setting_same_routing_rubric'][group]=dict(cases=len(selected),correct=success,accuracy=success/len(selected))
    write_json(args.output/'summary.json',summary)
    print(json.dumps(summary,indent=2),flush=True)
    if any(r['gate']['error'] or (r['business'] and r['business']['error']) or
           (r.get('oracle_business') and r['oracle_business']['error']) for r in rows):
        raise SystemExit('Completed with recorded inference errors')


if __name__=='__main__':
    main()
