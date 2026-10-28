"""Post-hoc positive domain descriptions, with explicitly cached-leaf quality replay."""
import argparse
import hashlib
import json
from pathlib import Path
import random

from banking_audio import ROOT,build_request,sha,validate_dataset,write_json
from banking_audio_hierarchy import CREDIT,samples,domain_for
from banking_audio_diagnostics import request_one


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--results',type=Path,required=True)
    parser.add_argument('--url',default='http://127.0.0.1:18084/v1/systemone')
    parser.add_argument('--audit',action='store_true')
    args=parser.parse_args();base=args.results
    output=base/'short-domain'
    spec=json.loads((base/'specification.json').read_text())
    all_samples,_=samples();lookup={s['key']:s for s in all_samples};_,task=validate_dataset(ROOT)
    events={e['id']:e for e in map(json.loads,(base/'classifications.jsonl').read_text().splitlines())}
    studies={r['key']:r for r in map(json.loads,(base/'business-study.jsonl').read_text().splitlines())}
    routes=list(map(json.loads,(base/'routes.jsonl').read_text().splitlines()))
    question=dict(type='choice',instructions='用户当前希望进入哪个办理业务域？按主要功能选择一个业务域，无法确定则选unknown。',
        criteria=dict(cash='现金业务：存款、取款',account='账户交易和查询：转账、余额、流水',
            card='卡片管理：取款密码、挂失、补卡、换卡',digital='渠道服务：改预留手机号、开通手机银行',
            credit='信用卡专属业务：还款、账单、额度、账单分期、申请信用卡',unknown='业务不明确或不在以上范围'))
    def body(sample):
        root=ROOT if sample['origin']=='legacy' else CREDIT
        return build_request(sample['case'],dict(task,questions={'decision':question}),sample['modality'],
                             root/sample['file'] if sample['file'] else None)
    if not args.audit:
        output.mkdir(exist_ok=False)
        write_json(output/'question.json',question)
        write_json(output/'metadata.json',dict(post_hoc=True,runner_sha256=sha(Path(__file__)),
            warning='Only domain is freshly inferred. Leaf and first gate are replayed from existing identical-input measurements; no live routing latency claim.'))
        keys=list(studies);random.Random(20261028).shuffle(keys)
        with (output/'predictions.jsonl').open('w') as stream:
            for index,key in enumerate(keys):
                sample=lookup[key];expected=domain_for(spec,sample['case']['expected']['function'])
                row=dict(key=key,**request_one(args.url,body(sample),expected))
                stream.write(json.dumps(row,ensure_ascii=False)+'\n');stream.flush()
                if index%32==0:print(index+1,'short domain',flush=True)
    predictions=list(map(json.loads,(output/'predictions.jsonl').read_text().splitlines()))
    assert len(predictions)==len(studies) and {r['key'] for r in predictions}==set(studies)
    assert json.loads((output/'question.json').read_text())==question
    from evaluate import grade
    for row in predictions:
        sample=lookup[row['key']];request=body(sample)
        assert row['expected']==domain_for(spec,sample['case']['expected']['function'])
        assert row['payload_sha256']==hashlib.sha256(json.dumps(request,ensure_ascii=False).encode()).hexdigest()
        assert row['error'] is None and row['http_status']==200
        payload=json.loads(row['response_body']);assert payload['model']==task['model']
        result=grade(dict(expected=row['expected'],request={'questions':request['questions']}),payload)
        assert all(row[k]==result[k] for k in ['correct','predicted','probabilities'])
    pred={r['key']:r for r in predictions};report={}
    for origin in ['legacy','credit','all']:
        for modality in ['text','audio']:
            selected=[r for r in predictions if (origin=='all' or lookup[r['key']]['origin']==origin)
                      and lookup[r['key']]['modality']==modality]
            tree=lambda key:pred[key]['correct'] and events[studies[key]['oracle_domain_leaf']]['correct']
            actual=[r for r in routes if (origin=='all' or lookup[r['key']]['origin']==origin)
                    and lookup[r['key']]['modality']==modality]
            success=0
            for row in actual:
                expected=lookup[row['key']]['case']['expected'];gate=events[row['gate']]
                if not gate['error'] and gate['predicted']==expected['mode']:
                    success+=expected['mode']=='inquire' or tree(row['key'])
            report[origin+'/'+modality]=dict(domain_cases=len(selected),domain_correct=sum(r['correct'] for r in selected),
                cached_leaf_tree_correct=sum(tree(r['key']) for r in selected),full_gate_and_leaf_replay_cases=len(actual),
                full_gate_and_leaf_replay_correct=success)
    write_json(output/'summary.json',report)
    print(json.dumps(report,indent=2),flush=True)


if __name__=='__main__':main()
