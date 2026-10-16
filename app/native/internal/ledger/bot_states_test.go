package ledger

import (
	"errors"
	"reflect"
	"testing"
)

func saveBotState(t *testing.T, s *Store, in BotControlInput) BotState {
	t.Helper()
	p, dup, err := s.ControlBot(in)
	if err != nil || dup {
		t.Fatal("save bot state", err)
	}
	return p
}

func TestBotStateDefaultAccountRetryAfterRemovalAndRestart(t *testing.T) {
	s, c, a, _, dir := accountFixture(t)
	p := saveBotState(t, s, BotControlInput{BotID: "bot-meiiie", Action: "defaults", ClientNonce: "state-default-account", DefaultAccountID: a.ID, DefaultAccountRevision: a.Revision, DefaultModel: a.Model})
	rev := p.Revision
	scope := SubmissionScope{UseBotDefaults: true, ExpectedBotStateRevision: &rev}
	in := input("state-account-old-intake", "background")
	old, _, err := s.AcceptProduct(in, scope)
	if err != nil || old.Account == nil || old.Account.AccountID != a.ID || old.BotState.DefaultModel != a.Model {
		t.Fatal("pin", err)
	}
	if err = c.RemoveAccount(a.ID, a.Revision); err != nil {
		t.Fatal(err)
	}
	saveBotState(t, s, BotControlInput{BotID: "bot-meiiie", Action: "defaults", ExpectedRevision: 1, ClientNonce: "state-default-clear"})
	saveBotState(t, s, BotControlInput{BotID: "bot-meiiie", Action: "archive", ExpectedRevision: 2, ClientNonce: "state-account-archive"})
	got, dup, err := s.AcceptProduct(in, scope)
	if err != nil || !dup || !reflect.DeepEqual(got, old) {
		t.Fatal("original receipt rebound", err)
	}
	if _, _, err = s.AcceptProduct(in, SubmissionScope{AccountID: a.ID, AccountRevision: a.Revision, ExpectedBotStateRevision: &rev}); !errors.Is(err, ErrConflict) {
		t.Fatal("same nonce mode changed", err)
	}
	s.Close()
	for n := 0; n < 3; n++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal("replay", err)
		}
		got, dup, err = s.AcceptProduct(in, scope)
		if err != nil || !dup || !reflect.DeepEqual(got, old) {
			t.Fatal("restart receipt changed", err)
		}
		s.Close()
	}
}

func TestBotStateRoutesDefaultsToTargetAndFencesStaleAccount(t *testing.T) {
	s, c, a, _, _ := accountFixture(t)
	saveBotState(t, s, BotControlInput{BotID: "bot-davin", Action: "defaults", ClientNonce: "state-route-default", DefaultAccountID: a.ID, DefaultAccountRevision: a.Revision, DefaultModel: a.Model})
	rev := uint64(1)
	in := Input{"bot-meiiie", "state-route-intake", "@davin Compare this", "foreground"}
	w, _, err := s.AcceptProduct(in, SubmissionScope{UseBotDefaults: true, ExpectedBotStateRevision: &rev})
	if err != nil || w.BotID != "bot-davin" || w.BotState.BotID != "bot-davin" || w.Account.AccountID != a.ID {
		t.Fatal("target pin", err)
	}
	if err = c.RemoveAccount(a.ID, a.Revision); err != nil {
		t.Fatal(err)
	}
	in.ClientNonce = "state-route-new-nonce"
	before := s.Snapshot().Sequence
	if _, _, err = s.AcceptProduct(in, SubmissionScope{UseBotDefaults: true, ExpectedBotStateRevision: &rev}); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("stale default fell back", err)
	}
}

