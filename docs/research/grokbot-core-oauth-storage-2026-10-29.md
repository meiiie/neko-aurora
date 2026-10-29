# Core MCP OAuth Windows credential storage — 2026-10-29

Code commit `6487abbf25bca2123b3090e58c5ca652ed9e6087`, based on `884f3dfb2ae30c89120a7b40b0c81f458a8bae4e`. Solo pstack Feature / Prove It Works / Show Me Your Work, with self-review. Follows the [token exclusion checkpoint](grokbot-core-oauth-exclusion-2026-10-29.md); no independent reviewer or live-provider acceptance is claimed.

## Behavior

MCP token and client-registration records are now encrypted on Windows with [DPAPI](https://learn.microsoft.com/en-us/dotnet/api/system.security.cryptography.protecteddata?view=windowsdesktop-9.0) with [CurrentUser scope](https://learn.microsoft.com/en-us/dotnet/api/system.security.cryptography.dataprotectionscope?view=windowsdesktop-9.0). Additional entropy binds each encrypted record to its server/resource/issuer/redirect identity and filename. The fixed Windows inbox PowerShell helper receives JSON through UTF-8 stdin, including base64 payloads. Credential values never enter argv, environment or temporary plaintext files. The helper is hidden, has a ten-second deadline and bounded output, and inherits only the minimal Windows system environment. Crypto/ACL failures have no plaintext fallback.

Before publishing encrypted bytes, the helper sets and verifies an [owner-only inheritable directory DACL](https://learn.microsoft.com/en-us/dotnet/api/system.io.directory.setaccesscontrol?view=netframework-4.8.1). Newly written files inherit it. The existing atomic publication helper replaces the old record only after encryption and ACL setup succeed. Stored envelopes, decoded UTF-8, plaintext byte counts, base64 spelling, record size, file type and hardlinks are checked. Windows parent junctions are refused before storage writes or removal. This is not a hostile same-user path-race isolation boundary.

Legacy plaintext is migrated only after validation and a reread under the existing record lock. A concurrent consuming claim/removal cannot be undone by an old read. Invalid legacy data and failed migration remain unchanged, and no credential is returned after migration failure. Client registration now shares the record lock with tokens and invalidation. Non-Windows storage retains restricted-mode JSON.

Each provider caches only its current identity's two decoded records. Every read still checks the file and compares the exact encrypted bytes; rotation, corruption and removal cannot be hidden by the cache. Returned objects are parsed afresh. Discovery changes and invalidation clear the cache. This avoids repeated child launches during SDK connection without turning memory into the source of token ownership.

## Validation

Windows amd64, PowerShell 7.6, Bun 1.4.0, Node 25.9.0 and Python 3.13.7. Isolated HOME/USERPROFILE/XDG/npmrc, allowlisted child environment, synthetic records only. No real OAuth account, provider call, grant, push or deployment. Exact commands and outcomes are in `.audit/2026-10-29-oauth-storage/validation.jsonl`; decisions.tsv preserves framing, failures and solo review decisions.

- Initial three storage regressions FAIL on the previous runtime (`oauth-storage-162421.log`), then PASS with encryption and migration.
- Initial SDK run exposes repeated decryption startup cost: 43 PASS / 7 FAIL. Exact-byte caching resolves most cases. The remaining fixture combined two connections and took over five seconds with actual Windows crypto; its outer test budget is now 15 seconds while each connection retains the original five-second deadline. No response-limit, cancellation or replay assertion was removed.
- A separate Vietnamese/quoted/emoji path regression FAILS in `oauth-unicode-164447.log`. Explicit UTF-8 stdin decoding makes it PASS; the earlier complete ASCII-path gate did not cover that defect.
- Final storage suite: 12 PASS using actual Windows DPAPI and ACLs. Includes confidential clients, cold reopen, migration failure, concurrent removal/claim, corruption, cross-file/identity substitution, hardlinks, parent junctions, cache invalidation and plaintext/envelope size limits. Separate actual-process token contention and dead-owner recovery remain covered.
- Final full Core gate: **2248 PASS / 42 SKIP / 0 FAIL across 226 files**. Sequential shards: 563/10, 616/19, 452/7 and 617/6 (PASS/SKIP). Typecheck, lint, doctor, policy and production build PASS. Earlier SDK-specific suite: 50 PASS; process exclusion: five PASS, including four two-process rounds.

Commands from core: `bun --no-env-file test test/mcp-oauth-storage.test.ts`, existing OAuth/response-bound/exclusion/lock test files, `bun --no-env-file run typecheck`, `bun --no-env-file run lint`, `bun --no-env-file test --shard=N/4` for N=1..4, `node bin/neko-source.cjs doctor`, `node bin/neko-source.cjs policy`, `bun --no-env-file run build`. Doctor/policy use an isolated working directory and absolute launcher. Build covers the compiled binary, ConPTY input, ACP and startup lifecycle/input scenarios. Existing isolated-profile, missing-credential/TTY and unconfined-mode warnings remain explicit.

Final manifest: `53ce9f193e591f9dd88a77b7f25ce4670ce7e3c38dec457735b2df627458c309`. All 1856 source files were verified against the code commit with existing LF normalization accounted for. The pre-checkpoint recognizable-secret scan checked five candidate files and 674 newly reachable history blobs with zero matches; it is not exhaustive. The unrelated adoption-document edit remains excluded. Documentation uses `python tools/check_docs.py` and local-link checks.

## Compatibility and remaining scope

This changes only the dedicated MCP OAuth store, not every Core provider/config credential. DPAPI requires the appropriate Windows user context; copying a record to another context or opening it with an older Core version can fail. Reconnect explicitly rather than falling back to plaintext. Existing copies/backups of legacy plaintext are not retroactively erased. JavaScript strings and operating-system memory are not guaranteed securely erased. The helper may block the calling thread up to its deadline; cache hits avoid repeated process startup.

These are local Windows primitive/SDK fixtures, not cross-user penetration testing, real provider consent/revocation, cloud sync, native-installer or other-platform acceptance. Public-DNS/dial pinning and provider-specific acceptance remain open. No additional changelog bullet is marked complete for this hardening. The [official changelog](https://x.ai/changelog/bot) was rechecked and still lists 0.78.1, including 0.78.0, as latest.
