package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCancelledEffectFailureReconcilesWithoutBlockingService(t *testing.T) {
	for _, outcome := range []string{"missing", "partial", "exact"} {
		t.Run(outcome, func(t *testing.T) {
			dir := t.TempDir()
			s, w := running(t, dir)
			text := "fixture durable bytes"
			hash := sha256.Sum256([]byte(w.RequestID + ":" + strings.Repeat("a", 32) + ":" + strings.Repeat("d", 32)))
			artifactID := hex.EncodeToString(hash[:])
			path := filepath.Join(dir, "artifacts", artifactID)
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation(text)); !errors.Is(err, ErrUnavailable) {
				t.Fatal("file failure not forced", err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if outcome != "missing" {
				content := []byte(text)
				if outcome == "partial" {
					content = []byte("partial fixture")
				}
				if err := os.WriteFile(path, content, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Cancel(w.RequestID); err != nil {
				t.Fatal(err)
			}
			s.Close()
			for i := 0; i < 3; i++ {
				var err error
				s, err = Open(dir)
				if err != nil {
					t.Fatal(err)
				}
				if err = s.RecoverInterrupted(); err != nil {
					t.Fatal("task-local IO failure blocked service", err)
				}
				restored, _ := s.Get(w.RequestID)
				if restored.Status != "cancelled" || restored.DesiredState != "cancelled" || restored.Epoch != w.Epoch+1 || restored.Reply != w.Reply {
					t.Fatal("reconcile revived cancelled work", restored.Status)
				}
				wanted := "unknown"
				count := 0
				if outcome == "exact" {
					wanted = "completed"
					count = 1
				}
				if restored.Operations[0].Status != wanted || len(restored.Artifacts) != count {
					t.Fatal("wrong receipt outcome", restored.Operations[0].Status)
				}
				if _, _, err = s.Accept(input("service-still-live-fixture", "foreground")); err != nil {
					t.Fatal("unrelated intake blocked", err)
				}
				s.Close()
			}
		})
	}
}
func TestUnresolvedEffectCannotCompleteTask(t *testing.T) {
	dir := t.TempDir()
	s, w := running(t, dir)
	defer s.Close()
	hash := sha256.Sum256([]byte(w.RequestID + ":" + strings.Repeat("a", 32) + ":" + strings.Repeat("d", 32)))
	if err := os.MkdirAll(filepath.Join(dir, "artifacts", hex.EncodeToString(hash[:])), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("fixture incomplete effect")); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if err := s.Finish(w.RequestID, w.Epoch, "completed", ""); !errors.Is(err, ErrConflict) {
		t.Fatal("unresolved effect completed", err)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "running" || current.Operations[0].Status != "started" {
		t.Fatal("rejected completion changed state")
	}
	invalid := copyWork(current)
	invalid.Status = "completed"
	if validReplay(s, Event{Type: "work.completed", Work: invalid, CompletionVersion: 1}) {
		t.Fatal("new completion event accepted unresolved effect")
	}
}

func TestLegacyCompletedEffectFailureRemainsReadable(t *testing.T) {
	dir := t.TempDir()
	s, w := running(t, dir)
	hash := sha256.Sum256([]byte(w.RequestID + ":" + strings.Repeat("a", 32) + ":" + strings.Repeat("d", 32)))
	path := filepath.Join(dir, "artifacts", hex.EncodeToString(hash[:]))
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("fixture legacy unresolved effect")); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	legacy, _ := s.Get(w.RequestID)
	legacy.Status = "completed"
	if !validReplay(s, Event{Type: "work.completed", Work: legacy}) {
		t.Fatal("original completion schema changed")
	}
	if err := s.commit(Event{Type: "work.completed", Work: legacy}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	for i := 0; i < 3; i++ {
		var err error
		s, err = Open(dir)
		if err != nil {
			t.Fatal("previously accepted journal became corrupt", err)
		}
		if err = s.RecoverInterrupted(); err != nil {
			t.Fatal(err)
		}
		current, _ := s.Get(w.RequestID)
		if current.Status != "completed" || current.Epoch != w.Epoch || current.Operations[0].Status != "unknown" || len(current.Artifacts) != 0 {
			t.Fatal("legacy receipt changed task or published unchecked bytes")
		}
		s.Close()
	}
}
