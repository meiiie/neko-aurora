# Licensed local fonts for the reference UI

The Go service exposes exactly three additional public GET paths:

- `/fonts/geist-latin-wght-normal.woff2`
- `/fonts/geist-latin-ext-wght-normal.woff2`
- `/fonts/geist-vietnamese-wght-normal.woff2`

Each response uses `font/woff2` and retains the local host gate, `nosniff` and the existing cache/session rules. CSP adds only `font-src 'self'`; inline scripts/styles, data fonts and external font origins remain disallowed. The public static whitelist also remains the boundary inside `web.Serve`. No directory listing, arbitrary embedded-file access, brand path or broader filesystem route is introduced.

The unchanged font bytes come from `NekoBot-01-Huong-dan-Logo-Kiem-thu.zip`, member prefix `package/design/nekobot-brand-v1/source/fonts/`. They match the UI worker's copies. The original copyright and SIL Open Font License 1.1 are preserved verbatim in `native/internal/web/assets/fonts/Geist-OFL-1.1.txt`; source members and SHA256 values are recorded in `docs/evidence/2026-10-11-fonts/reference-proof.json`. A local binary distribution must accompany the executable with that license, as done in `.build/licenses/Geist-OFL-1.1.txt`.

This supports the separate light reference UI. It does not alter the primary HTML/CSS/JS, imply visual acceptance, or add group/project/upload/cancel-all APIs. The font routes are independent of the permission runtime checkpoint 33efa44.

`TestEmbeddedFontResponseAndStaticBoundary` checks original font hashes, MIME, CSP, unlisted/traversal paths, non-GET requests and hostile Host values without a network listener. `scripts/test_fonts_local.py` starts an owned Go service and isolated headless Chrome; it checks that all three FontFace objects load under the actual response CSP and that the same font in a data URL is blocked. The latter is a browser/font gate, not a phone or native visual acceptance test.

Final code checkpoint `91260906defd3a64578583cafe849931b1b14688`: clean full Go suite, vet and build PASS. Actual Go/Chrome FontFace gate PASS: all three same-origin fonts loaded and the data URL was rejected with a `font-src` violation. Raw reports and build metadata are in `docs/evidence/2026-10-11-fonts`, with the summary in `validation-fonts-9126090.json`. Go executable SHA256 is `e5de1f6af82ac563e8e2368ef55b1df8370611d7fcf5c1b1d3e3cb7ef5596df9`. Ledger, runtime manager and Rust host source are byte-identical to runtime checkpoint 33efa44; those native permission/Core tests remain attributed to that checkpoint.
