"""Actual pinned Go/Rust/Core with synthetic loopback responses; no model/UI/cloud claim."""

import argparse
import hashlib
import io
import json
import os
import pathlib
import queue
import re
import subprocess
import sys
import tarfile
import threading
import time
from http.server import ThreadingHTTPServer

sys.dont_write_bytecode = True
ROOT = pathlib.Path(__file__).resolve().parents[1]
COMMIT = "e127a4189ffa39cf055930ed05a30d7360aba928"
SCRATCH = pathlib.Path("E:/CodexScratch/group-context-budget-20261013/native-e127").resolve()
CANDIDATE = pathlib.Path("E:/CodexScratch/parallel-product-20261013/candidate-5349523").resolve()
MANIFEST_SHA = "f761ab2ae3678781c3d9b744c5783330203951b3f881980fd8492e72003784e6"
PINNED = {
    "host": "a1693c4f3309c86e29b348ffc913c888cae3f8fd2bdf04bcac418e107a6654d2",
    "core": "356247616082e613abd99b97838fd533f5e14ab64b07f480d78ae799d10648bf",
    "writerHelper": "f9f404a07a7fe1aa56a2b7d3c073a2330366de5ee26d572b37421338e2b99a3a",
}
REPORT_TEXT = "Report evidence sentence. " * 240
REPORT_FILE = "# Published source report\nOwned synthetic fixture bytes.\n"


