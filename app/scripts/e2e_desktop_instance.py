"""Owned loopback desktop launch contract; verify-only never constructs a window."""
import argparse, hashlib, json, os, pathlib, shutil, socket, subprocess, sys, time, traceback
sys.dont_write_bytecode=True
ROOT=pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'scripts'))
from e2e_principal_device import Client,ready

def sha(p):return hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()
def main():
 p=argparse.ArgumentParser()
 for name in ['go','desktop','host','output']:p.add_argument('--'+name,type=pathlib.Path,required=True)
 p.add_argument('--working',action='store_true');args=p.parse_args()
 out=args.output.resolve();assert out.is_relative_to(pathlib.Path('E:/CodexScratch/desktop-instance-20261012').resolve());out.mkdir(exist_ok=False)
 private=out/'private-fixture';private.mkdir()
 env={n:os.environ[n] for n in ['SYSTEMROOT','WINDIR','PATH','PATHEXT','COMSPEC','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS'] if n in os.environ}
 env.update({'USERPROFILE':str(private),'HOME':str(private),'TEMP':str(private),'TMP':str(private),'PYTHONDONTWRITEBYTECODE':'1'})
 report={'status':'RUNNING','sourceCommit':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT).decode().strip(),'workingTree':bool(subprocess.check_output(['git','status','--porcelain'],cwd=ROOT)),'binaries':{n:sha(getattr(args,n)) for n in ['go','desktop','host']},'runnerSHA256':sha(__file__),'checks':[],'scope':'Actual compiled Go/Rust desktop CLI verify-only and owned loopback/state/cookie jars. No native window/CUA bypass, actual credential/profile/provider/grant/live8765.'}
 services=[];logs=[]
 def check(name,value):report['checks'].append({'name':name,'status':'PASS' if value else 'FAIL'});assert value,name
 def verify(record,state,endpoint=None,digest=None,extra=None):
  command=[str(args.desktop),'--launch-receipt',str(record),'--state',str(state),'--endpoint',endpoint or json.loads(record.read_text())['endpoint'],'--control-sha256',digest or sha(args.go),'--verify-only']
  if extra:command+=extra
  return subprocess.run(command,env=env,capture_output=True,timeout=10,creationflags=subprocess.CREATE_NO_WINDOW)
 def altered(source,state,name,**changes):
  data=json.loads(source.read_text());data.update(changes);f=state/(name+'.json');f.write_text(json.dumps(data),encoding='utf-8');return f
 def unused_port():
  s=socket.socket();s.bind(('127.0.0.1',0));port=s.getsockname()[1];s.close();assert port!=8765;return port
 try:
  check('clean source or explicitly labelled working probe',args.working or not report['workingTree'])
  peers=[]
  for name in ['a','b']:
   state=private/name;record=state/'desktop.json';log=(private/(name+'.log')).open('x',encoding='utf-8');logs.append(log)
   service=subprocess.Popen([str(args.go),'--state',str(state),'--listen','127.0.0.1:0','--host-executable',str(args.host),'--desktop-launch-receipt',str(record)],env=env,stdout=subprocess.PIPE,stderr=log,text=True,creationflags=subprocess.CREATE_NO_WINDOW);services.append(service)
   endpoint=ready(service);client=Client(endpoint);client.opener.open(endpoint+'/').read();data=json.loads(record.read_text());check('actual fresh '+name+' PID/endpoint binding',data['controlPid']==service.pid and data['endpoint']==endpoint and data['controlSha256']==sha(args.go))
   r=verify(record,state);check('desktop verifies actual '+name+' before window',r.returncode==0 and json.loads(r.stdout)['stateSha256']==data['stateSha256'] and b'challenge' not in r.stdout);peers.append((state,record,data,client))
  a,b=peers
  check('two simultaneous instances separate port/state/challenge',a[2]['endpoint']!=b[2]['endpoint'] and a[2]['stateSha256']!=b[2]['stateSha256'] and a[2]['challenge']!=b[2]['challenge'])
  work=a[3].request('/v1/messages',{'botId':'bot-meiiie','lane':'foreground','text':'Synthetic desktop state isolation only','clientNonce':'desktop-isolation-fixture'},code=201)['work']
  check('state history does not cross to second instance',not any(w['requestId']==work['requestId'] for w in b[3].request('/v1/snapshot')['work']))
  client=Client(b[2]['endpoint']);client.opener=a[3].opener;client.request('/v1/snapshot',code=401);check('separate owner cookie cannot attach second instance',True)
  for name,changes,digest in [('wrong-pid',{'controlPid':services[1].pid},None),('wrong-state',{'stateSha256':b[2]['stateSha256']},None),('wrong-challenge',{'challenge':'f'*64},None),('forged-hash',{'controlSha256':'f'*64},'f'*64),('old-version',{'version':0},None),('expired',{'createdAtUnixMs':int(time.time()*1000)-60000,'expiresAtUnixMs':int(time.time()*1000)-30000},None),('future-clock',{'createdAtUnixMs':int(time.time()*1000)+60000,'expiresAtUnixMs':int(time.time()*1000)+90000},None)]:
   record=altered(a[1],a[0],name,**changes);check('deny '+name+' before window',verify(record,a[0],digest=digest).returncode!=0)
  check('deny wrong selected binary hash',verify(a[1],a[0],digest='f'*64).returncode!=0)
  moved=altered(a[1],a[0],'port-spoof',endpoint=b[2]['endpoint']);check('deny listener port owned by other process',verify(moved,a[0]).returncode!=0)
  for endpoint in ['https://127.0.0.1:8767','http://example.invalid:8767','http://127.0.0.1:8767/?key=fixture','http://owner@127.0.0.1:8767','http://localhost:8767']:
   check('deny endpoint grammar '+str(len(report['checks'])),verify(a[1],a[0],endpoint=endpoint).returncode!=0)
  check('no implicit8765/default downgrade',subprocess.run([str(args.desktop)],env=env,capture_output=True,timeout=10,creationflags=subprocess.CREATE_NO_WINDOW).returncode!=0)
  check('duplicate CLI option denied',verify(a[1],a[0],extra=['--endpoint',a[2]['endpoint']]).returncode!=0)
  services[0].terminate();services[0].wait(timeout=10);check('stale receipt after own service exit denied',verify(a[1],a[0]).returncode!=0)
  check('other instance remains valid after first exit',verify(b[1],b[0]).returncode==0)
  for service in services:
   if service.poll() is None:service.terminate();service.wait(timeout=10)
  fixture=private/'launcher';(fixture/'.build').mkdir(parents=True);(fixture/'scripts').mkdir()
  manifest={'version':1,'scope':'local-journal','appCommit':report['sourceCommit'],'desktopLaunchContract':1}
  for name,file in [('control',args.go),('host',args.host),('desktop',args.desktop)]:
   target=fixture/'.build'/file.name;shutil.copyfile(file,target);manifest[name]={'path':target.relative_to(fixture).as_posix(),'sha256':sha(target)}
  shutil.copyfile(ROOT/'scripts/run-prebuilt-local.ps1',fixture/'scripts/run-prebuilt-local.ps1')
  path=fixture/'launch.json';path.write_text(json.dumps(manifest),encoding='utf-8')
  pwsh='C:/Users/Admin/.cache/codex-runtimes/codex-primary-runtime/dependencies/native/powershell/pwsh.exe'
  command=[pwsh,'-NoProfile','-File',str(fixture/'scripts/run-prebuilt-local.ps1'),'-Manifest',str(path),'-StateDirectory',str(private/'launcher-state'),'-Port',str(unused_port()),'-Desktop','-VerifyDesktopOnly']
  r=subprocess.run(command,env=env,capture_output=True,timeout=30);(private/'launcher-output.log').write_bytes(r.stdout+r.stderr);check('actual launcher resolves candidate port and proves desktop binding without window',r.returncode==0 and b'Verified desktop instance' in r.stdout)
  for name,change in [('legacy-contract',{'desktopLaunchContract':0}),('binary-hash',{'control':{**manifest['control'],'sha256':'f'*64}})]:
   altered_manifest={**manifest,**change};path.write_text(json.dumps(altered_manifest),encoding='utf-8');r=subprocess.run(command+['-CheckOnly'],env=env,capture_output=True,timeout=10);check('launcher rejects '+name+' before start',r.returncode!=0)
  report['status']='PASS'
 except BaseException as error:
  report.update({'status':'FAIL','failureType':type(error).__name__,'failure':str(error)});(out/'failure.txt').write_text(traceback.format_exc(),encoding='utf-8')
 finally:
  for service in services:
   if service.poll() is None:service.terminate();service.wait(timeout=10)
  for log in logs:log.close()
  report['ownedServiceStopped']=all(s.poll() is not None for s in services);(out/'result.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
 print(json.dumps(report));return report['status']!='PASS'
if __name__=='__main__':raise SystemExit(main())
