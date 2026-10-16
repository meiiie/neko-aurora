package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func groupContextReplayPolicy(t *testing.T, s *Store, g Collaboration, version int) (*Store, Collaboration) {
	t.Helper()
	probe := Work{BotID: g.CoordinatorBotID, OwnerUserID: g.OwnerUserID, SpaceID: g.SpaceID, ConversationID: g.ChannelID, ToolPolicyVersion: g.ToolPolicyVersion, PluginPins: g.PluginPins}
	s.pinBotProfile(&probe)
	policy := s.capturePublishedReportsVersion(probe, g.PublishedReportContext.AtSequence, version)
	if !pinnedTool(probe, "report.read_artifact") {
		stripPublishedFiles(policy.Pins)
	}
	dir := s.dir
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "transactions.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n"))
	var last record
	if err = json.Unmarshal(lines[len(lines)-1], &last); err != nil || last.Event.Type != "collaboration.created" || last.Event.Collaboration.ID != g.ID {
		t.Fatal("policy fixture must rewrite only its own final creation record", err)
	}
	last.Event.Collaboration.PublishedReportContext = policy
	last.Digest, err = digestRecord(last)
	if err != nil {
		t.Fatal(err)
	}
	lines[len(lines)-1], _ = json.Marshal(last)
	if err = os.WriteFile(path, append(bytes.Join(lines, []byte("\n")), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("historical policy replay", version, err)
	}
	t.Cleanup(func() { reopened.Close() })
	return reopened, groupState(t, reopened, g.ID)
}

func groupBudgetStage(t *testing.T, s *Store, g Collaboration, stage int) Work {
	t.Helper()
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	current := groupState(t, s, g.ID)
	if len(current.RequestIDs) != stage || current.Status != "running" {
		t.Fatalf("required peer stage %d missing: status=%s reason=%s stages=%d", stage, current.Status, current.Reason, len(current.RequestIDs))
	}
	w, err := s.Get(current.RequestIDs[stage-1])
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range current.RequestIDs[:stage-1] {
		found := false
		for _, source := range w.Context.Sources {
			found = found || source.RequestID == id
		}
		if !found {
			t.Fatalf("required peer result missing from stage %d", stage)
		}
	}
	return w
}

func TestGroupContextBudgetNearLimitReportPriorResultsContinue(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	source, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "group-budget-report-source", Text: "Completed near-limit private report", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, source, strings.Repeat("Report evidence sentence. ", 260))
	delivery := reportSubmission(c, "group-budget-report-delivery")
	delivery.Text = "@Meiiie gửi báo cáo mới nhất ở cuộc trò chuyện riêng vào kênh này"
	if _, _, err = s.DeliverReport(delivery); err != nil {
		t.Fatal(err)
	}
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare the published report", MaxStages: 3, ClientNonce: "group-budget-report-intake"})
	if err != nil || len(g.PublishedReportContext.Pins) != 1 {
		t.Fatal("near-limit group fixture", err)
	}
	if g.PublishedReportContext.Version != 3 {
		t.Fatal("new group did not select required-peer-first policy")
	}
	s.ConfigureContextAdmission(true)
	for stage := 1; stage <= 3; stage++ {
		w := groupBudgetStage(t, s, g, stage)
		raw, _ := json.Marshal(struct {
			Sources []ContextSource
			Reports []PublishedReportPin
		}{w.Context.Sources, w.Context.PublishedReports})
		if len(raw) > 8000 || len(w.Context.Sources)+len(w.Context.PublishedReports) > 8 {
			t.Fatal("context envelope expanded")
		}
		if stage == 1 && !reflect.DeepEqual(w.Context.PublishedReports, g.PublishedReportContext.Pins) || stage > 1 && (len(w.Context.PublishedReports) != 0 || w.Context.Omitted != 1) {
			t.Fatal("whole frozen report selection/omission did not preserve peer priority", stage)
		}
		if w.RuntimeContext == nil || w.RuntimeContext.Version != 1 || !boundedRuntimePrompt(w) {
			t.Fatal("existing Core manifest or prompt bound changed")
		}
		frame, err := MarshalRuntimeLaunch(w, nil)
		var launch map[string]any
		if err != nil || json.Unmarshal(frame, &launch) != nil || len(launch) != 7 {
			t.Fatal("seven-field Core launch contract changed", err)
		}
		for _, source := range w.Context.Sources {
			prior, _ := s.Get(source.RequestID)
			if source.Text != prior.Text || source.Reply != prior.Reply || !reflect.DeepEqual(source.Artifacts, prior.Artifacts) {
				t.Fatal("required peer result was truncated or rewritten")
			}
		}
		t.Logf("stage=%d sources=%d reports=%d omitted=%d envelopeBytes=%d pinSHA256=%s", stage, len(w.Context.Sources), len(w.Context.PublishedReports), w.Context.Omitted, len(raw), w.Context.SHA256)
		groupBudgetFinish(t, s, w, strings.Repeat(fmt.Sprintf("Proposal %d: verify source observations. ", stage), 15))
	}
	if err = s.AdvanceCollaborations(); err != nil || groupState(t, s, g.ID).Status != "completed" {
		t.Fatal("bounded near-limit group did not complete", err)
	}
}

