# Local adapter installation intake CAS

Capability: `local_adapter_installation_intake_cas_v1`.

`POST /v1/adapters/sessions/{adapterId}/messages` accepts an optional
`expectedInstallationPin`. Its shape is the seven-field, version-1
`installation` returned by the authenticated, read-only
`GET /v1/adapters/installation-catalog`: `version`, `executorId`,
`generation`, `hostSha256`, `coreSha256`, `rootSha256`, and
`capabilitiesSha256`. No filesystem paths, boot IDs, leases, credentials,
or process proofs are accepted through this field.

For a new request, the selected executor pin must exactly match that
input. The existing installation guard holds the registry lock through
the bounded `work.accepted` journal commit. Concurrent configure/revoke
therefore either follows the original intake or causes it to fail; it
cannot accept a task under a replacement installation. This guard
checks installation identity, not online readiness or provider access.
Malformed pins return 400; stale/revoked pins return 409. New intake
requires an available store and registry. None of these failures creates a
task or changes its source provenance.

A duplicate original nonce is compared with the task's original
`ExecutorPin`, rather than the registry's current pin. The same original
pin, message, adapter source IDs, account, project, channel revision, and
resource selection return the existing task with status 200 even after
an installation rotation. A changed expected pin returns 409. An ACK
for an old task remains readable without an available live registry;
a closed or poisoned store denies it. It does not grant that installation renewed execution or
change its immutable context. Historical replay uses the pin already
stored on the task and never consults the current installation to
rewrite it.

Omitting the field preserves the prior adapter intake contract. Trusted
Wiii clients should require this capability and send their persisted,
explicitly reviewed installation pin. They must keep that original pin
through lost-ACK retries, compare the returned task's pin, and require a
new owner-reviewed link when selecting another installation. Wiii's
own project ID is source provenance; Neko project/channel selection is
explicit and separately revision-checked.

Verification: `TestAdapterInstallationIntake*` exercises the real
authenticated API decoder and ledger, metadata-to-intake rotation,
original-nonce replay after rotation/restart, malformed/revoked pins,
legacy omission, and concurrent rotation/admission. Its original
baseline fails at HTTP 400 because the additive field is unknown. The
concurrency fixture is a bounded schedule test; it is not a race-detector
or real-provider acceptance claim. Wiii GUI, native owner approval, and
transport integration have separate gates.

`GET /v1/adapters/capabilities` provides capability discovery without
fetching private task history. Its exact body is
`{"version":1,"capabilities":["local_adapter_installation_intake_cas_v1"]}`.
The same cookie, Host, Origin, and no-store controls apply. Any query,
including a bare trailing `?`, is rejected. It reports supported code;
it does not observe, renew, or establish installation readiness. A
closed or poisoned store denies the lookup. The existing installation
and routing catalog response schemas remain unchanged.
