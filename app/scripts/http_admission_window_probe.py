"""Real HTTP intake ACK loss composed with genuine native Manager admission."""
import argparse,hashlib,http.client,importlib.util,json,os,pathlib,socket,subprocess,threading,time,uuid
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
SOURCE=pathlib.Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('native_base',SOURCE/'scripts/e2e_service_crash.py');base=importlib.util.module_from_spec(spec);spec.loader.exec_module(base)

class Proxy:
 def __init__(self,upstream,nonce):
  self.upstream=upstream;self.nonce=nonce;self.ack_ready=threading.Event();self.drop=threading.Event();self.trace=[];self.ack=None
  proxy=self
  class Handler(BaseHTTPRequestHandler):
   protocol_version='HTTP/1.1'
   def log_message(self,*_):pass
   def do_GET(self):self.forward()
   def do_POST(self):self.forward()
   def forward(self):
    body=self.rfile.read(int(self.headers.get('Content-Length','0')));headers={k:self.headers[k]for k in ['Cookie','Content-Type']if k in self.headers}
    address=proxy.upstream.split('http://',1)[1];conn=http.client.HTTPConnection(address,timeout=5)
    conn.request(self.command,self.path,body=body or None,headers=headers);response=conn.getresponse();raw=response.read(65537);assert len(raw)<=65536
    held=self.command=='POST'and self.path=='/v1/messages'and json.loads(body).get('clientNonce')==proxy.nonce and proxy.ack is None
    if held:
     assert response.status==201
     proxy.ack=json.loads(raw);proxy.trace.append({'event':'actual_upstream_HTTP201_received_withheld','status':response.status,'responseSHA256':hashlib.sha256(raw).hexdigest(),'requestSHA256':hashlib.sha256(body).hexdigest(),'timeNs':str(time.time_ns())});proxy.ack_ready.set()
     assert proxy.drop.wait(20),'Observer did not kill owned Go/drop ACK within bound'
     proxy.trace.append({'event':'client_socket_closed_without_response','timeNs':str(time.time_ns())})
     self.connection.shutdown(socket.SHUT_RDWR);self.connection.close();conn.close();return
    self.send_response(response.status)
    for k,v in response.getheaders():
     if k.lower()in ['set-cookie','content-type','cache-control']:self.send_header(k,v)
    self.send_header('Content-Length',str(len(raw)));self.end_headers();self.wfile.write(raw);self.wfile.flush();conn.close()
  self.server=ThreadingHTTPServer(('127.0.0.1',0),Handler);self.server.daemon_threads=True
  threading.Thread(target=self.server.serve_forever,daemon=True).start();self.url='http://127.0.0.1:%d'%self.server.server_port
 def close(self):self.drop.set();self.server.shutdown();self.server.server_close()

def wait_until(predicate,seconds):
 deadline=time.monotonic()+seconds
 while time.monotonic()<deadline:
  value=predicate()
  if value:return value
  time.sleep(.02)
 return None

def load_marker(path):
 try:return json.loads(path.read_text(encoding='utf-8'))
 except(FileNotFoundError,ValueError):return None

