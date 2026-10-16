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
	"testing"
)

func TestGroupContextRepeatedPublicationKeepsPeerResult(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	var latest ReportDelivery
	for i := 0; i < 8; i++ {
		var err error
		if latest, _, err = s.DeliverReport(reportSubmission(c, fmt.Sprintf("group-context-repeat-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	in := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare the published result", MaxStages: 3, ClientNonce: "group-context-repeated-intake"}
	g, _, err := s.CreateCollaboration(in)
	if err != nil {
		t.Fatal(err)
	}
	if g.PublishedReportContext.Version != 3 || len(g.PublishedReportContext.Pins) != 1 || g.PublishedReportContext.Pins[0].DeliveryID != latest.ID || g.PublishedReportContext.Omitted != 7 {
		t.Fatal("repeated result did not retain latest exact public pin and omitted count")
	}
	groupContextComplete(t, s, g)
	dir := s.dir
	before := groupState(t, s, g.ID)
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("new group policy replay", err)
	}
	defer reopened.Close()
	if duplicate, retry, err := reopened.CreateCollaboration(in); err != nil || !retry || !reflect.DeepEqual(before, duplicate) {
		t.Fatal("group nonce retry changed pinned selection or stage map", err)
	}
}

func groupContextComplete(t *testing.T, s *Store, g Collaboration) {
	t.Helper()
	var err error
	for stage := 1; stage <= 3; stage++ {
		if err = s.AdvanceCollaborations(); err != nil {
			t.Fatal(err)
		}
		current := groupState(t, s, g.ID)
		if len(current.RequestIDs) != stage {
			t.Fatalf("stage %d missing after repeated publication: status=%s reason=%s stages=%d", stage, current.Status, current.Reason, len(current.RequestIDs))
		}
		w, err := s.Get(current.RequestIDs[stage-1])
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(w.RuntimePrompt(), "private-source-prompt-sentinel") {
			t.Fatal("private report request entered group context")
		}
		if !reflect.DeepEqual(w.Context.PublishedReports, g.PublishedReportContext.Pins) {
			t.Fatal("group report pins changed between stages")
		}
		raw, _ := json.Marshal(struct {
			Sources []ContextSource
			Reports []PublishedReportPin
		}{w.Context.Sources, w.Context.PublishedReports})
		if len(w.Context.Sources)+len(w.Context.PublishedReports) > 8 || len(raw) > 8000 || !boundedRuntimePrompt(w) {
			t.Fatal("group widened existing context envelope")
		}
		for _, id := range current.RequestIDs[:stage-1] {
			found := false
			for _, source := range w.Context.Sources {
				found = found || source.RequestID == id
			}
			if !found {
				t.Fatalf("stage %d lost a required peer result", stage)
			}
		}
		w = admitGroup(t, s, w)
		result, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("Final bounded group report"))
		if err != nil || (stage < 3 && !bytes.Contains(result, []byte("isError"))) || (stage == 3 && bytes.Contains(result, []byte("isError"))) {
			t.Fatal("group final-writer boundary changed", stage, err)
		}
		if err = s.AppendText(w.RequestID, w.Epoch, fmt.Sprintf("Bounded peer result %d", stage)); err != nil {
			t.Fatal(err)
		}
		if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	if current := groupState(t, s, g.ID); current.Status != "completed" {
		t.Fatalf("group failed to complete: status=%s reason=%s", current.Status, current.Reason)
	}
}

func TestGroupContextIndependentPublicationsReserveBothPeerSlots(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	var deliveries []ReportDelivery
	for i := 0; i < 8; i++ {
		w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: fmt.Sprintf("group-context-independent-source-%d", i), Text: "Completed report private-source-prompt-sentinel", Lane: "foreground"})
		if err != nil {
			t.Fatal(err)
		}
		completeChannelWork(t, s, w, "Identical public report bytes with independent source authority")
		in := reportSubmission(c, fmt.Sprintf("group-context-independent-delivery-%d", i))
		in.Text = "@Meiiie gửi báo cáo mới nhất ở cuộc trò chuyện riêng vào kênh này"
		d, _, err := s.DeliverReport(in)
		if err != nil || d.Status != "delivered" || d.Source.RequestID != w.RequestID {
			t.Fatal("independent public report fixture", err)
		}
		deliveries = append(deliveries, d)
	}
	in := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare independent published reports", MaxStages: 3, ClientNonce: "group-context-independent-intake"}
	g, _, err := s.CreateCollaboration(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.PublishedReportContext.Pins) != 6 || g.PublishedReportContext.Omitted != 2 {
		t.Fatal("independent source authority was collapsed or peer slots were not reserved")
	}
	for i, p := range g.PublishedReportContext.Pins {
		if p.DeliveryID != deliveries[i+2].ID || p.Source.RequestID != deliveries[i+2].Source.RequestID {
			t.Fatal("group did not retain newest six independent authority pins")
		}
	}
	later := reportSubmission(c, "group-context-independent-later")
	later.Text = "@Meiiie gửi báo cáo mới nhất ở cuộc trò chuyện riêng vào kênh này"
	if _, _, err = s.DeliverReport(later); err != nil {
		t.Fatal(err)
	}
	groupContextComplete(t, s, g)
}

