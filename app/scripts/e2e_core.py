"""Real archived Core -> Rust host -> Go durable UI/API acceptance with fixture only."""
import argparse
import hashlib
import http.cookiejar
import json
import pathlib
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid
import sys
import ctypes
from ctypes import wintypes

ROOT = pathlib.Path(__file__).resolve().parents[1]
def linux(path):
    path = pathlib.Path(path).resolve()
    return "/mnt/" + path.drive[0].lower() + "/" + path.as_posix()[3:]
class Client:
    def __init__(self, base):
        self.base = base
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.opener.open(base + "/", timeout=5).close()
    def request(self, path, body=None):
        headers={}
        data=None
        if body is not None:
            data=json.dumps(body).encode();headers={"Content-Type":"application/json"}
        with self.opener.open(urllib.request.Request(self.base+path,data=data,headers=headers),timeout=5) as response:
            content=response.read()
            if path.startswith("/v1/artifacts/"):return content,response.headers
            return json.loads(content)
    def send(self, text, lane="foreground", bot="bot-meiiie", nonce=None):
        return self.request("/v1/messages",{"botId":bot,"text":text,"lane":lane,"clientNonce":nonce or str(uuid.uuid4())})["work"]
    def work(self,id):
        return next(w for w in self.request("/v1/snapshot")["work"] if w["requestId"]==id)
    def wait(self,id,predicate,timeout=25):
        deadline=time.monotonic()+timeout
        while time.monotonic()<deadline:
            work=self.work(id)
            if predicate(work):return work
            time.sleep(.1)
        raise AssertionError({"requestId":id,"last":work["status"],"failure":work.get("failure")})
