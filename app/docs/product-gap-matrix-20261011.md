# Neko Aurora: implemented local paths, missing code and acceptance gates

Current application: `f1af5e6d47020408b126e3fc357ae57c879419a2`, Core `5ad10a432b738bc26d53c6686c13994fcc5677b3`. [Published report Core context](delivery-published-report-core-context-20261012.md) adds selected-output reads, frozen group context and current revoke/projection guards, with genuine local Go/Rust/Core22 and independent selected29 evidence. Its exact source pairing and full Go/Rust/Core test scopes are recorded in the release packet. [Cross-channel report delivery](delivery-cross-channel-report-20261012.md) retains Go428/472, Go/Chrome79 and independent20/66 under its original2a74029 identity.

[Daily use](delivery-local-daily-use-20261012.md), [backup](delivery-local-backup-20261012.md), [conversations](delivery-conversations-retention-20261012.md), [bot/file/sync](delivery-local-bot-file-sync-20261012.md) and [older packets](product-local-current-20261011.md) retain their original identities and scope. Genuine Core Office20 belongs to application `f8fff28`, not a fresh run of this report slice.

All12 requirements and18 scenarios in the [direction](../reference/handoff/neko-development-direction.md) are mapped below. Missing code is distinct from an implemented path lacking acceptance on a real device/provider/actor boundary. No row is whole-product PASS solely because local fixtures pass.

## Requirements

| REQ-PB | Implemented local path | Missing product code/behavior | Separate acceptance gates |
|---|---|---|---|
| 001 Independent app | Tauri shell, Go UI/journal, Rust trusted host, hash-bound prebuilt launcher; default waiting_host. | Verified provider onboarding, signed installer/updater/rollback. | Current native UI, first real reply, OS/distribution/license. Launcher is not an installer. |
| 002 Core and Wiii split | Core alone runs semantic loop; admission/lease/stop/effect/recovery; separate Wiii ADE/bridge. | Complete owner transport/TUI renderer. | Positive Wiii owner approve/Cancel trace blocked by native tools. Historical native/crash receipts retain their versions. |
| 003 Multiple bots | Stable IDs/aliases/create/rename/profile; private conversations; bot archive/restore and model/account defaults for new intake. | Avatar/color editor, full memory-off/forget, destructive bot delete. | New-control review and semantic identity/memory. Defaults do not verify provider execution. |
| 004 Correct destination | Private/shared channels, tags, finite same-channel group; typed private-result to original shared-channel ReportDelivery/provenance/durable receipt. | Slack/Discord/email/webhook auth, verified recipients, multi-owner ACL and remote transport. | Authentic external thread/actor freshness and phone route. Local cookie represents one owner. |
| 005 Correct context | Context/account/audience/project/file/plugin/continuity/executor pins; source inspection; revoke fences; output-only report declassification; ContextPin5/selected admitted report reader/frozen group context and recursive derived-output revocation. | Editable/forgettable source memory, rich index/retrieval/person/conflict resolution; unrestricted large-file retrieval beyond the admitted prefix. | Semantic relevance/compaction/injection evaluation. Global profile and owner-wide Work SSE remain owner-readable, without whole-stream per-audience privacy. |
| 006 Autonomous coworkers | Two bots, finite3 stages/one goal/final writer, deadline/budget/recovery/cancel; two tags reuse it. | Natural-language team planner, dynamic assignment/strategy and richer budgets. | Real-model disagreement/clarification/sustained teamwork; fixed workflow does not establish arbitrary competence. |
| 007 Durable project/memory | Project/files/tasks/continuity/journal; private/shared/project JSON/text export; project archive/restore; encrypted service-state backup and isolated readonly restore. | Active/portable restore/migration, browser draft backup, purge/compaction/log retention and semantic memory. | Offsite DR/RPO/RTO and cross-device recovery authority. Restore never revives grants/jobs. |
| 008 Timely interruption | Foreground/background, linked supplement/replace, queued pause/resume, durable intent/read-only receipt, cancel fences; report stable across valid append-only text. | Active semantic pause, project-wide controls, priority/fairness/renewal/overload handling. | Sustained provider latency/capacity and external effects; authored streaming is not an SLO. |
| 009 Real files | Raw upload/result/hash/download; bounded TXT/DOCX/XLSX/PPTX literal text/original-byte provenance; scoped report files; backup bytes/mtime. | PDF/media/legacy Office/macros/OCR/advanced workbook semantics, parser process sandbox/quota isolation, richer editor/version/share bundles and orphan cleanup. | Physical OS/mobile file open and external object transport. Receipt is not readiness/current grant. |
| 010 Realtime desktop/mobile | Local SSE/GET-only replica scope/cursor/replay/restart; adaptive focus/touch drawers and headless mobile viewport gates. | Remote auth/device keys/pairing/relay/registry/commands, database/object-store adapter and phone/PWA/push lifecycle. | Two devices, OS keyboard/safe-area/background/AT/push/network SLO. Local sessions do not replace cloud/phone tests. |
| 011 Accounts/plugins/secrets | Rust DPAPI; stored_unverified account editor/selector/CAS and fixture leases; local plugin policy; default-off task-bound challenge simulator. | Real CredentialBroker/provider, OAuth PKCE/callback/refresh/renewal, owner challenge consumer, trusted egress, external runner/sandbox/cloud vault. | Real key/model consumption and grants deferred. Simulator grantsExecution=false; secret save is not provider connection. |
| 012 Few questions, correct authority | Bounded automatic ready-source/report selection; unique report needs no picker/reapproval; short subject clarification; nonce/CAS/read-only lookup/exact explicit retry. | Verified remote actor/recipient resolver, reconnect/remote approval, external compensation/private notification. | New-control review/AT/native and recipient/model quality. Technical autonomy creates no external grant. |

