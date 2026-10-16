package api

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestSharedHistoryExportScopesPublicMessages(t *testing.T) {
	server, client := setup(t)
	r := post(t, client, server.URL, "/v1/channels", `{"name":"Public history","botIds":["bot-meiiie","bot-davin"],"expectedRevision":0,"clientNonce":"export-shared-channel"}`, "")
	var v map[string]any
	json.NewDecoder(r.Body).Decode(&v)
	r.Body.Close()
	id := v["channel"].(map[string]any)["id"].(string)
	for _, body := range []string{`{"botId":"bot-meiiie","clientNonce":"export-public-message","text":"PUBLIC_SHARED_CANARY","lane":"background","channelId":"` + id + `"}`, `{"botId":"bot-davin","clientNonce":"export-private-message","text":"PRIVATE_HISTORY_CANARY","lane":"background"}`} {
		r = post(t, client, server.URL, "/v1/messages", body, "")
		r.Body.Close()
		if r.StatusCode != 201 {
			t.Fatal(r.StatusCode)
		}
	}
	response, err := client.Get(server.URL + "/v1/channels/" + id + "/export?format=json")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		t.Fatalf("shared export missing: %d", response.StatusCode)
	}
	if !strings.Contains(string(raw), "PUBLIC_SHARED_CANARY") || strings.Contains(string(raw), "PRIVATE_HISTORY_CANARY") || strings.Contains(string(raw), "contextPin") || strings.Contains(string(raw), "accountPin") {
		t.Fatal("export scope or projection")
	}
	if !strings.Contains(response.Header.Get("Content-Disposition"), "attachment;") {
		t.Fatal("not a download")
	}
}
