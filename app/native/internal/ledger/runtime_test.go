package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func running(t *testing.T, dir string) (*Store, Work) {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.Accept(input("runtime-fixture", "background"))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	task := json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`)
	if err = s.Admit(w.RequestID, w.Epoch, CoreBinding{SessionID: strings.Repeat("a", 32), Task: task, Execution: json.RawMessage(`{"version":1}`), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"}); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	return s, w
}
func invocation(text string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"name": "execute_tool", "arguments": map[string]any{"name": "artifact.write_text", "arguments": map[string]string{"filename": "report.md", "text": text}}, "_meta": map[string]any{"neko.operation": map[string]any{"version": 1, "operationId": strings.Repeat("d", 32), "sessionId": strings.Repeat("a", 32), "taskId": strings.Repeat("b", 32), "root": "/fixture", "activationEpoch": 1, "activationId": strings.Repeat("c", 32)}}})
	return b
}
func TestArtifactReceiptDuplicateConflictAndRestart(t *testing.T) {
	dir := t.TempDir()
	s, w := running(t, dir)
	result, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("actual UTF-8 report"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("actual UTF-8 report"))
	if err != nil || string(again) != string(result) {
		t.Fatal("duplicate did not return durable receipt")
	}
	if _, err = s.ExecuteTool(w.RequestID, w.Epoch, invocation("changed report")); !errors.Is(err, ErrConflict) {
		t.Fatal("payload conflict accepted")
	}
	w, _ = s.Get(w.RequestID)
	if len(w.Artifacts) != 1 || len(w.Operations) != 1 {
		t.Fatal("duplicate effect")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, b, err := s.ReadArtifact(w.Artifacts[0].ID)
	if err != nil || string(b) != "actual UTF-8 report" || a.SHA256 != w.Artifacts[0].SHA256 {
		t.Fatal("artifact lost or changed")
	}
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	recovered, _ := s.Get(w.RequestID)
	if recovered.Status != "recovery_required" || recovered.Core.SessionID != w.Core.SessionID {
		t.Fatal("incomplete task incorrectly completed")
	}
}
func TestEffectBeforeAcknowledgementReconcilesWithoutWritingAgain(t *testing.T) {
	dir := t.TempDir()
	s, w := running(t, dir)
	if _, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("physical once")); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	s.Close()
	path := filepath.Join(dir, "transactions.jsonl")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if err = os.WriteFile(path, []byte(strings.Join(lines[:len(lines)-1], "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(dir, "artifacts", w.Artifacts[0].ID))
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(filepath.Join(dir, "artifacts", w.Artifacts[0].ID))
	recovered, _ := s.Get(w.RequestID)
	if len(recovered.Artifacts) != 1 || recovered.Operations[0].Status != "completed" || !info.ModTime().Equal(after.ModTime()) {
		t.Fatal("effect repeated or result not reconciled")
	}
}
func TestCancellationFencesLateTextAndTools(t *testing.T) {
	s, w := running(t, t.TempDir())
	defer s.Close()
	if _, err := s.Cancel(w.RequestID); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendText(w.RequestID, w.Epoch, "late"); !errors.Is(err, ErrConflict) {
		t.Fatal("late text admitted")
	}
	if _, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("late report")); !errors.Is(err, ErrConflict) {
		t.Fatal("late effect admitted")
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "cancelling" || len(current.Artifacts) > 0 {
		t.Fatal("cancellation fabricated rollback or accepted effects")
	}
}
func TestForgedCoreOperationAndPathRejected(t *testing.T) {
	s, w := running(t, t.TempDir())
	defer s.Close()
	for _, change := range []string{"epoch", "root", "path"} {
		var v map[string]any
		json.Unmarshal(invocation("safe"), &v)
		op := v["_meta"].(map[string]any)["neko.operation"].(map[string]any)
		switch change {
		case "epoch":
			op["activationEpoch"] = 2
		case "root":
			op["root"] = "/other"
		case "path":
			v["arguments"].(map[string]any)["arguments"].(map[string]any)["filename"] = "../escape.md"
		}
		b, _ := json.Marshal(v)
		result, err := s.ExecuteTool(w.RequestID, w.Epoch, b)
		if err == nil && !strings.Contains(string(result), "isError") {
			t.Fatal("forged operation/path accepted")
		}
	}
	current, _ := s.Get(w.RequestID)
	if len(current.Artifacts) > 0 || len(current.Operations) > 0 {
		t.Fatal("rejected request started an effect")
	}
}
