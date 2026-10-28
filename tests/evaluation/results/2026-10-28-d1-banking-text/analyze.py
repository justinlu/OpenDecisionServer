"""Offline matched historical comparison and failure listing; run from repository root."""
import json
from pathlib import Path
import sys
sys.path.insert(0,str(Path('tests/evaluation').resolve()))
from banking_text import load

ROOT=Path(__file__).resolve().parent
rows=load();lookup={r['id']:r for r in rows}
events=list(map(json.loads,(ROOT/'events.jsonl').read_text().splitlines()))
s=json.loads((ROOT/'summary.json').read_text())
failures=[dict(id=e['id'],expected=e['expected'],predicted=e['predicted'],error=e['error'],
    text=lookup[e['row_id']]['case']['transcript'],history=lookup[e['row_id']]['case']['history']) for e in events if not e['correct']]
(ROOT/'failures.json').write_text(json.dumps(failures,ensure_ascii=False,indent=2)+'\n')
prior=list(map(json.loads,(ROOT.parent/'2026-10-28-banking-audio-v1/predictions.jsonl').read_text().splitlines()))
legacy=[r for r in prior if r['group']=='text'];current={e['row_id'].removeprefix('legacy/')+'::'+e['node']:e for e in events if e['row_id'].startswith('legacy/')}
assert len(legacy)==128
comparison={}
for key in ['mode','function']:
 comparison[key]=dict(cases=128,prior_omni_text_correct=sum(r['predicted'][key]==r['expected'][key] for r in legacy),
    current_d1_text_correct=s['legacy']['full-'+key]['correct'],
    winner_disagreements=sum(r['predicted'][key]!=current[r['case_id']+'::full-'+key]['predicted'] for r in legacy))
oldcascade=json.loads((ROOT.parent/'2026-10-28-banking-audio-cascade/summary.json').read_text())
comparison['legacy_two_stage']=dict(semantic_cases=100,d1_text=s['legacy']['two-stage'],
    prior_omni_text=oldcascade['text'],prior_omni_audio=oldcascade['audio'],
    caveat='Same frozen core semantics and routing rubric. Audio has two TTS voices (200 recordings), text has 100 semantic cases. Different model, modality, and run time: not an isolated causal comparison or speed ranking.')
(ROOT/'comparison.json').write_text(json.dumps(comparison,ensure_ascii=False,indent=2)+'\n')
print('Rebuilt failure listing and historical comparison')