def launch(state, config):
    p=subprocess.Popen([str(ROOT/".build/neko-control.exe"),"--listen","127.0.0.1:0","--state",str(state),"--core-fixture-config",str(config)],stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    ready=p.stdout.readline().strip()
    if not ready.startswith("READY "):raise RuntimeError("Go service failed to start")
    return p,Client(ready[6:])
def terminate_owned_core(pid,expected):
    kernel=ctypes.WinDLL("kernel32",use_last_error=True)
    kernel.OpenProcess.argtypes=[wintypes.DWORD,wintypes.BOOL,wintypes.DWORD];kernel.OpenProcess.restype=wintypes.HANDLE
    kernel.QueryFullProcessImageNameW.argtypes=[wintypes.HANDLE,wintypes.DWORD,wintypes.LPWSTR,ctypes.POINTER(wintypes.DWORD)]
    kernel.TerminateProcess.argtypes=[wintypes.HANDLE,wintypes.UINT];kernel.CloseHandle.argtypes=[wintypes.HANDLE]
    handle=kernel.OpenProcess(0x1001,False,pid)
    if not handle:raise RuntimeError("Owned Core handle unavailable")
    try:
        image=ctypes.create_unicode_buffer(32768);size=wintypes.DWORD(len(image))
        assert kernel.QueryFullProcessImageNameW(handle,0,image,ctypes.byref(size))
        assert pathlib.Path(image.value).resolve()==expected.resolve(),"Refusing to terminate another executable"
        assert kernel.TerminateProcess(handle,1)
    finally:kernel.CloseHandle(handle)
def main():
    parser=argparse.ArgumentParser();parser.add_argument("--output",type=pathlib.Path);parser.add_argument("--commit");parser.add_argument("--native-core",action="store_true");parser.add_argument("--serve",action="store_true");args=parser.parse_args()
    head=subprocess.check_output(["git","rev-parse","HEAD"],cwd=ROOT,text=True).strip()
    if args.commit and (head!=args.commit or subprocess.check_output(["git","status","--porcelain"],cwd=ROOT,text=True).strip()):raise AssertionError("Expected exact clean code commit")
    fixture_command=[sys.executable,str(ROOT/"scripts/fixture_provider.py")] if args.native_core else ["wsl.exe","-d","Ubuntu","--exec","/usr/bin/python3",linux(ROOT/"scripts/fixture_provider.py")]
    fixture=subprocess.Popen(fixture_command,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True)
    process=None;checks=[]
    try:
        endpoint=fixture.stdout.readline().strip();assert endpoint.startswith("READY ")
        temporary=tempfile.TemporaryDirectory(prefix="neko-core-e2e-",ignore_cleanup_errors=True)
        with temporary as directory:
            state=pathlib.Path(directory)/"state";config=pathlib.Path(directory)/"fixture.json"
            core_binary=ROOT/(".build/core-windows/neko.exe" if args.native_core else ".build/core-runtime/neko")
            config.write_text(json.dumps({"hostExecutable":str(ROOT/"host/target/debug/neko-host.exe"),"coreExecutable":str(core_binary),"supervisor":str(ROOT/"scripts/core_supervisor.py"),"fixtureEndpoint":endpoint[6:]}))
            process,client=launch(state,config)
            if args.serve:
                print(client.base,flush=True)
                input("Press Enter to stop fixture preview\n");return
            bg=client.send("[fixture:background] [fixture:artifact] Tạo tệp báo cáo cho tôi.","background")
            client.wait(bg["requestId"],lambda w:bool(w.get("reply")))
            start=time.monotonic();fg=client.send("[fixture:foreground] Tôi vừa chèn tin mới.")
            foreground=client.wait(fg["requestId"],lambda w:w["status"]=="completed")
            elapsed=round(time.monotonic()-start,3)
            assert client.work(bg["requestId"])["status"]=="running", "Foreground blocked behind background"
            assert foreground["reply"] and foreground["core"]["sessionId"]!=client.work(bg["requestId"])["core"]["sessionId"]
            checks.append({"name":"foreground_reply_while_background_running","pass":True,"seconds":elapsed})
            background=client.wait(bg["requestId"],lambda w:w["status"]=="completed")
            assert len(background["artifacts"])==1 and len(background["operations"])==1
            artifact=background["artifacts"][0];content,headers=client.request("/v1/artifacts/"+artifact["id"])
            assert hashlib.sha256(content).hexdigest()==artifact["sha256"]==headers["X-Artifact-SHA256"]
            assert "Tệp UTF-8" in content.decode()
            checks.append({"name":"real_core_tool_durable_artifact_download","pass":True,"sha256":artifact["sha256"]})
            again=client.send(bg["text"],"background",nonce=bg["clientNonce"]);assert again["requestId"]==bg["requestId"]
            assert len(client.work(bg["requestId"])["operations"])==1
            checks.append({"name":"duplicate_inbound_no_second_effect","pass":True})
            interrupted=client.send("[fixture:background] Công việc phục hồi sau crash.","background",bot="bot-davin")
            active=client.wait(interrupted["requestId"],lambda w:bool(w.get("reply")))
            process.kill();process.wait(timeout=5);time.sleep(7)
            process,client=launch(state,config)
            recovered=client.work(interrupted["requestId"]);assert recovered["status"]=="recovery_required"
            assert recovered["core"]==active["core"] and recovered["reply"]==active["reply"]
            assert client.work(bg["requestId"])["artifacts"]==background["artifacts"]
            client.request("/v1/recover",{"requestId":interrupted["requestId"]})
            resumed=client.wait(interrupted["requestId"],lambda w:w["status"]=="completed")
            assert resumed["core"]["sessionId"]==active["core"]["sessionId"]
            assert resumed["core"]["task"]["id"]==active["core"]["task"]["id"]
            assert resumed["core"]["task"]["activationEpoch"]==active["core"]["task"]["activationEpoch"]+1
            checks.append({"name":"hard_kill_then_core_resume_same_task_rotated_activation","pass":True})
            cancel=client.send("[fixture:background] Công việc cần hủy.","background")
            client.wait(cancel["requestId"],lambda w:bool(w.get("reply")))
            client.request("/v1/cancel",{"requestId":cancel["requestId"]})
            cancelled=client.wait(cancel["requestId"],lambda w:w["status"]=="cancelled")
            assert not cancelled.get("artifacts") and cancelled["epoch"]==2
            checks.append({"name":"cancel_fences_late_core_events","pass":True})
            if args.native_core:
                hard=client.send("[fixture:background] Forced Core crash fixture.","background",bot="bot-davin")
                running=client.wait(hard["requestId"],lambda w:bool(w.get("reply")))
                receipt=running["core"];lock=state/"tasks"/hard["requestId"] / ".core-home/.neko-core/task-sessions" / (receipt["sessionId"]+".lock")
                lock_hash=hashlib.sha256(lock.read_bytes()).hexdigest()
                terminate_owned_core(receipt["nativeProcessId"],core_binary)
                client.wait(hard["requestId"],lambda w:w["status"]=="recovery_required")
                client.request("/v1/recover",{"requestId":hard["requestId"]})
                blocked=client.wait(hard["requestId"],lambda w:w["status"]=="recovery_required" and w["epoch"]==2)
                assert blocked["failure"]=="core_writer_unavailable" and blocked["core"]==receipt
                assert hashlib.sha256(lock.read_bytes()).hexdigest()==lock_hash and not blocked.get("artifacts")
                checks.append({"name":"forced_core_kill_retains_mapping_and_lock_fails_closed","pass":True,"resumeGate":"blocked_by_pinned_Core_v1_writer_recovery_contract"})
            result={"scope":("Native Windows Go/Rust/Core" if args.native_core else "Windows Go/Rust + WSL Core")+" 1.8.0, loopback controlled fixture; no real provider. Go is hard-killed; Rust supervisor closes Core via ACP before resume. Forced Core kill with stale lock is a separate blocked gate.","commit":head,"workingTree":not bool(args.commit),"goBinarySha256":hashlib.sha256((ROOT/".build/neko-control.exe").read_bytes()).hexdigest(),"rustHostSha256":hashlib.sha256((ROOT/"host/target/debug/neko-host.exe").read_bytes()).hexdigest(),"coreSha256":hashlib.sha256(core_binary.read_bytes()).hexdigest(),"checks":checks}
            if args.output:args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding="utf-8")
            print(json.dumps(result,ensure_ascii=False,indent=2))
    finally:
        if process and process.poll() is None:process.kill();process.wait(timeout=5)
        fixture.stdin.close()
        try:fixture.wait(timeout=5)
        except subprocess.TimeoutExpired:fixture.terminate()
        time.sleep(1)
        if 'temporary' in locals():temporary.cleanup()
if __name__=="__main__":main()
