# Neko Bot

Local multi-bot application: Tauri shell and adaptive UI, Rust trusted host, Go durable control plane, Neko Core as the semantic runtime. Wiii is a separate optional ADE. Original ZIPs and recovered checkpoints are preserved; reconstruction history is in [baseline](docs/baseline.md).

The [product gap matrix](docs/product-gap-matrix-20261011.md) distinguishes local features, missing code and acceptance gates. Cloud/device sync, real provider evaluation and deployment remain deferred. Responsive viewport tests do not establish physical-phone acceptance.

## Run the verified local build

From PowerShell in this repository, after the release receipt and binaries have been materialized:

```powershell
.\scripts\run-prebuilt-local.ps1 -Manifest .checkpoints\local-backup-release\launch.json -CheckOnly
.\scripts\run-prebuilt-local.ps1 -Manifest .checkpoints\local-backup-release\launch.json
```

Open http://127.0.0.1:8765. The first command verifies binaries against `.checkpoints/local-backup-release/launch.json` without starting a service. The second starts Go in the foreground. Port8765 must be free. Ctrl+C stops the service; `.state` retains history. No runtime is configured by default: new messages are saved as **waiting for runtime**, with no provider calls.

For the Tauri window, add `-Desktop`. It starts an owned service and stops that service after its window exits. Interrupted tasks may require explicit recovery when a configured runtime starts next. This shell is not a signed installer. [Run, backup and rollback](docs/run-local-current.md).

## Local behavior

Create bots with stable IDs and `@alias`, rename display names without changing accepted tasks, create shared local channels/projects, save per-bot instructions and history selection for new requests, upload bounded UTF-8/DOCX sources and preview scoped text/result files. Runtime selects relevant ready text automatically. New profile changes keep accepted work pinned to its original configuration. A GET-only two-client local replica is a simulation; cloud/device pairing is still missing. Account inputs use Rust/DPAPI; stored accounts remain **unverified**, and external plugin descriptors remain disabled.

Foreground requests coexist with background jobs. Supplement/replace, queued pause/resume, cancellation, durable intents and original-context recovery have separate typed contracts. Default Go saves obligations; execution requires an explicitly configured reviewed runtime. Controlled loopback fixtures prove Core/host/journal/process behavior, not real-model quality. Create separate private conversations, archive/restore them without stopping accepted work, export narrow JSON/text history, and inspect disabled retention previews. See [conversation evidence](docs/delivery-conversations-retention-20261012.md), [previous evidence](docs/product-local-current-20261011.md) and [display-name contract](docs/bot-display-names-local.md).

## Development

Go1.27+ and Rust/Cargo/MSVC/Windows SDK are needed to rebuild Windows components. Go has no third-party dependencies. `scripts/run-local.ps1` is the older development launcher; it recompiles components. Prefer the prebuilt launcher for review.

Keep build caches and temporary files on E on this machine:

```powershell
$env:GOCACHE = 'E:\CodexScratch\neko-dev\gocache'
$env:GOTMPDIR = 'E:\CodexScratch\neko-dev\tmp'
$env:CARGO_TARGET_DIR = 'E:\CodexScratch\neko-dev\cargo-target'
$env:TEMP = 'E:\CodexScratch\neko-dev\tmp'
$env:TMP = $env:TEMP
New-Item -ItemType Directory -Force $env:GOCACHE,$env:GOTMPDIR,$env:CARGO_TARGET_DIR | Out-Null
```

From `native`: `go test -count=1 ./...`, `go vet ./...`, then `go build -o ../.build/neko-control.exe ./cmd/neko-control`. Keep old hash/backup and issue a fresh receipt before replacing the reviewed executable. From `host`: `cargo test --locked --offline`. Pin-compatible Core/helper binaries have separate manifests; arbitrary rebuilds are not admitted silently.

Old `e2e_core.py`/`test_ui_fixture.py` commands use historical pins/configuration. Current evidence packets identify exact source/driver/config/binary combinations. `scripts/ui_bot_names_local.mjs` accepts `--control`, `--host`, `--output` and optional clean `--commit` for the new actual-Go/browser gate.

No credentials, production state, public push or final ZIP belong in a checkpoint. Final packaging waits for user review of current images/video and release gates.

Offline encrypted service-state backup and verified restore into a new read-only directory are available. Current-user Windows DPAPI protects payloads and private manifest names; restored jobs never auto-resume. See [usage, exact checks and limits](docs/delivery-local-backup-20261012.md). This excludes browser drafts and portable/offsite recovery. No backups are uploaded.
