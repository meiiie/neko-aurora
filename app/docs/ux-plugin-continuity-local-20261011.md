# Local plugin and project-continuity UI — 2026-10-11

This UI slice starts from `77ac5bb88c5c6a55cc16b4437d891da5867c9a2f`. The runtime source is unchanged from that checkpoint (parent identifies its tested runtime as 3dc). Ownership is four embedded frontend assets, this document, and two owned headless test files. The approved light layout, composer row, account DPAPI input, channel/project selection, focus fences, and durable task/group command journal remain in use.

## Request tools

The existing composer menu has an optional **Công cụ cho yêu cầu mới** dialog. Normal requests keep automatic selection and require no additional confirmation. Manual selection freezes `{id, expectedRevision}` in the message outbox and collaboration intent; an explicit empty list is tool-free. Changing the current chooser never changes a pending request. Project-file reader availability is still decided by the backend's selected sources and live authority; a metadata reference never grants file ingestion.

The plugin tab reads `/v1/plugin-catalog` under `local_plugin_lifecycle_v1` and `trusted_local_tool_broker_v1`. Original builtin tools can be enabled/disabled. Outside plugin files are descriptive JSON imports/updates/removals only and always remain non-executable. There is no arbitrary endpoint, code editor, execution grant, provider call, or manifest execution path in this UI. Names and status are rendered as text. Tool schemas are not shown in the product flow or persisted by this slice.

Lifecycle POST uses `/v1/plugin-lifecycle`, version 1, exact action/id/revision/nonce, and optional manifest. Duplicate lifecycle calls return the current descriptor, not a historical command receipt. The UI describes current state and keeps the original page-lifetime retry body.

## Owner project notes

**Ghi chú dự án** is shown only for a currently selected project and exact channel with `local_project_continuity_v1`. Authenticated scoped GET reads `/v1/projects/{id}/continuity?channelId=...`. The editor contains owner instructions, confirmed/contested decisions, optional literal dates/entity identifiers, and descriptive HTTPS or already-scoped text-file references. It never reads hidden Work runtime context, pinned source excerpts, or continuity metadata from public Work objects.

The save body freezes project revision, channel revision, continuity revision and nonce from the explicit scoped read. Recorded historical provenance revisions are retained; they are not silently changed to current authority counters. GET does not fetch reference URLs. Same names are not merged; dates are not inferred; this adds no semantic memory or compaction engine.

A file reference retains its exact hash/access pin. A changed/regranted source is shown as requiring explicit reselection; the UI does not automatically replace it. Owner reselection captures current metadata for a new record. Metadata changes can fence old requests; this UI never refreshes or resumes them implicitly. Messages/groups use optional `expectedContinuityRevision` only with `local_continuity_intake_cas_v1` and an already-observed current descriptor. Existing pending bodies are immutable.

## Uncertain results, reload and navigation

Policy command bodies live only in the page. A bounded, nonsecret review marker (at most six, 4 KiB) is saved before POST using `navigator.locks` and a fresh storage merge. Different tab intents retain one another; the same scope cannot get a second pending intent. If storage/locks are unavailable, mutation stops before POST. Other chat functionality stays usable.

Transport/5xx failure permits a page-lifetime retry of exactly the same body/nonce/CAS. A permanent 400/404/409/501 blocks retry and requires explicit review. Plugin/continuity have no supported command-receipt kind, so reloaded markers never reconstruct a body, automatically POST, or mint a replacement nonce. They offer read-current-state and explicit review/stop-retry. Reading current state does not claim proof that the old nonce applied. A new save requires a subsequent explicit scoped read after review. Closing the editor clears its draft DOM.

Catalog read tokens and navigation tokens are separate: an authorized file import is not silently dropped when the initial catalog read finishes. Public SSE/catalog/descriptor counters never decrease. A stale editor keeps the old CAS and draft and becomes non-saveable. Late responses preserve current bot/channel/project/draft/focus. The rapid close/reopen handler ignores a queued old close event; otherwise it could clear the new editor. Focus moves before a control becomes unavailable and only restores an opener when navigation is unchanged and current focus is absent/hidden. Dialog headers remain visible while their forms scroll.

## Validation boundary

`scripts/ui_plugin_continuity_local.mjs` builds no runtime itself: use a separately compiled owned Go control binary in `.build/neko-control.exe`, then provide an existing Rust host path and the exact clean commit. It runs actual Go journal/API/embedded assets in private scratch state and headless Chrome, without Core configuration. Intentional CDP response faults lose already committed ACKs; the test checks original bodies and real journal receipts. It covers optional group fields through the existing durable receipt/reload path, but group stages remain queued local obligations.

Example (PowerShell, owned checkout):

```powershell
$env:GOCACHE='E:\CodexScratch\context-manifest-builder-3309dd3\go-cache'
$env:GOPROXY='off'; $env:GOSUMDB='off'; $env:CGO_ENABLED='0'
& 'C:\Users\Admin\Documents\Codex\2026-10-11\task\.tooling\go\bin\go.exe' -C native build -o ..\.build\neko-control.exe ./cmd/neko-control
node scripts/ui_plugin_continuity_local.mjs --host E:\CodexScratch\context-admission-rust-3309dd3\target\debug\neko-host.exe --commit <full-clean-commit> --output .build/evidence-plugin-continuity-exact
```

The reused sealer is built from old standalone helper checkpoint 3cbf4b5, with unchanged DPAPI CLI. It is not the parent's current native host proof. Account regression evidence only verifies its local DPAPI behavior and existing Go account API; no provider verification is implied. The packet records that host hash separately from this owned Go binary's VCS stamp and unchanged backend blobs.

The exact checkpoint, commands, environments, checks, artifact hashes and pass/fail status are in the generated packet. Prior RED, harness failures and reproduced product failures are retained in separate working evidence directories. Headless pixel inspection is not user approval, physical phone/OS keyboard/AT speech, native WebView2, cloud synchronization, model evaluation or new Core-native execution. Parent owns affected Tauri/native gates. Service-crash recovery v2 is deliberately not connected until its separate backend gate passes.

## Stable selectors for integration

- Composer: `plugin-policy-open`, `plugin-policy-dialog`, `plugin-policy-mode`, `[data-plugin-id]` checkboxes, `plugin-policy-apply`, `plugin-policy-close`.
- Catalog: `plugins-tab`, `plugin-refresh`, `plugin-list article[data-plugin-id]`, `plugin-import` (file input); outside descriptors have no enable control.
- Project notes: `continuity-open`, `continuity-dialog`, `continuity-instructions`, `continuity-decision-add`, `[data-decision-id]`, `continuity-reference-add`, `[data-reference-id]`, `continuity-save`, `continuity-reload`, `continuity-close`.

The new UI controls need review in the parent's combined release. No Root/E primary source, runtime, index, archive, provider credential, grant, GUI or deployment was modified here.
