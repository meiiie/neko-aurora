"""Genuine archived Core/host processes against a controlled loopback fixture only."""
import argparse,hashlib,json,os,pathlib,subprocess,sys,time,urllib.error
import psutil
from e2e_core import Client

ROOT=pathlib.Path(__file__).resolve().parents[1]
def digest(path):return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()
def main():
 p=argparse.ArgumentParser();p.add_argument('--go',required=True,type=pathlib.Path);p.add_argument('--host',required=True,type=pathlib.Path);p.add_argument('--core',required=True,type=pathlib.Path);p.add_argument('--output',required=True,type=pathlib.Path);a=p.parse_args()
 assert a.output.resolve().as_posix().startswith('E:/CodexScratch/local-daily-use-20261012/')
 a.output.mkdir(parents=True,exist_ok=False);temp=a.output/'private-fixture';temp.mkdir()
 assert digest(a.host)=='5beef30a02ac26768d26a90cdd9c4c1ed55ec99edd9687358d08a5ae22ae1423'
 assert digest(a.core)=='97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb'
 env={k:v for k,v in os.environ.items()if k.upper() in ('SYSTEMROOT','WINDIR','PATH','PATHEXT','COMSPEC','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS')};env.update({'TEMP':str(temp),'TMP':str(temp)})
 fixture=service=None;checks=[];check=lambda name,ok:checks.append({'name':name,'status':'PASS' if ok else 'FAIL'}) or (None if ok else (_ for _ in ()).throw(AssertionError(name)))
 report={'scope':'Actual local Go/Rust/Core with controlled fixture, no real provider/model/native GUI acceptance','sourceCommit':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),'workingTree':bool(subprocess.check_output(['git','status','--porcelain'],cwd=ROOT)),'binaryHashes':{k:digest(v)for k,v in [('go',a.go),('host',a.host),('core',a.core)]},'checks':checks}
 try:
  fixture=subprocess.Popen([sys.executable,str(ROOT/'scripts/fixture_provider.py')],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True,env=env);ready=fixture.stdout.readline().strip();assert ready.startswith('READY http://127.0.0.1:')
  config=temp/'fixture.json';config.write_text(json.dumps({'hostExecutable':str(a.host),'coreExecutable':str(a.core),'supervisor':str(ROOT/'scripts/core_supervisor.py'),'fixtureEndpoint':ready[6:]}))
  service=subprocess.Popen([str(a.go),'--listen','127.0.0.1:0','--state',str(temp/'state'),'--core-fixture-config',str(config)],stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=(temp/'service.log').open('w'),text=True,env=env);ready=service.stdout.readline().strip();assert ready.startswith('READY ')
  client=Client(ready[6:]);bg=client.send('[fixture:background] [fixture:artifact] Keep this accepted report','background');live=client.wait(bg['requestId'],lambda w:w['status']=='running' and bool(w.get('reply')))
  core=psutil.Process(live['core']['nativeProcessId']);check('background_authentic_core_live_before_archive',core.is_running() and pathlib.Path(core.exe()).resolve()==a.core.resolve());pin=live['contextPin'];binding=live['core']
  state=client.request('/v1/bots/bot-meiiie/state',{'action':'archive','expectedRevision':0,'clientNonce':'native-bot-archive'})['botState'];check('archive_while_background_core_stays_live',state['archived'] and core.is_running() and client.work(bg['requestId'])['status']=='running')
  start=time.monotonic()
  try:client.send('Fresh archived input');raise AssertionError('archived fresh admitted')
  except urllib.error.HTTPError as e:check('fresh_archived_intake_rejected',e.code==409)
  fg=client.send('[fixture:foreground] Reply promptly','foreground','bot-davin');fg=client.wait(fg['requestId'],lambda w:w['status']=='completed');check('other_bot_foreground_completes_while_old_background_live',bool(fg['reply']) and core.is_running() and client.work(bg['requestId'])['status']=='running');report['foregroundSeconds']=round(time.monotonic()-start,3)
  client.request('/v1/bots/bot-meiiie/state',{'action':'defaults','expectedRevision':1,'clientNonce':'native-bot-defaults','defaultAccountId':'','defaultAccountRevision':0,'defaultModel':'desired-fixture-preference'})
  old=client.wait(bg['requestId'],lambda w:w['status']=='completed');check('old_core_binding_context_and_file_completed',old['core']==binding and old['contextPin']==pin and len(old['artifacts'])==1)
  artifact=old['artifacts'][0];content,headers=client.request('/v1/artifacts/'+artifact['id']);check('old_file_bytes_sha_download_preserved',hashlib.sha256(content).hexdigest()==artifact['sha256']==headers['X-Artifact-SHA256'])
  same=client.send(bg['text'],'background',nonce=bg['clientNonce']);check('archived_old_nonce_same_work_no_second_effect',same['requestId']==old['requestId'] and len(client.work(old['requestId'])['operations'])==1)
  client.request('/v1/bots/bot-meiiie/state',{'action':'restore','expectedRevision':2,'clientNonce':'native-bot-restore'})
  new=client.request('/v1/messages',{'botId':'bot-meiiie','clientNonce':'native-bot-default-intake','text':'[fixture:foreground] Request desired default','lane':'foreground','useBotDefaults':True,'expectedBotStateRevision':3})['work'];done=client.wait(new['requestId'],lambda w:w['status']=='completed');check('restored_new_request_pins_desired_preference',done['botStatePin']['defaultModel']=='desired-fixture-preference' and done['useBotDefaults'])
  check('desired_model_is_not_runtime_provider_claim',done['core']['coreSha256']==report['binaryHashes']['core']);report['status']='PASS'
 except Exception as e:report['status']='FAIL';report['errorType']=type(e).__name__;report['error']=str(e);raise
 finally:
  for process in [service,fixture]:
   if process and process.poll() is None:process.terminate();process.wait(timeout=30)
  (a.output/'results.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report))
if __name__=='__main__':main()
