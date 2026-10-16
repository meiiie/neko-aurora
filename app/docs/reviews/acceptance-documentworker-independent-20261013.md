# Independent local document-worker acceptance — 2026-10-13

Scoped result: **PASS**, command exit 0, against immutable combined source `c9b41b182af3374e4682f4867d131a9ee6b93231`. There were 6 process/wire Go families with 10 nested cases, 6 ledger composition/authority families with 6 nested cases, 6 pinned real-executable protocol cases and 3 source-only wiring observations. All passed. Parent and nested Go events are reported separately rather than summed as independent cases.

Independent QA changed only its three new probes/runner and this review. It performed no production/index/Main edits, broad suite, API listener, Core execution, real profile/note/provider/grant, deploy, push or GUI/browser/native renderer action. Earlier UI and adapter-repair seals remain unchanged.

## Executed identity and command

- Clean source checkout: `E:/CodexScratch/office-worker-20261013/combined-source`.
- Exact code commit: `c9b41b182af3374e4682f4867d131a9ee6b93231`.
- Pinned executable: `E:/CodexScratch/office-worker-20261013/root-final-c9b41b1/bin/neko-control-c9b41b1.exe`.
- Executable SHA256: `0a9d8e002ed1a044f4a2145f94f60523a8f4197b40d77c1892a6e56fb8c99a3a`.
- Go build metadata was independently re-read: matching `vcs.revision` and `vcs.modified=false`.
- Published synthetic fixture: `E:/CodexScratch/office-worker-20261013/published-fixtures/ordinary.docx`, 813 bytes, SHA256 `f0355169286a15993e8dc0f54641f812a1e6ca482ca241a0dd24b0a7df8d135e`.
- Runtime: Node `v25.9.0`, Go `go1.27.2 windows/amd64`.
- Outer cwd: `C:/Users/Admin/Documents/Codex/2026-10-11/task/checkpoint-verify-20261011`.

```powershell
& 'C:\Program Files\nodejs\node.exe' .\scripts\acceptance_documentworker_independent_20261013.mjs --commit c9b41b182af3374e4682f4867d131a9ee6b93231 --executable E:/CodexScratch/office-worker-20261013/root-final-c9b41b1/bin/neko-control-c9b41b1.exe --executable-sha256 0a9d8e002ed1a044f4a2145f94f60523a8f4197b40d77c1892a6e56fb8c99a3a --fixture E:/CodexScratch/office-worker-20261013/published-fixtures/ordinary.docx --label c9b-v1
```

The runner exported 111 exact Git blobs to its new owned source tree, omitted all writer/existing `_test.go` files and testdata, and added only the independent probes plus the pinned synthetic DOCX. It ran these commands from receipt `source/native`:

```text
go test -json -count=1 -timeout=40s -run ^TestIndependentWorker ./internal/documentworker
go test -json -count=1 -timeout=40s -run ^TestIndependentDocumentWorker ./internal/ledger
```

The child received a named safe OS-variable allowlist and only receipt-owned profile/cache/temp/state/Go-cache paths below `E:/CodexScratch/ux-independent-20261013/documentworker-independent-c9b-v1`. `GOENV=off`, `GOWORK=off`, `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local` and `CGO_ENABLED=0` kept this local run offline. Environment metadata uses explicit named lookups; no inherited environment enumeration or real provider/credential variable was used. A deliberately published synthetic environment canary was created by one test solely to prove it was absent from the supervised child.

## Observed evidence

| Family | Result | What it establishes |
|---|---|---|
| Closed response combinations | 1 family / 7 nested PASS | Foreign digest, short body, multiple responses, rejection/foreign status with success material, oversized text declaration and invalid UTF-8 with matching digest refuse with no partial result. Independent Unicode text/digest round-trips. |
| Environment, format and Windows launch | 1 family PASS | Known synthetic/provider/proxy/PATH/profile variables are absent; TEMP/TMP/USERPROFILE belong to the call directory; the helper receives only the private argument; path/flag/URL/uppercase format choices refuse; hidden Windows launch attributes apply. |
| Streaming refusal and reuse | 1 family / 3 nested PASS | Exact 64 KiB stderr succeeds; stderr above that bound, stdout above 4 MiB and trailing responses refuse. Each refusal frees the slot for a subsequent successful helper. Stderr content is discarded. |
| Two slots, Close and reap | 1 family PASS | Two held owned children exhaust capacity; the next extraction refuses immediately without queuing. Close cancels/reaps both, signals their original Windows process handles, frees slots, removes the owned directory, and rejects subsequent extraction. Repeated Close is safe. |
| Deadline and reuse | 1 family PASS | A short authored two-second deadline cancels and reaps a held direct child; its original process handle is signaled and the slot is reusable. The production deadline constant remains 15 seconds. This is fixture headroom evidence, not a production service SLO. |
| Helper digest/path pin | 1 family PASS | A changed unlaunched owned helper copy refuses before execution and frees its slot. Relative helper/directory paths refuse. The production executable itself was never altered. |
| Mandatory constructor | 1 family / 2 nested PASS | Nil interface and typed nil worker refuse before creating Store state. |
| Parent complete-text identity | 1 family / 4 nested PASS | Authored extractors lying about hash, byte count, same-size changed text with an old witness, or UTF-8 are refused by the parent. Upload retains the existing acknowledged quarantine shape; later extractor success on exact retry preserves original receipt/ID/raw hash/status and journal sequence. |
| No configured fallback | 1 family PASS | The published DOCX first succeeds as a direct-parser control. The same bytes with a configured refusing extractor return that refusal and no text, proving the configured path does not fall back to the direct parser. |
| Preview old witness | 1 family PASS | Same-size changed extracted text with an old reported witness refuses preview without altering the original receipt or journal. |
| Held preview revocation | 1 family PASS | An authored extraction held off-lock allows a fixture file-access revocation; the final descriptor check refuses the late preview and creates no later journal/authority change. |
| Held upload project change | 1 family PASS | A held extraction permits a project revision change off-lock; the old upload then refuses at the final scope check without acquiring a file identity or adding a journal event. |

