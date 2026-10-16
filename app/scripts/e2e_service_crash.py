"""Owned native stop-outbox and explicit recovery fixtures; no external provider."""
import argparse
import ctypes
from ctypes import wintypes
import hashlib
import http.cookiejar
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import pathlib
import subprocess
import threading
import time
import urllib.error
import urllib.request
import uuid

ROOT = pathlib.Path(__file__).resolve().parents[1]
FLAGS = 0x08000000

class Provider(BaseHTTPRequestHandler):
    calls = {}
    lock = threading.Lock()
    def log_message(self, *_): pass
    def do_POST(self):
        if self.path != "/v1/chat/completions" or self.headers.get("Authorization") != "Bearer fixture-only":
            self.send_error(403); return
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        manifest = None
        for msg in reversed(body["messages"]):
            if msg.get("role") != "assistant" or not isinstance(msg.get("content"), str): continue
            try: item = json.loads(msg["content"])
            except ValueError: continue
            if item.get("kind") == "host_context_manifest": manifest = item["manifest"]; break
        assert manifest is not None, "Core omitted admitted context"
        key = manifest["primaryMessage"]["text"]
        with self.lock:
            count = self.calls.get(key, 0) + 1; self.calls[key] = count
        self.send_response(200); self.send_header("Content-Type", "text/event-stream"); self.send_header("Connection", "close"); self.end_headers()
        def chunk(delta, finish=None):
            data = {"id":"service-crash-fixture","object":"chat.completion.chunk","created":1,"model":"neko-controlled-fixture","choices":[{"index":0,"delta":delta,"finish_reason":finish}]}
            self.wfile.write(("data: " + json.dumps(data) + "\n\n").encode()); self.wfile.flush()
        try:
            chunk({"role":"assistant","content":"Saved before service crash. "})
            if count > 1 or "unrelated" in key:
                chunk({"content":"Explicit recovery completed."}); chunk({}, "stop")
                self.wfile.write(b"data: [DONE]\n\n"); self.wfile.flush(); return
            for _ in range(240):
                time.sleep(.25); chunk({"content":""})
        except (BrokenPipeError, ConnectionResetError): pass

class Client:
    def __init__(self, base):
        self.base = base
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.opener.open(base+"/", timeout=5).close()
    def request(self, path, body=None):
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.base+path, data=data, headers={} if body is None else {"Content-Type":"application/json"})
        with self.opener.open(req,timeout=5) as r: return json.load(r)
    def work(self, request): return next(w for w in self.request("/v1/snapshot")["work"] if w["requestId"]==request)
    def wait(self, request, predicate, timeout=25):
        deadline=time.monotonic()+timeout; last={}
        while time.monotonic()<deadline:
            last=self.work(request)
            if predicate(last): return last
            time.sleep(.05)
        raise AssertionError({"requestId":request,"status":last.get("status"),"failure":last.get("failure"),"runtimeStopped":last.get("runtimeStopped")})
    def deny(self,path,body,code):
        try: self.request(path,body)
        except urllib.error.HTTPError as e: assert e.code==code, (e.code,code)
        else: raise AssertionError("Unproved control accepted")

class LiveCore:
    def __init__(self,binding,expected):
        self.k=ctypes.WinDLL("kernel32",use_last_error=True)
        self.k.OpenProcess.argtypes=[wintypes.DWORD,wintypes.BOOL,wintypes.DWORD];self.k.OpenProcess.restype=wintypes.HANDLE
        self.k.QueryFullProcessImageNameW.argtypes=[wintypes.HANDLE,wintypes.DWORD,wintypes.LPWSTR,ctypes.POINTER(wintypes.DWORD)]
        self.k.GetProcessTimes.argtypes=[wintypes.HANDLE,ctypes.POINTER(wintypes.FILETIME),ctypes.POINTER(wintypes.FILETIME),ctypes.POINTER(wintypes.FILETIME),ctypes.POINTER(wintypes.FILETIME)]
        self.k.WaitForSingleObject.argtypes=[wintypes.HANDLE,wintypes.DWORD]; self.k.WaitForSingleObject.restype=wintypes.DWORD
        self.k.CloseHandle.argtypes=[wintypes.HANDLE]
        self.h=self.k.OpenProcess(0x101000,False,binding["nativeProcessId"])
        assert self.h, "Cannot retain owned live Core handle"
        image=ctypes.create_unicode_buffer(32768);size=wintypes.DWORD(len(image))
        assert self.k.QueryFullProcessImageNameW(self.h,0,image,ctypes.byref(size))
        assert pathlib.Path(image.value).resolve()==expected.resolve(), "Wrong owned executable"
        times=[wintypes.FILETIME() for _ in range(4)]
        assert self.k.GetProcessTimes(self.h,*[ctypes.byref(t) for t in times])
        self.generation=str((times[0].dwHighDateTime<<32)|times[0].dwLowDateTime)
        if "nativeProcessGeneration" in binding: assert self.generation==binding["nativeProcessGeneration"]
        assert self.k.WaitForSingleObject(self.h,0)==258, "Core was not live"
    def stopped(self): return self.k.WaitForSingleObject(self.h,0)==0
    def close(self): self.k.CloseHandle(self.h)

