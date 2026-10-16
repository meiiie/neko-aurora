# Local bounded DOCX extraction — 2026-10-12

`native/internal/documents` adds a local, dependency-free DOCX text reader. It uses Go's `archive/zip`, `encoding/xml`, and URL syntax parsing. It does not write extracted archive entries, execute document content, open hyperlinks, load external resources, or call providers. Neko Core retains ownership of the agent loop.

## Integration contract

```go
func ExtractDOCX(raw []byte) (Result, error)

type Result struct {
    Text       string
    TextSHA256 string
    TextBytes  int
}
```

`Text` is the complete extracted UTF-8 main-document text. `TextSHA256` is its lowercase hexadecimal SHA-256; `TextBytes` counts UTF-8 bytes. Errors return a zero `Result`, never partial text. Callers can use `errors.As(err, &rejected)` with `var rejected *documents.Error`. `Error.Kind` is one of `invalid_docx`, `unsafe_docx`, `limit_exceeded`, `corrupt_docx`, or `invalid_xml`. `Error.Reason` is a fixed identifier; it never includes source text, filenames, relationship targets, or parser diagnostics.

The integration owns the original raw SHA-256, extraction version/format, full-text secret screening, quarantine state, receipt/descriptor persistence, and a UTF-8-safe 16,000-byte excerpt with truncation metadata. Screen all `Text` before admitting `ready_text`; a safe excerpt does not prove the complete extraction is safe. A suggested version label is `local-docx-text-v1`.

## Bounds and supported content

| Check | Limit |
| --- | ---: |
| Raw DOCX bytes | 1,048,576 |
| ZIP entries, including directories | 256 |
| Total claimed uncompressed bytes | 8,388,608 |
| Total accepted actual uncompressed bytes | 8,388,608 |
| Each XML or relationship part | 2,097,152 |
| Complete extracted UTF-8 text bytes | 524,288 |
| XML element depth | 64 |
| Archive entry name bytes | 1,024 |

Each archive stream is limited before reading. A one-byte overrun witness triggers rejection. Every accepted entry is fully read and checked for ZIP CRC and claimed/actual size agreement, including unused media. Directory entries must have no payload. These are in-process parser quotas; they do not constitute an OS sandbox or a hard wall-clock timeout.

The reader requires `[Content_Types].xml`, `_rels/.rels`, and `word/document.xml`, validates the OOXML package and main-document namespaces/content type, and requires one root office-document relationship targeting the canonical document part. Transitional and strict Word namespaces are supported. Main document paragraphs become line-separated text; table cells use tabs and rows use line breaks. Text, explicit tabs, explicit breaks, Unicode, and empty documents are preserved. Only structural terminal paragraph/row newlines are normalized away; explicit final line breaks remain.

The reader rejects DTD/directive declarations, unknown entities, other processing instructions, malformed XML, duplicate attributes, multiple roots, namespace mismatches, duplicate/case-colliding entries, traversal/ambiguous names, Windows special names, symlinks/special file types, encrypted or unsupported-compression entries, active/macro/embedded-code parts/content types, dangling relationships, and escaping internal targets. Normal canonical hyperlink relationships with `http`, `https`, or `mailto` targets are inert: only visible Word text is extracted, and the target is never fetched. File links, other URI schemes, credential-bearing URL targets, and other external relationships are rejected.

Headers, footers, footnotes, comments, metadata, field instructions, image/OCR text, rendering, pagination, and noncanonical main-document locations are outside this extraction format. Inert media is integrity-checked and ignored. This is a bounded text subset, not a complete OOXML schema validator or rendered Word view. Unsupported/unsafe input remains eligible for integration quarantine rather than text readiness.

## Validation provenance

Recorded at `2026-10-12T01:22:10Z`. Exact repository HEAD was `fef7758f963e54bdb4f3356b9053564523ed6e3f`; the tested additions and concurrent integration edits were **working tree**, not a code commit. No staging/checkpoint/push was performed by this worker. The integrating owner must scan checkpoint candidates for secrets, record the resulting exact code commit, and rerun integrated acceptance on that commit.

Environment: Windows/amd64, PowerShell, explicit Go `go1.27.2 windows/amd64` from `C:\Users\Admin\Documents\Codex\2026-10-11\task\.tooling\go\bin\go.exe`. Working directory for Go commands was `C:\Users\Admin\Documents\Codex\2026-10-11\task\checkpoint-verify-20261011\native`. No third-party dependency was added. Provider credential environment variables were removed in each child shell without printing values. Network dependency resolution was disabled. Caches, temporary files, fuzz cache, and command logs remained under `E:\CodexScratch\local-ux-fef7758\docx-owned`.

