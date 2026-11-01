package aurora

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

func TestFollowupDoesNotCancelOrRewriteVisibleAnswer(t *testing.T) {
	const joke = "Một lập trình viên đi mua đồ, người yêu dặn: Mua một hộp sữa nhé. Nếu có trứng thì mua sáu hộp. Anh về với sáu hộp sữa vì cửa hàng có trứng."
	const email = "Tiêu đề: [Nội dung trao đổi]. Kính gửi [Người nhận], mình xin trao đổi về [chủ đề]. Trân trọng, [Tên]. Email này gửi cho ai và về việc gì?"
	a := &slowAI{delay: 20 * time.Millisecond, next: func(n int, _ string) string {
		if n == 0 {
			return joke
		}
		return email
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
	post(t, url, `{"text":"kể cho tôi một câu chuyện cười đi"}`)
	deadline := time.After(5 * time.Second)
waitPreview:
	for {
		select {
		case e := <-events:
			if e.Type == "delta" && strings.Contains(e.Data.(map[string]any)["text"].(string), "người yêu dặn") {
				break waitPreview
			}
		case <-deadline:
			t.Fatal("no streamed preview")
		}
	}
	post(t, url, `{"text":"và song song đó giúp tôi soạn một email sao cho chuyên nghiệp nhé"}`)
	waitTask(t, svc, bot)
	msgs := svc.view().messages(bot, 0)
	if len(msgs) != 4 || msgs[0].Role != "user" || msgs[1].Text != joke || msgs[1].Interrupted || msgs[2].Role != "user" || msgs[3].Text != email {
		t.Fatalf("expected original complete joke then separate email reply: %+v", msgs)
	}
	if a.calls() != 2 {
		t.Fatalf("completed response was regenerated: %d calls", a.calls())
	}
	if !strings.Contains(a.last(), joke) {
		t.Fatal("follow-up lost the completed assistant response")
	}
	saved := false
	for len(events) > 0 {
		e := <-events
		if e.Type == "message" {
			m := e.Data.(*Message)
			if m.Text == joke {
				saved = true
			}
		}
		if e.Type == "draft_end" && !saved {
			t.Fatal("cleared first draft before saving complete answer")
		}
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
