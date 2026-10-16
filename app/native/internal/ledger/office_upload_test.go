package ledger

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"sort"
	"strings"
	"testing"
)

func officeUploadFixture(t *testing.T, ext, text string) []byte {
	t.Helper()
	var escaped, raw bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(text)); err != nil {
		t.Fatal(err)
	}
	main, mime := "xl/workbook.xml", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	parts := map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Quotes" sheetId="1" r:id="s1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="s1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>` + escaped.String() + `</t></is></c><c r="B1"><v>120</v></c></row></sheetData></worksheet>`,
	}
	if ext == ".pptx" {
		main, mime = "ppt/presentation.xml", "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
		parts = map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="s1"/></p:sldIdLst></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="s1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/></Relationships>`,
			"ppt/slides/slide1.xml":           `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>` + escaped.String() + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`,
		}
	}
	partName, partType := "xl/worksheets/sheet1.xml", "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	if ext == ".pptx" {
		partName, partType = "ppt/slides/slide1.xml", "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"
	}
	parts["[Content_Types].xml"] = `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/` + main + `" ContentType="` + mime + `"/><Override PartName="/` + partName + `" ContentType="` + partType + `"/></Types>`
	parts["_rels/.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="main" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="` + main + `"/></Relationships>`
	w := zip.NewWriter(&raw)
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = p.Write([]byte(parts[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

func TestOfficeUploadOriginalTextAuthorityAndReplay(t *testing.T) {
	for _, ext := range []string{".xlsx", ".pptx"} {
		t.Run(ext, func(t *testing.T) {
			s, c, p := productFixture(t)
			defer func() { s.Close() }()
			raw := officeUploadFixture(t, ext, "Vietnamese quote 120 dollars")
			in := UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "office-original-upload", Filename: "quote" + ext, Content: raw}
			f, duplicate, err := s.UploadFile(in)
			h := sha256.Sum256(raw)
			if err != nil || duplicate || f.Status != "ready_text" || f.ExtractionVersion != 1 || f.SHA256 != hex.EncodeToString(h[:]) || f.Excerpt != "" {
				t.Fatal("office intake provenance", err, f.Status)
			}
			preview, err := s.PreviewProjectFile(p.ID, c.ID, f.ID, p.Revision, 0)
			if err != nil || !strings.Contains(preview.Text, "Vietnamese quote 120 dollars") || !strings.Contains(preview.Text, ".xml") || preview.TextSHA256 != f.TextSHA256 {
				t.Fatal("office extracted preview", err)
			}
			_, downloaded, err := s.ReadProjectFile(p.ID, c.ID, f.ID)
			if err != nil || !bytes.Equal(raw, downloaded) {
				t.Fatal("original bytes", err)
			}
			w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "office-source-task", Text: "Compare quote", Lane: "background"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}})
			if err != nil || len(w.Resources.Files) != 1 || !strings.Contains(w.Resources.Files[0].Excerpt, "Vietnamese quote") {
				t.Fatal("source pin", err)
			}
			if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, ExpectedRevision: 0, Granted: false, ClientNonce: "office-revoke-access"}); err != nil {
				t.Fatal(err)
			}
			if _, err = s.PreviewProjectFile(p.ID, c.ID, f.ID, p.Revision, 0); !errors.Is(err, ErrConflict) {
				t.Fatal("revoke preview", err)
			}
			dir := s.dir
			s.Close()
			for round := 0; round < 3; round++ {
				s, err = Open(dir)
				if err != nil {
					t.Fatal("office replay", err)
				}
				again, dup, e := s.UploadFile(in)
				if e != nil || !dup || again.ID != f.ID || again.Granted {
					t.Fatal("original receipt after revoke", e)
				}
				if _, e = s.PreviewProjectFile(p.ID, c.ID, f.ID, p.Revision, 0); !errors.Is(e, ErrConflict) {
					t.Fatal("replay access fence", e)
				}
				if round < 2 {
					s.Close()
				}
			}
		})
	}
}

func TestOfficeUploadQuarantineAndFullExtractedScreening(t *testing.T) {
	s, c, p := productFixture(t)
	defer s.Close()
	for i, ext := range []string{".xlsx", ".pptx"} {
		base := s.Snapshot().Sequence
		text := strings.Repeat("ordinary ", 2000) + "api_" + "key=\"" + strings.Repeat("x", 32) + "\""
		_, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "office-sensitive-" + strings.TrimPrefix(ext, "."), Filename: "screened" + ext, Content: officeUploadFixture(t, ext, text)})
		if !errors.Is(err, ErrInvalid) || s.Snapshot().Sequence != base {
			t.Fatal("extracted sensitive content persisted", err, i)
		}
		q, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "office-quarantine-" + strings.TrimPrefix(ext, "."), Filename: "invalid" + ext, Content: []byte("unsupported data")})
		if err != nil || q.Status != "quarantined_unsupported" {
			t.Fatal("invalid office text became ready", err)
		}
		if _, err = s.PreviewProjectFile(p.ID, c.ID, q.ID, p.Revision, 0); !errors.Is(err, ErrConflict) {
			t.Fatal("quarantine text preview", err)
		}
	}
}
