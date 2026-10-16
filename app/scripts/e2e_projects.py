"""Actual local Go/Rust/Core project/file path with an owned synthetic provider."""
import argparse,base64,contextlib,hashlib,json,pathlib,subprocess,sys,tempfile,urllib.error
from e2e_core import ROOT,launch
from e2e_permissions import OwnedCoreHandle

def main():
 p=argparse.ArgumentParser();p.add_argument('--commit',required=True);p.add_argument('--output',type=pathlib.Path,required=True);args=p.parse_args()
 head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip();assert head==args.commit and not subprocess.check_output(['git','status','--porcelain'],cwd=ROOT)
 args.output.parent.mkdir(parents=True,exist_ok=True)
 fixture=subprocess.Popen([sys.executable,str(ROOT/'scripts/fixture_provider.py')],stdin=subprocess.PIPE,stdout=subprocess.PIPE,text=True);process=None;handles=[];checks=[];directory=None
 try:
  endpoint=fixture.stdout.readline().strip();assert endpoint.startswith('READY ')
  with contextlib.nullcontext(tempfile.mkdtemp(prefix='owned-products-',dir=args.output.parent)) as directory:
   state=pathlib.Path(directory)/'state';config=pathlib.Path(directory)/'fixture.json';config.write_text(json.dumps({'hostExecutable':str(ROOT/'host/target/debug/neko-host.exe'),'coreExecutable':str(ROOT/'.build/core-windows/neko.exe'),'supervisor':str(ROOT/'scripts/core_supervisor.py'),'fixtureEndpoint':endpoint[6:]}))
   process,client=launch(state,config)
   channel=client.request('/v1/channels',{'name':'Vendor review','botIds':['bot-meiiie','bot-davin'],'clientNonce':'product-native-channel'})['channel']
   project=client.request('/v1/projects',{'name':'Quotes','goal':'Compare vendor quotes','channelIds':[channel['id']],'clientNonce':'product-native-project'})['project']
   data=('Vendor quote: Tiếng Việt costs 120 dollars. Descriptive source only.\n'+('Source detail 🐈.\n'*450)).encode()
   upload={'projectId':project['id'],'projectRevision':1,'channelId':channel['id'],'filename':'vendor-quote.txt','contentBase64':base64.b64encode(data).decode(),'clientNonce':'product-native-file'}
   file=client.request('/v1/uploads',upload)['file'];assert client.request('/v1/uploads',upload)['file']['id']==file['id']
   downloaded,headers=client.request('/v1/projects/'+project['id']+'/files/'+file['id']+'?channelId='+channel['id']);assert downloaded==data and hashlib.sha256(downloaded).hexdigest()==headers['X-File-SHA256']
   checks.append('upload_nonce_and_hash_verified_download')
   def send(nonce,text,bot='bot-meiiie',lane='foreground'):
    return client.request('/v1/messages',{'botId':bot,'clientNonce':nonce,'channelId':channel['id'],'expectedChannelRevision':1,'projectId':project['id'],'projectRevision':1,'text':text,'lane':lane})['work']
   first=send('product-native-meiiie','[fixture:context-inspect] Inspect vendor quote')
   done=client.wait(first['requestId'],lambda w:w['status']=='completed');assert file['id'] in done['reply'] and '120 dollars' in done['reply'] and done['resources']['files'][0]['sha256']==file['sha256']
   second=send('product-native-davin','@davin [fixture:context-inspect] Review vendor quote and shared discussion')
   done=client.wait(second['requestId'],lambda w:w['status']=='completed');assert first['requestId'] in done['reply'] and done['publication']['replyChannelId']==channel['id'] and done['botId']=='bot-davin'
   checks.append('two_colleagues_receive_project_pin_and_exact_channel_history')
   read=send('product-native-read','@davin [fixture:file-read] Read vendor quote')
   done=client.wait(read['requestId'],lambda w:w['status']=='completed');assert '120 dollars' in done['reply'] and file['sha256'] in done['reply'];checks.append('actual_Core_MCP_reads_selected_source_through_Rust_Go')
   cap=send('product-native-caps','[fixture:capabilities] Inspect vendor quote actions')
   done=client.wait(cap['requestId'],lambda w:w['status']=='completed');assert 'project.read_file' in done['reply'];checks.append('dynamic_task_capabilities_cross_actual_host_boundary')
   largeproject=client.request('/v1/projects',{'name':'Large bounded source pin','goal':'Inspect four descriptive sources','channelIds':[channel['id']],'clientNonce':'product-native-large-project'})['project']
   largefiles=[]
   for index in range(4):
    content=(('Descriptive vendor quote '+str(index)+' ')*220).encode()
    source=client.request('/v1/uploads',{'projectId':largeproject['id'],'projectRevision':1,'channelId':channel['id'],'filename':'vendor-source-'+str(index)+'.md','contentBase64':base64.b64encode(content).decode(),'clientNonce':'product-native-large-file-'+str(index)})['file'];largefiles.append(source)
   largebody={'botId':'bot-davin','clientNonce':'product-native-large-prompt','channelId':channel['id'],'expectedChannelRevision':1,'projectId':largeproject['id'],'projectRevision':1,'fileIds':[source['id'] for source in largefiles],'text':'[fixture:context-inspect] Inspect vendor sources. '.ljust(32735,'x'),'lane':'foreground'}
   if first.get('contextProtocolVersion')==1:
    for prior in (first,second,read,cap):
     client.wait(prior['requestId'],lambda w:w['status']=='completed' and w.get('runtimeStopped'))
    before=client.request('/v1/snapshot')['sequence']
    try: client.request('/v1/messages',largebody)
    except urllib.error.HTTPError as error: assert error.code==400
    else: raise AssertionError('Oversized encoded context manifest was admitted')
    assert client.request('/v1/snapshot')['sequence']==before
    checks.append('encoded_context_frame_above64000_rejected_before_commit')
    largebody['clientNonce']='product-native-bounded-context';largebody['text']=largebody['text'][:9000]
   large=client.request('/v1/messages',largebody)['work']
   if first.get('contextProtocolVersion')!=1:
    assert len(large['text'].encode())+sum(len(source['excerpt'].encode()) for source in large['resources']['files'])>48000
   done=client.wait(large['requestId'],lambda w:w['status']=='completed');assert all(source['id'] in done['reply'] for source in largefiles)
   if first.get('contextProtocolVersion')==1:
    assert done['text']==largebody['text'] and done['core']['contextAdmission']['manifestSHA256']==done['runtimeContextSHA256']
    checks.append('bounded_long_primary_and_four_sources_keep_full_input_and_native_manifest_receipt')
   else: checks.append('accepted_prompt_above48KiB_matches_bounded_host_contract')
   active=send('product-native-active','[fixture:background] Continue vendor quote report','bot-meiiie','background');running=client.wait(active['requestId'],lambda w:bool(w.get('reply')));handle=OwnedCoreHandle(running['core']['nativeProcessId']);handles.append(handle)
   urgent=send('product-native-urgent','@davin [fixture:context-inspect] Explain vendor quote while report continues')
   client.wait(urgent['requestId'],lambda w:w['status']=='completed');assert client.work(active['requestId'])['status']=='running';checks.append('large_prompt_does_not_block_snapshot_or_foreground_interjection')
   client.request('/v1/file-access',{'fileId':file['id'],'expectedRevision':0,'granted':False,'clientNonce':'product-native-revoke'})
   stopped=client.wait(active['requestId'],lambda w:w['status']=='suspended');handle.assert_stopped();assert not stopped.get('artifacts');checks.append('source_revoke_fences_output_and_stops_actual_owned_Core')
   for _ in range(3):
    process.kill();process.wait(timeout=5);process,client=launch(state,config);assert client.work(active['requestId'])==stopped and client.work(second['requestId'])['resources']==second['resources']
   checks.append('three_restart_replays_keep_source_pins_and_denied_authority')
  report={'commit':head,'status':'PASS','ownedJournalRoot':str(directory),'scope':'Actual Go/Rust/pinned Windows Core with loopback synthetic provider and scratch state. Source authority/bytes/MCP transport proof, not model quality or phone/cloud acceptance.','checks':[{'name':c,'pass':True} for c in checks]}
  for name,path in [('go',ROOT/'.build/neko-control.exe'),('host',ROOT/'host/target/debug/neko-host.exe'),('core',ROOT/'.build/core-windows/neko.exe')]:report[name+'Sha256']=hashlib.sha256(path.read_bytes()).hexdigest()
  args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8');print(json.dumps(report,indent=2))
 except BaseException as error:
  report={'commit':head,'status':'FAIL','ownedJournalRoot':str(directory),'failure':type(error).__name__+': '+str(error)[:1200],'checks':[{'name':c,'pass':True} for c in checks]}
  args.output.write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8');raise
 finally:
  for h in handles:h.close()
  if process and process.poll() is None:process.kill();process.wait(timeout=5)
  if process and directory:pathlib.Path(directory,'go-stderr.txt').write_text(process.stderr.read(65536),encoding='utf-8')
  fixture.stdin.close()
  try:fixture.wait(timeout=5)
  except subprocess.TimeoutExpired:fixture.kill();fixture.wait(timeout=5)
if __name__=='__main__':main()
