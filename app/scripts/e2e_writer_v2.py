"""Owned native writer recovery, admission ACK loss, and hash boundary checks."""
import argparse
import hashlib
import json
import pathlib
import subprocess
import sys
import tempfile
import time
from e2e_core import ROOT, launch, terminate_owned_core

def digest(path): return hashlib.sha256(path.read_bytes()).hexdigest()

def only_stopped_receipts(output):
    frames=[json.loads(line) for line in output.splitlines() if line]
    return all(f['kind']=='runtime_stopped' and f['data']['sessionId']=='' and f['data']['nativeProcessId']>0 for f in frames)

def raw_host(state,core,endpoint,spec):
    process=subprocess.Popen([str(ROOT/'host/target/debug/neko-host.exe'),'--runtime',str(core),'--supervisor',str(ROOT/'scripts/core_supervisor.py'),'--state-root',str(state/'tasks'),'--fixture-endpoint',endpoint],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    process.stdin.write(json.dumps(spec)+'\n');process.stdin.flush()
    return process

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--commit',required=True);parser.add_argument('--output',type=pathlib.Path,required=True);args=parser.parse_args()
    assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()==args.commit
    assert not subprocess.check_output(['git','status','--porcelain'],cwd=ROOT,text=True).strip()
    fixture=subprocess.Popen([sys.executable,str(ROOT/'scripts/fixture_provider.py')],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True)
    service=None;host=None;checks=[]
    core=ROOT/'.build/core-windows/neko.exe';helper=core.with_name('neko-writer-lease.exe')
    try:
        endpoint=fixture.stdout.readline().strip();assert endpoint.startswith('READY ')
        with tempfile.TemporaryDirectory(prefix='neko-writer-v2-',ignore_cleanup_errors=True) as directory:
            scratch=pathlib.Path(directory);state=scratch/'state';config=scratch/'fixture.json'
            config.write_text(json.dumps({'hostExecutable':str(ROOT/'host/target/debug/neko-host.exe'),'coreExecutable':str(core),'supervisor':str(ROOT/'scripts/core_supervisor.py'),'fixtureEndpoint':endpoint[6:]}))
            service,client=launch(state,config)
            initial=client.send('[fixture:background] Retain native writer request after admission ACK loss.','background')
            live=client.wait(initial['requestId'],lambda w:bool(w.get('reply')))
            saved=live['core'];terminate_owned_core(saved['nativeProcessId'],core)
            before=client.wait(initial['requestId'],lambda w:w['status']=='recovery_required')
            spec={'requestId':initial['requestId'],'text':'Continue the existing request from its durable Core task. Use recorded results; do not repeat completed effects. Preserve denied or unknown outcomes.','botId':initial['botId'],'resume':saved}
            host=raw_host(state,core,endpoint[6:],spec)
            admitted=json.loads(host.stdout.readline());assert admitted['kind']=='admitted',admitted
            receipt=admitted['data'];assert receipt['task']['activationEpoch']==saved['task']['activationEpoch']+1
            assert client.work(initial['requestId'])['core']==saved,'Raw fixture must not acknowledge admission into App journal'
            contender=raw_host(state,core,endpoint[6:],spec)
            contender.wait(timeout=12);out=contender.stdout.read();err=contender.stderr.read();contender.stdin.close()
            assert contender.returncode!=0 and only_stopped_receipts(out) and 'core_writer_unavailable' in err,err
            checks.append({'name':'actual_live_native_writer_denies_second_Rust_host','pass':True})
            terminate_owned_core(receipt['nativeProcessId'],core)
            host.stdin.close();host.wait(timeout=12);host=None
            checks.append({'name':'actual_Core_checkpoint_persisted_before_App_admission_ACK','pass':True,'savedEpoch':saved['task']['activationEpoch'],'unacknowledgedEpoch':receipt['task']['activationEpoch']})
            client.request('/v1/recover',{'requestId':initial['requestId']})
            recovered=client.wait(initial['requestId'],lambda w:w['status']=='completed')
            current=recovered['core'];assert current['sessionId']==saved['sessionId'] and current['task']['id']==saved['task']['id']
            assert current['task']['activationEpoch']==saved['task']['activationEpoch']+2
            assert current['execution']['authorityId']==saved['execution']['authorityId']
            assert current['taskRecovery']=={'version':1,'kind':'prior_receipt_retry'}
            assert recovered.get('contextPin')==live.get('contextPin') and recovered['publication']==live['publication'] and not recovered.get('artifacts')
            checks.append({'name':'lost_admission_ACK_recovers_same_task_with_pinned_prior_receipt_proof','pass':True,'recoveredEpoch':current['task']['activationEpoch']})
            service.kill();service.wait(timeout=5);service=None;time.sleep(1)
            for attempt in range(3):
                service,client=launch(state,config);assert client.work(initial['requestId'])['core']==current
                service.kill();service.wait(timeout=5);service=None
            checks.append({'name':'actual_recovery_receipt_survives_three_App_restarts','pass':True})
            lock=state/'tasks'/initial['requestId']/'.core-home/.neko-core/task-sessions'/(current['sessionId']+'.lock')
            released=lock.read_bytes();assert json.loads(released)['state']=='released','Completed writer must release ownership handle and retain a released marker'
            checks.append({'name':'completed_writer_retains_released_marker','pass':True})
            for name,content in [('empty',b''),('unknown',b'{"version":99}'),('v1',b'{"version":1,"pid":0}')]:
                lock.write_bytes(content);before_hash=digest(lock)
                invalid=raw_host(state,core,endpoint[6:],{**spec,'resume':current})
                invalid.wait(timeout=12);out=invalid.stdout.read();err=invalid.stderr.read();invalid.stdin.close()
                assert invalid.returncode!=0 and only_stopped_receipts(out) and 'core_writer_unavailable' in err,err
                assert digest(lock)==before_hash,'Invalid marker changed during denied resume'
                lock.write_bytes(released)
                checks.append({'name':'synthetic_'+name+'_marker_denied_unchanged_no_migration','pass':True})
            for name,helper_content in [('missing',None),('tampered',b'fixture-only invalid executable')]:
                runtime=scratch/name;runtime.mkdir();copy=runtime/'neko.exe';copy.write_bytes(core.read_bytes())
                if helper_content is not None:(runtime/helper.name).write_bytes(helper_content)
                process=raw_host(state,copy,endpoint[6:],spec)
                out,err=process.communicate(timeout=10);assert process.returncode!=0 and not out
                expected='writer_helper_missing' if helper_content is None else 'writer_helper_digest_mismatch'
                assert expected in err,err
                checks.append({'name':'helper_'+name+'_denied_before_Core_launch','pass':True})
            result={'commit':args.commit,'workingTree':False,'scope':'Actual native pinned Go/Rust/Core/helper with loopback fixture; no external provider or grants; ACK is intentionally withheld by an owned raw Rust host.', 'coreSha256':digest(core),'helperSha256':digest(helper),'hostSha256':digest(ROOT/'host/target/debug/neko-host.exe'),'goSha256':digest(ROOT/'.build/neko-aurora.exe'),'checks':checks}
            args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(result,indent=2),encoding='utf-8');print(json.dumps(result,indent=2))
    finally:
        if host and host.poll() is None:host.stdin.close();host.kill();host.wait(timeout=5)
        if service and service.poll() is None:service.kill();service.wait(timeout=5)
        fixture.stdin.close()
        try:fixture.wait(timeout=5)
        except subprocess.TimeoutExpired:fixture.kill();fixture.wait(timeout=5)

if __name__=='__main__':main()
