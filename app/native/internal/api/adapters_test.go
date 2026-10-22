package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestAdapterAPIAuthenticatedStrictOriginAndOpaqueReturnRoute(t *testing.T) {
	s, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server, err := New(s, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := s.SaveChannel(ledger.ChannelInput{Name: "Wiii shared", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "api-adapter-channel"})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body any, auth bool, origin string) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://127.0.0.1:8123"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if auth {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	input := map[string]any{"sourceApp": "wiii", "sourceSessionId": "wiii-session", "sourceThreadId": "wiii-thread", "channelId": c.ID, "expectedChannelRevision": c.Revision, "clientNonce": "api-adapter-register"}
	if w := request("POST", "/v1/adapters/sessions", input, false, ""); w.Code != 401 {
		t.Fatal("auth", w.Code)
	}
	if w := request("POST", "/v1/adapters/sessions", input, true, "http://foreign.invalid"); w.Code != 403 {
		t.Fatal("cross-origin", w.Code)
	}
	input["returnRoute"] = "https://external.invalid/exfil"
	if w := request("POST", "/v1/adapters/sessions", input, true, ""); w.Code != 400 {
		t.Fatal("caller-controlled destination", w.Code)
	}
	delete(input, "returnRoute")
	w := request("POST", "/v1/adapters/sessions", input, true, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct{ Session ledger.AdapterSession }
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	message := map[string]any{"botId": "bot-meiiie", "text": "@davin answer here", "lane": "foreground", "clientNonce": "api-source-nonce", "sourceMessageId": "wiii-message"}
	path := "/v1/adapters/sessions/" + result.Session.ID + "/messages"
	message["coreSessionId"] = "forged-core-authority"
	if w = request("POST", path, message, true, ""); w.Code != 400 {
		t.Fatal("Core authority accepted", w.Code)
	}
	delete(message, "coreSessionId")
	w = request("POST", path, message, true, "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var accepted struct{ Work ledger.Work }
	_ = json.Unmarshal(w.Body.Bytes(), &accepted)
	if accepted.Work.Adapter.SourceThreadID != "wiii-thread" || accepted.Work.Adapter.ClientNonce != "api-source-nonce" || accepted.Work.ClientNonce == "api-source-nonce" {
		t.Fatal("source identity not server-bound", accepted)
	}
	if w = request("POST", path, message, true, ""); w.Code != 200 {
		t.Fatal("lost ACK", w.Code)
	}
	if w = request("GET", result.Session.ReturnRoute+"?after=999", nil, true, ""); w.Code != 409 {
		t.Fatal("ahead cursor", w.Code)
	}
	if w = request("GET", result.Session.ReturnRoute+"?after=-1", nil, true, ""); w.Code != 400 {
		t.Fatal("invalid cursor", w.Code)
	}
	w = request("GET", result.Session.ReturnRoute, nil, true, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
