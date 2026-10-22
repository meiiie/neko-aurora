package aurora

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/tools"
)

func TestModelRequestedToolCitesSources(t *testing.T) {
	fa := scriptedAI(t, "Để mình tra.\nTOOL: {\"name\":\"web_search\",\"q\":\"Fed lãi suất\"}", "Fed giữ nguyên lãi suất [1].")
	defer fa.Close()
	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fwww.reuters.com%2Ffed">Fed <b>holds</b></a><a class="result__snippet" href="#">Fed giữ lãi suất</a>`)
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
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"Fed tuần này thế nào?"}`)
	waitTask(t, svc, bot)
	var m *Message
	for _, x := range svc.view().messages(bot, 0) {
		if len(x.Tools) > 0 {
			m = x
		}
	}
	if m == nil || m.Tools[0].Status != "done" || m.Tools[0].Name != "web_search" || len(m.Sources) != 1 || m.Sources[0].URL != "https://www.reuters.com/fed" {
		t.Fatalf("%+v", m)
	}
	if strings.Contains(m.Text, "TOOL") || !strings.Contains(m.Text, "[1]") {
		t.Fatalf("text %q", m.Text)
	}
	// reply flow: a short ack (the model's pre-TOOL sentence) is its own message, before the answer
	var ack, ans = -1, -1
	for i, x := range svc.view().messages(bot, 0) {
		if strings.HasSuffix(x.ID, ":ack") && x.Text == "Để mình tra." {
			ack = i
		}
		if x.ID == m.ID {
			ans = i
		}
	}
	if ack < 0 || ack > ans {
		t.Fatalf("ack %d answer %d", ack, ans)
	}
}

func TestFailedToolIsHonest(t *testing.T) {
	fa := scriptedAI(t, "Tra đã.\nTOOL: {\"name\":\"web_fetch\",\"url\":\"https://example.com/x\"}", "Mình không đọc được trang đó nên không có số liệu.")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"đọc example.com giúp"}`)
	waitTask(t, svc, bot)
	for _, x := range svc.view().messages(bot, 0) {
		if len(x.Tools) > 0 {
			if x.Tools[0].Status != "failed" || len(x.Sources) != 0 {
				t.Fatalf("%+v", x)
			}
			return
		}
	}
	t.Fatal("no tool card")
}

func TestRepeatedToolIsNotRefetched(t *testing.T) {
	fa := scriptedAI(t, "Tra.\nTOOL: {\"name\":\"web_search\",\"q\":\"Fed\"}", "Lại.\nTOOL: {\"name\":\"web_search\",\"q\":\"Fed\"}", "Xong [1].")
	defer fa.Close()
	hits := 0
	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fwww.reuters.com%2Ffed">Fed</a><a class="result__snippet" href="#">x</a>`)
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
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"Fed?"}`)
	waitTask(t, svc, bot)
	if hits != 1 {
		t.Fatalf("search hit %d times", hits)
	}
}

func TestToolBudgetEndsWithAnswer(t *testing.T) {
	loop := []string{}
	for i := 0; i < maxToolRounds+1; i++ {
		loop = append(loop, fmt.Sprintf("Tra tiếp.\nTOOL: {\"name\":\"web_search\",\"q\":\"giá gói %d\"}", i))
	}
	fa := scriptedAI(t, append(loop, "Bảng so sánh: gói A rẻ hơn [1].")...)
	defer fa.Close()
	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fa">Gói A</a><a class="result__snippet" href="#">Gói A 45.000đ</a>`)
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
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"So sánh giá các gói"}`)
	waitTask(t, svc, bot)
	var last *Message
	for _, x := range svc.view().messages(bot, 0) {
		if x.Role != "user" {
			last = x
		}
	}
	if last == nil || !strings.Contains(last.Text, "Bảng so sánh") || strings.Contains(last.Text, "chưa lấy thêm") {
		t.Fatalf("final %+v", last)
	}
}
