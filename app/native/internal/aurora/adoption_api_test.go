package aurora

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestAdoptionAPIQuestionsMemorySchedulesAndRetry(t *testing.T) {
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
	request := func(path string, body any) (int, map[string]any) {
		b, _ := json.Marshal(body)
		r := post(t, srv.URL+"/app/api"+path, string(b))
		defer r.Body.Close()
		var v map[string]any
		json.NewDecoder(r.Body).Decode(&v)
		return r.StatusCode, v
	}
	path := "/agents/" + bot
	if code, _ := request(path+"/messages", map[string]string{"text": "wrong account", "clientNonce": "scope-fixture-123", "workspaceId": "other-workspace"}); code != 409 || len(l.Snapshot().Work) != 0 {
		t.Fatal("scope gate", code)
	}
	body := map[string]string{"text": "question fixture", "clientNonce": "question-fixture-123", "workspaceId": s.workspaceID}
	code, first := request(path+"/messages", body)
	if code != 202 {
		t.Fatal(code)
	}
	work, _ := l.Get(first["taskId"].(string))
	work, err = l.DirectStart(work.RequestID, work.Epoch, ledger.DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	work, err = l.DirectFinish(work.RequestID, work.Epoch, "completed", "", func(d *ledger.DirectRun) {
		d.Choices = []string{"Sáng", "Chiều"}
		d.Approval = &ledger.DirectApproval{Kind: "send", Title: "Chờ chủ duyệt", Body: "Fixture", State: "pending"}
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, again := request(path+"/messages", body); code != 202 || again["id"] != first["id"] {
		t.Fatal("receipt replay", code)
	}
	current, _ := l.Get(work.RequestID)
	if current.Direct.Approval.State != "pending" {
		t.Fatal("duplicate superseded approval")
	}
	answer := map[string]string{"text": "Tối cuối tuần", "clientNonce": "choice-" + work.RequestID, "answerFor": work.RequestID, "workspaceId": s.workspaceID}
	code, receipt := request(path+"/messages", answer)
	if code != 202 || receipt["answerFor"] != work.RequestID {
		t.Fatal("custom answer", code)
	}
	if code, replay := request(path+"/messages", answer); code != 202 || replay["id"] != receipt["id"] {
		t.Fatal("answer replay", code)
	}
	answer["text"] = "different"
	if code, _ := request(path+"/messages", answer); code != 409 {
		t.Fatal("second answer accepted", code)
	}
	answer["answerFor"] = "missing"
	if code, _ := request(path+"/messages", answer); code != 400 {
		t.Fatal("invalid question accepted", code)
	}
	code, preview := request(path+"/memory/import/preview", map[string]string{"text": "Dùng tiếng Việt\nThích câu trả lời ngắn"})
	if code != 200 {
		t.Fatal("preview", code)
	}
	apply := map[string]any{"revision": preview["revision"], "items": []string{"Dùng tiếng Việt"}}
	if code, _ := request(path+"/memory/import", apply); code != 200 {
		t.Fatal("import", code)
	}
	if code, _ := request(path+"/memory/import", apply); code != 409 {
		t.Fatal("stale memory", code)
	}
	if !s.sched.add(schedule{ID: "api-fixture", Bot: bot, Text: "tin sáng", At: "08:00", Repeat: "daily"}) {
		t.Fatal("schedule setup")
	}
	if code, _ := request("/schedules/api-fixture/state", map[string]any{"workspaceId": s.workspaceID, "revision": 1, "paused": true}); code != 200 {
		t.Fatal("pause", code)
	}
	if code, _ := request("/schedules/api-fixture/delete", map[string]any{"workspaceId": s.workspaceID, "revision": 1}); code != 409 {
		t.Fatal("stale delete", code)
	}
	if code, _ := request("/schedules/api-fixture/state", map[string]any{"workspaceId": s.workspaceID, "revision": 2, "paused": false}); code != 200 {
		t.Fatal("resume", code)
	}
	if code, _ := request("/schedules/api-fixture/delete", map[string]any{"workspaceId": s.workspaceID, "revision": 3}); code != 200 {
		t.Fatal("delete", code)
	}
	if len(s.sched.list(bot)) != 0 {
		t.Fatal("delete not stored")
	}
	res, err := http.Get(srv.URL + "/app/api/agents/" + bot + "/messages")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var messages []Message
	if json.NewDecoder(res.Body).Decode(&messages) != nil {
		t.Fatal("history")
	}
	found := false
	for _, m := range messages {
		if m.AnswerFor == work.RequestID {
			found = true
		}
	}
	if !found {
		t.Fatal("answer receipt missing from reloaded history")
	}
}
