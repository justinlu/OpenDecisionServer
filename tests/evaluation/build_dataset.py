"""Build fixed, authored diagnostic cases. No model or transliterator is used."""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent
FORMS = ('zh', 'en', 'pinyin_spaced', 'pinyin_compact', 'zh_en', 'zh_digits',
         'en_digits', 'zh_pinyin', 'zh_en_digits_pinyin')


def make_case(family, domain, kind, target, state, form, language, question, options, rationale, phenomena):
    keys = ['false', 'true'] if kind == 'noul' else ([str(i) for i in range(len(options))] if kind == 'score' else ['c'+str(i) for i in range(len(options))])
    # Rotate choice order by family, fixed across languages/forms, without changing IDs.
    pairs = list(zip(keys, options))
    if kind == 'choice':
        shift = sum(family.encode()) % len(pairs)
        pairs = pairs[shift:] + pairs[:shift]
    criteria = options if kind == 'score' else dict(pairs)
    return dict(id=family+'/'+form+'/'+language, suite='custom', family=family,
                domain=domain, type=kind, form=form, prompt_language=language,
                family_size=6 if domain == "numeric" else 18,
                expected=keys[target], rationale=rationale, phenomena=phenomena,
                source='original-authored-v1', request=dict(model='julia-1', state=state,
                    questions={'decision': dict(type=kind, instructions=question, criteria=criteria)}))


def build_cases():
    cases = []
    for seed in json.loads((ROOT/'seeds.json').read_text()):
        states = dict(seed['states'])
        states['pinyin_compact'] = states['pinyin_spaced'].replace(' ', '')
        states['zh_digits'] = states['zh'] + ' 参考编号42，不影响判断。'
        states['en_digits'] = states['en'] + ' Reference ID 42; it does not affect the decision.'
        for form in FORMS:
            for language in ('zh', 'en'):
                cases.append(make_case(seed['id'], seed['domain'], seed['type'], seed['target'],
                    states[form], form, language, seed['question'][language], seed['options'][language],
                    seed['rationale'], seed['phenomena']))
    for kind, values, targets in [('choice', [9, 10, 20, 403], [0, 1, 2, 2]),
                                  ('noul', [17, 18, 0, 99], [0, 1, 0, 1]),
                                  ('score', [0, 10, 20, 25], [0, 1, 2, 2])]:
        for index, (value, target) in enumerate(zip(values, targets)):
            family = 'numeric-'+kind+'-'+str(index)
            for form, state in [('digits_ascii', str(value)), ('digits_fullwidth', str(value).translate(str.maketrans('0123456789', '０１２３４５６７８９'))), ('digits_leading_zero', str(value).zfill(5))]:
                for language in ('zh', 'en'):
                    if kind == 'noul':
                        q = {'zh':'把输入当作非负整数（全角数字和前导零不改变数值）。数值是否至少为十八？', 'en':'Read the input as a nonnegative integer; fullwidth digits and leading zeros do not change its value. Is it at least eighteen?'}
                        opts = {'zh':['小于十八','大于或等于十八'], 'en':['Less than eighteen','At least eighteen']}
                    else:
                        q = {'zh':'把输入当作非负整数（全角数字和前导零不改变数值）。按规则分类：小于十为低，十到十九为中，至少二十为高。', 'en':'Read the input as a nonnegative integer; fullwidth digits and leading zeros do not change its value. Classify by this rule: below ten is low, ten through nineteen is medium, at least twenty is high.'}
                        opts = {'zh':['低：小于十','中：十到十九','高：至少二十'], 'en':['Low: below ten','Medium: ten through nineteen','High: at least twenty']}
                    cases.append(make_case(family,'numeric',kind,target,state,form,language,q[language],opts[language],
                        '按题目明示的整数阈值判断；不要求外部知识。', ['threshold','boundary','numeric_format']))
    return cases


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, default=ROOT/'custom.jsonl')
    args = parser.parse_args()
    rows = build_cases()
    args.output.write_text(''.join(json.dumps(r, ensure_ascii=False, separators=(',', ':'))+'\n' for r in rows))
    print('Wrote', len(rows), 'cases to', args.output)


if __name__ == '__main__':
    main()
