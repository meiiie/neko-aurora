package ledger

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func docxUploadFixture(t *testing.T, text string) []byte {
	t.Helper()
	var escaped, raw bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(text)); err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + escaped.String() + `</w:t></w:r></w:p></w:body></w:document>`,
	}
	w := zip.NewWriter(&raw)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
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

func TestDOCXUploadKeepsOriginalBytesAndExtractedTextSeparately(t *testing.T) {
	s, c, p := productFixture(t)
	text := "Báo giá tiếng Việt <script>không chạy</script>: 120 dollars"
	raw := docxUploadFixture(t, text)
	f, dup, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID, ClientNonce: "docx-original-upload", Filename: "vendor-quote.docx", Content: raw})
	rawSHA := sha256.Sum256(raw)
	textSHA := sha256.Sum256([]byte(text))
	if err != nil || dup || f.Status != "ready_text" || f.ExtractionVersion != 1 || f.SHA256 != hex.EncodeToString(rawSHA[:]) || f.TextSHA256 != hex.EncodeToString(textSHA[:]) || f.TextBytes != len(text) || f.Excerpt != "" {
		t.Fatal("raw/extracted provenance", err, f.Status)
	}
	preview, err := s.PreviewProjectFile(p.ID, c.ID, f.ID, 1, 0)
	if err != nil || preview.Text != text || preview.Truncated || preview.TextSHA256 != f.TextSHA256 {
		t.Fatal("safe text preview", err)
	}
	_, download, err := s.ReadProjectFile(p.ID, c.ID, f.ID)
	if err != nil || !bytes.Equal(raw, download) {
		t.Fatal("original download changed", err)
	}
	w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "docx-intake-request", Text: "Compare vendor quote", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: 1, FileIDs: []string{f.ID}})
	if err != nil || len(w.Resources.Files) != 1 || w.Resources.Files[0].Excerpt != text {
		t.Fatal("DOCX source not selected", err)
	}
	manifest, err := s.buildRuntimeContext(w)
	if err != nil {
		t.Fatal("extracted text manifest", err)
	}
	found := false
	for _, source := range manifest.Manifest.Sources {
		if source.ID == f.ID && source.SHA256 == f.TextSHA256 && source.Bytes == f.TextBytes {
			found = true
		}
	}
	if !found {
		t.Fatal("manifest uses raw hash for extracted text")
	}
	w = startPermissionWork(t, s, w)
	var frame map[string]any
	json.Unmarshal(invocation("fixture-report"), &frame)
	frame["arguments"] = map[string]any{"name": "project.read_file", "arguments": map[string]any{"fileId": f.ID, "offset": 0, "bytes": 4000}}
	call, _ := json.Marshal(frame)
	result, err := s.ExecuteTool(w.RequestID, w.Epoch, call)
	if err != nil || bytes.Contains(result, []byte("isError")) || !bytes.Contains(result, []byte("120 dollars")) {
		t.Fatal("broker returned ZIP instead of text", err)
	}
	if _, err = s.PreviewProjectFile(p.ID, "conversation-bot-davin", f.ID, 1, 0); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-channel preview", err)
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, ExpectedRevision: 0, Granted: false, ClientNonce: "docx-preview-revoke"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PreviewProjectFile(p.ID, c.ID, f.ID, 1, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked preview", err)
	}
}

func TestDOCXFullTextScreeningQuarantineAndMissingBytesAreTaskLocal(t *testing.T) {
	s, c, p := productFixture(t)
	baseline := s.Snapshot().Sequence
	secretText := strings.Repeat("ordinary text ", 1500) + "api_key=\"" + strings.Repeat("x", 32) + "\""
	if _, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID, ClientNonce: "docx-secret-upload", Filename: "screened.docx", Content: docxUploadFixture(t, secretText)}); !errors.Is(err, ErrInvalid) || s.Snapshot().Sequence != baseline {
		t.Fatal("late extracted secret persisted", err)
	}
	q, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID, ClientNonce: "docx-invalid-upload", Filename: "invalid.docx", Content: []byte("not OOXML")})
	if err != nil || q.Status != "quarantined_unsupported" {
		t.Fatal("invalid DOCX ready", err)
	}
	if _, err = s.PreviewProjectFile(p.ID, c.ID, q.ID, 1, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("quarantine preview", err)
	}
	text := strings.Repeat("a", 15999) + "ế" + strings.Repeat("b", 9000)
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID, ClientNonce: "docx-large-upload", Filename: "large.docx", Content: docxUploadFixture(t, text)})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewProjectFile(p.ID, c.ID, f.ID, 1, 0)
	if err != nil || len(preview.Text) != 15999 || !preview.Truncated || preview.TextBytes != len(text) {
		t.Fatal("UTF8 excerpt", err)
	}
	dir := s.dir
	if err = os.Remove(filepath.Join(dir, "uploads", f.ID)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	for round := 0; round < 3; round++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal("missing bytes corrupted journal", err)
		}
		if _, err = s.PreviewProjectFile(p.ID, c.ID, f.ID, 1, 0); !errors.Is(err, ErrUnavailable) {
			t.Fatal("missing bytes preview", err)
		}
		if _, _, err = s.Accept(input("docx-unrelated-intake", "foreground")); err != nil {
			t.Fatal("unrelated intake blocked", err)
		}
		s.Close()
	}
}
