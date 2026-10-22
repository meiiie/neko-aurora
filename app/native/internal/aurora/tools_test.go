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
