# Independent adapter repair pair acceptance — 2026-10-13

Scoped result: **PASS**. The single resumed pair run completed with exit 0: 17 exact-Git TypeScript module fixture cases, two local Go API/ledger families, and their 13 nested subcases passed. No production source, index, Main checkout, installed profile, listener, grant, provider, deployment or push was changed or used by this QA task.

## Exact source and command

- Neko code commit: `e56718073dbbdd134220ed29251a4a4ae1053b53`.
- Wiii code commit: `d2e8284b34d2431e21e4ca8df41c82b3270d4a50`.
- Frozen contract SHA256: `672c2eed6afcc6f82622f6bd0584fd38cb52c2917dcdfcbb3cdbd89989bdfd59`.
- Command cwd: `C:/Users/Admin/Documents/Codex/2026-10-11/task/checkpoint-verify-20261011`.
- Execution environment: Windows amd64, Node `v25.9.0`, Go `go1.27.2 windows/amd64`.

```powershell
& 'C:\Program Files\nodejs\node.exe' --experimental-vm-modules .\scripts\acceptance_adapter_repair_pair_20261013.mjs --neko-commit e56718073dbbdd134220ed29251a4a4ae1053b53 --wiii-commit d2e8284b34d2431e21e4ca8df41c82b3270d4a50 --label e567-d2e-v1
```

The Go subprocess ran `go test -json -count=1 -run ^TestIndependentAdapterRepair ./internal/api` from the receipt's `neko/native` directory. Its source was exported from the pinned Git object, with the retained Go probe added only to that owned extracted test tree. The archive's Git export attribute differences were verified as CRLF-only and restored to exact object bytes before execution. The full TS modules were read from the pinned Wiii Git objects and executed in a VM; no fragment rewriting substituted for production functions.

The child was created with a named OS-variable allowlist. It used only receipt-owned `profile`, `cache`, `temp`, `gocache`, `gomodcache` and `gotmp` directories beneath `E:/CodexScratch/ux-independent-20261013/repair-current-pair-e567-d2e-v1`. `GOENV=off`, `GOWORK=off`, `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local` and `CGO_ENABLED=0` kept this local run offline. `command.json` records the exact allowed names and owned paths; no provider or credential variables were inherited.

## Observed checks

| Family | PASS | Observed behavior |
|---|---:|---|
| Baseline failure families | 4 | 100 accepted identities retain original receipt/nonce metadata while pending capacity stays separate; 409 and the actual TS native-client connection error hold presentation; an invalid mixed clear frame leaves no revoked old body visible. |
| Authority, cursor and generation | 5 | Cross-session continuation, mixed witnesses, independently detected body tampering and terminal authority denial refuse publication. A source-unavailable row replaces an old body at the same real sequence without fabricating a sequence. |
| Storage and reload | 4 | Candidate or active write failure preserves original receipts and keeps presentation held. Reload plus empty delta cannot unhold cached bodies. Changed shard content refuses hydration. |
| Pending, navigation and native client | 4 | Observed history does not promote an original queued intake or authorize resend. Navigation and TS native-generation changes fence publication. Missing repair capability remains a distinct refusal. |
| Go denied request forms | 9 nested | Empty, duplicate, unknown and encoded query forms, noncanonical raw path, body and chunked body refuse; JSON POST reaches route-level 404; foreign origin refuses. The journal sequence and original accepted Work/nonce remain unchanged. |
| Go witness replay boundaries | 4 nested | Cross-session and retired-signer witnesses refuse; terminal confirmation returns zero rows with unchanged digest; a live authority epoch change refuses. GETs do not mutate the journal or accepted Work/nonce. |

The two Go parent-family PASS events are additional to their 13 nested PASS events, not 15 independent cases. The 17 TS cases use authored transport/storage responses and independently encoded synthetic history witnesses. The actual TS native client is invoked with an authored `invoke` callback; these cases do not establish real Rust IPC or native plugin storage acceptance.

The disconnect-during-save case deliberately completes the authored active-pointer write while the TS native generation changes. The bridge still rejects with `connection_changed`, hides presentation, and requires new repair after reload. The active pointer's existence is not reported as physical deletion or a storage transaction guarantee.

