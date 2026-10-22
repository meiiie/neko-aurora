//go:build windows

package statebackup

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"nekoaurora.local/native/internal/ledger"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (string, ledger.Work) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	s, e := ledger.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	w, _, e := s.Accept(ledger.Input{BotID: "bot-meiiie", Text: "original context fixture", ClientNonce: "backup-exact-original-nonce", Lane: "background"})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.CreateConversation(ledger.ConversationCreateInput{BotID: "bot-meiiie", Title: "Kept private lane", ClientNonce: "backup-conversation-create"}); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(filepath.Join(dir, "artifacts"), 0700)
	if e = os.WriteFile(filepath.Join(dir, "artifacts", fmt.Sprintf("%x", sha256.Sum256([]byte("kept physical file")))), []byte("kept physical file"), 0600); e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(filepath.Join(dir, "connections", "secrets"), 0700)
	if e = os.WriteFile(filepath.Join(dir, "connections", "secrets", "fixture.dpapi"), []byte("synthetic-secret-plaintext-canary"), 0600); e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(filepath.Join(dir, "executors"), 0700)
	os.WriteFile(filepath.Join(dir, "executors", "executor-v1.lock"), []byte("persistent-executor-identity-marker"), 0600)
	return dir, w
}

func TestBackupEncryptedRestorePreservesJournalContextNonceAndFiles(t *testing.T) {
	dir, w := fixture(t)
	before, e := os.ReadFile(filepath.Join(dir, "transactions.jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	m, e := Create(dir, backup, "working-tree")
	if e != nil {
		t.Fatal(e)
	}
	if m.Version != 1 || m.Protection != "dpapi-current-user-v1" || !m.ExecutionDisabled || len(m.Files) < 4 {
		t.Fatal("wrong manifest")
	}
	filepath.WalkDir(backup, func(path string, entry os.DirEntry, e error) error {
		if e != nil {
			t.Fatal(e)
		}
		if !entry.IsDir() {
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			for _, canary := range [][]byte{[]byte("original context fixture"), []byte("synthetic-secret-plaintext-canary"), []byte("kept physical file")} {
				if bytes.Contains(raw, canary) {
					t.Fatal("plaintext escaped to backup", entry.Name())
				}
			}
		}
		return nil
	})
	if _, e = Verify(backup); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if _, e = RestoreDryRun(backup, target); e != nil {
		t.Fatal(e)
	}
	r, e := ledger.Open(target)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	got, e := r.Get(w.RequestID)
	if e != nil || !reflect.DeepEqual(w, got) {
		t.Fatal("original context/work changed", e)
	}
	dup, duplicate, e := r.Accept(ledger.Input{BotID: w.BotID, Text: w.Text, ClientNonce: w.ClientNonce, Lane: w.Lane})
	if e != nil || !duplicate || dup.RequestID != w.RequestID {
		t.Fatal("nonce lost", e)
	}
	if !r.ReadOnly() || r.Snapshot().Sequence != m.Sequence {
		t.Fatal("restore mode/sequence")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "transactions.jsonl"))
	restored, _ := os.ReadFile(filepath.Join(target, "transactions.jsonl"))
	if !bytes.Equal(before, after) || !bytes.Equal(before, restored) {
		t.Fatal("journal bytes changed")
	}
	for _, f := range m.Files {
		raw, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Path)))
		again, _ := os.ReadFile(filepath.Join(target, filepath.FromSlash(f.Path)))
		info, _ := os.Stat(filepath.Join(target, filepath.FromSlash(f.Path)))
		if !bytes.Equal(raw, again) || info.ModTime().UTC().Format(time.RFC3339Nano) != f.ModifiedAt {
			t.Fatal("file bytes/mtime changed", f.Path)
		}
	}
	if e = r.RecoverInterrupted(); e == nil {
		t.Fatal("restored job could recover")
	}
}

