"""Verify frozen context actually delivered through the native Core provider path."""
import argparse,hashlib,json,pathlib,subprocess,sys,tempfile,time
from e2e_core import ROOT,launch

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--commit');parser.add_argument('--output',type=pathlib.Path);args=parser.parse_args()
    head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
    if args.commit:assert head==args.commit and not subprocess.check_output(['git','status','--porcelain'],cwd=ROOT,text=True).strip()
    fixture=subprocess.Popen([sys.executable,str(ROOT/'scripts/fixture_provider.py')],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True)
    process=None;checks=[]
    try:
        ready=fixture.stdout.readline().strip();assert ready.startswith('READY ')
        with tempfile.TemporaryDirectory(prefix='neko-context-e2e-',ignore_cleanup_errors=True) as directory:
            state=pathlib.Path(directory)/'state';config=pathlib.Path(directory)/'fixture.json'
            config.write_text(json.dumps({'hostExecutable':str(ROOT/'host/target/debug/neko-host.exe'),'coreExecutable':str(ROOT/'.build/core-windows/neko.exe'),'supervisor':str(ROOT/'scripts/core_supervisor.py'),'fixtureEndpoint':ready[6:]}))
            process,client=launch(state,config)
            first=client.send('[fixture:background] Keep this foreground slot busy.','foreground')
            client.wait(first['requestId'],lambda w:bool(w.get('reply')))
            queued=client.send('[fixture:context-inspect] Inspect only the pin accepted now.','foreground')
            pin=queued['contextPin'];assert pin['sources']==[]
            done=client.wait(queued['requestId'],lambda w:w['status']=='completed',timeout=60)
            queued_reply=done['reply']
            assert done['contextPin']==pin and 'PIN_SHA='+pin['sha256'] in done['reply'] and 'PIN_SOURCES=[]' in done['reply']
            assert client.work(first['requestId'])['status']=='completed'
            checks.append({'name':'actual_Core_queued_prompt_excludes_late_completed_source','pass':True})
            latest=client.send('[fixture:context-inspect] Inspect the newer pin.','foreground')
            done=client.wait(latest['requestId'],lambda w:w['status']=='completed')
            assert 'PIN_SHA='+latest['contextPin']['sha256'] in done['reply'] and first['requestId'] in done['reply']
            checks.append({'name':'actual_Core_receives_retained_source_provenance_and_pin_hash','pass':True})
            process.kill();process.wait(timeout=5);time.sleep(2);process,client=launch(state,config)
            restored=client.work(queued['requestId']);assert restored['contextPin']==pin and restored['reply']==queued_reply
            checks.append({'name':'native_restart_retains_exact_accepted_pin','pass':True})
            report={'commit':head,'workingTree':not bool(args.commit),'scope':'Actual native Windows Core + loopback fixture; proves context bytes/provenance transport, not model quality','goBinarySha256':hashlib.sha256((ROOT/'.build/neko-aurora.exe').read_bytes()).hexdigest(),'rustHostSha256':hashlib.sha256((ROOT/'host/target/debug/neko-host.exe').read_bytes()).hexdigest(),'coreSha256':hashlib.sha256((ROOT/'.build/core-windows/neko.exe').read_bytes()).hexdigest(),'checks':checks}
            if args.output:args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(report,indent=2),encoding='utf-8')
            print(json.dumps(report,indent=2))
    finally:
        if process and process.poll() is None:process.kill();process.wait(timeout=5)
        fixture.stdin.close()
        try:fixture.wait(timeout=5)
        except subprocess.TimeoutExpired:fixture.terminate()
if __name__=='__main__':main()