def sha(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


def ready(process):
    result = queue.Queue()
    threading.Thread(target=lambda: result.put(process.stdout.readline().strip()), daemon=True).start()
    line = result.get(timeout=25)
    if not line.startswith("READY http://127.0.0.1:"):
        raise RuntimeError("launch_policy_or_startup_blocked")
    return line[6:]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=pathlib.Path)
    parser.add_argument("--go-tool", required=True, type=pathlib.Path)
    args = parser.parse_args()
    out = args.output.resolve()
    if out == SCRATCH or not out.is_relative_to(SCRATCH):
        parser.error("output must be a fresh child of the owned native-e127 directory")
    out.mkdir(parents=True, exist_ok=False)
    private = out / "private-fixture"
    private.mkdir()
    for name in ("tmp", "home"):
        (private / name).mkdir()
    named = ("SYSTEMROOT", "WINDIR", "PATH", "PATHEXT", "COMSPEC", "PROCESSOR_ARCHITECTURE", "NUMBER_OF_PROCESSORS")
    env = {name: os.environ[name] for name in named if name in os.environ}
    env.update({"TEMP": str(private / "tmp"), "TMP": str(private / "tmp"), "USERPROFILE": str(private / "home"), "PYTHONDONTWRITEBYTECODE": "1", "GOCACHE": "E:/CodexScratch/core-auth-bridge-20261012/root/gocache", "GOTMPDIR": str(private / "tmp"), "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOENV": "off", "CGO_ENABLED": "0"})
    report = {"status": "RUNNING", "scope": __doc__, "applicationCommit": COMMIT, "coreCode": "973a3bd93466baa4689e01c4050dfc59f5a7f7c4", "command": [sys.executable, *sys.argv], "runnerSHA256": sha(__file__), "safeEnvironmentKeys": sorted(env), "checks": [], "launches": []}
    service = server = None
    server_thread = None
    logs = []
    failure = None
    started = time.monotonic()

    def check(name, condition):
        report["checks"].append({"name": name, "status": "PASS" if condition else "FAIL"})
        if not condition:
            raise AssertionError(name)

    def stop_service():
        nonlocal service
        if service is not None and service.poll() is None:
            service.terminate()
            try:
                service.wait(timeout=15)
            except subprocess.TimeoutExpired:
                service.kill()
                service.wait(timeout=5)
        service = None

    try:
        manifest_path = CANDIDATE / ".checkpoints/desktop-release/launch.json"
        check("pinned_candidate_manifest", sha(manifest_path) == MANIFEST_SHA)
        manifest = json.loads(manifest_path.read_text(encoding="utf-8-sig"))
        roles = {}
        for role, expected in PINNED.items():
            path = (CANDIDATE / manifest[role]["path"]).resolve()
            check("pinned_" + role, path.is_relative_to(CANDIDATE) and manifest[role]["sha256"] == expected and sha(path) == expected)
            roles[role] = path
        archive = subprocess.check_output(["git", "archive", "--format=tar", COMMIT, "native", "contracts", "scripts"], cwd=ROOT, env=env)
        source = out / "immutable-source"
        source.mkdir()
        with tarfile.open(fileobj=io.BytesIO(archive)) as package:
            for member in package.getmembers():
                target = (source / member.name).resolve()
                if not target.is_relative_to(source) or not (member.isfile() or member.isdir()):
                    raise RuntimeError("archive_path_or_file_type_refused")
            package.extractall(source, filter="data")
        source_hashes = {p.relative_to(source).as_posix(): sha(p) for p in sorted(source.rglob("*")) if p.is_file()}
        report["source"] = {"commit": COMMIT, "tree": subprocess.check_output(["git", "rev-parse", COMMIT + "^{tree}"], cwd=ROOT, env=env, text=True).strip(), "archiveSHA256": hashlib.sha256(archive).hexdigest(), "sourceSetSHA256": hashlib.sha256(json.dumps(source_hashes, sort_keys=True, separators=(",", ":")).encode()).hexdigest(), "files": source_hashes, "archiveCommand": ["git", "archive", "--format=tar", COMMIT, "native", "contracts", "scripts"]}
        supervisor = source / "scripts/core_supervisor.py"
        trusted_supervisor = next(p for p in manifest["trustedFiles"] if p["path"] == "scripts/core_supervisor.py")
        check("immutable_supervisor_matches_pinned_bytes", sha(supervisor) == trusted_supervisor["sha256"] == sha(CANDIDATE / trusted_supervisor["path"]))
        binary = out / "neko-control-e127.exe"
        build_command = [str(args.go_tool.resolve()), "build", "-buildvcs=false", "-o", str(binary), "./cmd/neko-control"]
        build = subprocess.run(build_command, cwd=source / "native", env=env, capture_output=True, text=True, encoding="utf-8", timeout=120)
        (out / "build.stdout.txt").write_text(build.stdout, encoding="utf-8")
        (out / "build.stderr.txt").write_text(build.stderr, encoding="utf-8")
        report["build"] = {"command": build_command, "cwd": str(source / "native"), "exitCode": build.returncode, "embeddedVCS": False, "scope": "Exact selective immutable Git archive; no working-tree build or embedded VCS claim"}
        check("immutable_Go_archive_build", build.returncode == 0)
        report["binaryHashes"] = {"go": sha(binary), **{role: sha(path) for role, path in roles.items()}}
        report["buildInfo"] = subprocess.check_output([str(args.go_tool.resolve()), "version", "-m", str(binary)], env=env, text=True, encoding="utf-8")
        sys.path.insert(0, str(source / "scripts"))
        from e2e_core import Client
        from fixture_provider import Handler as BaseHandler, admitted_fixture_context

        class Handler(BaseHandler):
            audit = []
            replies = {}
            failures = []
            lock = threading.Lock()

            def call(self, name, arguments, identity):
                self.chunk({"role": "assistant", "tool_calls": [{"index": 0, "id": identity, "type": "function", "function": {"name": name, "arguments": json.dumps(arguments)}}]})
                self.chunk({}, "tool_calls")

            def answer(self, answer):
                self.chunk({"role": "assistant", "content": answer})
                self.chunk({}, "stop")

            def do_POST(self):
                if self.path != "/v1/chat/completions" or self.headers.get("Authorization") != "Bearer fixture-only":
                    self.send_error(403)
                    return
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 < length <= 2_000_000:
                    self.send_error(413)
                    return
                body = json.loads(self.rfile.read(length))
                messages = body["messages"]
                admitted = admitted_fixture_context(messages)
                if admitted is None:
                    self.send_error(409)
                    self.failures.append("missing_actual_Core_admitted_manifest")
                    return
                user = admitted["primaryMessage"]["text"]
                tools = [str(m.get("content", "")) for m in messages if m.get("role") == "tool"]
                derived = []
                for item in admitted["sources"]:
                    if item["kind"] == "derived":
                        try:
                            derived.append(json.loads(item["span"]["text"]))
                        except ValueError:
                            pass
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Connection", "close")
                self.end_headers()
                try:
                    if "[budgetnative:ambient]" in user:
                        self.answer("PRIVATE_AMBIENT_CANARY")
                    elif "[budgetnative:report]" in user:
                        if not tools:
                            self.call("mcp__nekobot__execute_tool", {"name": "artifact.write_text", "arguments": {"filename": "public-report.md", "text": REPORT_FILE}}, "source-report-write")
                        else:
                            self.answer(REPORT_TEXT)
                    else:
                        stage_data = next(x for x in derived if x.get("kind") == "generated-collaboration-stage")
                        stage, identifier = stage_data["stage"], stage_data["collaborationId"]
                        peers = [x for x in derived if x.get("kind") == "pinned-prior-turn"]
                        published = [x for x in derived if x.get("deliveryId")]
                        profiles = [x for x in derived if x.get("kind") == "pinned-bot-preferences"]
                        scenario = "near-report" if "[budgetnative:near-report]" in user else "history-off"
                        if len(peers) != stage - 1:
                            raise AssertionError("actual_Core_peer_count")
                        expected_reports = 1 if scenario == "near-report" and stage == 1 else 0
                        if len(published) != expected_reports:
                            raise AssertionError("actual_Core_whole_report_selection")
                        if not profiles or scenario == "history-off" and profiles[0]["includeCompletedConversationHistory"]:
                            raise AssertionError("actual_Core_history_off_profile")
                        for index, peer in enumerate(peers, 1):
                            assistant = next(m["text"] for m in peer["messages"] if m["role"] == "assistant")
                            if assistant != self.replies[(identifier, index)]:
                                raise AssertionError("actual_Core_complete_peer_bytes")
                        encoded = json.dumps(admitted)
                        if "PRIVATE_AMBIENT_CANARY" in encoded or "PRIVATE_PROMPT_CANARY" in encoded:
                            raise AssertionError("private_context_entered_group")
                        observation = {"groupId": identifier, "scenario": scenario, "stage": stage, "contextPinSHA256": admitted["contextPinSHA256"], "peerRequestIds": [x["requestId"] for x in peers], "peerReplySHA256": [hashlib.sha256(next(m["text"] for m in p["messages"] if m["role"] == "assistant").encode()).hexdigest() for p in peers], "reportCount": len(published), "profileHistory": profiles[0]["includeCompletedConversationHistory"], "privateCanariesExcluded": True, "completePeerBytes": True}
                        with self.lock:
                            self.audit.append(observation)
                        if not tools:
                            self.call("mcp__nekobot__capabilities", {}, "stage-discover")
                        else:
                            if ("artifact.write_text" in tools[0]) != (stage == 3):
                                raise AssertionError("actual_Core_final_writer_capability")
                            if stage == 1 and published and published[0].get("files") and len(tools) == 1:
                                p = published[0]
                                f = p["files"][0]
                                self.call("mcp__nekobot__execute_tool", {"name": "report.read_artifact", "arguments": {"deliveryId": p["deliveryId"], "artifactId": f["artifact"]["id"], "offset": 0, "bytes": len(f["excerpt"].encode())}}, "stage-read-published")
                            elif stage == 3 and len(tools) == 1:
                                self.call("mcp__nekobot__execute_tool", {"name": "artifact.write_text", "arguments": {"filename": "group-final.md", "text": "# Native group result\nScenario=" + scenario + "\nCompletePeerSources=2\n"}}, "stage-final-write")
                            else:
                                reply = "STAGE=" + str(stage) + "\nSCENARIO=" + scenario + "\nPEERS=" + json.dumps([p["requestId"] for p in peers]) + "\nREPORTS=" + str(len(published)) + "\n" + "Bounded proposal: verify source observations. " * 12
                                self.replies[(identifier, stage)] = reply
                                self.answer(reply)
                    self.wfile.write(b"data: [DONE]\n\n")
                    self.wfile.flush()
                except (BrokenPipeError, ConnectionResetError):
                    pass
                except Exception as error:
                    self.failures.append(type(error).__name__ + ":" + str(error))

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        server.daemon_threads = True
        server_thread = threading.Thread(target=server.serve_forever, daemon=True)
        server_thread.start()
        config = private / "fixture.json"
        config.write_text(json.dumps({"hostExecutable": str(roles["host"]), "coreExecutable": str(roles["core"]), "supervisor": str(supervisor), "fixtureEndpoint": "http://127.0.0.1:%d/v1" % server.server_port}), encoding="utf-8")
        state = private / "state"

        def launch():
            nonlocal service
            log = (private / ("service-%d.log" % len(logs))).open("w", encoding="utf-8")
            logs.append(log)
            command = [str(binary), "--listen", "127.0.0.1:0", "--state", str(state), "--core-fixture-config", str(config)]
            service = subprocess.Popen(command, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=log, env=env, text=True, encoding="utf-8", creationflags=subprocess.CREATE_NO_WINDOW)
            report["launches"].append({"pid": service.pid, "command": command})
            return Client(ready(service))

        client = launch()
        report["environment"] = {"python": sys.version.split()[0], "go": subprocess.check_output([str(args.go_tool.resolve()), "version"], env=env, text=True).strip(), "runtime": "actual native Windows Go/Rust/Core"}
        ambient = client.send("[budgetnative:ambient] Private lane canary", bot="bot-davin", nonce="native-budget-ambient")
        client.wait(ambient["requestId"], lambda w: w["status"] == "completed", timeout=60)
        for bot in ("bot-meiiie", "bot-davin"):
            client.request("/v1/bots/" + bot + "/profile", {"instructions": "Keep responses clear", "includeHistory": False, "expectedRevision": 0, "clientNonce": "native-budget-profile-off-" + bot})

        groups = []

        def run_group(scenario):
            channel = client.request("/v1/channels", {"name": "Native budget " + scenario, "botIds": ["bot-meiiie", "bot-davin"], "clientNonce": "native-budget-channel-" + scenario})["channel"]
            delivered = None
            if scenario == "near-report":
                client.request("/v1/bots/bot-meiiie/profile", {"instructions": "Keep responses clear", "includeHistory": True, "expectedRevision": 1, "clientNonce": "native-budget-coordinator-history-on"})
                source_work = client.send("[budgetnative:report] Write completed report PRIVATE_PROMPT_CANARY", nonce="native-budget-private-report")
                source_work = client.wait(source_work["requestId"], lambda w: w["status"] == "completed", timeout=60)
                check("actual_Core_private_report_and_file", source_work["reply"] == REPORT_TEXT and len(source_work.get("artifacts", [])) == 1 and source_work["core"]["coreSha256"] == PINNED["core"])
                delivered = client.request("/v1/report-deliveries", {"botId": "bot-meiiie", "destinationChannelId": channel["id"], "expectedDestinationRevision": channel["revision"], "clientNonce": "native-budget-report-publish", "text": "@Meiiie gửi báo cáo mới nhất ở cuộc trò chuyện riêng vào kênh này"})["delivery"]
                check("actual_report_published_to_exact_destination", delivered["status"] == "delivered" and delivered["source"]["requestId"] == source_work["requestId"] and len(delivered["artifacts"]) == 1)
            body = {"channelId": channel["id"], "expectedChannelRevision": channel["revision"], "participants": ["bot-meiiie", "bot-davin"], "coordinatorBotId": "bot-meiiie", "goal": "[budgetnative:" + scenario + "] Compare the evidence and save the final group file.", "maxStages": 3, "clientNonce": "native-budget-group-" + scenario}
            group = client.request("/v1/collaborations", body)["collaboration"]
            deadline = time.monotonic() + 100
            while time.monotonic() < deadline:
                group = next(g for g in client.request("/v1/collaborations")["collaborations"] if g["id"] == group["id"])
                if group["status"] == "completed":
                    break
                if group["status"] in ("failed", "needs_owner", "suspended"):
                    raise RuntimeError("native_group_stopped:" + group["status"] + ":" + group.get("reason", ""))
                time.sleep(.1)
            check(scenario + "_three_stages_completed", group["status"] == "completed" and len(group["requestIds"]) == 3)
            stages = [client.work(identifier) for identifier in group["requestIds"]]
            for index, work in enumerate(stages):
                pin = work["contextPin"]
                expected_reports = 1 if scenario == "near-report" and index == 0 else 0
                check(scenario + "_stage_%d_native_admission" % (index + 1), work["core"]["coreSha256"] == PINNED["core"] and work["core"]["nativeProcessId"] > 0 and work["core"]["contextAdmission"]["contextPinSHA256"] == pin["sha256"] and work["core"]["contextAdmission"]["manifestSHA256"] == work["runtimeContextSHA256"] and len(pin["sources"]) == index and len(pin.get("publishedReports", [])) == expected_reports)
                check(scenario + "_stage_%d_complete_sources_and_writer" % (index + 1), [p["requestId"] for p in pin["sources"]] == group["requestIds"][:index] and all(p["text"] == stages[j]["text"] and p["reply"] == stages[j]["reply"] for j, p in enumerate(pin["sources"])) and (len(work.get("operations", [])) == 1 and len(work.get("artifacts", [])) == 1 if index == 2 else not work.get("operations") and not work.get("artifacts")))
                check(scenario + "_stage_%d_native_stop_receipt" % (index + 1), work.get("runtimeStopped") and work["runtimeStopReceipt"]["nativeProcessId"] == work["core"]["nativeProcessId"])
            artifact = stages[2]["artifacts"][0]
            data, headers = client.request("/v1/channels/" + channel["id"] + "/artifacts/" + artifact["id"])
            check(scenario + "_actual_final_file_hash", data == ("# Native group result\nScenario=" + scenario + "\nCompletePeerSources=2\n").encode() and hashlib.sha256(data).hexdigest() == artifact["sha256"] == headers["X-Artifact-SHA256"])
            groups.append({"scenario": scenario, "body": body, "group": group, "stagePins": [w["contextPin"] for w in stages], "artifact": artifact, "corePids": [w["core"]["nativeProcessId"] for w in stages]})

        run_group("history-off")
        run_group("near-report")
        check("fixture_manifest_observations_no_failures", not Handler.failures)
        stop_service()
        client = launch()
        for completed in groups:
            retry = client.request("/v1/collaborations", completed["body"])
            stages = [client.work(identifier) for identifier in completed["group"]["requestIds"]]
            check(completed["scenario"] + "_completed_restart_same_nonce_pins_file", retry["duplicate"] and retry["collaboration"] == completed["group"] and [w["contextPin"] for w in stages] == completed["stagePins"] and stages[2]["artifacts"][0] == completed["artifact"])
        report["nativeGroups"] = [{"scenario": g["scenario"], "groupId": g["group"]["id"], "requestIds": g["group"]["requestIds"], "contextPinSHA256": [p["sha256"] for p in g["stagePins"]], "corePids": g["corePids"], "artifact": g["artifact"]} for g in groups]
        report["providerObservations"] = Handler.audit
        report["status"] = "PASS"
    except Exception as error:
        failure = {"type": type(error).__name__, "label": str(error)}
        report["status"] = "BLOCKED" if "launch_policy_or_startup_blocked" in str(error) else "FAIL"
        report["failure"] = failure
    finally:
        stop_service()
        if server is not None:
            server.shutdown()
            server.server_close()
            server_thread.join(timeout=5)
        for log in logs:
            log.close()
        report["cleanup"] = {"ownedServiceClosed": service is None, "ownedProviderThreadClosed": server_thread is None or not server_thread.is_alive()}
        report["elapsedSeconds"] = round(time.monotonic() - started, 3)
        (out / "results.json").write_text(json.dumps(report, indent=2), encoding="utf-8")
        print(json.dumps({"status": report["status"], "pass": sum(c["status"] == "PASS" for c in report["checks"]), "fail": sum(c["status"] == "FAIL" for c in report["checks"]), "failure": failure, "cleanup": report["cleanup"], "results": str(out / "results.json")}, indent=2))
    return 0 if report["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
