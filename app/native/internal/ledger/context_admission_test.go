package ledger

import (
	"encoding/json"
	"errors"
	"nekoaurora.local/native/internal/protocol"
	"reflect"
	"strings"
	"testing"
)

func TestContextAdmissionAuthenticatedIntakeAndProjection(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "context-intake-original", Text: "Owner literal <>& tiếng Việt", Lane: "background"})
	if err != nil {
		t.Fatal(err)
	}
	if w.ContextProtocolVersion != 1 || w.RuntimeContext == nil || w.RuntimeContext.PrimaryMessage.ID != w.SourceMessageID || w.RuntimeContext.PrimaryMessage.Text != w.Text || w.RuntimeContext.PrimaryMessage.SenderID != w.OwnerUserID {
		t.Fatal("original intake not attested")
	}
	if raw, e := MarshalRuntimeLaunch(w, nil); e != nil || !strings.Contains(string(raw), "contextManifestSHA256") {
		t.Fatal("packed launch missing manifest", e)
	}
	pub := s.PublicSnapshot()
	if pub.Work[0].RuntimeContext != nil {
		t.Fatal("private runtime manifest escaped public snapshot")
	}
	events, _ := s.Events(0)
	if events[0].Work.RuntimeContext != nil {
		t.Fatal("private manifest escaped SSE")
	}
	copy := copyWork(w)
	copy.RuntimeContext.PrimaryMessage.Text = "mutated"
	if w.RuntimeContext.PrimaryMessage.Text != w.Text {
		t.Fatal("manifest alias")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := s.Get(w.RequestID)
		if !reflect.DeepEqual(got.RuntimeContext, w.RuntimeContext) || got.RuntimeContextSHA256 != w.RuntimeContextSHA256 {
			t.Fatal("manifest drift on replay")
		}
		s.Close()
	}
}

func TestContextAdmissionEncodedBoundBeforeCommit(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.ConfigureContextAdmission(true)
	before := s.Snapshot().Sequence
	_, _, err = s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "context-too-large", Text: strings.Repeat("\\", 24000), Lane: "background"})
	if !errors.Is(err, ErrInvalid) || s.Snapshot().Sequence != before || len(s.Snapshot().Work) != 0 {
		t.Fatal("oversized encoded admission committed", err)
	}
	long := strings.Repeat("é", 4500)
	w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "context-long-primary", Text: long, Lane: "background"})
	if err != nil {
		t.Fatal(err)
	}
	source := w.RuntimeContext.Sources[0]
	if w.RuntimeContext.PrimaryMessage.Text != long || source.Bytes != len(long) || source.SHA256 != manifestHash(long) || source.Span.End > 8192 || source.Span.Text != long[:source.Span.End] {
		t.Fatal("bounded span lost full original identity")
	}
}

func TestContextAdmissionLegacyOversizeBlocksOnlyThatTask(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "legacy-context-bound", Text: strings.Repeat("\\", 24000), Lane: "background"})
	if err != nil {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	before := s.Snapshot().Sequence
	if _, err = s.Claim(w.RequestID, w.Epoch); !errors.Is(err, ErrInvalid) || s.Snapshot().Sequence != before {
		t.Fatal("invalid adoption committed", err)
	}
	if err = s.BlockContextAdmission(w.RequestID, w.Epoch); err != nil {
		t.Fatal(err)
	}
	blocked, _ := s.Get(w.RequestID)
	if blocked.Status != "suspended" || blocked.Failure != "context_admission_unavailable" || blocked.ContextProtocolVersion != 0 || !reflect.DeepEqual(blocked.Context, w.Context) || blocked.Epoch != w.Epoch+1 {
		t.Fatal("blocked task changed original authority")
	}
	if _, _, err = s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "context-bound-unrelated", Text: "Unrelated request remains available.", Lane: "foreground"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	for i := 0; i < 3; i++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := s.Get(w.RequestID)
		if !reflect.DeepEqual(got, blocked) {
			t.Fatal("task-local block lost on replay")
		}
		s.Close()
	}
}

