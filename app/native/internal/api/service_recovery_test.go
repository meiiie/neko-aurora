package api

import (
	"bytes"
	"encoding/json"
	"nekobot.local/native/internal/ledger"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServiceRecoveryAPIVersionAndStopProofRequired(t *testing.T) {
	s, e := ledger.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	a, e := New(s, "127.0.0.1:8123")
	if e != nil {
		t.Fatal(e)
	}
	w, _, e := s.Accept(ledger.Input{BotID: "bot-meiiie", ClientNonce: "service-api-intake", Text: "Original obligation", Lane: "background"})
	if e != nil {
		t.Fatal(e)
	}
	request := func(body map[string]any, cookie bool) int {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "http://127.0.0.1:8123/v1/task-controls", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if cookie {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: a.session})
		}
		out := httptest.NewRecorder()
		a.ServeHTTP(out, r)
		return out.Code
	}
	body := map[string]any{"version": 2, "action": "recover", "requestId": w.RequestID, "expectedEpoch": w.Epoch, "expectedRevision": w.TaskRevision, "clientNonce": "service-api-recover"}
	before := s.Snapshot().Sequence
	if request(body, false) != 401 || request(body, true) != 409 {
		t.Fatal("unproved recovery accepted")
	}
	body["version"] = 1
	if request(body, true) != 400 {
		t.Fatal("legacy recover intent")
	}
	body["version"] = 2
	body["action"] = "resume"
	if request(body, true) != 400 {
		t.Fatal("version2 broadened old actions")
	}
	body["action"] = "recover"
	body["nativeProcessGeneration"] = "134362178434013773"
	if request(body, true) != 400 {
		t.Fatal("renderer supplied native proof")
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("denied recovery mutated journal")
	}
}
