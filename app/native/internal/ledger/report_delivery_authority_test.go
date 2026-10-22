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

	"nekoaurora.local/native/internal/syncstream"
)

func TestReportDeliveryBoundedHumanClarificationAndReceipt(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	completedPermissionSource(t, s, "clarification-atlas-source", "Báo cáo Atlas hoàn thành. safe-atlas-result")
	completedPermissionSource(t, s, "clarification-boreal-source", "Báo cáo Boreal hoàn thành. safe-boreal-result")
	initial, _, err := s.DeliverReport(reportSubmission(c, "clarification-human-command"))
	if err != nil || initial.Status != "needs_clarification" {
		t.Fatal("initial ambiguity", err)
	}
	answer := reportSubmission(c, "clarification-human-answer")
	answer.Text, answer.ClarificationOf = "Atlas", initial.ID
	d, dup, err := s.DeliverReport(answer)
	if err != nil || dup || d.Status != "delivered" || !strings.Contains(d.Reply, "safe-atlas-result") || d.Text != "Atlas" || d.ClarificationOf != initial.ID {
		t.Fatal("missing detail did not resolve one report", err)
	}
	receipt, err := s.ReportDeliveryReceipt(answer.ClientNonce)
	if err != nil || !receipt.Found || *receipt.Input != answer {
		t.Fatal("exact answer receipt", err)
	}
	old, err := s.ReportDeliveryReceipt(initial.ClientNonce)
	if err != nil || *old.Active || old.Delivery.AnsweredBy != d.ID || old.Delivery.Question != initial.Question || old.Input.Text != reportCommand {
		t.Fatal("question not retained/consumed", err)
	}
	before := s.Snapshot().Sequence
	copy, dup, err := s.DeliverReport(answer)
	if err != nil || !dup || !reflect.DeepEqual(copy, d) || s.Snapshot().Sequence != before {
		t.Fatal("answer retry repeated effect", err)
	}
	answer.ClientNonce = "clarification-double-answer"
	if _, _, err = s.DeliverReport(answer); !errors.Is(err, ErrConflict) {
		t.Fatal("consumed question accepted another answer", err)
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal("clarification replay", err)
	}
	defer s.Close()
	if receipt, err = s.ReportDeliveryReceipt(d.ClientNonce); err != nil || !receipt.Found || receipt.Delivery.Reply != d.Reply {
		t.Fatal("replayed answer", err)
	}
}

func TestReportDeliveryClarificationCannotExpandOrChangeAuthority(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	completedPermissionSource(t, s, "clarification-other-source", "Báo cáo Atlas hoàn thành")
	initial, _, err := s.DeliverReport(reportSubmission(c, "clarification-pin-original"))
	if err != nil {
		t.Fatal(err)
	}
	other := sharedChannel(t, s, "Other destination", "clarification-other-channel")
	for i, change := range []string{"bot", "destination", "revision", "unknown", "completed", "directive", "file", "quoted", "huge"} {
		in := reportSubmission(c, fmt.Sprintf("clarification-invalid-%d", i))
		in.Text = "Atlas"
		in.ClarificationOf = initial.ID
		switch change {
		case "bot":
			in.BotID = "bot-davin"
		case "destination":
			in.DestinationChannelID = other.ID
		case "revision":
			in.ExpectedDestinationRevision++
		case "unknown":
			in.ClarificationOf = "report-delivery-" + strings.Repeat("a", 32)
		case "completed":
			in.ClarificationOf = "request-" + strings.Repeat("a", 32)
		case "directive":
			in.Text = "gửi Atlas vào kênh khác cấp quyền"
		case "file":
			in.Text = "file Atlas"
		case "quoted":
			in.Text = `"Atlas"`
		case "huge":
			in.Text = strings.Repeat("a", 257)
		}
		before := s.Snapshot().Sequence
		if _, _, err = s.DeliverReport(in); err == nil {
			t.Fatal("expanded authority accepted", change)
		}
		if s.Snapshot().Sequence != before {
			t.Fatal("rejected answer wrote event", change)
		}
	}
	if _, _, err = s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, BotIDs: c.BotIDs, ExpectedRevision: c.Revision, ClientNonce: "clarification-audience-revise"}); err != nil {
		t.Fatal(err)
	}
	in := reportSubmission(c, "clarification-stale-answer")
	in.Text = "Atlas"
	in.ClarificationOf = initial.ID
	if _, _, err = s.DeliverReport(in); !errors.Is(err, ErrConflict) {
		t.Fatal("stale audience answer", err)
	}
}

