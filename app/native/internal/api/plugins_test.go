package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestPluginLifecycleBoundaryAndSelection(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := New(store, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	request := func(path string, body any, cookie bool, origin string) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "http://127.0.0.1:8123"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if cookie {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		}
		out := httptest.NewRecorder()
		server.ServeHTTP(out, r)
		return out
	}
	body := map[string]any{"version": 1, "id": "neko-artifacts", "action": "disable", "expectedRevision": 0, "clientNonce": "api-plugin-disable"}
	if r := request("/v1/plugin-lifecycle", body, false, ""); r.Code != 401 {
		t.Fatal("unauthenticated lifecycle", r.Code)
	}
	if r := request("/v1/plugin-lifecycle", body, true, "https://example.invalid"); r.Code != 403 {
		t.Fatal("foreign origin", r.Code)
	}
	if r := request("/v1/plugin-lifecycle", body, true, ""); r.Code != 200 {
		t.Fatal("missing lifecycle", r.Code, r.Body.String())
	}
	var result struct {
		Plugin    ledger.PluginRecord
		Duplicate bool
	}
	r := request("/v1/plugin-lifecycle", body, true, "")
	_ = json.Unmarshal(r.Body.Bytes(), &result)
	if r.Code != 200 || !result.Duplicate || result.Plugin.Revision != 1 {
		t.Fatal("lost ACK", r.Code)
	}
	delete(body, "expectedRevision")
	body["clientNonce"] = "api-plugin-missing"
	if r = request("/v1/plugin-lifecycle", body, true, ""); r.Code != 400 {
		t.Fatal("missing CAS", r.Code)
	}
	body["expectedRevision"] = 0
	body["action"] = "enable"
	if r = request("/v1/plugin-lifecycle", body, true, ""); r.Code != 409 {
		t.Fatal("stale CAS", r.Code)
	}
	body["expectedRevision"] = 1
	body["execute"] = "external command"
	if r = request("/v1/plugin-lifecycle", body, true, ""); r.Code != 400 {
		t.Fatal("executable injection", r.Code)
	}
	msg := map[string]any{"botId": "bot-meiiie", "text": "plain task", "lane": "foreground", "clientNonce": "api-plugin-empty", "pluginSelection": []any{}}
	r = request("/v1/messages", msg, true, "")
	if r.Code != 201 {
		t.Fatal("explicit tool-free intake", r.Code, r.Body.String())
	}
	var work struct{ Work ledger.Work }
	_ = json.Unmarshal(r.Body.Bytes(), &work)
	if work.Work.ToolPolicyVersion != 1 || work.Work.PluginSelection == nil || len(work.Work.PluginPins) != 0 {
		t.Fatal("empty/default selection collapsed")
	}
	msg["pluginSelection"] = []any{map[string]any{"id": "neko-artifacts", "expectedRevision": 0}}
	if r = request("/v1/messages", msg, true, ""); r.Code != 409 {
		t.Fatal("retry changed policy", r.Code)
	}
	msg["clientNonce"] = "api-plugin-missing-pin"
	msg["pluginSelection"] = []any{map[string]any{"id": "neko-artifacts"}}
	if r = request("/v1/messages", msg, true, ""); r.Code != 400 {
		t.Fatal("missing selection revision", r.Code)
	}
	msg["pluginSelection"] = []any{map[string]any{"id": "neko-artifacts", "expectedRevision": 0, "grant": true}}
	if r = request("/v1/messages", msg, true, ""); r.Code != 400 {
		t.Fatal("selection authority injection", r.Code)
	}
}