def run(args,provider,mode):
 root=args.output_root/('owned-http-'+mode+'-'+uuid.uuid4().hex[:8]);root.mkdir();state=root/'state';config=root/'fixture.json';marker=root/'admission-observation.json';nonce='owned-http-'+uuid.uuid4().hex
 target_nonce=nonce if mode!='wrong-target'else 'owned-http-different-request'
 stage='pre-bind'if mode in ['baseline-red','pre-bind','wrong-target','default-off']else 'post-durable-pre-ack'
 hook=root/'hook-config.json';hook.write_text(json.dumps({'version':1,'clientNonce':target_nonce,'stage':stage,'observationPath':str(marker)}),encoding='utf-8')
 config.write_text(json.dumps({'hostExecutable':str(args.host),'coreExecutable':str(args.core),'supervisor':str(SOURCE/'scripts/core_supervisor.py'),'fixtureEndpoint':'http://127.0.0.1:%d/v1'%provider.server_port}),encoding='utf-8')
 process=None;handle=None;proxy=None;trace=[];out=root/'result.json';client_error={}
 old_env=os.environ.get('NEKO_ADMISSION_HTTP_FIXTURE_CONFIG');os.environ['NEKO_ADMISSION_HTTP_FIXTURE_CONFIG']=str(hook)
 try:
  initial=argparse.Namespace(**vars(args));initial.go=args.clean_go if mode=='default-off'else args.go
  process,direct=base.launch(initial,state,config);proxy=Proxy(direct.base,nonce);client=base.Client(proxy.url)
  body={'botId':'bot-meiiie','clientNonce':nonce,'text':('unrelated ' if mode=='post-durable-pre-ack' else '')+'owned HTTP admission '+mode+' '+uuid.uuid4().hex,'lane':'background'}
  def send():
   try:client.request('/v1/messages',body);client_error['unexpectedACK']=True
   except BaseException as e:client_error.update({'type':type(e).__name__,'responseReceived':False})
  thread=threading.Thread(target=send,daemon=True);thread.start();assert proxy.ack_ready.wait(10)
  req=proxy.ack['work']['requestId'];trace.append({'event':'upstream201held','requestId':req,'timeNs':str(time.time_ns())})
  needs_hook=mode in ['pre-bind','post-durable-pre-ack']
  observation=wait_until(lambda:load_marker(marker),5 if needs_hook else 1.5)
  if needs_hook:assert observation,'Exact fixture admission stage observation absent'
  if observation:
   assert observation['stage']==stage and observation['requestId']==req and observation['clientNonce']==nonce
   binding=observation['binding'];private=base.journal(state,req)
   assert private['runtimeContextSHA256']==binding['contextAdmission']['manifestSHA256']
   assert binding['contextAdmission']['sessionId']==binding['sessionId']
   if stage=='pre-bind':assert not private.get('core')and not private.get('runtimeAttemptBinding'),'PreBind window already committed binding'
   else:assert private['core']==binding and private['runtimeAttemptBinding']==binding,'Post-durable window lacks exact committed binding'
  else:
   assert mode in ['baseline-red','wrong-target','default-off']
   direct.wait(req,lambda w:w['status']=='running'and bool(w.get('reply')));private=base.journal(state,req);binding=private['runtimeAttemptBinding']
  handle=base.LiveCore(binding,args.core);assert binding['nativeProcessGeneration']==handle.generation
  trace.append({'event':'actual_live_Core_handle_and_journal_boundary','binding':binding,'status':private['status'],'bindingDurable':bool(private.get('runtimeAttemptBinding')),'timeNs':str(time.time_ns())})
  (root/'before-kill-work.json').write_text(json.dumps(private,indent=2)+'\n',encoding='utf-8')
  process.kill();process.wait(timeout=5);process=None;trace.append({'event':'whole_owned_Go_killed','timeNs':str(time.time_ns())})
  proxy.drop.set();thread.join(5);assert not thread.is_alive()and client_error.get('responseReceived')is False and not client_error.get('unexpectedACK'),'Original HTTP ACK reached client'
  assert wait_until(handle.stopped,15),'Actual retained Core HANDLE did not stop'
  trace.append({'event':'same_retained_Core_handle_signalled','generation':handle.generation,'timeNs':str(time.time_ns())})
  stop=state/'runtime-stops'/req/(private['attemptId']+'.json')
  if needs_hook:assert wait_until(stop.exists,5),'Authentic native stopped fact missing after genuine admission/Core HANDLE wait'
  elif mode!='baseline-red':assert wait_until(stop.exists,5),'Acknowledged native admission failed to publish authentic stop outbox'
  os.environ.pop('NEKO_ADMISSION_HTTP_FIXTURE_CONFIG',None)
  clean=argparse.Namespace(**vars(args));clean.go=args.clean_go
  process,direct=base.launch(clean,state,config);proxy.upstream=direct.base;client=base.Client(proxy.url)
  current=direct.wait(req,lambda w:(w['status']=='suspending' and not w.get('runtimeStopped')) if mode=='pre-bind' else (w['status']=='suspended' and bool(w.get('runtimeStopped'))) if mode=='post-durable-pre-ack' else w.get('runtimeStopped')or w['status']=='suspending')
  replay=base.journal(state,req)
  for key in ['requestId','sourceMessageId','clientNonce','text','contextPin','runtimeContext','runtimeContextSHA256','executorPin','currentAttemptLease','core','runtimeAttemptBinding']:
   assert replay.get(key)==private.get(key),'Restart rewrote original '+key
  before=direct.request('/v1/snapshot')['sequence'];duplicate=client.request('/v1/messages',body)
  assert duplicate['duplicate']and duplicate['work']['requestId']==req and direct.request('/v1/snapshot')['sequence']==before
  if mode=='pre-bind':
   direct.deny('/v1/task-controls',base.control(current,'owned-denied-recover-'+uuid.uuid4().hex),409)
   assert not direct.work(req).get('runtimeStopped')and direct.request('/v1/snapshot')['sequence']==before
   assert base.Provider.calls.get(body['text'],0)==0,'Model ran before admission ACK'
   outcome='HTTP original ACK lost; exact native admission window crash; proofless recovery blocked'
  elif mode!='baseline-red':
   if needs_hook:assert base.Provider.calls.get(body['text'],0)==0,'Model ran before admission ACK'
   settled=direct.wait(req,lambda w:w['status']=='suspended'and w.get('runtimeStopped'));intent=base.control(settled,'owned-http-recover-'+uuid.uuid4().hex)
   direct.request('/v1/task-controls',intent);assert direct.request('/v1/task-controls',intent)['duplicate']
   done=direct.wait(req,lambda w:w['status']=='completed'and w.get('runtimeStopped'));final=base.journal(state,req)
   assert final['core']['sessionId']==binding['sessionId']and final['runtimeContextSHA256']==private['runtimeContextSHA256']and final['executorPin']==private['executorPin']
   assert final['currentAttemptLease']['id']!=private['currentAttemptLease']['id']and final['currentAttemptLease']['fence']>private['currentAttemptLease']['fence']
   outcome=('Genuine admission observed in fixture build; 'if needs_hook else 'No fixture activation; ')+'real HTTP lost ACK and authenticated V2 same-context recovery'
  else:outcome='EXPECTED_RED: no deterministic preBind hook in baseline; Core binding already committed'
  other=direct.request('/v1/messages',{'botId':'bot-davin','clientNonce':uuid.uuid4().hex,'text':'unrelated owned HTTP window task '+uuid.uuid4().hex,'lane':'foreground'})['work'];direct.wait(other['requestId'],lambda w:w['status']=='completed'and w.get('runtimeStopped'))
  (root/'replayed-work.json').write_text(json.dumps(replay,indent=2)+'\n',encoding='utf-8')
  (root/'trace.json').write_text(json.dumps({'native':trace,'HTTPProxy':proxy.trace,'clientError':client_error},indent=2)+'\n',encoding='utf-8')
  data={'status':'EXPECTED_RED'if mode=='baseline-red'else'PASS','mode':mode,'outcome':outcome,'ownedState':str(state),'requestId':req,'clientNonce':nonce,'markerObserved':bool(observation),'nativeBinding':binding,'nativeOutboxPresent':stop.exists(),'localModelCalls':base.Provider.calls.get(body['text'],0),'unrelatedCompleted':other['requestId'],'goSHA256':hashlib.sha256(initial.go.read_bytes()).hexdigest(),'restartGoSHA256':hashlib.sha256(args.clean_go.read_bytes()).hexdigest(),'hostSHA256':hashlib.sha256(args.host.read_bytes()).hexdigest(),'coreSHA256':hashlib.sha256(args.core.read_bytes()).hexdigest(),'originalHTTP201Delivered':False,'actualCoreStopHandleObserved':True}
  out.write_text(json.dumps(data,indent=2)+'\n',encoding='utf-8');print(json.dumps({'mode':mode,'status':data['status'],'result':str(out)}),flush=True);return str(out)
 except BaseException as e:
  out.with_suffix('.failed.json').write_text(json.dumps({'status':'FAIL','mode':mode,'errorType':type(e).__name__,'error':str(e)[:1500],'trace':trace,'HTTPProxy':proxy.trace if proxy else[],'clientError':client_error},indent=2)+'\n',encoding='utf-8');raise
 finally:
  if process and process.poll()is None:process.kill();process.wait(timeout=5)
  if handle:handle.close()
  if proxy:proxy.close()
  if old_env is None:os.environ.pop('NEKO_ADMISSION_HTTP_FIXTURE_CONFIG',None)
  else:os.environ['NEKO_ADMISSION_HTTP_FIXTURE_CONFIG']=old_env

def main():
 p=argparse.ArgumentParser();p.add_argument('--go',type=pathlib.Path,required=True);p.add_argument('--clean-go',type=pathlib.Path,required=True);p.add_argument('--host',type=pathlib.Path,required=True);p.add_argument('--core',type=pathlib.Path,required=True);p.add_argument('--output-root',type=pathlib.Path,required=True);p.add_argument('--modes',nargs='+',required=True,choices=['baseline-red','pre-bind','post-durable-pre-ack','wrong-target','default-off']);args=p.parse_args()
 if not args.output_root.is_absolute():p.error('absolute owned output root required')
 args.output_root.mkdir(parents=True,exist_ok=False)
 server=ThreadingHTTPServer(('127.0.0.1',0),base.Provider);server.daemon_threads=True;threading.Thread(target=server.serve_forever,daemon=True).start()
 try:
  results=[run(args,server,mode)for mode in args.modes];print(json.dumps({'results':results}),flush=True)
 finally:server.shutdown();server.server_close()
if __name__=='__main__':main()
