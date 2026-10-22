# Neko Aurora repository instructions

- Preserve existing work, WIP/archive history and the original component repositories.
- Keep Tauri UI, Rust trusted host and Go control plane boundaries; Neko Core owns the agent loop. Wiii is a separate overlay requiring its verified base.
- Read each component's instructions and retain its LICENSE/NOTICE/provenance. Do not invent an app-wide license or copy unlicensed reference bundles.
- Do not use real credentials/provider accounts, create grants, deploy or push without the user's applicable authorization. Keep state, chat, account stores, caches and binaries out of commits.
- Scan candidate files and newly reachable history for secrets before every authorized checkpoint/push; never print matched values.
- For onboarding/docs/assets, run `python tools/check_docs.py`, check issue form schema and review desktop/mobile render. Static badges must not claim CI/release/license status that does not exist.
- Follow CONTRIBUTING.md and SECURITY.md. Record exact commit, command, environment and PASS/FAIL/BLOCKED; local fixtures are not provider quality, cloud sync or physical-device acceptance.
- Do not overwrite another Wiii checkout or treat this development snapshot as a production release.
