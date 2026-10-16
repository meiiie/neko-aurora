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
git_files = subprocess.run(["git", "-C", str(root), "ls-files", "--cached", "--others", "--exclude-standard", "-z"], capture_output=True)
candidates = [root / name for name in git_files.stdout.decode("utf-8").split("\0") if name] if git_files.returncode == 0 else list(root.rglob("*"))
for path in sorted(candidates):
    if not path.is_file() or any(part in excluded for part in path.relative_to(root).parts):
        continue
    rel = path.relative_to(root)
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
