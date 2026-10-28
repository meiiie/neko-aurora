# Vault deletion failure consistency

Code commit: `c9e9b078ab45dcaa9735f1870f097fc552411513`, based on `325fc8da78f87016a3b69b270c1366a8f650903a`. Solo pstack prerequisite for OAuth credential lifecycle.

A failed Vault.Delete previously removed the in-memory credential/cache even though its encrypted entry remained on disk. The current process stopped listing and redacting it, while reopening restored it. Deletion now restores the original entry and cached value under the existing lock when persistence fails. Successful deletion is unchanged.

Windows amd64, PowerShell 7.6, Go 1.27.1; provider/account environment excluded. Exact command, environment and manifests: `.audit/2026-10-28-vault-delete/validation.jsonl`. All final checks share source manifest `b4b45aa49835645ce0c872d166e93a73f5944e8a99a99abea0de3d4ae60e8fe9`, with no source changes during checks.

| Command (in app/native) | Result |
|---|---|
| `go test -v -count=1 ./internal/vault` | RED `vault-185151.log`: both injected save failures remove live credential. GREEN `vault-185205.log`: 11 tests/subtests, including creation/replacement failure, unchanged metadata/value/redaction/disk, reopen and successful explicit retry |
| `go test -json -count=1 -timeout=180s ./internal/aurora` | PASS: 282 tests/subtests, 24 existing skips, 0 failures; `aurora-185208.log` |
| `go vet ./...`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS `vet-185227.log`, `build-185230.log` |

Candidate scan: two files and 297 newly reachable history blobs, zero recognizable secret matches; no values printed. Simulated filesystem failures and reopen are not OS-power-loss durability, remote token revocation or completed OAuth disconnect behavior. No frontend change, provider calls, deployment or push.
