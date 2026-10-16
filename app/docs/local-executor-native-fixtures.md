# Owned native executor validation

This additive validation slice is based on sealed S3 source e7d053a27005eabb8dc1e2fe7fa0a328bba5c771. It changes only tagged fixtures/tests and this document. Production Go/Rust/Core logic and the original S3 evidence packet remain intact.

`executors.FixtureSetClock(registry, now)` exists only with `//go:build neko_native_fixture`. It rejects nil clock/registry, closed and poisoned registry under its mutex. No production environment toggle, IPC, HTTP route or normal-build clock hook exists. A clock callback must be bounded and must not reenter the registry.

The Windows runtime fixture requires explicit trusted `NEKO_NATIVE_FIXTURE_HOST`, `NEKO_NATIVE_FIXTURE_CORE`, `NEKO_NATIVE_FIXTURE_SUPERVISOR` paths. They are test configuration only and are excluded from manager child environment. The release host is built from sealed S3; actual native Core fa48 and its pinned writer helper are used without copying. The fixture serves a controlled model stream only on explicit TCP4 127.0.0.1 with fixture-only credentials. No external provider, GUI, user state or pairing configuration is used.

Run with approved E: GOCACHE/TEMP/TMP and explicit paths:

```
go test -tags=neko_native_fixture ./internal/runtime ./internal/executors -run '^TestNative' -count=1 -timeout=2m -json
```

The cases are:

- **TTL expiry:** after actual Core admission and text, atomic registry clock offset advances in 5s steps with a Registry.Heartbeat each step. At 605s/121 steps, the fixed 600000ms lease is retired `expired` while readiness stays live. These heartbeats do not renew the task lease. This is accelerated controlled-registry time, not ten minutes of actual wall time or proof of Rust wall-clock expiration.
- **Revocation:** actual Registry.Revoke retires authority. Manager fences and closes the current native run. Both cases check a held Windows process handle for exact Core image and OS exit before accepting current PID/session/proof receipt, preserving original lease epoch after work epoch changes. Wrong PID, old attempt, substituted epoch, late text, completion and actual-task artifact request are denied without journal mutation/effects. Three typed journal restarts preserve the settled work.
- **Normal-clock recovery:** an exact-image-verified owned Core process is forcibly terminated. Actual stop receipt is required before explicit Requeue. The next live claim has a new attempt/lease/fence while installation, context, Core task/session/runtime remain the same; Core activation increments by one. A second controlled local model response completes. Old receipt and output cannot settle the new attempt. Three typed restarts preserve the final work.

All scratch state is retained under the explicitly approved E: TEMP directory, including failures. Each successful case writes native-receipt.json with exact authority/stop metadata, hashes, clock offset, heartbeat count, retirement reason, local model call count and restart checks. Reading owned registry bytes produces post-stop evidence only; no file read supplies live execution authority.

The untagged `go list`/normal build must exclude the hook and all native fixture tests. This gate is separate from real phone/AT/GUI/cloud/provider acceptance, whole-product context admission, and actual ten-minute wall-clock expiry. The five-second manager probe deadline is unchanged; optimized release host is required because the known debug build exceeds it.
