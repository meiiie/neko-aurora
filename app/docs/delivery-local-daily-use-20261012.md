# Neko Aurora: local daily controls and same-channel collaboration

This slice adds usable local bot/project controls, scoped source inspection and public history downloads, durable metadata retries, literal Office text extraction, and a private local account editor. The display name is **Neko Aurora**. Two leading tags in an already-open shared channel now select the existing finite collaboration, with the first bot coordinating three stages. This is a local implementation checkpoint, not whole-product acceptance.

The application and delivery commit, binary hashes and launch paths are bound by `.checkpoints/local-daily-use-release/launch.json`. The new revision-named binaries do not overwrite the backup or earlier releases. The trusted Rust host, desktop shell and Core pins remain unchanged. No production state, original ZIP or existing local edit is reset. No source bundle, final ZIP, Library upload, public push or deployment is created by this delivery.

## Implemented behavior

| Slice | Behavior | Boundary |
|---|---|---|
| Bot lifecycle/defaults | Archive/restore and exact revision/account/model preferences for new requests; old work, files and history remain. | Requested model configuration is not verified provider execution. |
| Project lifecycle | Archive/restore blocks new intake/upload/group while preserving old pins, bytes and inspection. | No project-wide destructive action. |
| Sources/public export | A task shows source type, pinned revision and current usability; shared/project JSON and text downloads contain public goals, replies and file metadata. | No target-private history, raw context, raw file bundle or credential export. |
| Metadata retry | Exact typed intent is persisted before POST; reload uses read-only receipts, and unknown retries are explicit with the original body/nonce/revision. | Browser metadata durability is local, not remote identity or phone lifecycle acceptance. |
| Office documents | XLSX/PPTX join DOCX and plain text. Literal/cached values keep original-byte and extracted-text provenance; extraction happens outside the ledger lock before final authority checks. | PDF, legacy binary Office, macros, active/external content, OCR and advanced workbook semantics remain unsupported/quarantined. |
| Connections | Create/edit account metadata, preserve the sealed value on a blank edit, or rotate the exact local secret through Windows DPAPI. An existing account can open a temporary inline private card. | Accounts stay `stored_unverified`. A successful save is not provider readiness, OAuth consent or an execution grant. Pending account edits are page-lifetime metadata; secrets are never kept for automatic retry. |
| Leading tags | `@Meiiie @Davin Review the evidence` starts one finite group in the current shared channel. First tag coordinates; the other reviews; only the final coordinator writes the result. | Two distinct active members only, three stages/five minutes, explicit account or no account. No default-account fallback, arbitrary fanout or cross-channel delivery. |
| Challenge simulator | Default-off typed API binds task/attempt/account/destination/purpose and expiry; public receipts omit its one-use proof. Original and supplemental nonce/replay defects are closed. | No simulator UI, native owner dialog, credential consumer or real grant. `simulatorOnly=true`, `grantsExecution=false`. |

## Evidence and execution identity

The packet keeps original negative rounds separate from their corrected results. Every receipt records its own code/binary identity, command and scope. A dirty UI receipt is not relabelled a clean application run. The final validation receipt binds the clean combined build, original Go/Rust/Core pins and its selected checks.

| Check | Recorded result and scope |
|---|---|
| Full Go runtime at `be808ab` | PASS408 top-level/458 subtests. Runtime Go files were committed; UI was dirty, as recorded. This is historical scoped evidence, not the final clean combined test. |
| Clean `4a5383a` | Offline vet/build PASS. Real Go/Chrome/Rust DPAPI Connections PASS43; actual Office HTTP upload/preview/download/pin/revoke/reopen PASS23. |
| Genuine Core at `4a5383a` | PASS22 with two identities, three admitted stages, prior-result context, private/other-channel exclusion, one public goal, final-only file write/hash, foreground while a group Core is live, cancel/stopped proof, restart3 and audience revoke. |
| Independent simulator review | Original4 tests/16 leaves close at `be808ab`; the supplemental direct/shared intake composition remained RED there. Both unchanged probes independently PASS5/18 at `c714afe`. Creation-limit replay has a separate Root RED/GREEN, including valid resolution/reopen at the128 limit. |
| Working @tag UI | Actual source-function and compiled Go/headless Chrome gate, with exact asset hashes. It covers pinned group body, single-tag compatibility, ordinary internal mentions, one goal, committed ACK loss, receipt-only reload, explicit exact-body retry, draft/navigation/focus and390x260. Final clean build verification is recorded separately. |

Clean `4b9ffe7` passed Go410/460, tag49, finite-group native22, Office HTTP23 and the private Connections reopen suite21. A further genuine Core Office check then exposed the read-result hash mismatch; those passes do not hide its RED. The minimal producer fix is application `f8fff28b5d21f8250f5b6501e92834dba93477b0`: fresh full Go411/464 and vet/build PASS, followed by actual Rust/Core Office20/20. Both XLSX and PPTX return their admitted extracted-text hash and exact Unicode span through the unchanged seven-field Core observation contract; original ZIP downloads and raw-hash pins remain unchanged. TXT is a positive control and DOCX has the same corrected typed regression. The Office bridge was not run on the user's72-page document.

UI assets, Rust/Core/desktop code and all other product code are byte-equivalent to `4b9ffe7`, apart from that one Go read-result producer line and its new regression test. Earlier UI/native receipts retain their original application IDs; they are not relabelled as fresh `f8fff28` runs. The latest binary and full/native Office receipts are bound separately in the packet. The final delivery commit after application changes documentation only.

See the packet's `validation-final.json`, `manifest.json` and `preservation.json` for final results and hash verification. Source contracts and reproduction commands are in [Office integration](local-office-integration-20261012.md), [private Connections](ux-connections-secure-local-20261012.md), [leading tags](ux-tag-collaboration-local-20261012.md), [native collaboration](reviews/native-tag-collaboration-20261012.md) and [simulator boundaries](local-secure-challenge-simulator-20261012.md). The [verified Grok reference analysis](reviews/grok-reference-20261012/report.md) distinguishes client commands, coordinator routes, real adapter implementations and missing backend code; the bundle was not executed or copied into the app.

## Local launch

From `E:\Sach\Sua\NekoBot`, verify the new binaries without opening state or starting a process:

```powershell
.\scripts\run-prebuilt-local.ps1 -Manifest .checkpoints\local-daily-use-release\launch.json -Desktop -CheckOnly
```

For an intentional local run, pass the same manifest and an explicitly chosen state directory, then omit `-CheckOnly`. Without a separately reviewed runtime configuration, messages are durable and wait for execution; the launcher does not contact a provider. Controlled Core fixtures used by these checks remain private test state and are not shipped as product configuration. The simulator stays off.

## Remaining product gates

Cross-channel report delivery still lacks a typed backend command that separately checks source context and destination audience/file references. It must not be inferred from same-channel tags. Real provider/model routing, key broker/OAuth/connector consumers, semantic memory/compaction, destructive retention, portable/active restore and cloud identity/pairing/transport/object storage remain missing code or explicitly deferred work. No accounts, grants or cloud service were created to bypass those gates.

The new controls need user visual review; native Tauri interaction, physical phone/OS keyboard, nonzero safe-area, AT speech, actual five-minute group expiry, positive Wiii owner-dialog trace and formal Go race validation remain unaccepted. Headless geometry, a warm fixture timing and local process receipts do not establish those outcomes. Earlier release matrices/reports keep their historical identities; this document does not overwrite them.