The two Go JSON streams contain 16 process PASS events and 12 authority PASS events: 12 top-level families plus 16 nested cases, with no FAIL/SKIP events. Ledger cases use authored extractor results and isolated fixture Stores. Their synthetic raw canary and fixture authority mutations are composition tests, not real user document or authority changes.

Six invocations of the actual pinned production executable passed. Its sole private mode accepts the published DOCX and emits one complete valid UTF-8 NDR1 response with a recomputed digest. Trailing path metadata, a second frame, an unknown format byte and an over-bound raw declaration each produce only the fixed 41-byte reject frame and exit 2. Adding a state argument exits 2 with no output before Store setup. Each owned case cwd was empty after process exit, before evidence files were added. All real-executable stderr streams were empty.

Three exact-source observations confirmed early dispatch before flag/Store setup, mandatory `NewSelf` plus `OpenWithDocumentWorker` composition, and shared upload/preview/pinned-tool extraction with the existing final epoch/context/source checks. These observations do not establish genuine Core tool execution; Root's separate Rust/Core fixture gate owns that claim.

## Finding resolved before immutable execution

During preparation, independent QA identified that the original working ledger seam trusted extractor-reported hash/length rather than recomputing complete text identity, despite the approved contract requiring the parent check. Root accepted the finding and held staging. The writer introduced central `validDocumentResult` verification of actual byte length, UTF-8 and SHA256, while retaining secret screening, bounds and failure/quarantine policy. Root reported separate retained causal RED/GREEN writer receipts. This agent did not execute mutable pre-fix source and does not present those writer receipts as its own validation. Its four immutable lying-extractor cases and preview witness case above independently verify the correction.

Before execution, the source-only observation was adapted to the final shared `Extract(format, raw)` call after the writer centralized the three format branches. Behavior assertions were unchanged. The single final independent round passed without a harness fix or rerun.

## Sealed raw evidence and quiescence

Receipt root: `E:/CodexScratch/ux-independent-20261013/documentworker-independent-c9b-v1`.

| File | SHA256 |
|---|---|
| `results.json` | `28842ec27d1dc6187a1a1af1a92847c4d8b94ac3f1ba71042acf4930b704f5d6` |
| `command.json` | `bf4626100d4a7fadbe2c2453840bfbb22f439a48e73c6374a930182eab808f93` |
| `manifest.json` | `d62c84b532fda92ccf0b3192493848fbf212555ab5d748231294dd34bf7d719a` |
| `go-process.jsonl` | `1c3c18b5f8ad3d80667ad6c6f8f718f473d278071d101f186db9fa0b83e9965f` |
| `go-authority.jsonl` | `65a1e58bd91848bad5a328ccd671cad18866e4d1ae8d1377b307b35330bcc019` |
| `binary-build-info.txt` | `0c14ed90ee26b9507d3f280f837391df434eb9316fceb78d946f46fafab27b56` |
| `source-quiescence.json` | `f31c85f27da0be8f00bb30156da26a5c889bd1218912410026f805c25e4c5a3e` |
| `seal.json` | `94c6c6793ab3362a64878bf389f3eb0331f134e2f3f1c6b9ebe40cf225db9663` |

The seal independently re-read every Git blob and extracted snapshot byte, reconfirmed clean source HEAD c9b, rehashed the executable/fixture, verified executable VCS identity, and confirmed all three retained probes exactly match their executed copies:

- Runner SHA256: `715a18b98a7f8e22ffbb92351fc9f9ad8b4a83b8d8c3f8c586e223d9892cff0d`.
- Process probe SHA256: `3b2938db5508bc1a1cae916eda51f391823f394643013e21f24f8ef1996242dd`.
- Authority probe SHA256: `9d048e8f6c133554fc08f77044f47dda092a14a1d2944dc0038caee186273060`.

The retained UI seal remains `b4afe90f905d79e55bdddba0c5d06f3d854fd0be15346f9c57dc94dcc14ae17a`; its tested app/index bytes match the combined snapshot. The repair seal remains `10f9720b8be8489db6b31da3f4e14b3566039584541c7a83cd38b2d9a103d292`. Neither prior business probe was repeated.

Remaining blockers: none for this bounded independent local scope. This is process/output/deadline/concurrency supervision, not a Windows restricted-token/egress/filesystem sandbox, hard RSS limit, hostile-input fuzz campaign, Core/model evaluation, native/phone/assistive-technology acceptance, cloud sync or power-loss proof. Root owns the separate final secret scan, Git checkpoint and integration.
