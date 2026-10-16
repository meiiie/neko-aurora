"""Check public onboarding links, passive SVGs and versioned fixture screenshots.
Standard library only; this is a docs check, not application acceptance.
"""
from pathlib import Path
from html.parser import HTMLParser
from urllib.parse import unquote, urlsplit
import hashlib
import re
import sys
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
DOCS = ["README.md", "CONTRIBUTING.md", "SECURITY.md", "SUPPORT.md", "docs/README.md", "docs/getting-started.md", "docs/architecture.md", "docs/project-status.md", "docs/ROADMAP.md", "docs/assets/README.md", ".github/PULL_REQUEST_TEMPLATE.md"]
errors = []
links_checked = 0


def anchors(text):
    result = set()
    duplicates = {}
    for title in re.findall(r"^#{1,6} (.+)$", text, re.M):
        title = re.sub(r"<[^>]+>", "", title).replace("`", "").lower().strip()
        slug = "".join(c for c in title if c.isalnum() or c.isspace() or c in "-_").replace(" ", "-")
        count = duplicates.get(slug, 0)
        result.add(slug + ("-" + str(count) if count else ""))
        duplicates[slug] = count + 1
    return result


class Images(HTMLParser):
    def __init__(self):
        super().__init__()
        self.targets = []

    def handle_starttag(self, tag, attrs):
        for name, value in attrs:
            if tag in ("img", "source") and name in ("src", "srcset"):
                self.targets.append(value)


def target_exists(source, target):
    global links_checked
    url = urlsplit(unquote(target))
    if url.scheme or url.netloc:
        return
    path = (source.parent / url.path).resolve() if url.path else source
    links_checked += 1
    if not path.is_relative_to(ROOT) or not path.is_file():
        errors.append(f"{source.relative_to(ROOT)}: missing local target {target}")
    elif url.fragment and path.suffix == ".md" and url.fragment not in anchors(path.read_text(encoding="utf-8")):
        errors.append(f"{source.relative_to(ROOT)}: missing heading {target}")


for name in DOCS:
    source = ROOT / name
    text = source.read_text(encoding="utf-8")
    if re.search(r"(?i)\b[a-z]:[\\/]|libfile_|DESKTOP-PLLO4HG|LAPTOP-LNGFG0JP", text):
        errors.append(name + ": workstation path/private artifact identifier")
    for target in re.findall(r"!?\[[^\]]*\]\(([^)]+)\)", text):
        target_exists(source, target)
    parser = Images()
    parser.feed(text)
    for target in parser.targets:
        target_exists(source, target)

for path in (ROOT / "docs/assets").rglob("*.svg"):
    root = ET.fromstring(path.read_text(encoding="utf-8"))
    for element in root.iter():
        tag = element.tag.split("}")[-1]
        if tag in {"script", "foreignObject", "image", "animate", "set"}:
            errors.append(str(path.relative_to(ROOT)) + ": active/external SVG element")
        if any(name.startswith("on") or name.endswith("href") for name in element.attrib):
            errors.append(str(path.relative_to(ROOT)) + ": event or href attribute")

for name, expected in {
    "report-desktop-2a74029.png": "b068acbf477960f90119689b4c09fec51e3a21789d111731a1b2f916fb71f1cb",
    "report-mobile-2a74029.png": "5c96899df1018277689ad2b7b279a646c69974f2023cf9f892dd59bef2d018b5",
}.items():
    if hashlib.sha256((ROOT / "docs/assets/screenshots" / name).read_bytes()).hexdigest() != expected:
        errors.append(name + ": fixture image bytes changed; review provenance/version before replacing")

if (ROOT / ".github/workflows").exists():
    errors.append("Root workflows were added; review real execution and update CI statements before continuing")

for error in errors:
    print(error)
print(f"Docs check: {len(DOCS)} documents, {links_checked} local links; errors={len(errors)}. No app/native/provider test implied.")
sys.exit(bool(errors))
