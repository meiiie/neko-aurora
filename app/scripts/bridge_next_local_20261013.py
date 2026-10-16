import argparse
import hashlib
import json
import os
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
args = argparse.ArgumentParser()
args.add_argument("--output", required=True)
args.add_argument("--run", default="^TestAdapterIncrementalFileRevocationClearsConsumedReply$")
options = args.parse_args()
output = pathlib.Path(options.output).resolve()
owned = pathlib.Path("E:/CodexScratch/bridge-next-local-20261013").resolve()
if output == owned or owned not in output.parents:
    raise SystemExit("Owned E bridge evidence subdirectory required")
output.mkdir(parents=True, exist_ok=True)
cache = owned / "go-cache"
temporary = output / "temp"
temporary.mkdir(exist_ok=True)
profile_home = temporary / "profile"
profile_home.mkdir(exist_ok=True)
go = root.parent / ".tooling/go/bin/go.exe"
system_root = os.environ.get("SystemRoot", "C:/Windows")
environment = {
    "SystemRoot": system_root,
    "WINDIR": system_root,
    "COMSPEC": str(pathlib.Path(system_root) / "System32/cmd.exe"),
    "PATH": str(go.parent) + os.pathsep + str(pathlib.Path(system_root) / "System32"),
    "TEMP": str(temporary),
    "TMP": str(temporary),
    "USERPROFILE": str(profile_home),
    "GOCACHE": str(cache),
    "GOPATH": str(owned / "go-path"),
    "GOENV": "off",
    "GOTOOLCHAIN": "local",
    "GOPROXY": "off",
    "GOSUMDB": "off",
    "CGO_ENABLED": "0",
}
command = [str(go), "-C", "native", "test", "./internal/ledger", "-run", options.run, "-count=1", "-v"]
files = ["native/internal/ledger/adapters.go", "native/internal/ledger/store.go", "native/internal/ledger/adapter_invalidation_20261013_test.go", "scripts/bridge_next_local_20261013.py"]
result = {
    "sourceCommit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=root, text=True).strip(),
    "workingTree": bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=root, text=True).strip()),
    "sourceSHA256": {name: hashlib.sha256((root / name).read_bytes()).hexdigest() for name in files},
    "command": command,
    "environmentNames": sorted(environment),
    "environment": {"platform": sys.platform, "python": sys.version.split()[0]},
    "scope": "Targeted real Go ledger fixtures in owned scratch. No Go service, Core, native UI, Wiii process, provider, external credential or grant.",
}
if not go.is_file():
    result.update(status="BLOCKED", reason="Bundled offline Go tool missing")
    code = 2
else:
    version = subprocess.run([str(go), "version"], cwd=root, env=environment, capture_output=True, text=True)
    result["environment"]["go"] = version.stdout.strip()
    run = subprocess.run(command, cwd=root, env=environment, capture_output=True, text=True)
    (output / "stdout.txt").write_text(run.stdout, encoding="utf-8")
    (output / "stderr.txt").write_text(run.stderr, encoding="utf-8")
    result.update(status="PASS" if run.returncode == 0 else "FAIL", exitCode=run.returncode)
    code = run.returncode
(output / "validation.json").write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
print(json.dumps(result, indent=2))
sys.exit(code)
