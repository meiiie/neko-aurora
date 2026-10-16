# Baseline 2026-10-11

Observed directory: E:\\Sach\\Sua\\NekoBot. It had zero entries and no Git repository before this checkpoint.

Source provenance retained from the handoff:
- App c9dbbd77813458977002025d05ec72acf91e2efe: archived baseline, not restored or tested here.
- Core 3d5725f222815e15b12a212e372cf17057a459c0: archived baseline, not restored or tested here.
- App d61a45c42fbe39f77168cf9ff705439a58ff2f59 and newer native changes: unavailable.
- Sibling E:\\Sach\\Sua\\NekoCore: observed HEAD b1058bfdb10acd21d4467626c55c19006e247c46, with untracked files. Not edited, copied or treated as the pinned runtime.

Read in full through Library:
- neko-development-direction.md, libfile_a80eaf9fcb288191aa41665ba971c1db, version 1, 1683 lines.
- HUONG-DAN-CAC-BAN-CU.txt, libfile_bbacc8a1ffa08191bbcb46d88273e24a.

Local transfer of the five specified handoff files is blocked. Official helper first failed network access under the sandbox; the consumer-local retry downloaded the first item but failed before installation because Windows Python lacks os.setxattr. No supported metadata-preserving local copy was completed. Original Library items and ZIPs remain intact. No helper workaround or raw storage URL transfer was used.

Available tools at inspection: Git 2.47.1.windows.2, Node 25.9.0, npm 11.7.0, Rust/Cargo 1.97.1, Python 3.13.7. Go not in PATH; a private official portable toolchain is being prepared.

Historical tests are not evidence for this repository. Grok 0.68.1 is a client/reference bundle; neither a complete backend nor a verified reusable code license. Implementation here is original, based on the direction, without importing bundle code or assets.

Later in this session the transfer blocker was resolved using pre-existing Ubuntu WSL and the unchanged official helper. All five handoff files are now readable under reference/handoff or reference/archive; Library xattrs survive on NTFS. The Grok archive matches the owner-supplied SHA-256, and all 15 historical ZIPs match the supplied SHA-256 list and pass CRC. App c9dbbd7/Core 3d5725f are extracted read-only under reference/extracted. This establishes source availability, not integration or runtime test success.