func groupBudgetFinish(t *testing.T, s *Store, w Work, reply string) Work {
	t.Helper()
	w = admitPublished(t, s, w)
	for _, p := range w.Context.PublishedReports {
		for _, file := range p.Files {
			read, err := s.ExecuteTool(w.RequestID, w.Epoch, publishedReadFrame(w, p.DeliveryID, file.Artifact.ID, 0, len(file.Excerpt)))
			if err != nil || bytes.Contains(read, []byte("isError")) {
				t.Fatal("selected frozen report file reader was lost", err)
			}
		}
	}
	frame := invocation("Final bounded group result")
	if w.Core.ContextAdmission != nil {
		var call map[string]any
		json.Unmarshal(frame, &call)
		call["_meta"].(map[string]any)["neko.contextAdmission"] = w.Core.ContextAdmission
		frame, _ = json.Marshal(call)
	}
	result, err := s.ExecuteTool(w.RequestID, w.Epoch, frame)
	if err != nil || (w.Collaboration.Stage < 3 && !bytes.Contains(result, []byte("isError"))) || (w.Collaboration.Stage == 3 && bytes.Contains(result, []byte("isError"))) {
		t.Fatalf("final-only writer contract changed: stage=%d error=%v result=%s", w.Collaboration.Stage, err, result)
	}
	if err = s.AppendText(w.RequestID, w.Epoch, reply); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	return w
}

