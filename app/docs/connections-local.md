# Local connection cards

The local Go API exposes multiple account cards with stable IDs, creation nonce deduplication, config revisions and compare-and-set removal. Provider/endpoint cannot change under an existing credential binding. Every card remains stored_unverified; this slice performs no provider request, OAuth login, account selection for Core or external grant.

Secrets enter a separate password form, never the conversation. Go passes a bounded stdin frame to the trusted Rust host; the value is absent from command arguments, responses, snapshots and logs. Rust uses Windows user-scoped DPAPI with entropy tied to the local actor/space/account and opaque secret reference. Duplicate requests compare the secret within Rust without exporting plaintext; a different secret with the same nonce rejects. Buffer wiping is best-effort, not a guarantee about every browser/allocator memory copy.

The UI clears secrets on provider change, submit, Escape and close. Generation and AbortController fences reject late results for a newer form context. Dismissal does not claim an already submitted server write rolled back; the authoritative list is refreshed. All account/plugin content uses textContent. The narrow view is a scrollable dialog with keyboard dismissal and visible focus.

Metadata and ciphertext remain in ignored local state. Ciphertext sync precedes atomic metadata publication; a failed metadata write may leave an encrypted orphan. Removing an account removes its local metadata/use path. Encrypted orphan cleanup and provider-side revocation are separate, unimplemented operations. Core fixture runs use no saved account key. Runtime account leases, live key rotation, OAuth, cloud secrets and multi-owner authentication remain gates.

Plugin manifests are bounded declarations stored as manifest_only_disabled. They cannot execute commands, fetch remote schemas, create grants or expand Core's tool ceiling. The built-in artifact tool is the only executable capability, scoped to requested task files. Manifest license labels are user declarations, not verified reuse permission.

The composer separately keeps a bounded browser outbox of user-message fields, replays the same nonce after reload and removes entries after matching durable snapshot/event/ACK. Secure form data never uses that outbox. This local-owner cache is not a cloud/device session or logout cache policy.

Validation includes connection store revision/nonce tests, Windows DPAPI roundtrip/wrong-account test, actual host/API fixture testing, and owned Chrome/debug Tauri WebView2 tests for races, dismissal, outbox, replies and downloads. Exact commit and hashes are recorded with checkpoint evidence.
