# Private saved-account preparation

The trusted host may invoke Core with exactly:

    neko saved-account prepare --json --broker-stdin

This is capsule preparation only. It does not construct a runtime, call a provider,
execute tools, grant consent, or enable saved-account ACP execution. Go/Rust runtime
activation remains separate and disabled. Windows witness consumption remains
unsupported until native handle/reparse identity verification is implemented.

## Private transport

Use a private stdin pipe, close it after writing, and never place credentials,
revision keys, paths, or the envelope in argv or logs. Core accepts at most 16,384
UTF-8 bytes and requires EOF within two seconds of starting its stdin read. Input
must be canonical JSON: recursively sorted object keys, compact JSON encoding,
no duplicate fields or trailing data, and at most one optional final LF. BOM and
invalid UTF-8 are refused. Every preparation failure exits 1 with empty stdout and
only `Saved account preparation unavailable` plus LF on stderr.

The exact object fields are:

- `version`: 1
- `purpose`: `saved_account_task_v1`
- `binding`: the existing complete saved-account binding, including fixtureOnly,
  account/revision/generation, provider/endpoint/model, request/attempt/epoch/fence,
  context hash, consent/revision, expiry and limits
- `secret`: the private saved credential material
- `capsulePath`, `stateHome`, `revisionKey`: existing capsule preparation inputs
- `witnessPath`: absolute canonical path to the live authority witness
- `serviceGeneration`: exact executor boot identifier
- `authoritySHA256`: expected authority digest

Core checks the closed schema and binds the live witness reader to the binding,
boot, authority digest and revision key. The existing bridge checks that same
reader before and during publication. Witness failure never falls back to local
profiles, global credentials or environment keys. The trusted host must supply
and protect all inputs: this local pipe is not an authentication protocol for
untrusted callers who can choose their own keys and witnesses.

Private dispatch runs before ordinary argument validation, terminal output,
configuration loading, home credential discovery and updater startup. Extra,
duplicate, reordered or missing command arguments are refused; so are all present
`NEKO_PROFILE*` variables and `NEKO_CREDENTIAL_HOME`, including empty values. Clear
these from the child environment rather than mixing profile and account flows.
A nonthrowing probe uses the ordinary argument grammar to recognize prefixed
private commands without mistaking prompt text or flag values for a command.
Other ordinary CLI invocations retain their existing behavior.

## Public result and lifecycle

Success exits 0 and emits exactly one JSON receipt plus LF, with only `version`,
`capsuleId`, `bindingSHA256`, `expiresAtUnixMS`, and `fixtureOnly`. It contains no
credential, private path, revision key, or witness material. The existing bridge
creates encrypted capsule and authenticated current-marker files outside the
state home. A receipt is not permission to execute, and does not establish
provider connectivity or credential validity.

The host remains responsible for witness renewal/retirement and private-file
cleanup. A preparation that fails during publication may leave private artifacts;
do not reuse them as a successful receipt. Restart/tombstone and whole-state
rollback guarantees are outside this slice. Never infer saved-account execution
support from successful preparation.

## Focused evidence

`bun test test/saved-account-cli.test.ts` exercises the real CLI in subprocesses:
public-only preparation, no provider calls, isolated invalid home configuration,
malformed/oversized/duplicate input, extra fields, missing/tampered witness,
wrong key, mixed flags/environment, and input without EOF. Windows behavior is an
explicit denial test, skipped on POSIX. Adjacent bridge, witness and profile-auth
suites protect the reused boundaries. Synthetic local fixtures are not live
provider, desktop-host integration or Windows native verification.

The exported internal envelope function also accepts an optional exact
`nativeWitness: {executable, executableSHA256}` descriptor and uses the shared
native witness factory with awaited cleanup. The current CLI deliberately rejects
this property until native acceptance. See [internal ACP startup](SAVED-ACCOUNT-ACP.md).
