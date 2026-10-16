package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nekobot.local/native/internal/ledger"
)

func TestProjectAPIUploadSelectionCASScopeAndQuarantine(t *testing.T) {
	s, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server, err := New(s, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body any, authenticated bool) *httptest.ResponseRecorder {
		var data []byte
		if body != nil {
			data, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, "http://127.0.0.1:8123"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		if authenticated {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	c, _, err := s.SaveChannel(ledger.ChannelInput{Name: "Quotes", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "api-channel-nonce"})
	if err != nil {
		t.Fatal(err)
	}
	w := request("POST", "/v1/projects", ledger.ProjectInput{Name: "Quote review", Goal: "Compare quotes", ChannelIDs: []string{c.ID}, ClientNonce: "api-project-nonce"}, false)
	if w.Code != 401 {
		t.Fatal("unauthenticated project", w.Code)
	}
	w = request("POST", "/v1/projects", ledger.ProjectInput{Name: "Quote review", Goal: "Compare quotes", ChannelIDs: []string{c.ID}, ClientNonce: "api-project-nonce"}, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var p struct{ Project ledger.Project }
	if err = json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	w = request("POST", "/v1/uploads", ledger.UploadInput{ProjectID: p.Project.ID, ProjectRevision: 1, ChannelID: c.ID, Filename: "quote.txt", Content: []byte("Vendor quote 120 dollars"), ClientNonce: "api-upload-nonce"}, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), "Vendor quote") {
		t.Fatal("upload/status metadata boundary", w.Code, w.Body.String())
	}
	var upload struct{ File ledger.UploadedFile }
	_ = json.Unmarshal(w.Body.Bytes(), &upload)
	w = request("GET", "/v1/projects/"+p.Project.ID+"/files/"+upload.File.ID+"?channelId="+c.ID, nil, true)
	if w.Code != 200 || w.Body.String() != "Vendor quote 120 dollars" || w.Header().Get("X-File-SHA256") != upload.File.SHA256 {
		t.Fatal("download", w.Code)
	}
	wrong := request("GET", "/v1/projects/"+p.Project.ID+"/files/"+upload.File.ID+"?channelId=conversation-bot-davin", nil, true)
	if wrong.Code != 404 {
		t.Fatal("cross lane download", wrong.Code)
	}
	body := map[string]any{"botId": "bot-meiiie", "channelId": c.ID, "expectedChannelRevision": 0, "clientNonce": "api-message-nonce", "text": "@davin compare vendor quote", "lane": "foreground", "projectId": p.Project.ID, "projectRevision": 1}
	before := s.Snapshot().Sequence
	w = request("POST", "/v1/messages", body, true)
	if w.Code != 409 || s.Snapshot().Sequence != before {
		t.Fatal("stale audience committed", w.Code)
	}
	body["expectedChannelRevision"] = 1
	w = request("POST", "/v1/messages", body, true)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var accepted struct{ Work ledger.Work }
	_ = json.Unmarshal(w.Body.Bytes(), &accepted)
	if len(accepted.Work.Resources.Files) != 1 {
		t.Fatal("source was not selected")
	}
	_, _, err = s.SaveChannel(ledger.ChannelInput{ID: c.ID, Name: c.Name, BotIDs: []string{"bot-meiiie"}, ExpectedRevision: 1, ClientNonce: "api-channel-revoke"})
	if err != nil {
		t.Fatal(err)
	}
	w = request("POST", "/v1/messages", body, true)
	if w.Code != 200 {
		t.Fatal("lost ACK original receipt rejected", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &accepted)
	if accepted.Work.Status != "suspended" {
		t.Fatal("duplicate restored authority")
	}
	body["clientNonce"] = "api-message-new-nonce"
	w = request("POST", "/v1/messages", body, true)
	if w.Code != 409 {
		t.Fatal("new stale intake", w.Code)
	}
	body["ownerUserId"] = "forged"
	w = request("POST", "/v1/messages", body, true)
	if w.Code != 400 {
		t.Fatal("forged owner allowed", w.Code)
	}
}
