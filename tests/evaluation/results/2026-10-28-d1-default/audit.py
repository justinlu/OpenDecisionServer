"""Offline acceptance audit; run from the repository root."""
import hashlib
import json
from pathlib import Path
import sys
sys.path.insert(0,str(Path('tests/evaluation').resolve()))
from banking_text import audit as audit_bank
from evaluate import grade,load_rows,summarize

ROOT=Path(__file__).resolve().parent
for directory in ['chinese-banking','d1-native-banking']:audit_bank(ROOT/directory)
rows=list(map(json.loads,(ROOT/'d1-native-accuracy/predictions.jsonl').read_text().splitlines()))
source=load_rows([Path('tests/evaluation/custom.jsonl'),Path('tests/evaluation/toksuite.jsonl')])
assert len(rows)==len(source)==989
for recorded,original in zip(rows,source):
    assert all(recorded[k]==v for k,v in original.items())
    assert recorded['error'] is None and recorded['http_status']==200
    raw=json.loads(recorded['response_body']);assert raw['model']=='d1-2b'
    regraded=grade(original,raw,'median')
    assert all(recorded[k]==regraded[k] for k in regraded)
report=summarize(rows);saved=json.loads((ROOT/'d1-native-accuracy/summary.json').read_text())
assert all(saved[k]==v for k,v in report.items())
reference=list(map(json.loads,(ROOT.parent/'2026-10-28-liquid-d1-gguf/d1-accuracy/predictions.jsonl').read_text().splitlines()))
assert [r['id'] for r in rows]==[r['id'] for r in reference]
parity=dict(cases=989,winner_disagreements=sum(a['predicted']!=b['predicted'] for a,b in zip(rows,reference)),
    max_probability_absolute_delta=max(abs(a['probabilities'][k]-b['probabilities'][k]) for a,b in zip(rows,reference) for k in a['probabilities']))
assert parity['winner_disagreements']==0 and parity['max_probability_absolute_delta']==0
summary={name:json.loads((ROOT/name/'summary.json').read_text()) for name in ['chinese-banking','d1-native-banking']}
comparison={group:{name:{metric:summary[name][group][metric] for metric in ['full-mode','full-function','full-joint','two-stage','flat','domain','domain-then-leaf']} for name in summary} for group in ['legacy','credit','all']}
for name in ['native-contract.json','alias-contract.json']:
    contracts=json.loads((ROOT/name).read_text());assert len(contracts)==12
    assert all(r['status']==r['expected_status'] for r in contracts)
result=dict(native_vs_official=parity,banking=comparison)
path=ROOT/'comparison.json'
if path.exists():assert json.loads(path.read_text())==result
else:path.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
if (ROOT/'activation.json').exists():
    activation=json.loads((ROOT/'activation.json').read_text())
    release=json.loads((ROOT/'release.json').read_text())
    assert activation['commit']==release['commit']
    assert 'vcs.revision='+release['commit'] in release['build_info']
    assert 'vcs.modified=false' in release['build_info']
    assert activation['before_restart']['8080']['pid']!=activation['after_restart']['8080']['pid']
    assert activation['after_restart']['8080']['pid']==activation['after_restart']['18083']['pid']
    for port in ['8080','18083']:
        response=activation['after_restart'][port]['response']
        assert response['model']=='d1-2b' and response['answers']['business']['choice']=='withdraw'
        live=json.loads((ROOT/('live-contract-'+port+'.json')).read_text())
        assert len(live)==12 and all(r['status']==r['expected_status'] for r in live)
    print('Audited clean-release identity, shared live process, managed restart and 24 live protocol checks')
print('Audited 989 native responses, bank comparison, 24 protocol checks and exact probability parity')
