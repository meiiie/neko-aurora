package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestLocalBotRenameKeepsStableIdentityAndOriginalCreateReceipt(t *testing.T) {
	server, client := setup(t)
	response := post(t, client, server.URL, "/v1/bots", `{"name":"Research colleague","alias":"research","description":"Own research history","clientNonce":"owned-create-research"}`, "")
	defer response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatal(response.StatusCode)
	}
	var created struct {
		Bot map[string]any `json:"bot"`
	}
	if json.NewDecoder(response.Body).Decode(&created) != nil {
		t.Fatal("create receipt")
	}
	id := created.Bot["id"].(string)
	body := `{"name":"Davin Research","expectedDisplayRevision":0,"clientNonce":"owned-rename-research"}`
	response = post(t, client, server.URL, "/v1/bots/"+id+"/name", body, "")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stable bot rename endpoint absent or rejected: %d", response.StatusCode)
	}
	var renamed struct {
		Bot       map[string]any `json:"bot"`
		Duplicate bool           `json:"duplicate"`
	}
	if json.NewDecoder(response.Body).Decode(&renamed) != nil || renamed.Bot["id"] != id || renamed.Bot["alias"] != "research" || renamed.Bot["name"] != "Davin Research" || renamed.Bot["revision"] != float64(1) || renamed.Bot["displayRevision"] != float64(1) || renamed.Duplicate {
		t.Fatal("display change altered identity/runtime revision", renamed)
	}
	response = post(t, client, server.URL, "/v1/bots/"+id+"/name", body, "")
	defer response.Body.Close()
	if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&renamed) != nil || !renamed.Duplicate {
		t.Fatal("rename retry changed intent")
	}
	response = post(t, client, server.URL, "/v1/bots", `{"name":"Research colleague","alias":"research","description":"Own research history","clientNonce":"owned-create-research"}`, "")
	defer response.Body.Close()
	if response.StatusCode != 201 || json.NewDecoder(response.Body).Decode(&renamed) != nil || renamed.Bot["id"] != id || renamed.Bot["name"] != "Davin Research" || !renamed.Duplicate {
		t.Fatal("rename invalidated original create receipt")
	}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"name":"Another","expectedDisplayRevision":0,"clientNonce":"owned-rename-stale"}`, 409},
		{`{"name":"Different","expectedDisplayRevision":0,"clientNonce":"owned-rename-research"}`, 409},
		{`{"name":"Meiiie","expectedDisplayRevision":1,"clientNonce":"owned-rename-collision"}`, 409},
		{`{"name":"Changed","clientNonce":"owned-rename-no-revision"}`, 400},
		{`{"name":"Changed","expectedDisplayRevision":1,"clientNonce":"owned-rename-forged","alias":"other"}`, 400},
	} {
		t.Run(fmt.Sprint(tc.status)+tc.body[len(tc.body)-12:], func(t *testing.T) {
			r := post(t, client, server.URL, "/v1/bots/"+id+"/name", tc.body, "")
			defer r.Body.Close()
			if r.StatusCode != tc.status {
				t.Fatal(r.StatusCode, tc.status)
			}
		})
	}
}
