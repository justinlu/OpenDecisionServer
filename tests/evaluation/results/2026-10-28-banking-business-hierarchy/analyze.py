"""Derive coarse-page accuracy and fixed-gate flat replay; no inference."""
from collections import defaultdict
import json
from pathlib import Path
import sys

HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(HERE.parents[1]))
from banking_audio import write_json
from banking_audio_hierarchy import samples,domain_for


def main():
    all_samples,_=samples();lookup={s['key']:s for s in all_samples}
    spec=json.loads((HERE/'specification.json').read_text())
    events={e['id']:e for e in map(json.loads,(HERE/'classifications.jsonl').read_text().splitlines())}
    routes=list(map(json.loads,(HERE/'routes.jsonl').read_text().splitlines()))
    studies=list(map(json.loads,(HERE/'business-study.jsonl').read_text().splitlines()))
    study_by_key={r['key']:r for r in studies}
    report={}
    for origin in ['legacy','credit','all']:
        for modality in ['text','audio']:
            for voice in (['all','tingting','eddy'] if modality=='audio' else ['all']):
                selected=[r for r in routes if (origin=='all' or lookup[r['key']]['origin']==origin)
                          and lookup[r['key']]['modality']==modality
                          and (voice=='all' or lookup[r['key']]['voice']==voice)]
                study=[r for r in studies if (origin=='all' or lookup[r['key']]['origin']==origin)
                       and lookup[r['key']]['modality']==modality
                       and (voice=='all' or lookup[r['key']]['voice']==voice)]
                out=dict(routes=len(selected),navigation=0,coarse_page_correct=0,flat16_fixed_gate_replay_correct=0,
                         navigation_failures=dict(gate=0,domain=0,leaf=0,success=0),domain_confusion={})
                for row in selected:
                    expected=lookup[row['key']]['case']['expected'];gate=events[row['gate']]
                    if expected['mode']=='inquire':
                        good=not gate['error'] and gate['predicted']=='inquire'
                        out['coarse_page_correct']+=good;out['flat16_fixed_gate_replay_correct']+=good
                        continue
                    out['navigation']+=1
                    if gate['error'] or gate['predicted']!='navigate':
                        out['navigation_failures']['gate']+=1;continue
                    flat=events[study_by_key[row['key']]['flat16']]
                    out['flat16_fixed_gate_replay_correct']+=flat['correct']
                    domain=events[row['domain']]
                    if domain['error'] or domain['predicted']!=domain_for(spec,expected['function']):
                        out['navigation_failures']['domain']+=1;continue
                    out['coarse_page_correct']+=1
                    leaf=events[row['leaf']] if row['leaf'] else None
                    if not leaf or leaf['error'] or leaf['predicted']!=expected['function']:
                        out['navigation_failures']['leaf']+=1
                    else:out['navigation_failures']['success']+=1
                matrix=defaultdict(lambda:defaultdict(int))
                for row in study:
                    expected=domain_for(spec,lookup[row['key']]['case']['expected']['function'])
                    matrix[expected][events[row['domain']]['predicted'] or '__error__']+=1
                out['domain_confusion']={k:dict(v) for k,v in matrix.items()}
                out['business_study']={}
                for condition in ['flat11','flat16','domain','oracle_domain_leaf','oracle3','oracle5']:
                    eligible=[r for r in study if r.get(condition)]
                    out['business_study'][condition]=dict(cases=len(eligible),correct=sum(events[r[condition]]['correct'] for r in eligible))
                report[origin+'/'+modality+'/'+voice]=out
    report['warning']='Flat16 fixed-gate replay combines actual first-gate predictions with separately measured flat decisions. No replay latency or independent validation claim.'
    write_json(HERE/'additional-analysis.json',report)
    print('Recomputed coarse-page and fixed-gate replay scores, by source and synthetic condition.')


if __name__=='__main__':main()
