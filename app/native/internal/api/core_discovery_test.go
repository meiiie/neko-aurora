package api

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestCoreDiscoveryAuthenticatedOwnerMetadataOnly(t *testing.T) {
	h := newFixtureHTTP(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PATHEXT", ".EXE")
	t.Setenv("LOCALAPPDATA", t.TempDir())
	before := h.store.Snapshot().Sequence
	for _, who := range []string{"bob", "reader", "eve"} {
		response, e := h.clients[who].Get(h.server.URL + "/v1/core-discovery")
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 403 || strings.Contains(string(raw), "candidates") || strings.Contains(string(raw), "AppData") {
			t.Fatal("member discovered owner runtime metadata", who, response.StatusCode)
		}
	}
	response, e := h.clients["owner"].Get(h.server.URL + "/v1/core-discovery")
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("owner discovery absent", response.StatusCode)
	}
	var out map[string]any
	if json.NewDecoder(response.Body).Decode(&out) != nil {
		t.Fatal("discovery metadata decode")
	}
	if out["version"] != float64(1) || out["mode"] != "read_only_discovery" || out["usability"] != "not_evaluated" || out["configInspection"] != "not_performed" || out["credentialAccess"] != "not_requested" || out["execution"] != "not_started" || h.store.Snapshot().Sequence != before {
		t.Fatal("discovery mutated authority or overstated readiness")
	}
	for _, name := range []string{"accounts", "secret", "profile", "config", "token"} {
		if _, exists := out[name]; exists {
			t.Fatal("discovery credential/config field", name)
		}
	}
}
