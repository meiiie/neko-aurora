import hashlib
import pathlib
import re
import subprocess
import sys

root = pathlib.Path(sys.argv[1]).resolve()
excluded = {".git", ".state", ".build", ".checkpoints", "references", "archives", "node_modules", "target"}
rules = [
    ("private-key", re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----")),
    ("provider-token", re.compile(r"\b(?:sk-(?:proj-)?[A-Za-z0-9_-]{24,}|gh[pousr]_[A-Za-z0-9]{30,}|AKIA[A-Z0-9]{16}|xox[baprs]-[A-Za-z0-9-]{20,})\b")),
    ("credential-assignment", re.compile(r"""(?i)(?:api[_-]?key|access[_-]?token|password|client[_-]?secret)\s*[:=]\s*["']([^"'\r\n]{12,})["']""")),
]
findings = []
count = 0
reviewed_fonts = {
    "native/internal/web/assets/fonts/geist-latin-wght-normal.woff2": "19f9c92546aa300c312235e3125af1b81394d8db9a4bc4a425cd5b641d2d54e1",
    "native/internal/web/assets/fonts/geist-latin-ext-wght-normal.woff2": "824f485b5d26e2f2da3c2b236132ece1bc8e4e43373452950bb0e40548b4313f",
    "native/internal/web/assets/fonts/geist-vietnamese-wght-normal.woff2": "8fa40e5d248247735eb97a0bd593b8852440430600d6ba01364c31fe0abc1fe1",
}
reviewed_synthetic_probes = {
    "native/internal/ledger/review_source_export_probe_test.go": "b214b0a94bc41a97210f0d59f45aea2cf7204d1ad072a16a8f77e75f6f44589f",
    "native/internal/ledger/review_metadata_receipts_probe_test.go": "7d2b4759de4b32f8bccdab3cc07121cdecf6ff762b286457193c4c672fb85a1d",
}
git_files = subprocess.run(["git", "-C", str(root), "ls-files", "--cached", "--others", "--exclude-standard", "-z"], capture_output=True)
candidates = [root / name for name in git_files.stdout.decode("utf-8").split("\0") if name] if git_files.returncode == 0 else list(root.rglob("*"))
for path in sorted(candidates):
    if not path.is_file() or any(part in excluded for part in path.relative_to(root).parts):
        continue
    rel = path.relative_to(root)
    probe_hash = reviewed_synthetic_probes.get(rel.as_posix())
    if probe_hash:
        if hashlib.sha256(path.read_bytes().replace(b"\r\n", b"\n")).hexdigest() != probe_hash:
            findings.append((str(rel), 0, "reviewed-synthetic-probe-hash-mismatch"))
        continue
    font_hash = reviewed_fonts.get(rel.as_posix())
    if font_hash:
        if hashlib.sha256(path.read_bytes()).hexdigest() != font_hash:
            findings.append((str(rel), 0, "reviewed-font-hash-mismatch"))
        continue
    if path.name == ".env" or (path.name.startswith(".env.") and path.name != ".env.example") or path.suffix.lower() in {".pem", ".key", ".p12", ".pfx"}:
        findings.append((str(rel), 0, "credential-file"))
        continue
    if str(rel).replace("\\", "/") == "desktop/icons/icon.ico":
        continue
    try:
        content = path.read_text(encoding="utf-8")
    except UnicodeError:
        findings.append((str(rel), 0, "unreviewed-binary"))
        continue
    count += 1
    for line_number, line in enumerate(content.splitlines(), 1):
        for name, pattern in rules:
            match = pattern.search(line)
            if match and not (name == "credential-assignment" and re.search(r"(?i)example|placeholder|test-only|fixture", match.group(1))):
                findings.append((str(rel), line_number, name))
for path, line, rule in findings:
    print(f"{path}:{line}: {rule}")
print(f"Scanned {count} text files; findings={len(findings)}; values redacted")
sys.exit(bool(findings))
