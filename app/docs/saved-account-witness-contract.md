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

Still required before runtime activation: a Core witness reader, private
Rust/Core bootstrap dispatch, typed Work and explicit consent/source-policy
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
