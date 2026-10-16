"""Versioned local task controls through source-built Go/Rust and pinned native Core."""
import argparse
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import time
import urllib.error
from e2e_core import ROOT, Client, launch
from e2e_permissions import OwnedCoreHandle

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--commit',required=True);parser.add_argument('--go',type=pathlib.Path,required=True);parser.add_argument('--output',type=pathlib.Path,required=True);args=parser.parse_args()
    head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
    assert head==args.commit and not subprocess.check_output(['git','status','--porcelain'],cwd=ROOT,text=True).strip()
    build_metadata=subprocess.check_output([str(args.go),'version','-m',str(ROOT/'.build/neko-control.exe')],text=True)
    assert 'vcs.revision='+head in build_metadata and 'vcs.modified=false' in build_metadata, 'Go binary was not built from this clean commit'
    scratch=pathlib.Path(tempfile.mkdtemp(prefix='native-task-controls-',dir=ROOT/'.build'))
    checks=[];process=None;fixture=None;handles=[]
    report={'commit':head,'workingTree':False,'goBuildMetadata':build_metadata,'stateRoot':str(scratch),'scope':'Actual local Go/Rust/native Core with owned loopback fixture. Queued pause/resume and linked obligation controls; launched semantic pause is unsupported. No provider, cloud, phone or project-wide pause acceptance.'}
    def control(client,w,action,nonce,text=None):
        body={'version':1,'requestId':w['requestId'],'expectedEpoch':w['epoch'],'expectedRevision':w.get('taskRevision',0),'action':action,'clientNonce':nonce}
        if text is not None:body['text']=text
        return client.request('/v1/task-controls',body),body
    try:
        state=scratch/'state'
        process=subprocess.Popen([str(ROOT/'.build/neko-control.exe'),'--listen','127.0.0.1:0','--state',str(state)],stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
        ready=process.stdout.readline().strip();assert ready.startswith('READY ');client=Client(ready[6:])
        w=client.send('[fixture:foreground] Queued pause and resume fixture.','background',nonce='native-controls-queued')
        paused,pausebody=control(client,w,'pause','native-controls-pause')
        paused=paused['work'];assert paused['status']=='paused' and paused['desiredState']=='paused' and paused['contextPin']==w['contextPin']
        process.kill();process.wait(timeout=5)
        env={key:os.environ[key] for key in ('SystemRoot','WINDIR','TEMP','TMP','USERPROFILE') if key in os.environ}
        fixture=subprocess.Popen([sys.executable,str(ROOT/'scripts/fixture_provider.py')],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True,env=env)
        endpoint=fixture.stdout.readline().strip();assert endpoint.startswith('READY ')
        config=scratch/'fixture.json';config.write_text(json.dumps({'hostExecutable':str(ROOT/'host/target/debug/neko-host.exe'),'coreExecutable':str(ROOT/'.build/core-windows/neko.exe'),'supervisor':str(ROOT/'scripts/core_supervisor.py'),'fixtureEndpoint':endpoint[6:]}))
        process,client=launch(state,config);time.sleep(.5)
        assert client.work(w['requestId'])==paused
        duplicate=client.request('/v1/task-controls',pausebody);assert duplicate['duplicate'] and duplicate['work']==paused
        resumed,_=control(client,paused,'resume','native-controls-resume');assert resumed['work']['contextPin']==w['contextPin']
        done=client.wait(w['requestId'],lambda x:x['status']=='completed');assert done['requestId']==w['requestId'] and done['core']['task']['activationEpoch']==1
        checks.append({'name':'queued_pause_restart_no_dispatch_resume_same_request_pin_then_native_completion','pass':True})

        old=client.send('[fixture:background] Keep a report running for control tests.','background',nonce='native-controls-running')
        old=client.wait(old['requestId'],lambda x:bool(x.get('reply')));handle=OwnedCoreHandle(old['core']['nativeProcessId']);handles.append(handle)
        try:
            control(client,old,'pause','native-controls-unsupported');raise AssertionError('Launched pause relabelled cancellation')
        except urllib.error.HTTPError as error:assert error.code==501
        after=client.work(old['requestId']);assert after['status']=='running' and after['epoch']==old['epoch'] and after['core']==old['core']
        checks.append({'name':'unnegotiated_launched_pause_returns501_without_cancelling','pass':True})
        supplement,body=control(client,after,'supplement','native-controls-supplement','[fixture:foreground] Add compatible source notes.')
        assert supplement['work']['status']=='running' and supplement['work']['epoch']==old['epoch'] and supplement['work']['contextPin']==old['contextPin']
        new=supplement['related'];assert new['taskLink']['parentRequestId']==old['requestId'] and new['taskLink']['parentMessageId']==old['sourceMessageId']
        duplicate=client.request('/v1/task-controls',body);assert duplicate['duplicate'] and duplicate['related']['requestId']==new['requestId']
        interjection=client.send('[fixture:foreground] Explain a term while the old report continues.');client.wait(interjection['requestId'],lambda x:x['status']=='completed')
        assert client.work(old['requestId'])['status']=='running'
        checks.append({'name':'linked_supplement_receipt_and_foreground_interjection_preserve_active_run','pass':True})

        old=client.work(old['requestId']);replacement,body=control(client,old,'replace','native-controls-replace','[fixture:foreground] Make a document instead of slides.')
        successor=replacement['related'];assert successor['status']=='waiting_control_stop' and replacement['work']['desiredState']=='cancelled'
        parent=client.wait(old['requestId'],lambda x:x['status']=='cancelled' and x.get('runtimeStopped'))
        handle.assert_stopped();assert parent['runtimeStopReceipt']['nativeProcessId']==old['core']['nativeProcessId']
        final=client.wait(successor['requestId'],lambda x:x['status']=='completed')
        assert final['core']['sessionId']!=old['core']['sessionId'] and final['core']['task']['id']!=old['core']['task']['id']
        assert final['taskLink']['parentPinSha256']==old['contextPin']['sha256'] and parent['contextPin']==old['contextPin']
        reply=parent['reply'];time.sleep(.2);assert client.work(old['requestId'])['reply']==reply
        checks.append({'name':'replacement_waits_actual_native_process_exit_then_new_Core_task_late_old_reply_fenced','pass':True})
        try:
            control(client,old,'supplement','native-controls-offline-old','Offline extra condition');raise AssertionError('Offline stale command revived cancelled work')
        except urllib.error.HTTPError as error:assert error.code==409
        process.kill();process.wait(timeout=5);process,client=launch(state,config)
        duplicate=client.request('/v1/task-controls',body);assert duplicate['duplicate'] and duplicate['related']['requestId']==final['requestId'] and duplicate['related']['status']=='completed'
        assert client.work(old['requestId'])['reply']==reply
        checks.append({'name':'restart_idempotent_replacement_and_offline_cancelled_parent_rejection','pass':True})
        report['status']='PASS'
    except Exception as error:
        report['status']='FAIL';report['errorType']=type(error).__name__;report['error']=str(error);raise
    finally:
        if process and process.poll() is None:process.kill();process.wait(timeout=5)
        if fixture:
            fixture.stdin.close()
            try:fixture.wait(timeout=5)
            except subprocess.TimeoutExpired:fixture.terminate();fixture.wait(timeout=5)
        for handle in handles:handle.close()
        report['checks']=checks
        for name,path in {'goBinarySha256':ROOT/'.build/neko-control.exe','rustHostSha256':ROOT/'host/target/debug/neko-host.exe','coreSha256':ROOT/'.build/core-windows/neko.exe'}.items():report[name]=hashlib.sha256(path.read_bytes()).hexdigest()
        args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
        print(json.dumps(report,ensure_ascii=False,indent=2))

if __name__=='__main__':main()
