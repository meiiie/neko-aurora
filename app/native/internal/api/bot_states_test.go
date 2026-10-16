package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestBotArchiveBlocksFreshIntakeKeepsOriginalACKAndHistory(t *testing.T) {
	server, client := setup(t)
	message := `{"botId":"bot-meiiie","clientNonce":"bot-state-old-background","text":"Keep the accepted obligation","lane":"background"}`
	read := func(path, body string) (int, map[string]any) {
		r := post(t, client, server.URL, path, body, "")
		defer r.Body.Close()
		var data map[string]any
		json.NewDecoder(r.Body).Decode(&data)
		return r.StatusCode, data
	}
	status, first := read("/v1/messages", message)
	if status != 201 {
		t.Fatal(status)
	}
	status, saved := read("/v1/bots/bot-meiiie/state", `{"action":"archive","expectedRevision":0,"clientNonce":"bot-state-archive-first"}`)
	if status != http.StatusOK {
		t.Fatalf("bot archive endpoint missing: %d", status)
	}
	state, ok := saved["botState"].(map[string]any)
	if !ok || state["archived"] != true || state["revision"] != float64(1) {
		t.Fatal("archive metadata not stored", saved)
	}
	status, duplicate := read("/v1/messages", message)
	if status != 200 || duplicate["duplicate"] != true || !reflect.DeepEqual(first["work"], duplicate["work"]) {
		t.Fatal("archive changed original ACK", status)
	}
	for _, body := range []string{
		`{"botId":"bot-meiiie","clientNonce":"bot-state-fresh-direct","text":"Fresh must stop","lane":"foreground"}`,
		`{"botId":"bot-davin","clientNonce":"bot-state-fresh-route","text":"@meiiie Fresh route must stop","lane":"foreground"}`,
	} {
		if status, _ = read("/v1/messages", body); status != 409 {
			t.Fatal("archived bot accepts fresh intake", status)
		}
	}
	status, _ = read("/v1/bots/bot-meiiie/state", `{"action":"restore","expectedRevision":1,"clientNonce":"bot-state-restore-first"}`)
	if status != 200 {
		t.Fatal("restore failed", status)
	}
	if status, _ = read("/v1/messages", `{"botId":"bot-meiiie","clientNonce":"bot-state-after-restore","text":"Fresh after explicit restore","lane":"foreground"}`); status != 201 {
		t.Fatal("restored bot unavailable", status)
	}
	if status, _ = read("/v1/bots/bot-meiiie/state", `{"action":"archive","expectedRevision":0,"clientNonce":"bot-state-archive-stale"}`); status != 409 {
		t.Fatal("stale lifecycle changed restored bot", status)
	}
}

func TestBotDefaultRequestedModelPinsNewRequestAndKeepsExactRetry(t *testing.T) {
	server, client := setup(t)
	save := post(t, client, server.URL, "/v1/bots/bot-meiiie/state", `{"action":"defaults","expectedRevision":0,"defaultAccountId":"","defaultAccountRevision":0,"defaultModel":"fixture-requested-model","clientNonce":"bot-state-defaults-first"}`, "")
	defer save.Body.Close()
	if save.StatusCode != 200 {
		t.Fatalf("defaults endpoint missing: %d", save.StatusCode)
	}
	message := `{"botId":"bot-meiiie","clientNonce":"bot-state-default-intake","text":"Use pinned desired defaults","lane":"foreground","useBotDefaults":true,"expectedBotStateRevision":1}`
	response := post(t, client, server.URL, "/v1/messages", message, "")
	defer response.Body.Close()
	var original map[string]any
	if response.StatusCode != 201 || json.NewDecoder(response.Body).Decode(&original) != nil {
		t.Fatal("default intake", response.StatusCode)
	}
	work := original["work"].(map[string]any)
	pin, ok := work["botStatePin"].(map[string]any)
	if !ok || pin["defaultModel"] != "fixture-requested-model" || pin["revision"] != float64(1) || work["useBotDefaults"] != true {
		t.Fatal("desired model not pinned", work)
	}
	change := post(t, client, server.URL, "/v1/bots/bot-meiiie/state", `{"action":"defaults","expectedRevision":1,"defaultAccountId":"","defaultAccountRevision":0,"defaultModel":"next-desired-model","clientNonce":"bot-state-defaults-next"}`, "")
	change.Body.Close()
	if change.StatusCode != 200 {
		t.Fatal(change.StatusCode)
	}
	retry := post(t, client, server.URL, "/v1/messages", message, "")
	defer retry.Body.Close()
	var old map[string]any
	if retry.StatusCode != 200 || json.NewDecoder(retry.Body).Decode(&old) != nil || !reflect.DeepEqual(old["work"], work) {
		t.Fatal("defaults rebounded old nonce", retry.StatusCode)
	}
	stale := post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","clientNonce":"bot-state-stale-intake","text":"Do not silently choose new defaults","lane":"foreground","useBotDefaults":true,"expectedBotStateRevision":1}`, "")
	stale.Body.Close()
	if stale.StatusCode != 409 {
		t.Fatal("stale defaults silently rebound", stale.StatusCode)
	}
}
