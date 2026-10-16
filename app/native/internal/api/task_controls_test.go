package api

import (
	"bytes"
	"encoding/json"
	"nekobot.local/native/internal/ledger"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTaskControlAPIAuthenticationCASAndOfflineCancelFence(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := New(store, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	request := func(body any, cookie bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "http://127.0.0.1:8123/v1/task-controls", bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		if cookie {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, r)
		return response
	}
	w, _, err := store.Accept(ledger.Input{BotID: "bot-meiiie", ClientNonce: "api-control-intake", Text: "Keep working", Lane: "background"})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"version": 1, "requestId": w.RequestID, "expectedEpoch": w.Epoch, "expectedRevision": 0, "action": "pause", "clientNonce": "api-control-pause"}
	if r := request(body, false); r.Code != 401 {
		t.Fatal("anonymous control", r.Code)
	}
	if r := request(body, true); r.Code != 200 {
		t.Fatal("pause route unavailable", r.Code, r.Body.String())
	}
	var result struct {
		Work      ledger.Work
		Duplicate bool
	}
	r := request(body, true)
	_ = json.Unmarshal(r.Body.Bytes(), &result)
	if r.Code != 200 || !result.Duplicate || result.Work.Status != "paused" {
		t.Fatal("pause lost ACK")
	}
	missing := map[string]any{"version": 1, "requestId": w.RequestID, "action": "resume", "clientNonce": "api-control-missing"}
	if r = request(missing, true); r.Code != 400 {
		t.Fatal("missing CAS", r.Code)
	}
	body["action"] = "resume"
	body["clientNonce"] = "api-control-stale"
	if r = request(body, true); r.Code != 409 {
		t.Fatal("stale control", r.Code)
	}
	body["expectedEpoch"] = result.Work.Epoch
	body["expectedRevision"] = result.Work.TaskRevision
	body["action"] = "cancel"
	body["clientNonce"] = "api-control-cancel"
	if r = request(body, true); r.Code != 200 {
		t.Fatal("cancel", r.Code, r.Body.String())
	}
	body["action"] = "supplement"
	body["text"] = "Offline appendix"
	body["clientNonce"] = "api-control-offline"
	before := store.Snapshot().Sequence
	if r = request(body, true); r.Code != 409 || store.Snapshot().Sequence != before {
		t.Fatal("offline revive", r.Code)
	}
	body["ownerUserId"] = "forged"
	if r = request(body, true); r.Code != 400 {
		t.Fatal("owner forgery", r.Code)
	}
}
