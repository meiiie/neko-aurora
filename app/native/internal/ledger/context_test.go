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

func TestContextPinFrozenAtIntakeAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s, prior := running(t, dir)
	queued, _, err := s.Accept(input("context-queued-nonce", "background"))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(queued.Context)
	if len(queued.Context.Sources) != 0 {
		t.Fatal("unfinished work entered context")
	}
	if err = s.AppendText(prior.RequestID, prior.Epoch, "late-fixture-canary-response"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteTool(prior.RequestID, prior.Epoch, invocation("retained fixture artifact bytes")); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(prior.RequestID, prior.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	newer, _, err := s.Accept(input("context-newer-nonce", "foreground"))
	if err != nil || len(newer.Context.Sources) != 1 || newer.Context.Sources[0].RequestID != prior.RequestID || newer.Context.Sources[0].Reply != "late-fixture-canary-response" {
		t.Fatal("completed source was not pinned", err)
	}
	if len(newer.Context.Sources[0].Artifacts) != 1 {
		t.Fatal("artifact provenance absent")
	}
	artifactID := newer.Context.Sources[0].Artifacts[0].ID
	newer.Context.Sources[0].Reply = "caller-mutated-reply"
	newer.Context.Sources[0].Artifacts[0].ID = "caller-mutated-artifact"
	stored, _ := s.Get(newer.RequestID)
	if stored.Context.Sources[0].Reply == "caller-mutated-reply" || stored.Context.Sources[0].Artifacts[0].ID != artifactID {
		t.Fatal("caller changed durable pin through alias")
	}
	davinInput := input("context-davin-nonce", "foreground")
	davinInput.BotID = "bot-davin"
	davin, _, err := s.Accept(davinInput)
	if err != nil || len(davin.Context.Sources) != 0 || strings.Contains(davin.RuntimePrompt(), "late-fixture-canary-response") {
		t.Fatal("context crossed bot scope", err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, _ := s.Get(queued.RequestID)
	after, _ := json.Marshal(restored.Context)
	if !bytes.Equal(before, after) || strings.Contains(restored.RuntimePrompt(), "late-fixture-canary-response") {
		t.Fatal("queued pin changed after completion/restart")
	}
	_, content, err := s.ReadArtifact(artifactID)
	if err != nil || string(content) != "retained fixture artifact bytes" {
		t.Fatal("pinned artifact bytes lost", err)
	}
	duplicate, again, err := s.Accept(input("context-queued-nonce", "background"))
	if err != nil || !again {
		t.Fatal(err)
	}
	duplicateBytes, _ := json.Marshal(duplicate.Context)
	if !bytes.Equal(before, duplicateBytes) {
		t.Fatal("nonce retry rebuilt the pin")
	}
}

func TestContextPinForgeryWithRecomputedJournalDigestRejected(t *testing.T) {
	dir := t.TempDir()
	s, prior := running(t, dir)
	if err := s.AppendText(prior.RequestID, prior.Epoch, "original fixture context"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(prior.RequestID, prior.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Accept(input("context-forgery-nonce", "foreground")); err != nil {
		t.Fatal(err)
	}
	s.Close()
	path := filepath.Join(dir, "transactions.jsonl")
	raw, _ := os.ReadFile(path)
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	var last record
	if err := json.Unmarshal(lines[len(lines)-1], &last); err != nil {
		t.Fatal(err)
	}
	last.Event.Work.Context.Sources[0].Reply = "forged fixture context"
	last.Digest, _ = digestRecord(last)
	lines[len(lines)-1], _ = json.Marshal(last)
	if err := os.WriteFile(path, append(bytes.Join(lines, []byte{'\n'}), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if restored, err := Open(dir); !errors.Is(err, ErrCorrupt) {
		if restored != nil {
			restored.Close()
		}
		t.Fatal("forged pin accepted", err)
	}
}

func TestLegacyJournalDoesNotImportUnpinnedHistory(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	w, _, err := s.Accept(input("legacy-pin-nonce", "background"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	path := filepath.Join(dir, "transactions.jsonl")
	raw, _ := os.ReadFile(path)
	var legacy record
	json.Unmarshal(raw, &legacy)
	legacy.Event.Work.Context = nil
	legacy.Digest, _ = digestRecord(legacy)
	raw, _ = json.Marshal(legacy)
	os.WriteFile(path, append(raw, '\n'), 0600)
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, _ := s.Get(w.RequestID)
	if restored.Context != nil || !strings.Contains(restored.RuntimePrompt(), "No pinned history is available") {
		t.Fatal("legacy request gained ambient context")
	}
}

func TestEncodedPromptBoundRejectsBeforeCommit(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	in := input("bounded-pin-nonce", "foreground")
	in.Text = strings.Repeat("\x01", 32768)
	if _, _, err = s.Accept(in); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized transport accepted", err)
	}
	if s.Snapshot().Sequence != 0 {
		t.Fatal("unexecutable prompt committed")
	}
}
