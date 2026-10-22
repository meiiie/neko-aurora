"""Actual Go/Rust/Core finite group acceptance; owned state and loopback fixture only."""
import argparse
import contextlib
import uuid
import hashlib
import json
import pathlib
import subprocess
import sys
import tempfile
import time
import urllib.error

from e2e_core import ROOT, launch
from e2e_permissions import OwnedCoreHandle


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--commit", required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    args = parser.parse_args()
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    assert head == args.commit and not subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT)
    fixture = subprocess.Popen([sys.executable, str(ROOT / "scripts/collaboration_fixture.py")], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
    process = None
    handles = []
    checks = []
    try:
        endpoint = fixture.stdout.readline().strip()
        assert endpoint.startswith("READY ")
        args.output.parent.mkdir(parents=True,exist_ok=True)
        owned_directory=args.output.parent / ("owned-journal-"+uuid.uuid4().hex)
        owned_directory.mkdir()
        with contextlib.nullcontext(owned_directory) as directory:
            state = pathlib.Path(directory) / "state"
            config = pathlib.Path(directory) / "fixture.json"
            config.write_text(json.dumps({"hostExecutable": str(ROOT / "host/target/debug/neko-host.exe"), "coreExecutable": str(ROOT / ".build/core-windows/neko.exe"), "supervisor": str(ROOT / "scripts/core_supervisor.py"), "fixtureEndpoint": endpoint[6:]}))
            process, client = launch(state, config)
            channel = client.request("/v1/channels", {"name": "Owner finite review", "botIds": ["bot-meiiie", "bot-davin"], "clientNonce": "native-collab-channel"})["channel"]
            private = client.send("[fixture:foreground] private-Davin-canary", bot="bot-davin")
            client.wait(private["requestId"], lambda w: w["status"] == "completed")
            def create(nonce, goal):
                body = {"channelId": channel["id"], "expectedChannelRevision": channel["revision"], "participants": ["bot-meiiie", "bot-davin"], "coordinatorBotId": "bot-meiiie", "goal": goal, "clientNonce": nonce, "maxStages": 3}
                return client.request("/v1/collaborations", body)["collaboration"], body
            def group(id):
                return next(g for g in client.request("/v1/collaborations")["collaborations"] if g["id"] == id)
            def wait_group(id, predicate, timeout=35):
                deadline = time.monotonic() + timeout
                while time.monotonic() < deadline:
                    g = group(id)
                    if predicate(g): return g
                    time.sleep(.1)
                raise AssertionError(g)
            collaboration, body = create("native-collab-three-stages", "Compare plans and return a final file.")
            final = wait_group(collaboration["id"], lambda g: g["status"] == "completed")
            assert len(final["requestIds"]) == 3
            works = [client.work(id) for id in final["requestIds"]]
            assert [w["botId"] for w in works] == ["bot-meiiie", "bot-davin", "bot-meiiie"]
            assert all(w["status"] == "completed" and w["conversationId"] == channel["id"] for w in works)
            assert not works[0].get("operations") and not works[1].get("operations") and len(works[2]["artifacts"]) == 1
            assert "Only the final coordinator stage" in works[0]["reply"] and "Only the final coordinator stage" in works[1]["reply"]
            for i, w in enumerate(works):
                sources = [s["requestId"] for s in w["contextPin"]["sources"]]
                assert all(prior["requestId"] in sources and prior["requestId"] in w["reply"] for prior in works[:i])
                assert private["requestId"] not in sources and "private-Davin-canary" not in json.dumps(w["contextPin"])
            checks.append({"name": "actual_Core_three_stages_two_identities_exact_shared_context_and_only_final_file_writer", "pass": True})
            artifact = works[2]["artifacts"][0]
            content, headers = client.request("/v1/channels/" + channel["id"] + "/artifacts/" + artifact["id"])
            assert hashlib.sha256(content).hexdigest() == artifact["sha256"] == headers["X-Artifact-SHA256"]
            retry = client.request("/v1/collaborations", body)
            assert retry["duplicate"] and retry["collaboration"] == final
            time.sleep(.3)
            assert group(final["id"])["requestIds"] == final["requestIds"]
            checks.append({"name": "durable_group_nonce_budget_and_exact_final_download", "pass": True})
            slow, _ = create("native-collab-cancel-scope", "[collaboration:slow] Compare options with one final file.")
            admitted = wait_group(slow["id"], lambda g: bool(g["requestIds"]))
            active = client.wait(admitted["requestIds"][0], lambda w: bool(w.get("reply")))
            handle = OwnedCoreHandle(active["core"]["nativeProcessId"]); handles.append(handle)
            foreground = client.send("[fixture:foreground] Owner asks a new question now.")
            client.wait(foreground["requestId"], lambda w: w["status"] == "completed")
            assert client.work(active["requestId"])["status"] == "running"
            checks.append({"name": "foreground_owner_message_completes_while_collaboration_runs", "pass": True})
            cancel = {"expectedRevision": admitted["revision"], "clientNonce": "native-group-cancel-nonce"}
            client.request("/v1/collaborations/" + slow["id"] + "/cancel", cancel)
            cancelled = wait_group(slow["id"], lambda g: g["status"] == "cancelled")
            stopped = client.work(active["requestId"])
            handle.assert_stopped()
            assert stopped["status"] == "cancelled" and not stopped.get("operations") and not stopped.get("artifacts") and len(cancelled["requestIds"]) == 1
            assert client.work(foreground["requestId"])["status"] == "completed"
            assert client.request("/v1/collaborations/" + slow["id"] + "/cancel", cancel)["duplicate"]
            checks.append({"name": "scoped_group_cancel_waits_actual_owned_Core_stop_no_late_effect_or_second_stage", "pass": True})
            for _ in range(3):
                process.kill(); process.wait(timeout=5)
                process, client = launch(state, config)
                assert group(final["id"]) == final and group(slow["id"]) == cancelled
                assert client.work(works[2]["requestId"])["artifacts"] == works[2]["artifacts"]
            checks.append({"name": "restart3_preserves_group_request_map_terminal_budget_and_artifact", "pass": True})
            revoked, _ = create("native-collab-audience-revoke", "[collaboration:slow] Review plans.")
            admitted = wait_group(revoked["id"], lambda g: bool(g["requestIds"]))
            active = client.wait(admitted["requestIds"][0], lambda w: bool(w.get("reply")))
            handle = OwnedCoreHandle(active["core"]["nativeProcessId"]); handles.append(handle)
            client.request("/v1/channels", {"id": channel["id"], "name": channel["name"], "botIds": ["bot-meiiie"], "expectedRevision": 1, "clientNonce": "native-collab-remove-member"})
            halted = wait_group(revoked["id"], lambda g: g["status"] == "suspended")
            stopped = client.wait(active["requestId"], lambda w: w["status"] in ["cancelled", "suspended"])
            handle.assert_stopped()
            assert len(halted["requestIds"]) == 1 and not stopped.get("operations") and not stopped.get("artifacts")
            checks.append({"name": "audience_revision_change_halts_group_and_actual_Core_without_rebinding", "pass": True})
        report = {"commit": head, "workingTree": False, "scope": "Actual Windows Go/Rust/Core with owned temporary journal and loopback synthetic provider. Local owner/channel/account contracts only; no real provider, connector, cloud, UI or phone/model evaluation.", "checks": checks}
        for name, path in [("go", ROOT / ".build/neko-aurora.exe"), ("host", ROOT / "host/target/debug/neko-host.exe"), ("core", ROOT / ".build/core-windows/neko.exe"), ("helper", ROOT / ".build/core-windows/neko-writer-lease.exe")]:
            report[name + "Sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(report, indent=2))
    except Exception as error:
        failure={"commit":head,"workingTree":False,"pass":False,"checks":checks,"errorType":type(error).__name__,"error":str(error),"ownedState":str(state) if "state" in locals() else None}
        args.output.parent.mkdir(parents=True,exist_ok=True);args.output.with_suffix(".failed.json").write_text(json.dumps(failure,indent=2)+"\n",encoding="utf-8")
        raise
    finally:
        for handle in handles: handle.close()
        if process and process.poll() is None: process.kill(); process.wait(timeout=5)
        if process and process.stderr: args.output.with_suffix(".go-stderr.txt").write_text(process.stderr.read(),encoding="utf-8")
        fixture.stdin.close()
        try: fixture.wait(timeout=5)
        except subprocess.TimeoutExpired: fixture.kill(); fixture.wait(timeout=5)


if __name__ == "__main__":
    main()
