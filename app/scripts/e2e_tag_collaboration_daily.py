"""Genuine finite Core collaboration with explicit binaries and owned loopback state."""
import argparse
import base64
import ctypes
from ctypes import wintypes
import hashlib
import json
import os
import pathlib
import platform
import subprocess
import sys
import time
import urllib.error

from e2e_core import Client


ROOT = pathlib.Path(__file__).resolve().parents[1]
SCRATCH = pathlib.Path("E:/CodexScratch/local-daily-use-20261012/native-tag-worker").resolve()
HOST_HASH = "5beef30a02ac26768d26a90cdd9c4c1ed55ec99edd9687358d08a5ae22ae1423"
CORE_HASH = "97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb"


def digest(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


class CoreHandle:
    def __init__(self, pid, expected):
        self.kernel = ctypes.WinDLL("kernel32", use_last_error=True)
        self.kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
        self.kernel.OpenProcess.restype = wintypes.HANDLE
        self.kernel.QueryFullProcessImageNameW.argtypes = [wintypes.HANDLE, wintypes.DWORD, wintypes.LPWSTR, ctypes.POINTER(wintypes.DWORD)]
        self.kernel.WaitForSingleObject.argtypes = [wintypes.HANDLE, wintypes.DWORD]
        self.kernel.WaitForSingleObject.restype = wintypes.DWORD
        self.kernel.CloseHandle.argtypes = [wintypes.HANDLE]
        self.handle = self.kernel.OpenProcess(0x101000, False, pid)
        assert self.handle, "Owned Core handle unavailable"
        image = ctypes.create_unicode_buffer(32768)
        length = wintypes.DWORD(len(image))
        assert self.kernel.QueryFullProcessImageNameW(self.handle, 0, image, ctypes.byref(length))
        assert pathlib.Path(image.value).resolve() == expected.resolve(), "Unexpected Core executable"
        assert self.kernel.WaitForSingleObject(self.handle, 0) == 258, "Core was not live at retained-handle capture"
        self.pid = pid

    def stopped(self, timeout_ms=0):
        return self.kernel.WaitForSingleObject(self.handle, timeout_ms) == 0

    def close(self):
        self.kernel.CloseHandle(self.handle)


def main():
    parser = argparse.ArgumentParser()
    for name in ["go", "host", "core", "go-tool", "build-receipt", "output"]:
        parser.add_argument("--" + name, required=True, type=pathlib.Path)
    parser.add_argument("--commit", required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    assert output != SCRATCH and output.is_relative_to(SCRATCH), "Output must be an owned child of native-tag-worker"
    output.mkdir(parents=True, exist_ok=False)
    private = output / "private-fixture"
    private.mkdir()
    receipt = json.loads(args.build_receipt.read_text(encoding="utf-8-sig"))
    assert receipt["sourceCommit"] == args.commit and receipt["workingTree"] is False and receipt["exitCode"] == 0
    assert pathlib.Path(receipt["command"][receipt["command"].index("-o") + 1]).resolve() == args.go.resolve()
    build_info = subprocess.check_output([str(args.go_tool), "version", "-m", str(args.go)], text=True)
    assert "vcs.revision=" + args.commit in build_info and "vcs.modified=false" in build_info
    assert digest(args.host) == HOST_HASH and digest(args.core) == CORE_HASH
    dependencies = ["scripts/e2e_core.py", "scripts/collaboration_fixture.py", "scripts/fixture_provider.py", "scripts/core_supervisor.py"]
    checks = []
    report = {
        "status": "RUNNING", "runtimeCodeCommit": args.commit,
        "sourceHeadAtRun": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
        "workingTreeAtRun": bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT)),
        "runnerSha256": digest(__file__), "buildReceiptSha256": digest(args.build_receipt),
        "command": [sys.executable, *sys.argv],
        "environment": {"python": platform.python_version(), "platform": platform.platform(), "goBuildInfo": build_info},
        "binaryHashes": {name: digest(path) for name, path in [("go", args.go), ("host", args.host), ("core", args.core)]},
        "dependencyHashes": {name: digest(ROOT / name) for name in dependencies},
        "scope": "Actual Windows Go/Rust/Core, synthetic loopback provider and owned journal. No real model/provider, credential, grant, browser/UI, Tauri, phone, cloud or cross-channel delivery acceptance.",
        "checks": checks,
    }

    def check(name, condition):
        checks.append({"name": name, "status": "PASS" if condition else "FAIL"})
        assert condition, name

    environment = {key: value for key, value in os.environ.items() if key.upper() in ["SYSTEMROOT", "WINDIR", "PATH", "PATHEXT", "COMSPEC", "PROCESSOR_ARCHITECTURE", "NUMBER_OF_PROCESSORS"]}
    environment.update({"TEMP": str(private), "TMP": str(private), "PYTHONDONTWRITEBYTECODE": "1"})
    fixture = service = None
    handles = []
    service_logs = []
    error = None
    started = time.monotonic()
    try:
        fixture_log = (private / "fixture-stderr.txt").open("w", encoding="utf-8")
        fixture = subprocess.Popen([sys.executable, str(ROOT / "scripts/collaboration_fixture.py")], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=fixture_log, text=True, env=environment)
        ready = fixture.stdout.readline().strip()
        assert ready.startswith("READY http://127.0.0.1:") and ready.endswith("/v1"), "Loopback fixture startup"
        config = private / "fixture.json"
        config.write_text(json.dumps({"hostExecutable": str(args.host.resolve()), "coreExecutable": str(args.core.resolve()), "supervisor": str((ROOT / "scripts/core_supervisor.py").resolve()), "fixtureEndpoint": ready[6:]}), encoding="utf-8")
        state = private / "state"

        def launch():
            log = (private / ("service-%d.txt" % len(service_logs))).open("w", encoding="utf-8")
            service_logs.append(log)
            process = subprocess.Popen([str(args.go.resolve()), "--listen", "127.0.0.1:0", "--state", str(state), "--core-fixture-config", str(config)], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=log, text=True, env=environment)
            line = process.stdout.readline().strip()
            assert line.startswith("READY http://127.0.0.1:"), "Go startup failed"
            return process, Client(line[6:])

        service, client = launch()
        channel = client.request("/v1/channels", {"name": "Daily finite collaboration", "botIds": ["bot-meiiie", "bot-davin"], "clientNonce": "daily-native-group-channel"})["channel"]
        other_channel = client.request("/v1/channels", {"name": "Other audience", "botIds": ["bot-meiiie", "bot-davin"], "clientNonce": "daily-native-other-channel"})["channel"]
        private_work = client.send("[fixture:foreground] unrelated-target-private-canary", bot="bot-davin", nonce="daily-native-private-canary")
        client.wait(private_work["requestId"], lambda w: w["status"] == "completed")
        other_work = client.request("/v1/messages", {"botId": "bot-davin", "channelId": other_channel["id"], "expectedChannelRevision": other_channel["revision"], "text": "[fixture:foreground] unrelated-channel-canary", "lane": "foreground", "clientNonce": "daily-native-other-canary"})["work"]
        client.wait(other_work["requestId"], lambda w: w["status"] == "completed")
        synthetic_key = "fixture-only-group-canary"
        account = client.request("/v1/accounts", {"name": "Unverified local fixture metadata", "provider": "openai-compatible", "endpoint": "https://example.invalid/v1", "model": "fixture-model", "secret": synthetic_key, "clientNonce": "daily-native-group-account"})["account"]
        check("selected_account_stays_unverified_with_synthetic_sealed_key", account["status"] == "stored_unverified" and account["hasSecret"] and account["credentialGeneration"] == 1)
        project = client.request("/v1/projects", {"name": "Daily group source", "goal": "Compare descriptive evidence", "channelIds": [channel["id"]], "clientNonce": "daily-native-group-project"})["project"]
        source_bytes = "Descriptive source: phương án A costs 120; phương án B costs 140.\n".encode("utf-8")
        source = client.request("/v1/uploads", {"projectId": project["id"], "projectRevision": project["revision"], "channelId": channel["id"], "filename": "comparison-source.txt", "contentBase64": base64.b64encode(source_bytes).decode(), "clientNonce": "daily-native-group-source"})["file"]
        check("owned_source_ready_text_and_exact_original_hash", source["status"] == "ready_text" and source["sha256"] == hashlib.sha256(source_bytes).hexdigest())

        def create(nonce, goal):
            body = {"channelId": channel["id"], "expectedChannelRevision": channel["revision"], "participants": ["bot-meiiie", "bot-davin"], "coordinatorBotId": "bot-meiiie", "goal": goal, "clientNonce": nonce, "maxStages": 3, "accountId": account["id"], "accountRevision": account["revision"], "projectId": project["id"], "projectRevision": project["revision"], "fileIds": [source["id"]]}
            return client.request("/v1/collaborations", body)["collaboration"], body

        def group(identifier):
            return next(g for g in client.request("/v1/collaborations")["collaborations"] if g["id"] == identifier)

        def wait_group(identifier, predicate, timeout=45):
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                value = group(identifier)
                if predicate(value):
                    return value
                time.sleep(.1)
            raise AssertionError({"groupId": identifier, "status": value["status"], "reason": value.get("reason"), "stages": len(value["requestIds"])})

        goal = "Compare the two plans and return one final file to this shared channel."
        created, body = create("daily-native-three-stages", goal)
        final = wait_group(created["id"], lambda g: g["status"] == "completed")
        works = [client.work(identifier) for identifier in final["requestIds"]]
        check("three_stages_two_distinct_bots_first_coordinator_returns", len(works) == 3 and [w["botId"] for w in works] == ["bot-meiiie", "bot-davin", "bot-meiiie"] and [w["collaboration"]["stage"] for w in works] == [1, 2, 3])
        check("all_stages_completed_exact_shared_destination", all(w["status"] == "completed" and w["conversationId"] == channel["id"] and w["publication"]["sourceChannelId"] == channel["id"] and w["publication"]["replyChannelId"] == channel["id"] and w["publication"]["audienceRevision"] == channel["revision"] for w in works))
        check("account_project_file_pins_identical_across_stages", all(w["accountPin"] == final["accountPin"] and w["resources"] == final["resources"] and w["resources"]["files"][0]["sha256"] == source["sha256"] for w in works))
        check("genuine_Core_bindings_match_verified_executable", all(w["core"]["coreSha256"] == CORE_HASH and w["core"]["nativeProcessId"] > 0 and w["core"].get("contextAdmission") for w in works))
        check("early_stage_writes_denied_final_coordinator_only", all(not w.get("operations") and not w.get("artifacts") and "Only the final coordinator stage" in w["reply"] for w in works[:2]) and len(works[2]["artifacts"]) == 1 and len(works[2]["operations"]) == 1)
        check("previous_stage_results_in_exact_context", all(all(prior["requestId"] in [s["requestId"] for s in w["contextPin"]["sources"]] and prior["requestId"] in w["reply"] for prior in works[:index]) for index, w in enumerate(works)))
        check("target_private_and_other_channel_sources_excluded", all(private_work["requestId"] not in [s["requestId"] for s in w["contextPin"]["sources"]] and other_work["requestId"] not in [s["requestId"] for s in w["contextPin"]["sources"]] and "unrelated-target-private-canary" not in json.dumps(w["contextPin"]) and "unrelated-channel-canary" not in json.dumps(w["contextPin"]) for w in works))
        artifact = works[2]["artifacts"][0]
        content, headers = client.request("/v1/channels/" + channel["id"] + "/artifacts/" + artifact["id"])
        expected = ("# Finite conclusion\nGroup " + final["id"] + "; final stage 3.\n").encode()
        check("final_file_content_and_sha256_exact", content == expected and hashlib.sha256(content).hexdigest() == artifact["sha256"] == headers["X-Artifact-SHA256"])
        try:
            client.request("/v1/channels/" + other_channel["id"] + "/artifacts/" + artifact["id"])
            wrong_scope_denied = False
        except urllib.error.HTTPError as failure:
            wrong_scope_denied = failure.code == 404
        check("final_file_refused_in_other_shared_channel", wrong_scope_denied)
        exported = client.request("/v1/channels/" + channel["id"] + "/export?format=json&projectId=" + project["id"])
        rows = exported["messages"]
        check("one_owner_goal_no_internal_stage_requests_in_public_history", len(rows) == 1 and rows[0]["requestId"] == final["id"] and rows[0]["text"] == goal and rows[0]["reply"] == works[2]["reply"] and len(rows[0]["files"]) == 1 and not any(w["requestId"] in [row["requestId"] for row in rows] for w in works))
        retry = client.request("/v1/collaborations", body)
        check("original_group_nonce_returns_same_terminal_map", retry["duplicate"] and retry["collaboration"] == final)
        changed = dict(body, goal="Changed goal for an already-used nonce")
        try:
            client.request("/v1/collaborations", changed)
            changed_nonce_denied = False
        except urllib.error.HTTPError as failure:
            changed_nonce_denied = failure.code == 409
        check("same_nonce_changed_goal_conflicts", changed_nonce_denied)

        slow, _ = create("daily-native-cancel-scope", "[collaboration:slow] Review options and return one final file.")
        admitted = wait_group(slow["id"], lambda g: bool(g["requestIds"]))
        active = client.wait(admitted["requestIds"][0], lambda w: w["status"] == "running" and bool(w.get("reply")))
        handle = CoreHandle(active["core"]["nativeProcessId"], args.core)
        handles.append(handle)
        start = time.monotonic()
        foreground = client.send("[fixture:foreground] Answer the new owner message while the group is held.", nonce="daily-native-interrupt-foreground")
        foreground = client.wait(foreground["requestId"], lambda w: w["status"] == "completed")
        report["foregroundSecondsSingleWarmFixture"] = round(time.monotonic() - start, 3)
        check("foreground_completes_while_group_Core_is_live", bool(foreground["reply"]) and not handle.stopped() and client.work(active["requestId"])["status"] == "running")
        cancel = {"expectedRevision": group(slow["id"])["revision"], "clientNonce": "daily-native-cancel-group"}
        client.request("/v1/collaborations/" + slow["id"] + "/cancel", cancel)
        cancelled = wait_group(slow["id"], lambda g: g["status"] == "cancelled")
        stopped = client.work(active["requestId"])
        check("cancelled_group_waits_retained_authentic_Core_handle", handle.stopped() and stopped["status"] == "cancelled" and stopped["runtimeStopped"] and stopped["runtimeStopReceipt"]["nativeProcessId"] == active["core"]["nativeProcessId"] and stopped["runtimeStopReceipt"]["sessionId"] == active["core"]["sessionId"])
        frozen_reply = stopped["reply"]
        time.sleep(.6)
        check("cancel_fences_late_output_effect_and_next_stage", client.work(active["requestId"])["reply"] == frozen_reply and not stopped.get("operations") and not stopped.get("artifacts") and len(group(slow["id"])["requestIds"]) == 1)
        check("cancel_is_scoped_and_idempotent", client.work(foreground["requestId"])["status"] == "completed" and group(final["id"]) == final and client.request("/v1/collaborations/" + slow["id"] + "/cancel", cancel)["duplicate"])
        for _ in range(3):
            service.kill()
            service.wait(timeout=5)
            service, client = launch()
            assert group(final["id"]) == final and group(slow["id"]) == cancelled
            assert client.work(works[2]["requestId"])["artifacts"] == works[2]["artifacts"]
        check("restart3_preserves_terminal_group_map_and_final_file", True)

        revoked, _ = create("daily-native-audience-revoke", "[collaboration:slow] Hold this review before any new effect.")
        admitted = wait_group(revoked["id"], lambda g: bool(g["requestIds"]))
        active = client.wait(admitted["requestIds"][0], lambda w: w["status"] == "running" and bool(w.get("reply")))
        handle = CoreHandle(active["core"]["nativeProcessId"], args.core)
        handles.append(handle)
        client.request("/v1/channels", {"id": channel["id"], "name": channel["name"], "botIds": ["bot-meiiie"], "expectedRevision": channel["revision"], "clientNonce": "daily-native-member-revoke"})
        suspended = wait_group(revoked["id"], lambda g: g["status"] == "suspended")
        stopped = client.wait(active["requestId"], lambda w: w["status"] in ["cancelled", "suspended"])
        check("audience_change_stops_actual_Core_without_rebinding", handle.stopped() and stopped["runtimeStopped"] and len(suspended["requestIds"]) == 1 and not stopped.get("operations") and not stopped.get("artifacts") and stopped["contextPin"] == active["contextPin"] and suspended["expectedChannelRevision"] == channel["revision"])
        check("synthetic_account_value_excluded_from_task_journal", synthetic_key not in (state / "transactions.jsonl").read_text(encoding="utf-8"))
        check("tested_dependencies_unchanged_during_run", report["dependencyHashes"] == {name: digest(ROOT / name) for name in dependencies})
        report.update({"status": "PASS", "groupsCompleted": 1, "stagesCompleted": 3, "finalArtifactSha256": artifact["sha256"], "elapsedSeconds": round(time.monotonic() - started, 3)})
    except Exception as failure:
        error = failure
        report.update({"status": "FAIL", "errorType": type(failure).__name__, "error": str(failure), "elapsedSeconds": round(time.monotonic() - started, 3)})
    finally:
        if service and service.poll() is None:
            service.kill()
            service.wait(timeout=10)
        if fixture and fixture.poll() is None:
            fixture.stdin.close()
            try:
                fixture.wait(timeout=10)
            except subprocess.TimeoutExpired:
                fixture.kill()
                fixture.wait(timeout=5)
        stop_results = [{"pid": handle.pid, "stopped": handle.stopped(10000)} for handle in handles]
        for handle in handles:
            handle.close()
        for log in service_logs:
            log.close()
        if "fixture_log" in locals():
            fixture_log.close()
        report["cleanup"] = {"serviceExited": service is None or service.poll() is not None, "fixtureExited": fixture is None or fixture.poll() is not None, "retainedCoreHandles": stop_results}
        if not all(item["stopped"] for item in stop_results):
            report["status"] = "FAIL"
            error = error or AssertionError("Owned Core handle remained live after cleanup")
        (output / "results.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
        (output / "build-info.txt").write_text(build_info, encoding="utf-8")
        print(json.dumps({"status": report["status"], "checks": len(checks), "passed": sum(c["status"] == "PASS" for c in checks), "output": str(output), "cleanup": report["cleanup"]}))
    if error:
        raise error


if __name__ == "__main__":
    main()
