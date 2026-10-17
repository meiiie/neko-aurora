# Native saved-account witness consumer

`openNativeSavedAccountWitness` is a trusted-launcher prerequisite, not an active
CLI or ACP route. Windows preparation still fails closed through the existing
entrypoint. Native Windows handle/reparse execution evidence remains required
before activation. No provider request, consent, or execution permission follows
from opening this reader.

The caller supplies the existing binding/boot/authority/revision-key pins, one
absolute normalized witness path, and an absolute canonical helper executable
with its expected SHA-256. The executable is checked for aliases, regular-file
identity and hash before spawn; its installation and ancestors must remain protected against
replacement by the trusted host/platform installation policy. Hash-to-spawn is
not atomic: this adapter alone does not establish immutable execution identity
or validate Windows ACLs. That guarantee is a separate activation prerequisite. No PATH lookup, shell, model selection, or ambient credentials are
used. Windows receives only SystemRoot; no revision key is sent to the helper.

The helper is launched once with `--saved-witness-reader`. Its private stdin uses
compact NDJSON: an initial `{version:1,path}` object, then sequential
`{version:1,sequence}` reads starting at 1. Init has no reply. Each read returns
exactly `{version:1,sequence,witness}`, where witness is the original signed UTF-8
record string. Input frames are at most 8192 bytes including LF; replies are at
most 24576 including LF, and witness material is at most 4096 bytes. Duplicate
fields, extra fields, invalid UTF-8, unsolicited or extra frames, mismatched
sequences, EOF, diagnostics and subprocess failures permanently poison the
session. No reply or helper diagnostic is forwarded to public output.

Core awaits and validates the first response before exposing authority. The
original Core verifier retains HMAC, exact binding/boot/authority, revision
monotonicity, proof expiry, wall-clock regression and monotonic task deadlines.
The helper only supplies verified file bytes; it never decides authorization.

## Changed observation contract

This asynchronous reader samples on a 50ms cadence with at most one outstanding
request. Each response and every authority use must remain within 100ms of the
request send time. An independent observation deadline poisons the reader even
when no consumer calls authority; a suspended event loop cannot revive stale
observations. These are bounded observation/detection guarantees, not synchronous
filesystem verification at every use. Direct POSIX consumption retains its
existing synchronous semantics. Host cancellation must still stop execution.

Failure stops/kills the helper. The returned `close()` poisons authority and waits
for child close/reaping; callers must await it in their session cleanup. It is
idempotent. There is no fallback to profiles, home credentials or the POSIX reader.

The focused subprocess tests use a checksum-checked synthetic executable on POSIX
to exercise this real transport and cleanup. They do not verify Windows native
file identity, provider operation or runtime activation. Rust's real helper
rejects non-Windows execution; there is no production fixture switch.
