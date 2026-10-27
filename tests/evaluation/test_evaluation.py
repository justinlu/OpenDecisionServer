import math
import json
import hashlib
import http.client
from pathlib import Path
import tempfile
from unittest.mock import patch
import unittest

from build_dataset import build_cases
from evaluate import grade, summarize, stats, main
from import_toksuite import convert


class DatasetTests(unittest.TestCase):
    def test_complete_paired_coverage_and_reproducibility(self):
        rows = build_cases()
        self.assertEqual(rows, build_cases())
        self.assertEqual(len(rows), 504)
        self.assertEqual(len({r['id'] for r in rows}), len(rows))
        self.assertEqual(len({r['family'] for r in rows}), 36)
        for row in rows:
            self.assertIsInstance(row['request']['state'], str)
            if row['form'].startswith('digits_'):
                self.assertTrue(row['request']['state'].isdigit())
            if row['form'] in ('en', 'pinyin_spaced', 'pinyin_compact'):
                self.assertTrue(row['request']['state'].isascii())
        groups = {}
        for row in rows:
            groups.setdefault(row['family'], []).append(row)
        for family in groups.values():
            self.assertEqual(len({r['expected'] for r in family}), 1)
            self.assertEqual({r['prompt_language'] for r in family}, {'zh', 'en'})
            self.assertIn(len(family), (6, 18))
            self.assertTrue(all(r['family_size'] == len(family) for r in family))

    def test_checked_in_custom_file_matches_fixed_seeds(self):
        rows = [json.loads(line) for line in (Path(__file__).parent/'custom.jsonl').read_text().splitlines()]
        self.assertEqual(rows, build_cases())

    def test_external_snapshot_matches_pinned_manifest(self):
        root = Path(__file__).parent
        manifest = json.loads((root/'toksuite-manifest.json').read_text())
        raw = (root/'toksuite.jsonl').read_bytes()
        self.assertEqual(hashlib.sha256(raw).hexdigest(), manifest['converted_sha256'])
        self.assertEqual(hashlib.sha256((root/'toksuite-source-card.md').read_bytes()).hexdigest(), manifest['source_card_sha256'])
        rows = [json.loads(line) for line in raw.splitlines()]
        self.assertEqual(len(rows), 485)
        self.assertEqual(len({r['form'] for r in rows}), 13)
        self.assertEqual(len({r['family'] for r in rows}), 40)
        for row in rows:
            self.assertEqual(row['expected'], 'ABCD'[row['upstream']['answer']])

    def test_external_conversion_preserves_authors_answer_and_choices(self):
        source = dict(id='example', set_id='family', question='天气如何', choices=['晴','雨','雪','雾'], answer=2)
        row = convert(source, 'canonical')
        self.assertEqual(row['expected'], 'C')
        self.assertEqual(row['upstream'], source)
        self.assertEqual(row['request']['questions']['decision']['criteria']['C'], '雪')

    def test_task_and_domain_coverage(self):
        rows = build_cases()
        self.assertEqual({r['type'] for r in rows}, {'choice', 'noul', 'score'})
        self.assertEqual(len({r['domain'] for r in rows if r['domain'] != 'numeric'}), 8)
        self.assertEqual({r['expected'] for r in rows if r['type'] == 'noul'}, {'false', 'true'})