The final commands used this child-process setup:

```powershell
$scratch = 'E:\CodexScratch\local-ux-fef7758\docx-owned'
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:GOTOOLCHAIN = 'local'
$env:GOCACHE = "$scratch\cache"
$env:GOTMPDIR = "$scratch\tmp"
$env:TEMP = "$scratch\tmp"
$env:TMP = "$scratch\tmp"
Get-ChildItem Env: |
    Where-Object { $_.Name -match '(?i)(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL|OPENAI|ANTHROPIC|GROQ|GEMINI|XAI|AZURE|AWS_|GOOGLE_|HF_|HUGGINGFACE|COHERE|MISTRAL|DEEPSEEK|NEBIUS|TOGETHER)' } |
    ForEach-Object { Remove-Item -LiteralPath ('Env:' + $_.Name) }
Remove-Item -LiteralPath Env:NEKO_CONTEXT_MANIFEST_FIXTURE_DIR -ErrorAction SilentlyContinue
```

| Result | Exact Go/gofmt command arguments | Evidence |
| --- | --- | --- |
| FAIL, expected TDD red | `test ./internal/documents -count=1 -v` against a typed-error placeholder | `output/tdd-red.txt`; Unicode/table success and safety/quota expectations failed before implementation |
| FAIL, expected added boundary red | `test ./internal/documents -count=1 -run 'TestExtractDOCX(RejectsSymlinkDirectoryAndWindowsInvalidNames\|RejectsActiveContentAtArchiveRoot\|AcceptsExactTextQuota\|PreservesExplicitFinalBreak)$' -v` | `output/boundary-red.txt`; boundary regressions exposed before fixes; directory/symlink fixture was subsequently corrected to encode a symlink entry with a trailing slash |
| FAIL, expected hyperlink red | `test ./internal/documents -count=1 -run TestExtractDOCXPreservesWebHyperlinkDisplayWithoutFollowingIt -v` | `output/hyperlink-red.txt`; local visible-link extraction initially rejected |
| PASS | `gofmt.exe -w internal/documents/docx.go internal/documents/docx_test.go` | Final formatted source hashes below |
| PASS | `test ./internal/documents -count=1 -v`, with the explicit read-only reference environment variable below | `output/package-accepted-reference.txt`; 23 top-level passes including the read-only smoke test and fuzz seeds |
| PASS | `test ./... -count=1`, with `GOFLAGS=''` and reference/manifest fixture env unset | `output/native-suite.txt`; all eight test-bearing native packages passed, two packages had no tests; native fixture build tags were not enabled |
| PASS | `vet ./internal/documents` | `output/vet.txt`; exit 0, no diagnostics |
| PASS, smoke only | `test ./internal/documents -run '^$' -fuzz=FuzzExtractDOCX -fuzztime=10s -parallel=1` | `output/fuzz-final.txt`; three baseline seeds, 33,297 executions, one new interesting input, exit 0 |

Each command output was captured through `2>&1 | Tee-Object -FilePath "$scratch\output\<evidence file>"`, with the Go process exit code propagated using `exit $LASTEXITCODE`. Historical `package-with-reference.txt` records the initial reference failure under the stricter all-external-relationships policy; it is superseded by `package-accepted-reference.txt` after adding inert standard hyperlinks.

The reference smoke test explicitly set:

```powershell
$env:NEKO_DOCX_READONLY_REFERENCE = 'E:\Sach\Sua\NekoBot\reference\handoff\Neko_Dinh_huong_phat_trien.docx'
```

The source was read in place, not copied into the package or committed. Source size: `562053` bytes. Raw SHA-256: `7b81dffc6f53f08d257e996827d38a649078a29c504990791921848b7c28d465`. Full extracted UTF-8 size: `226819` bytes. Text SHA-256: `984212a0666a05501aa2d6899b27954792ef4b39e6509ce96839eb5a6169ae2b`. A read-only relationship inventory counted 73 ordinary external hyperlinks and zero other external relationships; no target or document text was printed.

Final raw file SHA-256 values:

| File | SHA-256 |
| --- | --- |
| `native/internal/documents/docx.go` | `a72714c663d135bd740cdfc50d676fff343bcba5fe1ec0d95a65d2c17f5ff81a` |
| `native/internal/documents/docx_test.go` | `638daaf30c141c4037d5e6b6409dbb17eec65cd3e61a311bb8584c3b09188970` |

These checks establish local synthetic parser behavior plus one read-only source extraction. They do not establish cloud sync, model evaluation, provider execution, OS isolation, native-window acceptance, browser/phone acceptance, or complete Office compatibility. Upload/preview/quarantine integration and exact committed-code acceptance remain the integrating owner's work.