def journal(state,request):
    found=None
    for line in (state/"transactions.jsonl").read_text(encoding="utf-8").splitlines():
        try: event=json.loads(line)["event"]
        except ValueError: continue
        if event.get("work",{}).get("requestId")==request: found=event["work"]
    assert found is not None
    return found

def launch(args,state,config):
    log=state.parent/('service-%d.stderr.log'%len(list(state.parent.glob('service-*.stderr.log'))))
    env={k:v for k,v in os.environ.items() if not any(x in k.upper() for x in ("API_KEY","TOKEN","SECRET","PASSWORD"))}
    p=subprocess.Popen([str(args.go),"--listen","127.0.0.1:0","--state",str(state),"--core-fixture-config",str(config)],stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=log.open("w"),text=True,env=env,creationflags=FLAGS)
    line=p.stdout.readline().strip()
    if not line.startswith("READY "): raise AssertionError("Owned Go startup failed: "+str(log))
    return p,Client(line[6:])

def control(w,nonce=None):
    return {"version":2,"action":"recover","requestId":w["requestId"],"expectedEpoch":w["epoch"],"expectedRevision":w.get("taskRevision",0),"clientNonce":nonce or str(uuid.uuid4())}

def context0_boundary(args):
    state=args.output.parent/("owned-context0-denial-"+uuid.uuid4().hex[:8]);state.mkdir();(state/"tasks").mkdir()
    for host in (args.baseline_host,args.host):
        if not host: continue
        command=[str(host),"--runtime",str(args.core),"--supervisor",str(ROOT/"scripts/core_supervisor.py"),"--state-root",str(state/"tasks"),"--fixture-endpoint","http://127.0.0.1:1/v1","--install-state-root",str(state)]
        frame={"requestId":"req-"+"a"*32,"botId":"bot-meiiie","text":"controlled missing-context boundary"}
        result=subprocess.run(command,input=json.dumps(frame)+"\n",capture_output=True,text=True,timeout=5,creationflags=FLAGS)
        assert result.returncode!=0 and result.stderr.strip()=="context_admission_required",result.stderr
        assert not list((state/"tasks").iterdir()) and not (state/"runtime-stops").exists()
    return {"mode":"native-context0-preexisting-denial","status":"PASS","ownedState":str(state),"outcome":"unchanged3dc and new host both require context1 before taskmkdir/Core spawn; outbox additionally opt-in only context1"}

def run_legacy(args,provider):
    base=args.output.parent/("owned-unchanged3dc-"+uuid.uuid4().hex[:8]);base.mkdir()
    state=base/"state";config=base/"fixture.json"
    config.write_text(json.dumps({"hostExecutable":str(args.baseline_host),"coreExecutable":str(args.core),"supervisor":str(ROOT/"scripts/core_supervisor.py"),"fixtureEndpoint":"http://127.0.0.1:%d/v1"%provider.server_port}))
    old=argparse.Namespace(**vars(args));old.go=args.baseline_go
    process=None;handle=None
    try:
        process,client=launch(old,state,config)
        accepted=client.request("/v1/messages",{"botId":"bot-meiiie","text":"owned-legacy3dc-"+uuid.uuid4().hex,"lane":"background","clientNonce":uuid.uuid4().hex})["work"]
        req=accepted["requestId"];client.wait(req,lambda w:w["status"]=="running" and bool(w.get("reply")))
        private=journal(state,req);binding=private["runtimeAttemptBinding"]
        assert private["contextProtocolVersion"]==1 and "nativeProcessGeneration" not in binding
        handle=LiveCore(binding,args.core);process.kill();process.wait(timeout=5);process=None
        deadline=time.monotonic()+15
        while not handle.stopped() and time.monotonic()<deadline:time.sleep(.05)
        assert handle.stopped() and not (state/"runtime-stops").exists()
        digest=hashlib.sha256(json.dumps(binding,sort_keys=True,separators=(",",":")).encode()).hexdigest()
        for _ in range(3):
            process,client=launch(args,state,config)
            current=client.wait(req,lambda w:w["status"]=="suspending")
            replay=journal(state,req)
            assert replay["core"]==private["core"] and replay["runtimeAttemptBinding"]==binding and replay["contextPin"]==private["contextPin"] and replay["runtimeContextSHA256"]==private["runtimeContextSHA256"]
            before=client.request("/v1/snapshot")["sequence"];client.deny("/v1/task-controls",control(current),409)
            assert client.request("/v1/snapshot")["sequence"]==before and not current.get("runtimeStopped")
            process.kill();process.wait(timeout=5);process=None
        process,client=launch(args,state,config)
        other=client.request("/v1/messages",{"botId":"bot-davin","text":"unrelated queued legacy compatibility obligation","lane":"foreground","clientNonce":uuid.uuid4().hex})["work"]
        assert other["requestId"]!=req and other["status"]=="waiting_host"
        return {"mode":"unchanged3dc-context1-journal","status":"PASS","ownedState":str(state),"restartCount":3,"priorBindingCanonicalSHA256":digest,"legacyStopRecovery":"BLOCKED_missing_generation_and_outbox","unrelatedIntake":"durable accepted; old host has no upgraded admission generation","producerSourceRoot":"immutable Root3dc","producerBinaryHashes":{name:hashlib.sha256(path.read_bytes()).hexdigest() for name,path in {"go":args.baseline_go,"host":args.baseline_host}.items()}}
    finally:
        if process and process.poll() is None:process.kill();process.wait(timeout=5)
        if handle:handle.close()

