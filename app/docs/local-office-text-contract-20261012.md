# Local XLSX/PPTX text extraction contract — 2026-10-12

Status: working-tree parser delivery for L06. Task-start source baseline was cb69ddb8a42fcc73f4d3efa1c28fea7211022f22; shared source HEAD at freeze inspection was d5c202e140592db02d3c2b3a5997ff2074551909. These are source context, not a claim that this new parser was tested at either commit. Root owns the final integrated checkpoint, candidate secret scan, and exact-commit verification.

## Interface and ownership

The documents package adds ExtractXLSX(raw []byte) (Result, error) and ExtractPPTX(raw []byte) (Result, error). Result is the existing typed DTO with Text string, TextSHA256 string, and TextBytes int. Text is complete extracted UTF-8 plain text, TextBytes counts bytes, and TextSHA256 is lowercase SHA-256 over precisely those bytes. Failure returns the zero Result and a typed *documents.Error; no partial output or silent truncation.

New ErrorKind values: invalid_office, unsafe_office, corrupt_office. Shared invalid_xml and limit_exceeded are retained. Reasons are static labels and do not include source contents. Root should quarantine any extraction failure and never mark it ready_text.

Owned changes are only office_text.go, office_text_test.go, and this document. DOCX implementation/tests remain unchanged. New source and synthetic fixtures were authored locally using Go's standard archive/zip and encoding/xml; no third-party package, copied Office/Grok code or asset, network resource, install, external parser executable, or new license grant is involved.

## Accepted text subset

Both formats require a real OOXML ZIP package with OPC content-types and package relationships in the correct namespaces. The main document relationship must point to the canonical xl/workbook.xml or ppt/presentation.xml with the exact non-macro XLSX/PPTX main MIME type. Required worksheet/slide/sharedStrings MIME types and paired transitional or strict namespaces are checked. A filename or arbitrary ZIP alone cannot select valid Office content.

XLSX follows workbook worksheet relationships, requires explicit valid cell addresses A1 through XFD1048576, and produces sheets in ascending numeric worksheet-part order, then cells in ascending row/column order. Part names must be canonical sheetN.xml without numeric aliases. Each sheet header includes its name, sheet ID, hidden/veryHidden state when present, and internal part path. Duplicate sheet IDs/names/targets and duplicate cell addresses reject.

Cell extraction supports shared strings, inline/rich strings, finite numeric cached values, booleans, literal strings/errors, and ISO date cells. Rich text runs concatenate; phonetic annotations are excluded. Whitespace and escaped literal script/backtick text remain data. Formatting/styles are not applied, so a date stored as a formatted numeric serial remains its raw number. A formula is never evaluated and its source is not exported. Cached formula values are explicitly labelled; an absent cache is labelled [no cached value]. Caches may be stale. Conservative external/DDE/web/XLM function/reference markers reject even though no formula execution exists.

PPTX follows presentation slide relationships and produces slides in ascending numeric slide-part order. Each header includes its internal part path. Slide text comes from DrawingML text within the slide's common shape tree, in XML shape/text order, including table text. Paragraph boundaries, explicit breaks and tabs are preserved; trailing structural paragraph separators are normalized while explicit breaks remain. It does not render transforms, infer reading order from geometry, perform OCR, extract notes/charts/comments, or match the presentation's UI slide order. Hidden slides are still text-extracted.

The subset deliberately rejects unsupported payload extensions, chartsheets/dialog sheets, noncanonical worksheet/slide targets, and packages whose safe text subset cannot be established. Inert image/audio/video extensions may be present; their bytes are integrity-checked but never decoded, rendered, or executed. This is text extraction, not a complete Office schema or media validator.

## Safety and resource bounds

| Resource | Hard limit |
| --- | --- |
| Raw package | 1 MiB |
| ZIP entries, including directories | 256 |
| Sum of claimed and actual uncompressed data | 8 MiB |
| Each XML/rels part, claimed and actual | 2 MiB |
| XML nesting depth | 64 |
| Complete extracted UTF-8 text, including provenance | 512 KiB |
| Shared string count and total shared-string UTF-8 bytes | 65,536 and 512 KiB |
| Cells per worksheet | 65,536 |
| Selected worksheet/slide parts | 128 |

All entries, including unused media/XML, are read within quotas and checked for CRC, claimed-size mismatch, duplicates (case insensitive), malformed names/traversal, symlinks/special modes, unsupported compression, encryption flags, and unsafe content types. ZIP directories must carry no payload. Unknown non-XML/non-media payloads fail closed. Macro parts/types, ActiveX/OLE/embedded objects, scripts, external links/data, DTD/directives, non-XML processing instructions, and malformed XML/entities reject. XML predefined escapes such as &amp; are valid literal data.

Every relationship part is checked, including relationships unused by extraction. Only local, resolving internal targets are accepted; external relationships—including inert hyperlinks—reject. Internal parent traversal is accepted only after canonical resolution remains inside the ZIP and points to a safe existing part. No filesystem extraction, file opening, link following, external resource loading, script execution, or formula calculation exists in the extractor.

