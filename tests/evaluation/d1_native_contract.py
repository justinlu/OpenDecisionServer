"""HTTP checks for the in-process text-only d1 adapter, no model tuning."""
import argparse
import copy
import json
import urllib.request
import urllib.error
from pathlib import Path


def checks(url,output):
    question=dict(type='choice',instructions='用户要办理什么业务？',criteria=dict(withdraw='现金取款',deposit='现金存款',transfer='转账'))
    base=dict(state='我要取现金',questions={'business':question})
    jobs=[]
    jobs.append(('default-model',base,200))
    jobs.append(('explicit-model',dict(base,model='d1-2b'),200))
    jobs.append(('unknown-model',dict(base,model='chinese-decision-2b'),404))
    for field in ['files','images','videos']:
        jobs.append(('reject-'+field,dict(base,**{field:['data:application/octet-stream;base64,AAAA']}),422))
    jobs.append(('reject-spoken-question',dict(state='x',questions={'q':dict(type='noul',instructions={'type':'input_audio','input_audio':{'data':'AAAA','format':'wav'}})}),422))
    jobs.append(('long-input',dict(base,state='customer balance inquiry '*20000),422))
    jobs.append(('multiple',dict(base,questions={'q':dict(question,multiple=True)}),422))
    jobs.append(('over-64-questions',dict(base,questions={f'q{i}':question for i in range(65)}),422))
    jobs.append(('forty-options',dict(state='the exact category is category 39',questions={'q':dict(type='choice',instructions='Choose the matching category.',criteria={f'k{i}':f'category {i}' for i in range(40)})}),200))
    jobs.append(('all-types-ordered',dict(state='用户想要取现金',questions={'z':question,'a':dict(type='noul',instructions='用户想取款吗？'),'score':dict(type='score',instructions='用户想取款的程度',criteria=['不想','不确定','想'])}),200))
    rows=[]
    for name,body,status in jobs:
        data=json.dumps(body,ensure_ascii=False).encode();req=urllib.request.Request(url,data=data,headers={'Content-Type':'application/json'})
        try:
            with urllib.request.urlopen(req,timeout=120) as response:code=response.status;raw=response.read().decode()
        except urllib.error.HTTPError as error:code=error.code;raw=error.read().decode()
        payload=json.loads(raw);assert code==status,(name,code,raw)
        if code==200:
            assert payload['model']=='d1-2b'
            assert payload['usage']['input_tokens']>0 and payload['usage']['output_tokens']==0
            if name in ['default-model','explicit-model']:assert payload['answers']['business']['choice']=='withdraw'
            if name=='forty-options':assert len(payload['answers']['q']['probabilities'])==40
            if name=='all-types-ordered':
                assert payload['answers']['z']['choice']=='withdraw'
                assert payload['answers']['a']['noul']>.5
                probs=payload['answers']['score']['probabilities'];total=0;median=None
                for i in range(3):
                    total+=probs[str(i)]
                    if total>=.5:median=i;break
                assert payload['answers']['score']['score']==median
        rows.append(dict(name=name,request=body,expected_status=status,status=code,response_body=raw))
    output.write_text(json.dumps(rows,ensure_ascii=False,indent=2)+'\n');print('Passed',len(rows),'HTTP contract checks')

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--url',default='http://127.0.0.1:18085/v1/systemone');p.add_argument('--output',type=Path,required=True);a=p.parse_args();checks(a.url,a.output)