def run_case(args,provider,mode):
    base=args.output.parent/("owned-"+mode+"-"+uuid.uuid4().hex[:8]);base.mkdir()
    state=base/"state";config=base/"fixture.json"
    config.write_text(json.dumps({"hostExecutable":str(args.host),"coreExecutable":str(args.core),"supervisor":str(ROOT/"scripts/core_supervisor.py"),"fixtureEndpoint":"http://127.0.0.1:%d/v1"%provider.server_port}))
    process=None;handle=None
    try:
        process,client=launch(args,state,config)
        text="owned-service-crash-"+mode+"-"+uuid.uuid4().hex
        accepted=client.request("/v1/messages",{"botId":"bot-meiiie","text":text,"lane":"background","clientNonce":uuid.uuid4().hex})["work"]
        req=accepted["requestId"]
        active=client.wait(req,lambda w:w["status"]=="running" and bool(w.get("reply")))
        private=journal(state,req);binding=private["runtimeAttemptBinding"]
        handle=LiveCore(binding,args.core)
        if mode=="cancel":client.request("/v1/cancel",{"requestId":req})
        process.kill();process.wait(timeout=5);process=None
        outbox=state/"runtime-stops"/req/(private["attemptId"]+".json")
        early_start_missing=None
        if mode=="early":
            early_start_missing=not outbox.exists()
            process,client=launch(args,state,config)
            current=client.work(req)
            if not current.get("runtimeStopped"):
                before=client.request("/v1/snapshot")["sequence"]
                client.deny("/v1/task-controls",control(current),409)
                assert client.request("/v1/snapshot")["sequence"]==before
        deadline=time.monotonic()+15
        while not outbox.exists() and time.monotonic()<deadline:time.sleep(.02)
        assert outbox.exists(), "Actual wait did not publish stop-outbox"
        assert handle.stopped(), "Receipt published while retained Core handle is live"
        raw=outbox.read_bytes();assert len(raw)<=16384
        record=json.loads(raw)
        assert record["binding"]==binding and record["receipt"]["nativeProcessGeneration"]==handle.generation
        assert "runtimeContext" not in record and "contextPin" not in record and "scopeManifest" not in raw.decode()
        preserved=base/"original-stop-record.json";preserved.write_bytes(raw)
        if mode in ("missing","delayed"):outbox.rename(base/"delayed-stop.json")
        elif mode=="corrupt":outbox.write_bytes(raw[:-1])
        elif mode=="mismatch":record["receipt"]["nativeProcessGeneration"]=str(int(handle.generation)+1);outbox.write_text(json.dumps(record))
        if mode!="early":process,client=launch(args,state,config)
        if mode in ("missing","corrupt","mismatch","delayed"):
            current=client.wait(req,lambda w:w["status"]=="suspending")
            before=client.request("/v1/snapshot")["sequence"]
            client.deny("/v1/task-controls",control(current),409)
            assert client.request("/v1/snapshot")["sequence"]==before and not client.work(req).get("runtimeStopped")
            unrelated=client.request("/v1/messages",{"botId":"bot-davin","text":"unrelated local obligation "+uuid.uuid4().hex,"lane":"foreground","clientNonce":uuid.uuid4().hex})["work"]
            client.wait(unrelated["requestId"],lambda w:w["status"]=="completed")
            if mode!="delayed":return {"mode":mode,"status":"PASS","outcome":"task-local blocked, unrelated task completed","ownedState":str(state),"actualWaitBeforeOriginalReceipt":True}
            (base/"delayed-stop.json").rename(outbox)
        if mode=="cancel":
            stopped=client.wait(req,lambda w:bool(w.get("runtimeStopped")))
            assert stopped["desiredState"]=="cancelled"
            client.deny("/v1/task-controls",control(stopped),409)
            return {"mode":mode,"status":"PASS","outcome":"stop settles cancelled task without revival","ownedState":str(state),"actualWaitBeforeOriginalReceipt":True}
        settled=client.wait(req,lambda w:w["status"]=="suspended" and w.get("runtimeStopped"))
        old=journal(state,req)
        assert old["core"]==private["core"] and old["contextPin"]==private["contextPin"] and old["runtimeContextSHA256"]==private["runtimeContextSHA256"] and old["currentAttemptLease"]==private["currentAttemptLease"]
        time.sleep(.3);assert Provider.calls[text]==1 and client.work(req)["status"]=="suspended", "Automatic resume"
        intent=control(settled)
        ack=client.request("/v1/task-controls",intent)
        assert ack["work"]["requestId"]==req
        repeated=client.request("/v1/task-controls",intent);assert repeated["duplicate"]
        resumed=client.wait(req,lambda w:w["status"]=="completed")
        final=journal(state,req)
        assert final["contextPin"]==private["contextPin"] and final["runtimeContextSHA256"]==private["runtimeContextSHA256"]
        assert final["currentAttemptLease"]["id"]!=private["currentAttemptLease"]["id"] and final["currentAttemptLease"]["fence"]>private["currentAttemptLease"]["fence"] and final["executorPin"]==private["executorPin"]
        assert resumed["core"]["sessionId"]==active["core"]["sessionId"] and resumed["core"]["task"]["id"]==active["core"]["task"]["id"] and resumed["core"]["task"]["activationEpoch"]==active["core"]["task"]["activationEpoch"]+1
        assert resumed["core"]["contextAdmission"]["sourceIds"]==active["core"]["contextAdmission"]["sourceIds"]
        assert not final.get("operations") and not final.get("artifacts")
        return {"mode":mode,"status":"PASS","outcome":"explicit V2 same context/session, fresh lease, activation+1","ownedState":str(state),"actualWaitBeforeOriginalReceipt":True,"originalGeneration":handle.generation,"originalStopSHA256":hashlib.sha256(raw).hexdigest(),"finalManifestSHA256":final["runtimeContextSHA256"],"localModelCalls":Provider.calls[text],"delayedDeliveryFixture":mode=="delayed","earlyStartupBeforeOutbox":early_start_missing}
    finally:
        if process and process.poll() is None:process.kill();process.wait(timeout=5)
        if handle:handle.close()

