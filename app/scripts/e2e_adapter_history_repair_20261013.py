import argparse
import hashlib
import json
import os
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument("--output", required=True)
parser.add_argument("--package", default="./internal/ledger")
parser.add_argument("--run", default="^TestAdapterHistoryRepair.*20261013$")
args = parser.parse_args()
owned = pathlib.Path("E:/CodexScratch/adapter-history-repair-20261013").resolve()
output = pathlib.Path(args.output).resolve()
if output == owned or owned not in output.parents:
    raise SystemExit("Owned history-repair evidence subdirectory required")
output.mkdir(parents=True, exist_ok=True)
temporary = output / "temp"
home = temporary / "profile"
home.mkdir(parents=True, exist_ok=True)
go = root.parent / ".tooling/go/bin/go.exe"
windows = pathlib.Path(os.environ.get("SystemRoot", "C:/Windows"))
environment = {
    "SystemRoot": str(windows), "WINDIR": str(windows),
    "COMSPEC": str(windows / "System32/cmd.exe"),
    "PATH": str(go.parent) + os.pathsep + str(windows / "System32"),
    "TEMP": str(temporary), "TMP": str(temporary), "USERPROFILE": str(home),
    "GOCACHE": str(owned / "go-cache"), "GOPATH": str(owned / "go-path"),
    "GOENV": "off", "GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off", "CGO_ENABLED": "0",
}
names = ["native/internal/ledger/adapters.go", "native/internal/ledger/store.go", "native/internal/api/server.go", "native/internal/ledger/adapter_history.go", "native/internal/ledger/adapter_history_repair_20261013_test.go", "scripts/e2e_adapter_history_repair_20261013.py", "scripts/adapter_history_repair_vectors_20261013.json", "docs/adapter-history-repair-contract-20261013.md"]
command = [str(go), "-C", "native", "test", args.package, "-run", args.run, "-count=1", "-v"]
result = {
    "sourceCommit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=root, text=True).strip(),
    "workingTree": bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=root, text=True).strip()),
    "sourceSHA256": {name: hashlib.sha256((root / name).read_bytes()).hexdigest() if (root / name).is_file() else None for name in names},
    "command": command, "environmentNames": sorted(environment),
    "environment": {"platform": sys.platform, "python": sys.version.split()[0]},
    "scope": "Actual offline Go journal fixtures and authored sizing/contract data only. No service, native/browser/Wiii/Core process, real history/account/profile, grant or provider.",
}
version = subprocess.run([str(go), "version"], cwd=root, env=environment, text=True, capture_output=True)
result["environment"]["go"] = version.stdout.strip()
run = subprocess.run(command, cwd=root, env=environment, text=True, capture_output=True)
(output / "stdout.txt").write_text(run.stdout, encoding="utf-8")
(output / "stderr.txt").write_text(run.stderr, encoding="utf-8")
result.update(status="PASS" if run.returncode == 0 else "FAIL", exitCode=run.returncode)
(output / "validation.json").write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
print(json.dumps(result, indent=2))
sys.exit(run.returncode)
