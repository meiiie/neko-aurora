package documents

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
)

const wordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
const contentTypesNS = "http://schemas.openxmlformats.org/package/2006/content-types"
const relationshipsNS = "http://schemas.openxmlformats.org/package/2006/relationships"
const officeDocumentType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
const documentContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"

type fixturePart struct {
	name string
	data string
	mode os.FileMode
}

func docXML(body string) string {
	return `<w:document xmlns:w="` + wordNS + `"><w:body>` + body + `</w:body></w:document>`
}

func baseParts(document string) []fixturePart {
	return []fixturePart{
		{"[Content_Types].xml", `<Types xmlns="` + contentTypesNS + `"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="` + documentContentType + `"/></Types>`, 0},
		{"_rels/.rels", `<Relationships xmlns="` + relationshipsNS + `"><Relationship Id="rId1" Type="` + officeDocumentType + `" Target="word/document.xml"/></Relationships>`, 0},
		{"word/document.xml", document, 0},
	}
}

func archiveFixture(t *testing.T, parts []fixturePart) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, p := range parts {
		h := &zip.FileHeader{Name: p.name, Method: zip.Deflate}
		if p.mode != 0 {
			h.SetMode(p.mode)
		}
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(p.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func rejection(t *testing.T, raw []byte, kind ErrorKind) {
	t.Helper()
	result, err := ExtractDOCX(raw)
	var rejected *Error
	if !errors.As(err, &rejected) || rejected.Kind != kind {
		t.Fatalf("got result=%#v error=%v; want typed rejection %q", result, err, kind)
	}
	if result != (Result{}) {
		t.Fatalf("rejected extraction returned partial text: %#v", result)
	}
}

func mutateCentral(t *testing.T, raw []byte, name string, offset int, value uint32) []byte {
	t.Helper()
	copyRaw := append([]byte(nil), raw...)
	for i := 0; i+46 <= len(copyRaw); i++ {
		if !bytes.Equal(copyRaw[i:i+4], []byte("PK\x01\x02")) {
			continue
		}
		n := int(binary.LittleEndian.Uint16(copyRaw[i+28 : i+30]))
		if i+46+n <= len(copyRaw) && string(copyRaw[i+46:i+46+n]) == name {
			binary.LittleEndian.PutUint32(copyRaw[i+offset:i+offset+4], value)
			return copyRaw
		}
	}
	t.Fatalf("central entry %s missing", name)
	return nil
}

func TestExtractDOCXPreservesUnicodeParagraphTableTabAndBreak(t *testing.T) {
	body := `<w:p><w:r><w:t>Xin chào 猫 &amp; bạn</w:t><w:tab/><w:t>tab</w:t><w:br/><w:t>line</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>B</w:t></w:r></w:p><w:p><w:r><w:t>C</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`<w:p><w:r><w:t>last</w:t><w:cr/><w:t>end</w:t></w:r></w:p>`
	got, err := ExtractDOCX(archiveFixture(t, baseParts(docXML(body))))
	if err != nil {
		t.Fatal(err)
	}
	want := "Xin chào 猫 & bạn\ttab\nline\nA\tB\nC\nlast\nend"
	if got.Text != want || got.TextBytes != len(want) {
		t.Fatalf("got %#v, want text %q and UTF-8 length %d", got, want, len(want))
	}
	sum := sha256.Sum256([]byte(want))
	if got.TextSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("wrong text digest %q", got.TextSHA256)
	}
}

func TestExtractDOCXStrictNamespace(t *testing.T) {
	doc := strings.ReplaceAll(docXML(`<w:p><w:r><w:t>strict</w:t></w:r></w:p>`), wordNS, "http://purl.oclc.org/ooxml/wordprocessingml/main")
	parts := baseParts(doc)
	parts[1].data = strings.ReplaceAll(parts[1].data, officeDocumentType, "http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument")
	got, err := ExtractDOCX(archiveFixture(t, parts))
	if err != nil || got.Text != "strict" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestExtractDOCXRejectsNonWordPackages(t *testing.T) {
	for _, test := range []struct {
		name  string
		parts []fixturePart
	}{
		{"plain_zip", []fixturePart{{"notes.txt", "not a docx", 0}}},
		{"missing_relationship", append(baseParts(docXML(""))[:1], baseParts(docXML(""))[2])},
		{"wrong_document_namespace", baseParts(`<document xmlns="urn:fake"><body><t>not word</t></body></document>`)},
		{"missing_body", baseParts(`<w:document xmlns:w="` + wordNS + `"/>`)},
		{"two_bodies", baseParts(`<w:document xmlns:w="` + wordNS + `"><w:body/><w:body/></w:document>`)},
	} {
		t.Run(test.name, func(t *testing.T) { rejection(t, archiveFixture(t, test.parts), InvalidDOCX) })
	}
	parts := baseParts(docXML(""))
	parts[0].data = strings.ReplaceAll(parts[0].data, documentContentType, "application/xml")
	rejection(t, archiveFixture(t, parts), InvalidDOCX)
}

func TestExtractDOCXRejectsMacroAndEmbeddedCode(t *testing.T) {
	for _, name := range []string{"word/vbaProject.bin", "word/activeX/activeX1.xml", "word/embeddings/oleObject1.bin", "word/script.js"} {
		t.Run(name, func(t *testing.T) {
			parts := append(baseParts(docXML("")), fixturePart{name, "unsafe", 0})
			rejection(t, archiveFixture(t, parts), UnsafeDOCX)
		})
	}
	parts := baseParts(docXML(""))
	parts[0].data = strings.ReplaceAll(parts[0].data, documentContentType, "application/vnd.ms-word.document.macroEnabled.main+xml")
	rejection(t, archiveFixture(t, parts), UnsafeDOCX)
}

func TestExtractDOCXRejectsUnsafeEntryNamesAndLinks(t *testing.T) {
	for _, name := range []string{"../outside.xml", "/absolute.xml", `word\bad.xml`, "word/./bad.xml", "word//bad.xml", "word/a%2fb.xml", "C:/bad.xml", "word/CON", "word/bad. ", "word/a\x00.xml"} {
		t.Run(name, func(t *testing.T) {
			rejection(t, archiveFixture(t, append(baseParts(docXML("")), fixturePart{name, "x", 0})), UnsafeDOCX)
		})
	}
	rejection(t, archiveFixture(t, append(baseParts(docXML("")), fixturePart{"word/link.xml", "target", os.ModeSymlink | 0777})), UnsafeDOCX)
}

func TestExtractDOCXRejectsDuplicateEntries(t *testing.T) {
	for _, name := range []string{"word/document.xml", "WORD/DOCUMENT.XML"} {
		rejection(t, archiveFixture(t, append(baseParts(docXML("")), fixturePart{name, docXML(""), 0})), UnsafeDOCX)
	}
}

func TestExtractDOCXRejectsDTDAndEntitiesInEveryXMLPart(t *testing.T) {
	for _, part := range []string{"word/document.xml", "[Content_Types].xml", "_rels/.rels", "word/unused.xml"} {
		t.Run(part, func(t *testing.T) {
			parts := baseParts(docXML(""))
			found := false
			for i := range parts {
				if parts[i].name == part {
					parts[i].data = `<!DOCTYPE doc [<!ENTITY e "unsafe">]>` + parts[i].data
					found = true
				}
			}
			if !found {
				parts = append(parts, fixturePart{part, `<!DOCTYPE doc SYSTEM "file:///secret"><doc/>`, 0})
			}
			rejection(t, archiveFixture(t, parts), UnsafeDOCX)
		})
	}
	rejection(t, archiveFixture(t, baseParts(docXML(`<w:p><w:r><w:t>&unknown;</w:t></w:r></w:p>`))), InvalidXML)
	rejection(t, archiveFixture(t, baseParts(`<?xml-stylesheet href="https://example.invalid/style"?>`+docXML(""))), UnsafeDOCX)
}

func TestExtractDOCXRejectsExternalAndEscapingRelationships(t *testing.T) {
	for _, relation := range []string{
		`<Relationship Id="rId2" Type="hyperlink" Target="https://example.invalid" TargetMode="External"/>`,
		`<Relationship Id="rId2" Type="hyperlink" Target="../../outside.xml"/>`,
		`<Relationship Id="rId2" Type="hyperlink" Target="file:///secret"/>`,
		`<Relationship Id="rId2" Type="hyperlink" Target="missing.xml"/>`,
	} {
		parts := baseParts(docXML(""))
		parts = append(parts, fixturePart{"word/_rels/document.xml.rels", `<Relationships xmlns="` + relationshipsNS + `">` + relation + `</Relationships>`, 0})
		rejection(t, archiveFixture(t, parts), UnsafeDOCX)
	}
}

func TestExtractDOCXRejectsCorruptCRCIncludingUnusedParts(t *testing.T) {
	parts := append(baseParts(docXML("")), fixturePart{"word/media/image.png", "pixels", 0})
	raw := archiveFixture(t, parts)
	rejection(t, mutateCentral(t, raw, "word/media/image.png", 16, 0), CorruptDOCX)
	rejection(t, []byte("not a zip"), InvalidDOCX)
	rejection(t, raw[:len(raw)-12], CorruptDOCX)
}

func TestExtractDOCXRejectsResourceBudgets(t *testing.T) {
	t.Run("raw", func(t *testing.T) { rejection(t, make([]byte, 1<<20+1), LimitExceeded) })
	t.Run("entries", func(t *testing.T) {
		parts := baseParts(docXML(""))
		for i := 0; i < 254; i++ {
			parts = append(parts, fixturePart{"word/media/" + strings.Repeat("a", i+1) + ".png", "x", 0})
		}
		rejection(t, archiveFixture(t, parts), LimitExceeded)
	})
	t.Run("claimed_uncompressed", func(t *testing.T) {
		raw := archiveFixture(t, append(baseParts(docXML("")), fixturePart{"word/media/a.png", "x", 0}))
		rejection(t, mutateCentral(t, raw, "word/media/a.png", 24, 8<<20+1), LimitExceeded)
	})
	t.Run("xml", func(t *testing.T) {
		parts := append(baseParts(docXML("")), fixturePart{"word/unused.xml", "<doc>" + strings.Repeat("x", 2<<20) + "</doc>", 0})
		rejection(t, archiveFixture(t, parts), LimitExceeded)
	})
	t.Run("text", func(t *testing.T) {
		doc := docXML(`<w:p><w:r><w:t>` + strings.Repeat("x", 512<<10+1) + `</w:t></w:r></w:p>`)
		rejection(t, archiveFixture(t, baseParts(doc)), LimitExceeded)
	})
	t.Run("depth", func(t *testing.T) {
		doc := docXML(strings.Repeat("<w:customXml>", 64) + strings.Repeat("</w:customXml>", 64))
		rejection(t, archiveFixture(t, baseParts(doc)), LimitExceeded)
	})
}

func TestExtractDOCXTextBudgetCountsUTF8Bytes(t *testing.T) {
	doc := docXML(`<w:p><w:r><w:t>` + strings.Repeat("猫", (512<<10)/3+1) + `</w:t></w:r></w:p>`)
	rejection(t, archiveFixture(t, baseParts(doc)), LimitExceeded)
}

func TestExtractDOCXAcceptsInertMediaAndEmptyDocument(t *testing.T) {
	parts := append(baseParts(docXML("")), fixturePart{"word/media/image.png", "inert bytes are never interpreted", 0})
	result, err := ExtractDOCX(archiveFixture(t, parts))
	if err != nil || result.Text != "" || result.TextBytes != 0 || result.TextSHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("got %#v, %v", result, err)
	}
}

func TestExtractDOCXRejectsSymlinkDirectoryAndWindowsInvalidNames(t *testing.T) {
	t.Run("symlink_directory", func(t *testing.T) {
		rejection(t, archiveFixture(t, append(baseParts(docXML("")), fixturePart{"word/link/", "", os.ModeSymlink | 0777})), UnsafeDOCX)
	})
	for _, name := range []string{"word/a<b.xml", `word/a"b.xml`, "word/a|b.xml", "word/a*b.xml"} {
		t.Run(name, func(t *testing.T) {
			rejection(t, archiveFixture(t, append(baseParts(docXML("")), fixturePart{name, "<doc/>", 0})), UnsafeDOCX)
		})
	}
}

func TestExtractDOCXRejectsActiveContentAtArchiveRoot(t *testing.T) {
	for _, name := range []string{"activeX/item.xml", "embeddings/item.xml"} {
		rejection(t, archiveFixture(t, append(baseParts(docXML("")), fixturePart{name, "<doc/>", 0})), UnsafeDOCX)
	}
}

func TestExtractDOCXAcceptsExactTextQuota(t *testing.T) {
	want := strings.Repeat("x", 512<<10)
	got, err := ExtractDOCX(archiveFixture(t, baseParts(docXML(`<w:p><w:r><w:t>`+want+`</w:t></w:r></w:p>`))))
	if err != nil || got.Text != want || got.TextBytes != len(want) {
		t.Fatalf("exact text quota must be accepted: bytes=%d error=%v", got.TextBytes, err)
	}
}

func TestExtractDOCXPreservesExplicitFinalBreak(t *testing.T) {
	got, err := ExtractDOCX(archiveFixture(t, baseParts(docXML(`<w:p><w:r><w:t>last</w:t><w:br/></w:r></w:p>`))))
	if err != nil || got.Text != "last\n" {
		t.Fatalf("explicit final break must survive paragraph normalization: %#v, %v", got, err)
	}
}

func TestExtractDOCXRejectsMalformedAndAmbiguousXML(t *testing.T) {
	for _, doc := range []string{
		docXML("<w:p>"),
		docXML("") + docXML(""),
		`<w:document xmlns:w="` + wordNS + `" duplicate="one" duplicate="two"><w:body/></w:document>`,
	} {
		rejection(t, archiveFixture(t, baseParts(doc)), InvalidXML)
	}
}

func TestExtractDOCXRejectsWrongMetadataNamespaces(t *testing.T) {
	for _, index := range []int{0, 1} {
		parts := baseParts(docXML(""))
		parts[index].data = strings.ReplaceAll(strings.ReplaceAll(parts[index].data, contentTypesNS, "urn:fake"), relationshipsNS, "urn:fake")
		rejection(t, archiveFixture(t, parts), InvalidDOCX)
	}
}

func TestExtractDOCXAllowsSafeRelativeInternalRelationship(t *testing.T) {
	parts := baseParts(docXML(`<w:p><w:r><w:t>only main text</w:t></w:r></w:p>`))
	parts = append(parts, fixturePart{"docProps/core.xml", `<core>metadata text stays out</core>`, 0})
	parts = append(parts, fixturePart{"word/_rels/document.xml.rels", `<Relationships xmlns="` + relationshipsNS + `"><Relationship Id="r1" Type="core-properties" Target="../docProps/core.xml"/></Relationships>`, 0})
	got, err := ExtractDOCX(archiveFixture(t, parts))
	if err != nil || got.Text != "only main text" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestExtractDOCXPreservesWebHyperlinkDisplayWithoutFollowingIt(t *testing.T) {
	parts := baseParts(docXML(`<w:p><w:hyperlink><w:r><w:t>visible link text</w:t></w:r></w:hyperlink></w:p>`))
	parts = append(parts, fixturePart{"word/_rels/document.xml.rels", `<Relationships xmlns="` + relationshipsNS + `"><Relationship Id="r1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.invalid/unreachable" TargetMode="External"/></Relationships>`, 0})
	got, err := ExtractDOCX(archiveFixture(t, parts))
	if err != nil || got.Text != "visible link text" {
		t.Fatalf("inert hyperlink display must be extracted locally: %#v, %v", got, err)
	}
	parts[3].data = strings.ReplaceAll(parts[3].data, "https://example.invalid/unreachable", "file:///outside")
	rejection(t, archiveFixture(t, parts), UnsafeDOCX)
}

func TestExtractDOCXRejectsClaimedSizeMismatch(t *testing.T) {
	raw := archiveFixture(t, baseParts(docXML(`<w:p><w:r><w:t>text</w:t></w:r></w:p>`)))
	rejection(t, mutateCentral(t, raw, "word/document.xml", 24, 1), CorruptDOCX)
}

func TestExtractDOCXReadOnlyReference(t *testing.T) {
	reference := os.Getenv("NEKO_DOCX_READONLY_REFERENCE")
	if reference == "" {
		t.Skip("read-only source smoke test requires explicit local reference path")
	}
	raw, err := os.ReadFile(reference)
	if err != nil {
		t.Fatal("read-only source unavailable")
	}
	result, err := ExtractDOCX(raw)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	t.Logf("source bytes=%d sha256=%s extracted UTF-8 bytes=%d text sha256=%s", len(raw), hex.EncodeToString(sum[:]), result.TextBytes, result.TextSHA256)
}

func FuzzExtractDOCX(f *testing.F) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, part := range baseParts(docXML(`<w:p><w:r><w:t>猫</w:t></w:r></w:p>`)) {
		file, err := w.Create(part.name)
		if err != nil {
			f.Fatal(err)
		}
		if _, err := file.Write([]byte(part.data)); err != nil {
			f.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		f.Fatal(err)
	}
	f.Add(b.Bytes())
	f.Add([]byte("not a document"))
	f.Add([]byte("PK\x03\x04"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		result, err := ExtractDOCX(raw)
		if err != nil {
			var rejected *Error
			if !errors.As(err, &rejected) || result != (Result{}) {
				t.Fatal("rejection must be typed and must not expose partial text")
			}
			return
		}
		if result.TextBytes != len(result.Text) || result.TextBytes > MaxTextBytes {
			t.Fatal("successful extraction exceeded or miscounted its UTF-8 quota")
		}
	})
}
