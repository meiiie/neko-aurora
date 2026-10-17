# Saved-account authority witness (default-off)

This checkpoint adopts the isolated Go witness library from the preserved
`origin/wip/saved-account-witness` branch. It does not activate saved accounts,
change runtime pins, add a provider loop, or wire task consent and admission.
Neko Core remains the agent loop. Saved accounts remain `stored_unverified`.

## Wire contract

A witness binds the exact saved-task binding digest, service boot generation,
authority digest, monotonic positive revision, issue/expiry timestamps and
`active` or terminal `retired` state. Active witnesses last at most five seconds,
never beyond the original binding deadline. A live controller also anchors a
monotonic deadline for each observed proof; repeated reads of the same revision
do not reset it, and a frozen wall clock cannot extend renewal. The revision key is private and
absent from the witness. Its derived HMAC key uses the domain separator
`neko-saved-key-v1/authority-witness`.

Canonical JSON has sorted object keys, exact integer numbers and no duplicate
keys. The MAC covers the body without `mac`; the signed record is canonical too.
The deterministic fixture binding hash and signed record in
`TestWitnessCrossLanguageVector` were independently calculated using Node's
SHA-256/HMAC and a sorted-key JSON encoder, not the Go implementation under test.
The revision key in that test is authored synthetic material.

Endpoint admission must preserve the literal endpoint identity, rather than
silently normalizing it. Production is HTTPS; fixtures use literal
`http://127.0.0.1:<port>/...`. Core's existing `URL.href === endpoint` rule rejects
missing paths, default or padded ports, uppercase host names, dot segments,
raw spaces and unescaped Unicode. Fixture ports must be nonzero; explicit ports must be at most 65535.
The focused rejection/acceptance vectors were also executed directly against
Core's exported `parseSavedAccountBinding` using Bun 1.4.0. This is representative
cross-language contract evidence, not an exhaustive WHATWG URL implementation
or a claim that every Go/Rust/Core parser edge case is now equivalent.

## Trust and remaining integration

Only the owning service supplies `CurrentTask`; nil, false or a panic denies
issuance and retires the controller. The controller detects live rollback,
deleted/replaced evidence, foreign boot/authority/key and terminal revocation.
It is single-service-owner local state, not cross-process arbitration or
rollback-resistant storage against an owner who restores an entire old state.
Only one live controller may own a witness path. Concurrent controllers are not
arbitrated: a stale controller can fail retirement after another controller
advances the file. Deleting a retired file and opening the same configuration
can look like first initialization. Runtime integration must enforce exclusive
ownership and define restart/tombstone provenance before claiming durable
revocation across such lifecycle transitions. The present library contract is
not complete for production activation.

Windows file-handle/reparse/hardlink checks remain intact. POSIX checks do not
prove Windows path identity or DPAPI behavior.

Still required before runtime activation: native Windows witness consumption, private
Rust/Core execution bootstrap, typed Work and explicit consent/source-policy
binding, current account/lease checks, reviewed capabilities and runtime pins,
and actual Windows end-to-end synthetic DPAPI/ACP acceptance. Changed-attempt
recovery must not create a fresh budget or reset consumed limits.

## Validation boundary

Linux can validate portable Go controller behavior and Core endpoint parsing.
The historical valid Windows Issue failure is still an open native gate;
this checkpoint does not claim to diagnose or repair it. Windows must establish
valid Issue/renewal/retirement, file identity negatives and real DPAPI-to-Core
private handoff before accepting this feature. No real credentials, providers,
external grants, deployment or account readiness are tested here.

## Checkpoint receipts (2026-10-16)

Base: `bc3def7007adf78b7c44549239b25d655a8e3d05`; adopted WIP source:
`fcc6608aefea3f7dfbbff78545f4f4374b730c11`. Results describe the uncommitted
focused change on that base, not a published release.

- Linux x64, official Go 1.27.2: `go test -count=1 ./internal/providerexec` PASS.
  Prior RED captured the placeholder vector, endpoint identity mismatches,
  over-budget opening and callbacks crossing witness expiry. The fix rechecks
  prior evidence after callbacks and refuses an expired witness on return.
