# Neko Bot: private report into the requested local channel

In a shared channel, write `@Meiiie gửi báo cáo đã làm ở cuộc trò chuyện riêng vào kênh này`. Neko Bot selects that bot's permitted completed report and delivers its result, safe provenance and authorized report file into the original channel. A unique result needs no picker or reapproval. If ambiguous, answer the short question with a subject such as `Atlas`; the original durable human request remains the authorization.

Application: `2a740296e0711989d82601ac09a247bf2a5fa86c`. The documentation-only delivery commit, binary pins and launch paths are in `.checkpoints/local-report-delivery-release/launch.json`; exact receipts/hashes are in `validation-final.json` and `manifest.json`. This is a local-owner checkpoint, not whole-product acceptance.

## Implemented boundary

A separate ReportDelivery keeps the original command, stable bot, destination audience revision, nonce, result hash and provenance. One fsynced event binds intent and result or minimal question. It does not copy source Work into a new task or invoke a model to transfer a completed result. Private request/context/history/resources/operations and unrelated files are excluded. Instructions inside the report remain content and cannot authorize tools/recipients.

The local session represents one `local-owner` in `local-space`. Cookie/Host/Origin/strict JSON reject forged actor/source fields. Called bot and current source/resource/context access are checked separately from destination membership/revision. Archived private conversations can supply permitted output without reopening. Report projection/export/replica cursor/download recheck current authority. Global bot profile and owner-readable Work snapshot/SSE remain existing boundaries; this is not whole-stream per-audience privacy.

New publications use completion order, including the newest128 eligible candidates; access edits do not change latest. Internal policy2 records the rule. Genuine older candidate journals without that field retain historical intake ordering and original receipts. Malformed legacy commands become inactive metadata under the strict current parser. Compatibility creates no new publication or result change for an old nonce. Unknown policy versions/orphan fields are rejected.

Exact retry preserves the original result even after newer completion. Changed body with the same nonce conflicts; other command/intake nonces cannot take it over. Reload uses read-only receipt/snapshot reconciliation. Unknown outcomes offer explicit same-body/nonce retry, never automatic POST. Late ACK cannot change navigation, clear a new draft or steal focus. Valid append-only text from known running Work does not mask reports or fetch a snapshot per token; authority/status changes, unknown fields and cursor gaps still fence before resnapshot.

## Fresh evidence

| Gate at clean application2a74029 | Result and scope |
|---|---|
| Offline Go | PASS428 top-level/472 subtests; vet/build PASS. Optional reference-DOCX/browser-seed tests are skipped by the default suite; gated typed browser seed is separately exercised by the browser runner. |
| Compiled Go + Chrome | PASS79:47 source checks and32 actual browser gates. Binary VCS2a74029/modified=false; all4 served raw assets match. Exact request/report file/short clarification, ACK-loss/reload/duplicate, invalid outbox, stale authority, draft/navigation/focus and390x260 are checked. Legacy two-tag source checks PASS23 separately. |
| Independent selected journal/API | PASS20 top-level/66 test/subtest entries across two commands: unchanged parser negatives, report-only projections/revocation, nonce fencing, strict HTTP, concurrency/replay, completion order/newest128 and hash-valid forged policy headers. Five pre-contract or mis-scoped assertions have explicit replacements; the whole failed original suite is not relabeled PASS. |
| Genuine old producer compatibility | Exact old3ac/4ebe binaries write journals through real HTTP201. New reader preserves old safe result/duplicate, selects latest completion for a new nonce, retains malformed Reporter inactive, and opens each journal three times. No fabricated journal/Core/provider execution. |

The packet retains parser/completion RED logs, failed browser harness rounds and corrected scopes. Older evidence keeps its commit/working-tree identity. Current PNGs/MP4s are actual Chrome served by compiled Go. Completed reports are authored synthetic seeds, not evidence that a real model wrote them. Streaming10-frame checks are separately authored event fixtures, not latency/load measurements.

Root inspected desktop/mobile/short/clarification PNGs and video contact frames. Approved light layout and **Neko Bot** name remain. Owned fixture service/browser exited. Original17 ZIPs,1068 protected files, five older packets and inherited raw edits are checked after Main fast-forward. No source bundle/final ZIP/raw state/profile/vault/config/Library upload/public push/deployment is included.

## Launch and demo

From `E:\Sach\Sua\NekoBot`, verify without opening state or starting a service:

```powershell
.\scripts\run-prebuilt-local.ps1 -Manifest .checkpoints\local-report-delivery-release\launch.json -Desktop -CheckOnly
```

An intentional local run uses that manifest and an explicitly chosen state directory. Default mode saves messages and waits for execution without a provider. Rust host/desktop/Core pins are unchanged, without a fresh native test claim.

Revision-named desktop/mobile PNG and MP4 demos are in `preview/cross-channel-2a74029-report-story-*`. [Design](local-cross-channel-report-design-20261012.md) records verified Grok/official pstack Feature/poteto-mode sources, both designs and ownership. [UI contract](ux-cross-channel-report-20261012.md) records durable dispatch/focus. [Gap matrix](product-gap-matrix-20261011.md) covers12 requirements/18 scenarios without treating missing cloud/provider code as merely untested.

## Remaining work

Published reports are not automatically future Core context. Selection is bounded deterministic matching, not semantic retrieval. Multi-human auth, external connector/recipient ACLs/transport, broker/OAuth/provider/model quality, cloud pairing/sync/object storage, rich source memory and portable/active recovery remain missing or deferred. Native Tauri, physical phone/keyboard/safe-area/AT speech, new-control user review, Wiii owner trace and formal Go race remain separate gates. No new account, grant or credential bypassed them.
