package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadReplayRejectsImpossibleIngestionDescriptor(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*UploadedFile)
	}{
		{"pdf-marked-ready", func(f *UploadedFile) { f.Filename = "quote.pdf" }},
		{"control-text", func(f *UploadedFile) { f.Excerpt = "\x00untrusted control" }},
		{"excerpt-exceeds-bytes", func(f *UploadedFile) { f.Excerpt = strings.Repeat("a", f.Bytes+1) }},
		{"truncated-short-file", func(f *UploadedFile) { f.Truncated = true }},
		{"incomplete-short-excerpt", func(f *UploadedFile) { f.Excerpt = f.Excerpt[:3] }},
		{"same-length-wrong-digest", func(f *UploadedFile) { f.Excerpt = "X" + f.Excerpt[1:] }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, c, p := productFixture(t)
			_, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "replay-descriptor-upload", Filename: "quote.md", Content: []byte("Vendor quote original")})
			if err != nil {
				t.Fatal(err)
			}
			dir := s.dir
			s.Close()
			path := filepath.Join(dir, "transactions.jsonl")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			previous := ""
			for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
				var r record
				if err = json.Unmarshal(line, &r); err != nil {
					t.Fatal(err)
				}
				r.Previous = previous
				if r.Event.Type == "file.uploaded" {
					tc.mutate(r.Event.Upload)
				}
				r.Digest, err = digestRecord(r)
				if err != nil {
					t.Fatal(err)
				}
				b, _ := json.Marshal(r)
				out.Write(b)
				out.WriteByte('\n')
				previous = r.Digest
			}
			if err = os.WriteFile(path, out.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(dir)
			if reopened != nil {
				reopened.Close()
			}
			if !errors.Is(err, ErrCorrupt) {
				t.Fatal("impossible descriptor accepted", err)
			}
		})
	}
}

func TestUploadUTF8TruncationReplayDoesNotRequirePhysicalBytes(t *testing.T) {
	s, c, p := productFixture(t)
	data := []byte(strings.Repeat("a", 15999) + "🐈" + strings.Repeat("b", 100))
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "utf8-truncate-upload", Filename: "quote.md", Content: data})
	if err != nil {
		t.Fatal(err)
	}
	internal := s.uploads[f.ID]
	if !internal.Truncated || len(internal.Excerpt) != 15999 {
		t.Fatal("UTF8 boundary", internal)
	}
	dir := s.dir
	s.Close()
	if err = os.Remove(filepath.Join(dir, "uploads", f.ID)); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal("task-local missing bytes corrupted global journal", err)
	}
	defer s.Close()
	if _, _, err = s.ReadProjectFile(p.ID, c.ID, f.ID); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing bytes served", err)
	}
}