func TestGroupContextBudgetPeerResultsWithHistoryDisabled(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := sharedChannel(t, s, "History disabled group", "group-budget-history-channel")
	ambient, _, err := s.AcceptScoped(Input{BotID: "bot-davin", ClientNonce: "group-budget-history-ambient", Text: "Unrelated shared history", Lane: "foreground"}, c.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, ambient, "AMBIENT_HISTORY_EXCLUSION_SENTINEL")
	for _, id := range []string{"bot-meiiie", "bot-davin"} {
		if _, _, err = s.SetBotProfile(id, BotProfileInput{Instructions: "Keep responses clear", IncludeHistory: false, ClientNonce: "group-budget-history-profile-" + id}); err != nil {
			t.Fatal(err)
		}
	}
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare bounded plans", MaxStages: 3, ClientNonce: "group-budget-history-intake"})
	if err != nil {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	for stage := 1; stage <= 3; stage++ {
		w := groupBudgetStage(t, s, g, stage)
		if w.Context.Profile == nil || w.Context.Profile.IncludeHistory || strings.Contains(w.RuntimePrompt(), "AMBIENT_HISTORY_EXCLUSION_SENTINEL") {
			t.Fatal("history preference or ambient exclusion changed")
		}
		if w.RuntimeContext == nil || w.RuntimeContext.Version != 1 || !manifestValid(*w.RuntimeContext) {
			t.Fatal("required peers with history disabled failed existing manifest admission")
		}
		peerSources := 0
		for _, source := range w.RuntimeContext.Sources {
			if strings.HasPrefix(source.ID, "history:") {
				peerSources++
			}
		}
		if peerSources != stage-1 {
			t.Fatal("required dependency count changed in canonical admitted manifest")
		}
		groupBudgetFinish(t, s, w, fmt.Sprintf("Required explicit group peer result %d", stage))
	}
	if err = s.AdvanceCollaborations(); err != nil || groupState(t, s, g.ID).Status != "completed" {
		t.Fatal("history-disabled explicit group did not complete", err)
	}
}

func groupBudgetFileFixture(t *testing.T) (*Store, Collaboration, Work) {
	t.Helper()
	s, c, _ := reportFixture(t, false)
	source, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "group-budget-file-source", Text: "Completed near-limit report with selected output", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	source = startPermissionWork(t, s, source)
	if _, err = s.ExecuteTool(source.RequestID, source.Epoch, invocation("Selected report file bytes")); err != nil {
		t.Fatal(err)
	}
	if err = s.AppendText(source.RequestID, source.Epoch, strings.Repeat("Report evidence sentence. ", 240)); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(source.RequestID, source.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	source, _ = s.Get(source.RequestID)
	delivery := reportSubmission(c, "group-budget-file-delivery")
	delivery.Text = "@Meiiie gửi báo cáo mới nhất ở cuộc trò chuyện riêng vào kênh này"
	if _, _, err = s.DeliverReport(delivery); err != nil {
		t.Fatal(err)
	}
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare published report and selected file", MaxStages: 3, ClientNonce: "group-budget-file-intake"})
	if err != nil || len(g.PublishedReportContext.Pins) != 1 || len(g.PublishedReportContext.Pins[0].Files) != 1 {
		t.Fatal("near-limit selected file fixture", err)
	}
	w := groupBudgetStage(t, s, g, 1)
	if !reflect.DeepEqual(w.Context.PublishedReports, g.PublishedReportContext.Pins) {
		t.Fatal("stage 1 omitted original dependency")
	}
	groupBudgetFinish(t, s, w, strings.Repeat("Proposal: verify original report authority. ", 15))
	return s, g, source
}

func TestGroupContextBudgetOmittedReportKeepsOriginalAuthority(t *testing.T) {
	for _, fence := range []string{"source", "destination", "reader"} {
		t.Run(fence, func(t *testing.T) {
			s, g, source := groupBudgetFileFixture(t)
			w := groupBudgetStage(t, s, g, 2)
			if len(w.Context.PublishedReports) != 0 || len(w.Context.Sources) != 1 || !s.contextAllowed(w) {
				t.Fatal("peer priority did not omit whole oversized report")
			}
			original := copyWork(w).Context
			switch fence {
			case "source":
				_, _, err := s.SetSourceAccess(AccessInput{RequestID: source.RequestID, Granted: false, ClientNonce: "group-budget-original-revoke"})
				if err != nil {
					t.Fatal(err)
				}
			case "destination":
				c, _ := s.channel(g.ChannelID)
				if _, _, err := s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, BotIDs: []string{"bot-meiiie"}, ExpectedRevision: c.Revision, ClientNonce: "group-budget-audience-revoke"}); err != nil {
					t.Fatal(err)
				}
			case "reader":
				if _, _, err := s.ControlPlugin(PluginInput{Version: 1, ID: "neko-published-reports", Action: "disable", ClientNonce: "group-budget-reader-revoke"}); err != nil {
					t.Fatal(err)
				}
			}
			if s.contextAllowed(w) {
				t.Fatal("omitted report lost recursive original authority fence", fence)
			}
			if _, err := s.Claim(w.RequestID, w.Epoch); !errors.Is(err, ErrConflict) {
				t.Fatal("revoked omitted report remained launchable", err)
			}
			if err := s.AdvanceCollaborations(); err != nil {
				t.Fatal(err)
			}
			current := groupState(t, s, g.ID)
			stored, _ := s.Get(w.RequestID)
			if current.Status != "suspended" || len(current.RequestIDs) != 2 || !reflect.DeepEqual(stored.Context, original) || !reflect.DeepEqual(current.PublishedReportContext, g.PublishedReportContext) {
				t.Fatal("revocation changed frozen selection or expanded stages")
			}
			switch fence {
			case "source":
				if _, _, err := s.SetSourceAccess(AccessInput{RequestID: source.RequestID, Granted: true, ExpectedRevision: 1, ClientNonce: "group-budget-original-regrant"}); err != nil {
					t.Fatal(err)
				}
			case "destination":
				c, _ := s.channel(g.ChannelID)
				if _, _, err := s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, BotIDs: []string{"bot-meiiie", "bot-davin"}, ExpectedRevision: c.Revision, ClientNonce: "group-budget-audience-regrant"}); err != nil {
					t.Fatal(err)
				}
			case "reader":
				if _, _, err := s.ControlPlugin(PluginInput{Version: 1, ID: "neko-published-reports", Action: "enable", ExpectedRevision: 1, ClientNonce: "group-budget-reader-reenable"}); err != nil {
					t.Fatal(err)
				}
			}
			if s.contextAllowed(stored) || s.validPluginWork(stored) {
				t.Fatal("regrant rebound original omitted-report authority or plugin policy")
			}
		})
	}
}

