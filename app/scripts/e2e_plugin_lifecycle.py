"""Owned compiled-Go HTTP/journal fixtures; no Core, host, provider or grants."""
import argparse
import base64
import concurrent.futures
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import queue
import subprocess
import threading
import time
import urllib.error
import urllib.request


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


class Service:
    def __init__(self, binary, state, root):
        self.binary, self.state, self.root = binary, state, root

    def start(self):
        env = {k: v for k, v in os.environ.items() if k.upper() in {"SYSTEMROOT", "WINDIR", "PATH", "TEMP", "TMP"}}
        self.process = subprocess.Popen([str(self.binary), "--state", str(self.state), "--listen", "127.0.0.1:0", "--host-executable", str(self.root / ".build" / "not-installed-host.exe")], cwd=self.root, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        lines = queue.Queue()
        threading.Thread(target=lambda: lines.put(self.process.stdout.readline()), daemon=True).start()
        line = lines.get(timeout=20).strip()
        if not line.startswith("READY http://127.0.0.1:"):
            raise AssertionError("owned Go service did not become ready")
        self.url = line[6:]
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.opener.open(self.url + "/", timeout=5).close()
        return self

    def stop(self):
        if self.process.poll() is None:
            self.process.kill()
        self.process.wait(timeout=10)

    def request(self, path, body=None, expected=200):
        raw = None if body is None else json.dumps(body, ensure_ascii=False).encode()
        req = urllib.request.Request(self.url + path, data=raw, headers={"Content-Type": "application/json", "Origin": self.url})
        try:
            with self.opener.open(req, timeout=10) as response:
                status, data = response.status, response.read()
        except urllib.error.HTTPError as error:
            status, data = error.code, error.read()
        if status != expected:
            raise AssertionError(f"unexpected HTTP status {path}: {status}, wanted {expected}")
        return json.loads(data) if status < 400 else None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", required=True)
    parser.add_argument("--go", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    commit = subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"], text=True).strip()
    if subprocess.check_output(["git", "-C", str(root), "status", "--porcelain"], text=True).strip():
        raise AssertionError("fixture requires immutable clean source")
    binary = Path(args.binary).resolve()
    info = subprocess.check_output([args.go, "version", "-m", str(binary)], text=True)
    if f"vcs.revision={commit}" not in info or "vcs.modified=false" not in info:
        raise AssertionError("fixture binary does not match clean code commit")
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=False)
    state = output / "scratch-state"
    service = Service(binary, state, root).start()
    results = []

    def passed(name, evidence):
        results.append({"name": name, "status": "PASS", "evidence": evidence})

    def lifecycle(action, revision, nonce, id="neko-artifacts", **extra):
        return {"version": 1, "id": id, "action": action, "expectedRevision": revision, "clientNonce": nonce, **extra}

    try:
        catalog = service.request("/v1/plugin-catalog")
        assert len(catalog["plugins"]) == 2 and all(p["trustedBuiltin"] and p["revision"] == 0 for p in catalog["plugins"])
        passed("builtin-catalog", {"count": 2, "revision": 0, "execution": catalog["execution"]})

        manifest = {"name": "Offline descriptor", "version": "1.0.0", "license": "unknown", "tools": [{"name": "artifact.write_text", "description": "Untrusted same-name tool", "inputSchema": {"type": "object"}}]}
        imported = lifecycle("import", 0, "http-plugin-import", "external-descriptor", manifest=manifest)
        plugin = service.request("/v1/plugin-lifecycle", imported)["plugin"]
        assert plugin["status"] == "manifest_only_disabled" and not plugin["trustedBuiltin"]
        update = lifecycle("update", 1, "http-plugin-update", plugin["id"], manifest={**manifest, "version": "2.0.0"})
        assert service.request("/v1/plugin-lifecycle", update)["plugin"]["revision"] == 2
        service.request("/v1/plugin-lifecycle", lifecycle("enable", 2, "http-external-enable", plugin["id"]), 501)
        service.request("/v1/plugin-lifecycle", {**update, "clientNonce": "http-external-command", "execute": "denied"}, 400)
        service.request("/v1/plugin-lifecycle", lifecycle("remove", 2, "http-plugin-remove", plugin["id"]))
        duplicate = service.request("/v1/plugin-lifecycle", imported)
        assert duplicate["duplicate"] and duplicate["plugin"]["status"] == "removed" and duplicate["plugin"]["revision"] == 3
        passed("external-lifecycle-fail-closed", {"importUpdateRemoveRevisions": [1, 2, 3], "enable": 501, "executableField": 400, "lostAckTombstone": True})

        body = {"botId": "bot-meiiie", "lane": "background", "text": "Original task", "clientNonce": "http-pinned-message", "pluginSelection": [{"id": "neko-artifacts", "expectedRevision": 0}]}
        work = service.request("/v1/messages", body, 201)["work"]
        empty = {"botId": "bot-davin", "lane": "foreground", "text": "Tool-free separate work", "clientNonce": "http-empty-message", "pluginSelection": []}
        other = service.request("/v1/messages", empty, 201)["work"]
        paused = service.request("/v1/messages", {**body, "clientNonce": "http-paused-message"}, 201)["work"]
        control = {"version": 1, "requestId": paused["requestId"], "expectedEpoch": paused["epoch"], "expectedRevision": 0, "action": "pause", "clientNonce": "http-plugin-pause"}
        paused = service.request("/v1/task-controls", control)["work"]
        assert work["toolPolicyVersion"] == 1 and work["pluginPins"][0]["revision"] == 0 and other["pluginSelection"] == [] and not other.get("pluginPins")
        passed("task-intake-pin-and-explicit-empty", {"requestId": work["requestId"], "otherRequestId": other["requestId"], "pausedRequestId": paused["requestId"]})

        disable = lifecycle("disable", 0, "http-plugin-disable")
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            responses = list(pool.map(lambda _: service.request("/v1/plugin-lifecycle", disable), range(2)))
        assert sorted(r["duplicate"] for r in responses) == [False, True]
        snapshot = service.request("/v1/snapshot")
        tasks = {w["requestId"]: w for w in snapshot["work"]}
        assert tasks[work["requestId"]]["status"] == "suspended" and tasks[other["requestId"]]["status"] == "waiting_host"
        service.request("/v1/task-controls", {**control, "action": "resume", "expectedEpoch": paused["epoch"], "expectedRevision": paused["taskRevision"], "clientNonce": "http-plugin-stale-resume"}, 409)
        service.request("/v1/messages", {**body, "clientNonce": "http-disabled-new"}, 409)
        passed("concurrent-nonce-and-queued-revocation", {"oneMutation": True, "oldSuspended": True, "otherContinues": True, "pausedResumeDenied": 409})

        service.request("/v1/plugin-lifecycle", lifecycle("enable", 1, "http-plugin-enable"))
        duplicate = service.request("/v1/messages", body)
        assert duplicate["duplicate"] and duplicate["work"]["requestId"] == work["requestId"] and duplicate["work"]["pluginPins"][0]["revision"] == 0 and duplicate["work"]["status"] == "suspended"
        service.request("/v1/messages", {**body, "pluginSelection": [{"id": "neko-artifacts", "expectedRevision": 2}]}, 409)
        service.request("/v1/context-refresh", {"requestId": work["requestId"], "expectedEpoch": tasks[work["requestId"]]["epoch"]}, 409)
        fresh = service.request("/v1/messages", {**body, "clientNonce": "http-fresh-message", "pluginSelection": [{"id": "neko-artifacts", "expectedRevision": 2}]}, 201)["work"]
        assert fresh["pluginPins"][0]["revision"] == 2
        passed("reenable-never-rebinds-old-task", {"oldPin": 0, "newPin": 2, "oldRefresh": 409, "changedRetry": 409})

        channel = service.request("/v1/channels", {"name": "Sources fixture", "botIds": ["bot-meiiie", "bot-davin"], "clientNonce": "http-plugin-channel"})["channel"]
        project = service.request("/v1/projects", {"name": "Source policy", "goal": "Read only pinned sources", "channelIds": [channel["id"]], "clientNonce": "http-plugin-project"})["project"]
        content = b"plugin-source-http-fixture"
        file = service.request("/v1/uploads", {"projectId": project["id"], "projectRevision": project["revision"], "channelId": channel["id"], "filename": "fixture.md", "contentBase64": base64.b64encode(content).decode(), "clientNonce": "http-plugin-file"})["file"]
        scoped = {"botId": "bot-meiiie", "lane": "background", "text": "Read selected fixture", "clientNonce": "http-plugin-source-message", "channelId": channel["id"], "expectedChannelRevision": channel["revision"], "projectId": project["id"], "projectRevision": project["revision"], "fileIds": [file["id"]]}
        source_work = service.request("/v1/messages", scoped, 201)["work"]
        assert any(p["id"] == "neko-project-sources" and p["revision"] == 0 for p in source_work["pluginPins"])
        service.request("/v1/messages", {**scoped, "clientNonce": "http-source-empty", "pluginSelection": []}, 409)
        service.request("/v1/plugin-lifecycle", lifecycle("disable", 0, "http-reader-disable", "neko-project-sources"))
        sequence = service.request("/v1/snapshot")["sequence"]
        service.request("/v1/messages", {**scoped, "clientNonce": "http-source-disabled"}, 409)
        assert service.request("/v1/snapshot")["sequence"] == sequence
        assert sha(state / "uploads" / file["id"]) == hashlib.sha256(content).hexdigest()
        passed("source-inline-ingestion-cannot-bypass-reader", {"missingReader": 409, "disabledDefaultReader": 409, "precommitSequenceStable": True, "ownedUploadHash": file["sha256"]})

        before = service.request("/v1/snapshot")["sequence"]
        service.request("/v1/messages", {"botId": "bot-davin", "lane": "foreground", "text": "\\" * 32000, "clientNonce": "http-plugin-large"}, 400)
        service.request("/v1/messages", {**empty, "clientNonce": "http-plugin-missing-cas", "pluginSelection": [{"id": "neko-artifacts"}]}, 400)
        assert service.request("/v1/snapshot")["sequence"] == before
        passed("encoded-bound-and-required-selection-cas", {"oversizedPrompt": 400, "missingSelectionRevision": 400, "precommitSequenceStable": True})

        for _ in range(3):
            service.stop()
            service.start()
            duplicate = service.request("/v1/plugin-lifecycle", disable)
            assert duplicate["duplicate"] and duplicate["plugin"]["revision"] == 2 and duplicate["plugin"]["status"] == "ready_builtin_local"
            duplicate = service.request("/v1/messages", body)
            assert duplicate["duplicate"] and duplicate["work"]["requestId"] == work["requestId"] and duplicate["work"]["status"] == "suspended"
            assert duplicate["work"]["pluginPins"][0]["revision"] == 0
        passed("hardkill-restart3-lost-ack-no-revival", {"restarts": 3, "originalTaskIdentity": work["requestId"], "currentCatalogRevision": 2, "oldPin": 0})
    except BaseException as error:
        results.append({"name": "fixture-execution", "status": "FAIL", "error": str(error)})
        raise
    finally:
        service.stop()
        report = {"codeCommit": commit, "workingTree": False, "sourceRootCommit": "931c4b76eb82bc92b1ca2c80a89ab04112c0d5db", "binarySha256": sha(binary), "goBuildInfo": info, "command": [str(Path(__file__)), *os.sys.argv[1:]], "environment": {"python": os.sys.version.split()[0], "os": os.name, "providerCredentials": "not inherited", "hostCore": "not launched"}, "scope": "actual compiled Go HTTP and durable journal only; no native stop proof or model evaluation", "results": results, "pass": sum(r["status"] == "PASS" for r in results), "fail": sum(r["status"] == "FAIL" for r in results)}
        (output / "results.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
        print(json.dumps({"commit": commit, "pass": report["pass"], "fail": report["fail"]}))


if __name__ == "__main__":
    main()
