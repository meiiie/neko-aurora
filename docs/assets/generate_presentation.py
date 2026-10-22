"""Regenerate static README SVGs from the existing Neko Peek/Geist assets.
Requires fontTools with WOFF2/Brotli support. No network or runtime dependency.
"""
from pathlib import Path
from html import escape
import re
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont
from fontTools.pens.svgPathPen import SVGPathPen

ROOT = Path(__file__).resolve().parents[2]
OUT = Path(__file__).resolve().parent
BRAND = ROOT / "app/native/internal/web/assets/brand/neko-peek"
FONTS = ROOT / "app/native/internal/web/assets/fonts"
FILL = "#2A2928"


def fonts(weight):
    result = []
    for name in ("geist-latin-wght-normal.woff2", "geist-vietnamese-wght-normal.woff2", "geist-latin-ext-wght-normal.woff2"):
        font = TTFont(FONTS / name)
        if "fvar" in font:
            font = instantiateVariableFont(font, {"wght": weight}, inplace=False)
        result.append((font, font.getBestCmap(), font.getGlyphSet()))
    return result


F600 = fonts(600)
F450 = fonts(450)


def lettering(text, x, baseline, size, weight=600, color=FILL, tracking=0):
    run = F600 if weight == 600 else F450
    paths = []
    cursor = x
    for char in text:
        selected = next((f for f in run if ord(char) in f[1]), None)
        if selected is None:
            raise ValueError("Missing glyph: " + char)
        font, cmap, glyphs = selected
        name = cmap[ord(char)]
        pen = SVGPathPen(glyphs)
        glyphs[name].draw(pen)
        data = pen.getCommands()
        scale = size / font["head"].unitsPerEm
        if data:
            paths.append(f'<path d="{data}" transform="translate({cursor:.3f} {baseline}) scale({scale:.6f} {-scale:.6f})"/>')
        cursor += font["hmtx"][name][0] * scale + tracking
    return f'<g fill="{color}" aria-label="{escape(text, quote=True)}">' + "".join(paths) + "</g>"


def svg(width, height, title, description, body):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}" role="img" aria-labelledby="title desc"><title id="title">{escape(title)}</title><desc id="desc">{escape(description)}</desc><!-- Neko Peek artwork: meiiie/wiii 6b1c30b15b4c67e3c841ae870e6989577fc228f8, AGPL-3.0. Geist glyph source: OFL-1.1. See README.md in this directory; no app-wide license change. -->{body}</svg>\n'


def lockup(x, y, width):
    raw = (BRAND / "nekobot-lockup-light.svg").read_text(encoding="utf-8")
    inner = raw[raw.index(">") + 1:raw.rindex("</svg>")]
    inner = re.sub(r"<title.*?</title>|<desc.*?</desc>|<!--.*?-->", "", inner, flags=re.S)
    return f'<svg x="{x}" y="{y}" width="{width}" height="{width * 96 / 337.4959:.3f}" viewBox="0 0 337.4959 96" aria-hidden="true">{inner}</svg>'


def conversation(x, y, scale=1):
    # Abstract channel motif, intentionally without application chrome or fake UI text.
    return f'<g transform="translate({x} {y}) scale({scale})" aria-hidden="true"><path d="M64 85C160 85 68 237 176 237" fill="none" stroke="#C9C6BC" stroke-width="2"/><rect x="30" y="24" width="240" height="112" rx="26" fill="#E9E8E2"/><circle cx="64" cy="60" r="7" fill="#716D65"/><rect x="85" y="53" width="136" height="12" rx="6" fill="#D3D0C7"/><rect x="59" y="82" width="135" height="10" rx="5" fill="#D9D6CD"/><rect x="81" y="159" width="240" height="112" rx="26" fill="#F5F0E6" stroke="#D9D5CB"/><circle cx="115" cy="195" r="7" fill="#8D8370"/><rect x="136" y="188" width="126" height="12" rx="6" fill="#D6D0C2"/><rect x="110" y="218" width="153" height="10" rx="5" fill="#DED8CB"/><circle cx="280" cy="97" r="25" fill="#FAFAF8" stroke="#D5D2C8"/><path d="M272 97h16m-8-8v16" stroke="#716D65" stroke-width="2" stroke-linecap="round"/></g>'


def banner(mobile=False):
    if mobile:
        w, h = 720, 500
        body = '<rect x=".5" y=".5" width="719" height="499" rx="24" fill="#FAFAF8" stroke="#E5E3DC"/>'
        body += lockup(40, 30, 295)
        body += lettering("Đa bot.", 40, 222, 67, tracking=-1.4)
        body += lettering("Đa kênh.", 40, 300, 67, tracking=-1.4)
        body += lettering("Ưu tiên dữ liệu cục bộ.", 42, 375, 29, weight=450, color="#65615A")
        body += conversation(408, 136, .72)
        body += '<circle cx="49" cy="441" r="5" fill="#8D8370"/>'
        body += lettering("Đang phát triển", 65, 449, 23, weight=450, color="#65615A")
    else:
        w, h = 1200, 400
        body = '<rect x=".5" y=".5" width="1199" height="399" rx="24" fill="#FAFAF8" stroke="#E5E3DC"/>'
        body += lockup(58, 35, 315)
        body += lettering("Đa bot. Đa kênh.", 64, 218, 67, tracking=-1.4)
        body += lettering("Ưu tiên dữ liệu cục bộ.", 66, 275, 31, weight=450, color="#65615A")
        body += '<circle cx="72" cy="340" r="5" fill="#8D8370"/>'
        body += lettering("Đang phát triển", 88, 348, 22, weight=450, color="#65615A")
        body += conversation(800, 52, 1.05)
    return svg(w, h, "Neko Aurora — đa bot, đa kênh", "Logo Neko Peek trên nền ivory, cùng minh họa trừu tượng các luồng hội thoại. Bản phát triển, ưu tiên dữ liệu cục bộ; đây không phải ảnh ứng dụng.", body)


def badge(name, text, width):
    body = f'<rect x=".5" y=".5" width="{width - 1}" height="31" rx="8" fill="#F5F4EF" stroke="#DBD8CE"/>'
    body += lettering(text, 12, 21.5, 13, weight=450, color="#454241")
    return svg(width, 32, name, "Nhãn tĩnh mô tả trạng thái hoặc kiến trúc; không phải kết quả CI hay giấy phép.", body)


if __name__ == "__main__":
    OUT.mkdir(exist_ok=True)
    (OUT / "badges").mkdir(exist_ok=True)
    for name, content in {
        "neko-bot-banner.svg": banner(),
        "neko-bot-banner-mobile.svg": banner(True),
        "badges/development.svg": badge("Đang phát triển", "Status · Development", 165),
        "badges/stack.svg": badge("Tauri, Rust và Go", "Tauri · Rust · Go", 133),
        "badges/runtime.svg": badge("Runtime: Neko Core", "Runtime · Neko Core", 160),
    }.items():
        (OUT / name).write_text(content, encoding="utf-8", newline="\n")
