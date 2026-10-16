package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func input(nonce, lane string) Input {
	return Input{BotID: "bot-meiiie", ClientNonce: nonce, Text: "Keep the original report goal", Lane: lane}
}
func TestDurableIdentityLanesAndNonce(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	background, dup, err := s.Accept(input("nonce-background", "background"))
	if err != nil || dup {
		t.Fatal(err)
	}
	foreground, _, err := s.Accept(input("nonce-foreground", "foreground"))
	if err != nil {
		t.Fatal(err)
	}
	if foreground.RequestID == background.RequestID || foreground.SourceMessageID == background.SourceMessageID {
		t.Fatal("identities collapsed")
	}
	if _, _, err = s.Accept(Input{BotID: "bot-davin", ClientNonce: "nonce-background", Text: "Independent bot context", Lane: "foreground"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Accept(Input{BotID: background.BotID, ClientNonce: background.ClientNonce, Text: "Changed goal", Lane: background.Lane}); !errors.Is(err, ErrConflict) {
		t.Fatal("nonce conflict not rejected", err)
	}
	before := s.Snapshot()
	if before.Work[0].Text != background.Text || before.Work[0].Status != "waiting_host" {
		t.Fatal("foreground modified background")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	replay, dup, err := s.Accept(input("nonce-background", "background"))
	if err != nil || !dup || replay.RequestID != background.RequestID {
		t.Fatal("restart duplicated accepted work")
	}
	if s.Snapshot().Sequence != 3 || len(s.Snapshot().Work) != 3 {
		t.Fatal("history lost or duplicated")
	}
	events, err := s.Events(1)
	if err != nil || len(events) != 2 || events[0].Sequence != 2 {
		t.Fatal("cursor replay failed")
	}
}
func TestConcurrentSameNonceHasOneTransaction(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var wg sync.WaitGroup
	for i := 0; i < 48; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := s.Accept(input("concurrent-nonce", "foreground")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	snap := s.Snapshot()
	if snap.Sequence != 1 || len(snap.Work) != 1 {
		t.Fatal("concurrent duplicate")
	}
}
func TestWriterLockReleasedOnClose(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Open(dir); err == nil {
		other.Close()
		t.Fatal("dual writer permitted")
	}
	s.Close()
	other, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	other.Close()
}
func TestUnacknowledgedTornTailRecovered(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	work, _, err := s.Accept(input("survives-crash", "background"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	path := filepath.Join(dir, "transactions.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("{\"version\":1,\"event\":"); err != nil {
		t.Fatal(err)
	}
	f.Sync()
	f.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, dup, err := s.Accept(input("survives-crash", "background"))
	if err != nil || !dup || got.RequestID != work.RequestID || s.Snapshot().Sequence != 1 {
		t.Fatal("torn tail damaged committed prefix")
	}
	if _, _, err = s.Accept(input("after-crash", "foreground")); err != nil {
		t.Fatal(err)
	}
}
func TestCommittedCorruptionFailsClosed(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	s.Accept(input("original-nonce", "foreground"))
	s.Close()
	path := filepath.Join(dir, "transactions.jsonl")
	b, _ := os.ReadFile(path)
	var r record
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	r.Event.Work.Text = "tampered"
	b, _ = json.Marshal(r)
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := Open(dir); !errors.Is(err, ErrCorrupt) {
		if other != nil {
			other.Close()
		}
		t.Fatal("corruption accepted", err)
	}
}
func TestForgedOwnerEvenWithRecomputedDigestRejected(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	s.Accept(input("owner-bound-nonce", "foreground"))
	s.Close()
	path := filepath.Join(dir, "transactions.jsonl")
	b, _ := os.ReadFile(path)
	var r record
	json.Unmarshal(b, &r)
	r.Event.Work.OwnerUserID = "another-owner"
	r.Digest, _ = digestRecord(r)
	b, _ = json.Marshal(r)
	os.WriteFile(path, append(b, '\n'), 0600)
	if other, err := Open(dir); !errors.Is(err, ErrCorrupt) {
		if other != nil {
			other.Close()
		}
		t.Fatal("forged owner accepted")
	}
}
func TestCancelDoesNotCancelOtherLaneAndIsDurable(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	background, _, _ := s.Accept(input("cancel-background", "background"))
	foreground, _, _ := s.Accept(input("keep-foreground", "foreground"))
	cancelled, err := s.Cancel(background.RequestID)
	if err != nil || cancelled.Epoch != 2 {
		t.Fatal(err)
	}
	s.Cancel(background.RequestID)
	if s.Snapshot().Sequence != 3 {
		t.Fatal("duplicate cancel")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	old, dup, err := s.Accept(input("cancel-background", "background"))
	if err != nil || !dup || old.Status != "cancelled" {
		t.Fatal("cancel not replayed")
	}
	for _, w := range s.Snapshot().Work {
		if w.RequestID == foreground.RequestID && w.Status != "waiting_host" {
			t.Fatal("cancelled wrong lane")
		}
	}
}
func TestFailedWriteCannotBeAcknowledgedOrUpdateMemory(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.file.Close()
	if _, _, err = s.Accept(input("failed-write-nonce", "foreground")); !errors.Is(err, ErrUnavailable) {
		t.Fatal("write failure acknowledged")
	}
	if s.Snapshot().Sequence != 0 || len(s.Snapshot().Work) != 0 {
		t.Fatal("uncommitted state visible")
	}
	if _, _, err = s.Accept(input("retry-poisoned-store", "foreground")); !errors.Is(err, ErrUnavailable) {
		t.Fatal("poisoned writer reused")
	}
}