The panel's lifetime callback, bounded reply window, unavailable caption and absence of repair-triggered draft clearing were inspected in exact source. Those are source observations, not interactive browser, installed Wiii, phone, composer-flow, model-evaluation or cloud-sync acceptance. Root's separate genuine Go/Rust/TS 513-intake receipt remains separate evidence and was not rerun here.

## Sealed raw evidence

Receipt root: `E:/CodexScratch/ux-independent-20261013/repair-current-pair-e567-d2e-v1`.

| File | SHA256 |
|---|---|
| `results.json` | `f3ad5e82997a963b877b2db8db4261915cd81e68684f17a9275740a9018cee2e` |
| `command.json` | `620b2e3c831141955a86fcbd4792d9f36cc0f1da7951920d68be8ecb3adcbdba` |
| `manifest.json` | `5732350c2dff94d087d6a14775007a4ce4a5f02e7b168ccb039e14b5b999fa4f` |
| `go-test.jsonl` | `b42adce6cf46d52084d4ba58b55125bd2f83423e6070375a6b1806bf066a1054` |
| Executed `harness.mjs` | `4467771a5f3ffd068182fbc2eb6053ddd43f8e78e7a2ddf6389153d630c7a671` |
| Executed `probe.go` | `9d621a4e9c19e1f3d56655bdc7bf71b14d4176fc86e65d49403b3991c6c55146` |
| `source.tar` | `89cc34e8d15fd0e2173a498acfc8ef06b61b0578abe1ff418dd42ca69732f69c` |
| `source-quiescence.json` | `5c514fd438c5684523536ca6b0da8a611a43fe99d1e2ed58601e184d531c88b6` |
| `seal.json` | `10f9720b8be8489db6b31da3f4e14b3566039584541c7a83cd38b2d9a103d292` |

The seal independently re-read all 278 exact Git blobs and extracted files, verified each byte count and SHA256 against the executed manifest, and confirmed both pinned HEADs. Wiii was clean; Neko had no native worktree delta or staged delta. Neko native tree was `f7ddf63b3c263ff453a8fb9859a0579b102f1141`. Since production commit `e6ae06c57a8b8afe09492c25a8e33a8f30eceaa6`, the only native delta at this tested HEAD was `native/internal/api/adapters_installation_intake_test.go`; production implementation bytes were unchanged. Root-owned documentation drafts remained outside these execution inputs.

## Retained receipts and metadata hygiene

The interrupted attempt had no pair receipt directory and no Node/Go process before this one run started, so no uncertain probe was duplicated. Standard sandbox reads initially returned a CIM access denial and an E Git not-worktree diagnostic; the narrow escalated read returned the pinned clean Wiii checkout. These were infrastructure diagnostics, not source failures. No infrastructure artifact or old failed/RED receipt was overwritten.

The previous baseline remains 8 PASS / 4 RED at `repair-current-baseline-eaeb-43dd-v3/results.json`, raw SHA256 `b2b95f6358eb72c6000c94715803d947d5557f058fc6623ff5c76f11f7db912e`. The independent canonical-vector receipt remains 9 PASS at `repair-current-vectors-a0af-v1/results.json`, raw SHA256 `9d334374b9b4b13a768d94e3a1837d87d940cc395702d97ba6118e90c05912ca`. They retain their original scope and results.

After execution, the retained `.mjs` received one metadata-only edit: child metadata now lists explicit known environment names using named lookups instead of enumerating `process.env`. The executed child already received the named allowlist; its original metadata also named Windows-injected OS keys and contained no inherited real-provider environment. The immutable v1 evidence continues to bind to the executed `4467771a...7a671` hash. The retained future harness SHA256 is `09577b8c8379d00c8d59b9d74c6bc177bd4a505aa506adc363552d947a3c3c30`; its `node --check` passed. The seal verifies this is the sole retained-versus-executed harness change. Business tests were not rerun or attributed to the new hash. The retained Go probe remains at the executed hash above.

Remaining blockers: none for this targeted independent local receipt. Root owns the separate secret scan and Git checkpoint; this QA task performed no Git writes.
