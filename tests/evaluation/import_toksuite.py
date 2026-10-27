"""Import the 13 documented TokSuite Chinese configs at an immutable revision.

Only this optional import step requires pyarrow. Build/evaluation use the standard library.
"""
from collections import Counter
import hashlib
import io
import json
from pathlib import Path
import urllib.request

ROOT = Path(__file__).resolve().parent
REVISION = 'f3b76350c0bb4f94d008746f7ab1f3a04642958e'
BASE = 'https://huggingface.co/datasets/toksuite/toksuite_chinese/resolve/'+REVISION+'/'
CONFIGS = ('canonical', 'code_language_script_switching', 'colloquial', 'equivalent_expressions',
           'keyboard_proximity_errors', 'ocr_errors', 'optional_diacritics', 'partially_romanized',
           'romanization', 'space_removal', 'spelled_out', 'traditional',
           'word_spacing_zero-width_characters_extra_space')


def convert(row, config):
    if len(row['choices']) != 4 or not 0 <= row['answer'] < 4:
        raise ValueError('Unexpected upstream schema')
    keys = list('ABCD')
    return dict(id='toksuite/'+config+'/'+str(row['id']), suite='toksuite',
        family='toksuite/'+str(row['set_id']), domain='knowledge_and_completion', type='choice',
        form=config, prompt_language='zh', expected=keys[row['answer']],
        rationale='作者提供的原始答案；未根据本地模型结果修改。',
        source=BASE+'toksuite_chinese_'+config+'/test-00000-of-00001.parquet',
        upstream=row,
        request=dict(model='julia-1', state=row['question'], questions={'decision': dict(type='choice',
            instructions='根据题目选择正确答案或补全内容。', criteria=dict(zip(keys,row['choices'])))}))


def main():
    import pyarrow.parquet as pq
    manifest = dict(dataset='toksuite/toksuite_chinese', revision=REVISION, license='MIT',
        license_source=BASE+'README.md', files=[])
    rows = []
    with urllib.request.urlopen(BASE+'README.md', timeout=60) as stream:
        card = stream.read()
    (ROOT/'toksuite-source-card.md').write_bytes(card)
    manifest['source_card_sha256'] = hashlib.sha256(card).hexdigest()
    for config in CONFIGS:
        name='toksuite_chinese_'+config+'/test-00000-of-00001.parquet'
        with urllib.request.urlopen(BASE+name, timeout=60) as stream:
            raw=stream.read()
        source_rows=pq.read_table(io.BytesIO(raw)).to_pylist()
        manifest['files'].append(dict(path=name,sha256=hashlib.sha256(raw).hexdigest(),rows=len(source_rows)))
        rows.extend(convert(row,config) for row in source_rows)
        print(config,len(source_rows),flush=True)
    counts=Counter(row['family'] for row in rows)
    for row in rows:
        row['family_size']=counts[row['family']]
    if len(rows) != 485 or len({row['id'] for row in rows}) != len(rows):
        raise ValueError('Pinned source count/identity differs from expected 485 rows')
    output=''.join(json.dumps(row,ensure_ascii=False,separators=(',',':'))+'\n' for row in rows)
    manifest['converted_sha256']=hashlib.sha256(output.encode()).hexdigest()
    manifest['converted_rows']=len(rows)
    (ROOT/'toksuite.jsonl').write_text(output)
    (ROOT/'toksuite-manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')


if __name__ == '__main__':
    main()
