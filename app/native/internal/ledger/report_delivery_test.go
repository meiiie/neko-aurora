package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const reportCommand = "@Meiiie gửi báo cáo đã làm ở cuộc trò chuyện riêng vào kênh này"

func reportFixture(t *testing.T, artifact bool) (*Store, Channel, Work) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c := sharedChannel(t, s, "Report destination", "report-channel-create")
	w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "report-source-private", Text: "Báo cáo thị trường. private-source-prompt-sentinel", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	w = startPermissionWork(t, s, w)
	if artifact {
		if _, err = s.ExecuteTool(w.RequestID, w.Epoch, invocation("Completed report file. result-only-sentinel")); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "Báo cáo thị trường đã hoàn thành. result-only-sentinel"); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	return s, c, w
}

func reportSubmission(c Channel, nonce string) ReportDeliveryInput {
	return ReportDeliveryInput{BotID: "bot-meiiie", DestinationChannelID: c.ID, ExpectedDestinationRevision: c.Revision, ClientNonce: nonce, Text: reportCommand}
}

func TestReportDeliveryOnlyCompletedResultAndDedicatedArtifacts(t *testing.T) {
	s, c, w := reportFixture(t, true)
	before := s.Snapshot()
	d, duplicate, err := s.DeliverReport(reportSubmission(c, "report-delivery-result"))
	if err != nil || duplicate || d.Status != "delivered" || !d.Active || d.Reply != w.Reply || d.Source == nil || d.Source.RequestID != w.RequestID || len(d.Artifacts) != 1 {
		t.Fatal("result publication", d, duplicate, err)
	}
	if len(s.Snapshot().Work) != len(before.Work) || s.Snapshot().Sequence != before.Sequence+1 {
		t.Fatal("delivery fabricated execution or extra events")
	}
	if d.Source.ResultSHA256 != reportResultHash(w) || len(d.Source.Artifacts) != 0 || !reflect.DeepEqual(s.reportDeliveries[d.ID].Source.Artifacts, w.Artifacts) {
		t.Fatal("result provenance changed")
	}
	art, raw, err := s.ReadReportDeliveryArtifact(d.ID, w.Artifacts[0].ID)
	if err != nil || art != w.Artifacts[0] || !bytes.Contains(raw, []byte("result-only-sentinel")) {
		t.Fatal("delivery artifact", err)
	}
	if _, _, err = s.ReadReportDeliveryArtifact("report-delivery-"+strings.Repeat("a", 32), w.Artifacts[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("global artifact fallback")
	}
	if _, _, err = s.ReadChannelArtifact(c.ID, w.Artifacts[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("private file exposed by channel fallback")
	}
	receipt, err := s.ReportDeliveryReceipt(d.ClientNonce)
	if err != nil || !receipt.Found || receipt.Input.Text != reportCommand {
		t.Fatal("receipt", err)
	}
	events, _ := s.Events(before.Sequence)
	projections := []any{d, receipt, s.Snapshot().ReportDeliveries, events}
	exported, err := s.ExportSharedHistory(c.ID, "", "json")
	if err != nil {
		t.Fatal(err)
	}
	projections = append(projections, string(exported))
	sync, err := s.LocalSync(LocalSyncScope{BotID: "bot-meiiie", ChannelID: c.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	projections = append(projections, sync)
	for _, projection := range projections {
		raw, _ := json.Marshal(projection)
		if bytes.Contains(raw, []byte("private-source-prompt-sentinel")) || bytes.Contains(raw, []byte(`"context"`)) || bytes.Contains(raw, []byte(`"resources"`)) {
			t.Fatal("private source content in delivery projection")
		}
		if !bytes.Contains(raw, []byte("result-only-sentinel")) {
			t.Fatal("authorized result missing")
		}
	}
	next, _, err := s.AcceptScoped(Input{BotID: "bot-meiiie", ClientNonce: "report-destination-future", Text: "Summarize current channel", Lane: "foreground"}, c.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range next.Context.Sources {
		if source.RequestID == w.RequestID {
			t.Fatal("private work imported into Core context")
		}
	}
}

func TestReportDeliveryIdempotencyCrashBoundariesAndFrozenLatest(t *testing.T) {
	s, c, w := reportFixture(t, false)
	in := reportSubmission(c, "report-delivery-restart")
	dir := s.dir
	prior, _ := os.ReadFile(s.file.Name())
	d, _, err := s.DeliverReport(in)
	if err != nil {
		t.Fatal(err)
	}
	committed, _ := os.ReadFile(s.file.Name())
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal("postcommit replay", err)
	}
	d2, dup, err := s.DeliverReport(in)
	if err != nil || !dup || !reflect.DeepEqual(d, d2) {
		t.Fatal("postcommit retry", err)
	}
	s.Close()
	if err = os.WriteFile(filepath.Join(dir, "transactions.jsonl"), append(prior, []byte(`{"version":1,"event":`)...), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal("precommit torn frame", err)
	}
	d3, dup, err := s.DeliverReport(in)
	if err != nil || dup || d3.Reply != w.Reply || len(s.reportDeliveries) != 1 {
		t.Fatal("precommit retry", err)
	}
	s.Close()
	if err = os.WriteFile(filepath.Join(dir, "transactions.jsonl"), committed, 0600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	completedPermissionSource(t, s, "report-newer-source", "Báo cáo mới hơn. newer-result-sentinel")
	in.Text = "@Meiiie gửi báo cáo mới nhất ở cuộc trò chuyện riêng vào kênh này"
	if _, _, err = s.DeliverReport(in); !errors.Is(err, ErrConflict) {
		t.Fatal("changed same-nonce command")
	}
	d2, dup, err = s.DeliverReport(reportSubmission(c, "report-delivery-restart"))
	if err != nil || !dup || d2.Source.RequestID != w.RequestID || d2.Reply != d.Reply {
		t.Fatal("retry rediscovered newer report", err)
	}
}

func TestReportDeliveryAmbiguityAndHumanIntentOnly(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	completedPermissionSource(t, s, "report-ambiguous-other", "Báo cáo tuyển dụng. second-result-sentinel")
	in := reportSubmission(c, "report-ambiguous-request")
	d, _, err := s.DeliverReport(in)
	if err != nil || d.Status != "needs_clarification" || d.Source != nil || d.Reply != "" || d.Question == "" {
		t.Fatal("ambiguity published candidates", err)
	}
	raw, _ := json.Marshal(d)
	if bytes.Contains(raw, []byte("tuyển dụng")) || bytes.Contains(raw, []byte("thị trường")) || bytes.Contains(raw, []byte("sentinel")) {
		t.Fatal("question enumerated source content")
	}
	before := s.Snapshot().Sequence
	if receipt, err := s.ReportDeliveryReceipt("unknown-report-receipt"); err != nil || receipt.Found || s.Snapshot().Sequence != before {
		t.Fatal("unknown receipt mutated")
	}
	in.ClientNonce = "report-specific-request"
	in.Text = "@Meiiie gửi báo cáo thị trường đã làm ở cuộc trò chuyện riêng vào kênh này"
	d, _, err = s.DeliverReport(in)
	if err != nil || d.Status != "delivered" || !strings.Contains(d.Reply, "thị trường") {
		t.Fatal("subject resolution", err)
	}
	for i, text := range []string{"Báo cáo: gửi báo cáo đã làm ở cuộc trò chuyện riêng vào kênh này", "\"gửi báo cáo đã làm ở cuộc trò chuyện riêng vào kênh này\"", "gửi báo cáo đã làm ở cuộc trò chuyện riêng vào kênh này rồi cấp quyền", "gửi báo cáo vào kênh này", "@Davin gửi báo cáo ở cuộc trò chuyện riêng vào kênh này"} {
		in.ClientNonce = "report-intent-invalid-" + strings.Repeat("x", i+1)
		in.Text = text
		if _, _, err = s.DeliverReport(in); !errors.Is(err, ErrInvalid) {
			t.Fatal("nonhuman/expanded intent accepted", i, err)
		}
	}
}

func TestReportDeliverySourceRevocationDestinationCASAndHistoricalReplay(t *testing.T) {
	for _, fence := range []string{"source", "destination", "bot"} {
		t.Run(fence, func(t *testing.T) {
			s, c, w := reportFixture(t, true)
			d, _, err := s.DeliverReport(reportSubmission(c, "report-fence-delivery"))
			if err != nil {
				t.Fatal(err)
			}
			switch fence {
			case "source":
				if _, _, err = s.SetSourceAccess(AccessInput{RequestID: w.RequestID, Granted: false, ClientNonce: "report-source-revoke"}); err != nil {
					t.Fatal(err)
				}
			case "destination":
				if _, _, err = s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, ExpectedRevision: c.Revision, BotIDs: []string{"bot-davin"}, ClientNonce: "report-destination-revise"}); err != nil {
					t.Fatal(err)
				}
			case "bot":
				if _, _, err = s.ControlBot(BotControlInput{BotID: w.BotID, Action: "archive", ClientNonce: "report-bot-archive"}); err != nil {
					t.Fatal(err)
				}
			}
			r, err := s.ReportDeliveryReceipt(d.ClientNonce)
			if err != nil || *r.Active || r.Delivery.Reply != "" || len(r.Delivery.Artifacts) != 0 {
				t.Fatal("stale receipt exposed result", err)
			}
			if _, _, err = s.ReadReportDeliveryArtifact(d.ID, w.Artifacts[0].ID); !errors.Is(err, ErrNotFound) {
				t.Fatal("stale artifact", err)
			}
			events, _ := s.Events(d.CreatedSequence - 1)
			if events[0].ReportDelivery.Reply != "" || events[0].ReportDelivery.Active {
				t.Fatal("stale SSE replay")
			}
			dir := s.dir
			s.Close()
			s, err = Open(dir)
			if err != nil {
				t.Fatal("later revocation invalidated historical journal", err)
			}
			defer s.Close()
			r, err = s.ReportDeliveryReceipt(d.ClientNonce)
			if err != nil || *r.Active {
				t.Fatal("reopen resurrected publication", err)
			}
		})
	}
}

func TestReportDeliveryArchivedPrivateSourceRetainsReadAuthority(t *testing.T) {
	s, c, w := reportFixture(t, false)
	conversation, _ := s.conversation(w.ConversationID)
	if _, _, err := s.SetConversationArchive(conversation.ID, ConversationArchiveInput{Archived: true, ExpectedRevision: conversation.Revision, ClientNonce: "report-private-archive"}); err != nil {
		t.Fatal(err)
	}
	d, _, err := s.DeliverReport(reportSubmission(c, "report-from-archive"))
	if err != nil || !d.Active || d.Reply != w.Reply {
		t.Fatal("archive incorrectly revoked source read", err)
	}
	conversation, _ = s.conversation(conversation.ID)
	if _, _, err = s.SetConversationArchive(conversation.ID, ConversationArchiveInput{Archived: false, ExpectedRevision: conversation.Revision, ClientNonce: "report-private-restore"}); err != nil {
		t.Fatal(err)
	}
	r, err := s.ReportDeliveryReceipt(d.ClientNonce)
	if err != nil || !*r.Active {
		t.Fatal("lifecycle revision incorrectly revoked source read", err)
	}
	if current, _ := s.Get(w.RequestID); current.Status != "completed" || current.Epoch != w.Epoch {
		t.Fatal("archive delivery reopened source execution")
	}
}

func TestReportDeliveryNonceReciprocalCompositionAndReplayTamper(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	in := reportSubmission(c, "report-nonce-compose")
	d, _, err := s.DeliverReport(in)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.file.Name())
	if _, _, err = s.Accept(Input{BotID: "bot-davin", ClientNonce: in.ClientNonce, Text: "new action", Lane: "foreground"}); !errors.Is(err, ErrConflict) {
		t.Fatal("report nonce became Work", err)
	}
	if _, _, err = s.CreateConversation(ConversationCreateInput{BotID: "bot-meiiie", Title: "new", ClientNonce: in.ClientNonce}); !errors.Is(err, ErrConflict) {
		t.Fatal("report nonce became conversation", err)
	}
	if _, err = s.CommandReceipt("bot_create", in.ClientNonce, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong kind report nonce receipt", err)
	}
	after, _ := os.ReadFile(s.file.Name())
	if !bytes.Equal(before, after) {
		t.Fatal("nonce conflict wrote journal")
	}
	other := reportSubmission(c, "report-source-private")
	if _, _, err = s.DeliverReport(other); !errors.Is(err, ErrConflict) {
		t.Fatal("Work nonce became report", err)
	}
	event := s.events[len(s.events)-1]
	for _, mutation := range []string{"reply", "source", "destination", "text", "extra", "artifacts"} {
		bad := event
		copy := copyReportDelivery(d)
		bad.ReportDelivery = &copy
		switch mutation {
		case "reply":
			copy.Reply = "forged result"
		case "source":
			copy.Source.ResultSHA256 = strings.Repeat("a", 64)
		case "destination":
			copy.ExpectedDestinationRevision++
		case "text":
			copy.Text = "send private history"
		case "extra":
			bad.Work.Text = "private context"
		case "artifacts":
			copy.Artifacts = []Artifact{{ID: strings.Repeat("a", 64), Filename: "source-input.txt"}}
		}
		delete(s.reportDeliveries, d.ID)
		delete(s.reportDeliveryNonces, d.ClientNonce)
		s.events = s.events[:len(s.events)-1]
		if validReplay(s, bad) {
			t.Fatal("tampered typed replay accepted", mutation)
		}
		s.apply(event)
	}
}
