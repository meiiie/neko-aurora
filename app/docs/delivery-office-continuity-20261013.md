# Supervised Office extraction and scoped draft clearing

Office uploads, text previews and `project.read_file` now use a pinned copy of the running control-plane executable in a private, closed document-worker mode. Two active slots, a 15-second invocation context, streaming output bounds and owned-child reap contain parser failures. The parent independently verifies full UTF-8 text, byte count and SHA-256 before existing authority checks. Worker failure retains the original upload quarantine/receipt policy; configured production never falls back to an in-process parser. This does not introduce an OS filesystem/network sandbox or a hard memory/power-loss guarantee.

The continuity editor also offers **Xóa bản nháp**. It clears only the loaded project/channel editor, preserving its scope and CAS versions. It creates no command, nonce or POST. The existing explicit Save can commit the reviewed empty note. Pending reads, commands, changed scope and storage failure retain their guards; late responses cannot restore an old draft. This is editing, not history deletion or semantic forgetting.

## Identity

- Starting Main: `0188e2b2be3d9bcfe1f5637da7885251d738d70b`.
- Office source: `1a35716b4700050a5498d38c61782472401e95bc`.
- UI source: `66aada94f120a4c98f95585aa043ee0f1c2ab520`; independent UI report/probes checkpoint `9fa807ae1d3b12d39b827c9b68163c29099f609d`.
- Exact combined tested source: `c9b41b182af3374e4682f4867d131a9ee6b93231`, clean checkout `E:/CodexScratch/office-worker-20261013/combined-source`.
- Go executable: `E:/CodexScratch/office-worker-20261013/root-final-c9b41b1/bin/neko-control-c9b41b1.exe`; SHA-256 `0a9d8e002ed1a044f4a2145f94f60523a8f4197b40d77c1892a6e56fb8c99a3a`, VCS modified=false.
- A later delivery commit adds only verification scripts/reviews and this record. The packet verifies production-byte equivalence to the tested combined commit; the executable is attributed to c9b41b1, not restamped as that later commit.

## Fresh evidence

Windows amd64; Go 1.27.2, Python 3.13.7, Node 25.9.0. Runtime children use named environment allowlists and private E-drive state/home/temp, offline dependencies and loopback fixtures. Existing user profiles, accounts, files and live services are not acceptance inputs.

| Result | Check and exact evidence |
| --- | --- |
| PASS | Full combined `go test ./... -count=1 -json`: 557 top-level tests and 567 subcases; `go vet ./...` and `go build -trimpath -buildvcs=true ./cmd/neko-control`. Commands, logs, source hashes and binary stamp: `E:/CodexScratch/office-worker-20261013/root-final-c9b41b1/results.json`. |
| SKIP | Optional `TestExtractDOCXReadOnlyReference` and `TestReportDeliveryBrowserSeed`; no passing claim for those gates. |
| PASS | Actual compiled worker/service: 33/33, including DOCX/XLSX/PPTX, bounded large text, original bytes, preview/revoke, quarantine, duplicate receipts and restart. `owner-immutable-service-c9b41b1-r1/results.json` and `seal.json`. |
| PASS | Independent worker process/closed-wire tests: 6 families + 10 nested cases; independent parent composition/authority: 6 families + 6 nested cases; 6 exact executable cases and 3 source-only observations. `E:/CodexScratch/ux-independent-20261013/documentworker-independent-c9b-v1/results.json` and `seal.json`. Writer tests are omitted from the independent export. |
| PASS | Actual pinned Rust/Windows Core `project.read_file`: 10/10 on all three authored Office formats; extracted full-text digest, original-byte pin, Unicode text, context admission, completed stop receipt and exact duplicate intake. `E:/CodexScratch/office-worker-20261013/actual-core-c9b41b1-r2/results.json`. |
| PASS | Independent actual-source UI VM: 25/25 against exact 66aada9 bytes, including zero POST/nonce on clear and explicit Save through existing session/retained-command guards. `E:/CodexScratch/ux-independent-20261013/continuity-clear-ui-immutable-66a-v1/results.json`. |
| PASS | Actual compiled Go continuity API: 8/8, empty CAS save, original nonce duplicate, changed payload/stale revision rejection, read and restart. `E:/CodexScratch/continuity-clear-ui-20261013/actual-http-c9b41b1/results.json`. |
| BLOCKED | Formal Go race detector: no supported C compiler. Process/CAS fixtures do not substitute for it. |

Every path above is relative to the Office scratch root where its prefix is omitted. Exact arrays, environments and SHA-256s in the sealed receipts are authoritative.

## Retained negative evidence and limits

The baseline lacked worker mode. Independent composition review exposed lying extractor metadata; unchanged tests demonstrated false hash/size and changed suffix behavior before the shared parent validation. Those REDs and the corrected GREEN remain preserved. Earlier working 33-case service and inherited-environment receipts predate the final parent guard and remain historical.

The first Core composition runner compared the tool's extracted-text SHA to the raw ZIP SHA and failed. Inspection of the UTF-8 synthetic journal confirmed the returned text hash and original pin were correct. Round 2 changes only that observation: parse the typed tool result, recompute its full UTF-8 digest, compare it to `textSHA256`, and independently compare the pin to the original raw hash. No production patch was made for this runner failure. The initial failed round remains in `actual-core-c9b41b1-r1`.

These are local source/process/HTTP/Core transport checks with authored documents and controlled replies. They do not accept model quality, GUI/WebView rendering, physical keyboard/phone, AT speech, real provider/OAuth/key broker, cloud sync, hard OS confinement, or power loss. The current installed Wiii pair remains unknown; the earlier isolated Wiii repair candidate is preserved. Existing installed binaries/live services are not replaced by source import.

Main source integration must preserve the three pre-existing raw script changes, 17 ZIPs, 1509 protected original files, 47 untracked media/audit artifacts, installed Core hash, the sealed 791-artifact repair packet and previous 90-artifact packet. The integration receipt records the actual checks, not an assumption based on earlier runs.

Next local product seam: source onboarding/preloading and scoped manual draft recovery require a separate bounded design against the current cache/session and pending-command behavior. No draft backup, automatic restoration, purge or wider authority is implied by this delivery.
