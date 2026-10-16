package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProjectArchivePreservesPinnedJobBytesAndRestart(t *testing.T) {
	s, c, p := productFixture(t)
	upload := UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "project-lifecycle-upload", Filename: "retained.md", Content: []byte("retained source bytes")}
	f, _, err := s.UploadFile(upload)
	if err != nil {
		t.Fatal(err)
	}
	in := Input{"bot-meiiie", "project-lifecycle-intake", "Use retained source", "background"}
	scope := SubmissionScope{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID}
	old, _, err := s.AcceptProduct(in, scope)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.dir, "uploads", f.ID)
	before, _ := os.Stat(path)
	command := ProjectControlInput{ProjectID: p.ID, Action: "archive", ClientNonce: "project-lifecycle-archive"}
	state, _, err := s.ControlProject(command)
	if err != nil || !state.Archived {
		t.Fatal(err)
	}
	got, dup, err := s.AcceptProduct(in, scope)
	if err != nil || !dup || !reflect.DeepEqual(old, got) {
		t.Fatal("old work changed", err)
	}
	if _, dup, err = s.UploadFile(upload); err != nil || !dup {
		t.Fatal("old upload ACK changed", err)
	}
	if _, b, err := s.ReadProjectFile(p.ID, c.ID, f.ID); err != nil || string(b) != "retained source bytes" {
		t.Fatal("archived file no longer readable", err)
	}
	w, err := s.Claim(old.RequestID, old.Epoch)
	if err != nil || w.Status != "starting" {
		t.Fatal("old obligation stopped", err)
	}
	newer := in
	newer.ClientNonce = "project-lifecycle-new-intake"
	if _, _, err = s.AcceptProduct(newer, scope); !errors.Is(err, ErrConflict) {
		t.Fatal("new archived work", err)
	}
	after, _ := os.Stat(path)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("archive touched raw file")
	}
	dir := s.dir
	s.Close()
	for n := 0; n < 3; n++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal("archive replay", err)
		}
		receipt, err := s.CommandReceipt("project_control", command.ClientNonce, "")
		if err != nil || !receipt.Found || !receipt.ProjectState.Archived || !reflect.DeepEqual(receipt.Input, command) {
			t.Fatal("original state receipt", err)
		}
		s.Close()
	}
}

func TestProjectArchiveOldFiniteGroupAndRestoreCAS(t *testing.T) {
	s, c, p := productFixture(t)
	in := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, ProjectID: p.ID, ProjectRevision: p.Revision, Participants: []string{"bot-davin", "bot-meiiie"}, CoordinatorBotID: "bot-meiiie", Goal: "Keep accepted goal", MaxStages: 3, ClientNonce: "project-state-old-group"}
	group, _, err := s.CreateCollaboration(in)
	if err != nil {
		t.Fatal(err)
	}
	archive := ProjectControlInput{ProjectID: p.ID, Action: "archive", ClientNonce: "project-state-group-archive"}
	if _, _, err = s.ControlProject(archive); err != nil {
		t.Fatal(err)
	}
	in.ClientNonce = "project-state-new-group"
	if _, _, err = s.CreateCollaboration(in); !errors.Is(err, ErrConflict) {
		t.Fatal("new archived group", err)
	}
	_, stage := advanceGroup(t, s, group.ID)
	if stage.Resources.Project.Revision != p.Revision {
		t.Fatal("authority revision changed")
	}
	if _, _, err = s.ControlProject(ProjectControlInput{ProjectID: p.ID, Action: "restore", ClientNonce: "project-state-stale-restore"}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale restore", err)
	}
	if _, _, err = s.ControlProject(ProjectControlInput{ProjectID: p.ID, Action: "restore", ExpectedRevision: 1, ClientNonce: "project-state-real-restore"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.CreateCollaboration(in); err != nil {
		t.Fatal("restored fresh group", err)
	}
}
