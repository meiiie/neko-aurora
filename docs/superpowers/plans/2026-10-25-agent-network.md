# Scoped agent messaging implementation plan

> For agentic workers: use the subagent-driven-development workflow or execute this plan task by task. Preserve the prior Windows bubblewrap portability change.

**Goal:** Give Aurora an authenticated session directory and durable agent mailbox with explicit delivery and task states, FYI versus wake, bounded text attachments, and trustworthy human decision provenance.

**Architecture:** The existing Go ledger owns durable session identity, mail and context delivery. A typed reference on the existing Work makes wake admission atomic in the same journal record. The current runtime owns model execution; HTTP and UI adapt this capability without another inference loop or another database.

**Tech stack:** Go 1.27, existing JSONL journal, same-origin authenticated HTTP and SSE, existing JavaScript/CSS web UI. No new provider or P2P dependency.

**Spec:** The design and acceptance below synthesize `design-a.txt` and `design-b.txt` kept in the local build folder. The Reason source study is pinned to `5e995a580520d95e7fa130b3a2d1915364f2d746`.

## Design and synthesis

Two shapes were considered. A second durable mail service would isolate transport but introduce a second log, backup surface and admission/reconciliation crash boundary. Extending the existing replay-validated journal permits one `work.accepted` record to associate a message with exactly one Work. Choose the latter. Keep the alternative's narrow HTTP/service API, explicit actor boundary, scoped file download and wake quota checks.

The user authorized applying the researched ideas. Implementation proceeds without a further design checkpoint. This is a design choice by the implementer, not a claim that the user selected individual internal types.

Durable session IDs bind the Store instance, owner, space, bot, conversation and device. Aliases do not authorize access and are never used to retarget queued mail. Capabilities reflect the actual Aurora adapter; browser presence does not confer runtime ownership. Ephemeral presence belongs in the service, not an fsync heartbeat in the journal.

Every network record is a strict typed event. Message origin is constructed from authenticated owner context or a live source Work. JSON cannot select tenant, actor, source Work, human-decision origin or a causal root. Source channel revisions and audience are checked at enqueue and dispatch. Unsupported principal modes fail closed.

`Work.PeerMessageID` is immutable. Dispatch uses a reserved deterministic nonce, disables text-based `@alias` rerouting and calls the existing intake under its lock. Applying the single accepted Work derives the message link. Ordinary intake cannot forge this reserved nonce or field.

FYI creates no Work and does not wake the runner. On a legitimate target turn, a delivery event binds FYI context to that existing Work. The same context is rebuilt after restart, including after the delivery acknowledgement. Peer Work is excluded from the owner steering inbox and rendered with its sender; it never receives the owner-only instruction label.

Delivery, task and external effect receipts are different facts. Human decision provenance comes from the existing linked DirectApproval and DirectDecide path. An agent's text cannot create a human decision or executor grant. Generic mailbox UI acknowledgement means read by the user, not read by the model. The existing effect and vault approval gates remain authoritative.

Inline coordination files are UTF-8 text only, at most eight files, 32 KiB each and 64 KiB total. Message text is at most 16 KiB. Reject unsafe or hidden names and invalid content before durable acceptance. These limits fit the existing 1 MiB journal frame even with JSON escaping. DOCX, PDF and PPTX continue to use the existing scoped artifact path. Context uses those payload limits and binds at most one new FYI per safe turn; download returns the complete permitted text.

Root/hop and pair limits are based on journaled message provenance so restarting cannot reset an agent loop budget. Retries return the original message and do not spend another delivery or model admission. Expiry and archived/revoked audiences are checked again before work is admitted.

The current hosted login and local session boundary are the transport authority. This implementation does not install Reason hooks, change global MCP settings, use its friend-admin roster or expose its OS-user-trusted IPC remotely. Independent peer federation, direct iroh transport and physical-device acceptance are separate contracts and will not be claimed from a local test.

## Global constraints

- Preserve Tauri UI, Rust host, Go control plane and Core runtime ownership.
- Reuse the existing one-active-turn-per-bot scheduler and current Work recovery.
- Keep state, credentials, binaries and reference clones out of Git.
- New HTTP input is strict JSON with bounded bodies; every read, write and download checks its effective scope.
- User-visible copy distinguishes queued, delivered and completed.
- Do not deploy or push as part of local implementation.

