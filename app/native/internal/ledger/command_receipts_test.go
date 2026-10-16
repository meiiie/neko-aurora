package ledger

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestReadOnlyCommandReceiptsSurviveAuthorityChangeAndRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	w, _, err := s.Accept(input("receipt-original", "background"))
	if err != nil {
		t.Fatal(err)
	}
	in := TaskControlInput{Version: 1, RequestID: w.RequestID, ExpectedEpoch: w.Epoch, Action: "supplement", ClientNonce: "receipt-supplement", Text: "Keep this authorized addition"}
	result, _, err := s.ControlTask(in)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: w.RequestID, ExpectedRevision: 0, Granted: false, ClientNonce: "receipt-revoke"}); err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 3; restart++ {
		before := s.Snapshot().Sequence
		r, err := s.CommandReceipt("task_control", in.ClientNonce, "")
		if err != nil || !r.Found || !reflect.DeepEqual(r.Input, in) || r.Work.RequestID != w.RequestID || r.Related.RequestID != result.Related.RequestID {
			t.Fatal("original command proof lost", r, err)
		}
		if r.Work.Epoch <= in.ExpectedEpoch || r.Work.DesiredState != "suspended" {
			t.Fatal("receipt did not return current graph", r.Work)
		}
		encoded, _ := json.Marshal(r)
		if strings.Contains(string(encoded), "contextPin") {
			t.Fatal("receipt exposes context history")
		}
		r.Work.Text = "caller mutation"
		unknown, err := s.CommandReceipt("task_control", "receipt-never-posted", "")
		if err != nil || unknown.Found || unknown.Input != nil || s.Snapshot().Sequence != before {
			t.Fatal("read-only lookup mutated or invented command", unknown, err)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCollaborationReceiptsKeepOriginalInputAndCurrentGraph(t *testing.T) {
	s, g, in, dir := collaborationFixture(t)
	cancel := CollaborationCancelInput{ExpectedRevision: g.Revision, ClientNonce: "receipt-group-cancel"}
	if _, _, err := s.CancelCollaboration(g.ID, cancel); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	before := s.Snapshot().Sequence
	create, err := s.CommandReceipt("collaboration_create", in.ClientNonce, "")
	if err != nil || !create.Found || !reflect.DeepEqual(create.Input, normalizeCollaboration(in)) || create.Collaboration.ID != g.ID || create.Collaboration.Status != "cancelled" {
		t.Fatal(create, err)
	}
	stopped, err := s.CommandReceipt("collaboration_cancel", cancel.ClientNonce, g.ID)
	if err != nil || !stopped.Found || !reflect.DeepEqual(stopped.Input, cancel) || stopped.Collaboration.Revision <= cancel.ExpectedRevision {
		t.Fatal(stopped, err)
	}
	if _, err := s.CommandReceipt("collaboration_cancel", cancel.ClientNonce, "collab-"+strings.Repeat("a", 32)); !errors.Is(err, ErrConflict) {
		t.Fatal("receipt rebound to other group", err)
	}
	for _, query := range [][3]string{{"bad", in.ClientNonce, ""}, {"task_control", "", ""}, {"collaboration_create", in.ClientNonce, g.ID}, {"collaboration_cancel", cancel.ClientNonce, ""}} {
		if _, err := s.CommandReceipt(query[0], query[1], query[2]); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid lookup accepted", query, err)
		}
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("lookup changed sequence")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommandReceipt("collaboration_create", in.ClientNonce, ""); !errors.Is(err, ErrUnavailable) {
		t.Fatal("closed store represented as absent receipt", err)
	}
}

func TestAdapterCatalogContainsOnlyRoutingMetadata(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := sharedChannel(t, s, "Shared route", "catalog-channel")
	if _, _, err = s.Accept(input("catalog-private", "foreground")); err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Sequence
	metadata, err := s.AdapterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(metadata)
	if strings.Contains(string(b), "Keep the original") || strings.Contains(string(b), "work") || strings.Contains(string(b), "account") || strings.Contains(string(b), "description") {
		t.Fatal("private metadata exposed", string(b))
	}
	found := false
	for i := range metadata.Channels {
		if metadata.Channels[i].ID == c.ID {
			found = true
			metadata.Channels[i].BotIDs[0] = "caller mutation"
		}
	}
	if !found || metadata.Version != 1 || metadata.OwnerUserID != "local-owner" || metadata.SpaceID != "local-space" {
		t.Fatal(metadata)
	}
	fresh, err := s.AdapterCatalog()
	if err != nil || s.Snapshot().Sequence != before {
		t.Fatal("catalog mutated store", err)
	}
	for _, channel := range fresh.Channels {
		if channel.ID == c.ID && !reflect.DeepEqual(channel.BotIDs, c.BotIDs) {
			t.Fatal("catalog aliases store")
		}
	}
}