func TestBackupRefusesActiveWriterAndExistingNestedDestinations(t *testing.T) {
	dir, _ := fixture(t)
	s, e := ledger.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Create(dir, filepath.Join(t.TempDir(), "backup"), "working-tree"); e == nil {
		t.Fatal("active writer backed up")
	}
	s.Close()
	for _, destination := range []string{dir, filepath.Join(dir, "inside")} {
		if _, e = Create(dir, destination, "working-tree"); e == nil {
			t.Fatal("overlapping backup destination")
		}
	}
	existing := t.TempDir()
	os.WriteFile(filepath.Join(existing, "keep"), []byte("old data"), 0600)
	if _, e = Create(dir, existing, "working-tree"); e == nil {
		t.Fatal("existing backup overwritten")
	}
}

func TestBackupProtectsPrivateManifestPaths(t *testing.T) {
	dir, _ := fixture(t)
	const name = "private-filename-fixture-canary.txt"
	if e := os.WriteFile(filepath.Join(dir, name), []byte("private source fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if _, e := Create(dir, backup, "working-tree"); e != nil {
		t.Fatal(e)
	}
	e := filepath.WalkDir(backup, func(path string, entry os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !entry.IsDir() {
			raw, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if bytes.Contains(raw, []byte(name)) {
				t.Fatal("private filename exposed by manifest")
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}

func TestBackupRefusesMissingPublishedArtifact(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s, e := ledger.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	w, _, e := s.Accept(ledger.Input{BotID: "bot-meiiie", Text: "physical-file fixture", ClientNonce: "backup-file-receipt-nonce", Lane: "foreground"})
	if e != nil {
		t.Fatal(e)
	}
	w, e = s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	binding := ledger.CoreBinding{SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1}`), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"}
	if e = s.Admit(w.RequestID, w.Epoch, binding); e != nil {
		t.Fatal(e)
	}
	frame, _ := json.Marshal(map[string]any{"name": "execute_tool", "arguments": map[string]any{"name": "artifact.write_text", "arguments": map[string]string{"filename": "backup-report.txt", "text": "real synthetic bytes"}}, "_meta": map[string]any{"neko.operation": map[string]any{"version": 1, "operationId": strings.Repeat("d", 32), "sessionId": strings.Repeat("a", 32), "taskId": strings.Repeat("b", 32), "root": "/fixture", "activationEpoch": 1, "activationId": strings.Repeat("c", 32)}}})
	if _, e = s.ExecuteTool(w.RequestID, w.Epoch, frame); e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(w.RequestID, w.Epoch, "completed", ""); e != nil {
		t.Fatal(e)
	}
	w, e = s.Get(w.RequestID)
	if e != nil || len(w.Artifacts) != 1 {
		t.Fatal("missing genuine ledger artifact", e)
	}
	s.Close()
	if e = os.Remove(filepath.Join(dir, "artifacts", w.Artifacts[0].ID)); e != nil {
		t.Fatal(e)
	}
	if _, e = Create(dir, filepath.Join(t.TempDir(), "backup"), "working-tree"); e == nil {
		t.Fatal("backup blessed a lost committed file")
	}
}

func TestBackupStrictLegacyAndUnfinishedSource(t *testing.T) {
	dir, _ := fixture(t)
	if e := os.Remove(filepath.Join(dir, "local-sync.identity")); e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if _, e := Create(dir, backup, "working-tree"); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if _, e := RestoreDryRun(backup, target); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(target, "local-sync.identity")); !os.IsNotExist(e) {
		t.Fatal("restore invented sync identity")
	}
	journal := filepath.Join(dir, "transactions.jsonl")
	raw, e := os.ReadFile(journal)
	if e != nil {
		t.Fatal(e)
	}
	raw = append(raw, []byte(`{"partial":`)...)
	os.WriteFile(journal, raw, 0600)
	destination := filepath.Join(t.TempDir(), "newbackup")
	if _, e = Create(dir, destination, "working-tree"); e == nil {
		t.Fatal("torn input accepted")
	}
	after, _ := os.ReadFile(journal)
	if !bytes.Equal(raw, after) {
		t.Fatal("backup repaired original torn input")
	}
	if _, e = os.Stat(destination); !os.IsNotExist(e) {
		t.Fatal("invalid source created backup")
	}
}

func TestBackupInnerManifestRejectsUnsafeNamesAndBounds(t *testing.T) {
	for _, bad := range []string{"../escape", "C:/outside", "alt:stream", "NUL", "a\\b", "a/../b", "Writer.Lock", "restore-readonly.json/x", "a.", "a "} {
		t.Run(bad, func(t *testing.T) {
			dir, _ := fixture(t)
			backup := filepath.Join(t.TempDir(), "backup")
			m, e := Create(dir, backup, "working-tree")
			if e != nil {
				t.Fatal(e)
			}
			m.Files[0].Path = bad
			raw, _ := json.Marshal(m)
			replaceManifestPayload(t, backup, raw)
			target := filepath.Join(t.TempDir(), "newstate")
			if _, e = RestoreDryRun(backup, target); e == nil {
				t.Fatal("unsafe path accepted")
			}
			if _, e = os.Stat(target); !os.IsNotExist(e) {
				t.Fatal("unsafe manifest created target")
			}
		})
	}
	dir, _ := fixture(t)
	backup := filepath.Join(t.TempDir(), "backup")
	m, e := Create(dir, backup, "working-tree")
	if e != nil {
		t.Fatal(e)
	}
	m.Files[0].Bytes = maxFile + 1
	raw, _ := json.Marshal(m)
	replaceManifestPayload(t, backup, raw)
	if _, e = Verify(backup); e == nil {
		t.Fatal("oversized file descriptor accepted")
	}
	if safePath("/absolute") || safePath("x/y:") || safePath("..") {
		t.Fatal("unsafe lexical path")
	}
}

func TestBackupTamperAndTraversalRefusedBeforeRestoreTargetCreation(t *testing.T) {
	for _, kind := range []string{"cipher", "path", "unknown", "extra", "digest"} {
		t.Run(kind, func(t *testing.T) {
			dir, _ := fixture(t)
			backup := filepath.Join(t.TempDir(), "backup")
			m, e := Create(dir, backup, "working-tree")
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "cipher":
				p := filepath.Join(backup, filepath.FromSlash(m.Files[0].Blob))
				b, _ := os.ReadFile(p)
				b[len(b)/2] ^= 1
				os.WriteFile(p, b, 0600)
			case "path":
				m.Files[0].Path = "../escape"
				b, _ := json.Marshal(m)
				replaceManifestPayload(t, backup, b)
			case "unknown":
				b, _ := os.ReadFile(filepath.Join(backup, "manifest.json"))
				b = append(bytes.TrimSuffix(b, []byte("}")), []byte(`,"enableExecution":true}`)...)
				os.WriteFile(filepath.Join(backup, "manifest.json"), b, 0600)
				os.WriteFile(filepath.Join(backup, "manifest.sha256"), []byte(fmt.Sprintf("%x\n", sha256.Sum256(b))), 0600)
			case "extra":
				os.WriteFile(filepath.Join(backup, "foreign.json"), []byte("{}"), 0600)
			case "digest":
				os.WriteFile(filepath.Join(backup, "manifest.sha256"), bytes.Repeat([]byte("0"), 64), 0600)
			}
			target := filepath.Join(t.TempDir(), "newstate")
			if _, e = RestoreDryRun(backup, target); e == nil {
				t.Fatal("corruption accepted")
			}
			if _, e = os.Stat(target); !os.IsNotExist(e) {
				t.Fatal("bad backup created restore target")
			}
		})
	}
}

func replaceManifestPayload(t *testing.T, backup string, raw []byte) {
	t.Helper()
	original, e := os.ReadFile(filepath.Join(backup, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var header envelope
	if e = json.Unmarshal(original, &header); e != nil {
		t.Fatal(e)
	}
	cipher, e := protect(raw, manifestEntropy(header.SnapshotID))
	if e != nil {
		t.Fatal(e)
	}
	header.CipherBytes = int64(len(cipher))
	header.CipherSHA256 = sum(cipher)
	os.WriteFile(filepath.Join(backup, "manifest.dpapi"), cipher, 0600)
	original, _ = json.Marshal(header)
	os.WriteFile(filepath.Join(backup, "manifest.json"), original, 0600)
	os.WriteFile(filepath.Join(backup, "manifest.sha256"), []byte(sum(original)+"\n"), 0600)
}
