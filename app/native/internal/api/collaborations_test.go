package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestCollaborationAPIOwnerNonceAndCASBoundary(t *testing.T) {
	s, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server, err := New(s, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := s.SaveChannel(ledger.ChannelInput{Name: "Review", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "collab-api-channel"})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body any, auth bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://127.0.0.1:8123"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	body := map[string]any{"channelId": c.ID, "expectedChannelRevision": c.Revision, "participants": []string{"bot-meiiie", "bot-davin"}, "coordinatorBotId": "bot-meiiie", "goal": "Compare two local plans", "maxStages": 3, "clientNonce": "collab-api-create"}
	if w := request("POST", "/v1/collaborations", body, false); w.Code != 401 {
		t.Fatal("owner session", w.Code)
	}
	w := request("POST", "/v1/collaborations", body, true)
	if w.Code != 201 {
		t.Fatal("collaboration intake missing", w.Code, w.Body.String())
	}
	var result struct {
		Collaboration struct {
			ID       string
			Revision uint64
		}
		Duplicate bool
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Collaboration.ID == "" || result.Collaboration.Revision != 1 {
		t.Fatal("receipt", err)
	}
	if w = request("POST", "/v1/collaborations", body, true); w.Code != 200 {
		t.Fatal("lost ACK retry", w.Code)
	}
	body["goal"] = "Changed goal"
	before := s.Snapshot().Sequence
	if w = request("POST", "/v1/collaborations", body, true); w.Code != 409 || s.Snapshot().Sequence != before {
		t.Fatal("nonce conflict committed", w.Code)
	}
	body["clientNonce"] = "collab-api-stale-audience"
	body["expectedChannelRevision"] = 0
	if w = request("POST", "/v1/collaborations", body, true); w.Code != 409 || s.Snapshot().Sequence != before {
		t.Fatal("stale intake CAS", w.Code)
	}
	body["expectedChannelRevision"] = c.Revision
	body["clientNonce"] = "collab-api-forged"
	body["ownerUserId"] = "someone-else"
	if w = request("POST", "/v1/collaborations", body, true); w.Code != 400 {
		t.Fatal("forged actor accepted", w.Code)
	}
	path := "/v1/collaborations/" + result.Collaboration.ID + "/cancel"
	if w = request("POST", path, map[string]any{"expectedRevision": 0, "clientNonce": "collab-api-cancel"}, true); w.Code != 409 {
		t.Fatal("stale group CAS", w.Code)
	}
	if w = request("POST", path, map[string]any{"expectedRevision": 1, "clientNonce": "collab-api-cancel"}, true); w.Code != 200 {
		t.Fatal("cancel", w.Code, w.Body.String())
	}
	if w = request("GET", "/v1/collaborations", nil, true); w.Code != 200 {
		t.Fatal("group read", w.Code)
	}
}
