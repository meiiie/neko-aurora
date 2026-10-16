package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestConversationHTTPArchiveExportAndPreviewAreScopedNonDestructive(t *testing.T) {
	server, client := setup(t)
	read := func(r *http.Response) map[string]any {
		t.Helper()
		defer r.Body.Close()
		var d map[string]any
		if json.NewDecoder(r.Body).Decode(&d) != nil {
			t.Fatal("invalid JSON")
		}
		return d
	}
	r := post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","clientNonce":"http-old-session-request","text":"Old visible history","lane":"background"}`, "")
	if r.StatusCode != 201 {
		t.Fatal(r.StatusCode)
	}
	old := read(r)["work"].(map[string]any)
	body := `{"botId":"bot-meiiie","title":"New private session","expectedRevision":0,"clientNonce":"http-new-conversation"}`
	r = post(t, client, server.URL, "/v1/conversations", body, "")
	if r.StatusCode != 201 {
		r.Body.Close()
		t.Fatal("create route", r.StatusCode)
	}
	c := read(r)["conversation"].(map[string]any)
	id := c["id"].(string)
	r = post(t, client, server.URL, "/v1/conversations/conversation-bot-meiiie", `{"archived":true,"expectedRevision":0,"clientNonce":"http-archive-original"}`, "")
	if r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	read(r)
	r = post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","clientNonce":"http-old-session-request","text":"Old visible history","lane":"background"}`, "")
	d := read(r)
	if r.StatusCode != 200 || d["duplicate"] != true || d["work"].(map[string]any)["requestId"] != old["requestId"] {
		t.Fatal("archive broke original receipt")
	}
	r = post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","clientNonce":"http-new-archived-reject","text":"New blocked","lane":"foreground"}`, "")
	r.Body.Close()
	if r.StatusCode != 409 {
		t.Fatal("archived accepted new work", r.StatusCode)
	}
	r = post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","channelId":"`+id+`","expectedChannelRevision":1,"clientNonce":"http-new-session-request","text":"New session private canary","lane":"foreground"}`, "")
	d = read(r)
	if r.StatusCode != 201 || d["work"].(map[string]any)["conversationId"] != id {
		t.Fatal("new scope")
	}
	r, err := client.Get(server.URL + "/v1/conversations/conversation-bot-meiiie/export?format=json")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || !strings.Contains(r.Header.Get("Content-Disposition"), "attachment") || !strings.Contains(string(raw), "Old visible history") || strings.Contains(string(raw), "New session private canary") || strings.Contains(string(raw), "contextPin") || strings.Contains(string(raw), "clientNonce") {
		t.Fatal("export scope or metadata", r.StatusCode, string(raw))
	}
	r = post(t, client, server.URL, "/v1/conversations/"+id+"/retention", `{"days":30,"expectedRevision":0,"clientNonce":"http-retention-preview"}`, "")
	if r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	d = read(r)
	if d["preference"].(map[string]any)["days"] != float64(30) {
		t.Fatal(d)
	}
	r, err = client.Get(server.URL + "/v1/conversations/" + id + "/retention-preview?days=30")
	if err != nil {
		t.Fatal(err)
	}
	d = read(r)
	if r.StatusCode != 200 || d["enabled"] != false || d["conversationId"] != id || len(d["candidates"].([]any)) != 0 || len(d["protected"].([]any)) != 1 {
		t.Fatal("retention enabled or omitted active work", d)
	}
	r, err = client.Get(server.URL + "/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	d = read(r)
	if len(d["work"].([]any)) != 2 {
		t.Fatal("retention removed history")
	}
}
func TestConversationHTTPRejectsAuthorityDeletionAndMalformedQueries(t *testing.T) {
	server, client := setup(t)
	for _, body := range []string{`{"botId":"bot-meiiie","title":"Unsafe","expectedRevision":0,"clientNonce":"conv-extra-grant","grant":true}`, `{"botId":"bot-meiiie","title":"Missing CAS","clientNonce":"conv-missing-cas"}`} {
		r := post(t, client, server.URL, "/v1/conversations", body, "")
		r.Body.Close()
		if r.StatusCode != 400 {
			t.Fatal(r.StatusCode)
		}
	}
	for _, body := range []string{`{"days":30,"expectedRevision":0,"clientNonce":"retention-enable-policy","enabled":true}`, `{"days":30,"expectedRevision":0,"clientNonce":"retention-delete-files","deleteFiles":true}`} {
		r := post(t, client, server.URL, "/v1/conversations/conversation-bot-meiiie/retention", body, "")
		r.Body.Close()
		if r.StatusCode != 400 {
			t.Fatal("authority accepted", r.StatusCode)
		}
	}
	for _, tail := range []string{"export?format=json&format=text", "export?format=json&privateContext=true", "retention-preview?days=-1", "retention-preview?days=30&days=60"} {
		r, err := client.Get(server.URL + "/v1/conversations/conversation-bot-meiiie/" + tail)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 400 {
			t.Fatal(tail, r.StatusCode)
		}
	}
	r, err := http.Get(server.URL + "/v1/conversations/conversation-bot-meiiie/export?format=json")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatal("unauthorized export", r.StatusCode)
	}
	req, _ := http.NewRequest("DELETE", server.URL+"/v1/conversations/conversation-bot-meiiie", nil)
	req.Header.Set("Content-Type", "application/json")
	r, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 405 {
		t.Fatal("unexpected delete route", r.StatusCode)
	}
}