These are in-process memory/input quotas, not an OS sandbox or a hard wall-clock deadline. ZIP/XML parsing and CRC remain standard-library operations.

## Root integration boundary

Root owns UploadFile dispatch, raw SHA-256, authentication/authorization, quarantine/readiness, extraction version/format metadata, UTF-8 excerpt truncation at 16,000 bytes, and screening the full extracted text for secrets before publication. Extraction should occur outside the ledger lock. Result.Text is untrusted plain data and UI rendering must escape it as text. A successful parser result does not grant file access, download readiness, provider use, or sharing authority.

PDF remains outside this module. Installed Python PDF libraries do not make the standalone Neko app PDF-ready. Legacy binary XLS/PPT, encrypted packages and macro-enabled variants are unsupported. No standalone PDF dependency/license/runtime route is claimed.

## Validation evidence

Environment: Windows/amd64, go version go1.27.2 windows/amd64, explicit executable C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe. The minimal overlay at E:/CodexScratch/local-daily-use-20261012/office-worker/overlay contains only native/go.mod and the documents package; it has no Git checkout.

All caches, temp files, outputs and receipts are below E:/CodexScratch/local-daily-use-20261012/office-worker. GOPROXY=off, GOSUMDB=off, GOTOOLCHAIN=local, GOFLAGS empty. GOCACHE points to cache; GOTMPDIR/TEMP/TMP point to tmp. Provider credential environment names are removed in the child shell without printing values, and NEKO_DOCX_READONLY_REFERENCE is unset.

Reproduction setup and final commands from the overlay directory:

~~~powershell
$officeScratch = 'E:\CodexScratch\local-daily-use-20261012\office-worker'
$env:GOPROXY='off'; $env:GOSUMDB='off'; $env:GOTOOLCHAIN='local'
$env:GOCACHE="$officeScratch\cache"; $env:GOTMPDIR="$officeScratch\tmp"
$env:TEMP="$officeScratch\tmp"; $env:TMP="$officeScratch\tmp"; $env:GOFLAGS=''
Get-ChildItem Env: | Where-Object { $_.Name -match '(?i)(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL|OPENAI|ANTHROPIC|GROQ|GEMINI|XAI|AZURE|AWS_|GOOGLE_|HF_|HUGGINGFACE|COHERE|MISTRAL|DEEPSEEK|NEBIUS|TOGETHER)' } | ForEach-Object { Remove-Item -LiteralPath ('Env:' + $_.Name) }
Remove-Item -LiteralPath Env:NEKO_DOCX_READONLY_REFERENCE -ErrorAction SilentlyContinue
& 'C:\Users\Admin\Documents\Codex\2026-10-11\task\.tooling\go\bin\go.exe' test ./... -count=1 -v
& 'C:\Users\Admin\Documents\Codex\2026-10-11\task\.tooling\go\bin\go.exe' vet ./...
~~~

| Evidence | Result and scope |
| --- | --- |
| output/tdd-red.txt | FAIL (exit 1), genuine initial tests against placeholder implementation |
| output/office-edge-red.txt | FAIL (exit 1), exposed inline phonetics/shared-formula presence, numeric alias, foreign shared-text namespace and out-of-tree slide text |
| output/office-payload-red.txt | FAIL (exit 1), exposed unrecognized payload acceptance |
| output/office-cache-red.txt | FAIL (exit 1), exposed missing bool/date/shared-string formula cache handling |
| output/office-final-package-test-frozen.txt | PASS (exit 0), 18 Office test functions plus existing DOCX tests and seed cases |
| output/office-final-vet-frozen.json | PASS (exit 0), go vet ./... with empty diagnostics |
| Existing read-only DOCX reference smoke | SKIP, explicit reference environment unset |

Fixtures are genuine locally generated OOXML ZIP/XML structures, including Unicode, numeric ordering, rich strings, tables/text, formula caches, strict namespaces, malformed types/parts, CRC/encryption, duplicates/symlinks/traversal, DTD/depth, and raw/XML/count/text quota failures. These are parser fixtures; they are not Office application rendering acceptance, model evaluation, hostile-input fuzz campaign, cloud sync, or full application integration tests. Root must run the fresh combined tests after the exact integrated checkpoint. No child commit/staging/Git mutation occurred.

Frozen parser source SHA-256:

| File | SHA-256 |
| --- | --- |
| native/internal/documents/office_text.go | e3046567c6b12f8cc513e3f205d77ff689b8d6e099f1666f3b07a15974ba4776 |
| native/internal/documents/office_text_test.go | a403c0e01ce5c913e72a6e896e28e93a6bea48fc8e2e2334f1cb684b4aaa3bc9 |
| Existing docx.go, unchanged in overlay/shared source | a72714c663d135bd740cdfc50d676fff343bcba5fe1ec0d95a65d2c17f5ff81a |
| Existing docx_test.go, unchanged in overlay/shared source | 638daaf30c141c4037d5e6b6409dbb17eec65cd3e61a311bb8584c3b09188970 |