func TestGroupContextBudgetConcurrentAdvanceAndCancelStayScoped(t *testing.T) {
	s, g, _ := groupBudgetFileFixture(t)
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := s.AdvanceCollaborations(); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	current := groupState(t, s, g.ID)
	if len(current.RequestIDs) != 2 {
		t.Fatal("concurrent advance duplicated a stage")
	}
	w, _ := s.Get(current.RequestIDs[1])
	in := CollaborationCancelInput{ExpectedRevision: current.Revision, ClientNonce: "group-budget-concurrent-cancel"}
	if _, _, err := s.CancelCollaboration(g.ID, in); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(w.RequestID, w.Epoch); !errors.Is(err, ErrConflict) || len(groupState(t, s, g.ID).RequestIDs) != 2 {
		t.Fatal("cancelled group resumed omitted context or expanded stages", err)
	}
	if _, duplicate, err := s.CancelCollaboration(g.ID, in); err != nil || !duplicate {
		t.Fatal("cancel nonce receipt changed", err)
	}
}

func TestGroupContextBudgetReplayStableSelectionAndNonce(t *testing.T) {
	s, g, _ := groupBudgetFileFixture(t)
	w := groupBudgetStage(t, s, g, 2)
	original := copyWork(w)
	before := groupState(t, s, g.ID)
	dir := s.dir
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		raw, _ := os.ReadFile(filepath.Join(dir, "transactions.jsonl"))
		lines := bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n"))
		probe := t.TempDir()
		var priorState *Store
		for i := range lines {
			os.WriteFile(filepath.Join(probe, "transactions.jsonl"), append(bytes.Join(lines[:i+1], []byte("\n")), '\n'), 0600)
			copy, failure := Open(probe)
			if failure != nil {
				var record record
				json.Unmarshal(lines[i], &record)
				t.Logf("first failing journal record sequence=%d type=%s", record.Event.Sequence, record.Event.Type)
				if priorState != nil && record.Event.Type == "work.accepted" {
					w := record.Event.Work
					t.Logf("replay guards: profile=%t runtime=%t plugins=%t context=%t group=%t publication=%t", priorState.validProfileIntake(w), priorState.validNewRuntimeContext(w), priorState.validPluginWork(w), reflect.DeepEqual(w.Context, priorState.captureContextVersion(w, w.Context.Version)), priorState.validCollaborationWork(w), priorState.validPublication(w))
				}
				break
			}
			copy.Close()
			priorState = copy
		}
		t.Fatal("V3 group replay", err)
	}
	defer func() { reopened.Close() }()
	stored, err := reopened.Get(w.RequestID)
	if err != nil || !reflect.DeepEqual(original.Context, stored.Context) || stored.Context.SHA256 != original.Context.SHA256 || !reflect.DeepEqual(before, groupState(t, reopened, g.ID)) {
		t.Fatal("V3 replay reselected frozen context or stage graph", err)
	}
	got, duplicate, err := reopened.CreateCollaboration(g.CollaborationInput)
	if err != nil || !duplicate || !reflect.DeepEqual(got, before) {
		t.Fatal("V3 group original nonce changed pinned receipt", err)
	}
	for restart := 2; restart <= 3; restart++ {
		reopened.Close()
		reopened, err = Open(dir)
		if err != nil {
			t.Fatal("V3 repeated restart", restart, err)
		}
		stored, err = reopened.Get(w.RequestID)
		got, duplicate, retryErr := reopened.CreateCollaboration(g.CollaborationInput)
		if err != nil || retryErr != nil || !duplicate || !reflect.DeepEqual(got, before) || !reflect.DeepEqual(stored.Context, original.Context) || stored.Context.SHA256 != original.Context.SHA256 {
			t.Fatal("V3 repeated restart changed original pin/nonce receipt", restart, err, retryErr)
		}
	}
	changed := g.CollaborationInput
	changed.Goal += " changed"
	if _, _, err = reopened.CreateCollaboration(changed); !errors.Is(err, ErrConflict) {
		t.Fatal("same nonce accepted changed group intent", err)
	}
	changed = g.CollaborationInput
	changed.ClientNonce = "group-budget-new-owner-intake"
	newGroup, duplicate, err := reopened.CreateCollaboration(changed)
	if err != nil || duplicate || newGroup.ID == g.ID || newGroup.PublishedReportContext.Version != 3 {
		t.Fatal("new nonce did not create independent V3 owner intent", err)
	}
	stored = admitPublished(t, reopened, stored)
	p := g.PublishedReportContext.Pins[0]
	read, err := reopened.ExecuteTool(stored.RequestID, stored.Epoch, publishedReadFrame(stored, p.DeliveryID, p.Artifacts[0].ID, 0, len(p.Files[0].Excerpt)))
	if err == nil && !bytes.Contains(read, []byte("isError")) {
		t.Fatal("whole omitted report regained a direct file reader")
	}
	if bytes.Contains(actionCapabilities(stored), []byte("report.read_artifact")) {
		t.Fatal("whole omitted report regained discoverable reader capability")
	}
	t.Logf("replayPinSHA256=%s frozenReportPins=%d selectedReportPins=%d omitted=%d", stored.Context.SHA256, len(g.PublishedReportContext.Pins), len(stored.Context.PublishedReports), stored.Context.Omitted)
}

