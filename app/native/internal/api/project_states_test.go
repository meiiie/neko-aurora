package api

import (
	"encoding/json"
	"testing"
)

func TestProjectArchiveBlocksFreshKeepsOldACKAndFileRead(t *testing.T) {
	server, client := setup(t)
	call := func(path, body string) (int, map[string]any) {
		r := post(t, client, server.URL, path, body, "")
		defer r.Body.Close()
		var v map[string]any
		json.NewDecoder(r.Body).Decode(&v)
		return r.StatusCode, v
	}
	status, c := call("/v1/channels", `{"name":"Lifecycle shared","botIds":["bot-meiiie","bot-davin"],"expectedRevision":0,"clientNonce":"project-state-channel"}`)
	if status != 201 && status != 200 {
		t.Fatal("channel", status)
	}
	channel := c["channel"].(map[string]any)["id"].(string)
	status, p := call("/v1/projects", `{"name":"Project archive","goal":"Keep files and history","channelIds":["`+channel+`"],"expectedRevision":0,"clientNonce":"project-state-create"}`)
	if status != 201 && status != 200 {
		t.Fatal("project", status)
	}
	id := p["project"].(map[string]any)["id"].(string)
	message := `{"botId":"bot-meiiie","text":"Use project","lane":"background","clientNonce":"project-state-old-work","channelId":"` + channel + `","projectId":"` + id + `","projectRevision":1}`
	status, w := call("/v1/messages", message)
	if status != 201 {
		t.Fatal(status)
	}
	req := w["work"].(map[string]any)["requestId"]
	status, _ = call("/v1/projects/"+id+"/state", `{"action":"archive","expectedRevision":0,"clientNonce":"project-state-archive"}`)
	if status != 200 {
		t.Fatalf("project archive missing: %d", status)
	}
	status, dupe := call("/v1/messages", message)
	if status != 200 || dupe["work"].(map[string]any)["requestId"] != req {
		t.Fatal("archive changed accepted request", status)
	}
	status, _ = call("/v1/messages", `{"botId":"bot-meiiie","text":"Fresh project","lane":"foreground","clientNonce":"project-state-new-work","channelId":"`+channel+`","projectId":"`+id+`","projectRevision":1}`)
	if status != 409 {
		t.Fatal("fresh intake archived", status)
	}
	status, _ = call("/v1/uploads", `{"projectId":"`+id+`","projectRevision":1,"channelId":"`+channel+`","clientNonce":"project-state-new-upload","filename":"notes.txt","contentBase64":"bm90ZXM="}`)
	if status != 409 {
		t.Fatal("fresh upload archived", status)
	}
	status, _ = call("/v1/projects/"+id+"/state", `{"action":"restore","expectedRevision":1,"clientNonce":"project-state-restore"}`)
	if status != 200 {
		t.Fatal("restore", status)
	}
	status, _ = call("/v1/uploads", `{"projectId":"`+id+`","projectRevision":1,"channelId":"`+channel+`","clientNonce":"project-state-restored-upload","filename":"notes.txt","contentBase64":"bm90ZXM="}`)
	if status != 201 && status != 200 {
		t.Fatal("restored upload", status)
	}
}