func TestGroupContextFrozenFileSourceRevocationStopsGroup(t *testing.T) {
	s, c, source := reportFixture(t, true)
	for i := 0; i < 8; i++ {
		if _, _, err := s.DeliverReport(reportSubmission(c, fmt.Sprintf("group-context-file-repeat-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare the published file", MaxStages: 3, ClientNonce: "group-context-file-intake"})
	if err != nil || len(g.PublishedReportContext.Pins) != 1 || len(g.PublishedReportContext.Pins[0].Files) != 1 {
		t.Fatal("file publication freeze", err)
	}
	_, w := advanceGroup(t, s, g.ID)
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: source.RequestID, Granted: false, ClientNonce: "group-context-file-revoke"}); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	current := groupState(t, s, g.ID)
	if current.Status != "suspended" || current.Reason != "authority_changed" || len(current.RequestIDs) != 1 || !reflect.DeepEqual(current.PublishedReportContext, g.PublishedReportContext) {
		t.Fatal("revocation expanded or refreshed group authority")
	}
	if _, err = s.Claim(w.RequestID, w.Epoch); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked frozen source remained launchable", err)
	}
	if _, _, err = s.ReadReportDeliveryArtifact(g.PublishedReportContext.Pins[0].DeliveryID, source.Artifacts[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked frozen file remained readable", err)
	}
}

