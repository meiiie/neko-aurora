"""Owned local ledger fixtures; no provider, host, Core, browser or cloud acceptance."""

import argparse
import hashlib
import json
import os
import pathlib
import platform
import subprocess
import sys
import time

ROOT = pathlib.Path(__file__).resolve().parents[1]
SCRATCH = pathlib.Path("E:/CodexScratch/group-context-20261013").resolve()


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", type=pathlib.Path, required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--run", default="^TestGroupContext")
    args = parser.parse_args()
    output = args.output.resolve()
    if output == SCRATCH or not output.is_relative_to(SCRATCH):
        parser.error("output must be an owned child of E:/CodexScratch/group-context-20261013")
    output.mkdir(parents=True, exist_ok=False)
    tmp = output / "tmp"
    tmp.mkdir()
    allowed = ("SYSTEMROOT", "WINDIR", "PATH", "PATHEXT", "COMSPEC", "PROCESSOR_ARCHITECTURE", "NUMBER_OF_PROCESSORS")
    env = {key: os.environ[key] for key in allowed if key in os.environ}
    env.update({"GOCACHE": "E:/CodexScratch/core-auth-bridge-20261012/root/gocache", "GOTMPDIR": str(tmp), "TEMP": str(tmp), "TMP": str(tmp), "USERPROFILE": str(tmp), "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOENV": "off", "CGO_ENABLED": "0"})
    command = [str(args.go.resolve()), "test", "-json", "./internal/ledger", "-run", args.run, "-count=1"]
    paths = [pathlib.Path("scripts/e2e_group_context_20261013.py")]
    paths.extend(path.relative_to(ROOT) for path in sorted((ROOT / "native").rglob("*.go")))
    hashes = {path.as_posix(): sha256(ROOT / path) for path in paths}
    digest = hashlib.sha256(json.dumps(hashes, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    report = {"scope": __doc__, "commit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(), "workingTree": subprocess.check_output(["git", "status", "--short"], cwd=ROOT, text=True).splitlines(), "command": command, "cwd": str(ROOT / "native"), "environment": {"platform": platform.platform(), "python": sys.version.split()[0], "go": subprocess.check_output([str(args.go.resolve()), "version"], env=env, text=True).strip(), "safeEnvironmentKeys": sorted(env), "GOENV": "off", "GOPROXY": "off", "GOSUMDB": "off", "CGO_ENABLED": "0"}, "sourceSHA256": hashes, "sourceSetSHA256": digest}
    started = time.monotonic()
    result = subprocess.run(command, cwd=ROOT / "native", env=env, capture_output=True, text=True, encoding="utf-8", timeout=180)
    (output / "go-test.jsonl").write_text(result.stdout, encoding="utf-8")
    (output / "go-test.stderr.txt").write_text(result.stderr, encoding="utf-8")
    events = [json.loads(line) for line in result.stdout.splitlines() if line.startswith("{")]
    report.update({"status": "PASS" if result.returncode == 0 else "FAIL", "exitCode": result.returncode, "elapsedSeconds": round(time.monotonic() - started, 3), "passedTests": [event["Test"] for event in events if event.get("Action") == "pass" and "Test" in event], "failedTests": [event["Test"] for event in events if event.get("Action") == "fail" and "Test" in event]})
    (output / "results.json").write_text(json.dumps(report, indent=2), encoding="utf-8")
    for event in events:
        if event.get("Action") == "output" and ("missing after repeated publication" in event.get("Output", "") or "FAIL" in event.get("Output", "")):
            print(event["Output"].rstrip())
    print(json.dumps({"status": report["status"], "commit": report["commit"], "sourceSetSHA256": digest, "passedTests": report["passedTests"], "failedTests": report["failedTests"], "results": str(output / "results.json")}, indent=2))
    return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
