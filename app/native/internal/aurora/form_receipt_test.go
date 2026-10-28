package aurora

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestFormReceiptRequiresDurableAnswer(t *testing.T) {
	dir := t.TempDir()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s)
	defer srv.Close()
	bot := l.Snapshot().Bots[0].ID
	work, _, err := l.Accept(ledger.Input{BotID: bot, ClientNonce: "form-fixture-source", Text: "collect details"})
	if err != nil {
		t.Fatal(err)
	}
	s.ext.setForm(work.RequestID, &form{Title: "Details", Fields: []formField{{Name: "name", Label: "Name", Type: "text", Required: true}}})
	request := func(path string, body any) (int, map[string]any) {
		b, _ := json.Marshal(body)
		r := post(t, srv.URL+"/app/api"+path, string(b))
		defer r.Body.Close()
		var v map[string]any
		json.NewDecoder(r.Body).Decode(&v)
		return r.StatusCode, v
	}
	done := "/forms/" + work.RequestID + "/done"
	if code, _ := request(done, map[string]string{}); code != 409 {
		t.Fatal("unacknowledged form closed", code)
	}
	path := "/agents/" + bot + "/messages"
	answer := map[string]string{"text": "Name: Aurora", "clientNonce": "form-" + work.RequestID, "formFor": work.RequestID, "workspaceId": s.workspaceID}
	answer["formFor"] = "missing"
	if code, _ := request(path, answer); code != 400 {
		t.Fatal("unknown form", code)
	}
	answer["formFor"] = work.RequestID
	answer["ref"] = "other-chat"
	if code, _ := request(path, answer); code != 400 {
		t.Fatal("mutable context on answer", code)
	}
	delete(answer, "ref")
	code, first := request(path, answer)
	if code != 202 || first["formFor"] != work.RequestID {
		t.Fatal("receipt", code)
	}
	if code, replay := request(path, answer); code != 202 || replay["id"] != first["id"] {
		t.Fatal("receipt replay", code)
	}
	answer["text"] = "replacement"
	if code, _ := request(path, answer); code != 409 {
		t.Fatal("conflicting answer", code)
	}
	if code, _ := request(done, map[string]string{}); code != 200 {
		t.Fatal("accepted form not complete", code)
	}
	_, cached := s.ext.side(work.RequestID)
	if cached.Done {
		t.Fatal("completion must derive from durable receipt")
	}
	s.ext = openExt(filepath.Join(dir, "aurora"))
	m := s.decorate([]*Message{{Role: "agent", TaskID: work.RequestID}})[0]
	if m.Form == nil || !m.Form.Done {
		t.Fatal("receipt lost on side-store reload")
	}
	if len(l.Snapshot().Work) != 2 {
		t.Fatal("duplicate work")
	}

	other, _, err := l.Accept(ledger.Input{BotID: bot, ClientNonce: "form-fixture-failure", Text: "another form"})
	if err != nil {
		t.Fatal(err)
	}
	s.ext.setForm(other.RequestID, &form{Title: "Failure", Fields: cached.Fields})
	l.Close()
	answer["formFor"], answer["clientNonce"] = other.RequestID, "form-"+other.RequestID
	if code, _ := request(path, answer); code < 400 {
		t.Fatal("failed admission acknowledged", code)
	}
	if s.decorate([]*Message{{Role: "agent", TaskID: other.RequestID}})[0].Form.Done {
		t.Fatal("failed admission completed form")
	}
}

