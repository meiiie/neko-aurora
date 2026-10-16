# Local Core adoption 2360aa6

The Windows launcher pins Core `2360aa6b6ee015d3e9262a9f425011dfec8f46e7`, binary SHA-256 `fa48d43f59257dcd171441f09a8ac53d3702485ddf7f80ac13a0796d53ac6310`. It creates fresh writer-v2 tasks. The unchanged helper is `f9f404a07a7fe1aa56a2b7d3c073a2330366de5ee26d572b37421338e2b99a3a`. Runtime verification remains before Core launch; a saved task with a different Core digest is refused before its task directory or process is created.

Historical 8d writer-v2 and original v1 receipts retain their ledger validation pins. That validation permits journal replay, not launch with an old executable or automatic rebinding to this Core. Existing task IDs, source pins, authority, effects and runtime provenance remain unchanged. A runtime mismatch requires a separate recovery decision; new intake can proceed independently.

Live admission and journal replay share a digest-continuity invariant: when a task already has a Core binding, its next admitted receipt must retain that digest. This applies even to an unstaged legacy admission. It preserves legitimate same-runtime proof-free historical receipts while refusing a hash-valid provenance transplant. Context refresh creates a new task without the old Core binding and retains the original binding in the archived context attempt.

The delivered Core includes `task-session migrate-v1 <absolute-manifest-path>`, an explicit offline metadata import tool. It creates new metadata-only IDs with labels/ancestry and byte-identical source backups. It does not adopt the old active task, context, effect receipts or authority. Neko Bot never invokes this tool automatically. No user state was migrated during adoption.

Private retained delivery: `E:/Sach/Sua/NekoBot/.checkpoints/core-adoption-2360aa6/candidate/`. `artifacts.json` SHA-256 is `d72d90dca1c9c69d4fa936d9f1e9c3b11d4e7004c19f657418bfbd158a05e601`; it binds the compiled binary, source bundle, AGPL license/notice, exact source provenance and offline migration evidence. The source bundle is derived local history; the recorded upstream snapshot revision is not represented as an original upstream Git object.

The predecessor binary and build manifest are retained under `historical-8d1aefb/`. The historical build manifest records an older v1 build and must not be used to identify the retained 8d binary; its actual SHA is in `preservation.json`. The clean application checkpoint 2d9d413 is also preserved as a verified local Git bundle there. Original archives and the E application source have not changed at this preparation checkpoint.

Core's exact offline gate passed 2114 tests with 26 explicit skips, formatting, typechecking, lint, policy/doctor and native build. Compiled migration tests passed four scenarios with 59 assertions. These are Core tool evidence, separate from application adoption, device UI, live provider and cloud acceptance. Application adoption receipts must name their own commit and actual executable hashes.
