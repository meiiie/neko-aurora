"""Compiled group service-crash recovery, owned state and synthetic loopback only."""
import argparse
import hashlib
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
import json
import pathlib
import re
import subprocess
import threading
import time
import uuid

from e2e_service_crash import ROOT,launch,LiveCore,journal

class Provider(BaseHTTPRequestHandler):
    calls={}
    hold_stage=1
    lock=threading.Lock()
    def log_message(self,*_):pass
    def do_POST(self):
        if self.path!="/v1/chat/completions" or self.headers.get("Authorization")!="Bearer fixture-only":
            self.send_error(403);return
        body=json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        manifest=None
        for message in reversed(body["messages"]):
            if message.get("role")!="assistant" or not isinstance(message.get("content"),str):continue
            try:item=json.loads(message["content"])
            except ValueError:continue
            if item.get("kind")=="host_context_manifest":manifest=item["manifest"];break
        assert manifest is not None,"Missing actual admitted context"
        stage_text=""
        prior_ids=[]
        for source in manifest["sources"]:
            if source["kind"]!="derived":continue
            try:value=json.loads(source["span"]["text"])
            except ValueError:continue
            if value.get("kind")=="generated-collaboration-stage":stage_text=value["text"]
            if value.get("kind")=="pinned-prior-turn":prior_ids.append(value["requestId"])
        match=re.search(r"Finite collaboration (collab-[0-9a-f]+), stage ([123]) of 3",stage_text)
        group,stage=(match.group(1),int(match.group(2))) if match else (manifest["primaryMessage"]["id"],0)
        key=group+"/"+str(stage)
        with self.lock:count=self.calls.get(key,0)+1;self.calls[key]=count
        self.send_response(200);self.send_header("Content-Type","text/event-stream");self.send_header("Connection","close");self.end_headers()
        def chunk(delta,finish=None):
            payload={"id":"group-crash-fixture","object":"chat.completion.chunk","created":1,"model":"neko-controlled-fixture","choices":[{"index":0,"delta":delta,"finish_reason":finish}]}
            self.wfile.write(("data: "+json.dumps(payload)+"\n\n").encode());self.wfile.flush()
        try:
            chunk({"role":"assistant","content":"Original group %s stage %d saved. "%(group,stage)})
            if stage==self.hold_stage and count==1:
                for _ in range(240):time.sleep(.25);chunk({"content":""})
            elif stage==3 and not any(m.get("role")=="tool" for m in body["messages"]):
                report="# Original finite conclusion\nGroup: %s\nGoal: %s\nPrior requests: %s\n"%(group,manifest["primaryMessage"]["text"],json.dumps(prior_ids))
                tool={"name":"artifact.write_text","arguments":{"filename":"group-recovery-final.md","text":report}}
                chunk({"tool_calls":[{"index":0,"id":"group-final-artifact","type":"function","function":{"name":"mcp__nekobot__execute_tool","arguments":json.dumps(tool)}}]});chunk({},"tool_calls")
                self.wfile.write(b"data: [DONE]\n\n");self.wfile.flush()
            else:
                chunk({"content":"Original finite stage resolved."});chunk({},"stop");self.wfile.write(b"data: [DONE]\n\n");self.wfile.flush()
        except (BrokenPipeError,ConnectionResetError):pass

def group(client,id):return next(g for g in client.request("/v1/collaborations")["collaborations"] if g["id"]==id)
def wait_group(client,id,predicate,timeout=35):
    deadline=time.monotonic()+timeout;last={}
    while time.monotonic()<deadline:
        last=group(client,id)
        if predicate(last):return last
        time.sleep(.05)
    raise AssertionError(last)
def recovery(g,w,nonce):
    return {"version":2,"requestId":w["requestId"],"expectedRevision":g["revision"],"expectedEpoch":w["epoch"],"expectedTaskRevision":w.get("taskRevision",0),"clientNonce":nonce}
def immutable_group(g):return {k:v for k,v in g.items() if k not in ("status","reason","revision")}
def invariant_context(w):return {k:w.get(k) for k in ("requestId","sourceMessageId","clientNonce","text","collaboration","contextPin","runtimeContextSHA256","publication","accountPin","resources","pluginPins","executorPin","reply","operations","artifacts")}

