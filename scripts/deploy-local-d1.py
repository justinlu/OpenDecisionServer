#!/usr/bin/env python3
"""Stage and activate one self-contained d1 ODS release on this Mac, with rollback."""
import argparse
from datetime import datetime,timezone
import hashlib
import json
import os
from pathlib import Path
import plistlib
import shutil
import signal
import subprocess
import time
import urllib.request

ROOT=Path(__file__).resolve().parents[1]
HOME_DIR=Path.home()
AGENTS=HOME_DIR/'Library/LaunchAgents'
BASE=HOME_DIR/'Library/Application Support/VoiceAI/decision'
LABELS=['app.powermobile.voiceai.decision','app.powermobile.voiceai.decision-chinese']
DOMAIN=f'gui/{os.getuid()}'


def command(*args,check=True):return subprocess.run(args,check=check,capture_output=True,text=True)
def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda:stream.read(8*1024*1024),b''):h.update(block)
    return h.hexdigest()

def write_plist(path,data):
    temp=path.with_suffix('.plist.tmp');temp.write_bytes(plistlib.dumps(data));temp.chmod(0o644);os.replace(temp,path)

def stage():
    if command('git','-C',str(ROOT),'status','--porcelain').stdout.strip():raise RuntimeError('Commit the release sources before staging')
    commit=command('git','-C',str(ROOT),'rev-parse','HEAD').stdout.strip()
    build_info=command('go','version','-m',str(ROOT/'bin/ods-metal')).stdout
    assert 'vcs.revision='+commit in build_info and 'vcs.modified=false' in build_info,'Rebuild the binary from this clean commit'
    release=BASE/'releases'/(datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-'+commit[:8]);release.mkdir(parents=True,exist_ok=False)
    (release/'bin').mkdir();shutil.copy2(ROOT/'bin/ods-metal',release/'bin/ods-metal')
    shutil.copytree(ROOT/'lib/darwin-arm64/d1',release/'native')
    (release/'models').mkdir();shutil.copy2(ROOT/'dist/liquid-d1/models/d1-2B-Q8_0.gguf',release/'models/d1-2B-Q8_0.gguf')
    shutil.copy2(ROOT/'dist/liquid-d1/models/d1-2b-LICENSE',release/'models/LICENSE-d1')
    model=json.loads((ROOT/'tests/evaluation/liquid-d1-models.json').read_text())['models'][1]
    assert model['id']=='d1-2b'
    assert digest(release/'models/d1-2B-Q8_0.gguf')==model['files'][0]['sha256']
    backup=release/'rollback';backup.mkdir()
    disabled=command('launchctl','print-disabled',DOMAIN).stdout
    for label in LABELS:
        path=AGENTS/(label+'.plist');shutil.copy2(path,backup/path.name)
        assert command('launchctl','print',DOMAIN+'/'+label,check=False).returncode==0,'Expected existing managed ODS job: '+label
        assert f'"{label}" => true' not in disabled,'Existing job is disabled: '+label
    state={str(port):[m['name'] for m in json.load(urllib.request.urlopen(f'http://127.0.0.1:{port}/v1/models',timeout=3))['models']] for port in [8080,18083]}
    (backup/'state.json').write_text(json.dumps(state,indent=2)+'\n')
    plist=dict(plistlib.loads((AGENTS/(LABELS[0]+'.plist')).read_bytes()))
    plist.update(ProgramArguments=[str(release/'bin/ods-metal'),'-http','127.0.0.1:8080,127.0.0.1:18083','-llama-model',str(release/'models/d1-2B-Q8_0.gguf'),'-llama-lib',str(release/'native/lib/libods_d1.dylib')],WorkingDirectory=str(release))
    (release/'launchd.plist').write_bytes(plistlib.dumps(plist))
    manifest=dict(commit=commit,binary_sha256=digest(release/'bin/ods-metal'),build_info=build_info,model=model,source_root=str(ROOT),ports=[8080,18083],label=LABELS[0])
    (release/'release.json').write_text(json.dumps(manifest,indent=2)+'\n')
    print(release)


def restore(release):
    assert release.parent==BASE/'releases' and (release/'rollback').is_dir()
    errors=[]
    def attempt(action):
        try:action()
        except Exception as error:errors.append(str(error))
    for label in LABELS:attempt(lambda:command('launchctl','bootout',DOMAIN+'/'+label,check=False))
    for label in LABELS:
        path=AGENTS/(label+'.plist')
        attempt(lambda:write_plist(path,plistlib.loads((release/'rollback'/path.name).read_bytes())))
        attempt(lambda:command('launchctl','enable',DOMAIN+'/'+label))
        attempt(lambda:command('launchctl','bootstrap',DOMAIN,str(path)))
    expected=json.loads((release/'rollback/state.json').read_text())
    try:report=wait_original(expected)
    except Exception as error:report=dict(recovered=False,verification_error=str(error))
    report['command_errors']=errors
    (release/'rollback-report.json').write_text(json.dumps(report,indent=2)+'\n')
    if not report['recovered']:raise RuntimeError('Rollback did not restore both original services: '+json.dumps(report))


def wait_original(expected):
    last=None
    for _ in range(90):
        try:
            observed={}
            for port,names in expected.items():
                root=f'http://127.0.0.1:{port}'
                assert json.load(urllib.request.urlopen(root+'/health/ready',timeout=2))['status']=='ready'
                actual=[m['name'] for m in json.load(urllib.request.urlopen(root+'/v1/models',timeout=2))['models']]
                assert actual==names,(port,actual,names)
                observed[port]=actual
            return dict(recovered=True,models=observed)
        except Exception as error:last=error;time.sleep(1)
    raise RuntimeError(str(last))


def protected_restore(release):
    handlers={sig:signal.getsignal(sig) for sig in [signal.SIGINT,signal.SIGTERM]}
    try:
        for sig in handlers:signal.signal(sig,signal.SIG_IGN)
        return restore(release)
    finally:
        for sig,handler in handlers.items():signal.signal(sig,handler)


def smoke():
    info={}
    for port in [8080,18083]:
        root=f'http://127.0.0.1:{port}'
        assert json.load(urllib.request.urlopen(root+'/health/ready',timeout=3))['status']=='ready'
        models=json.load(urllib.request.urlopen(root+'/v1/models',timeout=3))
        assert [m['name'] for m in models['models']]==['d1-2b']
        body=dict(state='我要取现金',questions={'business':dict(type='choice',instructions='用户要办理什么业务？',criteria={'withdraw':'取款','deposit':'存款','transfer':'转账'})})
        req=urllib.request.Request(root+'/v1/systemone',data=json.dumps(body,ensure_ascii=False).encode(),headers={'Content-Type':'application/json'})
        response=json.load(urllib.request.urlopen(req,timeout=60));assert response['model']=='d1-2b' and response['answers']['business']['choice']=='withdraw'
        pid=command('lsof','-t','-nP',f'-iTCP:{port}','-sTCP:LISTEN').stdout.strip()
        info[str(port)]=dict(pid=int(pid),models=models,response=response)
    assert info['8080']['pid']==info['18083']['pid'],'Ports must share one ODS process'
    return info


def wait_ready(previous_pid=None):
    error=None
    for _ in range(90):
        try:
            result=smoke()
            if result['8080']['pid']==previous_pid:raise RuntimeError('Previous process is still draining')
            return result
        except Exception as e:error=e;time.sleep(1)
    raise RuntimeError('New release did not pass readiness: '+str(error))


def activate(release):
    assert release.parent==BASE/'releases' and (release/'rollback').is_dir()
    manifest=json.loads((release/'release.json').read_text())
    assert digest(release/'bin/ods-metal')==manifest['binary_sha256']
    for label in LABELS:
        name=label+'.plist'
        assert (AGENTS/name).read_bytes()==(release/'rollback'/name).read_bytes(),'Managed configuration changed after staging'
    try:
        for label in LABELS:command('launchctl','bootout',DOMAIN+'/'+label)
        command('launchctl','disable',DOMAIN+'/'+LABELS[1])
        target=AGENTS/(LABELS[0]+'.plist');write_plist(target,plistlib.loads((release/'launchd.plist').read_bytes()))
        command('launchctl','enable',DOMAIN+'/'+LABELS[0]);command('launchctl','bootstrap',DOMAIN,str(target))
        first=wait_ready()
        # Verify a managed restart from the self-contained release, not the checkout.
        command('launchctl','bootout',DOMAIN+'/'+LABELS[0]);command('launchctl','bootstrap',DOMAIN,str(target))
        second=wait_ready(first['8080']['pid'])
        libraries=command('lsof','-p',str(second['8080']['pid'])).stdout
        assert str(release/'native/lib/libods_d1.dylib') in libraries
        assert 'libonnxruntime' not in libraries
        report=dict(release=str(release),commit=manifest['commit'],before_restart=first,after_restart=second,
            native_libraries=[line for line in libraries.splitlines() if str(release/'native/lib') in line])
        (release/'activation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
        print(json.dumps(report,ensure_ascii=False))
    except BaseException:
        protected_restore(release)
        raise


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('command',choices=['stage','activate','rollback']);p.add_argument('--release',type=Path);a=p.parse_args()
    if a.command=='stage':stage()
    elif a.command=='activate':
        # Convert a managed termination into the same rollback path as Ctrl-C.
        def interrupted(signum,frame):raise KeyboardInterrupt('Deployment interrupted')
        signal.signal(signal.SIGTERM,interrupted)
        activate(a.release.resolve())
    else:protected_restore(a.release.resolve())

if __name__=='__main__':main()
