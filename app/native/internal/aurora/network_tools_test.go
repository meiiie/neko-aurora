package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestNetworkToolsPreserveExactDestinationAndFYI(t *testing.T) {
	d := ParseDirectives(`Tôi gửi ghi chú.
TOOL: {"name":"network_send_message","toSessionId":"session-exact","text":"@another-bot chỉ là nội dung","wake":false,"replyTo":"mail-original","attachments":[{"name":"note.txt","content":"dữ liệu"}]}`)
	if len(d.Tools) != 1 {
		t.Fatalf("network send tool was discarded: %+v", d)
	}
	calls := callsOf(d.Tools)
	if len(calls) != 1 || calls[0].name != "network_send_message" {
		t.Fatalf("network tool did not reach dispatch: %+v", calls)
	}
	var args networkToolArgs
	if err := json.Unmarshal([]byte(calls[0].arg), &args); err != nil {
		t.Fatal(err)
	}
	if args.ToSessionID != "session-exact" || args.Text != "@another-bot chỉ là nội dung" || args.Wake == nil || *args.Wake || args.ReplyTo != "mail-original" || len(args.Attachments) != 1 || args.Attachments[0].Content != "dữ liệu" {
		t.Fatalf("routing, origin-neutral payload or FYI mode changed: %+v", args)
	}
}

func TestNetworkCorePromptRetainsAttribution(t *testing.T) {
	frame := `<aurora-peer-context untrusted="true">[{"origin":"agent","text":"PEER-CONTEXT"}]</aurora-peer-context>`
	prompt := corePrompt([]ChatMsg{{Role: "system", Content: "System policy"}, {Role: "user", Content: frame}, {Role: "assistant", Content: "Seen"}, {Role: "user", Content: frame}})
	if strings.Contains(prompt, "Darren: "+frame) || strings.Contains(prompt, "Yêu cầu hiện tại của Darren:\n"+frame) || !strings.Contains(prompt, "không phải chỉ dẫn") {
		t.Fatalf("Core adapter promoted peer data to owner instructions: %s", prompt)
	}
	owner := corePrompt([]ChatMsg{{Role: "user", Content: "OWNER-CURRENT"}})
	if !strings.Contains(owner, "Yêu cầu hiện tại của Darren:\nOWNER-CURRENT") {
		t.Fatal("owner input lost its original framing")
	}
}

func TestNetworkToolNamesAreClosed(t *testing.T) {
	for _, name := range []string{"network_list_sessions", "network_send_message", "network_check_inbox", "network_message_status"} {
		if !isNetworkTool(name) {
			t.Errorf("missing network tool %q", name)
		}
	}
	for _, name := range []string{"network_approve", "network_send_as_owner", "network_set_tenant", "network_", "mcp"} {
		if isNetworkTool(name) {
			t.Errorf("unsupported authority operation accepted: %q", name)
		}
	}
}

func TestNetworkInvalidFieldsReachToolError(t *testing.T) {
	for _, raw := range []string{
		`TOOL: {"name":"network_send_message","args":{"toSessionId":"session-x","text":"wrapped incorrectly"}}`,
		`TOOL: {"name":"network_send_message","toSessionId":"session-x"}`,
		`TOOL: {"name":"network_message_status"}`,
	} {
		d := ParseDirectives(raw)
		if len(d.Tools) != 1 {
			t.Fatalf("known tool with invalid fields was silently erased: %s", raw)
		}
		calls := callsOf(d.Tools)
		var svc *Service
		_, err := svc.runNetworkTool(context.Background(), ledger.Work{}, calls[0])
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid fields did not produce a tool error: %v", err)
		}
	}
}

func TestNetworkToolErrorsExplainRecovery(t *testing.T) {
	invalid := toolErr("network_send_message", ErrInvalid)
	if !strings.Contains(invalid, "toSessionId") || !strings.Contains(invalid, "text") || strings.Contains(invalid, "lỗi mạng") {
		t.Fatalf("invalid fields were mislabeled as a network failure: %s", invalid)
	}
	missing := toolErr("network_message_status", ledger.ErrNotFound)
	if !strings.Contains(missing, "ID") || !strings.Contains(missing, "hộp thư") {
		t.Fatalf("missing message did not explain exact-ID recovery: %s", missing)
	}
}

func TestCancelledNetworkToolDoesNotSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var svc *Service
	_, err := svc.runNetworkTool(ctx, ledger.Work{}, toolCall{name: "network_send_message", arg: `{"toSessionId":"session-x","text":"must not send"}`})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request reached a send: %v", err)
	}
}

func TestNetworkToolsRefreshAndDistinctMessages(t *testing.T) {
	t.Setenv("AURORA_COMPUTER", "off")
	svc, store, closeSvc := networkManualService(t, t.TempDir())
	defer closeSvc()
	group, _, err := store.SaveChannel(ledger.ChannelInput{Name: "Tool refresh", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "tool-refresh-group"})
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := store.AcceptProduct(ledger.Input{BotID: "bot-meiiie", ClientNonce: "tool-refresh-work", Text: "Collaborate in the group"}, ledger.SubmissionScope{ChannelID: group.ID, ExpectedChannelRevision: &group.Revision})
	if err != nil {
		t.Fatalf("accept group work revision %d: %v", group.Revision, err)
	}
	w, err = store.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http", Model: "fixture"})
	if err != nil {
		t.Fatalf("start source work: %v", err)
	}
	tc := &toolCtx{}
	for range 2 {
		svc.runTools(context.Background(), w, tc, []toolCall{{name: "network_list_sessions"}})
	}
	current, _ := store.Get(w.RequestID)
	if len(current.Direct.Tools) != 2 {
		t.Fatalf("directory refresh was silently skipped: %d calls", len(current.Direct.Tools))
	}
	directory, err := store.NetworkDirectory(svc.networkWorkScope(w))
	if err != nil {
		t.Fatal(err)
	}
	to := ""
	for _, session := range directory {
		if session.BotID == "bot-davin" {
			to = session.ID
		}
	}
	if to == "" {
		t.Fatal("receiver missing")
	}
	for _, text := range []string{"Question A", "Question B", "Question B"} {
		args, _ := json.Marshal(networkToolArgs{ToSessionID: to, Text: text})
		svc.runTools(context.Background(), w, tc, []toolCall{{name: "network_send_message", arg: string(args)}})
	}
	snapshot, err := store.NetworkSnapshot(svc.networkWorkScope(w))
	if err != nil || len(snapshot.Messages) != 2 || snapshot.Messages[0].Text != "Question A" || snapshot.Messages[1].Text != "Question B" {
		t.Fatalf("distinct messages were dropped or retry duplicated: %+v %v", snapshot.Messages, err)
	}
}