## Review focus

1. A text containing another bot's alias must not change its explicit destination.
2. A crash after mail acceptance or FYI delivery must retain both identity and context without duplicate Work.
3. A peer request cannot be folded into owner steering or supersede an unrelated pending owner approval.
4. Two tenants with the same bot IDs and aliases cannot read, acknowledge, wake or download one another's mail.
5. FYI and duplicate retries cannot bypass or accidentally consume the hosted model quota.

## Task 1: Journal-owned identity and mailbox

Files: new ledger network types, events, methods and tests; narrow hooks in `store.go`, `projects.go` and route validation.

- [x] Write failing tests for stable session identity, replay, nonce conflict and exact addressed routing.
- [x] Implement strict network event validation and copied projections.
- [x] Add atomic `PeerMessageID` Work admission and immutable replay checks.
- [x] Test FYI context binding, expiry, archives, channel revision changes, read-only restore and persisted causal limits.
- [x] Run the focused ledger tests and the existing ledger suite.

## Task 2: Authenticated transport and existing runtime

Files: new Aurora network service/routes/tests; small hooks in service, turn, history and steering; authenticated API/Hub integration.

- [x] Write failing HTTP and fake-model tests for wake, FYI, provenance and restart.
- [x] Adapt authenticated actor scope into the ledger and expose directory, mail, read, wake and downloads.
- [x] Integrate wake dispatch into the existing runner and durable FYI context into existing turns.
- [x] Exclude peer Work from owner steering and preserve explicit message origin in history and views.
- [x] Derive approval provenance from existing Work decisions without a new grant type.
- [x] Test hosted tenant isolation and wake metering, including duplicate requests.

## Task 3: Agent tools and user interface

Files: network tool adapter and web network assets with focused hooks into the existing shell.

- [x] Expose scoped list, send, inbox and status tools to a running agent Work.
- [x] Provide a directory/mailbox view, FYI/wake composer, text attachments and distinct delivery/task status.
- [x] Use attributed peer bubbles and the existing approval UI for actual effects.
- [x] Verify keyboard, focus, escaping, mobile layout and visible errors in the real browser.

## Task 4: Integrated acceptance and documentation

- [x] Run a controlled provider fixture with real server processes and two client contexts.
- [x] Restart around mail acceptance/delivery and prove one Work, preserved unread context and correct status.
- [x] Exercise tenant/channel denial, unsafe file rejection and source/decision spoof rejection.
- [x] Build Windows and cross-compile Linux, run Go vet and the complete Go suite.
- [x] Compare any full-suite failures to the recorded pre-existing Windows baseline.
- [x] Run `tools/check_docs.py`, inspect the final diff and retain concise exact-command evidence.
- [x] Build and open the implemented local candidate for user review, preserving the prior local state.

## Local audit artifacts

The append-only decision trail and command logs live under `app/.build/network/`. They contain no credentials. Prior baseline failures are recorded in `app/.build/validation.json`; they must not be described as new feature regressions or silently omitted.

## Completion evidence

Implemented on `feat/scoped-agent-mailbox`, base `ffab4500e3a7abab9e22d3d5375527190ff8f08b`, with uncommitted source changes. Exact local commands and limitations are in app/.build/network/validation.json. The final binary passes 13 process-level acceptance checks. Desktop interactions, literal HTML display, Escape/focus restoration and responsive layout were reviewed; 390px render fixtures are static DOM/CSS snapshots, not phone acceptance. The current live pane also renders the responsive layout.

The full Windows run retains the 11 recorded baseline failures; a later concurrent full run additionally hit an unchanged document-worker timing failure, which passed three isolated repeats. After final startup recovery changes, all affected runtime/API/Hub/command package tests were rerun, with only their seven baseline failures. No all-green full-suite or provider-quality claim is made. Docs check still finds two pre-existing missing asset links.

The user-facing process runs on the copied app/.state/network-live state, preserving app/.state/local. Live Meiiie-to-Davin messaging completed after a retry with explicit TOOL fields; the initial model attempt did not send. The new adapter feeds recognized invalid-field calls back as tool errors. The reference clone remains excluded from Git. No deployment, push or global agent configuration changes were made.
