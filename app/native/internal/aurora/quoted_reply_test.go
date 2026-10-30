package aurora

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestQuotedReplyScopeAndReceiptSurviveRestart(t *testing.T) {
	for _, group := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "group"}[group], func(t *testing.T) {
			dir := t.TempDir()
			l, err := ledger.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s, err := New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(s)
			defer func() { srv.Close(); l.Close() }()
			thread := "bot-meiiie"
			if group {
				c, _, err := l.SaveChannel(ledger.ChannelInput{Name: "Quote", BotIDs: []string{thread, "bot-davin"}, ClientNonce: "quote-group-fixture"})
				if err != nil {
					t.Fatal(err)
				}
				thread = c.ID
			}
			request := func(target string, body map[string]string) (int, map[string]any) {
				b, _ := json.Marshal(body)
				r := post(t, srv.URL+"/app/api/agents/"+target+"/messages", string(b))
				defer r.Body.Close()
				var v map[string]any
				json.NewDecoder(r.Body).Decode(&v)
				return r.StatusCode, v
			}
			code, source := request(thread, map[string]string{"text": "Original source @all <script>plain</script>", "clientNonce": "quoted-source-fixture"})
			if code != 202 {
				t.Fatal("source", code)
			}
			_, foreign := request("bot-davin", map[string]string{"text": "private other chat", "clientNonce": "foreign-source-fixture"})
			bad := map[string]string{"text": "reply", "clientNonce": "bad-quote-fixture", "replyTo": foreign["id"].(string)}
			if code, _ := request(thread, bad); code != 400 {
				t.Fatal("cross-thread quote accepted", code)
			}
			body := map[string]string{"text": "Explain this", "clientNonce": "quoted-answer-fixture", "replyTo": source["id"].(string)}
			code, first := request(thread, body)
			quote, ok := first["quote"].(map[string]any)
			if code != 202 || !ok || quote["id"] != source["id"] || quote["text"] != "Original source @all <script>plain</script>" || first["text"] != body["text"] {
				t.Fatal("quote snapshot missing", code, first)
			}
			w, _ := l.Get(first["taskId"].(string))
			if !strings.Contains(w.Text, "Original source @all") {
				t.Fatal("model context omitted quote")
			}
			sourceWork, _ := l.Get(source["taskId"].(string))
			l.DirectStart(sourceWork.RequestID, sourceWork.Epoch, ledger.DirectRun{Runtime: "direct-http"})
			l.AppendText(sourceWork.RequestID, sourceWork.Epoch, "Assistant snapshot")
			agentBody := map[string]string{"text": "Reply to assistant", "clientNonce": "quote-agent-fixture", "replyTo": sourceWork.RequestID}
			if code, _ := request(thread, agentBody); code != 400 {
				t.Fatal("unfinished assistant quoted", code)
			}
			l.DirectFinish(sourceWork.RequestID, sourceWork.Epoch, "completed", "", nil)
			code, agentReply := request(thread, agentBody)
			agentQuote, ok := agentReply["quote"].(map[string]any)
			if code != 202 || !ok || agentQuote["text"] != "Assistant snapshot" || agentQuote["role"] != "agent" {
				t.Fatal("completed assistant quote", code, agentReply)
			}
			if group {
				n := 0
				for _, task := range l.Snapshot().Work {
					if strings.HasPrefix(task.ClientNonce, "quoted-answer-fixture_g") {
						n++
					}
				}
				if n != 1 {
					t.Fatal("quoted @all changed normal group routing", n)
				}
			}
			count := len(l.Snapshot().Work)
			srv.Close()
			l.Close()
			l, err = ledger.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			s, err = New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
			if err != nil {
				t.Fatal(err)
			}
			srv = httptest.NewServer(s)
			code, again := request(thread, body)
			if code != 202 || again["id"] != first["id"] || len(l.Snapshot().Work) != count {
				t.Fatal("retry changed receipt", code)
			}
			var history []map[string]any
			getJSON(t, srv.URL+"/app/api/agents/"+thread+"/messages", &history)
			found := false
			for _, m := range history {
				if m["id"] == first["id"] {
					q, ok := m["quote"].(map[string]any)
					found = ok && q["id"] == source["id"] && m["text"] == body["text"]
				}
			}
			if !found {
				t.Fatal("history lost quote")
			}
			body["replyTo"] = foreign["id"].(string)
			if code, _ := request(thread, body); code != 409 {
				t.Fatal("changed quote reused nonce", code)
			}
		})
	}
}
