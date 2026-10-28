import importlib.util
import json
from pathlib import Path
import plistlib
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('local_d1_deploy',Path(__file__).resolve().parents[2]/'scripts/deploy-local-d1.py')
deploy=importlib.util.module_from_spec(spec);spec.loader.exec_module(deploy)

class LocalD1DeploymentTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();root=Path(self.temp.name)
        self.base=root/'decision';self.agents=root/'agents';self.agents.mkdir()
        self.release=self.base/'releases'/'release';(self.release/'rollback').mkdir(parents=True)
        (self.release/'bin').mkdir();binary=self.release/'bin/ods-metal';binary.write_bytes(b'binary')
        (self.release/'release.json').write_text(json.dumps(dict(binary_sha256=deploy.digest(binary))))
        for label in deploy.LABELS:
            data=plistlib.dumps(dict(Label=label))
            (self.agents/(label+'.plist')).write_bytes(data)
            (self.release/'rollback'/(label+'.plist')).write_bytes(data)
        (self.release/'rollback/state.json').write_text(json.dumps({'8080':['julia-1'],'18083':['chinese-decision-2b']}))
        self.base_patch=patch.object(deploy,'BASE',self.base);self.base_patch.start()
        self.agents_patch=patch.object(deploy,'AGENTS',self.agents);self.agents_patch.start()
    def tearDown(self):self.agents_patch.stop();self.base_patch.stop();self.temp.cleanup()
    def test_interruption_after_activation_begins_restores_old_jobs(self):
        with patch.object(deploy,'command',side_effect=KeyboardInterrupt),patch.object(deploy,'restore') as restore:
            with self.assertRaises(KeyboardInterrupt):deploy.activate(self.release)
            restore.assert_called_once_with(self.release)
    def test_restore_attempts_second_job_after_first_bootstrap_failure(self):
        calls=[]
        def command(*args,**kwargs):
            calls.append(args)
            if 'bootstrap' in args and str(args[-1]).endswith(deploy.LABELS[0]+'.plist'):raise RuntimeError('first job failed')
            return SimpleNamespace(stdout='',returncode=0)
        with patch.object(deploy,'command',side_effect=command),patch.object(deploy,'wait_original',return_value={'recovered':True},create=True):
            deploy.restore(self.release)
        self.assertTrue(any('bootstrap' in args and deploy.LABELS[1] in str(args) for args in calls))
        self.assertTrue(json.loads((self.release/'rollback-report.json').read_text())['command_errors'])


class LaunchdDrainTests(unittest.TestCase):
    def test_bootout_waits_for_old_process_to_exit(self):
        from types import SimpleNamespace
        def command(*args,**kwargs):return SimpleNamespace(stdout='\n pid = 42\n',returncode=0)
        with patch.object(deploy,'command',side_effect=command),patch.object(deploy.os,'kill',side_effect=[None,None,ProcessLookupError]) as alive,patch.object(deploy.time,'sleep'):
            deploy.stop_job('app.example')
            self.assertEqual(alive.call_count,3)

class InterruptedDrainTests(unittest.TestCase):
    def test_recovery_remembers_pid_after_job_is_unregistered(self):
        present=SimpleNamespace(stdout='\n pid = 43\n',returncode=0)
        absent=SimpleNamespace(stdout='',returncode=3)
        with patch.object(deploy,'command',return_value=present),patch.object(deploy.os,'kill',side_effect=KeyboardInterrupt):
            with self.assertRaises(KeyboardInterrupt):deploy.stop_job('app.interrupted')
        self.assertEqual(deploy.DRAINING['app.interrupted'],43)
        with patch.object(deploy,'command',return_value=absent),patch.object(deploy.os,'kill',side_effect=[None,ProcessLookupError]) as alive,patch.object(deploy.time,'sleep'):
            deploy.stop_job('app.interrupted')
            self.assertEqual(alive.call_count,2)
        self.assertNotIn('app.interrupted',deploy.DRAINING)

if __name__=='__main__':unittest.main()
