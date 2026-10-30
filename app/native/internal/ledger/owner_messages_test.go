package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
)

func ownerGroup(t *testing.T, s *Store) Channel {
	t.Helper()
	c, _, err := s.SaveChannel(ChannelInput{Name: "Fixture", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "owner-group-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestOwnerMessageAtomicValidationAndRetry(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	s.ConfigureContextAdmission(true)
	c := ownerGroup(t, s)
	in := OwnerMessageInput{ThreadID: c.ID, ClientNonce: "owner-message-fixture", Text: "Summarize", ReferenceID: "bot-meiiie"}
	plan := OwnerMessagePlan{BotIDs: c.BotIDs, ChannelRevision: c.Revision, ReferenceText: "\nPinned source"}
	before := s.Snapshot().Sequence
	bad := plan
	bad.BotIDs = []string{c.BotIDs[0], "missing-bot"}
	if _, _, err := s.AcceptOwnerMessage(in, bad); err == nil {
		t.Fatal("accepted invalid second recipient")
	}
	if s.Snapshot().Sequence != before || len(s.Snapshot().Work) != 0 {
		t.Fatal("partial admission")
	}
	bad = plan
	bad.ChannelRevision++
	if _, _, err := s.AcceptOwnerMessage(in, bad); !errors.Is(err, ErrConflict) {
		t.Fatal("stale channel", err)
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("stale revision wrote work")
	}
	works, dup, err := s.AcceptOwnerMessage(in, plan)
	if err != nil || dup || len(works) != 2 {
		t.Fatal(works, dup, err)
	}
	if works[1].CreatedSequence != works[0].CreatedSequence+1 {
		t.Fatal("event cursor")
	}
	events, _ := s.Events(before)
	if len(events) != 2 || events[0].Work.RequestID != works[0].RequestID || events[1].Work.RequestID != works[1].RequestID {
		t.Fatal("history lost a recipient")
	}
	_, _, err = s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, BotIDs: c.BotIDs[:1], ExpectedRevision: c.Revision, ClientNonce: "changed-group-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, original := range works {
		current, err := s.Get(original.RequestID)
		if err != nil || current.Text != original.Text || current.CreatedAt != original.CreatedAt {
			t.Fatal("identity or context changed", err)
		}
		works[i] = current
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			again, dup, err := s.AcceptOwnerMessage(in, OwnerMessagePlan{ReferenceText: "new source"})
			if err != nil || !dup || !reflect.DeepEqual(again, works) {
				t.Errorf("receipt changed: %v %v", dup, err)
			}
		}()
	}
	wg.Wait()
	if len(s.Snapshot().Work) != 2 {
		t.Fatal("retry added work")
	}
	for _, changed := range []OwnerMessageInput{
		{ThreadID: in.ThreadID, ClientNonce: in.ClientNonce, Text: "different", ReferenceID: in.ReferenceID},
		{ThreadID: in.ThreadID, ClientNonce: in.ClientNonce, Text: in.Text, ReferenceID: "bot-davin"},
	} {
		if _, _, err := s.AcceptOwnerMessage(changed, plan); !errors.Is(err, ErrConflict) {
			t.Fatal("nonce conflict", err)
		}
	}
}

func TestOwnerMessageWriteFailureAndTruncatedFrame(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := ownerGroup(t, s)
	in := OwnerMessageInput{ThreadID: c.ID, ClientNonce: "atomic-frame-fixture", Text: "All or none"}
	plan := OwnerMessagePlan{BotIDs: c.BotIDs, ChannelRevision: c.Revision}
	path := s.file.Name()
	prefix, _ := os.ReadFile(path)
	s.file.Close()
	if _, _, err := s.AcceptOwnerMessage(in, plan); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if len(s.Snapshot().Work) != 0 {
		t.Fatal("failed write exposed work")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AcceptOwnerMessage(in, plan); err != nil {
		t.Fatal(err)
	}
	s.Close()
	full, _ := os.ReadFile(path)
	for _, n := range []int{1, len(full[len(prefix):]) / 2, len(full) - len(prefix) - 1} {
		if err := os.WriteFile(path, full[:len(prefix)+n], 0600); err != nil {
			t.Fatal(err)
		}
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Snapshot().Work) != 0 {
			t.Fatal("torn transaction exposed a recipient")
		}
		s.Close()
		data, _ := os.ReadFile(path)
		if !bytes.Equal(data, prefix) {
			t.Fatal("incomplete frame not removed")
		}
	}
	if err := os.WriteFile(path, full, 0600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(s.Snapshot().Work) != 2 {
		t.Fatal("complete frame lost recipients")
	}
}

func TestOwnerMessageReplayRejectsRehashedTampering(t *testing.T) {
	mutations := map[string]func(*record){
		"receipt text":        func(r *record) { r.Message.Input.Text = "changed" },
		"receipt reference":   func(r *record) { r.Message.Input.ReferenceID = "" },
		"missing recipient":   func(r *record) { r.Batch = r.Batch[:1] },
		"duplicate recipient": func(r *record) { r.Batch[1] = r.Batch[0] },
		"authority pin":       func(r *record) { r.Batch[1].Work.Publication.AudienceRevision++ },
		"context hash":        func(r *record) { r.Batch[1].Work.Context.SHA256 = "changed" },
		"sequence":            func(r *record) { r.Batch[1].Sequence++ },
		"version downgrade":   func(r *record) { r.Version = 1 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			c := ownerGroup(t, s)
			_, _, err = s.AcceptOwnerMessage(OwnerMessageInput{ThreadID: c.ID, ClientNonce: "tamper-frame-fixture", Text: "Original", ReferenceID: "bot-meiiie"}, OwnerMessagePlan{BotIDs: c.BotIDs, ChannelRevision: c.Revision, ReferenceText: "\nPinned"})
			if err != nil {
				t.Fatal(err)
			}
			path := s.file.Name()
			s.Close()
			data, _ := os.ReadFile(path)
			start := bytes.LastIndexByte(data[:len(data)-1], '\n') + 1
			var r record
			if err := json.Unmarshal(data[start:], &r); err != nil {
				t.Fatal(err)
			}
			mutate(&r)
			r.Digest, _ = digestRecord(r)
			line, _ := json.Marshal(r)
			if err := os.WriteFile(path, append(append(data[:start], line...), '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			if reopened, err := Open(dir); !errors.Is(err, ErrCorrupt) {
				if reopened != nil {
					reopened.Close()
				}
				t.Fatal("tampering accepted", err)
			}
		})
	}
}

func TestOwnerMessageDoesNotFillLegacyPartial(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := ownerGroup(t, s)
	_, _, err = s.AcceptProduct(Input{BotID: c.BotIDs[0], ClientNonce: "legacy-partial-fixture_g1", Text: "Original"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.AcceptOwnerMessage(OwnerMessageInput{ThreadID: c.ID, ClientNonce: "legacy-partial-fixture", Text: "Original"}, OwnerMessagePlan{BotIDs: c.BotIDs, ChannelRevision: c.Revision})
	if !errors.Is(err, ErrConflict) || len(s.Snapshot().Work) != 1 {
		t.Fatal("filled a legacy partial transaction", err)
	}
}
