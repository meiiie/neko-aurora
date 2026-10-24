package aurora

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nekoaurora.local/native/internal/tools"
)

// slowAI streams each reply word by word (delay per word) and records every
// request body, so tests can send messages while a turn is mid-stream.
type slowAI struct {
	mu     sync.Mutex
	bodies []string
	next   func(n int, body string) string
	delay  time.Duration
}

func (a *slowAI) server(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		a.mu.Lock()
		n := len(a.bodies)
		a.bodies = append(a.bodies, string(b))
		a.mu.Unlock()
		rep := a.next(n, string(b))
		w.Header().Set("Content-Type", "text/event-stream")
		for _, part := range strings.SplitAfter(rep, " ") {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(a.delay):
			}
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", part)
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func (a *slowAI) calls() int { a.mu.Lock(); defer a.mu.Unlock(); return len(a.bodies) }
func (a *slowAI) last() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.bodies[len(a.bodies)-1]
}

var burst = []string{"thêm ý một", "thêm ý hai", "thêm ý ba", "thêm ý bốn", "thêm ý năm"}

func sendBurst(t *testing.T, url string) {
	for i, m := range burst {
		post(t, url, fmt.Sprintf(`{"text":%q,"clientNonce":"burst-%d-%d"}`, m, i, time.Now().UnixNano()))
	}
}

// checkOneReply: every burst message absorbed by the first run, exactly one
// agent bubble, all queued turns closed silently, user messages before it.
func checkOneReply(t *testing.T, svc *Service, bot string, a *slowAI) {
	t.Helper()
	waitTask(t, svc, bot)
	msgs := svc.view().messages(bot, 0)
	agent, users, lastUser, firstAgent := 0, 0, -1, -1
	for i, m := range msgs {
		switch m.Role {
		case "agent":
			agent++
			if firstAgent < 0 {
				firstAgent = i
			}
		case "user":
			users++
			lastUser = i
		case "system":
			t.Fatalf("unexpected system message %q", m.Text)
		}
	}
	if agent != 1 || users != 6 || lastUser > firstAgent {
		var ids []string
		for _, m := range msgs {
			ids = append(ids, m.Role+":"+clean(m.Text, 30))
		}
		t.Fatalf("agent=%d users=%d lastUser=%d firstAgent=%d\n%s", agent, users, lastUser, firstAgent, strings.Join(ids, "\n"))
	}
	body := a.last()
	for _, m := range burst {
		if !strings.Contains(body, m) {
			t.Fatalf("final model call is missing %q", m)
		}
	}
	for _, w := range svc.view().byBot[bot] {
		if w.Status != "completed" {
			t.Fatalf("work %s ended %s (%s)", w.RequestID, w.Status, w.Failure)
		}
	}
}

func TestBurstDuringStreamingMergesIntoOneReply(t *testing.T) {
	inboxPoll = 20 * time.Millisecond
	a := &slowAI{delay: 30 * time.Millisecond, next: func(int, string) string {
		return "Đây là câu trả lời khá dài để có thời gian nhắn thêm trong lúc đang viết từng chữ một nhé bạn."
	}}
	fa := a.server(t)
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	svc.Tools = nil
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	url := srv.URL + "/app/api/agents/" + bot + "/messages"
	post(t, url, `{"text":"Kể mình nghe về Hải Phòng"}`)
	for a.calls() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond) // mid-stream
	sendBurst(t, url)
	checkOneReply(t, svc, bot, a)
	// bound: the first call, maxInterrupts rebuilt calls, plus one final
	// re-answer in runTurn for messages that landed during the last
	// (unwatched) call. The old "1+maxInterrupts" check ignored that last
	// call and failed whenever the burst straddled it (timing-dependent).
	if a.calls() > 2+maxInterrupts {
		t.Fatalf("too many model calls: %d", a.calls())
	}
}

func TestBurstDuringToolCallIsAbsorbedAtNextStep(t *testing.T) {
	inboxPoll = 20 * time.Millisecond
	a := &slowAI{delay: 2 * time.Millisecond, next: func(n int, _ string) string {
		if n == 0 {
			return "Để mình tra.\nTOOL: {\"name\":\"web_search\",\"q\":\"Hải Phòng\"}"
		}
		return "Hải Phòng là thành phố cảng [1]."
	}}
	fa := a.server(t)
	defer fa.Close()
	inTool := make(chan struct{}, 1)
	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inTool <- struct{}{}
		time.Sleep(400 * time.Millisecond)
		fmt.Fprint(w, `<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fvnexpress.net%2Fhp">HP</a><a class="result__snippet" href="#">cảng</a>`)
	}))
	defer search.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	k := tools.New()
	k.Client, k.Search = search.Client(), search.URL
	svc.Tools = k
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	url := srv.URL + "/app/api/agents/" + bot + "/messages"
	post(t, url, `{"text":"Tra về Hải Phòng"}`)
	<-inTool
	sendBurst(t, url)
	checkOneReply(t, svc, bot, a)
}

func TestResendSameNonceIsOneMessage(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "")
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	url := srv.URL + "/app/api/agents/" + bot + "/messages"
	for i := 0; i < 3; i++ { // client outbox retrying after a dropped connection
		post(t, url, `{"text":"xin chào","clientNonce":"outbox-retry-1"}`)
	}
	n := 0
	for _, m := range svc.view().messages(bot, 0) {
		if m.Role == "user" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("retries with one nonce must be one message, got %d", n)
	}
}
