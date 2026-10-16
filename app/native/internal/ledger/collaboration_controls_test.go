package ledger

import (
	"errors"
	"reflect"
	"testing"
)

func TestCombinedGroupStageControlsCannotForkOrReviveGroup(t *testing.T) {
	s, g, _, _ := collaborationFixture(t)
	_, stage := advanceGroup(t, s, g.ID)
	before := s.Snapshot()
	for _, action := range []string{"pause", "resume", "cancel", "supplement", "replace"} {
		in := controlInput(stage, action, "combined-group-control-"+action)
		if action == "supplement" || action == "replace" {
			in.Text = "Change this group stage independently"
		}
		if _, _, err := s.ControlTask(in); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s forked group lifecycle: %v", action, err)
		}
		if !reflect.DeepEqual(before, s.Snapshot()) {
			t.Fatalf("%s changed group despite rejection", action)
		}
	}
	if _, _, err := s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: groupState(t, s, g.ID).Revision, ClientNonce: "combined-group-scoped-cancel"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	if current := groupState(t, s, g.ID); current.Status != "cancelled" || len(current.RequestIDs) != 1 {
		t.Fatal("scoped cancellation failed", current)
	}
}

func TestCombinedReplayRejectsMixedControlAndGroupPayloads(t *testing.T) {
	s, g, _, _ := collaborationFixture(t)
	groupEvent := s.events[len(s.events)-1]
	delete(s.collaborations, g.ID)
	delete(s.collaborationNonces, g.ClientNonce)
	if !validReplay(s, groupEvent) {
		t.Fatal("unmodified group event failed positive replay control")
	}
	groupEvent.TaskControl = &TaskControlEvent{Version: 1}
	if validReplay(s, groupEvent) {
		t.Fatal("group event admitted task-control payload")
	}
	groupEvent.TaskControl = nil
	groupEvent.AdapterSession = &AdapterSession{}
	if validReplay(s, groupEvent) {
		t.Fatal("group event admitted adapter payload")
	}
	s.collaborations[g.ID] = g
	s.collaborationNonces[g.ClientNonce] = g.ID
	w, _, err := s.Accept(input("combined-replay-control-work", "foreground"))
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := s.ControlTask(controlInput(w, "pause", "combined-replay-control-pause"))
	if err != nil || result.Work.Status != "paused" {
		t.Fatal(err)
	}
	controlEvent := s.events[len(s.events)-1]
	delete(s.taskControlNonces, controlEvent.TaskControl.Input.ClientNonce)
	s.work[w.RequestID] = w
	if !validReplay(s, controlEvent) {
		t.Fatal("unmodified task-control event failed positive replay control")
	}
	controlEvent.Collaboration = &Collaboration{}
	if validReplay(s, controlEvent) {
		t.Fatal("task-control event admitted group payload")
	}
}
