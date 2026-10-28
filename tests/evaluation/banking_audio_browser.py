"""Create a self-contained, offline audio review page beside the frozen WAV files."""
import argparse
import html
import json
from pathlib import Path

from banking_audio import ROOT, CONTROLS, control_case, validate_dataset, verify_media


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dataset', type=Path, default=ROOT)
    parser.add_argument('--results', type=Path, required=True)
    args = parser.parse_args()
    cases, _ = validate_dataset(args.dataset)
    manifest = verify_media(args.dataset)
    cases += list(map(control_case, CONTROLS))
    predictions = {r['file']:r for r in map(json.loads,(args.results/'predictions.jsonl').read_text().splitlines())
                   if r['group'] in ['clean','noise15','control']}
    entries = {}
    for entry in manifest['audio']:
        entries.setdefault(entry['case_id'], []).append(entry)
    blocks = []
    escape = lambda value: html.escape(str(value), quote=True)
    for case in cases:
        items = []
        any_error = False
        for entry in entries[case['id']]:
            result = predictions[entry['file']]
            correct = result['predicted'] == case['expected']
            any_error |= not correct
            predicted = result['predicted']
            items.append('<li><b>'+escape(entry['voice']+' / '+entry['condition'])+'</b> · '+
                ('正确' if correct else '错误')+' · '+escape(predicted['mode'])+' / '+escape(predicted['function'])+
                '<br><audio controls preload="none" src="'+escape(entry['file'])+'"></audio></li>')
        search = case['id']+' '+(case['transcript'] or '')+' '+str(case['expected'])+' '+' '.join(case['tags'])
        history = '\n'.join(h['role']+'：'+h['content'] for h in case['history'])
        blocks.append('<details data-error="'+str(int(any_error))+'" data-search="'+escape(search.lower())+'">'+
            '<summary>'+('⚠ ' if any_error else '✓ ')+escape(case['transcript'] or '无语言控制：'+case['id'])+
            ' <small>'+escape(case['expected']['mode']+' / '+case['expected']['function'])+'</small></summary>'+
            '<p><code>'+escape(case['id'])+'</code> · '+escape(', '.join(case['tags']))+'</p>'+
            ('<pre>'+escape(history)+'</pre>' if history else '')+'<ul>'+''.join(items)+'</ul></details>')
    page='''<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>银行机具音频意图评测 v1</title><style>
body{font:16px/1.65 system-ui;max-width:1000px;margin:32px auto;padding:0 20px;color:#162736;background:#f5f7fa}
details{background:white;border:1px solid #d6e0e8;border-radius:8px;padding:12px 16px;margin:10px 0}summary{cursor:pointer}
small{color:#47627a}input{font:inherit;padding:8px;max-width:90%;width:480px}li{margin:14px 0}audio{width:min(500px,100%)}
pre{white-space:pre-wrap}label{margin:12px;display:inline-block}</style>
<h1>银行机具音频意图评测 v1</h1><p>原生音频回归 · 两种合成声线 · 合成噪声。页面展示冻结标签和本轮模型判断；不代表真人或银行生产准确率。</p>
<input id="query" placeholder="搜索语句、业务、标签或 case ID"><label><input id="errors" type="checkbox" style="width:auto">仅看有错例的用例</label><p id="count"></p>
'''+''.join(blocks)+'''<script>
const query=document.querySelector('#query'), errors=document.querySelector('#errors');
function filter(){let n=0;for(const row of document.querySelectorAll('details')){
 row.hidden=!(row.dataset.search.includes(query.value.toLowerCase())&&(!errors.checked||row.dataset.error==='1'));
 if(!row.hidden)n++;}document.querySelector('#count').textContent='显示 '+n+' 个语义/控制用例；展开后可试听各版本。';}
query.addEventListener('input',filter);errors.addEventListener('change',filter);filter();</script></html>'''
    (args.dataset/'index.html').write_text(page)
    print(args.dataset/'index.html')


if __name__ == '__main__':
    main()