func TestFormReplayAfterSideCacheEvictionAndRestart(t *testing.T) {
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
			defer func() { srv.Close(); cancel(); l.Close() }()
			bot := l.Snapshot().Bots[0].ID
			thread := bot
			scope := ledger.SubmissionScope{}
			if group {
				other := "bot-davin"
				if bot == other {
					other = "bot-meiiie"
				}
				channel, _, err := l.SaveChannel(ledger.ChannelInput{Name: "Form replay", BotIDs: []string{bot, other}, ClientNonce: "form-replay-group-fixture"})
				if err != nil {
					t.Fatal(err)
				}
				thread = channel.ID
				scope.ChannelID = channel.ID
				scope.ExpectedChannelRevision = &channel.Revision
			}
			source, _, err := l.AcceptProduct(ledger.Input{BotID: bot, ClientNonce: "form-eviction-source", Text: "collect"}, scope)
			if err != nil {
				t.Fatal(err)
			}
			unanswered, _, err := l.AcceptProduct(ledger.Input{BotID: bot, ClientNonce: "form-eviction-unanswered", Text: "unanswered"}, scope)
			if err != nil {
				t.Fatal(err)
			}
			legacy, _, err := l.AcceptProduct(ledger.Input{BotID: bot, ClientNonce: "form-eviction-legacy", Text: "legacy"}, scope)
			if err != nil {
				t.Fatal(err)
			}
			for _, task := range []string{source.RequestID, unanswered.RequestID, legacy.RequestID} {
				s.ext.setForm(task, &form{Title: "Name", Fields: []formField{{Name: "name", Label: "Name", Type: "text", Required: true}}})
			}
			request := func(path string, body any) (int, map[string]any) {
				r := post(t, srv.URL+"/app/api"+path, mustJSON(body))
				defer r.Body.Close()
				var response map[string]any
				json.NewDecoder(r.Body).Decode(&response)
				return r.StatusCode, response
			}
			path := "/agents/" + thread + "/messages"
			answer := map[string]string{"text": "Name: original", "clientNonce": "form-" + source.RequestID, "formFor": source.RequestID, "workspaceId": s.workspaceID}
			code, first := request(path, answer)
			if code != 202 {
				t.Fatal(code)
			}
			// Exercise the actual eviction policy, then persist/reopen its side store.
			s.ext.mu.Lock()
			for i := 0; i <= maxSide; i++ {
				s.ext.touchLocked(fmt.Sprintf("eviction-%d", i))
			}
			s.ext.saveLocked()
			s.ext.mu.Unlock()
			if _, f := s.ext.side(source.RequestID); f != nil {
				t.Fatal("fixture did not evict form")
			}
			if _, _, err := l.Accept(ledger.Input{BotID: bot, ClientNonce: "form-" + legacy.RequestID, Text: "Name: original"}); err != nil {
				t.Fatal(err)
			}
			count := len(l.Snapshot().Work)
			srv.Close()
			cancel()
			l.Close()
			l, err = ledger.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			restartCtx, restartCancel := context.WithCancel(context.Background())
			defer restartCancel()
			s, err = New(restartCtx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
			if err != nil {
				t.Fatal(err)
			}
			srv = httptest.NewServer(s)
			if code, replay := request(path, answer); code != 202 || replay["id"] != first["id"] {
				t.Fatalf("evicted form lost its journal receipt: status=%d", code)
			}
			if len(l.Snapshot().Work) != count {
				t.Fatal("retry created new work")
			}
			if code, _ := request("/forms/"+source.RequestID+"/done", map[string]string{}); code != 200 {
				t.Fatal("completion lost receipt", code)
			}
			answer["text"] = "changed"
			if code, _ := request(path, answer); code != 409 {
				t.Fatal("changed retry accepted", code)
			}
			answer["text"] = "Name: original"
			answer["workspaceId"] = "other-workspace"
			if code, _ := request(path, answer); code != 409 {
				t.Fatal("cross-workspace replay", code)
			}
			answer["workspaceId"] = s.workspaceID
			if code, _ := request("/agents/not-the-original-bot/messages", answer); code != 400 {
				t.Fatal("cross-chat replay", code)
			}
			for _, task := range []string{unanswered.RequestID, legacy.RequestID} {
				answer["clientNonce"] = "form-" + task
				answer["formFor"] = task
				if code, _ := request(path, answer); code != 400 {
					t.Fatal("missing owner receipt accepted", code)
				}
				if code, _ := request("/forms/"+task+"/done", map[string]string{}); code != 404 {
					t.Fatal("completion fabricated from missing form or legacy nonce", code)
				}
			}

		})
	}
}
