# Bounded adapter history repair and current Wiii overlay

Neko Aurora now offers explicit same-session history repair when its incremental adapter projection is insufficient. The available Wiii source can read that bounded projection without registering a replacement session, resending user intent, changing its nonce or granting execution. Pending requests and exact accepted receipts remain separate from observed reply rows. Revoked output stays masked; a reload never releases cached bodies solely because an empty delta arrived.

## Exact source and validation

| Item | Identity | Evidence / result |
| --- | --- | --- |
| Neko production | e6ae06c57a8b8afe09492c25a8e33a8f30eceaa6 | New ledger/API projection and capability only; store, old delta, Core, host and desktop unchanged |
| Neko tested code | e56718073dbbdd134220ed29251a4a4ae1053b53 | Go544 top-level +542 subtests, vet and VCS-stamped build PASS; two optional tests skipped |
| Control executable | SHA256 6f90bb4de7340f8055960b2f6147fdddbd5b5ab39080e044708498cca6d21e6c | root/final-e567180/bin/neko-control-e567180.exe; vcs.modified=false |
| Wiii source | d2e8284b34d2431e21e4ca8df41c82b3270d4a50 from43dd366 | Isolated source-43dd, 13 reviewed paths, clean; original C checkout unchanged |
| Wiii materialization | Historical943-file source manifest + current43dd overlay +13-path delta | All historical source bytes verified; installed/running source pairing UNKNOWN |
| Wiii actual-source checks | Node25.9 TypeScript modules |29 PASS, including new8-artifact/80-UTF8-byte bounds, storage compensation and held reload |
| Wiii native module | Rust1.97.1 full current locked/offline Tauri crate |17 PASS; separate real service test exercises the previously ignored case |
| Wiii frontend | Final d2e8284 materialization | TypeScript noEmit and Vite build PASS; E-only runner loader adaptation recorded |
| Genuine current pair | Go e567 + Rust test executable + TS d2e |8 PASS;513 synthetic accepted unanswered requests,100 original receipts retained, read phase12 GETs across initial repair and reload, journal unchanged |
| Independent exact-pair QA | Retained source harness, resumed independent owner |17 exact-Git TS cache/ACL fixtures and two Go API families/13 nested subcases PASS; no broad suite repeated |

Primary receipts: `E:/CodexScratch/adapter-history-repair-20261013/root/final-e567180/results.json`, `E:/CodexScratch/wiii-current-repair-20261013/root-code-checkpoint/receipt.json`, `root-bounds-final/typescript/result.json`, `root-bounds-final/native-current-result.json`, `root-frontend-d2e8284/{tsc,vite}-result.json`, and `genuine-pair-e567-d2e8284-r2/results.json`. All commands, environments and hashes are recorded there. The shared [frozen contract](adapter-history-repair-contract-20261013.md) and [validation detail](adapter-history-repair-validation-20261013.md) explain the bounds and causal failures.

Independent final receipt is `E:/CodexScratch/ux-independent-20261013/repair-current-pair-e567-d2e-v1/results.json`. The interrupted pre-offline command produced no directory/process; after reconnection it ran once and completed exit0. It reads immutable Git modules and uses authored pages/isolated API fixtures, with independent portable encoding. Authority changes before final confirmation, mixed generations, corrupted or failed storage, held reload, unconfirmed intake, native connection changes and navigation changes all fail closed. Its evidence remains separate from the genuine Go/Rust/TS service check and native UI acceptance.

The new endpoint uses at most128 rows/900,000 JSON bytes per page,8,192 rows/64MiB canonical history per repair and five minutes. Larger histories fail explicitly without truncation/deletion. A signed cursor binds the exact source/session/thread/channel/owner/space, frozen head and authority witness; its random signer is separate from the HTTP authentication cookie. Final authority is checked after candidate persistence and before active-cache publication. This is bounded local projection recovery, not unlimited history or Core memory reconstruction.

Wiii v1 arrays retain their100-item policy. New immutable content-addressed shards separate pending input, exact accepted receipts and history; each shard retains a100-entry/900KB bound. Publication uses a small checked active pointer and compensates failed storage writes. The actual plugin store uses a plain file write, so logical publication and fail-closed corruption handling do not establish physical power-loss atomicity. The UI offers an explicit repair action and a100-row presentation window; unknown rows never manufacture original request bodies or ACKs.

## Preserved failures and limits

The current-source43dd baseline retains8 PASS/4 RED. Independent portable framing/digest math passes9. Neko's first full combined run retains one obsolete capability-list assertion; the final one-line test update accepts the intentionally advertised second capability. No production guard was relaxed. Wiii's old regression expecting a reply observation alone to promote uncertain input is retained as53 PASS/1 RED; the current typed-ACK requirement is an explicit semantic correction. Its completed54-case regression remains separate from the29-case new-repair family.

The first genuine-pair runner expected the legacy first200-event delta to reject513 total rows. The source actually supplies an incremental page; that mistaken observation and the failed round remain preserved. Round2 corrects only the observation and signal-based process-closure bookkeeping. No product edit or journal fabrication was used. The513 current service rows are waiting-host requests with empty output, while the separate ledger fixtures prove513 completed selected-file replies and legitimate escaped-output page packing.

No live service or installed app was restarted, and no installed Core settings, real user history/account, credentials, grants, provider, cloud, deployment or push was used. Original archives, Main's prior edits/media, previous sealed packets, original Wiii checkout and historical build artifacts remain protected. Main receives a source-only fast-forward after independent QA and preservation checks; Wiii stays a separate source candidate. Temporary executor disconnection blocked the first finalization attempt; reconnection checks verified exact source/binary hashes before resuming, without repeating completed broad builds/tests.

Remaining acceptance gates are supported native owner-dialog/WebView interaction and installed binary/source pairing; physical phone/keyboard/IME/AT/safe-area; physical crash/power-loss storage durability; real provider/model/OAuth/account broker and device/cloud/multi-owner synchronization. Formal Go race is blocked by the unavailable C compiler. No unsupported automation route is used to close these gates.