def run_case(args,provider,mode,stage):
    Provider.hold_stage=stage
    own=args.output.parent/("owned-group-%s-stage%d-"%(mode,stage)+uuid.uuid4().hex[:8]);own.mkdir()
    state=own/"state";config=own/"fixture.json"
    config.write_text(json.dumps({"hostExecutable":str(args.host),"coreExecutable":str(args.core),"supervisor":str(ROOT/"scripts/core_supervisor.py"),"fixtureEndpoint":"http://127.0.0.1:%d/v1"%provider.server_port}))
    process=None;handle=None
    try:
        process,client=launch(args,state,config)
        channel=client.request("/v1/channels",{"name":"Original group crash","botIds":["bot-meiiie","bot-davin"],"clientNonce":uuid.uuid4().hex})["channel"]
        create={"channelId":channel["id"],"expectedChannelRevision":channel["revision"],"participants":["bot-meiiie","bot-davin"],"coordinatorBotId":"bot-meiiie","goal":"Original bounded goal, save a final report file. "+uuid.uuid4().hex,"maxStages":3,"clientNonce":uuid.uuid4().hex}
        g=client.request("/v1/collaborations",create)["collaboration"]
        mapped=wait_group(client,g["id"],lambda x:len(x["requestIds"])==stage)
        req=mapped["requestIds"][-1]
        running=client.wait(req,lambda x:x["status"]=="running" and bool(x.get("reply")))
        private=journal(state,req);binding=private["runtimeAttemptBinding"];handle=LiveCore(binding,args.core)
        original_context=invariant_context(private);original_group=immutable_group(mapped)
        receipt=state/"runtime-stops"/req/(private["attemptId"]+".json")
        process.kill();process.wait(timeout=5);process=None
        deadline=time.monotonic()+15
        while (not handle.stopped() or not receipt.exists()) and time.monotonic()<deadline:time.sleep(.05)
        assert handle.stopped() and receipt.exists(),"Actual wait/outbox missing"
        actual=json.loads(receipt.read_text());assert actual["binding"]==binding and actual["receipt"]["nativeProcessGeneration"]==handle.generation
        retained=None
        if mode in ("missing","delayed","corrupt","cancel-delayed"):
            retained=own/"authentic-stop-retained.json";receipt.rename(retained)
            if mode=="corrupt":receipt.write_bytes(b'{"version":1,')
        process,client=launch(args,state,config)
        suspended=client.wait(req,lambda x:x["status"] in ("suspending","suspended"))
        stopped_group=wait_group(client,g["id"],lambda x:x["status"]=="suspended")
        assert immutable_group(stopped_group)==original_group and len(stopped_group["requestIds"])==stage
        assert "local_collaboration_service_recovery_v1" in client.request("/v1/snapshot")["capabilities"]
        path="/v1/collaborations/"+g["id"]+"/recover"
        if retained:
            sequence=client.request("/v1/snapshot")["sequence"]
            client.deny(path,recovery(stopped_group,suspended,uuid.uuid4().hex),409)
            assert client.request("/v1/snapshot")["sequence"]==sequence
            assert Provider.calls[g["id"]+"/"+str(stage)]==1,"Auto resumed without proof"
            if mode=="cancel-delayed":
                client.request("/v1/collaborations/"+g["id"]+"/cancel",{"expectedRevision":stopped_group["revision"],"clientNonce":uuid.uuid4().hex})
                cancelling=wait_group(client,g["id"],lambda x:x["status"]=="cancelling")
                pending=client.wait(req,lambda x:x["status"]=="cancelling" and not x.get("runtimeStopped"))
                client.deny(path,recovery(cancelling,pending,uuid.uuid4().hex),409)
                retained.rename(receipt)
                cancelled=wait_group(client,g["id"],lambda x:x["status"]=="cancelled")
                settled=client.work(req);assert settled["status"]=="cancelled" and settled["desiredState"]=="cancelled" and settled["runtimeStopped"]
                assert settled["runtimeStopReceipt"]["nativeProcessGeneration"]==handle.generation
                client.deny(path,recovery(cancelled,settled,uuid.uuid4().hex),409)
                assert Provider.calls[g["id"]+"/"+str(stage)]==1
                return {"mode":mode,"stage":stage,"status":"PASS","ownedState":str(state),"proof":"actual post-wait receipt withheld until explicit cancel; delayed delivery settles cancel without recovery or next stage"}
            if mode=="delayed":
                retained.rename(receipt)
                suspended=client.wait(req,lambda x:x["status"]=="suspended" and x.get("runtimeStopped"))
            else:
                other=client.request("/v1/messages",{"botId":"bot-davin","text":"Unrelated owner work remains alive","lane":"foreground","clientNonce":uuid.uuid4().hex})["work"]
                client.wait(other["requestId"],lambda x:x["status"]=="completed")
                return {"mode":mode,"stage":stage,"status":"PASS","ownedState":str(state),"proof":"authentic post-wait receipt withheld or partial; task-local recovery denied","unrelated":"completed"}
        suspended=client.wait(req,lambda x:x["status"]=="suspended" and x.get("runtimeStopped"))
        stopped_group=group(client,g["id"])
        before=journal(state,req);assert invariant_context(before)==original_context
        assert Provider.calls[g["id"]+"/"+str(stage)]==1,"Implicit provider work before explicit recovery"
        if mode=="cancel":
            client.request("/v1/collaborations/"+g["id"]+"/cancel",{"expectedRevision":stopped_group["revision"],"clientNonce":uuid.uuid4().hex})
            cancelled=wait_group(client,g["id"],lambda x:x["status"]=="cancelled")
            client.deny(path,recovery(cancelled,client.work(req),uuid.uuid4().hex),409)
            assert len(cancelled["requestIds"])==stage and Provider.calls[g["id"]+"/"+str(stage)]==1
            return {"mode":mode,"stage":stage,"status":"PASS","ownedState":str(state),"outcome":"explicit cancel retains same native proof and blocks recovery/next stage"}
        if mode=="ancestor-revoke":
            prior=stopped_group["requestIds"][0]
            client.request("/v1/context-access",{"requestId":prior,"expectedRevision":0,"granted":False,"clientNonce":uuid.uuid4().hex})
            sequence=client.request("/v1/snapshot")["sequence"]
            client.deny(path,recovery(group(client,g["id"]),client.work(req),uuid.uuid4().hex),409)
            assert client.request("/v1/snapshot")["sequence"]==sequence
            client.request("/v1/context-access",{"requestId":prior,"expectedRevision":1,"granted":True,"clientNonce":uuid.uuid4().hex})
            client.deny(path,recovery(group(client,g["id"]),client.work(req),uuid.uuid4().hex),409)
            assert Provider.calls[g["id"]+"/"+str(stage)]==1
            return {"mode":mode,"stage":stage,"status":"PASS","ownedState":str(state),"outcome":"revoked prior source and regrant cannot repin original stage authority"}
        body=recovery(stopped_group,suspended,uuid.uuid4().hex)
        accepted=client.request(path,body);assert not accepted["duplicate"]
        assert immutable_group(accepted["collaboration"])==original_group
        # API public receipt strips the runtime manifest; inspect durable private journal instead.
        fresh=client.wait(req,lambda x:x["status"]=="completed")
        after=journal(state,req)
        for field in ("requestId","sourceMessageId","clientNonce","text","collaboration","contextPin","runtimeContextSHA256","publication","accountPin","resources","pluginPins","executorPin"):
            assert after.get(field)==before.get(field),(field,"authority drift")
        assert after["attemptId"]!=before["attemptId"] and after["currentAttemptLease"]["id"]!=before["currentAttemptLease"]["id"]
        assert after["currentAttemptLease"]["fence"]>before["currentAttemptLease"]["fence"]
        assert after["core"]["sessionId"]==before["core"]["sessionId"]
        old_admission,new_admission=before["core"]["contextAdmission"],after["core"]["contextAdmission"]
        for field in ("sourceIds","contextPinSHA256","manifestSHA256","sessionId"):assert old_admission[field]==new_admission[field]
        assert after["core"]["task"]["id"]==before["core"]["task"]["id"] and after["core"]["task"]["activationEpoch"]==before["core"]["task"]["activationEpoch"]+1
        final=wait_group(client,g["id"],lambda x:x["status"]=="completed")
        assert len(final["requestIds"])==3 and final["requestIds"][:stage]==mapped["requestIds"]
        assert final["deadlineAt"]==mapped["deadlineAt"] and final["createdAt"]==mapped["createdAt"]
        assert [client.work(id)["botId"] for id in final["requestIds"]]==["bot-meiiie","bot-davin","bot-meiiie"]
        stages=[client.work(id) for id in final["requestIds"]]
        assert not stages[0].get("operations") and not stages[1].get("operations") and len(stages[2]["operations"])==1 and stages[2]["operations"][0]["status"]=="completed"
        artifact=stages[2]["artifacts"][0]
        with client.opener.open(client.base+"/v1/channels/"+channel["id"]+"/artifacts/"+artifact["id"],timeout=5) as response:
            content=response.read();assert response.headers["X-Artifact-SHA256"]==artifact["sha256"]
        expected="# Original finite conclusion\nGroup: %s\nGoal: %s\nPrior requests: %s\n"%(g["id"],create["goal"],json.dumps(final["requestIds"][:2]))
        assert content==expected.encode() and hashlib.sha256(content).hexdigest()==artifact["sha256"]
        for i,w in enumerate(stages):
            assert w["contextProtocolVersion"]==1
            sources=[src["requestId"] for src in w["contextPin"]["sources"]]
            assert all(id in sources for id in final["requestIds"][:i])
            private_stage=journal(state,w["requestId"])
            assert private_stage["runtimeContext"]["primaryMessage"]["text"]==create["goal"]
            assert private_stage["runtimeContext"]["primaryMessage"]["id"]==g["id"]
        retry=client.request(path,body);assert retry["duplicate"] and retry["collaboration"]==final and retry["work"]["requestId"]==req
        lookup="/v1/command-receipts?kind=collaboration_recover&clientNonce="+body["clientNonce"]+"&collaborationId="+g["id"]
        result=client.request(lookup);assert result["found"] and result["input"]==body and result["work"]["requestId"]==req
        assert "contextPin" not in result["work"] and "runtimeContext" not in result["work"] and "contextAttempts" not in result["work"]
        process.kill();process.wait(timeout=5);process=None
        process,client=launch(args,state,config)
        reread=client.request(lookup);assert reread["found"] and reread["input"]==body and group(client,g["id"])["requestIds"]==final["requestIds"]
        return {"mode":mode,"stage":stage,"status":"PASS","ownedState":str(state),"nativeGeneration":handle.generation,"sameContextSessionAndSourceIds":True,"newAttemptLeaseFence":True,"originalDeadline":final["deadlineAt"],"requestIds":final["requestIds"],"commandReceiptSurvivesRestart":True,"onlyFinalWriterArtifactSHA256":artifact["sha256"],"exactOriginalGoalAndFileBytes":True}
    finally:
        if process and process.poll() is None:process.kill();process.wait(timeout=5)
        if handle:handle.close()