func TestGroupContextBudgetEscapedRequiredResultKeepsOwnerGate(t *testing.T) {
	s, g, _, _ := collaborationFixture(t)
	w := groupBudgetStage(t, s, g, 1)
	reply := strings.Repeat("<>&", 500)
	w = completeChannelWork(t, s, w, reply)
	encoded := groupContextEnvelopeBytes([]ContextSource{groupContextSource(w)}, nil)
	if len(reply) >= 8000 || encoded <= 8000 {
		t.Fatal("escaped required-source fixture does not distinguish raw and encoded bytes")
	}
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	current := groupState(t, s, g.ID)
	if current.Status != "needs_owner" || current.Reason != "stage_context_unavailable" || len(current.RequestIDs) != 1 {
		t.Fatal("encoded oversized required result bypassed owner gate")
	}
	t.Logf("rawReplyBytes=%d encodedRequiredEnvelopeBytes=%d stop=%s/%s", len(reply), encoded, current.Status, current.Reason)
}

func TestGroupContextBudgetPluginEligibilityRejectsForgedGroupBindings(t *testing.T) {
	s, g, _ := groupBudgetFileFixture(t)
	w := groupBudgetStage(t, s, g, 2)
	if !s.validPluginWork(w) {
		t.Fatal("positive frozen plugin policy control failed")
	}
	for _, mutate := range []func(*Work){
		func(w *Work) { w.Collaboration.ID = "collab-" + strings.Repeat("f", 32) },
		func(w *Work) { w.ToolPolicyVersion = 1 },
		func(w *Work) { w.PluginPins = nil },
		func(w *Work) { w.OwnerUserID = "foreign-owner" },
		func(w *Work) { w.ConversationID = "channel-" + strings.Repeat("f", 32) },
	} {
		forged := copyWork(w)
		mutate(&forged)
		if s.validPluginWork(forged) {
			t.Fatal("forged group identity or frozen policy gained reader eligibility")
		}
	}
	for _, field := range []string{"message", "epoch", "access", "version"} {
		forged := copyCollaboration(g)
		p := &forged.PublishedReportContext.Pins[0]
		switch field {
		case "message":
			p.Source.MessageID = "msg-" + strings.Repeat("f", 32)
		case "epoch":
			p.Source.Epoch++
		case "access":
			p.Source.AccessRevision++
		case "version":
			forged.PublishedReportContext.Version = 4
		}
		p.SHA256 = publishedReportDigest(*p)
		s.mu.Lock()
		original := s.collaborations[g.ID]
		s.collaborations[g.ID] = forged
		accepted := s.validPluginWork(w)
		s.collaborations[g.ID] = original
		s.mu.Unlock()
		if accepted {
			t.Fatal("forged frozen source/policy pin gained reader eligibility", field)
		}
	}
}

