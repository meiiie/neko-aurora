import argparse
import hashlib
import json
import pathlib
import subprocess
import tempfile
import time
import sys
from e2e_core import ROOT,linux

def main():
    parser=argparse.ArgumentParser();parser.add_argument("--chrome",default=r"C:\Program Files\Google\Chrome\Application\chrome.exe");parser.add_argument("--tauri",action="store_true");parser.add_argument("--core",action="store_true");parser.add_argument("--native-core",action="store_true");parser.add_argument("--commit");args=parser.parse_args()
    head=subprocess.check_output(["git","rev-parse","HEAD"],cwd=ROOT,text=True).strip()
    if args.commit:assert head==args.commit and not subprocess.check_output(["git","status","--porcelain"],cwd=ROOT,text=True).strip()
    fixture=None;process=None;temporary=tempfile.TemporaryDirectory(prefix="neko-ui-fixture-",ignore_cleanup_errors=True)
    try:
        state=pathlib.Path(temporary.name)/"state";command=[str(ROOT/".build/neko-aurora.exe"),"--listen","127.0.0.1:8765" if args.tauri else "127.0.0.1:0","--state",str(state),"--host-executable",str(ROOT/"host/target/debug/neko-host.exe")]
        if args.core:
            fixture_command=[sys.executable,str(ROOT/"scripts/fixture_provider.py")] if args.native_core else ["wsl.exe","-d","Ubuntu","--exec","/usr/bin/python3",linux(ROOT/"scripts/fixture_provider.py")]
            fixture=subprocess.Popen(fixture_command,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True)
            ready=fixture.stdout.readline().strip();assert ready.startswith("READY ")
            config=pathlib.Path(temporary.name)/"fixture.json";config.write_text(json.dumps({"hostExecutable":str(ROOT/"host/target/debug/neko-host.exe"),"coreExecutable":str(ROOT/(".build/core-windows/neko.exe" if args.native_core else ".build/core-runtime/neko")),"supervisor":str(ROOT/"scripts/core_supervisor.py"),"fixtureEndpoint":ready[6:]}));command.extend(["--core-fixture-config",str(config)])
        process=subprocess.Popen(command,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True);ready=process.stdout.readline().strip();assert ready.startswith("READY ")
        ui=["node",str(ROOT/"scripts/ui_smoke.mjs"),str(ROOT/".build/neko-desktop.exe") if args.tauri else args.chrome,ready[6:]]
        if args.tauri:ui.append("--tauri")
        if args.core:ui.append("--core")
        subprocess.run(ui,cwd=ROOT,check=True,timeout=180)
        output=ROOT/".build/evidence/ui-smoke.json";report=json.loads(output.read_text());report.update({"commit":head,"workingTree":not bool(args.commit),"coreFixture":args.core,"nativeWindowsCore":args.core and args.native_core,"goBinarySha256":hashlib.sha256((ROOT/".build/neko-aurora.exe").read_bytes()).hexdigest(),"rustHostSha256":hashlib.sha256((ROOT/"host/target/debug/neko-host.exe").read_bytes()).hexdigest()})
        if args.core:report["coreSha256"]=hashlib.sha256((ROOT/(".build/core-windows/neko.exe" if args.native_core else ".build/core-runtime/neko")).read_bytes()).hexdigest()
        if args.tauri:report["tauriBinarySha256"]=hashlib.sha256((ROOT/".build/neko-desktop.exe").read_bytes()).hexdigest()
        output.write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding="utf-8")
        if args.commit:assert not subprocess.check_output(["git","status","--porcelain"],cwd=ROOT,text=True).strip()
    finally:
        if process and process.poll() is None:process.kill();process.wait(timeout=5)
        if fixture:
            fixture.stdin.close()
            try:fixture.wait(timeout=5)
            except subprocess.TimeoutExpired:fixture.terminate()
        time.sleep(1);temporary.cleanup()
if __name__=="__main__":main()
