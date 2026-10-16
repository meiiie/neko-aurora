package api

import (
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/ledger"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdapterInstallationCatalogAuthenticationReadonlyAndStrictQuery(t *testing.T) {
	store, e := ledger.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	s, e := New(store, "127.0.0.1:8765")
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, path, origin, host string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+s.host+path, nil)
		r.Host = host
		r.Header.Set("Origin", origin)
		if auth {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: s.session})
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	path := "/v1/adapters/installation-catalog"
	if call("GET", path, "", s.host, false).Code != 401 || call("GET", path, "http://evil.invalid", s.host, true).Code != 403 || call("GET", path, "", "evil.invalid", true).Code != 403 {
		t.Fatal("auth bypass")
	}
	if call("GET", path, "", s.host, true).Code != 503 {
		t.Fatal("unconfigured registry accepted")
	}
	d := executors.Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: strings.Repeat("2", 64), RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)}
	reg, e := executors.Open(filepath.Join(t.TempDir(), "installation"), d)
	if e != nil {
		t.Fatal(e)
	}
	defer reg.Close()
	store.ConfigureExecutor(reg)
	before := store.Snapshot().Sequence
	out := call("GET", path, "http://"+s.host, s.host, true)
	if out.Code != 200 || out.Header().Get("Cache-Control") != "no-store" || strings.Contains(out.Body.String(), "bootId") {
		t.Fatal(out.Code, out.Body.String())
	}
	for _, q := range []string{"?after=0", "?projectId=unknown", "?x=1&x=2"} {
		if call("GET", path+q, "", s.host, true).Code != 400 {
			t.Fatal("query accepted", q)
		}
	}
	if store.Snapshot().Sequence != before {
		t.Fatal("lookup committed")
	}
}