## Scenarios

| SCN-PB | Local behavior present | Still missing or unproven |
|---|---|---|
| 001 First use | Bot/default waiting_host/private account editor. | Verified provider/onboarding/first real reply. |
| 002 Separate bots | Stable profile/identity, fresh private lane, archive/defaults preserve old work. | Semantic memory/forget/identity and full memory-off. |
| 003 Thread A to B | Original destination; private result delivered into shared B; no late ACK relocation. | Authentic external thread/phone transport. |
| 004 Bot without access | Source/channel/bot guards; no alias/account/file fallback. | Real connector membership/auth UX. |
| 005 Owner impersonation | Cookie/Host/Origin, server-owned local principal and typed human command; no actor override. | Remote/multi-human/webhook authenticity. |
| 006 Two bots loop | Finite group/deadline/final writer/source pins. | Model-selected steps/sources/strategy. |
| 007 Disagreement | Contested continuity/fixed roles; report ambiguity asks missing subject. | Model reasoning about verification/preference/useful questions. |
| 008 Interrupt | Independent lanes, old context preserved through navigation/archive; unrelated text leaves report visible. | Sustained real-provider latency/capacity. |
| 009 Change output | Linked replacement and old-output fence. | Semantic in-run steering/content reuse. |
| 010 Cancel during send | Durable effect states/stop proof; no blind retry. | External API/send unknown outcome/compensation. |
| 011 Pause/resume | Queued controls and explicit V2/service/group recovery; readonly restore does not resume. | Active checkpoint/project pause. |
| 012 Conflicting sources | Dated/versioned/contested continuity/immutable pins and result provenance. | Semantic relevance/conflict/person resolver. |
| 013 Interrupted upload | Durable dedup/receipt/exact reselection, Office/raw identity and backup bytes. | Large/object-store resumable transfer/two devices/parser sandbox. |
| 014 Device switch | Local replica scope/cursor/restart. | Authenticated pairing/remote lifecycle remains missing code. |
| 015 Offline stale | TTL/CAS/hash, GET-only reload then explicit retry; report nonce preserves old result after new completion/replay. | Remote tombstone propagation/stale phone UI. |
| 016 Wrong/expired account | Exact ID/revision/generation, no fallback. | OAuth/provider identity/refresh/reconnect. |
| 017 Revoke mid-run | Source/plugin/audience/account/executor fences; report projection/file/export/cursor current checks. | External revocation/semantic sanitation. Already-read bytes cannot be retracted. |
| 018 Phone/AT/private notify | Focus/status/inert/reduced-motion/browser geometry. | Physical speech/keyboard/safe-area/screenlock/push privacy/authenticated file open. |

## Boundaries and next dependencies

Default mode saves obligations without a model. Controlled fixtures prove local protocol/journal/browser behavior. Real broker/OAuth/provider and remote identity/transport require product code, not merely tests. Accounts remain stored_unverified. Retention remains disabled, with no purge. Same-machine/user DPAPI backup does not establish portable recovery after profile loss.

The Core adapter and its exact runtime pin change in this slice; Rust host source stays unchanged but its binary is rebuilt with the embedded pin. Rust26, current Core11 unit/ACP, full Go and native22 gates have their own exact identities. Desktop/UI assets stay unchanged; full Core/Tauri/phone suites are not inferred. Historical HTTP admission/ACK causal RED/GREEN stays in auditv6. Simultaneous host+Go power-loss/storage failure, original179 pre-POST timeout attribution, formal Go race validation without CC, Wiii owner trace and physical phone/AT remain open.

1. The local published-context path is implemented. Next authority boundary: typed authenticated principal/device-session and canonical actor/device/scope/revision/expiry/nonce/payload-digest verification, default-off against independent peer fixtures. Production issuer/login, human membership and device bootstrap remain missing code. Source edit/forget and isolated parser quotas remain separate wholly local work.
2. Trusted provider/owner-challenge consumer before real keys. Simulator/receipt acceptance grants no execution.
3. Remote principals/device keys/revocation/pairing and connector recipient/audience authenticity before remote sync/publication.
4. Portable recovery keys, active restore checks, draft backup/schedules before offsite DR. Purge/compaction is a separate authorized slice.

No repeated stack, Core/Wiii split, light direction or local Git decision is required. Real provider/cloud credentials, grants and deployment remain deferred.
