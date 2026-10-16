package ledger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFileReadTextIdentityMatchesSelectedManifest(t *testing.T) {
	for _, ext := range []string{".txt", ".docx", ".xlsx", ".pptx"} {
		t.Run(ext, func(t *testing.T) {
			s, channel, project := productFixture(t)
			defer s.Close()
			literal := "Báo giá tiếng Việt: 120 dollars"
			raw := []byte(literal)
			if ext == ".docx" {
				raw = docxUploadFixture(t, literal)
			} else if ext == ".xlsx" || ext == ".pptx" {
				raw = officeUploadFixture(t, ext, literal)
			}
			file, _, err := s.UploadFile(UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel.ID, ClientNonce: "tool-identity-upload", Filename: "quote" + ext, Content: raw})
			if err != nil || file.Status != "ready_text" {
				t.Fatal("ready source", err)
			}
			w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "tool-identity-work", Text: "Read quote", Lane: "foreground"}, SubmissionScope{ChannelID: channel.ID, ProjectID: project.ID, ProjectRevision: project.Revision, FileIDs: []string{file.ID}})
			if err != nil {
				t.Fatal(err)
			}
			built, err := s.buildRuntimeContext(w)
			if err != nil {
				t.Fatal(err)
			}
			var selected ManifestSource
			for _, source := range built.Manifest.Sources {
				if source.ID == file.ID {
					selected = source
				}
			}
			if selected.ID == "" {
				t.Fatal("selected source missing")
			}
			w = startPermissionWork(t, s, w)
			var call map[string]any
			if err := json.Unmarshal(invocation("tool-identity-read"), &call); err != nil {
				t.Fatal(err)
			}
			call["arguments"] = map[string]any{"name": "project.read_file", "arguments": map[string]any{"fileId": file.ID, "offset": selected.Span.Offset, "bytes": selected.Span.End - selected.Span.Offset}}
			frame, _ := json.Marshal(call)
			result, err := s.ExecuteTool(w.RequestID, w.Epoch, frame)
			var response map[string]json.RawMessage
			if err != nil || json.Unmarshal(result, &response) != nil {
				t.Fatal("read result", err)
			}
			var content []map[string]string
			if json.Unmarshal(response["content"], &content) != nil || len(content) != 1 {
				t.Fatal("text content missing")
			}
			var observed map[string]any
			if json.Unmarshal([]byte(content[0]["text"]), &observed) != nil || len(observed) != 7 {
				t.Fatal("Core exact observation shape")
			}
			text, ok := observed["text"].(string)
			if !ok || observed["fileId"] != selected.ID || observed["sha256"] != selected.SHA256 || text != selected.Span.Text || !strings.Contains(text, literal) {
				t.Error("tool text identity differs from admitted source")
			}
			if file.ExtractionVersion == 1 && (selected.SHA256 != file.TextSHA256 || selected.SHA256 == file.SHA256) {
				t.Error("extracted and original provenance conflated")
			}
			descriptor, download, err := s.ReadProjectFile(project.ID, channel.ID, file.ID)
			if err != nil || !bytes.Equal(download, raw) || descriptor.SHA256 != file.SHA256 {
				t.Fatal("original binary download identity changed", err)
			}
		})
	}
}