func TestBotArchiveRetainsOldGroupStagesAndLinkedPreference(t *testing.T) {
	s, g, in, _ := collaborationFixture(t)
	p := saveBotState(t, s, BotControlInput{BotID: "bot-meiiie", Action: "defaults", ClientNonce: "state-linked-default", DefaultModel: "desired-original"})
	rev := p.Revision
	old, _, err := s.AcceptProduct(input("state-linked-intake", "background"), SubmissionScope{UseBotDefaults: true, ExpectedBotStateRevision: &rev})
	if err != nil {
		t.Fatal(err)
	}
	saveBotState(t, s, BotControlInput{BotID: "bot-meiiie", Action: "archive", ExpectedRevision: 1, ClientNonce: "state-group-archive"})
	saveBotState(t, s, BotControlInput{BotID: "bot-meiiie", Action: "defaults", ExpectedRevision: 2, ClientNonce: "state-linked-new-default", DefaultModel: "desired-next"})
	control := controlInput(old, "supplement", "state-linked-supplement")
	control.Text = "Include the allowed appendix"
	result, _, err := s.ControlTask(control)
	if err != nil || result.Related == nil || !reflect.DeepEqual(result.Related.BotState, old.BotState) || !reflect.DeepEqual(result.Related.ExpectedBotStateRevision, old.ExpectedBotStateRevision) {
		t.Fatal("linked preference not frozen", err)
	}
	if _, err = s.Claim(result.Related.RequestID, result.Related.Epoch); err != nil {
		t.Fatal("old linked obligation stopped by archive", err)
	}
	in.ClientNonce = "state-new-group-blocked"
	if _, _, err = s.CreateCollaboration(in); !errors.Is(err, ErrConflict) {
		t.Fatal("fresh group includes archive", err)
	}
	for n := 0; n < 3; n++ {
		_, w := advanceGroup(t, s, g.ID)
		w = admitGroup(t, s, w)
		if err = s.AppendText(w.RequestID, w.Epoch, "existing group reply"); err != nil {
			t.Fatal(err)
		}
		if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.AdvanceCollaborations(); err != nil || groupState(t, s, g.ID).Status != "completed" {
		t.Fatal("archive interrupted old group", err)
	}
}

func TestBotStateCopiesReceiptsAndReplayRejectAuxiliaryEvents(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	in := BotControlInput{BotID: "bot-meiiie", Action: "defaults", ClientNonce: "state-copy-default", DefaultModel: "desired-copy"}
	saveBotState(t, s, in)
	rev := uint64(1)
	w, _, err := s.AcceptProduct(input("state-copy-intake", "foreground"), SubmissionScope{UseBotDefaults: true, ExpectedBotStateRevision: &rev})
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	snap.BotStates[0].DefaultModel = "mutated"
	w.BotState.DefaultModel = "mutated"
	*w.ExpectedBotStateRevision = 99
	events, _ := s.Events(0)
	for i := range events {
		if events[i].BotState != nil {
			events[i].BotState.DefaultModel = "mutated"
			events[i].BotControlInput.DefaultModel = "mutated"
		}
	}
	receipt, err := s.CommandReceipt("bot_control", in.ClientNonce, "")
	if err != nil || !receipt.Found || receipt.BotState.DefaultModel != "desired-copy" {
		t.Fatal("receipt mutation", err)
	}
	current, _ := s.Get(w.RequestID)
	if current.BotState.DefaultModel != "desired-copy" || *current.ExpectedBotStateRevision != 1 {
		t.Fatal("work alias mutation")
	}
	if _, _, err = s.RenameBot(in.BotID, BotNameInput{Name: "Conflicting", ClientNonce: in.ClientNonce}); !errors.Is(err, ErrConflict) {
		t.Fatal("cross metadata nonce", err)
	}
	next := BotState{Version: 1, BotID: in.BotID, Revision: 2, Archived: true, DefaultModel: "desired-copy"}
	action := BotControlInput{BotID: in.BotID, Action: "archive", ExpectedRevision: 1, ClientNonce: "state-copy-next"}
	e := Event{Type: "bot.state_updated", BotState: &next, BotControlInput: &action, Nonce: action.ClientNonce}
	if !validReplay(s, e) {
		t.Fatal("valid control rejected")
	}
	e.Work = current
	if validReplay(s, e) {
		t.Fatal("auxiliary Work accepted")
	}
	forged := copyWork(current)
	forged.RequestID = "req-forged-state"
	forged.ClientNonce = "state-forged-intake"
	forged.CreatedSequence = s.Snapshot().Sequence + 1
	forged.BotState.DefaultModel = "forged"
	if validReplay(s, Event{Sequence: forged.CreatedSequence, Type: "work.accepted", Work: forged}) {
		t.Fatal("forged default intake accepted")
	}
}
