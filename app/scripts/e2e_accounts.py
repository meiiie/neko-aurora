"""Account lease gates with owned native Core and synthetic credentials only."""
import argparse, hashlib, json, pathlib, subprocess, sys, tempfile, uuid, urllib.error
from e2e_core import ROOT, launch
from e2e_permissions import OwnedCoreHandle

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--commit',required=True);parser.add_argument('--output',type=pathlib.Path,required=True);args=parser.parse_args()
    head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
    assert head==args.commit and not subprocess.check_output(['git','status','--porcelain'],cwd=ROOT)
    fixture=subprocess.Popen([sys.executable,str(ROOT/'scripts/fixture_provider.py')],stdin=subprocess.PIPE,stdout=subprocess.PIPE,text=True)
    process=None;handles=[];checks=[]
    try:
        ready=fixture.stdout.readline().strip();assert ready.startswith('READY ')
        with tempfile.TemporaryDirectory(prefix='neko-account-authority-',ignore_cleanup_errors=True) as directory:
            state=pathlib.Path(directory)/'state';config=pathlib.Path(directory)/'fixture.json'
            config.write_text(json.dumps({'hostExecutable':str(ROOT/'host/target/debug/neko-host.exe'),'coreExecutable':str(ROOT/'.build/core-windows/neko.exe'),'supervisor':str(ROOT/'scripts/core_supervisor.py'),'fixtureEndpoint':ready[6:]}))
            process,client=launch(state,config)
            account_input={'clientNonce':'native-account-create','name':'Owned fixture card','provider':'openai-compatible','endpoint':'https://example.invalid/v1','model':'fixture-model','secret':'fixture-only-account-canary'}
            account=client.request('/v1/accounts',account_input)['account']
            assert account['status']=='stored_unverified' and account['credentialGeneration']==1
            blocker=client.send('[fixture:background] Keep the foreground lane occupied.');client.wait(blocker['requestId'],lambda w:bool(w.get('reply')))
            queued_body={'botId':'bot-meiiie','clientNonce':'native-account-queued','text':'[fixture:artifact] Queued file obligation.','lane':'foreground','accountId':account['id'],'accountRevision':account['revision']}
            queued=client.request('/v1/messages',queued_body)['work']
            client.request('/v1/accounts/remove',{'id':account['id'],'expectedRevision':account['revision']})
            stopped=client.work(queued['requestId']);assert stopped['status']=='suspended' and not stopped.get('attemptId') and not stopped.get('core')
            checks.append({'name':'queued_selected_account_removed_before_native_dispatch','pass':True})
            assert client.request('/v1/messages',queued_body)['work']['requestId']==queued['requestId']
            try:client.request('/v1/accounts',account_input);raise AssertionError('Removed nonce recreated account')
            except urllib.error.HTTPError as e:assert e.code==409
            checks.append({'name':'lost_message_ACK_keeps_identity_removed_account_nonce_never_recreates','pass':True})
            account_input['clientNonce']='native-account-create-second';account=client.request('/v1/accounts',account_input)['account']
            active_body={'botId':'bot-davin','clientNonce':'native-account-active','text':'[fixture:background] [fixture:artifact] Active file obligation.','lane':'background','accountId':account['id'],'accountRevision':account['revision']}
            active=client.request('/v1/messages',active_body)['work'];running=client.wait(active['requestId'],lambda w:bool(w.get('reply')))
            lease=running['accountLease'];assert lease['attemptId']==running['attemptId'] and lease['audience']==running['conversationId'] and lease['fence']==1
            handle=OwnedCoreHandle(running['core']['nativeProcessId']);handles.append(handle)
            update=dict(account_input,id=account['id'],expectedRevision=account['revision'],clientNonce='native-account-rotate',secret='fixture-only-rotated-canary')
            current=client.request('/v1/accounts',update)['account'];assert current['credentialGeneration']==2
            suspended=client.wait(active['requestId'],lambda w:w['status']=='suspended');handle.assert_stopped()
            assert suspended['runtimeStopped'] and suspended['core']==running['core'] and not suspended.get('operations') and not suspended.get('artifacts')
            checks.append({'name':'rotation_fences_live_native_Core_and_waits_for_exit_before_suspended','pass':True})
            client.wait(blocker['requestId'],lambda w:w['status']=='completed',timeout=40)
            fresh=dict(active_body,clientNonce='native-account-fresh',text='[fixture:artifact] Create an explicit fresh local report.',accountRevision=current['revision'])
            fresh_work=client.request('/v1/messages',fresh)['work'];done=client.wait(fresh_work['requestId'],lambda w:w['status']=='completed')
            assert done['accountPin']['credentialGeneration']==2 and done['artifacts']
            data,headers=client.request('/v1/artifacts/'+done['artifacts'][0]['id']);assert hashlib.sha256(data).hexdigest()==headers['X-Artifact-SHA256']
            checks.append({'name':'explicit_current_account_revision_completes_requested_file_unrelated_lane_continues','pass':True})
            journal=(state/'transactions.jsonl').read_text(encoding='utf-8');assert 'fixture-only-account-canary' not in journal and 'fixture-only-rotated-canary' not in journal and 'secretRef' not in journal
            checks.append({'name':'native_DPAPI_account_credentials_excluded_from_task_journal','pass':True})
            for _ in range(3):
                process.kill();process.wait(timeout=5);process,client=launch(state,config)
                assert client.work(active['requestId'])==suspended
            checks.append({'name':'restart_three_times_preserves_suspended_account_pin_and_receipt','pass':True})
        report={'commit':head,'scope':'Actual native Windows Go/Rust/Core; owned loopback fixture and synthetic sealed keys only. Saved keys are not provider-authenticated or consumed by Core. No third-party grants, cloud, UI or phone acceptance.','checks':checks}
        for name,path in [('go',ROOT/'.build/neko-aurora.exe'),('host',ROOT/'host/target/debug/neko-host.exe'),('core',ROOT/'.build/core-windows/neko.exe')]:report[name+'Sha256']=hashlib.sha256(path.read_bytes()).hexdigest()
        args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8');print(json.dumps(report,indent=2))
    finally:
        for handle in handles:handle.close()
        if process and process.poll() is None:process.kill();process.wait(timeout=5)
        fixture.stdin.close()
        try:fixture.wait(timeout=5)
        except subprocess.TimeoutExpired:fixture.kill();fixture.wait(timeout=5)

if __name__=='__main__':main()