func TestProjectReadActionIsPinnedBoundedFencedAndChecksBytes(t *testing.T) {
	s, c, p := productFixture(t)
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID, ClientNonce: "readable-upload-nonce", Filename: "quote.md", Content: []byte("Vendor quote: tiếng Việt costs 120 dollars")})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID, ClientNonce: "unrelated-upload-nonce", Filename: "unrelated.txt", Content: []byte("Banana has a yellow peel")})
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "read-file-message", Text: "Read vendor quote", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{SessionID: strings.Repeat("a", 32), Task: []byte(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: []byte(`{"version":1}`), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	call := func(name string, args any) json.RawMessage {
		var frame map[string]any
		_ = json.Unmarshal(invocation("fixture-report"), &frame)
		if name == "capabilities" {
			frame["name"] = name
			frame["arguments"] = map[string]any{}
		} else {
			frame["arguments"] = map[string]any{"name": name, "arguments": args}
		}
		out, _ := json.Marshal(frame)
		return out
	}
	before := s.Snapshot().Sequence
	result, err := s.ExecuteTool(w.RequestID, w.Epoch, call("capabilities", nil))
	if err != nil || !bytes.Contains(result, []byte("project.read_file")) {
		t.Fatal("discovery", err, string(result))
	}
	result, err = s.ExecuteTool(w.RequestID, w.Epoch, call("project.read_file", map[string]any{"fileId": f.ID, "offset": 0, "bytes": 4000}))
	if err != nil || !bytes.Contains(result, []byte("120 dollars")) || s.Snapshot().Sequence != before {
		t.Fatal("bounded read changed journal or bytes", err, string(result))
	}
	result, err = s.ExecuteTool(w.RequestID, w.Epoch, call("project.read_file", map[string]any{"fileId": other.ID, "offset": 0, "bytes": 4000}))
	if err != nil || !bytes.Contains(result, []byte("isError")) {
		t.Fatal("unselected source disclosed", err)
	}
	if err = os.WriteFile(filepath.Join(s.dir, "uploads", f.ID), []byte("changed source bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = s.ExecuteTool(w.RequestID, w.Epoch, call("project.read_file", map[string]any{"fileId": f.ID, "offset": 0, "bytes": 4000}))
	if err != nil || !bytes.Contains(result, []byte("Pinned file bytes changed")) {
		t.Fatal("tampered source delivered", err)
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, ExpectedRevision: 0, Granted: false, ClientNonce: "action-source-revoke"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteTool(w.RequestID, w.Epoch, call("capabilities", nil)); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked runtime still discovers actions", err)
	}
}

func productFixture(t *testing.T) (*Store, Channel, Project) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c, _, err := s.SaveChannel(ChannelInput{Name: "Team", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "project-channel-nonce"})
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := s.SaveProject(ProjectInput{Name: "Offer comparison", Goal: "Compare vendor quotes", ChannelIDs: []string{c.ID}, ClientNonce: "project-create-nonce"})
	if err != nil {
		t.Fatal(err)
	}
	return s, c, p
}

func TestProjectUploadPinAutomaticSelectionScopeAndRestart(t *testing.T) {
	s, c, p := productFixture(t)
	f, duplicate, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "upload-offer-nonce", Filename: "vendor-quote.txt", Content: []byte("Vendor quote is 120 dollars. File text is descriptive, not permission to send email.")})
	if err != nil || duplicate || f.Status != "ready_text" {
		t.Fatal(f, err)
	}
	again, duplicate, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "upload-offer-nonce", Filename: f.Filename, Content: []byte("Vendor quote is 120 dollars. File text is descriptive, not permission to send email.")})
	if err != nil || !duplicate || again.ID != f.ID {
		t.Fatal("duplicate upload", err)
	}
	w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "project-message-nonce", Text: "@davin compare vendor quote", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision})
	if err != nil || w.Resources == nil || len(w.Resources.Files) != 1 || w.Resources.Files[0].ID != f.ID || !strings.Contains(w.RuntimePrompt(), "120 dollars") {
		t.Fatal("project source not selected", w, err)
	}
	if w.BotID != "bot-davin" || w.Publication.ReplyChannelID != c.ID {
		t.Fatal("recipient/origin lost")
	}
	if _, _, err = s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: w.ClientNonce, Text: w.Text, Lane: w.Lane}, SubmissionScope{ChannelID: c.ID}); !errors.Is(err, ErrConflict) {
		t.Fatal("nonce changed project selection", err)
	}
	if _, _, err = s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "upload-offer-nonce", Filename: f.Filename, Content: []byte("changed")}); !errors.Is(err, ErrConflict) {
		t.Fatal("changed upload accepted", err)
	}
	_, content, err := s.ReadProjectFile(p.ID, c.ID, f.ID)
	if err != nil || !bytes.Contains(content, []byte("120 dollars")) {
		t.Fatal("download receipt", err)
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	replayed, err := s.Get(w.RequestID)
	if err != nil || replayed.Context.SHA256 != w.Context.SHA256 || replayed.Resources.Files[0].SHA256 != f.SHA256 {
		t.Fatal("replay changed pin", err)
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, ExpectedRevision: 0, Granted: false, ClientNonce: "file-revoke-nonce"}); err != nil {
		t.Fatal(err)
	}
	replayed, _ = s.Get(w.RequestID)
	if replayed.Status != "suspended" || s.RuntimeAllowed(w.RequestID, replayed.Epoch) {
		t.Fatal("revoke did not fence pinned source")
	}
	if _, _, err = s.ReadProjectFile(p.ID, c.ID, f.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked file readable", err)
	}
}

