import http.client
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from liquid_d1_media import main, request_for


class MediaTests(unittest.TestCase):
    def test_audio_requests_do_not_leak_transcripts_or_labels(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root/'clip.wav').write_bytes(b'example audio')
            case = dict(audio='clip.wav', transcript='Secret transcript', expected={'q': 'yes'},
                        request={'state': None, 'questions': {'q': {'type': 'noul', 'instructions': 'Speech?'}}})
            request = request_for(case, root)
            self.assertIsNone(request['state'])
            self.assertEqual(len(request['files']), 1)
            self.assertNotIn('Secret transcript', json.dumps(request))
            self.assertNotIn('expected', request)

    def test_truncated_media_http_response_is_recorded_and_later_cases_continue(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)/'results'
            argv = ['media.py', '--url', 'http://localhost/v1/systemone', '--output', str(output), '--image-only']
            # Two repetitions prove that an incomplete first response does not abort the loop.
            argv.extend(['--repeats', '2'])
            with patch('sys.argv', argv), patch('liquid_d1_media.request_for', return_value={'state': None, 'questions': {}}), \
                    patch('urllib.request.urlopen', side_effect=http.client.IncompleteRead(b'partial', 9)):
                try:
                    main()
                except SystemExit as error:
                    self.assertEqual(error.code, 1)
                except http.client.HTTPException as error:
                    self.fail('Truncated response aborted evaluation: '+str(error))
            records = [json.loads(line) for line in (output/'predictions.jsonl').read_text().splitlines()]
            self.assertEqual(len(records), 2)
            self.assertTrue(all(record['error'] for record in records))
            self.assertTrue(all(record['correct'] == 0 for record in records))
            self.assertEqual(json.loads((output/'summary.json').read_text())['image']['questions'], 6)


if __name__ == '__main__':
    unittest.main()