func TestContextAdmissionGroupOriginalGoalAndGeneratedStage(t *testing.T) {
	s, g, _, _ := collaborationFixture(t)
	s.ConfigureContextAdmission(true)
	_, w := advanceGroup(t, s, g.ID)
	if w.RuntimeContext == nil || w.RuntimeContext.PrimaryMessage.ID != g.ID || w.RuntimeContext.PrimaryMessage.Text != g.Goal || w.RuntimeContext.PrimaryMessage.ID == w.SourceMessageID {
		t.Fatal("generated stage was attested as owner request")
	}
	if w.RuntimeContext.Sources[1].Kind != "derived" || !strings.Contains(w.RuntimeContext.Sources[1].Span.Text, "generated-collaboration-stage") {
		t.Fatal("stage instruction lost")
	}
	raw, err := MarshalRuntimeLaunch(w, nil)
	var frame map[string]json.RawMessage
	if err != nil || json.Unmarshal(raw, &frame) != nil {
		t.Fatal(err)
	}
	var text string
	json.Unmarshal(frame["text"], &text)
	if text != g.Goal {
		t.Fatal("wrapper promoted generated stage text")
	}
}

func TestContextAdmissionGroupCapturesExecutorBeforeStageAttestation(t *testing.T) {
	s, _ := executorFixture(t)
	s.ConfigureContextAdmission(true)
	c := sharedChannel(t, s, "Pinned collaboration", "context-executor-group-channel")
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare plans and return a file", MaxStages: 3, ClientNonce: "context-executor-group-create"})
	if err != nil {
		t.Fatal(err)
	}
	if g.ExecutorPolicyVersion != 1 || g.ExecutorPin == nil {
		t.Fatal("fixture must pin a ready executor")
	}
	current, w := advanceGroup(t, s, g.ID)
	if w.ExecutorPolicyVersion != g.ExecutorPolicyVersion || !reflect.DeepEqual(w.ExecutorPin, g.ExecutorPin) || w.RuntimeContext == nil || w.RuntimeContext.PrimaryMessage.ID != g.ID || w.RuntimeContext.PrimaryMessage.Text != g.Goal {
		t.Fatal("executor or original owner goal lost during stage intake")
	}
	dir := s.dir
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	re, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer re.Close()
	got, err := re.Get(w.RequestID)
	if err != nil || !reflect.DeepEqual(got, w) || !reflect.DeepEqual(groupState(t, re, g.ID), current) {
		t.Fatal("stage attestation lost on replay", err)
	}
}

func TestContextAdmissionDefaultReaderSelectsReadyText(t *testing.T) {
	s, channel, project := productFixture(t)
	s.ConfigureContextAdmission(true)
	file, _, err := s.UploadFile(UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel.ID, ClientNonce: "context-default-reader-file", Filename: "quote.txt", Content: []byte("Vendor quote costs 120 dollars.")})
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "context-default-reader-intake", Text: "Review vendor quote", Lane: "foreground"}, SubmissionScope{ChannelID: channel.ID, ProjectID: project.ID, ProjectRevision: project.Revision})
	if err != nil {
		t.Fatal("ready builtin reader refused default source", err)
	}
	found := false
	for _, source := range w.RuntimeContext.Sources {
		if source.Kind == "retrieved" && source.ID == file.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("selected source missing trusted manifest provenance")
	}
}

