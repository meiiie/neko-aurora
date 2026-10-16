# Published report source inspection, 2026-10-13

Tested UI/application source: `27901b091e1b4316a183fd45f6e28c96f846d98a`, from delivery `1484845d9f49367c799974d4fc05cda9ed3de346`. This follow-up changes only `app.js` and its VM regression script. Core remains `973a3bd93466baa4689e01c4050dfc59f5a7f7c4`; Go ledger/API, Rust host, Tauri and approved light CSS/fonts/branding are unchanged. A later documentation checkpoint is the delivery identity, not a different runtime test.

When a shared-channel request uses a previously published report, **Nguồn ngữ cảnh** now accepts the backend's exact `published_report` / `owner_published_result_destination_only` metadata and shows **Báo cáo đã công bố** with its destination-only reason. The panel keeps IDs, revisions and availability only. It does not render the report reply, excerpts, private conversation identifiers or raw source pins. A report can be unavailable even when its pinned/current sequence numbers are equal.

Authority/reconciliation fences now clear both an open source list and pending GET read tokens, including history-derived reports. Public report descriptors and context presence participate in the source signature. A delayed pre-revoke reply cannot repopulate the list; the existing explicit refresh reads current availability. The existing append-only text path remains unfenced, so unrelated streamed text does not discard valid inspection.

## Evidence and limits

All caches, fixture state and process profiles use owned E scratch. Evidence root: `E:/CodexScratch/published-sources-20261013`.

| Command / exact scope | Status |
| --- | --- |
| Node25.9 Windows, `node scripts/ui_published_sources_local.mjs`, clean279 | PASS16 extracted-source VM cases, including renderer functions in a minimal authored DOM. This is not a browser screenshot. |
| `node scripts/ui_sources_history_local.mjs`, clean279 | PASS19 existing metadata/scope/export VM cases. |
| `node scripts/ui_principal_report_seams.mjs <owned-E-output>`, clean279 | PASS6 actor/source projection and delayed-preparation VM cases. Initial invocation used the wrong output argument and is retained as a harness error. |
| Independent read-only reviewer, clean279 | PASS16 +19 and independently authored11 VM probes; bounded diff review found no P1/P2. No browser/Go/Core execution by reviewer. |
| Go1.27.2, `go test ./internal/api -run 'Test.*SourceExplanation\|TestReportDelivery' -count=1 -json`; `go build -o <owned-E>/neko-control.exe ./cmd/neko-control`, clean279 | PASS exit0. Default binary SHA256 `16c7dfc58e6a50ae1d970a941614ac1235aed16f5a82528212eb0c3bf8b11242`; build VCS279, modified=false. No fresh full regression claim. |
| Python3.13 actual HTTP/local journal fixture, `run-published-sources-http.py`, clean279 default binary | PASS10: publish authored completed result, exact destination intake, metadata-only source GET, wrong-channel409, no read mutation/runtime launch, source revoke and restart retaining unavailable identity. First fixture stdout decoding failed before service launch; UTF-8 harness-only correction and a fresh run are retained. No Core/model execution. |
| Actual Go/headless Chrome product renderer driver, rounds r1/r2 | BLOCKED before UI cases. Chrome reported that remote debugging requires a non-default data directory despite an explicit owned `--user-data-dir`. Profile isolation was not established. Owned launcher/service processes were stopped; no policy change or further browser workaround. No new media. |
| Native tool recheck, 2026-10-13 | BLOCKED: `await cua.getState()` kernel exited(code=1), `MXC launcher: cannot locate required Windows platform directory: HRESULT 0x80070003`. Required node_repl/sky runtime is unavailable; native APIs remain disabled. Candidate was not opened. |

RED baseline148 retained9 FAIL /7 PASS. The first new minimal-DOM adapter used the wrong `node(tag,class,text)` signature, leaving a renderer assertion false after product repair; correction was confined to the test adapter, and the original result remains. The old cross-channel report VM harness independently has missing modern session globals and invalid downstream fixture state; the same17 failure names reproduce on baseline148 and the repair. Its extracted environment additionally lacks the new source-fence helper, changing that one error message. These are not accepted regression evidence. None of those results are relabelled PASS.

Current native user step remains opening the separately preserved `candidate-c91e4de/Start-Neko-Desktop.cmd` and inspecting Connections/Tab/Escape without consenting to profile access. That candidate has the earlier assets and remains distinct from this source-only repair. Its52-file sealed packet, launch bindings and all prior media are unchanged. A new native artifact must be separately packaged and inspected before being described as accepted.

Main integration must preserve its three existing raw-byte script changes,17 ZIPs,1509 protected files and installed Core hash. No actual profile/credential, paid provider, external grant, live8765, installed Core mutation, deploy or public push is authorized by this slice. Native renderer/phone/AT and real provider/cloud remain separate gates. Onboarding source preloading and an explicit scoped note-clear draft affordance are still useful independent next steps; semantic memory forgetting/purge is not implied by note editing.
