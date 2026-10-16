import argparse,hashlib,json,os,pathlib,subprocess,sys,threading,time,traceback,urllib.parse
from http.server import ThreadingHTTPServer
sys.dont_write_bytecode=True
ROOT=pathlib.Path(__file__).resolve().parents[1];sys.path.insert(0,str(ROOT/'scripts'))
from e2e_core_auth_bridge import AuthModel,CoreHandle,Client,ready
def sha(p):return hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()
def main():
 parser=argparse.ArgumentParser()
 for n in ['go','host','core','go-tool','output']:parser.add_argument('--'+n,required=True,type=pathlib.Path)
 parser.add_argument('--commit',required=True);args=parser.parse_args()
 out=args.output.resolve();assert out.is_relative_to(pathlib.Path('E:/CodexScratch/core-auth-concurrency-20261012').resolve());out.mkdir(parents=True,exist_ok=False);private=out/'private-fixture';private.mkdir();home=private/'source-home';(home/'.neko-core').mkdir(parents=True);user=private/'user';user.mkdir();state=private/'state'
 AuthModel.oauth=True;AuthModel.no_key=False;AuthModel.secret='fixture-cached-parallel-access';AuthModel.secondary='fixture-cached-parallel-refresh';AuthModel.observations=[];AuthModel.release.clear();AuthModel.reject_release.clear()
 server=ThreadingHTTPServer(('127.0.0.1',0),AuthModel);threading.Thread(target=server.serve_forever,daemon=True).start();origin='http://127.0.0.1:%d'%server.server_address[1]
 source=home/'.neko-core/config.json';auth=home/'.neko-core/kimi-auth.json';source.write_text(json.dumps({'active_profile':'parallel-fixture','profiles':{'parallel-fixture':{'provider':'kimi','auth':'kimi_oauth','model':'auth-fixture-model','base_url':origin+'/v1'}}}),encoding='utf-8');auth.write_text(json.dumps({'access_token':AuthModel.secret,'refresh_token':AuthModel.secondary,'expires_at':int(time.time())+3900,'token_type':'Bearer'}),encoding='utf-8');source_sha=sha(source);auth_sha=sha(auth)
 config=private/'trusted-sources.json';config.write_text(json.dumps({'version':1,'hostExecutable':str(args.host),'coreExecutable':str(args.core),'supervisor':str(ROOT/'scripts/core_supervisor.py'),'sources':[{'id':'parallel-core-fixture','label':'Synthetic parallel auth','kind':'fixture','credentialHome':str(home),'providerOrigin':origin}]}),encoding='utf-8')
 env={k:os.environ[k] for k in ['SYSTEMROOT','WINDIR','PATH','PATHEXT','COMSPEC','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS'] if k in os.environ};env.update({'USERPROFILE':str(user),'HOME':str(user),'TEMP':str(private),'TMP':str(private),'PYTHONDONTWRITEBYTECODE':'1'})
 report={'status':'RUNNING','applicationCode':args.commit,'variant':'default','binaryHashes':{n:sha(getattr(args,n)) for n in ['go','host','core']},'runnerSHA256':sha(__file__),'checks':[],'scope':'One actual production manager, one synthetic Kimi source, four bot/lane jobs, owned loopback. No actual credential/provider/UI/live/candidate access.'};handles=[];service=None;log=(private/'service.log').open('x',encoding='utf-8');works=[]
 def check(name,value):report['checks'].append({'name':name,'status':'PASS' if value else 'FAIL'});assert value,name
 try:
  info=subprocess.check_output([str(args.go_tool),'version','-m',str(args.go)]).decode();check('binary VCS and unchanged native source','vcs.revision='+args.commit in info and 'vcs.modified=false' in info and not subprocess.check_output(['git','diff',args.commit,'HEAD','--','native','host'],cwd=ROOT))
  service=subprocess.Popen([str(args.go),'--listen','127.0.0.1:0','--state',str(state),'--host-executable',str(args.host),'--core-profile-config',str(config)],stdout=subprocess.PIPE,stderr=log,text=True,env=env,creationflags=subprocess.CREATE_NO_WINDOW);client=Client(ready(service));client.opener.open(client.endpoint+'/').read();channel=client.request('/v1/channels',{'name':'Parallel auth fixture','botIds':['bot-davin','bot-meiiie'],'clientNonce':'parallel-channel-create'})['channel']
  s=client.request('/v1/core-profile-sources')['sources'][0];ic=client.request('/v1/core-profile-inspection-consents',{'sourceId':s['id'],'expectedSourceRevision':s['revision'],'approveInspection':True,'ttlSeconds':300,'clientNonce':'parallel-inspection-create'})['inspectionConsent'];p=next(row for row in client.request('/v1/core-profiles?'+urllib.parse.urlencode({'sourceId':s['id'],'inspectionConsentId':ic['id']}))['profiles'] if row['id']=='parallel-fixture')
  c=client.request('/v1/core-profile-consents',{'sourceId':s['id'],'expectedSourceRevision':s['revision'],'profileId':p['id'],'profileRevision':p['profileRevision'],'provider':p['provider'],'model':p['model'],'providerOrigin':p['providerOrigin'],'channelId':channel['id'],'expectedChannelRevision':channel['revision'],'botIds':['bot-davin','bot-meiiie'],'maxInputBytes':64000,'maxOutputTokens':4096,'maxRequests':1,'maxSteps':1,'maxSeconds':90,'ttlSeconds':300,'approveExecution':True,'clientNonce':'parallel-execution-create'})['consent']
  def submit(bot,lane,n):return client.request('/v1/messages',{'botId':bot,'channelId':channel['id'],'expectedChannelRevision':channel['revision'],'lane':lane,'text':'[auth:hold] Parallel fixture '+str(n),'clientNonce':'parallel-work-'+str(n),'coreProfileSelection':{'consentId':c['id'],'expectedConsentRevision':c['revision']}},code=201)['work']
  for n,(bot,lane) in enumerate([('bot-meiiie','foreground'),('bot-meiiie','background'),('bot-davin','foreground'),('bot-davin','background')]):works.append(submit(bot,lane,n))
  deadline=time.monotonic()+35;observed={}
  while time.monotonic()<deadline:
   values={w['requestId']:w for w in client.request('/v1/snapshot')['work']}
   for w in works:
    current=values[w['requestId']]
    if current['status'] in ['recovery_required','failed','suspended','cancelled']:raise AssertionError('native admission stopped: '+current['status']+'/'+current.get('failure',''))
    if current['status']=='running' and current.get('core',{}).get('nativeProcessId',0) and w['requestId'] not in observed:
     h=CoreHandle(current['core']['nativeProcessId'],args.core);handles.append(h);observed[w['requestId']]=h
   if len(observed)==4 and len(AuthModel.observations)==4:break
   time.sleep(.05)
  check('four actual Core handles simultaneously live in one manager',len(observed)==4 and all(not h.stopped() for h in handles) and len(AuthModel.observations)==4)
  began=time.monotonic();fifth=submit('bot-meiiie','foreground',4);report['fifthAckMs']=round((time.monotonic()-began)*1000,3);check('new foreground ACK while four jobs remain alive',fifth['status']=='waiting_host' and all(not h.stopped() for h in handles))
  AuthModel.release.set()
  for w in [*works,fifth]:client.wait(w['requestId'],lambda row:row['status']=='completed',timeout=40)
  check('original four plus queued request complete once without authority changes',len(AuthModel.observations)==5 and all(o['credentialMatches'] and o['identityHeadersMatch'] for o in AuthModel.observations))
  check('source config and token cache unchanged',sha(source)==source_sha and sha(auth)==auth_sha);report['status']='PASS'
 except BaseException as error:
  report.update({'status':'FAIL','failureType':type(error).__name__,'failure':str(error)});(out/'failure.txt').write_text(traceback.format_exc(),encoding='utf-8')
 finally:
  AuthModel.release.set();AuthModel.reject_release.set()
  if service is not None and service.poll() is None:service.terminate();service.wait(timeout=10)
  for h in handles:h.close()
  log.close();server.shutdown();server.server_close();report['modelCalls']=len(AuthModel.observations);report['ownedServiceStopped']=service is None or service.poll() is not None;(out/'result.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
 print(json.dumps(report));return report['status']!='PASS'
if __name__=='__main__':raise SystemExit(main())
