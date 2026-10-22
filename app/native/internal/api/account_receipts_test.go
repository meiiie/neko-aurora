package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/connections"
	"nekoaurora.local/native/internal/ledger"
)

func TestAccountSaveReceiptOriginalNonceReadOnlyAndRetirement(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog, err := connections.Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(store, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	server.EnableConnections(catalog)
	in := connections.AccountInput{ClientNonce: "account-receipt-create", Name: "Local test", Provider: "openai-compatible", Endpoint: "https://example.invalid/v1", Model: "fixture-model"}
	a, _, err := catalog.SaveAccount(in)
	if err != nil {
		t.Fatal(err)
	}
	get := func(query string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8123/v1/account-receipts?"+query, nil)
		if auth {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	if get("clientNonce="+in.ClientNonce, false).Code != 401 {
		t.Fatal("receipt requires local session")
	}
	before := catalog.Snapshot().Revision
	w := get("clientNonce="+in.ClientNonce, true)
	if w.Code != 200 {
		t.Fatalf("account receipt unavailable: %d", w.Code)
	}
	var receipt struct {
		Version                       int
		Found, Used                   bool
		ClientNonce, InputFingerprint string
		Account                       connections.Account
	}
	if json.Unmarshal(w.Body.Bytes(), &receipt) != nil || !receipt.Found || !receipt.Used || receipt.Version != 1 || receipt.ClientNonce != in.ClientNonce || len(receipt.InputFingerprint) != 64 || receipt.Account.ID != a.ID {
		t.Fatal("committed receipt identity")
	}
	if strings.Contains(w.Body.String(), "secretRef") || strings.Contains(w.Body.String(), "hasSubmittedSecret") || catalog.Snapshot().Revision != before {
		t.Fatal("receipt privacy or mutation")
	}
	for _, query := range []string{"clientNonce=unknown-account-command", "clientNonce=" + in.ClientNonce + "&clientNonce=other", "clientNonce=" + in.ClientNonce + "&extra=x", "clientNonce="} {
		out := get(query, true)
		if strings.HasPrefix(query, "clientNonce=unknown") {
			if out.Code != 200 || !strings.Contains(out.Body.String(), `"found":false`) || !strings.Contains(out.Body.String(), `"used":false`) {
				t.Fatal("unknown receipt mutated or fabricated")
			}
		} else if out.Code != 400 {
			t.Fatal("noncanonical receipt query", out.Code)
		}
	}
	in.ID, in.ExpectedRevision, in.ClientNonce, in.Name = a.ID, a.Revision, "account-receipt-edit", "Renamed local"
	if _, _, err = catalog.SaveAccount(in); err != nil {
		t.Fatal(err)
	}
	w = get("clientNonce=account-receipt-create", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"found":false`) || !strings.Contains(w.Body.String(), `"used":true`) || strings.Contains(w.Body.String(), `"account"`) {
		t.Fatal("old receipt silently rebound")
	}
	current := catalog.Snapshot().Accounts[0]
	if err = catalog.RemoveAccount(current.ID, current.Revision); err != nil {
		t.Fatal(err)
	}
	w = get("clientNonce=account-receipt-edit", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"used":true`) || strings.Contains(w.Body.String(), `"account"`) {
		t.Fatal("removed receipt fabricated current account")
	}
}
