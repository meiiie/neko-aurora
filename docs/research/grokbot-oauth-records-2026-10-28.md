# Private encrypted OAuth records

Code commit: `f6939bcd9bd789cc8974db02d5caecd3b5a278c9`, based on `9d2f0374e94e92b8871e0040518bfef648cc50dc`. Solo pstack prerequisite for Aurora OAuth persistence. No authorization route or account grant is introduced here.

The existing AES-GCM Vault can now store versioned OAuth records under `record:mcp-oauth:<64 lowercase hex>`, a namespace impossible under public secret-name validation. Dedicated methods read, replace and delete these records. Public Get/Has/Meta/List/Set/Delete and typed item listings cannot reveal or alter them. Records reuse encrypted storage, name-bound AAD, privatefs permissions and atomic save/rollback; no additional plaintext token file is created.

Each record separates JSON protocol/binding data from named secret strings. The envelope is at most 32 KiB, data at most 16 KiB, and there are at most 16 secrets of 4–8,192 bytes. IDs, field names, UTF-8, object JSON, envelope version and fields are checked. Private ciphertext size is checked before decryption; decoded state and secrets are validated on reopen. Get returns newly decoded data rather than aliases into live state. Individual secrets are masked longest-first by the existing Vault and process log writers.

Two existing failure paths were also repaired: a corrupt Open no longer replaces a healthy live Vault in the process log-redaction registry, and nonce length is checked before calling GCM so malformed storage returns ErrCorrupt instead of panicking. Duplicate stored names are rejected. Failed record create/update/delete preserves prior cache, redaction and stored bytes.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1; allowlisted environment without live/provider credentials. Temporary state directories, real encryption/privatefs and injected filesystem failures; no real account credentials or network calls. Exact executable paths, commands and source manifests: `.audit/2026-10-28-oauth-records/validation.jsonl`.

All final checks share manifest `f92e3ad146aa5b9cb09915c63f187da9ec2ac36e7c0481b90b2bcff6e1ea74da`, with no source changes during checks. 1,778 source files verified against the code commit, allowing existing Git LF normalization.

| Command (app/native) | Result |
|---|---|
| `go test -v -count=1 ./internal/vault` | RED `vault-193233.log`: failed-open redaction loss and malformed-nonce panic. GREEN `vault-193314.log`; final `vault-193453.log`: 37 tests/subtests pass, including private API isolation, encryption/reopen, field redaction, copied returns, bounds, wrong hosted key, record identity/envelope, duplicate entries and save/delete rollback |
| `go test -json -count=1 -timeout=180s ./internal/aurora` | 282 pass, 24 skip, 0 fail; `aurora-193532.log` |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | 1,779 pass, 37 skip, 0 fail across 25 packages; `go-serial-193551.log`, 194.23 seconds |
| `go vet ./...`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS `vet-193905.log`, `build-193908.log` |

Candidate scan: four files and 313 newly reachable history blobs, zero recognizable secret matches; no values printed.

This is a storage contract, not OAuth exchange/refresh/disconnect integration. Callers must place credentials in Secrets and validate protocol data/binding for their state machine. Tests do not establish redaction across separately split writer calls, total Vault file-size bounds, cross-process writer coordination, OS-power-loss durability, remote revocation or provider acceptance. Older binaries that only accept public Vault names cannot open a Vault after private records have been created; rollback requires a compatible binary or explicit migration, not silent data deletion. No frontend change, deployment or push.
