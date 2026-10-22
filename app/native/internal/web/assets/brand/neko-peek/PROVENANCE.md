# Asset provenance and licenses

This is a Neko Aurora-specific adaptation of the existing Neko Family v1 identity, not a wholly original character or a Grok-derived mark.

## Neko Peek artwork

- Source repository: https://github.com/meiiie/wiii
- Source revision: `6b1c30b15b4c67e3c841ae870e6989577fc228f8`
- Canonical source directory: `docs/assets/brand/neko-family-v1/`
- Artwork source: `logo/neko-peek-mark.svg`, `logo/neko-peek-mark-on-dark.svg`, and `logo/neko-peek-mark-mono.svg`
- Direction record: `README.md` identifies Neko Peek as approved family direction on 2026-08-15.
- Source license: GNU Affero General Public License v3.0, copied unchanged to `licenses/AGPL-3.0.txt`. No separate artwork license was present in the inspected canonical source directory.
- Original canonical SVG copies remain in `source/upstream-*.svg`.

Changes made for Neko Aurora on 2026-10-09:
- Kept the canonical cat, eye positions, rear-tail path and front-tail path in the primary color marks.
- Added a fine graphite edge around the ivory head for light-background legibility.
- Lifted the two tail tones in the dark variant for visible boundary contrast.
- Rebuilt true one-color marks with continuous ear outlines, a transparent face, solid eyes and a transparent tail fold. This repairs the upstream mono variant's lost ears and background-colored “cutouts.”
- Produced optical micro variants with heavier head boundaries, larger eyes and a simplified one-color tail.
- Added outlined Neko Aurora wordmarks, horizontal lockups, app-icon plates, maskable safe-zone icons and native-size favicons.

The derivative cat artwork and lockup/icon files containing it retain the source artwork's AGPL-3.0 provenance. Do not label those files Apache-2.0 merely because the surrounding application uses Apache-2.0. Keep this notice, license text, canonical source copies and the editable/export scripts with distributions of this brand kit. Application-wide licensing questions are outside this design task; review the distribution arrangement before public release.

## Typography

- The 2026-10-09 Geist lockups and Geist webfonts were removed on 2026-10-22: the product now uses Be Vietnam Pro (SIL OFL 1.1, see `app/aurora/fonts/`) and the wordmark below is outlined from it, so no font is fetched to draw it.

## Production and verification

No stock art, paid assets, generated raster artwork, external icon libraries or third-party product marks were used. SVG source and deterministic export scripts are included. Pixel exports were rendered with the application's installed Sharp; local SVG/PNG rendering and the separate Chromium verification limit are recorded in `VERIFICATION.md`. Design and inspection were local only; this kit has not been published.

## 2026-10-22: Wiii logo adopted as the Neko Aurora app icon

Darren chose the Wiii logo as the Neko Aurora logo/mascot (no redesign).
- App/PWA/desktop icons (`app/icon-180/192/512.png`, `app/icon-maskable-512.png`, `app/favicon.ico`, `app/desktop/icons/*`) are rendered from `wiii-desktop/public/wiii-mascot-full.png` (1254 px, meiiie/wiii @ `6b1c30b15b4c67e3c841ae870e6989577fc228f8`), placed unchanged on the Wiii Mist→Fog tile gradient (#EEEBE6→#AEABAB).
- `app/icon.svg` is `logo/neko-peek-mark.svg` from the same revision, unchanged.
- `app/aurora/wordmark.svg` pairs that mark (with the graphite head edge for light surfaces) with an outlined Be Vietnam Pro SemiBold (OFL) "Neko Aurora".
These files carry the source artwork's AGPL-3.0 provenance (see above).