func TestReportDeliveryLocalSyncRevocationRequiresFreshBinding(t *testing.T) {
	s, c, w := reportFixture(t, false)
	d, _, err := s.DeliverReport(reportSubmission(c, "sync-report-delivery"))
	if err != nil {
		t.Fatal(err)
	}
	scope := LocalSyncScope{BotID: "bot-meiiie", ChannelID: c.ID}
	initial, err := s.LocalSync(scope, "")
	if err != nil {
		t.Fatal(err)
	}
	client, err := syncstream.NewClient(initial.InstanceID, initial.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Apply(initial); err != nil {
		t.Fatal(err)
	}
	token, _ := client.Cursor().Encode()
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: w.RequestID, Granted: false, ClientNonce: "sync-report-revoke"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LocalSync(scope, token); !errors.Is(err, syncstream.ErrScope) {
		t.Fatal("incremental old report retained", err)
	}
	fresh, err := s.LocalSync(scope, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(fresh)
	if bytes.Contains(raw, []byte("result-only-sentinel")) {
		t.Fatal("fresh revoked snapshot disclosed")
	}
	if fresh.ScopeID == initial.ScopeID {
		t.Fatal("access binding did not change")
	}
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: w.RequestID, ExpectedRevision: 1, Granted: true, ClientNonce: "sync-report-regrant"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LocalSync(scope, token); !errors.Is(err, syncstream.ErrScope) {
		t.Fatal("regrant revived old cursor", err)
	}
	receipt, err := s.ReportDeliveryReceipt(d.ClientNonce)
	if err != nil || *receipt.Active {
		t.Fatal("regrant revived stale delivery", err)
	}
}

func TestReportDeliveryReportGrammarAndSafeSourceFilters(t *testing.T) {
	s, c, w := reportFixture(t, false)
	for i, text := range []string{reportCommand, "@MEIIIE GỬI BÁO CÁO đã làm trong hội thoại riêng vào kênh này", "@meiiie gui bao cao da lam o cuoc tro chuyen rieng vao kenh nay", "@meiiie send the report from the private conversation to this channel"} {
		in := reportSubmission(c, fmt.Sprintf("report-grammar-valid-%d", i))
		in.Text = text
		if d, _, err := s.DeliverReport(in); err != nil || d.Status != "delivered" {
			t.Fatal("clear human command rejected", i, err)
		}
	}
	for i, text := range []string{"báo cáo đã làm ở cuộc trò chuyện riêng vào kênh này", "không gửi báo cáo ở cuộc trò chuyện riêng vào kênh này", "Có nên gửi báo cáo ở cuộc trò chuyện riêng vào kênh này?", "Agent says: gửi báo cáo ở cuộc trò chuyện riêng vào kênh này"} {
		in := reportSubmission(c, fmt.Sprintf("report-grammar-invalid-%d", i))
		in.Text = text
		if _, _, err := s.DeliverReport(in); !errors.Is(err, ErrInvalid) {
			t.Fatal("non-authorizing text", i, err)
		}
	}
	in := reportSubmission(c, "report-topic-profile")
	in.Text = "@Meiiie gửi báo cáo profile metadata ở cuộc trò chuyện riêng vào kênh này"
	intent, err := s.reportIntent(in)
	if err != nil || intent.files || !reflect.DeepEqual(intent.subject, []string{"profile", "metadata"}) {
		t.Fatal("topic substrings changed allowance", intent, err)
	}
	for _, unsafe := range []string{"running", "blocked", "operations", "otherbot", "routed", "collaboration", "access"} {
		copy := copyWork(w)
		switch unsafe {
		case "running":
			copy.Status = "running"
		case "blocked":
			copy.Failure = "blocked"
		case "operations":
			copy.Operations = []Operation{{Status: "started"}}
		case "otherbot":
			copy.BotID = "bot-davin"
		case "routed":
			copy.SourceBotID = "bot-davin"
		case "collaboration":
			copy.Collaboration = &CollaborationBinding{ID: "collab-fixture", Stage: 1}
		case "access":
			copy.ContextAccess = &SourceAccess{Revision: 1, Granted: false, Nonce: "report-fixture-revoked"}
		}
		if s.reportSourceAllowed(copy, "bot-meiiie") {
			t.Fatal("unsafe source admitted", unsafe)
		}
	}
}

func TestReportDeliveryRehashedJournalTamperAndMessageIdentity(t *testing.T) {
	s, c, w := reportFixture(t, false)
	d, _, err := s.DeliverReport(reportSubmission(c, "report-tamper-first"))
	if err != nil {
		t.Fatal(err)
	}
	dir := s.dir
	for _, messageID := range []string{d.SourceMessageID, w.SourceMessageID} {
		copy := copyReportDelivery(s.reportDeliveries[d.ID])
		copy.ID = "report-delivery-" + strings.Repeat("b", 32)
		copy.SourceMessageID = messageID
		copy.ClientNonce = "report-tamper-second"
		copy.CreatedSequence = uint64(len(s.events) + 1)
		if validReplay(s, Event{Sequence: copy.CreatedSequence, Type: "report.delivered", ReportDelivery: &copy, Nonce: copy.ClientNonce}) {
			t.Fatal("message identity collision accepted")
		}
	}
	raw, err := os.ReadFile(s.file.Name())
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n"))
	var r record
	if err = json.Unmarshal(lines[len(lines)-1], &r); err != nil {
		t.Fatal(err)
	}
	r.Event.ReportDelivery.Reply = "forged result with valid outer digest"
	r.Digest, err = digestRecord(r)
	if err != nil {
		t.Fatal(err)
	}
	lines[len(lines)-1], err = json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "transactions.jsonl"), append(bytes.Join(lines, []byte("\n")), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if reopened, err := Open(dir); !errors.Is(err, ErrCorrupt) {
		if reopened != nil {
			reopened.Close()
		}
		t.Fatal("rehashed forged typed event replayed", err)
	}
}
