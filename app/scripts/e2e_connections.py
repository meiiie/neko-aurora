import argparse
import hashlib
import json
import pathlib
import subprocess
import tempfile
import urllib.error
from e2e_core import Client,ROOT

def launch(state):
    process=subprocess.Popen([str(ROOT/".build/neko-aurora.exe"),"--listen","127.0.0.1:0","--state",str(state),"--host-executable",str(ROOT/"host/target/debug/neko-host.exe")],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    ready=process.stdout.readline().strip()
    if not ready.startswith("READY "):raise RuntimeError("Owned fixture service unavailable")
    return process,Client(ready[6:])
def conflict(client,path,body):
    try:client.request(path,body)
    except urllib.error.HTTPError as error:assert error.code==409;return
    raise AssertionError("Conflicting mutation was accepted")
def main():
    parser=argparse.ArgumentParser();parser.add_argument("--commit");parser.add_argument("--output",type=pathlib.Path);args=parser.parse_args()
    head=subprocess.check_output(["git","rev-parse","HEAD"],cwd=ROOT,text=True).strip()
    if args.commit:assert head==args.commit and not subprocess.check_output(["git","status","--porcelain"],cwd=ROOT,text=True).strip()
    process=None;checks=[];temporary=tempfile.TemporaryDirectory(prefix="neko-vault-e2e-",ignore_cleanup_errors=True)
    try:
        state=pathlib.Path(temporary.name)/"state";process,client=launch(state)
        secret="fixture-only-generated-"+hashlib.sha256(temporary.name.encode()).hexdigest()
        first={"id":"","expectedRevision":0,"clientNonce":"fixture-account-one","name":"Tài khoản thử một","provider":"openai-compatible","endpoint":"https://example.invalid/v1","model":"fixture-model","secret":secret}
        result=client.request("/v1/accounts",first);account=result["account"]
        assert account["hasSecret"] and account["status"]=="stored_unverified" and not result["duplicate"]
        duplicate=client.request("/v1/accounts",first);assert duplicate["duplicate"] and duplicate["account"]["id"]==account["id"]
        changed={**first,"secret":secret+"changed"};conflict(client,"/v1/accounts",changed)
        checks.append({"name":"real_windows_dpapi_seal_duplicate_and_changed_secret_conflict","pass":True})
        second={**first,"clientNonce":"fixture-account-two","name":"Tài khoản thử hai","secret":secret+"second"};client.request("/v1/accounts",second)
        snapshot=client.request("/v1/connections");assert len(snapshot["accounts"])==2 and snapshot["providerExecution"]=="deferred"
        assert secret not in json.dumps(snapshot) and "secretRef" not in json.dumps(snapshot)
        for file in state.rglob("*"):
            if file.name=="writer.lock":assert file.stat().st_size==0;continue
            if file.is_file():assert secret.encode() not in file.read_bytes(),"Plaintext secret persisted"
        checks.append({"name":"multiple_accounts_no_plaintext_in_disk_or_public_projection","pass":True})
        plugin={"id":"fixture-reader","name":"Fixture Reader","version":"1.0.0","license":"MIT","tools":[{"name":"report","description":"Read a report","inputSchema":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":False}}]}
        saved=client.request("/v1/plugins",plugin);assert saved["status"]=="manifest_only_disabled"
        checks.append({"name":"plugin_manifest_saved_without_execution_grant","pass":True})
        process.kill();process.wait(timeout=5);process,client=launch(state)
        again=client.request("/v1/accounts",first);assert again["duplicate"] and again["account"]["id"]==account["id"]
        conflict(client,"/v1/accounts/remove",{"id":account["id"],"expectedRevision":0})
        client.request("/v1/accounts/remove",{"id":account["id"],"expectedRevision":1})
        assert len(client.request("/v1/connections")["accounts"])==1
        checks.append({"name":"restart_retains_account_binding_and_stale_remove_rejected","pass":True})
        report={"commit":head,"workingTree":not bool(args.commit),"scope":"Actual Windows Rust DPAPI and local Go API; fixture secrets only, no provider network requests or grants","checks":checks}
        if args.output:args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding="utf-8")
        print(json.dumps(report,ensure_ascii=False,indent=2))
    finally:
        if process and process.poll() is None:process.kill();process.wait(timeout=5)
        temporary.cleanup()
if __name__=="__main__":main()
