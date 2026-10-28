import json
import math
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from banking_audio import audit, build_request, noise_cases, noisy_pcm, send, summarize, validate_dataset, ROOT


class BankingAudioTests(unittest.TestCase):
    def test_noise_is_reproducible_and_targets_the_declared_snr(self):
        samples = [2000*math.sin(2*math.pi*300*i/16000) for i in range(16000)]
        mixed = noisy_pcm(samples, 15, 123)
        self.assertEqual(mixed, noisy_pcm(samples, 15, 123))
        power = sum(x*x for x in samples)
        noise_power = sum((a-b)**2 for a,b in zip(mixed, samples))
        self.assertAlmostEqual(10*math.log10(power/noise_power), 15, places=8)
        cases, task = validate_dataset(ROOT)
        selected = [c for c in cases if c['id'] in noise_cases(cases, task)]
        self.assertEqual(len(selected), 32)
        self.assertEqual(len({c['pair'] for c in selected}), 16)
        self.assertEqual(len({c['expected']['function'] for c in selected}), 10)
        self.assertEqual(sum(c['expected']['mode']=='navigate' for c in selected), 16)

    def test_non_object_success_responses_are_recorded_errors(self):
        class Response:
            status = 200
            def __init__(self, value):
                self.value = value
            def __enter__(self):
                return self
            def __exit__(self, *args):
                pass
            def read(self):
                return json.dumps(self.value).encode()
        cases, task = validate_dataset(ROOT)
        body = build_request(cases[0], task, 'text')
        for value in [None, [], 'not a decision response']:
            with self.subTest(value=value), patch('banking_audio.urllib.request.urlopen', return_value=Response(value)):
                result = send('http://localhost/test', body, cases[0]['expected'])
                self.assertEqual(result['http_status'], 200)
                self.assertIsNotNone(result['error'])
                self.assertEqual(result['predicted'], {'mode': None, 'function': None})

    def test_auditor_rejects_changed_case_pair_family_and_tags(self):
        cases, _ = validate_dataset(ROOT)
        case = cases[0]
        job = dict(id='audio/original.wav', case_id=case['id'], group='clean',
                   voice='tingting', file='audio/original.wav', modality='audio')
        original = dict(job, expected=case['expected'], pair=case['pair'], family=case['family'], tags=case['tags'])
        changes = {'case_id': cases[2]['id'], 'pair': 'false-pair', 'family': 'false-cluster', 'tags': []}
        for key, value in changes.items():
            with self.subTest(field=key), tempfile.TemporaryDirectory() as directory:
                output = Path(directory)
                (output/'predictions.jsonl').write_text(json.dumps(dict(original, **{key:value}))+'\n')
                with patch('banking_audio.verify_media', return_value={'audio':[]}), \
                        patch('banking_audio.jobs_for', return_value=[job]):
                    with self.assertRaisesRegex(ValueError, 'Recorded metadata differs'):
                        audit(ROOT, output)

    def test_dataset_has_balanced_business_contrasts_and_valid_labels(self):
        cases, task = validate_dataset(ROOT)
        self.assertEqual(len(cases), 128)
        pairs = {}
        for case in cases:
            if case['pair']:
                pairs.setdefault(case['pair'], []).append(case)
        self.assertEqual(len(pairs), 50)
        for pair in pairs.values():
            self.assertEqual({c['expected']['mode'] for c in pair}, {'navigate', 'inquire'})
            self.assertEqual(len({c['expected']['function'] for c in pair}), 1)

    def test_audio_carries_history_but_never_current_transcript_or_gold(self):
        cases, task = validate_dataset(ROOT)
        case = dict(cases[0], transcript='THIS IS A SECRET TRANSCRIPT',
                    history=[{'role': 'assistant', 'content': '是否进入取款？'}])
        with tempfile.TemporaryDirectory() as directory:
            wav = Path(directory)/'private-expected-navigate.wav'
            wav.write_bytes(b'RIFF example bytes')
            body = build_request(case, task, 'audio', wav)
        serialized = json.dumps(body, ensure_ascii=False)
        self.assertIn('是否进入取款', serialized)
        self.assertNotIn('THIS IS A SECRET TRANSCRIPT', serialized)
        self.assertNotIn('private-expected', serialized)
        self.assertNotIn('expected', body)
        self.assertEqual(set(body), {'model', 'state', 'files', 'questions'})
        self.assertTrue(body['files'][0].startswith('data:audio/wav;base64,'))

    def test_text_and_reversed_candidates_keep_question_semantics(self):
        cases, task = validate_dataset(ROOT)
        original = json.dumps(task, ensure_ascii=False)
        normal = build_request(cases[0], task, 'text')
        reverse = build_request(cases[0], task, 'text', reverse=True)
        self.assertEqual(normal['state'], cases[0]['transcript'])
        for key in task['questions']:
            self.assertEqual(list(reverse['questions'][key]['criteria']),
                             list(reversed(normal['questions'][key]['criteria'])))
        self.assertEqual(json.dumps(task, ensure_ascii=False), original)

    def test_errors_and_missing_function_stay_in_joint_denominator(self):
        def record(id, mode, function, predicted_mode, predicted_function, error=None):
            return dict(case_id=id, expected={'mode': mode, 'function': function},
                        predicted={'mode': predicted_mode, 'function': predicted_function},
                        error=error, latency_ms=10, pair=None)
        rows = [record('a', 'inquire', 'balance', 'navigate', 'balance'),
                record('b', 'navigate', 'balance', 'navigate', None, 'missing answer'),
                record('c', 'inquire', 'balance', None, None, 'timeout'),
                record('d', 'navigate', 'balance', 'navigate', 'balance')]
        result = summarize(rows)
        self.assertEqual(result['requests'], 4)
        self.assertEqual(result['joint_correct'], 1)
        self.assertEqual(result['request_errors'], 2)
        self.assertEqual(result['inquiry_to_navigation']['count'], 1)
        self.assertEqual(result['inquiry_to_navigation']['denominator'], 2)
        self.assertEqual(result['mode']['confusion']['inquire']['__error__'], 1)
        self.assertEqual(result['joint_accuracy'], .25)

    def test_pair_score_requires_both_members_and_all_voices_correct(self):
        rows = []
        for id, mode in [('a', 'navigate'), ('b', 'inquire')]:
            for voice in ['one', 'two']:
                rows.append(dict(case_id=id, pair='contrast', expected={'mode': mode, 'function': 'balance'},
                    predicted={'mode': 'navigate' if id == 'b' and voice == 'two' else mode,
                               'function': 'balance'}, error=None, latency_ms=10))
        result = summarize(rows)
        self.assertEqual(result['paired_mode_all_correct']['pairs'], 1)
        self.assertEqual(result['paired_mode_all_correct']['correct'], 0)
        self.assertEqual(result['unique_semantic_cases'], 2)


if __name__ == '__main__':
    unittest.main()