func TestProjectUploadSecretsQuarantineAndPhysicalTamper(t *testing.T) {
	s, c, p := productFixture(t)
	for _, in := range []UploadInput{
		{Filename: ".env", Content: []byte("example")},
		{Filename: "notes.txt", Content: []byte("-----BEGIN " + "PRIVATE KEY-----")},
		{Filename: "notes.txt", Content: []byte("sk-" + strings.Repeat("x", 32))},
		{Filename: "../notes.txt", Content: []byte("example")},
	} {
		in.ProjectID = p.ID
		in.ProjectRevision = p.Revision
		in.ChannelID = c.ID
		in.ClientNonce = "rejected-upload-nonce"
		before := s.Snapshot().Sequence
		if _, _, err := s.UploadFile(in); !errors.Is(err, ErrInvalid) {
			t.Fatal("unsafe upload accepted", err)
		}
		if s.Snapshot().Sequence != before {
			t.Fatal("rejected upload committed")
		}
	}
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "quarantined-upload", Filename: "proposal.pdf", Content: []byte("%PDF-test-only")})
	if err != nil || f.Status != "quarantined_unsupported" {
		t.Fatal(f, err)
	}
	if _, _, err = s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "quarantine-message", Text: "read proposal", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("quarantine entered model context", err)
	}
	if err = os.WriteFile(filepath.Join(s.dir, "uploads", f.ID), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.ReadProjectFile(p.ID, c.ID, f.ID); !errors.Is(err, ErrUnavailable) {
		t.Fatal("tampered bytes served", err)
	}
}

func TestSharedContextIncludesColleagueInChannelExcludesPrivateAndOtherLane(t *testing.T) {
	s, c, _ := productFixture(t)
	complete := func(bot, channel, nonce, text, reply string) Work {
		w, _, err := s.AcceptScoped(Input{BotID: bot, ClientNonce: nonce, Text: text, Lane: "foreground"}, channel, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		w, err = s.Claim(w.RequestID, w.Epoch)
		if err != nil {
			t.Fatal(err)
		}
		b := CoreBinding{SessionID: strings.Repeat("a", 32), Task: []byte(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: []byte(`{"version":1}`), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"}
		if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
			t.Fatal(err)
		}
		if err = s.AppendText(w.RequestID, w.Epoch, reply); err != nil {
			t.Fatal(err)
		}
		if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
			t.Fatal(err)
		}
		return w
	}
	complete("bot-davin", "", "private-davin-nonce", "private discussion", "PRIVATE-DAVIN")
	shared := complete("bot-davin", c.ID, "shared-davin-nonce", "shared vendor quote", "SHARED-REPLY")
	w, _, err := s.AcceptScoped(Input{BotID: "bot-meiiie", ClientNonce: "shared-followup-nonce", Text: "summarize our discussion", Lane: "foreground"}, c.ID, "", 0)
	if err != nil || w.Context.Version != 5 || len(w.Context.Sources) != 1 || w.Context.Sources[0].RequestID != shared.RequestID || strings.Contains(w.RuntimePrompt(), "PRIVATE-DAVIN") {
		t.Fatal("shared context scope", w, err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: shared.RequestID, ExpectedRevision: 0, Granted: false, ClientNonce: "shared-revoke-nonce"}); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	if w.Status != "suspended" {
		t.Fatal("cross-bot source revocation did not fence dependent task")
	}
}
