package ledger

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestProductCompositionReceiptsRedactContinuity(t *testing.T) {
	s, c, p := productFixture(t)
	metadata := continuityInput(p, c, "composition-continuity")
	metadata.Instructions = "PRIVATE_CONTINUITY_SENTINEL"
	record, _, err := s.SaveProjectContinuity(metadata)
	if err != nil {
		t.Fatal(err)
	}
	revision := record.Revision
	groupInput := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare evidence", MaxStages: 3, ClientNonce: "composition-group", ProjectID: p.ID, ProjectRevision: p.Revision, ExpectedContinuityRevision: &revision}
	group, _, err := s.CreateCollaboration(groupInput)
	if err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Sequence
	receipt, err := s.CommandReceipt("collaboration_create", groupInput.ClientNonce, "")
	if err != nil || !receipt.Found || receipt.Collaboration.ID != group.ID || !reflect.DeepEqual(receipt.Input, normalizeCollaboration(groupInput)) {
		t.Fatal("receipt identity lost", err)
	}
	raw, _ := json.Marshal(receipt)
	if strings.Contains(string(raw), metadata.Instructions) || receipt.Collaboration.Resources.Continuity.Metadata != nil {
		t.Fatal("read-only receipt leaked continuity metadata")
	}
	if s.Snapshot().Sequence != before || s.collaborations[group.ID].Resources.Continuity.Metadata == nil {
		t.Fatal("projection mutated pinned metadata")
	}
}

func TestProductCompositionPluginRejectsDecodedCredentialsBeforeCommit(t *testing.T) {
	for _, field := range []string{"name", "description", "schema"} {
		t.Run(field, func(t *testing.T) {
			s, _, _ := productFixture(t)
			secret := "password = \"" + strings.Repeat("x", 24) + "\""
			manifest := PluginManifest{Name: "Descriptor", Version: "1", License: "local", Tools: []PluginTool{{Name: "describe", Description: "Read a descriptor", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}}}
			switch field {
			case "name":
				manifest.Name = secret
			case "description":
				manifest.Tools[0].Description = secret
			case "schema":
				manifest.Tools[0].InputSchema, _ = json.Marshal(map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string", "description": secret}}})
			}
			before := s.Snapshot().Sequence
			if _, _, err := s.ControlPlugin(PluginInput{Version: 1, ID: "local-descriptor", Action: "import", ClientNonce: "composition-import", Manifest: &manifest}); !errors.Is(err, ErrInvalid) {
				t.Fatal("decoded credential assignment was committed", err)
			}
			if s.Snapshot().Sequence != before {
				t.Fatal("rejected manifest changed journal")
			}
		})
	}
}

func TestProductCompositionFrozenPinsSurvivePolicyChangesAndRestart(t *testing.T) {
	s, c, p := productFixture(t)
	file, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "composition-ready-file", Filename: "evidence.md", Content: []byte("Dated evidence for the comparison")})
	if err != nil {
		t.Fatal(err)
	}
	metadata := continuityInput(p, c, "composition-owner-record")
	record, _, err := s.SaveProjectContinuity(metadata)
	if err != nil {
		t.Fatal(err)
	}
	revision := record.Revision
	selection := []PluginSelection{{ID: "neko-artifacts", ExpectedRevision: 0}, {ID: "neko-project-sources", ExpectedRevision: 0}}
	scope := SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{file.ID}, ExpectedContinuityRevision: &revision, PluginSelection: &selection}
	in := Input{BotID: "bot-meiiie", Text: "Compare authorized sources", Lane: "background", ClientNonce: "composition-parent-work"}
	w, _, err := s.AcceptProduct(in, scope)
	if err != nil {
		t.Fatal(err)
	}
	command := TaskControlInput{Version: 1, RequestID: w.RequestID, ExpectedEpoch: w.Epoch, Action: "supplement", ClientNonce: "composition-child-command", Text: "Include dated differences"}
	result, _, err := s.ControlTask(command)
	if err != nil {
		t.Fatal(err)
	}
	child := result.Related
	if child == nil || !reflect.DeepEqual(child.Resources, w.Resources) || !reflect.DeepEqual(child.PluginPins, w.PluginPins) {
		t.Fatal("linked child changed frozen policy")
	}
	group, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{file.ID}, ExpectedContinuityRevision: &revision, PluginSelection: &selection, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-davin", Goal: "Produce a final comparison", MaxStages: 3, ClientNonce: "composition-pinned-group"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	group = s.collaborations[group.ID]
	if len(group.RequestIDs) != 1 {
		t.Fatal("first stage missing")
	}
	stage, _ := s.Get(group.RequestIDs[0])
	if !reflect.DeepEqual(stage.Resources, group.Resources) || !reflect.DeepEqual(stage.PluginPins, group.PluginPins) {
		t.Fatal("group stage changed policy")
	}
	metadata.ExpectedRevision = record.Revision
	metadata.ClientNonce = "composition-owner-update"
	metadata.Instructions = "Use the revised owner guidance"
	if _, _, err = s.SaveProjectContinuity(metadata); err != nil {
		t.Fatal(err)
	}
	pluginChange(t, s, "neko-artifacts", "disable", "composition-disable-artifacts", 0)
	pluginChange(t, s, "neko-artifacts", "enable", "composition-enable-artifacts", 1)
	dir := s.dir
	s.Close()
	for i := 0; i < 3; i++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal("valid combined journal failed recovery", err)
		}
		if err = s.RecoverInterrupted(); err != nil {
			t.Fatal(err)
		}
		if err = s.AdvanceCollaborations(); err != nil {
			t.Fatal(err)
		}
		for _, original := range []Work{w, *child, stage} {
			current, _ := s.Get(original.RequestID)
			if s.RuntimeAllowed(current.RequestID, current.Epoch) || !reflect.DeepEqual(current.Resources, original.Resources) || !reflect.DeepEqual(current.PluginPins, original.PluginPins) {
				t.Fatal("stale task rebound after restart")
			}
		}
		if len(s.collaborations[group.ID].RequestIDs) != 1 || s.collaborations[group.ID].Status != "suspended" {
			t.Fatal("stale group advanced")
		}
		receipt, err := s.CommandReceipt("task_control", command.ClientNonce, "")
		if err != nil || !receipt.Found || receipt.Related.RequestID != child.RequestID || receipt.Related.Resources.Continuity.Metadata != nil {
			t.Fatal("receipt lost identity or leaked metadata", err)
		}
		duplicate, existed, err := s.AcceptProduct(in, scope)
		if err != nil || !existed || duplicate.RequestID != w.RequestID || duplicate.Resources.Continuity.Revision != revision {
			t.Fatal("old nonce reused current authority", err)
		}
		if _, _, err = s.Accept(input("composition-unrelated-work", "foreground")); err != nil {
			t.Fatal("unrelated intake blocked", err)
		}
		s.Close()
	}
}
