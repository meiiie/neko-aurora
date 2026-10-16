package api

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestMetadataReceiptKeepsOriginalNameInputAfterNextRevision(t *testing.T) {
	server, client := setup(t)
	for _, body := range []string{`{"name":"First visible name","expectedDisplayRevision":0,"clientNonce":"metadata-name-original"}`, `{"name":"Second visible name","expectedDisplayRevision":1,"clientNonce":"metadata-name-next"}`} {
		r := post(t, client, server.URL, "/v1/bots/bot-meiiie/name", body, "")
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatal(r.StatusCode)
		}
	}
	r, err := client.Get(server.URL + "/v1/command-receipts?kind=bot_name&clientNonce=metadata-name-original")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var out map[string]any
	if r.StatusCode != 200 {
		t.Fatalf("metadata receipt missing: %d", r.StatusCode)
	}
	if json.NewDecoder(r.Body).Decode(&out) != nil || out["found"] != true {
		t.Fatal("original receipt")
	}
	in := out["input"].(map[string]any)
	current := out["bot"].(map[string]any)
	if in["botId"] != "bot-meiiie" || in["name"] != "First visible name" || in["expectedDisplayRevision"] != float64(0) || current["name"] != "Second visible name" {
		t.Fatal("receipt rebound original intent")
	}
	wrong, _ := client.Get(server.URL + "/v1/command-receipts?kind=project_save&clientNonce=metadata-name-original")
	raw, _ := io.ReadAll(wrong.Body)
	wrong.Body.Close()
	if wrong.StatusCode != 409 || strings.Contains(string(raw), "First visible name") {
		t.Fatal("wrong kind exposed metadata", wrong.StatusCode)
	}
}
