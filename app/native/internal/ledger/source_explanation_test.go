package ledger

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSourceExplanationHasLiteralReasonsAndNoPinnedRawText(t *testing.T) {
	s, c, p := productFixture(t)
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, Filename: "source.md", Content: []byte("SOURCE_EXCERPT_CANARY"), ClientNonce: "explain-file-upload"})
	if err != nil {
		t.Fatal(err)
	}
	in := Input{"bot-meiiie", "explain-pinned-intake", "SOURCE_EXCERPT_CANARY", "background"}
	scope := SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}}
	w, _, err := s.AcceptProduct(in, scope)
	if err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Sequence
	out, err := s.ExplainSources(w.RequestID, c.ID, p.ID)
	if err != nil || !out.Usable || s.Snapshot().Sequence != before {
		t.Fatal("inspection mutated", err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "SOURCE_EXCERPT_CANARY") || strings.Contains(string(raw), "contextPin") {
		t.Fatal("raw excerpt exposed")
	}
	found := false
	for _, src := range out.Sources {
		if src.ID == f.ID {
			found = src.Reason == "explicit_file_selection_bounded" && src.Usable && src.Name == f.Filename
		}
	}
	if !found {
		t.Fatal("missing exact selection reason")
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "explain-source-revoke"}); err != nil {
		t.Fatal(err)
	}
	out, err = s.ExplainSources(w.RequestID, c.ID, p.ID)
	if err != nil || out.Usable {
		t.Fatal("stale availability", err)
	}
	for _, src := range out.Sources {
		if src.ID == f.ID && (src.Usable || src.Name != "" || src.CurrentRevision != 1) {
			t.Fatal("revoked source exposed")
		}
	}
	if _, err = s.ExplainSources(w.RequestID, c.ID, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("missing project scope allowed", err)
	}
}