func TestGroupContextLegacyPublicationPolicyReplayKeepsOriginalPins(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	for i := 0; i < 8; i++ {
		if _, _, err := s.DeliverReport(reportSubmission(c, fmt.Sprintf("group-context-legacy-repeat-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	in := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Legacy frozen report policy", MaxStages: 3, ClientNonce: "group-context-legacy-intake"}
	g, _, err := s.CreateCollaboration(in)
	if err != nil {
		t.Fatal(err)
	}
	legacy := s.capturePublishedReports(Work{BotID: g.CoordinatorBotID, OwnerUserID: g.OwnerUserID, SpaceID: g.SpaceID, ConversationID: g.ChannelID}, g.PublishedReportContext.AtSequence)
	dir := s.dir
	s.Close()
	path := filepath.Join(dir, "transactions.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n"))
	var last record
	if err = json.Unmarshal(lines[len(lines)-1], &last); err != nil {
		t.Fatal(err)
	}
	last.Event.Collaboration.PublishedReportContext = legacy
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
		t.Fatal("legacy group policy replay", err)
	}
	defer reopened.Close()
	got, duplicate, err := reopened.CreateCollaboration(in)
	if err != nil || !duplicate || got.PublishedReportContext.Version != 1 || got.PublishedReportContext.Omitted != 0 || len(got.PublishedReportContext.Pins) != 8 || !reflect.DeepEqual(legacy, got.PublishedReportContext) {
		t.Fatal("legacy retry reselected historical report pins", err)
	}
	reopened.Close()
	for _, version := range []int{0, 4} {
		last.Event.Collaboration.PublishedReportContext.Version = version
		last.Digest, _ = digestRecord(last)
		lines[len(lines)-1], _ = json.Marshal(last)
		if err = os.WriteFile(path, append(bytes.Join(lines, []byte("\n")), '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		if accepted, err := Open(dir); err == nil {
			accepted.Close()
			t.Fatalf("unsupported group report policy version %d replayed", version)
		}
	}
}

func TestGroupContextDifferentArtifactSelectionsRemainSeparate(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := sharedChannel(t, s, "Group artifact selections", "group-context-selection-channel")
	w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "group-context-selection-source", Text: "Write a private report with two output files", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	w = startPermissionWork(t, s, w)
	if _, err = s.ExecuteTool(w.RequestID, w.Epoch, invocation("First selected report output")); err != nil {
		t.Fatal(err)
	}
	var second map[string]any
	if err = json.Unmarshal(invocation("Second selected report output"), &second); err != nil {
		t.Fatal(err)
	}
	second["arguments"].(map[string]any)["arguments"].(map[string]any)["filename"] = "second-report.md"
	second["_meta"].(map[string]any)["neko.operation"].(map[string]any)["operationId"] = strings.Repeat("e", 32)
	frame, _ := json.Marshal(second)
	if _, err = s.ExecuteTool(w.RequestID, w.Epoch, frame); err != nil {
		t.Fatal(err)
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "Completed public report reply"); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	plain, _, err := s.DeliverReport(reportSubmission(c, "group-context-selection-plain"))
	if err != nil || len(plain.Artifacts) != 0 {
		t.Fatal("reply-only publication fixture", err)
	}
	files := reportSubmission(c, "group-context-selection-files")
	files.Text = "@Meiiie gửi báo cáo tệp ở cuộc trò chuyện riêng vào kênh này"
	withFiles, _, err := s.DeliverReport(files)
	if err != nil || len(withFiles.Artifacts) != 2 {
		t.Fatal("file selection publication fixture", err)
	}
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare both public output selections", MaxStages: 3, ClientNonce: "group-context-selection-intake"})
	if err != nil || len(g.PublishedReportContext.Pins) != 2 || g.PublishedReportContext.Omitted != 0 {
		t.Fatal("divergent public artifact selections collapsed", err)
	}
	if g.PublishedReportContext.Pins[0].DeliveryID != plain.ID || len(g.PublishedReportContext.Pins[0].Files) != 0 || g.PublishedReportContext.Pins[1].DeliveryID != withFiles.ID || len(g.PublishedReportContext.Pins[1].Files) != 2 {
		t.Fatal("public artifact selection pins changed")
	}
	groupContextComplete(t, s, g)
}

func TestGroupContextOversizedPeerResultStillStopsAtByteBudget(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	if _, _, err := s.DeliverReport(reportSubmission(c, "group-context-byte-delivery")); err != nil {
		t.Fatal(err)
	}
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare the published result", MaxStages: 3, ClientNonce: "group-context-byte-intake"})
	if err != nil {
		t.Fatal(err)
	}
	_, w := advanceGroup(t, s, g.ID)
	completeChannelWork(t, s, w, strings.Repeat("x", 8000))
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	current := groupState(t, s, g.ID)
	if current.Status != "needs_owner" || current.Reason != "stage_context_unavailable" || len(current.RequestIDs) != 1 {
		t.Fatal("group bypassed existing byte budget for oversized peer context")
	}
}

func TestGroupContextVersion2NearLimitReportRetainsByteBudgetOwnerGate(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	source, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "group-context-near-limit-source", Text: "Completed near-limit private report", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, source, strings.Repeat("Report evidence sentence. ", 260))
	delivery := reportSubmission(c, "group-context-near-limit-delivery")
	delivery.Text = "@Meiiie gửi báo cáo mới nhất ở cuộc trò chuyện riêng vào kênh này"
	if _, _, err = s.DeliverReport(delivery); err != nil {
		t.Fatal(err)
	}
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare the published report", MaxStages: 3, ClientNonce: "group-context-near-limit-intake"})
	if err != nil || len(g.PublishedReportContext.Pins) != 1 {
		t.Fatal("near-limit frozen report fixture", err)
	}
	s, g = groupContextReplayPolicy(t, s, g, 2)
	reportBytes, _ := json.Marshal(g.PublishedReportContext.Pins)
	if len(reportBytes) < 7000 || len(reportBytes) > 8000 {
		t.Fatalf("report fixture is not near the existing byte limit: %d", len(reportBytes))
	}
	_, w := advanceGroup(t, s, g.ID)
	proposal := strings.Repeat("Proposal: verify source observations. ", 15)
	w = completeChannelWork(t, s, w, proposal)
	required, _ := json.Marshal(struct {
		Sources []ContextSource
		Reports []PublishedReportPin
	}{[]ContextSource{{RequestID: w.RequestID, MessageID: w.SourceMessageID, Sequence: w.CreatedSequence, Epoch: w.Epoch, Text: w.Text, Reply: w.Reply, AccessRevision: accessRevision(w), BotID: w.BotID}}, g.PublishedReportContext.Pins})
	if len(required) <= 8000 {
		t.Fatalf("near-limit report and bounded proposal did not exceed combined budget: %d", len(required))
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	current := groupState(t, s, g.ID)
	if current.Status != "needs_owner" || current.Reason != "stage_context_unavailable" || len(current.RequestIDs) != 1 {
		t.Fatal("near-limit group bypassed existing byte-budget owner gate")
	}
	t.Logf("frozenReportBytes=%d proposalBytes=%d requiredCombinedBytes=%d stop=%s/%s", len(reportBytes), len(proposal), len(required), current.Status, current.Reason)
}