func TestGroupContextBudgetCreationCountsEnvelopeHeaders(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	source, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "group-budget-header-source", Text: "Completed near-limit report", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, source, strings.Repeat("x", 7340))
	in := reportSubmission(c, "group-budget-header-delivery")
	in.Text = "@Meiiie gửi báo cáo mới nhất ở cuộc trò chuyện riêng vào kênh này"
	delivery, _, err := s.DeliverReport(in)
	if err != nil {
		t.Fatal(err)
	}
	pins := []PublishedReportPin{s.pinPublishedReport(delivery)}
	raw, _ := json.Marshal(pins)
	encoded := groupContextEnvelopeBytes([]ContextSource{}, pins)
	if len(raw) > 8000 || encoded <= 8000 {
		t.Fatalf("header fixture did not cross exact envelope boundary: pins=%d encoded=%d", len(raw), encoded)
	}
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare only admitted complete reports", MaxStages: 3, ClientNonce: "group-budget-header-intake"})
	if err != nil || len(g.PublishedReportContext.Pins) != 0 || g.PublishedReportContext.Omitted != 1 {
		t.Fatal("creation froze a report that cannot fit stage 1 with envelope headers", err)
	}
	w := groupBudgetStage(t, s, g, 1)
	if len(w.Context.PublishedReports) != 0 || w.Context.Omitted != 1 || groupContextEnvelopeBytes(w.Context.Sources, w.Context.PublishedReports) > 8000 {
		t.Fatal("creation omission did not persist at stage 1")
	}
	t.Logf("rawPinsBytes=%d encodedReportEnvelopeBytes=%d frozenPins=%d omitted=%d", len(raw), encoded, len(g.PublishedReportContext.Pins), g.PublishedReportContext.Omitted)
}
