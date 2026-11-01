package aurora

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"nekoaurora.local/native/internal/tools"
)

type ownerURLTransport func(*http.Request) (*http.Response, error)

func (f ownerURLTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOwnerURLCanBeReadWithoutAuthorizingOtherPages(t *testing.T) {
	const requested = "https://docs.example.test/release"
	for _, tc := range []struct {
		name, text, target string
		allowed            bool
	}{
		{"explicit", "Đọc " + requested, requested, true},
		{"another page", "Đọc " + requested, requested + "/private", false},
		{"invented target", "Đọc thông tin bản phát hành", requested, false},
		{"insecure URL", "Đọc http://docs.example.test/release", "http://docs.example.test/release", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call, _ := json.Marshal(map[string]string{"name": "web_fetch", "url": tc.target})
			fa := scriptedAI(t, "TOOL: "+string(call), "Kết quả đọc trang.")
			defer fa.Close()
			t.Setenv("ZAI_API_KEY", "fixture")
			t.Setenv("AURORA_AI_BASE_URL", fa.URL)
			svc, _ := newSvc(t)
			var reached atomic.Int32
			kit := tools.New()
			kit.Client = &http.Client{Transport: ownerURLTransport(func(r *http.Request) (*http.Response, error) {
				reached.Add(1)
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("<title>Fixture release</title><p>Verified fixture result</p>")), Request: r}, nil
			})}
			svc.Tools = kit
			srv := httptest.NewServer(svc)
			defer srv.Close()
			body, _ := json.Marshal(map[string]string{"text": tc.text})
			bot := firstBot(svc)
			post(t, srv.URL+"/app/api/agents/"+bot+"/messages", string(body))
			waitTask(t, svc, bot)
			if (reached.Load() == 1) != tc.allowed {
				t.Fatalf("owner URL permission: reached=%d allowed=%v", reached.Load(), tc.allowed)
			}
			for _, m := range svc.view().messages(bot, 0) {
				if len(m.Tools) == 0 {
					continue
				}
				if (m.Tools[0].Status == "done") != tc.allowed || (len(m.Sources) == 1) != tc.allowed {
					t.Fatalf("tool result/attribution mismatch: %+v", m)
				}
				return
			}
			t.Fatal("missing tool receipt")
		})
	}
}
