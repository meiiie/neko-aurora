import argparse
import hashlib
import json
import pathlib
import subprocess
import tempfile
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--chrome", default=r"C:\Program Files\Google\Chrome\Application\chrome.exe")
    parser.add_argument("--host", default=str(ROOT / ".build/neko-host.exe"))
    parser.add_argument("--commit")
    parser.add_argument("--context-fixture", action="store_true")
    parser.add_argument("--core", action="store_true")
    parser.add_argument("--tauri", action="store_true")
    args = parser.parse_args()
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    dirty = bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT, text=True).strip())
    if args.commit:
        assert head == args.commit and not dirty
    with tempfile.TemporaryDirectory(prefix="neko-ui-redesign-", dir=ROOT / ".build", ignore_cleanup_errors=True) as temporary:
        fixture = None
        command = [str(ROOT / ".build/neko-aurora.exe"), "--listen", "127.0.0.1:8765" if args.tauri else "127.0.0.1:0", "--state", str(pathlib.Path(temporary)/"state"), "--host-executable", args.host]
        if args.core:
            fixture = subprocess.Popen([sys.executable,str(ROOT/"scripts/fixture_provider.py")],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True)
            ready = fixture.stdout.readline().strip()
            assert ready.startswith("READY ")
            config = pathlib.Path(temporary)/"fixture.json"
            config.write_text(json.dumps({"hostExecutable":args.host,"coreExecutable":str(ROOT/".build/core-windows/neko.exe"),"supervisor":str(ROOT/"scripts/core_supervisor.py"),"fixtureEndpoint":ready[6:]}),encoding="utf-8")
            command.extend(["--core-fixture-config",str(config)])
        service = subprocess.Popen(command, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            ready = service.stdout.readline().strip()
            assert ready.startswith("READY "), "Owned local service did not start: " + service.stderr.read()
            subprocess.run(["node", str(ROOT / "scripts/ui_redesign_smoke.mjs"), str(ROOT/".build/neko-desktop.exe") if args.tauri else args.chrome, ready[6:]] + (["--context-fixture"] if args.context_fixture else []) + (["--core"] if args.core else []) + (["--tauri"] if args.tauri else []), cwd=ROOT, check=True, timeout=240)
            evidence = ROOT / ".build/evidence-redesign/ui-smoke.json"
            report = json.loads(evidence.read_text(encoding="utf-8"))
            report.update({"codeCommit": head, "workingTree": dirty, "command": "python scripts/test_ui_redesign.py" + (" --commit " + args.commit if args.commit else "") + (" --context-fixture" if args.context_fixture else ""), "environment": "Windows; owned isolated headless Chrome; Go local journal; existing Rust DPAPI host; no Core/model execution", "sourceUiBaseline": "3cb9b6ff12195b432e1c5c9d69b0db9b56699229", "runtimeCodeCommit": head, "goBinarySha256": hashlib.sha256((ROOT / ".build/neko-aurora.exe").read_bytes()).hexdigest(), "rustHostSha256": hashlib.sha256(pathlib.Path(args.host).read_bytes()).hexdigest()})
            report["contextFixture"] = "owned CDP endpoint/snapshot stub only; no real context grant or provider" if args.context_fixture else "not_run"
            report["nativeWindowsCore"] = args.core
            report["environment"] = "Windows; owned Tauri WebView2" if args.tauri else "Windows; owned headless Chrome"
            report["environment"] += "; compiled Go journal; Rust DPAPI; native Core loopback fixture" if args.core else "; compiled Go journal; Rust DPAPI; no Core/model execution"
            report["command"] += (" --core" if args.core else "") + (" --tauri" if args.tauri else "")
            if args.core: report["coreSha256"] = hashlib.sha256((ROOT/".build/core-windows/neko.exe").read_bytes()).hexdigest()
            if args.tauri: report["tauriBinarySha256"] = hashlib.sha256((ROOT/".build/neko-desktop.exe").read_bytes()).hexdigest()
            report["assetSha256"] = {str(p.relative_to(ROOT / "native/internal/web/assets")): hashlib.sha256(p.read_bytes()).hexdigest() for p in (ROOT / "native/internal/web/assets").rglob("*") if p.is_file()}
            evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        finally:
            if service.poll() is None:
                service.kill()
                service.wait(timeout=5)
            if fixture:
                fixture.stdin.close()
                try: fixture.wait(timeout=5)
                except subprocess.TimeoutExpired: fixture.terminate(); fixture.wait(timeout=5)


if __name__ == "__main__":
    main()