func TestContextAdmissionBindingTamperAndExactReadEnvelope(t *testing.T) {
	w, in := manifestWork("original")
	addManifestFile(&w, &in)
	built, err := BuildContextManifest(w, in)
	if err != nil {
		t.Fatal(err)
	}
	w.ContextProtocolVersion = 1
	w.RuntimeContext = &built.Manifest
	w.RuntimeContextSHA256 = built.SHA256
	b := CoreBinding{CoreSHA256: "97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb", SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"authorityId":"fixture"}`)}
	b.ContextAdmission = &ContextAdmission{Version: 1, SessionID: b.SessionID, ContextPinSHA256: w.Context.SHA256, ManifestSHA256: built.SHA256, SourceIDs: []string{strings.Repeat("d", 64)}, Task: append(json.RawMessage(nil), b.Task...), Execution: append(json.RawMessage(nil), b.Execution...)}
	if !validContextBinding(w, b) {
		t.Fatal("valid receipt rejected")
	}
	for _, runtime := range []string{"fa48d43f59257dcd171441f09a8ac53d3702485ddf7f80ac13a0796d53ac6310", "2a618df35470fb54d6058c44275999f00a95f55c2435f3aadafe19cc6b791904", "4b3fb6202551912f86b5dd5faffccf75d854174c2ddd6b9fb331470a2b2e0b36", "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"} {
		old := b
		old.CoreSHA256 = runtime
		if !protocol.CoreRuntimeAllowed(runtime) || protocol.CoreContextAdmission(runtime) {
			t.Fatal("historical test pin changed")
		}
		if validContextBinding(w, old) {
			t.Fatalf("historical runtime %s admitted context1", runtime)
		}
	}
	bad := b
	copy := *b.ContextAdmission
	copy.SourceIDs = []string{"file-one"}
	bad.ContextAdmission = &copy
	if validContextBinding(w, bad) {
		t.Fatal("renderer source IDs accepted as native journal proof")
	}
	copy = *b.ContextAdmission
	copy.ManifestSHA256 = strings.Repeat("f", 64)
	bad.ContextAdmission = &copy
	if validContextBinding(w, bad) {
		t.Fatal("wrong manifest accepted")
	}
	w.Core = &b
	source := w.RuntimeContext.Sources[len(w.RuntimeContext.Sources)-1]
	op := json.RawMessage(`{"version":1,"operationId":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","sessionId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","taskId":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`)
	if envelope := contextReadEnvelope(w, source.ID, source.Span.Offset, source.Span.End, source.Span.Text, op); envelope == nil {
		t.Fatal("exact selected read missing envelope")
	}
	if contextReadEnvelope(w, source.ID, source.Span.Offset, source.Span.End-1, source.Span.Text[:len(source.Span.Text)-1], op) != nil {
		t.Fatal("other authorized range minted selected provenance")
	}
}

func TestContextAdmissionSafeQueuedClaimAndExplicitRefreshUpgrade(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, _, err := s.Accept(input("context-queued-upgrade", "background"))
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := s.Accept(input("context-refresh-upgrade", "background"))
	if err != nil {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil || w.ContextProtocolVersion != 1 || w.RuntimeContext == nil {
		t.Fatal("safe queued claim could not attest durable owner intake", err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: other.RequestID, ExpectedRevision: 0, Granted: false, ClientNonce: "context-refresh-revoke"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: other.RequestID, ExpectedRevision: 1, Granted: true, ClientNonce: "context-refresh-regrant"}); err != nil {
		t.Fatal(err)
	}
	other, _ = s.Get(other.RequestID)
	fresh, err := s.RefreshContext(RefreshInput{RequestID: other.RequestID, ExpectedEpoch: other.Epoch})
	if err != nil || fresh.ContextProtocolVersion != 1 || fresh.RuntimeContext.PrimaryMessage.ID != other.SourceMessageID {
		t.Fatal("explicit fresh context could not adopt new protocol", err)
	}
}

func TestContextAdmissionTamperedAcceptedReplayRejected(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.ConfigureContextAdmission(true)
	w, _, err := s.Accept(input("context-replay-attestation", "background"))
	if err != nil {
		t.Fatal(err)
	}
	blank, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer blank.Close()
	baseline := Event{Type: "work.accepted", Sequence: 1, Work: copyWork(w)}
	if !validReplay(blank, baseline) {
		t.Fatal("valid typed context intake replay failed")
	}
	for _, kind := range []string{"primary", "digest", "protocol", "claim-version"} {
		t.Run(kind, func(t *testing.T) {
			e := baseline
			e.Work = copyWork(w)
			switch kind {
			case "primary":
				e.Work.RuntimeContext.PrimaryMessage.Text = "forged owner"
			case "digest":
				e.Work.RuntimeContextSHA256 = strings.Repeat("a", 64)
			case "protocol":
				e.Work.ContextProtocolVersion = 2
			case "claim-version":
				e.ContextClaimVersion = 1
			}
			if validReplay(blank, e) {
				t.Fatal("forged context admission replay accepted")
			}
		})
	}
}
