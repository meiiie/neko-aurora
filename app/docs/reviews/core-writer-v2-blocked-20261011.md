# Private Core writer v2 — BLOCKED, do not promote

Exact source commit: `7b396c85d726680c43067e88aca03d0c4950e3b0`.
Tree: `c5697d228c32cd72650ae52e53c0a452c74de778`.
Fresh verified baseline: `80b1394952bfd4594bd3cf9ff850b101ea762e12`, derived from
Core snapshot `3d5725f222815e15b12a212e372cf17057a459c0`. All 772 original files
still match their manifest; the original ZIPs and App source/pin were not changed.
This bundle preserves honest private Git provenance, not historical upstream Git objects.
The helper and Core remain AGPL-3.0-only; LICENSE and NOTICE are included.

## Deliverables

- `core-writer-v2-7b396c8.patch`: selective baseline-to-candidate patch; clean apply
  and all 780 resulting file bytes verified against the candidate Git tree.
- `core-writer-v2-7b396c8.bundle`: complete private Git history; bundle verify,
  fsck and independent restore completed; all 780 restored file bytes match.
- `helper-source/`: standalone Rust source and locked dependency manifest.
- `neko-writer-lease.exe`: 391168 bytes, nlink=1,
  SHA-256 `f9f404a07a7fe1aa56a2b7d3c073a2330366de5ee26d572b37421338e2b99a3a`.
- `neko-core-7b396c8-unpromoted.exe`: 94247936 bytes, nlink=1,
  SHA-256 `ee3581759739b48d28b83bd41b3d02a1ce70e334a5efd04141deb90dcb0cae9c`.
  PE copyright is AGPL-3.0-only. Its inherited 1.8.0 version is not a new published release.
- `validation.json`, `evidence/`, restoration reports and `artifacts.json`:
  exact commands/environment, original and redacted log hashes, PASS/FAIL/BLOCKED status.
  The final secret scan found no unknown credential findings and printed no values.

## Trusted opt-in contract

The trusted launcher passes both `NEKO_TASK_WRITER_HELPER` (absolute path to the
one-link copied helper) and `NEKO_TASK_WRITER_HELPER_SHA256` (exact lowercase SHA).
An absent descriptor retains v1. Invalid or partial opt-in fails closed.
Use the deploy copy, not Cargo's possibly hardlinked target cache.

Core launches the helper with a sanitized SystemRoot-only environment and private
stdin/stdout pipes. CLI arguments are `--directory`, `--session`, `--parent` and
`--parent-sha256`, each followed by its value. Private bounded JSONL acquire/release
uses version 2 and a token that stays out of arguments, ACP and model context.

The helper retains the actual parent kernel process handle and full creation FILETIME,
keeps the lock file open with Windows share_mode(0), rejects reparse/hardlink files,
and never unlinks a v2 lock. A held v2 lock is reclaimed only with exclusive file
ownership and proof the exact old process identity is dead or has been replaced.
Legacy v1, corrupt, empty, partial, unknown, permission-denied or live-owner locks
remain fail closed. Canonical decimal FILETIME validation rejects fake PID reuse
through leading zeros or plus signs. Release fsyncs state and closes ownership
before ACK; Core awaits it only after runtime quiescence and close.
See `docs/process/TASK-WRITER-WINDOWS.md` in the restored source for the full contract.

## Validation and blocker

On native Windows x64, Bun 1.4.0 and Rust/Cargo 1.97.1:

- PASS: actual helper kernel ownership tests, 4 tests / 75 assertions, including
  concurrent writers, helper crash while Core lives, invalid/legacy lock denial,
  release/reuse and forced Core source-coordinator kill with same session/task,
  durable messages and activation +1. This uses a deterministic runtime fixture.
- PASS: typecheck, lint, doctor/policy with explicit isolated-profile warnings,
  offline locked helper build/fmt, full Core build, compiled UI probe, real ConPTY
  input, ACP smoke, startup lifecycle and all four startup input probes.
- FAIL: the full four sequential Bun shards: 2096 pass, 26 skip, 2 fail.
  Both failures are EPERM on atomic rename over an imported session JSON at
  `src/shared/atomic.ts:18`; original bytes are retained. One fails before any
  loopback provider request. The same EPERM was reproduced using original baseline
  production runtime at 80b1394. Its specific kernel sharing/permission cause is
  still unproved. It is not attributed to sandboxing. No retry-until-green, blind
  unlink or weakened persistence guard was added.
- BLOCKED: candidate promotion and root-owned pinned App ACP recovery acceptance.
  Identify/fix the import persistence failure and repeat full gates on the resulting
  exact commit before promotion. Existing App Core pin remains unchanged.

The intermediate 17421bb had a green full gate but a subsequently found canonical
FILETIME defect. Its evidence is preserved; it is superseded and must not be used.
Initial isolated-home Rustup failures and resolved installed-toolchain attempts
are both recorded. Skips are not platform acceptance. No live model/provider,
cloud sync, deployment, grants, public push or device UX acceptance was performed.