- `CGO_ENABLED=1 go test -race -count=1 ./internal/providerexec` PASS.
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ./internal/providerexec`
  PASS (compilation only, no Windows execution).
- Bun 1.4.0: direct calls to the existing Core endpoint parser denied all 14
  authored negative endpoint vectors and accepted all three positive vectors.
- Independent review reproduced the delayed-callback defects before the fix
  and passed the corrected real 5.1-second callback, pre-write expiry and
  excessive-binding-lifetime checks afterward. A frozen-wall-clock renewal
  regression also reproduced RED before adding per-proof monotonic deadlines.
- The initial full Go suite exposed pre-existing Windows-only statebackup test
  failures on Linux. A separate portability checkpoint owns those tests and
  the final full-suite/vet result; this focused package does not claim that
  initial full run passed.

## Isolated Core consumer (default-off)

`core/src/adapters/saved-account-witness.ts` now exposes
`createSavedAccountWitnessAuthority`. A trusted caller supplies the exact binding,
private witness path, expected service generation and authority digest, and the
revision key. Focused integration tests pass the returned stateful callback
unchanged to both capsule preparation and bridge loading. The private
[Core preparation command](../../core/docs/process/SAVED-ACCOUNT-PREPARE.md)
now constructs this reader before preparing a capsule. Ordinary CLI/environment
selection and saved-account runtime execution remain disabled.

The consumer verifies the literal Go wire format and HMAC domain, canonical UTF-8
bytes (including rejection of BOM/duplicate keys), full binding identity, boot,
authority, safe integer revisions and the five-second maximum proof lifetime.
Readers may skip renewals during a pause when a newer authenticated proof is
still current; expiry of an old observed revision alone does not reject that
new proof. Repeated reads cannot refresh a proof deadline; the original
wall/monotonic mapping also bounds newer revisions with unchanged expiry. Any rejected read permanently
poisons that callback. Missing initial evidence is an error, not initialization.

On POSIX, nonblocking opens prevent substituted FIFOs from hanging the reader.
Bounded descriptor reads compare named/opened/post-read identity and
metadata, reject hardlinks and symlink ancestry, and recheck parent identities.
Atomic rename between successful reads is permitted because the Go writer renews
that way. This is local single-owner evidence, not an adversarial-filesystem or
whole-state rollback guarantee. Windows consumption deliberately fails closed
until native handle/reparse/final-path verification is integrated and executed.

The new focused tests consume the literal Go signed vector and exercise renewal,
rollback, retirement, frozen-clock expiry, expiry during a read, file replacement
inside a read, a FIFO substituted between stat/open in a bounded subprocess,
and actual loopback provider cancellation. A real bridge reload
with the same binding/state directory retains consumed request reservations;
renewal does not create another budget. Changed-attempt recovery remains outside
this slice: its full-binding budget key changes, so a future host must not admit
it by creating a fresh state directory or resetting consumed limits.

Linux x64 / Bun 1.4.0 acceptance is scoped to the isolated reader and existing
saved-account bridge tests. Windows native execution, private Rust/Core execution bootstrap,
restart/tombstone provenance and provider/account readiness remain open gates.

## Native acceptance handoff for the staged path

The internal host/Core implementation has progressed beyond the earlier reader
checkpoint; see [cloud evidence](../../docs/cloud-checkpoint-20261016.md) and the
[private ACP contract](../../core/docs/process/SAVED-ACCOUNT-ACP.md). Its public
activation is still off. Run the following from the repository root on actual
Windows, using installed Rust/Cargo, Go and Bun. Do not enter a real API key:
these cases use authored synthetic values and temporary DPAPI data.

PowerShell preparation for the existing DPAPI-to-Core fixture:

```powershell
if ($env:OS -ne 'Windows_NT') { throw 'Native Windows is required.' }
$env:FIXTURE_BUN_EXE = (Get-Command bun -ErrorAction Stop).Source
$env:FIXTURE_SAVED_CORE_MODULE = (Resolve-Path 'core/src/adapters/saved-account-bridge.ts').Path
cargo test --locked --manifest-path app/host/Cargo.toml --test executable_guard --test saved_task_witness --test saved_key
if ($LASTEXITCODE -ne 0) { throw 'Native host acceptance failed.' }
cargo test --locked --manifest-path app/host/Cargo.toml --lib saved_account_process::native_tests
if ($LASTEXITCODE -ne 0) { throw 'Native broker acceptance failed.' }
Push-Location app/native
try {
    go test -count=1 ./internal/providerexec ./internal/runtime ./internal/ledger
    if ($LASTEXITCODE -ne 0) { throw 'Native authority acceptance failed.' }
} finally { Pop-Location }
```

Use a fresh terminal for these fixture environment variables; close it afterward.
Record the exact source commit, tool versions and complete outcomes. A zero-test
result, skipped platform case or cross-compilation does not satisfy this handoff.
The expected broker scenarios include descendants holding pipes, blocked input,
cancellation, oversized receipts and nonzero exit; verify they actually execute.

These tests are necessary, not a complete release gate. A separate end-to-end
launch must prove the validated Go envelope, durable admission acknowledgments,
private Rust preparation and the same Core ACP session cooperate under cancel,
account rotation and restart denial. Only then should the existing public gate be
considered for activation. Real provider/account acceptance remains separate and
requires the owner's secure handoff; fixture success never proves GLM access.