class ScoringTests(unittest.TestCase):
    def case(self, kind='score'):
        return next(r for r in build_cases() if r['type'] == kind)

    def test_score_uses_argmax_not_rounded_expected_index(self):
        row = self.case()
        response = {'model': 'julia-1', 'answers': {'decision': {
            'type': 'score', 'score': 0.9, 'probabilities': {'0': .55, '1': 0., '2': .45}}}}
        self.assertEqual(grade(row, response)['predicted'], '0')

    def test_null_answer_is_a_validation_error(self):
        row = self.case('choice')
        for response in (None, [], {'model': 'julia-1', 'answers': None},
                         {'model': 'julia-1', 'answers': {'decision': None}}):
            with self.assertRaises(ValueError):
                grade(row, response)

    def test_noul_tie_uses_false_first(self):
        row = self.case('noul')
        response = {'model': 'julia-1', 'answers': {'decision': {'type': 'noul', 'noul': .5}}}
        self.assertEqual(grade(row, response)['predicted'], 'false')

    def test_rejects_bad_probability_and_mismatched_choice(self):
        row = self.case('choice')
        for probs in ({'c0': math.nan, 'c1': 0., 'c2': 1.}, {'c0': .8, 'c1': .8, 'c2': .1}):
            with self.assertRaises(ValueError):
                grade(row, {'model': 'julia-1', 'answers': {'decision': {
                    'type': 'choice', 'choice': 'c0', 'probabilities': probs}}})
        with self.assertRaises(ValueError):
            grade(row, {'model': 'julia-1', 'answers': {'decision': {
                'type': 'choice', 'choice': 'c1', 'probabilities': {'c0': .8, 'c1': .1, 'c2': .1}}}})

    def test_macro_f1_includes_all_declared_candidate_labels(self):
        row = self.case('choice')
        records = [dict(row, expected='c0', predicted='c0', correct=True, error=None),
                   dict(row, expected='c0', predicted='c1', correct=False, error=None)]
        self.assertAlmostEqual(stats(records)['macro_f1'], 2/9)

    def test_http_malformed_response_preserves_all_rows_and_summary(self):
        rows = build_cases()[:2]
        class Response:
            def __enter__(self): return self
            def __exit__(self, *args): pass
            def read(self): return b'{"model":"julia-1","answers":{"decision":null}}'
        with tempfile.TemporaryDirectory() as directory:
            dataset = Path(directory)/'cases.jsonl'
            dataset.write_text(''.join(json.dumps(row)+'\n' for row in rows))
            output = Path(directory)/'results'
            with patch('sys.argv', ['evaluate.py','--dataset',str(dataset),'--output',str(output)]), patch('urllib.request.urlopen', return_value=Response()):
                with self.assertRaises(SystemExit) as exit_result:
                    main()
                self.assertEqual(exit_result.exception.code, 1)
            summary = json.loads((output/'summary.json').read_text())
            self.assertEqual(summary['overall']['total'], 2)
            self.assertEqual(summary['overall']['errors'], 2)
            self.assertEqual(len((output/'predictions.jsonl').read_text().splitlines()), 2)

    def test_truncated_http_response_is_counted_as_an_error(self):
        row = self.case('choice')
        with tempfile.TemporaryDirectory() as directory:
            dataset = Path(directory)/'cases.jsonl'
            dataset.write_text(json.dumps(row)+'\n')
            output = Path(directory)/'results'
            with patch('sys.argv', ['evaluate.py','--dataset',str(dataset),'--output',str(output)]), patch('urllib.request.urlopen', side_effect=http.client.IncompleteRead(b'', 10)):
                with self.assertRaises(SystemExit) as exit_result:
                    main()
                self.assertEqual(exit_result.exception.code, 1)
            self.assertEqual(json.loads((output/'summary.json').read_text())['overall']['errors'], 1)

    def test_errors_stay_in_denominator_and_pairing_is_not_success(self):
        rows = [r for r in build_cases() if r['family'] == 'routing-choice'][:2]
        records = [dict(rows[0], correct=True, predicted=rows[0]['expected'], error=None),
                   dict(rows[1], correct=False, predicted=None, error='HTTP 422')]
        report = summarize(records)
        self.assertEqual(report['overall']['accuracy'], .5)
        self.assertEqual(report['overall']['errors'], 1)
        self.assertEqual(report['paired']['families_all_correct'], 0)
        self.assertEqual(report['paired']['families_complete'], 0)


if __name__ == '__main__':
    unittest.main()
