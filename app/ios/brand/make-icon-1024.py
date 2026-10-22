# Re-export the 1024 px App Store icon master from the original Wiii mascot
# (meiiie/wiii @ 6b1c30b, wiii-desktop/public/wiii-mascot-full.png, 1254 px: a
# DOWNscale, never an upscale) on the Wiii Mist→Fog gradient, with the same
# placement as app/icon-512.png. AGPL-3.0 provenance: see
# app/native/internal/web/assets/brand/neko-peek/PROVENANCE.md.
# Usage: python3 make-icon-1024.py /path/to/wiii-mascot-full.png brand/icon-1024.png
import sys
from PIL import Image
src, out = sys.argv[1], sys.argv[2]
m = Image.open(src).convert("RGBA")
S, k = 1024, 2
g = Image.new("RGB", (S, S))
for y in range(S):
    t = y / (S - 1)
    g.paste(tuple(round(a + (b - a) * t) for a, b in zip((0xEE, 0xEB, 0xE6), (0xAE, 0xAB, 0xAB))), (0, y, S, y + 1))
sz = round(1254 * 0.447 * k)
r = m.resize((sz, sz), Image.LANCZOS)
g.paste(r, (round(-20.2 * k), round(-2.4 * k)), r)
g.save(out)  # RGB: opaque, as App Store Connect requires