def main():
    p=argparse.ArgumentParser();p.add_argument("--go",type=pathlib.Path,required=True);p.add_argument("--host",type=pathlib.Path,required=True);p.add_argument("--core",type=pathlib.Path,required=True);p.add_argument("--output",type=pathlib.Path,required=True);p.add_argument("--commit");p.add_argument("--baseline-go",type=pathlib.Path);p.add_argument("--baseline-host",type=pathlib.Path);args=p.parse_args()
    head=subprocess.check_output(["git","rev-parse","HEAD"],cwd=ROOT,text=True).strip()
    if args.commit:assert head==args.commit and not subprocess.check_output(["git","status","--porcelain"],cwd=ROOT,text=True).strip(),"Exact clean commit required"
    args.output.parent.mkdir(parents=True,exist_ok=True)
    server=ThreadingHTTPServer(("127.0.0.1",0),Provider);threading.Thread(target=server.serve_forever,daemon=True).start()
    checks=[]
    try:
        for mode in ("positive","early","delayed","missing","corrupt","mismatch","cancel"):
            checks.append(run_case(args,server,mode));print(mode+" PASS",flush=True)
        if args.baseline_go and args.baseline_host:
            checks.append(run_legacy(args,server));print("unchanged3dc-context1 PASS",flush=True)
            checks.append(context0_boundary(args));print("context0-existing-denial PASS",flush=True)
        result={"codeCommit":head,"workingTree":not bool(args.commit),"scope":"Actual owned Windows Go/Rust/Core97, whole-Go kill, retained live OS handle, controlled loopback model. Delayed case defers delivery of authentic post-wait bytes. No GUI/provider/cloud/groups.","binaries":{name:hashlib.sha256(path.read_bytes()).hexdigest() for name,path in {"go":args.go,"host":args.host,"core":args.core}.items()},"checks":checks}
        args.output.write_text(json.dumps(result,indent=2)+"\n")
    except BaseException as e:
        args.output.with_suffix(".failed.json").write_text(json.dumps({"codeCommit":head,"checks":checks,"errorType":type(e).__name__,"error":str(e)},indent=2)+"\n")
        raise
    finally:server.shutdown();server.server_close()
if __name__=="__main__":main()
