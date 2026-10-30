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

func TestMessageReferenceAndGroupReceiptSurviveChanges(t *testing.T) {
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
			bot, ref := "bot-meiiie", "bot-davin"
			var channel ledger.Channel
			if group {
				channel, _, err = l.SaveChannel(ledger.ChannelInput{Name: "Message fixture", BotIDs: []string{bot, ref}, ClientNonce: "message-group-fixture"})
				if err != nil {
					t.Fatal(err)
				}
				bot = channel.ID
			}
			if _, _, err := l.Accept(ledger.Input{BotID: ref, ClientNonce: "ref-source-before", Text: "original source"}); err != nil {
				t.Fatal(err)
			}
			request := func(body map[string]string) (int, map[string]any) {
				b, _ := json.Marshal(body)
				r := post(t, srv.URL+"/app/api/agents/"+bot+"/messages", string(b))
				defer r.Body.Close()
				var v map[string]any
				json.NewDecoder(r.Body).Decode(&v)
				return r.StatusCode, v
			}
			text := "summarize"
			if group {
				text = "@all summarize"
			}
			body := map[string]string{"text": text, "clientNonce": "reference-retry-fixture", "ref": ref}
			code, first := request(body)
			if code != 202 {
				t.Fatal(code, first)
			}
			w, _ := l.Get(first["taskId"].(string))
			if !strings.Contains(w.Text, "original source") || first["text"] != body["text"] {
				t.Fatal("reference was not pinned or exposed in bubble")
			}
			if group {
				code, _ := request(map[string]string{"text": "@meiiie @davin each review", "clientNonce": "selected-members-fixture"})
				if code != 202 {
					t.Fatal("member mentions", code)
				}
				members := map[string]bool{}
				for _, task := range l.Snapshot().Work {
					if strings.HasPrefix(task.ClientNonce, "selected-members-fixture_g") {
						members[task.BotID] = true
					}
				}
				if !members["bot-meiiie"] || !members["bot-davin"] {
					t.Fatal("mention routing collapsed recipients")
				}
			}
			if _, _, err := l.Accept(ledger.Input{BotID: ref, ClientNonce: "ref-source-after", Text: "later unrelated source"}); err != nil {
				t.Fatal(err)
			}
			if group {
				_, _, err = l.SaveChannel(ledger.ChannelInput{ID: channel.ID, Name: channel.Name, BotIDs: []string{"bot-meiiie"}, ExpectedRevision: channel.Revision, ClientNonce: "message-group-changed"})
				if err != nil {
					t.Fatal(err)
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
			code, again := request(body)
			if code != 202 || again["id"] != first["id"] || again["t"] != first["t"] || len(l.Snapshot().Work) != count {
				t.Fatal("receipt changed after restart", code, again)
			}
			current, _ := l.Get(w.RequestID)
			if current.Text != w.Text || strings.Contains(current.Text, "later unrelated source") {
				t.Fatal("reference context changed")
			}
			body["ref"] = "bot-meiiie"
			if code, _ := request(body); code != 409 {
				t.Fatal("changed reference accepted", code)
			}
			body["ref"] = ref
			body["text"] = "changed"
			if code, _ := request(body); code != 409 {
				t.Fatal("changed text accepted", code)
			}
		})
	}
}
