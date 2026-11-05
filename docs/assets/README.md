# Neko Aurora repository presentation assets

The banner is a static, original composition for this repository, not an application screenshot or a Neko Core banner. Desktop and narrow-screen layouts share the existing approved Neko Peek identity, ivory/graphite palette and Geist typography. No external scripts, fonts, images or animation are loaded by the SVGs.

## Artwork and typography

- Existing Neko Aurora lockup is reused without changing its path geometry: [historical source asset](https://github.com/meiiie/neko-aurora/blob/cfdb5cf21a02fcb3333aeea6f9f65ada6a77595c/app/native/internal/web/assets/brand/neko-peek/nekobot-lockup-light.svg).
- Neko Peek is derived from `meiiie/wiii`, revision `6b1c30b15b4c67e3c841ae870e6989577fc228f8`. Preserve its [AGPL3 license](../../app/native/internal/web/assets/brand/neko-peek/licenses/AGPL-3.0.txt) and [provenance](../../app/native/internal/web/assets/brand/neko-peek/PROVENANCE.md).
- Geist glyphs are outlined from the existing Latin/Vietnamese/Latin Extended WOFF2 subsets. The [SIL OFL1.1 notice](licenses/Geist-OFL-1.1.txt) is preserved here byte-for-byte from revision `cfdb5cf21a02fcb3333aeea6f9f65ada6a77595c` for the historical banner glyphs. The product removed the Geist source fonts on 2026-10-22, as recorded in its provenance. No runtime font fetch is needed.
- `generate_presentation.py` is the historical generator for the two banners and three static badges. Regeneration requires the source assets from the historical revision above and fontTools with WOFF2/Brotli support; the current checkout no longer contains those inputs. The new conversation motif is an abstract illustration, not a fabricated UI capture.
- Badges describe development status and source architecture. They do not assert passing CI, a stable release or an application-wide license. No new license grant for unrelated application code is made here.

## Historical application screenshots

The two PNGs are unedited, previously recorded actual web UI from local Go service + Chrome headless, application source `2a740296e0711989d82601ac09a247bf2a5fa86c`. Data is a controlled fixture: bot names, channel and report are synthetic; no real user chat, account or secret is shown. These are not mockups and not evidence for the current source, native Tauri rendering, physical phone, live provider/model quality or cloud sync.

| File | Pixels | SHA256 |
|---|---|---|
| `screenshots/report-desktop-2a74029.png` | 1440 × 900 | `b068acbf477960f90119689b4c09fec51e3a21789d111731a1b2f916fb71f1cb` |
| `screenshots/report-mobile-2a74029.png` | 390 × 844 | `5c96899df1018277689ad2b7b279a646c69974f2023cf9f892dd59bef2d018b5` |


## Workspace screenshots — 2026-10-30

Unedited Chrome 156 fixture screenshots from application source `6f93ea26dbe7d99e60b0e63fc6df2fee1c5fc471` and manifest `80977f7bff121a0eb00d28d7934c402048a359a907135ee82a14cbb718dcf8f9`, recorded by `app/tests/aurora/workspace-browser.py`. Bot names, messages, schedules and workspace changes are synthetic. These show the actual web UI, not native WebView or physical-phone acceptance. Existing app fonts, icons and branding retain their existing notices; no OpenMausBot image or mascot is included. Historical screenshots above are unchanged.

| File | Pixels | SHA256 |
|---|---|---|
| `screenshots/workspace-home-6f93ea2.png` | 1440 × 1000 | `6d9346a28216c6811adfbd0659225bc584c03eb9d378c58e8bd36baad7ee0260` |
| `screenshots/workspace-context-6f93ea2.png` | 1440 × 1000 | `30e15c60931a7cded17f0c6469750c46066c0e7ae145ad4a33fc307d4da42e15` |
| `screenshots/workspace-mobile-6f93ea2.png` | 390 × 844 | `d2c4b76266592d61d1fdc2e85eb6153d0229355a2c2cddefc23a6bc78a7885a3` |
