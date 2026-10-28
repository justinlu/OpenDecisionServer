"""Post-hoc probes; never replace the frozen primary banking audio evaluation."""
import argparse
import copy
import hashlib
import http.client
import json
from pathlib import Path
import random
import time
import urllib.error
import urllib.request

from banking_audio import ROOT, build_request, sha, validate_dataset, verify_media, write_json
from evaluate import grade


def request_one(url, body, expected):
    encoded=json.dumps(body,ensure_ascii=False).encode()
    row=dict(expected=expected,predicted=None,correct=False,error=None,http_status=None,response_body=None,
             payload_sha256=hashlib.sha256(encoded).hexdigest())
    started=time.perf_counter()
    try:
        req=urllib.request.Request(url,data=encoded,headers={'Content-Type':'application/json'})
        with urllib.request.urlopen(req,timeout=90) as response:
            row.update(http_status=response.status,response_body=response.read().decode())
        payload=json.loads(row['response_body'])
        if not isinstance(payload,dict) or payload.get('model')!=body['model']:
            raise ValueError('Invalid response or model')
        row.update(grade(dict(expected=expected,request={'questions':body['questions']}),payload))
    except urllib.error.HTTPError as error:
        row.update(http_status=error.code,response_body=error.read().decode(errors='replace'),error=str(error))
    except (OSError,http.client.HTTPException,ValueError,KeyError,TypeError) as error:
        row['error']=type(error).__name__+': '+str(error)
    row['latency_ms']=(time.perf_counter()-started)*1000
    return row


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dataset',type=Path,default=ROOT)
    parser.add_argument('--output',type=Path,required=True)
    parser.add_argument('--url',default='http://127.0.0.1:18084/v1/systemone')
    args=parser.parse_args()
    cases,task=validate_dataset(args.dataset)
    manifest=verify_media(args.dataset)
    args.output.mkdir(parents=True,exist_ok=False)
    write_json(args.output/'metadata.json',dict(post_hoc=True,
        purpose='Binary label restriction and closed-set sentence discrimination after primary evaluation. No ASR accuracy claim.',
        dataset_manifest_sha256=sha(args.dataset/'manifest.json'),runner_sha256=sha(Path(__file__)),url=args.url))
    clean={(e['case_id'],e['voice']):e for e in manifest['audio'] if e['condition']=='clean'}
    core=[c for c in cases if 'core' in c['tags']]
    binary=copy.deepcopy(task)
    question=copy.deepcopy(task['questions']['mode'])
    question['instructions']='只区分最新用户现在是要进入、打开、办理或执行查询功能，还是只咨询业务规则、费用、含义、步骤。礼貌问句不等于咨询，以真正诉求为准。'
    question['criteria']={k:question['criteria'][k] for k in ['navigate','inquire']}
    binary['questions']={'decision':question}
    jobs=[]
    for case in core:
        jobs.append((case,binary,'binary_text',None,case['expected']['mode']))
        for voice in task['voices']:
            jobs.append((case,binary,'binary_audio',clean[(case['id'],voice['id'])],case['expected']['mode']))
    rng=random.Random(task['seed'])
    topics=['withdraw','deposit','transfer','balance','statement','pin']
    lookup={c['id']:c for c in core}
    for i,topic in enumerate(topics):
        other=topics[(i+1)%len(topics)]
        for mode in ['navigate','inquire']:
            case=lookup[f'{topic}-02-{mode}']
            options=[lookup[f'{t}-02-{m}'] for t in [topic,other] for m in ['navigate','inquire']]
            rng.shuffle(options)
            expected='c'+str(next(j for j,c in enumerate(options) if c['id']==case['id']))
            probe=dict(task,questions={'decision':dict(type='choice',
                instructions='只根据音频，选择说话人实际说出的那一句话。不是回答或执行这些句子。',
                criteria={'c'+str(j):c['transcript'] for j,c in enumerate(options)})})
            for voice in task['voices']:
                jobs.append((case,probe,'sentence_audio',clean[(case['id'],voice['id'])],expected))
    rng.shuffle(jobs)
    rows=[]
    with (args.output/'predictions.jsonl').open('w') as stream:
        for index,(case,variant,group,entry,expected) in enumerate(jobs):
            body=build_request(case,variant,'audio' if entry else 'text',args.dataset/entry['file'] if entry else None)
            row=dict(case_id=case['id'],group=group,voice=entry['voice'] if entry else 'none',
                file=entry['file'] if entry else None,question=variant['questions']['decision'],
                **request_one(args.url,body,expected))
            rows.append(row)
            stream.write(json.dumps(row,ensure_ascii=False)+'\n')
            stream.flush()
            if index%32==0 or row['error']:
                print(index+1,group,row['error'],flush=True)
    def summarize(selected):
        return dict(requests=len(selected),correct=sum(r['correct'] for r in selected),
            accuracy=sum(r['correct'] for r in selected)/len(selected),errors=sum(r['error'] is not None for r in selected),
            inquiry_to_navigation=sum(r['expected']=='inquire' and r['predicted']=='navigate' for r in selected),
            missed_navigation=sum(r['expected']=='navigate' and r['predicted']!='navigate' for r in selected))
    summary={group:summarize([r for r in rows if r['group']==group]) for group in ['binary_text','binary_audio','sentence_audio']}
    summary['by_voice']={group:{voice['id']:summarize([r for r in rows if r['group']==group and r['voice']==voice['id']])
        for voice in task['voices']} for group in ['binary_audio','sentence_audio']}
    write_json(args.output/'summary.json',summary)
    print(json.dumps(summary,indent=2),flush=True)
    if any(r['error'] for r in rows):
        raise SystemExit('Completed with recorded errors')


if __name__=='__main__':
    main()
