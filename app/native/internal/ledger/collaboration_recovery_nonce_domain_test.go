package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func oldGroupNonceFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/group-recovery-b655-create-overlap.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != "583a73c5c470df34a6ff9a85f863407fb8255d3cba3da1ea2d8af07a1763a6a9" {
		t.Fatal("old b655 live-writer history changed")
	}
	return data
}

func TestGroupServiceRecoveryGenuineLegacyCreateOverlapReplayAndReceipts(t *testing.T) {
	data := oldGroupNonceFixture(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "transactions.jsonl"), data, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		s, err := Open(dir)
		if err != nil {
			t.Fatal("genuine old writer history cannot open", err)
		}
		var event Event
		for _, candidate := range s.events {
			if candidate.Type == "collaboration.recovered" {
				event = candidate
				break
			}
		}
		if event.Type != "collaboration.recovered" || event.CollaborationRecovery == nil || event.CollaborationRecovery.NonceDomainVersion != 0 {
			t.Fatal("fixture not old writer recovery")
		}
		in := event.CollaborationRecovery.Input
		g := groupState(t, s, event.Collaboration.ID)
		w, _ := s.Get(in.RequestID)
		before := s.Snapshot().Sequence
		s.now = func() time.Time { return time.Now().Add(24 * time.Hour) }
		if got, dup, e := s.RecoverCollaboration(g.ID, in); e != nil || !dup || !reflect.DeepEqual(got.Work, w) || !reflect.DeepEqual(got.Collaboration, g) {
			t.Fatal("old successful recovery receipt lost", e)
		}
		if got, dup, e := s.CreateCollaboration(g.CollaborationInput); e != nil || !dup || !reflect.DeepEqual(got, g) {
			t.Fatal("old successful creation receipt lost", e)
		}
		for _, kind := range []string{"collaboration_create", "collaboration_recover"} {
			id := ""
			if kind == "collaboration_recover" {
				id = g.ID
			}
			r, e := s.CommandReceipt(kind, in.ClientNonce, id)
			if e != nil || !r.Found {
				t.Fatal("old overlap lookup lost", kind, e)
			}
		}
		if s.Snapshot().Sequence != before {
			t.Fatal("legacy receipt revived expired group")
		}
		if _, duplicate, err := s.Accept(input("group-legacy-unrelated-"+string(rune('0'+i)), "foreground")); err != nil || duplicate {
			t.Fatal("historical overlap blocked unrelated intake", err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGroupServiceRecoveryNonceDomainNewReplayAndUnknownVersions(t *testing.T) {
	data := oldGroupNonceFixture(t)
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	var record struct {
		Event Event `json:"event"`
	}
	if err := json.Unmarshal(lines[len(lines)-1], &record); err != nil {
		t.Fatal(err)
	}
	prefix := append(bytes.Join(lines[:len(lines)-1], []byte("\n")), byte('\n'))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "transactions.jsonl"), prefix, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !validReplay(s, record.Event) {
		t.Fatal("genuine old recovery event refused")
	}
	for _, version := range []int{1, -1, 2} {
		event := record.Event
		meta := *event.CollaborationRecovery
		meta.NonceDomainVersion = version
		event.CollaborationRecovery = &meta
		if validReplay(s, event) {
			t.Fatal("new create-collision or unknown version admitted", version)
		}
	}
	fresh, _, g, w, _ := groupRecoveryFixture(t, 1)
	in := groupRecoverInput(g, w, "group-new-domain-nonce")
	if _, _, err := fresh.RecoverCollaboration(g.ID, in); err != nil {
		t.Fatal(err)
	}
	event := fresh.events[len(fresh.events)-1]
	if event.CollaborationRecovery.NonceDomainVersion != 1 {
		t.Fatal("new writer omitted domain version")
	}
	receipt, err := fresh.CommandReceipt("collaboration_recover", in.ClientNonce, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(receipt.Input)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("nonceDomainVersion")) {
		t.Fatal("internal version entered caller input")
	}
}

func TestGroupServiceRecoveryAllNewCommandKindDirections(t *testing.T) {
	kinds := []string{"collaboration_create", "task_control", "collaboration_cancel", "collaboration_recover"}
	for _, known := range kinds {
		for _, next := range kinds {
			if known == next {
				continue
			}
			t.Run(known+"/"+next, func(t *testing.T) {
				s, _, g, w, _ := groupRecoveryFixture(t, 1)
				nonce := "group-cross-kind-known"
				switch known {
				case "collaboration_create":
					nonce = g.ClientNonce
				case "task_control":
					other, _, e := s.Accept(input("group-cross-known-task", "foreground"))
					if e != nil {
						t.Fatal(e)
					}
					if _, _, e = s.ControlTask(controlInput(other, "pause", nonce)); e != nil {
						t.Fatal(e)
					}
				case "collaboration_cancel":
					if _, _, e := s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: g.Revision, ClientNonce: nonce}); e != nil {
						t.Fatal(e)
					}
				case "collaboration_recover":
					if _, _, e := s.RecoverCollaboration(g.ID, groupRecoverInput(g, w, nonce)); e != nil {
						t.Fatal(e)
					}
				}
				target, _, e := s.Accept(input("group-cross-target-task", "foreground"))
				if e != nil {
					t.Fatal(e)
				}
				current := groupState(t, s, g.ID)
				before := s.Snapshot().Sequence
				switch next {
				case "collaboration_create":
					in := g.CollaborationInput
					in.ClientNonce = nonce
					in.Goal += " distinct"
					_, _, e = s.CreateCollaboration(in)
				case "task_control":
					_, _, e = s.ControlTask(controlInput(target, "pause", nonce))
				case "collaboration_cancel":
					_, _, e = s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: current.Revision, ClientNonce: nonce})
				case "collaboration_recover":
					now, _ := s.Get(w.RequestID)
					_, _, e = s.RecoverCollaboration(g.ID, groupRecoverInput(current, now, nonce))
				}
				if !errors.Is(e, ErrConflict) || s.Snapshot().Sequence != before {
					t.Fatal("new command rebound cross-kind nonce", known, next, e)
				}
			})
		}
	}
}