def main():
    p=argparse.ArgumentParser();p.add_argument("--commit",required=True);p.add_argument("--go",type=pathlib.Path,required=True);p.add_argument("--host",type=pathlib.Path,required=True);p.add_argument("--core",type=pathlib.Path,required=True);p.add_argument("--output",type=pathlib.Path,required=True);p.add_argument("--case",action="append");args=p.parse_args();args.output=args.output.resolve()
    head=subprocess.check_output(["git","rev-parse","HEAD"],cwd=ROOT,text=True).strip();assert head==args.commit
    assert not subprocess.check_output(["git","status","--porcelain"],cwd=ROOT),"Dirty source"
    args.output.parent.mkdir(parents=True,exist_ok=True)
    report={"commit":head,"workingTree":False,"environment":"Windows; compiled Go/Rust/Core; retained actual live Core handle; synthetic loopback; no GUI/cloud/provider credential","binaries":{n:{"path":str(v),"sha256":hashlib.sha256(v.read_bytes()).hexdigest()} for n,v in {"go":args.go,"host":args.host,"core":args.core}.items()},"cases":[]}
    provider=ThreadingHTTPServer(("127.0.0.1",0),Provider);threading.Thread(target=provider.serve_forever,daemon=True).start()
    cases=[("positive",1),("positive",2),("positive",3),("delayed",1),("missing",1),("corrupt",1),("cancel",2),("cancel-delayed",2),("ancestor-revoke",2)]
    if args.case:cases=[(s.split(":")[0],int(s.split(":")[1])) for s in args.case]
    try:
        for mode,stage in cases:
            try:report["cases"].append(run_case(args,provider,mode,stage))
            except Exception as error:
                report["cases"].append({"mode":mode,"stage":stage,"status":"FAIL","error":str(error)});raise
            finally:args.output.write_text(json.dumps(report,indent=2))
    finally:provider.shutdown();provider.server_close()
    print(json.dumps({"status":"PASS","cases":len(report["cases"]),"evidence":str(args.output)}))
if __name__=="__main__":main()
