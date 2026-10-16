# Independent adapter repair / available Wiii acceptance

Current status: baseline RED on immutable Neko `eaeb54efe55ec0450a6b85a43b853d6032abfed4` and available Wiii `43dd366c22f2b2af51e9dbf1f99f4fbd7bd5138f`. Four authored families contain twelve checks: **8 PASS / 4 RED**, exit 1. The repair implementation/schema and immutable repaired revision pair are pending. These observations execute the full current protocol, bridge and native-client TypeScript modules. They are not historical C9 snippet tests or installed Wiii acceptance.

Only the new independent runner and this review were written. Existing acceptance runners/reviews/packets, implementation, Git/index/Main and other repositories remain outside this writer's scope. Root owns the shared contract and integration.

## Reproducible baseline

Run from `C:/Users/Admin/Documents/Codex/2026-10-11/task/checkpoint-verify-20261011`:

```powershell
node --experimental-vm-modules scripts/acceptance_adapter_repair_current_wiii_20261013.mjs --neko-commit eaeb54efe55ec0450a6b85a43b853d6032abfed4 --wiii-commit 43dd366c22f2b2af51e9dbf1f99f4fbd7bd5138f --label baseline-eaeb-43dd-v3
```

Evidence: `E:/CodexScratch/ux-independent-20261013/repair-current-baseline-eaeb-43dd-v3/`. `results.json`, `manifest.json`, `command.json`, exact Git source bytes, harness copy and stdout/stderr are retained. The result records thirteen immutable files / 169094 bytes and confirms their Git-object hashes unchanged. It never reads working implementation files for execution.

| Receipt | SHA256 |
| --- | --- |
| Runner | `d8d3d53a8608796903559f3294f875cd71e65a67588380cd765ec633e227af4f` |
| `results.json` | `b2b95f6358eb72c6000c94715803d947d5557f058fc6623ff5c76f11f7db912e` |
| `manifest.json` | `8313f0a693ef239b544a2c6873d4a312b235db9c082a257517ae7588a5bfd4a1` |
| `command.json` | `975cd80647e1f256cecb6fca4570bd2356e2e45f7224f6a7356bb2077ce8a33f` |

The initial `...-v1` packet stopped at an incorrect harness path for the panel and produced no product checks. It remains preserved and excluded from results. `...-v2` completed 8 PASS / 4 RED with the same assertions. The final v3 launch also explicitly sets `HOMEDRIVE`/`HOMEPATH` to owned E scratch; the change concerns execution isolation, not test expectations.

Execution uses Node `v25.9.0`, Windows x64, `node:module.stripTypeScriptTypes(mode=transform)` and `vm.SourceTextModule`. Complete protocol/bridge/native TS Git modules link together in one VM; there are no copied function snippets. Transport and persistence are authored in-memory fixtures. A thirty-second subprocess bound and one-second module evaluation bounds apply. No external package installation or network transport occurs.

The child is launched with named Windows system/path variables plus owned `USERPROFILE`, `HOMEDRIVE`/`HOMEPATH`, `APPDATA`, `LOCALAPPDATA`, `TEMP`/`TMP`, empty `NODE_OPTIONS`, warnings disabled, and read-only Git configuration flags (`GIT_CONFIG_GLOBAL=NUL`, `GIT_CONFIG_NOSYSTEM=1`, `GIT_OPTIONAL_LOCKS=0`). Windows adds ordinary process identity/system variables; actual names are recorded in `results.json`. All home/cache/temp paths resolve under the fresh E packet. No account/profile/store/installed settings/history directory is read. No live8765, registration, execution POST, provider/Core/model/grant/bootstrap or installed native invocation occurs.

## Causal RED and preserved intent

| Family / check | Result | Evidence and implication |
| --- | --- | --- |
| Accepted history / new intent after100 accepted | RED | A valid saved snapshot has100 accepted receipts, zero pending rows. `enqueue` for message101 rejects `outbox_full`. Accepted history exhausts the pending budget before the requested >512-history journey. |
| Accepted history / refusal preserves receipts | PASS | Snapshot, original100 request IDs/nonces and cursor remain identical; no POST. |
| Conflict / limit409 safe metadata | PASS | An authored GET409 makes `http_409`, holds presentation and preserves outbox/nonce/cursor. |
| Conflict / explicit repair operation | RED | The actual full bridge exposes no `repairHistory` operation. This is a baseline availability observation before the new method/schema is agreed. |
| Native drift / old text hidden | RED | After a successful refresh, native-client invoke rejects the exact Rust generation error string `neko_bot_local:connection_changed`. Bridge reports `transport_unavailable`; both old replies remain visible. |
| Atomic page / wrong-thread rejection | PASS | A page with correct other fields and foreign `sourceThreadId` rejects `reply_binding_mismatch`; no partial cursor/cache/receipt mutation. |
| Atomic page / mixed clear and unknown receipt | PASS | A valid bound page first clears known message1 at sequence11, then includes unrecorded same-session message999 at sequence12. `reply_message_mismatch` correctly refuses to fabricate a receipt/nonce or commit the first row partially. |
| Atomic page / observed clear hides old content | RED | That same refused page leaves old message1 text visible at cached cursor10. Holding presentation must be independent of committing a partial cache. |
| Atomic page / final storage failure and reload | PASS | Refused persistence publishes no provisional replacement text and keeps prior intent/receipts. Fresh hydrate hides cached replies until live verification. |
| Encoded budget / admitted single row | PASS | One65536-byte valid UTF-8 control-text row is393671 encoded page bytes, below900000. This does not claim an impossible >900KB raw single production reply. |
| Encoded budget / aggregate frame refusal | PASS | Three individually valid rows encode1180461 bytes, exceeding the current1MiB frontend frame. `response_oversized` leaves cache/intent unchanged. This is an authored receiver-bound fixture; the real backend900000 gate would intervene before returning that frame. |
| Capability rollout / exact current catalog | PASS | Adding an undeclared capability field to the actual catalog parser rejects `unsupported_fields`; capability rollout needs an explicit compatible parser/schema change. |

