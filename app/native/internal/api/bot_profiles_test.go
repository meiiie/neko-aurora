package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBotProfileIntakePinsPreferencesAndHistoryWithoutRewritingWork(t *testing.T) {
	server, client := setup(t)
	first := post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","clientNonce":"profile-before-request","text":"Original request","lane":"background"}`, "")
	defer first.Body.Close()
	if first.StatusCode != 201 {
		t.Fatal(first.StatusCode)
	}
	var old struct {
		Work map[string]any `json:"work"`
	}
	if json.NewDecoder(first.Body).Decode(&old) != nil {
		t.Fatal("original intake")
	}
	body := `{"instructions":"Answer concisely in Vietnamese.","includeHistory":false,"expectedRevision":0,"clientNonce":"profile-config-first"}`
	r := post(t, client, server.URL, "/v1/bots/bot-meiiie/profile", body, "")
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("profile endpoint missing: %d", r.StatusCode)
	}
	var saved struct {
		Profile   map[string]any `json:"profile"`
		Duplicate bool           `json:"duplicate"`
	}
	if json.NewDecoder(r.Body).Decode(&saved) != nil || saved.Profile["revision"] != float64(1) || saved.Profile["includeHistory"] != false || saved.Duplicate {
		t.Fatal(saved)
	}
	second := post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","clientNonce":"profile-after-request","text":"New request","lane":"foreground"}`, "")
	defer second.Body.Close()
	var newer struct {
		Work map[string]any `json:"work"`
	}
	if second.StatusCode != 201 || json.NewDecoder(second.Body).Decode(&newer) != nil {
		t.Fatal("profile intake", second.StatusCode)
	}
	pin, ok := newer.Work["botProfilePin"].(map[string]any)
	if !ok || pin["revision"] != float64(1) || pin["instructions"] != "Answer concisely in Vietnamese." || pin["includeHistory"] != false {
		t.Fatal("missing frozen profile", newer)
	}
	retry := post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","clientNonce":"profile-before-request","text":"Original request","lane":"background"}`, "")
	defer retry.Body.Close()
	var original struct {
		Work      map[string]any `json:"work"`
		Duplicate bool           `json:"duplicate"`
	}
	if retry.StatusCode != 200 || json.NewDecoder(retry.Body).Decode(&original) != nil || !original.Duplicate || original.Work["botProfilePin"] != nil || original.Work["requestId"] != old.Work["requestId"] {
		t.Fatal("profile rewrote original work", retry.StatusCode, original.Duplicate, original.Work["requestId"], old.Work["requestId"], original.Work["botProfilePin"])
	}
	for _, invalid := range []string{
		`{"includeHistory":true,"expectedRevision":1,"clientNonce":"profile-missing-instructions"}`,
		`{"instructions":"New","includeHistory":true,"clientNonce":"profile-missing-cas"}`,
		`{"instructions":"New","expectedRevision":1,"clientNonce":"profile-missing-boolean"}`,
		`{"instructions":"New","includeHistory":true,"expectedRevision":1,"clientNonce":"profile-extra-authority","grant":true}`,
	} {
		response := post(t, client, server.URL, "/v1/bots/bot-meiiie/profile", invalid, "")
		response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatal("profile accepted incomplete or authority fields", response.StatusCode)
		}
	}
	stale := post(t, client, server.URL, "/v1/bots/bot-meiiie/profile", `{"instructions":"Other","includeHistory":true,"expectedRevision":0,"clientNonce":"profile-stale-config"}`, "")
	defer stale.Body.Close()
	if stale.StatusCode != 409 {
		t.Fatal("stale settings accepted", stale.StatusCode)
	}
}
