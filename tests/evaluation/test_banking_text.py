import unittest
from banking_audio import ROOT,validate_dataset
from banking_text import dataset,body

class BankingTextTests(unittest.TestCase):
    def test_exact_authored_semantics_once_without_asr(self):
        rows=dataset()
        self.assertEqual(len(rows),158)
        self.assertEqual(len({r['id'] for r in rows}),158)
        self.assertEqual(sum('core' in r['case']['tags'] for r in rows),130)
        original,_=validate_dataset(ROOT)
        self.assertEqual([r['case'] for r in rows if r['origin']=='legacy'],original)
        for row in rows:
            payload=body(row,row['full_questions']['mode'],'d1-2b')
            self.assertNotIn('files',payload)
            self.assertEqual(payload['model'],'d1-2b')
            self.assertTrue(payload['state'].endswith(row['case']['transcript']))
            self.assertNotIn('expected',payload)

if __name__=='__main__':unittest.main()
