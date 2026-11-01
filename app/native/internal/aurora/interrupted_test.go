package aurora

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

func TestInterruptedPreviewHidesIncompleteControls(t *testing.T) {
	for _, control := range []string{"TOOL", "APPROVAL", "FORM", "MEMORY", "HANDOFF", "SCHEDULE", "PLAN", "QUESTIONS", "SECRET", "CHOICES"} {
		for _, gap := range []string{"\n", ""} {
			text := "Mình đã đọc phần đầu." + gap + control + ": {\n\"private\": \"unfinished"
			if got := interruptedText(text); got != "Mình đã đọc phần đầu." {
				t.Fatalf("%s: leaked control payload: %q", control, got)
			}
		}
	}
	for _, text := range []string{"", "…", "**", "Ch", "TOOL: {\nunfinished"} {
		if got := interruptedText(text); got != "" {
			t.Fatalf("empty/control preview became a message: %q", got)
		}
	}
}

func TestInterruptedStreamIsSavedBeforeDraftEnds(t *testing.T) {
	oldPoll := inboxPoll
	inboxPoll = 20 * time.Millisecond
	t.Cleanup(func() { inboxPoll = oldPoll })
	a := &slowAI{delay: 20 * time.Millisecond, next: func(n int, _ string) string {
		if n == 0 {
			return "Đoạn đầu đang được viết. " + strings.Repeat("Nội dung tiếp theo vẫn đang được viết. ", 20)
		}
		return "Đã nhận yêu cầu bổ sung."
	}}
	fa := a.server(t)
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "fixture")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	svc.Tools = nil
	events := make(chan Event, 512)
	svc.mu.Lock()
	svc.subs[events] = struct{}{}
	svc.mu.Unlock()
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	url := srv.URL + "/app/api/agents/" + bot + "/messages"
	post(t, url, `{"text":"Viết một đoạn văn về mùa thu."}`)
	deadline := time.After(5 * time.Second)
waitPreview:
	for {
		select {
		case e := <-events:
			if e.Type == "delta" && strings.Contains(e.Data.(map[string]any)["text"].(string), "Đoạn đầu đang được viết.") {
				break waitPreview
			}
		case <-deadline:
			t.Fatal("no streamed preview")
		}
	}
	post(t, url, `{"text":"Bổ sung: chỉ cần xác nhận đã nhận yêu cầu bổ sung."}`)
	saved := false
waitSaved:
	for {
		select {
		case e := <-events:
			if e.Type == "message" && e.Data.(*Message).Interrupted {
				saved = true
			}
			if e.Type == "draft_end" {
				if !saved {
					t.Fatal("visible draft cleared before durable message was published")
				}
				break waitSaved
			}
		case <-deadline:
			t.Fatal("no saved interruption")
		}
	}
	waitTask(t, svc, bot)
	msgs := svc.view().messages(bot, 0)
	if len(msgs) != 4 || msgs[0].Role != "user" || !msgs[1].Interrupted || msgs[2].Role != "user" || msgs[3].Interrupted || msgs[3].Text != "Đã nhận yêu cầu bổ sung." {
		t.Fatalf("lost or reordered messages: %+v", msgs)
	}
	if !strings.HasPrefix(msgs[1].Text, "Đoạn đầu đang được viết.") || msgs[1].ID == msgs[3].ID {
		t.Fatalf("partial replaced by final: %+v", msgs[1])
	}
}

func TestRepeatedInterjectionsKeepConversationOrder(t *testing.T) {
	st, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bot := st.Snapshot().Bots[0].ID
	a, _, _ := st.Accept(ledger.Input{BotID: bot, ClientNonce: "partial-order-a", Text: "Yêu cầu đầu"})
	b, _, _ := st.Accept(ledger.Input{BotID: bot, ClientNonce: "partial-order-b", Text: "Bổ sung lần một"})
	c, _, _ := st.Accept(ledger.Input{BotID: bot, ClientNonce: "partial-order-c", Text: "Bổ sung lần hai"})
	a, err = st.DirectStart(a.RequestID, a.Epoch, ledger.DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.DirectUpdate(a.RequestID, a.Epoch, func(r *ledger.DirectRun) {
		r.Steered = []string{b.RequestID, c.RequestID}
		r.Partials = []ledger.DirectPartial{{Text: "Đoạn một", At: a.CreatedAt, BeforeRequestID: b.RequestID}, {Text: "Đoạn hai", At: a.CreatedAt, BeforeRequestID: c.RequestID}}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendText(a.RequestID, a.Epoch, "Kết quả cuối"); err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, m := range newView(st.Snapshot()).messages(bot, 0) {
		texts = append(texts, m.Text)
	}
	if got := strings.Join(texts, "|"); got != "Yêu cầu đầu|Đoạn một|Bổ sung lần một|Đoạn hai|Bổ sung lần hai|Kết quả cuối" {
		t.Fatalf("interleaving: %s", got)
	}
}
