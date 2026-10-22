"""Two local channels through actual Core; no external connector or grant."""
import argparse, hashlib, json, pathlib, subprocess, sys, tempfile, urllib.error
from e2e_core import ROOT,launch
from e2e_permissions import OwnedCoreHandle

def main():
 p=argparse.ArgumentParser();p.add_argument('--commit',required=True);p.add_argument('--output',type=pathlib.Path,required=True);args=p.parse_args()
 head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip();assert head==args.commit and not subprocess.check_output(['git','status','--porcelain'],cwd=ROOT)
 fixture=subprocess.Popen([sys.executable,str(ROOT/'scripts/fixture_provider.py')],stdin=subprocess.PIPE,stdout=subprocess.PIPE,text=True);process=None;handles=[];checks=[]
 try:
  ready=fixture.stdout.readline().strip();assert ready.startswith('READY ')
  with tempfile.TemporaryDirectory(prefix='neko-local-channels-',ignore_cleanup_errors=True) as directory:
   state=pathlib.Path(directory)/'state';config=pathlib.Path(directory)/'fixture.json';config.write_text(json.dumps({'hostExecutable':str(ROOT/'host/target/debug/neko-host.exe'),'coreExecutable':str(ROOT/'.build/core-windows/neko.exe'),'supervisor':str(ROOT/'scripts/core_supervisor.py'),'fixtureEndpoint':ready[6:]}))
   process,client=launch(state,config)
   channels=[client.request('/v1/channels',{'name':name,'botIds':['bot-meiiie','bot-davin'],'clientNonce':'native-channel-'+name})['channel'] for name in ['Alpha','Beta']]
   private=client.send('[fixture:foreground] private-Davin-canary',bot='bot-davin');client.wait(private['requestId'],lambda w:w['status']=='completed')
   def send(channel,nonce,text):return client.request('/v1/messages',{'botId':'bot-meiiie','clientNonce':nonce,'channelId':channel['id'],'text':'@davin '+text,'lane':'background'})['work']
   alpha=send(channels[0],'native-origin-alpha','[fixture:background] [fixture:artifact] Respond to Alpha with a file.');live=client.wait(alpha['requestId'],lambda w:bool(w.get('reply')))
   beta=send(channels[1],'native-origin-beta','[fixture:artifact] Respond to Beta with a file.');assert beta['contextPin']['sources']==[]
   done=client.wait(alpha['requestId'],lambda w:w['status']=='completed');second=client.wait(beta['requestId'],lambda w:w['status']=='completed')
   assert all(w['botId']=='bot-davin' and w['conversationId']==c['id'] and w['publication']['replyChannelId']==c['id'] for w,c in zip([done,second],channels))
   assert done['contextPin']['sources']==[] and second['contextPin']['sources']==[]
   checks.append({'name':'two_bots_two_channels_keep_tagged_origin_while_owner_uses_other_channel','pass':True})
   artifact=done['artifacts'][0];data,headers=client.request('/v1/channels/'+channels[0]['id']+'/artifacts/'+artifact['id']);assert hashlib.sha256(data).hexdigest()==headers['X-Artifact-SHA256']
   try:client.request('/v1/channels/'+channels[1]['id']+'/artifacts/'+artifact['id']);raise AssertionError('Cross-channel artifact allowed')
   except urllib.error.HTTPError as e:assert e.code==404
   checks.append({'name':'file_result_served_only_under_its_pinned_channel','pass':True})
   follow=send(channels[0],'native-alpha-context','[fixture:context-inspect] Inspect eligible Alpha history.');finished=client.wait(follow['requestId'],lambda w:w['status']=='completed');assert [x['requestId'] for x in follow['contextPin']['sources']]==[alpha['requestId']]
   assert alpha['requestId'] in finished['reply'] and private['requestId'] not in finished['reply'] and beta['requestId'] not in finished['reply']
   checks.append({'name':'actual_Core_pin_excludes_target_private_history_and_other_channel_history','pass':True})
   active=send(channels[0],'native-alpha-revoke','[fixture:background] [fixture:artifact] Stop on audience revision change.');running=client.wait(active['requestId'],lambda w:bool(w.get('reply')));handle=OwnedCoreHandle(running['core']['nativeProcessId']);handles.append(handle)
   client.request('/v1/channels',{'id':channels[0]['id'],'name':'Alpha','botIds':['bot-meiiie'],'expectedRevision':1,'clientNonce':'native-remove-channel-davin'})
   stopped=client.wait(active['requestId'],lambda w:w['status']=='suspended');handle.assert_stopped();assert not stopped.get('operations') and not stopped.get('artifacts')
   checks.append({'name':'membership_revision_fences_late_output_tool_and_stops_owned_Core','pass':True})
   for _ in range(3):process.kill();process.wait(timeout=5);process,client=launch(state,config);assert client.work(active['requestId'])==stopped and client.work(beta['requestId'])['publication']==second['publication']
   checks.append({'name':'restart_preserves_provenance_thread_and_revoked_audience_fence','pass':True})
  report={'commit':head,'scope':'Actual Go/Rust/native Core + loopback synthetic fixture; server-authenticated local owner, local channel membership only. No Slack/Discord connector, external grant, channel UI, real provider, cloud or phone acceptance.','checks':checks}
  for name,path in [('go',ROOT/'.build/neko-aurora.exe'),('host',ROOT/'host/target/debug/neko-host.exe'),('core',ROOT/'.build/core-windows/neko.exe')]:report[name+'Sha256']=hashlib.sha256(path.read_bytes()).hexdigest()
  args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8');print(json.dumps(report,indent=2))
 finally:
  for h in handles:h.close()
  if process and process.poll() is None:process.kill();process.wait(timeout=5)
  fixture.stdin.close()
  try:fixture.wait(timeout=5)
  except subprocess.TimeoutExpired:fixture.kill();fixture.wait(timeout=5)
if __name__=='__main__':main()
