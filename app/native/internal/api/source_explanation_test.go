package api

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestTaskSourceExplanationIsReadOnlyScopedMetadata(t *testing.T) {
	server, client := setup(t)
	r := post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","clientNonce":"explain-original-message","text":"Authorized private request canary","lane":"background"}`, "")
	var v map[string]any
	json.NewDecoder(r.Body).Decode(&v)
	r.Body.Close()
	id := v["work"].(map[string]any)["requestId"].(string)
	response, err := client.Get(server.URL + "/v1/tasks/" + id + "/sources?channelId=conversation-bot-meiiie")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		t.Fatalf("source explanation missing: %d", response.StatusCode)
	}
	if strings.Contains(string(raw), "Authorized private request canary") || strings.Contains(string(raw), "contextPin") || strings.Contains(string(raw), "runtimeContext") || strings.Contains(string(raw), "secret") {
		t.Fatal("source explanation leaks raw data")
	}
	if !strings.Contains(string(raw), "owner_request") {
		t.Fatal("no selection explanation")
	}
	wrong, _ := client.Get(server.URL + "/v1/tasks/" + id + "/sources?channelId=conversation-bot-davin")
	wrong.Body.Close()
	if wrong.StatusCode != 409 {
		t.Fatal("wrong destination exposed", wrong.StatusCode)
	}
}