The native-error fixture executes actual current `NekoBotNativeClient.transport` and the complete bridge, with an authored invoke rejection matching the inspected Rust error. It does not execute native IPC. The mixed-clear fixture is a valid typed same-session transport page, not a claim that an actual ledger emitted it. No original nonce is manufactured, replaced or resent by these probes.

Neko owner's separate ledger fixtures cover genuine >4096 unrelated events and >512 fanout/row gates. This runner does not duplicate those scenarios or present its synthesized409 as genuine journal evidence.

## Exact current-source seams

| Source | Git SHA256 | Relevant immutable lines |
| --- | --- | --- |
| `nekobot-local-protocol.ts` | `3be090938d7ff2a309d79754ad0210e818fd923cb0c8601a0ea7156235bfea9f` | Exact scope/session/page parsers,512-page rows /1MiB bytes. |
| `nekobot-local-bridge.ts` | `c9238701fbc160cf153c782f5b164c795e54469ae224abdacbad1cc7d62e633e` | 39–40 cache/outbox100;77 generic error;145 enqueue100;208 receipt refusal;223 status-only authority hold;235 accepted rows retained;240 presentation gate. |
| `nekobot-native.ts` | `2702fc919b0fc06aba14555361da1e5100be153d704e9a926c9a1f801033cbe4` | 13 exact catalog parser;54 lease-bound native transport, no structured native-error conversion. |
| `local_bot_bridge.rs` | `408fc78931d5bcbeedb0398eaf654a3e97167d9d4fc69ada2cf1bef685200c8c` | 427 hydrated POST verifies old `/replies?after=0`;527 discards non2xx bodies;566 route variants;618 sole old GET route. Generation checks reject changed connections. |
| `NekoBotLocalPanel.tsx` | `7707ac32327cc46671c02d26835fd5d128f2b4abb24b0e11ee2efdfefba67fbf` | 79 refresh action, no repair action;88 blocked wording says reconnect channel;generic catch masks denial versus repair limit. |
| Neko `adapters.go` | `5fee31621c672acddfc13eb5b06e39a339263880970ea56a39d0608dc0d3acc8` | Existing clear lag4096,512 rows and900000 encoded JSON bounds inspected, not changed. |

Rust and React conclusions above are source inspection only. The main-window capability file allows existing local status/connect/disconnect/link/request commands. The new read-only history path belongs inside an exact session-bound native allowlist; widening arbitrary URL/query/body access is unnecessary. A valid history verification also needs to establish the same native adapter provenance without depending on the old potentially blocked `after=0` verification.

The available43dd overlay lacks `wiii-desktop/package.json`, `tsconfig.json`, the panel's `composer-drafts.ts`, and the `tray.rs` referenced by native `lib.rs`. No full current frontend/native build or installed-runtime pairing is established from this incomplete overlay. A separately manifested materialization can be assessed when Root supplies it. Standalone TS modules executed successfully; Rust/native UI/React interaction remains BLOCKED.

## Next immutable pairing gate

Root approved bounded repair:128 rows /900000 encoded JSON bytes per page;8192 rows /64MiB total; five-minute lifetime; explicit413, no truncation/journal rewrite. The proposed read-only `/history` path, opaque cursor, capability and final schema are still awaiting the agreed contract. This is bounded history recovery, not unlimited transcript import or Core memory recreation.

Independent follow-up will use the new immutable Neko/Wiii revision pair and a few causal families:

1. Cross-session/scope/cursor/page tamper, mixed repair generations and final-manifest/digest mismatch; no provisional transcript or fabricated original nonce.
2. Mid-page revoke/regrant, server restart/expiry and final authority confirmation; old revoked text/files remain held, receipts and pending intent remain intact.
3. Interrupted pages, final storage failure and reload; staged pages never become the active manifest. Accepted receipts beyond100 coexist with the unchanged100 pending-intent bound.
4. Late completion after navigation/new draft, capability unavailable/denial/error distinction, legitimately admitted encoded-output limits and exact native allowlist/generation fences.

The canonical digest rule must define exact interoperable bytes. Go's default JSON escaping of `<`, `>`, `&`, U+2028/U+2029 differs from `JSON.stringify`; incoming object-key order is not authority. Unicode/control/HTML/field-order vectors are a required contract check before final manifest publication. A server-lifetime random cursor key independent of the cookie and a live authority witness that detects revoke/regrant are Root's agreed design constraints.

Native Tauri/CUA/browser/physical phone, installed Wiii pairing, cloud/OAuth/device/provider/model-quality acceptance remain explicitly BLOCKED or out of scope. No GUI workaround was attempted.
