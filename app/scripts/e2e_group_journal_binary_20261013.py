"""Pinned real Go binaries and fresh journals only; no Core/provider/host/browser."""
import argparse,hashlib,http.cookiejar,json,os,pathlib,queue,shutil,subprocess,threading,urllib.error,urllib.request

ROOT=pathlib.Path(__file__).resolve().parents[1]
SCRATCH=pathlib.Path('E:/CodexScratch/group-journal-binary-20261013').resolve()
NEW=pathlib.Path('E:/CodexScratch/parallel-product-20261013/candidate-5349523/.build/release/neko-aurora.exe')
OLD=pathlib.Path('E:/CodexScratch/desktop-instance-20261012/candidate-c91e4de/.build/desktop-c91e4de/neko-aurora.exe')
NEW_SHA='c2732194124fb989dc84b301fb06e910f64579e94f79e930bf57b1b91ffa337d'
OLD_SHA='a7663f40fb0d3893f64e1fac8f933d42672812228b6ffb7837665d01bb6ecd03'
sha=lambda p:hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()

def main():
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--output',type=pathlib.Path,required=True);args=parser.parse_args();out=args.output.resolve()
 assert out.is_relative_to(SCRATCH) and out!=SCRATCH;out.mkdir(parents=True,exist_ok=False);private=out/'private-fixture';private.mkdir()
 env={k:os.environ[k] for k in ['SYSTEMROOT','WINDIR','PATH','PATHEXT','COMSPEC','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS'] if k in os.environ};env.update({'TEMP':str(private),'TMP':str(private),'USERPROFILE':str(private)})
 result={'status':'RUNNING','command':['C:/Python313/python.exe',str(pathlib.Path(__file__).resolve()),'--output',str(out)],'sourceReference':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT).decode().strip(),'runnerSHA256':sha(__file__),'workingSourceMode':'Other workers may edit source; executed binaries are independently pinned and no working source is compiled.','binaryCode':'53495231a06aac97c1092029fda35971bdbc08c5','environment':{'platform':'Windows amd64','python':'3.13','safeEnvKeys':sorted(env)},'binaries':{'new':{'path':str(NEW),'sha256':sha(NEW)},'old':{'path':str(OLD),'sha256':sha(OLD)}},'checks':[],'launches':[],'scope':__doc__};processes=[];logs=[]
 def check(name,ok):
  result['checks'].append({'name':name,'status':'PASS' if ok else 'FAIL'});assert ok,name
 def launch(binary,state):
  log=(private/('service-'+str(len(processes))+'.stderr')).open('w',encoding='utf-8');logs.append(log);command=[str(binary),'--listen','127.0.0.1:0','--state',str(state)]
  p=subprocess.Popen(command,cwd=ROOT,env=env,stdout=subprocess.PIPE,stderr=log,text=True,encoding='utf-8');processes.append(p);result['launches'].append({'binarySHA256':sha(binary),'command':command,'pid':p.pid})
  q=queue.Queue();threading.Thread(target=lambda:q.put(p.stdout.readline().strip()),daemon=True).start();line=q.get(timeout=15);result['launches'][-1]['startupLine']=line;return p,line
 def stop(p):
  if p.poll() is None:p.terminate();p.wait(timeout=10)
 def api(endpoint):
  opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()));opener.open(endpoint,timeout=5).read()
  def call(route,body=None):
   req=urllib.request.Request(endpoint+route,data=json.dumps(body).encode() if body is not None else None,headers={'Origin':endpoint,'Content-Type':'application/json'})
   try:
    with opener.open(req,timeout=5) as response:return response.status,json.loads(response.read())
   except urllib.error.HTTPError as error:return error.code,json.loads(error.read())
  return call
 def group(call,nonce):
  code,value=call('/v1/channels',{'name':'Owned journal compatibility fixture','botIds':['bot-meiiie','bot-davin'],'expectedRevision':0,'clientNonce':nonce+'-channel'});assert code==200,(code,value);channel=value['channel']
  body={'channelId':channel['id'],'expectedChannelRevision':channel['revision'],'participants':['bot-meiiie','bot-davin'],'coordinatorBotId':'bot-meiiie','goal':'Prove local journal compatibility without executing Core','maxStages':3,'clientNonce':nonce}
  code,value=call('/v1/collaborations',body);assert code==201,(code,value);return value['collaboration'],body
 def raw_group(state,id):
  events=[json.loads(line) for line in(state/'transactions.jsonl').read_text(encoding='utf-8').splitlines()];groups=[]
  def find(value):
   if isinstance(value,dict):
    if value.get('id')==id and 'publishedReportContext'in value:groups.append(value)
    for item in value.values():find(item)
   elif isinstance(value,list):
    for item in value:find(item)
  find(events);assert groups;return groups[0]
 try:
  check('candidate_and_legacy_binary_hashes_match',sha(NEW)==NEW_SHA and sha(OLD)==OLD_SHA)
  go='C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe';info=subprocess.check_output([go,'version','-m',str(NEW)],env=env,text=True,encoding='utf-8');check('candidate_binary_vcs534_unmodified','vcs.revision=53495231a06aac97c1092029fda35971bdbc08c5'in info and 'vcs.modified=false'in info);(out/'candidate-build-info.txt').write_text(info,encoding='utf-8')
  v2state=private/'new-v2';p,line=launch(NEW,v2state);check('new_binary_opens_fresh_state',line.startswith('READY http://127.0.0.1:'));call=api(line[6:]);g2,in2=group(call,'binary-compat-v2-20261013');snap=call('/v1/snapshot')[1];check('v2_group_accepted_with_no_running_work',any(g['id']==g2['id'] for g in snap['collaborations']) and all(w['status']!='running' for w in snap['work']));stop(p);raw2=raw_group(v2state,g2['id']);check('real_http_created_journal_policy_v2',raw2['publishedReportContext']['version']==2)
  p,line=launch(NEW,v2state);check('same_candidate_replays_v2_journal',line.startswith('READY http://127.0.0.1:'));call=api(line[6:]);code,dup=call('/v1/collaborations',in2);check('v2_restart_original_nonce_is_duplicate',code==200 and dup['duplicate'] and dup['collaboration']['id']==g2['id']);stop(p);check('v2_frozen_policy_pin_unchanged',raw_group(v2state,g2['id'])['publishedReportContext']==raw2['publishedReportContext'])
  denied=private/'old-reader-v2-copy';assert denied.resolve().is_relative_to(private.resolve());shutil.copytree(v2state,denied);before=sha(denied/'transactions.jsonl');p,line=launch(OLD,denied);p.wait(timeout=10);check('old_reader_rejects_new_v2_before_ready',not line.startswith('READY ') and p.returncode!=0);logs[-1].flush();stderr=(private/('service-'+str(len(processes)-1)+'.stderr')).read_text(encoding='utf-8');check('old_reader_returns_safe_store_unavailable_error','Local store unavailable; inspect state before retry'in line+stderr);check('rejected_copy_journal_is_unchanged',sha(denied/'transactions.jsonl')==before)
  v1state=private/'legacy-v1';p,line=launch(OLD,v1state);check('old_binary_opens_separate_fresh_state',line.startswith('READY http://127.0.0.1:'));call=api(line[6:]);g1,in1=group(call,'binary-compat-v1-20261013');stop(p);raw1=raw_group(v1state,g1['id']);check('legacy_http_created_journal_policy_v1',raw1['publishedReportContext']['version']==1)
  p,line=launch(NEW,v1state);check('candidate_opens_genuine_v1_journal',line.startswith('READY http://127.0.0.1:'));call=api(line[6:]);code,dup=call('/v1/collaborations',in1);check('v1_duplicate_identity_retained_after_upgrade',code==200 and dup['duplicate'] and dup['collaboration']['id']==g1['id']);stop(p);check('upgrade_retains_original_v1_frozen_policy',raw_group(v1state,g1['id'])['publishedReportContext']==raw1['publishedReportContext'])
  result['journalPolicies']={'new':raw2['publishedReportContext']['version'],'legacy':raw1['publishedReportContext']['version']};result['status']='PASS'
 except Exception as error:result['status']='FAIL';result['error']=str(error)
 finally:
  for p in processes:stop(p)
  for log in logs:log.close()
  result['ownedProcessesStopped']=all(p.poll() is not None for p in processes);(out/'results.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8');print(json.dumps(result));return int(result['status']!='PASS')

if __name__=='__main__':raise SystemExit(main())
