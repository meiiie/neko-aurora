# Core MCP OAuth process exclusion — 2026-10-29

Code commit `60fe223a2b1f40093569bbc2cba28e9069766557`, based on `43e61aa16f567aa06cba2b5f8d92e628c276303c`. Solo pstack Feature / Prove It Works / Show Me Your Work; self-review, no subagents. This continues the [OAuth response-bound work](grokbot-core-oauth-body-2026-10-28.md) without adding feature-completion credit.

## Behavior

The durable consuming marker previously prevented retries by one provider instance, but checking and replacing shared token files was not serialized across processes. A stale refresh reader could reuse a token already claimed by another process. A late response could also overwrite a newer authorization or recreate removed credentials.

The [provider](../../core/src/adapters/mcp-oauth.ts) now claims a refresh token inside a short [record lock](../../core/src/adapters/mcp-oauth-lock.ts), comparing it with the current ready record. A version-2 consuming record carries a random attempt nonce; completion must match that nonce before publishing ready tokens. Credential removal uses the same lock. Legacy ready tokens and version-1 consuming records remain readable. An unknown network outcome retains the consuming marker and requires explicit reconnect; there is no automatic replay.

The lock atomically publishes a nonempty directory containing a unique PID/nonce owner filename. It is never stolen because a timeout elapsed. Recovery requires the operating system to report the owner PID absent; ambiguous ownership, aliases and inaccessible/live PIDs fail closed. Cleanup removes only its own filename and then a nonrecursive empty directory, so it cannot delete a successor's lease. No lock is held across network I/O.

## Validation

Windows amd64, PowerShell 7.6, Bun 1.4.0, Node 25.9.0 and Python 3.13.7. The harness isolates child USERPROFILE/XDG/npmrc and excludes provider credentials. No live account, grant, paid call, push or deployment. Exact commands, timestamps, manifests and results are in `.audit/2026-10-29-oauth-exclusion/validation.jsonl`; framing and self-review decisions are retained in decisions.tsv.

- **RED:** four new stale-reader/completion regressions fail on the old runtime in `oauth-exclusion-160939.log`.
- **Fixture corrections:** `mcp-oauth-161047.log` records nine assertions expecting the old consuming-record shape. They now strictly require the nonce. `oauth-lock-161214.log` records a child HOME reused as the lock directory; Bun's `.bun` directory invalidated the empty-directory assertion. HOME and record directory are now separate.
- **Focused PASS:** five exclusion tests, three lock tests, 50 OAuth/response-bound tests and 63 broader MCP tests. Two actual child processes contend for the same cached refresh token in four rounds; exactly one claims it each round. A separate killed process proves recovery, while its live state excludes another owner.
- **Full gate PASS:** **2236 PASS / 42 SKIP / 0 FAIL across 225 files**, split into sequential shards: 444/27, 586/0, 648/7 and 558/8 (PASS/SKIP). Typecheck, lint, doctor, policy and production build pass. Build includes real ConPTY input, ACP smoke, startup lifecycle and four startup/input scenarios. Doctor/policy retain explicit isolated-profile/TTY/credential/sandbox warnings.

Commands from core: `bun --no-env-file test test/mcp-oauth-exclusion.test.ts test/mcp-oauth-lock.test.ts`, OAuth tests in `test/mcp-oauth.test.ts` and `test/mcp-oauth-http.test.ts`, `bun --no-env-file run typecheck`, `bun --no-env-file run lint`, `bun --no-env-file test --shard=N/4` for N=1..4, `node bin/neko-source.cjs doctor`, `node bin/neko-source.cjs policy`, and `bun --no-env-file run build`. Doctor/policy ran in an isolated audit workspace with an absolute launcher path.

Final manifest `d2e32615b27e17706a28ae3071ddb572e26b1d5b7ccb0d9a5bd2e464cc21f2e2` covers 1853 files verified against the code commit, accounting for existing LF normalization. The pre-checkpoint recognizable-secret scan covered six candidates and 666 newly reachable history blobs with zero matches; this is not exhaustive detection. The unrelated pre-existing adoption-document edit remains excluded. Documentation checks use `python tools/check_docs.py` and local report-link resolution.

## Limits

This protects cooperating updated processes on one local filesystem. PID reuse can conservatively require manual recovery. It is not a hostile same-user filesystem isolation boundary, distributed lock, power-loss durability guarantee, remote revocation or globally exactly-once refresh protocol. Providers that deliberately reuse a refresh value can still refresh sequentially after a prior completed response. Explicit new authorization can supersede an unknown old attempt; the old completion is rejected.

Windows credential sealing/ACLs, public-DNS/dial pinning and live provider acceptance remain open at this checkpoint. Go/Rust/frontend code is unchanged, so their gates were not repeated. The two existing MCP source bullets remain partially validated; the broader Grok Bot backlog remains open.
