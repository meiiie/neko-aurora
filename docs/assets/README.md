# Neko Bot repository presentation assets

The banner is a static, original composition for this repository, not an application screenshot or a Neko Core banner. Desktop and narrow-screen layouts share the existing approved Neko Peek identity, ivory/graphite palette and Geist typography. No external scripts, fonts, images or animation are loaded by the SVGs.

## Artwork and typography

- Existing Neko Bot lockup is reused without changing its path geometry: [canonical local asset](../../app/native/internal/web/assets/brand/neko-peek/nekobot-lockup-light.svg).
- Neko Peek is derived from `meiiie/wiii`, revision `6b1c30b15b4c67e3c841ae870e6989577fc228f8`. Preserve its [AGPL3 license](../../app/native/internal/web/assets/brand/neko-peek/licenses/AGPL-3.0.txt) and [provenance](../../app/native/internal/web/assets/brand/neko-peek/PROVENANCE.md).
- Geist glyphs are outlined from the existing Latin/Vietnamese/Latin Extended WOFF2 subsets. The [SIL OFL1.1 notice](../../app/native/internal/web/assets/fonts/Geist-OFL-1.1.txt) remains with the font sources. No runtime font fetch is needed.
- `generate_presentation.py` regenerates the two banners and three static badges, using fontTools with WOFF2/Brotli support. The new conversation motif is an abstract illustration, not a fabricated UI capture.
- Badges describe development status and source architecture. They do not assert passing CI, a stable release or an application-wide license. No new license grant for unrelated application code is made here.

## Historical application screenshots

The two PNGs are unedited, previously recorded actual web UI from local Go service + Chrome headless, application source `2a740296e0711989d82601ac09a247bf2a5fa86c`. Data is a controlled fixture: bot names, channel and report are synthetic; no real user chat, account or secret is shown. These are not mockups and not evidence for the current source, native Tauri rendering, physical phone, live provider/model quality or cloud sync.

| File | Pixels | SHA256 |
|---|---|---|
| `screenshots/report-desktop-2a74029.png` | 1440 × 900 | `b068acbf477960f90119689b4c09fec51e3a21789d111731a1b2f916fb71f1cb` |
| `screenshots/report-mobile-2a74029.png` | 390 × 844 | `5c96899df1018277689ad2b7b279a646c69974f2023cf9f892dd59bef2d018b5` |
