package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nekobot.local/native/internal/ledger"
)

func TestReadOnlyReceiptAndCatalogAuthenticationBoundary(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := New(store, "127.0.0.1:8765")
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := store.Accept(ledger.Input{BotID: "bot-meiiie", Text: "original command", Lane: "background", ClientNonce: "api-receipt-source"})
	if err != nil {
		t.Fatal(err)
	}
	in := ledger.TaskControlInput{Version: 1, RequestID: w.RequestID, ExpectedEpoch: w.Epoch, Action: "pause", ClientNonce: "api-receipt-pause"}
	if _, _, err := store.ControlTask(in); err != nil {
		t.Fatal(err)
	}
	request := func(path, origin, host string, authenticated bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8765"+path, nil)
		r.Host = host
		if authenticated {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: s.session})
		}
		r.Header.Set("Origin", origin)
		out := httptest.NewRecorder()
		s.ServeHTTP(out, r)
		return out
	}
	for _, path := range []string{"/v1/adapters/catalog", "/v1/command-receipts?kind=task_control&clientNonce=api-receipt-pause"} {
		if request(path, "", s.host, false).Code != 401 || request(path, "http://evil.test", s.host, true).Code != 403 || request(path, "", "evil.test", true).Code != 403 {
			t.Fatal("read-only route bypassed session boundary", path)
		}
		out := request(path, "http://"+s.host, s.host, true)
		if out.Code != 200 || out.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(out.Code, out.Body.String())
		}
	}
	before := store.Snapshot().Sequence
	for _, path := range []string{"/v1/adapters/catalog?after=0", "/v1/command-receipts?kind=task_control&kind=collaboration_create&clientNonce=x", "/v1/command-receipts?kind=task_control&clientNonce=x&accountId=unknown", "/v1/command-receipts?kind=collaboration_cancel&clientNonce=x"} {
		if out := request(path, "", s.host, true); out.Code != 400 {
			t.Fatal("ambiguous query accepted", path, out.Code)
		}
	}
	out := request("/v1/command-receipts?kind=task_control&clientNonce=api-receipt-pause", "", s.host, true)
	var receipt ledger.CommandReceipt
	if json.Unmarshal(out.Body.Bytes(), &receipt) != nil || !receipt.Found || receipt.Work.RequestID != w.RequestID || strings.Contains(out.Body.String(), "contextPin") {
		t.Fatal(out.Body.String())
	}
	out = request("/v1/command-receipts?kind=task_control&clientNonce=never-posted", "", s.host, true)
	if out.Code != 200 || strings.Contains(out.Body.String(), "\"found\":true") || strings.Contains(out.Body.String(), "\"input\"") || store.Snapshot().Sequence != before {
		t.Fatal("lookup mutation", out.Body.String())
	}
}
