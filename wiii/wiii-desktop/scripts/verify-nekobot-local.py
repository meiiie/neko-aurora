"""Actual restored Wiii modules -> injected owner IPC -> Go/Rust/Core owned loopback fixture.
No Wiii native bootstrap, browser cookie access, production state or real provider.
"""
import argparse
import contextlib
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile
import urllib.error


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--server-root", required=True, type=pathlib.Path)
    parser.add_argument("--server-commit", required=True)
    parser.add_argument("--wiii-commit", required=True)
    parser.add_argument("--overlay-root", required=True, type=pathlib.Path)
    parser.add_argument("--bun", required=True, type=pathlib.Path)
    parser.add_argument("--output", required=True, type=pathlib.Path)
    parser.add_argument("--large-input", action="store_true")
    args = parser.parse_args()
    server = args.server_root.resolve()
    overlay = args.overlay_root.resolve()
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=server, text=True).strip()
    assert head == args.server_commit
    assert not subprocess.check_output(["git", "status", "--porcelain"], cwd=server)
    assert subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=overlay, text=True).strip() == args.wiii_commit
    assert not subprocess.check_output(["git", "status", "--porcelain"], cwd=overlay)
    source = pathlib.Path(__file__).resolve().parents[1]
    ipc = source / "scripts/verify-nekobot-local-ipc.ts"
    assert ipc.read_bytes() == subprocess.check_output(["git", "show", args.wiii_commit + ":wiii-desktop/scripts/verify-nekobot-local-ipc.ts"], cwd=overlay)
    sys.path.insert(0, str(server / "scripts"))
    from e2e_core import Client
    env = {k: v for k, v in os.environ.items() if not re.search(r"(?i)(api.?key|token|password|secret|credential)", k)}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    owned = pathlib.Path(tempfile.mkdtemp(prefix="wiii-owned-bridge-", dir=args.output.parent)).resolve()
    assert owned.is_relative_to(args.output.parent.resolve())
    logs = []
    fixture_log = (owned / "fixture-stderr.txt").open("w", encoding="utf8")
    logs.append(fixture_log)
    fixture = subprocess.Popen([sys.executable, str(server / "scripts/fixture_provider.py")], stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE, stderr=fixture_log, text=True, env=env)
    process = None
    clients = []
    calls = []
    checks = []
    results = []
    def launch(state, config):
        nonlocal process
        server_log = (owned / ("go-stderr-" + str(len(logs)) + ".txt")).open("w", encoding="utf8")
        logs.append(server_log)
        process = subprocess.Popen([str(server / ".build/neko-control.exe"), "--listen", "127.0.0.1:0", "--state", str(state),
                                    "--core-fixture-config", str(config)], stdin=subprocess.DEVNULL,
                                   stdout=subprocess.PIPE, stderr=server_log, text=True, env=env)
        ready = process.stdout.readline().strip()
        assert ready.startswith("READY "), "Owned Go service failed to start"
        return process, Client(ready[6:])
    try:
        ready = fixture.stdout.readline().strip()
        assert ready.startswith("READY ")
        with contextlib.nullcontext(owned):
            state = owned / "server-state"
            storage = owned / "wiii-storage.json"
            config = owned / "fixture-config.json"
            config.write_text(json.dumps({"hostExecutable": str(server / "host/target/debug/neko-host.exe"),
                                          "coreExecutable": str(server / ".build/core-windows/neko.exe"),
                                          "supervisor": str(server / "scripts/core_supervisor.py"),
                                          "fixtureEndpoint": ready[6:]}), encoding="utf8")
            process, owner = launch(state, config)
            private = owner.send("[fixture:foreground] private-Davin-Wiii-bridge-canary", bot="bot-davin")
            owner.wait(private["requestId"], lambda w: w["status"] == "completed")
            channel = owner.request("/v1/channels", {"name": "Wiii owned channel", "botIds": ["bot-meiiie", "bot-davin"], "clientNonce": "wiii-owned-create-channel"})["channel"]
            binding = {"sourceApp": "wiii", "sourceSessionId": "wiii-owned-session", "sourceThreadId": "wiii-owned-thread",
                       "channelId": channel["id"], "expectedChannelRevision": channel["revision"],
                       "ownerUserId": "local-owner", "spaceId": "local-space"}
            dropped = False

            def run(phase):
                nonlocal dropped
                child_log_path = owned / ("wiii-" + phase + "-stderr.txt")
                child_log = child_log_path.open("w", encoding="utf8")
                logs.append(child_log)
                child = subprocess.Popen([str(args.bun), str(ipc)], cwd=source, stdin=subprocess.PIPE,
                                         stdout=subprocess.PIPE, stderr=child_log, text=True, encoding="utf8", env=env)
                clients.append(child)
                child.stdin.write(json.dumps({"op": "config", "value": {"source": binding, "phase": phase, "storagePath": str(storage), "largeInput": args.large_input}}) + "\n")
                child.stdin.flush()
                for line in child.stdout:
                    item = json.loads(line)
                    if item["op"] == "result":
                        results.append(item)
                        checks.extend(item["checks"])
                        child.wait(timeout=5)
                        assert child.returncode == 0, child_log_path.read_text(encoding="utf8")
                        return item
                    assert item["op"] == "request"
                    path = item["relativePath"]
                    assert path == "/v1/adapters/sessions" or re.fullmatch(r"/v1/adapters/sessions/adapter-[a-f0-9]{32}/(?:messages|replies\?after=[0-9]+)", path)
                    assert item["method"] == ("GET" if "/replies?" in path else "POST")
                    body = item.get("body")
                    calls.append({"phase": phase, "method": item["method"], "path": path,
                                  "sourceMessageId": body.get("sourceMessageId") if body else None})
                    try:
                        response = owner.request(path, body)
                        value = {"status": 200, "body": json.dumps(response, ensure_ascii=False)}
                        if body and body.get("sourceMessageId") == "wiii-live-lost-ack" and not dropped:
                            dropped = True
                            value = {"status": 503, "body": "owned fixture intentionally loses ACK after commit"}
                    except urllib.error.HTTPError as error:
                        value = {"status": error.code, "body": "owned local HTTP rejection"}
                    child.stdin.write(json.dumps({"op": "response", "id": item["id"], "value": value}, ensure_ascii=False) + "\n")
                    child.stdin.flush()
                child.wait(timeout=5)
                raise AssertionError("Wiii client exited before result: " + child_log_path.read_text(encoding="utf8"))

            first = run("send")
            work = {w["requestId"]: w for w in owner.request("/v1/snapshot")["work"]}
            primary_id = next(m["requestId"] for m in first["mappings"] if m["sourceMessageId"] == "wiii-live-primary")
            inspect_id = next(m["requestId"] for m in first["mappings"] if m["sourceMessageId"] == "wiii-live-lost-ack")
            primary, inspection = work[primary_id], work[inspect_id]
            if args.large_input:
                assert 8192 < len(primary["text"].encode("utf8")) < 32768
                assert primary["text"].endswith("\nWiii owned initial frame boundary. " * 256)
                checks.append({"name": "actual_Go_preserves_large_Wiii_primary_input_through_completed_Core_artifact", "pass": True})
            assert all(w["conversationId"] == channel["id"] and w["publication"]["replyChannelId"] == channel["id"] for w in [primary, inspection])
            source_ids = [s["requestId"] for s in inspection["contextPin"]["sources"]]
            if args.large_input:
                assert source_ids == [] and inspection["contextPin"]["omitted"] == 1, "large original exceeds the unchanged 8000-byte source pin budget"
            else:
                assert source_ids == [primary_id], "small eligible shared-channel source must be pinned"
            assert private["requestId"] not in inspection["reply"]
            assert sum(c["sourceMessageId"] == "wiii-live-lost-ack" for c in calls) == 1
            checks.append({"name": "actual_Core_explicitly_omits_oversized_shared_source_without_private_history" if args.large_input
                                   else "actual_Core_context_excludes_target_private_history_and_keeps_pinned_channel", "pass": True})
            artifact = primary["artifacts"][0]
            data, headers = owner.request("/v1/channels/" + channel["id"] + "/artifacts/" + artifact["id"])
            assert hashlib.sha256(data).hexdigest() == artifact["sha256"] == headers["X-Artifact-SHA256"]
            checks.append({"name": "actual_bridge_result_artifact_hash_matches_scoped_Go_download", "pass": True})
            wiii_saved = storage.read_text(encoding="utf8")
            assert all(w["core"]["sessionId"] not in wiii_saved for w in [primary, inspection])
            checks.append({"name": "Wiii_durable_adapter_state_does_not_store_Core_session_or_runtime_receipts", "pass": True})
            process.kill()
            process.wait(timeout=5)
            process, owner = launch(state, config)
            run("replay")
            owner.request("/v1/channels", {"id": channel["id"], "name": channel["name"], "botIds": ["bot-meiiie"],
                                          "expectedRevision": channel["revision"], "clientNonce": "wiii-owned-remove-recipient"})
            run("revoked")
        report = {"wiiiCommit": args.wiii_commit, "serverCommit": args.server_commit, "workingTree": False,
                  "scope": "Restored Wiii strict storage/client/handoff/projector source -> owner-injected IPC -> actual Go/Rust/Core owned loopback fixture. No Wiii native bootstrap/GUI, browser cookie access, real provider, cloud, phone, external grant or production state.",
                  "checks": checks, "calls": calls, "phases": results, "status": "PASS", "ownedJournal": str(owned),
                  "command": sys.argv, "largeInput": args.large_input, "primaryInputUtf8Bytes": len(primary["text"].encode("utf8")),
                  "primaryInputSha256": hashlib.sha256(primary["text"].encode("utf8")).hexdigest()}
        for key, path in [("go", server / ".build/neko-control.exe"), ("host", server / "host/target/debug/neko-host.exe"), ("core", server / ".build/core-windows/neko.exe")]:
            report[key + "Sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf8")
        print(json.dumps({"pass": len(checks), "fail": 0, "wiiiCommit": args.wiii_commit, "serverCommit": args.server_commit}))
    except BaseException as error:
        args.output.write_text(json.dumps({"wiiiCommit": args.wiii_commit, "serverCommit": args.server_commit,
                                          "workingTree": False, "status": "FAIL", "ownedJournal": str(owned),
                                          "reason": str(error), "checks": checks, "calls": calls, "phases": results},
                                         ensure_ascii=False, indent=2) + "\n", encoding="utf8")
        raise
    finally:
        for child in clients:
            if child.poll() is None:
                child.kill()
                child.wait(timeout=5)
        if process and process.poll() is None:
            process.kill()
            process.wait(timeout=5)
        fixture.stdin.close()
        try:
            fixture.wait(timeout=5)
        except subprocess.TimeoutExpired:
            fixture.kill()
            fixture.wait(timeout=5)
        for log in logs:
            log.close()


if __name__ == "__main__":
    main()
