# Pure Rust context admission helper

This slice adds `host/src/context_admission.rs` without wiring it into `main.rs`, `lib.rs`, the manager, transport, Store, or Core. The base is `3309dd3ded89dad671d8d3dd9b193f795896123f`. The production integrator owns negotiation, live ACL and reader checks, launch-envelope size, process lifetime, publication fences, and persisted receipt handling.

## Interface

`LaunchContext::parse(&serde_json::Value) -> Result<Option<LaunchContext>, ContextError>` accepts legacy absence as `None`. Production must require `Some` for a newly negotiated context-enabled launch. Partial context fields, null protocol, and mixed fresh/resume fields fail closed.

Fresh launch fields are `contextProtocol: 1`, `contextManifest`, `contextManifestSHA256`, and `contextPinSHA256`. The manifest and both hashes must agree. Fresh mode rejects `contextExpected` and a present `resume` field.

Resume fields are `contextProtocol: 1`, `contextExpected`, and the existing saved `resume` object. The strict expected receipt must match that saved object's `sessionId`, `task`, and `execution`. Resume rejects every fresh manifest/hash field; no manifest selection or rebind occurs.

`session_meta()` returns only the additive `neko.contextProtocol: {version: 1}` and fresh `neko.contextManifest` or resume `neko.contextExpected`. The caller merges those two keys into its existing session metadata, retaining `neko.taskExpected` and `neko.executionExpected` on resume. The helper never supplies or replaces those task/execution expectations.

`admit(&admission, &Binding) -> Result<AdmittedContext, ContextError>` reads `_meta["neko.contextAdmission"]`. That receipt has exactly seven keys: `version`, `sessionId`, `contextPinSHA256`, `manifestSHA256`, `sourceIds`, `task`, and `execution`. It requires version 1, the existing Binding's exact session/task/execution values, lowercase hash identities, and exactly one opaque 64-hex Core-issued source identity. This identity is not any manifest source's ID and is never inferred from those IDs.

Fresh admission requires the parsed launch's pin and manifest hashes. Resume retains the previous session, pin, manifest hash, and native source identity plus stable task ID/root and authority/policy IDs. Current activation must match the supplied Binding. The integrator must also run the existing `Binding::valid_resume` epoch/prior-receipt checks; this helper does not replace them.

`AdmittedContext::receipt()` exposes the validated immutable receipt. `meta(&Binding)` preserves the existing `Binding::meta()` task/execution pair and adds its exact context receipt. Use this metadata on task-bound prompt, close, and action calls. `echo(&Binding, &params)` requires the existing exact task/execution echo and identical context receipt on inbound tool calls, rejecting parallel `neko.context*` keys. Other established metadata, including host-effect operation metadata, remains the caller's responsibility.

`manifest_digest(&Value)` computes the bounded frozen Core canonical hash. `ContextError` reports `Protocol`, `Manifest`, `Receipt`, `Binding`, or `Bound`; error output contains no source excerpts or credentials.

## Frozen Core contract

The comparison source is unchanged Core commit `15654176bc73021030959ca0baabbfe63499c6ce`:

- `src/core/context/admission.ts`: SHA256 `88e64909bee6260bb1a732422f745d52b20b69f0353a0185868b6dc59fa54de5`.
- `src/shared/wire.ts`: SHA256 `571e844e3eb33368f0a00094e4f1455a783379e18a937ea77c38ecbf44a7d79a`.
- `docs/process/CONTEXT-ADMISSION.md`: SHA256 `e5ae061ee64d4c207ef881a92ec27cd8446253ddd31c97fcc107f8f507340d15`.

The schema enforces exact fields, safe integers, exact common scope, primary sender equal to owner, at most 16 sources and 64 unresolved candidates, a nonempty primary of at most 32768 UTF-8 bytes, and exact nonempty selected spans of at most 8192 UTF-8 bytes. The aggregate manifest bound is 49152 encoded bytes. Dates retain explicit canonical Gregorian/UTC values or null; entity descriptors and unresolved candidates never confer user authority. Retrieved content remains descriptive; the pure decoder cannot prove a live read grant or observation.

The hash sorts object keys by UTF-16 ordinal order, preserves array order and literal Unicode, and uses JSON.stringify-compatible escaping. It does not apply HTML escaping or replace literal `\\u2028` sequences. Integral JSON numbers normalize as JavaScript numbers, including negative zero. Invalid fractional or unsafe schema integers fail closed.

## Verification

The standalone Cargo integration harness imports the new file by path, so the module is compiled while production remains unwired:

```powershell
$env:CARGO_TARGET_DIR='E:\CodexScratch\context-admission-rust-3309dd3\target'
$env:TEMP='E:\CodexScratch\context-admission-rust-3309dd3\temp'
$env:TMP=$env:TEMP
$env:NEKO_CONTEXT_FIXTURES_DIR='E:\CodexScratch\context-manifest-builder-3309dd3\evidence\core-fixtures-final'
$env:NEKO_CONTEXT_CASES_PATH='E:\CodexScratch\context-admission-rust-3309dd3\evidence\rust-core-vectors-final.json'
$env:PATH='C:\Users\Admin\.rustup\toolchains\1.97.1-x86_64-pc-windows-msvc\bin;'+$env:PATH
Set-Location host
cargo test --offline --locked --test context_admission
```

The external Go export test explicitly reports when no fixture directory is supplied. The recorded final validation sets it and asserts all 11 unchanged exports from the accepted Go builder `fb4187fe85ca6112f0c1d620d3829a59ed388e30`: 10 actual manifests and one ordinal hash value. Portable Unicode/ordinal goldens run without that directory.

The Rust vector test exports 21 valid/invalid bounded schema cases when `NEKO_CONTEXT_CASES_PATH` is set. An owned evidence-only Node probe calls the unchanged actual Core parser/digest on those cases and the frozen Go fixtures, and verifies the three Core source hashes. Its 35 gates distinguish parser agreement from Core runtime acceptance.

The initial RED log retains the missing new-module error and an unrelated missing compile-time supervisor fixture in the sparse checkout. Only that unchanged tracked `scripts/core_supervisor.py` was materialized; neither production source nor the existing assertion was modified to hide the failure.

Exact final commit, commands, tool versions, result counts, hashes, secret scans, and selective patch are recorded in the accompanying E scratch handoff. Offline Rust tests and actual TypeScript parser agreement do not establish native context negotiation, live reader proof, provider/model behavior, phone/cloud acceptance, or a production runtime adoption. Those remain integrator gates.
