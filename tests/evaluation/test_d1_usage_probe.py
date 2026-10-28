import unittest

from banking_audio import ROOT,validate_dataset
from d1_usage_probe import protocol,body_for,selected_reference


class OfficialUsageProbeTests(unittest.TestCase):
    def test_reference_is_a_fixed_subset_of_identical_native_jobs(self):
        jobs=protocol()['jobs']
        self.assertEqual(len(jobs),264)
        self.assertEqual(len({j['id'] for j in jobs}),264)
        reference=[j for j in jobs if selected_reference(j)]
        self.assertEqual(len(reference),132)
        for job in reference:
            self.assertIn(job['id'],{j['id'] for j in jobs})
            if job['question']['type']=='choice':self.assertIn(job['expected'],job['question']['criteria'])
            else:self.assertIn(job['expected'],['true','false'])

    def test_audio_variants_do_not_add_current_transcript(self):
        cases,_=validate_dataset(ROOT);lookup={c['id']:c for c in cases}
        for job in protocol()['jobs']:
            if job['modality']!='audio':continue
            body=body_for(job)
            self.assertNotIn(lookup[job['case_id']]['transcript'],body['state'] or '')
            self.assertNotIn('expected',body)
            self.assertEqual(set(body['questions']),{'decision'})
            self.assertTrue(body['files'][0].startswith('data:audio/wav;base64,'))


if __name__=='__main__':unittest.main()
