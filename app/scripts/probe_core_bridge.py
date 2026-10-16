import json
import pathlib
import subprocess
import sys
import tempfile

root = pathlib.Path(__file__).resolve().parents[1]
def linux(path):
    path = pathlib.Path(path).resolve()
    return "/mnt/" + path.drive[0].lower() + "/" + path.as_posix()[3:]
native="--native-core" in sys.argv
fixture_command=[sys.executable,str(root/"scripts/fixture_provider.py")] if native else ["wsl.exe", "-d", "Ubuntu", "--exec", "/usr/bin/python3", linux(root / "scripts/fixture_provider.py")]
fixture = subprocess.Popen(fixture_command, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
try:
    ready = fixture.stdout.readline().strip()
    if not ready.startswith("READY "):
        raise RuntimeError("fixture unavailable")
    with tempfile.TemporaryDirectory(prefix="core-probe-") as state:
        process = subprocess.Popen([str(root / "host/target/debug/neko-host.exe"), "--runtime", str(root / (".build/core-windows/neko.exe" if native else ".build/core-runtime/neko")), "--supervisor", str(root / "scripts/core_supervisor.py"), "--state-root", state, "--fixture-endpoint", ready[6:]], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8")
        process.stdin.write(json.dumps({"requestId": "req-" + "1" * 32, "text": "[fixture:foreground] Xin chào Neko"}) + "\n")
        process.stdin.flush()
        for line in process.stdout:
            event = json.loads(line)
            print(json.dumps(event, ensure_ascii=False), flush=True)
            process.stdin.write(json.dumps({"sequence": event["sequence"], "ok": True, "result": {}}) + "\n")
            process.stdin.flush()
        error = process.stderr.read()
        print("EXIT", process.wait(), error, flush=True)
finally:
    fixture.stdin.close()
    try:
        fixture.wait(timeout=5)
    except subprocess.TimeoutExpired:
        fixture.terminate()
