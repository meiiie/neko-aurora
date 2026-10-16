# Neko Bot

Native reconstruction of the multi-bot product, following the direction dated 2026-10-11.

Target boundaries: Tauri desktop and adaptive web UI; Rust trusted host; Go durable control plane; Neko Core as the only agent runtime. Wiii remains a separate ADE.

This repository starts from an empty Windows directory. It does not pretend to contain the lost App history or the newer Tauri/Rust/Go changes. See [baseline](docs/baseline.md) and [architecture decision](docs/adr-001-local-foundation.md).

Cloud deployment and real-provider evaluation remain deferred. Original ZIPs and other local repositories are preserved.

## Run locally

Prerequisites: Go 1.27+; Rust/Cargo and Windows build tools for the optional Tauri shell. The Go service has no third-party dependencies.

From PowerShell in this repo:

    .\\scripts\\run-local.ps1 -GoExe <path-to-go.exe>

Open http://127.0.0.1:8765. For the desktop shell, add -Desktop. Port 8765 must be free. The shell loads this loopback UI with no native IPC capabilities. It is an experimental local shell, not an installer or signed release. State lives in .state and is excluded from Git; keep it for history, and stop the service before copying it for a local backup.

Validation: cd native; go test -count=1 ./... and go vet ./...; cd ../host; cargo test --locked. Build the control service with go build -o ../.build/neko-control.exe ./cmd/neko-control; run scripts/e2e_local.py with --binary and the exact --commit to exercise two clients and hard kill/restart.

Current scope: two named bot identities, durable foreground/background work, nonce and canonical tool deduplication, event replay, actual Core ACP admission, streamed replies, immutable report artifacts, cancellation fences and supervised process recovery. The default launch holds requests until an explicitly configured runtime is available. The controlled development fixture uses the pinned Linux Core through WSL and calls no real provider. See [Core adapter](docs/next-core-adapter.md).

For the archived Linux process test, restore with `python scripts/restore_core_runtime.py reference/archive/original .build/core-runtime` (existing Ubuntu WSL/Python required). For native Windows Core, `python scripts/build_core_windows.py reference/extracted/core-3d5725f/package/core` verifies all pinned sources, installs frozen dependencies without lifecycle scripts and compiles a private AGPL runtime. Binary hashes must match the reviewed runtime pin manifest; a different rebuild is not admitted silently.

Build `host` with `cargo build --locked`, build Go, then run `python scripts/e2e_core.py --native-core --commit <clean-code-commit> --output .build/e2e-core.json`. `--serve` starts an owned fixture preview until Enter. Secure accounts/plugin declarations and durable browser outbox are documented in [local connections](docs/connections-local.md); `scripts/e2e_connections.py` verifies real DPAPI and `scripts/test_ui_fixture.py --core --native-core --tauri` verifies the owned native window. Real provider/cloud, signed distribution and real-phone acceptance remain separate gates.
