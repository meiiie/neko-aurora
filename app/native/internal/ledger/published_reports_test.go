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
	"time"

	"nekobot.local/native/internal/protocol"
	"nekobot.local/native/internal/syncstream"
)

func TestPublishedReportCrossBotIntakeBaseline(t *testing.T) {
	s, c, source := reportFixture(t, true)
	if _, _, err := s.DeliverReport(reportSubmission(c, "published-context-delivery")); err != nil {
		t.Fatal(err)
	}
	w, _, err := s.AcceptScoped(Input{BotID: "bot-davin", ClientNonce: "published-context-davin", Text: "Summarize the published report", Lane: "foreground"}, c.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.RuntimePrompt(), source.Reply) {
		t.Fatal("published report result missing from destination colleague intake")
	}
	if strings.Contains(w.RuntimePrompt(), "private-source-prompt-sentinel") {
		t.Fatal("source private prompt imported")
	}
}

func admitPublished(t *testing.T, s *Store, w Work) Work {
	t.Helper()
	w, err := s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{CoreSHA256: "97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb", SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"authorityId":"fixture"}`)}
	if w.ContextProtocolVersion == 1 {
		b.ContextAdmission = &ContextAdmission{Version: 1, SessionID: b.SessionID, ContextPinSHA256: contextHash(w), ManifestSHA256: w.RuntimeContextSHA256, SourceIDs: []string{strings.Repeat("f", 64)}, Task: b.Task, Execution: b.Execution}
	}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	return w
}

func publishedReadFrame(w Work, delivery, artifact string, offset, size int) json.RawMessage {
	meta := map[string]any{"neko.operation": map[string]any{"version": 1, "operationId": strings.Repeat("d", 32), "sessionId": w.Core.SessionID, "taskId": strings.Repeat("b", 32), "root": "/fixture", "activationEpoch": 1, "activationId": strings.Repeat("c", 32)}}
	if w.Core.ContextAdmission != nil {
		meta["neko.contextAdmission"] = w.Core.ContextAdmission
	}
	raw, _ := json.Marshal(map[string]any{"name": "execute_tool", "arguments": map[string]any{"name": "report.read_artifact", "arguments": map[string]any{"deliveryId": delivery, "artifactId": artifact, "offset": offset, "bytes": size}}, "_meta": meta})
	return raw
}

func publishedWork(t *testing.T, s *Store, c Channel, nonce string) Work {
	t.Helper()
	w, _, err := s.AcceptScoped(Input{BotID: "bot-davin", ClientNonce: nonce, Text: "Summarize the published report", Lane: "foreground"}, c.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestPublishedReportReaderExactRawBytesAdmissionAndScope(t *testing.T) {
	s, c, source := reportFixture(t, true)
	d, _, err := s.DeliverReport(reportSubmission(c, "published-file-delivery"))
	if err != nil {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	w := publishedWork(t, s, c, "published-file-consumer")
	if w.Context.Version != 5 || w.ToolPolicyVersion != 2 || len(w.Context.PublishedReports) != 1 || len(w.Context.Sources) != 0 || !pinnedTool(w, "report.read_artifact") {
		t.Fatal("published intake/policy")
	}
	if len(w.Context.PublishedReports[0].Files) != 1 || w.Context.PublishedReports[0].Source.Artifacts != nil {
		t.Fatal("file-only output pin")
	}
	frame, err := MarshalRuntimeLaunch(w, nil)
	if err != nil {
		t.Fatal(err)
	}
	var launch map[string]any
	if json.Unmarshal(frame, &launch) != nil || len(launch) != 7 {
		t.Fatal("Core launch protocol changed")
	}
	if bytes.Contains(frame, []byte("private-source-prompt-sentinel")) {
		t.Fatal("private source reached launch")
	}
	w = admitPublished(t, s, w)
	before := s.Snapshot().Sequence
	result, err := s.ExecuteTool(w.RequestID, w.Epoch, publishedReadFrame(w, d.ID, source.Artifacts[0].ID, 0, 4000))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if json.Unmarshal(result, &response) != nil || len(response.Content) != 1 || response.Meta["neko.contextSource"] == nil {
		t.Fatal("missing exact context envelope", string(result))
	}
	var body map[string]any
	if json.Unmarshal([]byte(response.Content[0].Text), &body) != nil || len(body) != 7 || body["fileId"] != source.Artifacts[0].ID || body["sha256"] != source.Artifacts[0].SHA256 || body["text"] != "Completed report file. result-only-sentinel" {
		t.Fatal("original seven-field byte read", body)
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("read created mutation")
	}
	for _, bad := range []struct {
		delivery, artifact string
		offset, size       int
	}{{"report-delivery-" + strings.Repeat("b", 32), source.Artifacts[0].ID, 0, 4000}, {d.ID, strings.Repeat("c", 64), 0, 4000}, {d.ID, source.Artifacts[0].ID, 1, 8}, {d.ID, source.Artifacts[0].ID, 0, 4001}} {
		out, err := s.ExecuteTool(w.RequestID, w.Epoch, publishedReadFrame(w, bad.delivery, bad.artifact, bad.offset, bad.size))
		if err == nil && !bytes.Contains(out, []byte("isError")) {
			t.Fatal("unbound/unattested range read")
		}
	}
}

func TestPublishedReportExplicitPluginOmissionDoesNotReadFiles(t *testing.T) {
	s, c, _ := reportFixture(t, true)
	if _, _, err := s.DeliverReport(reportSubmission(c, "published-explicit-delivery")); err != nil {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	selection := []PluginSelection{{ID: "neko-artifacts"}}
	w, _, err := s.AcceptProduct(Input{BotID: "bot-davin", ClientNonce: "published-reader-omitted", Text: "Describe the report reply", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, PluginSelection: &selection})
	if err != nil {
		t.Fatal(err)
	}
	if pinnedTool(w, "report.read_artifact") || hasPublishedArtifacts(w) || !strings.Contains(w.RuntimePrompt(), "result-only-sentinel") {
		t.Fatal("selection not preserved")
	}
	for _, p := range w.Context.PublishedReports {
		if len(p.Files) != 0 || len(p.Artifacts) != 0 {
			t.Fatal("omitted reader admitted file bytes")
		}
	}
	for _, source := range w.RuntimeContext.Sources {
		if source.Kind == "retrieved" {
			t.Fatal("omitted reader admitted retrieved span")
		}
	}
	dir := s.dir
	s.Close()
	re, err := Open(dir)
	if err != nil {
		t.Fatal("omitted reader replay", err)
	}
	defer re.Close()
}

func TestPublishedReportRevocationFencesQueuedRunningAndDerivedOutputs(t *testing.T) {
	for _, fence := range []string{"source", "destination", "reader"} {
		t.Run(fence, func(t *testing.T) {
			s, c, source := reportFixture(t, true)
			d, _, err := s.DeliverReport(reportSubmission(c, "published-revoke-delivery"))
			if err != nil {
				t.Fatal(err)
			}
			queued := publishedWork(t, s, c, "published-revoke-queued")
			running := admitPublished(t, s, publishedWork(t, s, c, "published-revoke-running"))
			completed := admitPublished(t, s, publishedWork(t, s, c, "published-revoke-completed"))
			if err = s.AppendText(completed.RequestID, completed.Epoch, "DERIVED-REPORT-SUMMARY-CANARY"); err != nil {
				t.Fatal(err)
			}
			if _, err = s.ExecuteTool(completed.RequestID, completed.Epoch, invocation("DERIVED-REPORT-FILE-CANARY")); err != nil {
				t.Fatal(err)
			}
			if err = s.Finish(completed.RequestID, completed.Epoch, "completed", ""); err != nil {
				t.Fatal(err)
			}
			completed, _ = s.Get(completed.RequestID)
			child := publishedWork(t, s, c, "published-revoke-transitive")
			scope := LocalSyncScope{BotID: "bot-davin", ChannelID: c.ID}
			sync, err := s.LocalSync(scope, "")
			if err != nil {
				t.Fatal(err)
			}
			cursor := syncstream.Cursor{Version: 1, InstanceID: sync.InstanceID, ScopeID: sync.ScopeID, Sequence: sync.To}
			token, _ := cursor.Encode()
			switch fence {
			case "source":
				_, _, err = s.SetSourceAccess(AccessInput{RequestID: source.RequestID, Granted: false, ClientNonce: "published-revoke-source"})
			case "destination":
				_, _, err = s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, BotIDs: []string{"bot-meiiie"}, ExpectedRevision: c.Revision, ClientNonce: "published-revoke-destination"})
			case "reader":
				_, _, err = s.ControlPlugin(PluginInput{Version: 1, ID: "neko-published-reports", Action: "disable", ClientNonce: "published-revoke-reader"})
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(queued.RequestID, queued.Epoch); !errors.Is(err, ErrConflict) {
				t.Fatal("queued source still launchable", err)
			}
			if err = s.AppendText(running.RequestID, running.Epoch, "late private-derived reply"); !errors.Is(err, ErrConflict) {
				t.Fatal("late running text", err)
			}
			if _, err = s.ExecuteTool(running.RequestID, running.Epoch, publishedReadFrame(running, d.ID, source.Artifacts[0].ID, 0, 4000)); !errors.Is(err, ErrConflict) {
				t.Fatal("late read", err)
			}
			if s.contextAllowed(child) {
				t.Fatal("transitive context summary survived revoke")
			}
			for _, projection := range []any{s.PublicWork(completed), s.PublicSnapshot().Work, func() []Event { events, _ := s.Events(0); return events }()} {
				raw, _ := json.Marshal(projection)
				if bytes.Contains(raw, []byte("DERIVED-REPORT-")) {
					t.Fatal("stale derived output projected")
				}
			}
			if _, _, err = s.ReadArtifact(completed.Artifacts[0].ID); !errors.Is(err, ErrNotFound) {
				t.Fatal("derived file fallback after revoke", err)
			}
			if exported, err := s.ExportSharedHistory(c.ID, "", "json"); err == nil && bytes.Contains(exported, []byte("DERIVED-REPORT-")) {
				t.Fatal("stale summary export")
			}
			if _, err = s.LocalSync(scope, token); err == nil {
				t.Fatal("old derived sync cursor retained output")
			}
			dir := s.dir
			s.Close()
			re, err := Open(dir)
			if err != nil {
				t.Fatal("revoked report context journal", err)
			}
			defer re.Close()
			if w, err := re.Get(completed.RequestID); err != nil || re.contextAllowed(w) {
				t.Fatal("replay revived derived report", err)
			}
		})
	}
}

func TestPublishedReportFiniteGroupPinsFreezeAndAllStagesRead(t *testing.T) {
	s, c, _ := reportFixture(t, true)
	d, _, err := s.DeliverReport(reportSubmission(c, "published-group-delivery"))
	if err != nil {
		t.Fatal(err)
	}
	in := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare this published report and return a file", MaxStages: 3, ClientNonce: "published-group-intake"}
	g, _, err := s.CreateCollaboration(in)
	if err != nil {
		t.Fatal(err)
	}
	if g.PublishedReportContext == nil || len(g.PublishedReportContext.Pins) != 1 {
		t.Fatal("group omitted intake publication")
	}
	if _, _, err = s.DeliverReport(reportSubmission(c, "published-after-group-intake")); err != nil {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	for stage := 1; stage <= 3; stage++ {
		_, w := advanceGroup(t, s, g.ID)
		if len(w.Context.PublishedReports) != 1 || w.Context.PublishedReports[0].DeliveryID != d.ID {
			t.Fatal("group gained newer publication", stage)
		}
		w = admitPublished(t, s, w)
		if out, err := s.ExecuteTool(w.RequestID, w.Epoch, publishedReadFrame(w, d.ID, d.Artifacts[0].ID, 0, 4000)); err != nil || bytes.Contains(out, []byte("isError")) {
			t.Fatal("group read", stage, err)
		}
		if stage == 3 {
			if _, err = s.ExecuteTool(w.RequestID, w.Epoch, invocation("Final published report comparison")); err != nil {
				t.Fatal(err)
			}
		} else if strings.Contains(string(actionCapabilities(w)), "artifact.write_text") {
			t.Fatal("intermediate group writer")
		}
		if err = s.AppendText(w.RequestID, w.Epoch, fmt.Sprintf("Stage %d derived report result", stage)); err != nil {
			t.Fatal(err)
		}
		if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	if groupState(t, s, g.ID).Status != "completed" {
		t.Fatal("group did not stop at three")
	}
	dir := s.dir
	s.Close()
	re, err := Open(dir)
	if err != nil {
		t.Fatal("group report pin replay", err)
	}
	defer re.Close()
}

func TestPublishedReportRepeatPublicationDeduplicatesFileIdentity(t *testing.T) {
	s, c, _ := reportFixture(t, true)
	first, _, err := s.DeliverReport(reportSubmission(c, "published-repeat-one"))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := s.DeliverReport(reportSubmission(c, "published-repeat-two"))
	if err != nil {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	w := publishedWork(t, s, c, "published-repeat-consumer")
	count := 0
	for _, p := range w.RuntimeContext.Sources {
		if p.Kind == "retrieved" && p.ID == first.Artifacts[0].ID {
			count++
			if p.Version != 1 {
				t.Fatal("byte source version changed")
			}
		}
	}
	if count != 1 || len(w.Context.PublishedReports) != 2 {
		t.Fatal("publication/file identity dedup")
	}
	w = admitPublished(t, s, w)
	for _, id := range []string{first.ID, second.ID} {
		if out, err := s.ExecuteTool(w.RequestID, w.Epoch, publishedReadFrame(w, id, first.Artifacts[0].ID, 0, 4000)); err != nil || bytes.Contains(out, []byte("isError")) {
			t.Fatal("exact duplicate delivery binding", err)
		}
	}
	dir := s.dir
	s.Close()
	re, err := Open(dir)
	if err != nil {
		t.Fatal("repeat replay", err)
	}
	defer re.Close()
}

func TestPublishedReportHistoricalContextAndPolicyVersionsRemainReplayable(t *testing.T) {
	for version := 1; version <= 4; version++ {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			s, c, _ := reportFixture(t, true)
			if _, _, err := s.DeliverReport(reportSubmission(c, "published-legacy-delivery")); err != nil {
				t.Fatal(err)
			}
			if version == 4 {
				if _, _, err := s.SetBotProfile("bot-davin", BotProfileInput{Instructions: "Historical profile", IncludeHistory: true, ClientNonce: "published-legacy-profile"}); err != nil {
					t.Fatal(err)
				}
			}
			req, _ := newID("req-")
			msg, _ := newID("msg-")
			w := Work{RequestID: req, SourceMessageID: msg, OwnerUserID: "local-owner", SpaceID: "local-space", BotID: "bot-davin", ConversationID: c.ID, ClientNonce: fmt.Sprintf("published-legacy-v%d", version), Text: "Historical context", Lane: "foreground", Status: "waiting_host", DesiredState: "running", Epoch: 1, BotConfigRevision: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), CreatedSequence: uint64(len(s.events) + 1), ToolPolicyVersion: 1}
			w.Publication = &Publication{Version: 1, ActorID: w.OwnerUserID, Platform: "nekobot-local", SourceChannelID: c.ID, SourceMessageID: msg, ReplyChannelID: c.ID, ReplyThreadID: "thread-" + msg, AudienceRevision: c.Revision}
			s.pinBotProfile(&w)
			w.PluginPins, _ = s.selectPlugins(w, nil)
			w.Context = s.captureContextVersion(w, version)
			if len(w.Context.PublishedReports) != 0 || pinnedTool(w, "report.read_artifact") {
				t.Fatal("legacy gained publication/reader")
			}
			e := Event{Sequence: w.CreatedSequence, Type: "work.accepted", Work: w}
			if !validReplay(s, e) {
				t.Fatal("legacy positive replay control")
			}
			if err := s.commit(e); err != nil {
				t.Fatal(err)
			}
			dir := s.dir
			s.Close()
			re, err := Open(dir)
			if err != nil {
				t.Fatal("historical replay", err)
			}
			defer re.Close()
			got, _ := re.Get(w.RequestID)
			if !reflect.DeepEqual(got.Context, w.Context) || got.ToolPolicyVersion != 1 {
				t.Fatal("historical pin bytes changed")
			}
		})
	}
}

func TestPublishedReportPinForgery(t *testing.T) {
	s, c, _ := reportFixture(t, true)
	if _, _, err := s.DeliverReport(reportSubmission(c, "published-forgery-delivery")); err != nil {
		t.Fatal(err)
	}
	w := publishedWork(t, s, c, "published-forgery-consumer")
	path := s.file.Name()
	s.Close()
	raw, _ := os.ReadFile(path)
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n"))
	var r record
	json.Unmarshal(lines[len(lines)-1], &r)
	r.Event.Work.Context.PublishedReports[0].Reply = "Forged published result"
	r.Event.Work.Context.PublishedReports[0].SHA256 = publishedReportDigest(r.Event.Work.Context.PublishedReports[0])
	pin := *r.Event.Work.Context
	pin.SHA256 = ""
	data, _ := json.Marshal(pin)
	r.Event.Work.Context.SHA256 = manifestHash(string(data))
	r.Digest, _ = digestRecord(r)
	lines[len(lines)-1], _ = json.Marshal(r)
	os.WriteFile(filepath.Join(s.dir, "transactions.jsonl"), append(bytes.Join(lines, []byte("\n")), '\n'), 0600)
	if re, err := Open(s.dir); !errors.Is(err, ErrCorrupt) {
		if re != nil {
			re.Close()
		}
		t.Fatal("rehashed publication pin forgery", err)
	}
	_ = w
}

func TestPublishedReportLargeResultOmittedAndPinRetryFrozen(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	long := strings.Repeat("Oversized report result canary. ", 500)
	completedPermissionSource(t, s, "published-large-source", long)
	in := reportSubmission(c, "published-large-delivery")
	in.Text = "@Meiiie gửi báo cáo mới nhất ở cuộc trò chuyện riêng vào kênh này"
	d, _, err := s.DeliverReport(in)
	if err != nil || d.Reply != long {
		t.Fatal("original large published result", err)
	}
	w := publishedWork(t, s, c, "published-large-consumer")
	if w.Context.Omitted != 1 || len(w.Context.PublishedReports) != 0 || strings.Contains(w.RuntimePrompt(), "Oversized report result canary") {
		t.Fatal("large result was partially imported or omission hidden")
	}
	dir := s.dir
	s.Close()
	re, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer re.Close()
	copy, dup, err := re.AcceptScoped(Input{BotID: "bot-davin", ClientNonce: w.ClientNonce, Text: w.Text, Lane: w.Lane}, c.ID, "", 0)
	if err != nil || !dup || !reflect.DeepEqual(copy.Context, w.Context) {
		t.Fatal("retry expanded omitted intake", err)
	}
}

func TestPublishedReportUnselectedPrivateArtifactNeverEntersConsumer(t *testing.T) {
	s, source := running(t, t.TempDir())
	defer s.Close()
	if _, err := s.ExecuteTool(source.RequestID, source.Epoch, invocation("Published file bytes")); err != nil {
		t.Fatal(err)
	}
	var frame map[string]any
	json.Unmarshal(invocation("UNSELECTED-PRIVATE-FILE-CANARY"), &frame)
	frame["_meta"].(map[string]any)["neko.operation"].(map[string]any)["operationId"] = strings.Repeat("e", 32)
	frame["arguments"].(map[string]any)["arguments"].(map[string]any)["filename"] = "private-notes-canary.txt"
	raw, _ := json.Marshal(frame)
	if _, err := s.ExecuteTool(source.RequestID, source.Epoch, raw); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendText(source.RequestID, source.Epoch, "Published report result"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(source.RequestID, source.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	source, _ = s.Get(source.RequestID)
	c := sharedChannel(t, s, "Published source canaries", "published-canary-channel")
	d, _, err := s.DeliverReport(reportSubmission(c, "published-canary-delivery"))
	if err != nil || len(d.Artifacts) != 1 {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	w := publishedWork(t, s, c, "published-canary-consumer")
	launch, err := MarshalRuntimeLaunch(w, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{source.Text, "private-notes-canary.txt", "UNSELECTED-PRIVATE-FILE-CANARY", source.Artifacts[1].ID} {
		if bytes.Contains(launch, []byte(canary)) {
			t.Fatal("unselected private source in published context")
		}
	}
	w = admitPublished(t, s, w)
	if out, err := s.ExecuteTool(w.RequestID, w.Epoch, publishedReadFrame(w, d.ID, source.Artifacts[1].ID, 0, 4000)); err == nil && !bytes.Contains(out, []byte("isError")) {
		t.Fatal("unselected artifact fallback")
	}
}

func TestPublishedReportReaderReenableDoesNotReviveOldPin(t *testing.T) {
	s, c, _ := reportFixture(t, true)
	if _, _, err := s.DeliverReport(reportSubmission(c, "published-reader-lifecycle")); err != nil {
		t.Fatal(err)
	}
	old := publishedWork(t, s, c, "published-reader-old-pin")
	if _, _, err := s.ControlPlugin(PluginInput{Version: 1, ID: "neko-published-reports", Action: "disable", ClientNonce: "published-reader-disable"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ControlPlugin(PluginInput{Version: 1, ID: "neko-published-reports", Action: "enable", ExpectedRevision: 1, ClientNonce: "published-reader-enable"}); err != nil {
		t.Fatal(err)
	}
	if s.contextAllowed(old) {
		t.Fatal("old selected reader revision revived")
	}
	newer := publishedWork(t, s, c, "published-reader-new-pin")
	if !s.contextAllowed(newer) || !pinnedTool(newer, "report.read_artifact") {
		t.Fatal("new explicit intake unavailable")
	}
	for _, p := range newer.PluginPins {
		if p.ID == "neko-published-reports" && p.Revision != 2 {
			t.Fatal("new reader revision not pinned")
		}
	}
}

func TestPublishedReportAdmissionCanonicalOrderAndExactAuthority(t *testing.T) {
	s, c, _ := reportFixture(t, true)
	if _, _, err := s.DeliverReport(reportSubmission(c, "published-canonical-delivery")); err != nil {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	w := admitPublished(t, s, publishedWork(t, s, c, "published-canonical-consumer"))
	raw := publishedReadFrame(w, w.Context.PublishedReports[0].DeliveryID, w.Context.PublishedReports[0].Artifacts[0].ID, 0, 4000)
	canonical, err := protocol.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	var frame struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if json.Unmarshal(canonical, &frame) != nil || !contextMetaMatches(w, frame.Meta) {
		t.Fatal("semantically equivalent admission fixture key order rejected")
	}
	for _, change := range []string{"task", "epoch", "source", "manifest", "number"} {
		c := *copyContextAdmission(w.Core.ContextAdmission)
		switch change {
		case "task":
			c.Task = json.RawMessage(`{"id":"cccccccccccccccccccccccccccccccc","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`)
		case "epoch":
			c.Task = json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":2,"activationId":"cccccccccccccccccccccccccccccccc"}`)
		case "source":
			c.SourceIDs[0] = strings.Repeat("e", 64)
		case "manifest":
			c.ManifestSHA256 = strings.Repeat("e", 64)
		case "number":
			c.Execution = json.RawMessage(`{"authorityId":"fixture","fence":9007199254740992}`)
		}
		data, _ := json.Marshal(c)
		meta := map[string]json.RawMessage{"neko.operation": frame.Meta["neko.operation"], "neko.contextAdmission": data}
		if contextMetaMatches(w, meta) {
			t.Fatal("changed admission authority accepted", change)
		}
	}
}
