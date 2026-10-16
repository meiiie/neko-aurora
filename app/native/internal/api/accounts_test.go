package api

import (
	"encoding/json"
	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/ledger"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type localAuthorityVault struct{}

func (localAuthorityVault) Seal(string, string, []byte) error          { return nil }
func (localAuthorityVault) Match(string, string, []byte) (bool, error) { return true, nil }

func TestAccountSelectionServerBindingAndRevokeACK(t *testing.T) {
	dir := t.TempDir()
	store, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog, err := connections.Open(filepath.Join(dir, "connections"), localAuthorityVault{})
	if err != nil {
		t.Fatal(err)
	}
	account, _, err := catalog.SaveAccount(connections.AccountInput{ClientNonce: "api-account-fixture", Name: "Fixture Account", Provider: "openai-compatible", Endpoint: "https://example.invalid/v1", Model: "fixture-model", Secret: "fixture-only-not-a-key"})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(store, "127.0.0.1:8765")
	if err != nil {
		t.Fatal(err)
	}
	handler.EnableConnections(catalog)
	call := func(path string, body any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "http://127.0.0.1:8765"+path, strings.NewReader(string(b)))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: "neko_local", Value: handler.session})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	message := map[string]any{"botId": "bot-meiiie", "clientNonce": "api-bound-fixture", "text": "make local fixture report", "accountId": account.ID, "accountRevision": account.Revision}
	reply := call("/v1/messages", message)
	if reply.Code != 201 {
		t.Fatal("selected account not accepted", reply.Code, reply.Body.String())
	}
	var ack struct{ Work ledger.Work }
	json.Unmarshal(reply.Body.Bytes(), &ack)
	if ack.Work.Account == nil || ack.Work.Account.AccountID != account.ID || ack.Work.Account.Revision != account.Revision || ack.Work.Account.CredentialGeneration != 1 {
		t.Fatal("server did not pin public authority")
	}
	message["accountPin"] = map[string]any{"accountId": "forged", "revision": 9}
	if reply = call("/v1/messages", message); reply.Code != 400 {
		t.Fatal("client-supplied authority allowed", reply.Code)
	}
	delete(message, "accountPin")
	reply = call("/v1/accounts/remove", map[string]any{"id": account.ID, "expectedRevision": account.Revision})
	if reply.Code != 200 {
		t.Fatal(reply.Code)
	}
	current, _ := store.Get(ack.Work.RequestID)
	if current.Status != "suspended" || current.DesiredState != "suspended" {
		t.Fatal("remove ACK preceded suspension")
	}
	if reply = call("/v1/messages", message); reply.Code != 200 {
		t.Fatal("lost message ACK not idempotent", reply.Code)
	}
	message["clientNonce"] = "another-bound-fixture"
	if reply = call("/v1/messages", message); reply.Code != 409 {
		t.Fatal("deleted selection accepted", reply.Code)
	}
}
