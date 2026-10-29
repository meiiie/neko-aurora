# Creation-anchored interval routines — 2026-10-29

Backend commit `a84df1989ee2a4168d7d7261eddf26959f61ffbf`, based on `401676da0d2aa1278188b6b0ca36ae6f08d6634f`; UI commit `51b39b75f2ee689d0da5540747da7a7ae11b2926`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing, with self-review. No subagents, live provider calls, credentials, grants, push or deployment.

This partially implements the creation-counted routine behavior in [Grok Bot 0.31.0, Improved-2](https://x.ai/changelog/bot#v0.31.0). The observed source sentence's SHA-256 is `de2b84b567d3c5632c820a098d87d2b62d8d8d7b0aa8566a943d1f18b8c1d9cb`, matching the existing index despite changed page line numbers. The [owner guide](../routines.md) explains the difference between elapsed intervals and wall-clock schedules.

## Behavior

[Interval scheduling](../../app/native/internal/aurora/schedule_interval.go) accepts whole intervals from 15 to 10080 minutes. The service supplies a canonical UTC anchor, and the first occurrence is one full interval later. Subsecond creation precision is retained. A separate cadence generation identifies occurrences; pausing or changing text does not create a new occurrence identity. A cadence change creates a fresh anchor/generation. Returning to a calendar schedule clears interval metadata.

After downtime only the latest elapsed slot is eligible. No backlog burst is replayed, and rolling the clock backward does not run an older slot. DST does not change elapsed cadence. The existing 30-second scheduler tick and runtime capacity still affect when admitted work actually runs; this is not a hard real-time guarantee.

The existing atomic private schedule file stores the anchor, generation and consumed slot. Malformed state fails closed. Ledger nonce admission remains authoritative when progress-file persistence fails: restart recognizes already accepted work without duplicating it. Existing quota hooks, pause/delete behavior and the 12-schedule limit remain in effect.

[Schedule editing](../../app/native/internal/aurora/schedule.go) requires workspace identity and revision. The [editor](../../app/native/internal/web/assets/app/schedule-editor.js) hides calendar fields in interval mode, validates the minute range and explains restart/edit behavior. The list labels cadence, and Vietnamese/English strings are included. Service-worker cache advances to v89. A model directive can describe the interval; its supplied execution anchor/generation/last fields are discarded in favor of service state. Existing peer-origin exclusion and owner-intent prompt instructions are retained. This does not add an enforced natural-language intent classifier or demonstrate actual model compliance.

## Evidence

Windows amd64, PowerShell 7.6, Go 1.27.1, Python 3.13.7, Node 25.9.0 and Chrome 156.0.8078.49. Allowlisted credential-free environment; isolated USERPROFILE/XDG for Go, temporary Playwright profiles with the OS USERPROFILE preserved for Chrome.

- API RED: interval edit returned 400 before implementation. Seven focused Go tests now PASS, covering exact/subsecond boundaries, spring/fall DST, late tick, rollback, restart, stale revisions, text/cadence changes, pause/resume, invalid edits, corrupt storage, failed writes after durable admission and server-owned directive identity.
- Full sequential Go: **2099 PASS / 38 SKIP / 0 FAIL across 25 packages**. Vet and production build PASS. The new Page script adds one embedded-asset subtest compared with the preceding backend-only Pages count; the seven interval tests account for the other additions.
- Browser RED: no interval option existed. Final schedule editor: **12 scenario groups PASS**, including exact scoped payload, invalid/noninteger/out-of-range values, reopening, text-only editing and switching back to a time-zone calendar schedule. Existing schedule-loading suite: **14 scenario groups PASS**, including failed loads, reconnect, stale replies, workspace switches and preservation of open edits. The 390×844 interval screenshot was inspected. Its unrelated Computer unavailable toast comes from the fixture; no Computer acceptance is claimed.
- Embedded web assets and final native build PASS. Go backend sources are unchanged between backend and UI checkpoints, so the full Go gate was not repeated for UI-only changes.

Exact commands, environment, base, manifests and pass/fail results are retained in `.audit/2026-10-29-schedule-interval/validation.jsonl`; framing and self-review are in `plan.md` and `decisions.tsv`. From app/native: `go test -v -count=1 -timeout=120s -run TestScheduleInterval ./internal/aurora`, `go test -count=1 -timeout=120s -run "Test(Schedule|AdoptionAPI|Network)" ./internal/aurora`, `go test -json -p=1 -count=1 -timeout=300s ./...`, `go vet ./...`, `go test -count=1 ./internal/web`, and `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora`. From the root: `python -u app/tests/aurora/schedule-browser.py` and `python -u app/tests/aurora/schedule-loading-browser.py`.

Backend manifest `c40fb2a9c5f7d167608ec2de589efa3069974c5e178fb27954387e3444d1b13b`; final UI manifest `37a03421efc52867143340d0bc80d5fe91784ec92d9a6eac19c8c31e16551656`. Each verifies 1869 source files against its commit, accounting for existing LF normalization. No source changed during a gate. Recognizable-secret scans checked five backend candidates and 719 newly reachable history blobs, then five UI candidates and 724 history blobs, with zero matches; not an exhaustive scan. The unrelated adoption-document edit remains excluded.

## Limits

Existing daily/weekdays/once schedules retain wall-clock semantics. A 1440-minute interval is elapsed time and can move by an hour relative to local time at a DST transition. No webhook, new handoff channel, distributed scheduler, physical-device acceptance or live model evaluation is included. A binary without interval support rejects the new schedule state; keep a compatible binary. Existing real user state was not opened or migrated by these tests.

The historical index adds one **partial-local-validation** entry, not complete parity. Across both stored indexes there are now **35 partial entries, one safeguard-preservation exclusion and 884 unvalidated source bullets**, 920 total. Pages publication/collaboration, provider acceptance, cloud sync, realtime voice, team authorization, payments and release/device gates remain open.
