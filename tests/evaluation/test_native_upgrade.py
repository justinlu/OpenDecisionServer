"""Exercise the actual upgrade gate with persisted native observations; no GPU needed."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent
RECORDED = ROOT/'results/2026-10-28-llama-upgrade-chinese'
AUDITOR = ROOT/'native_probe/audit_upgrade.py'


class NativeUpgradeGateTests(unittest.TestCase):
    def fixture(self, root, new_version):
        (root/'reference.json').symlink_to(RECORDED/'reference.json')
        for version, source in [('old', 'old'), ('new', new_version)]:
            target = root/version
            target.mkdir()
            for name in ['parity.jsonl', 'latency.jsonl']:
                (target/name).symlink_to(RECORDED/source/name)
            (target/'summary.json').write_text((RECORDED/source/'summary.json').read_text())

    def run_gate(self, root):
        return subprocess.run([sys.executable, str(AUDITOR), '--results', str(root), '--require-pass'],
                              capture_output=True, text=True)

    def test_recorded_numerical_regression_fails_strict_gate(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.fixture(root, 'new')
            result = self.run_gate(root)
            self.assertEqual(result.returncode, 1, result.stderr)
            report = json.loads((root/'audit.json').read_text())
            self.assertFalse(report['strict_upgrade_pass'])
            self.assertEqual(report['comparison']['winner_changes'], 1)
            self.assertIn('Strict migration gate failed', result.stderr)

    def test_equal_outputs_only_certify_the_default_path(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            # Both sides use the same real old observations: first prove that
            # this fixture otherwise satisfies the entire numerical gate.
            self.fixture(root, 'old')
            result = self.run_gate(root)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue(json.loads((root/'audit.json').read_text())['strict_upgrade_pass'])
            summary_path = root/'new/summary.json'
            summary = json.loads(summary_path.read_text())
            summary['full_hidden'] = True
            summary_path.write_text(json.dumps(summary))
            result = self.run_gate(root)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('Diagnostic full-hidden mode cannot certify', result.stderr)
            self.assertFalse((root/'audit.json').exists())


if __name__ == '__main__':
    unittest.main()
