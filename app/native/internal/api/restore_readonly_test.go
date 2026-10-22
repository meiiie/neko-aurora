package api

import (
	"bytes"
	"io"
	"nekoaurora.local/native/internal/ledger"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoredAPINonGETRefusedBeforeSecureCatalog(t *testing.T) {
	dir := t.TempDir()
	store, e := ledger.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	store.Close()
	os.WriteFile(filepath.Join(dir, "restore-readonly.json"), []byte(`{"version":1,"executionDisabled":true,"manifestSHA256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`), 0600)
	store, e = ledger.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	server := httptest.NewUnstartedServer(nil)
	handler, e := New(store, server.Listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	server.Config.Handler = handler
	server.Start()
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, e := client.Get(server.URL + "/")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	for _, path := range []string{"/v1/messages", "/v1/conversations", "/v1/connections/accounts", "/v1/executors/recovery", "/v1/collaborations"} {
		request, _ := http.NewRequest("POST", server.URL+path, bytes.NewBufferString(`{"secret":"fixture-only-no-host-call"}`))
		request.Header.Set("Content-Type", "application/json")
		response, e = client.Do(request)
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 403 || !bytes.Contains(body, []byte("restored_state_read_only")) {
			t.Fatal(path, response.StatusCode)
		}
	}
	response, e = client.Get(server.URL + "/v1/snapshot")
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte(`"readOnly":true`)) {
		t.Fatal("read-only snapshot missing")
	}
	if _, e = os.Stat(filepath.Join(dir, "connections")); !os.IsNotExist(e) {
		t.Fatal("catalog initialized by blocked POST")
	}
}
