package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBotProfileAppearanceEndpointPersistsAndPins(t *testing.T) {
	server, client := setup(t)
	body := `{"instructions":"Answer concisely in Vietnamese.","includeHistory":false,"avatar":"🐱","color":"#7C5CFF","expectedRevision":0,"clientNonce":"profile-appearance-first"}`
	r := post(t, client, server.URL, "/v1/bots/bot-meiiie/profile", body, "")
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("appearance profile rejected: %d", r.StatusCode)
	}
	var saved struct {
		Profile map[string]any `json:"profile"`
	}
	if json.NewDecoder(r.Body).Decode(&saved) != nil || saved.Profile["avatar"] != "🐱" || saved.Profile["color"] != "#7C5CFF" || saved.Profile["revision"] != float64(1) {
		t.Fatal(saved)
	}

	// New intake pins the appearance alongside instructions
	sent := post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","clientNonce":"profile-appearance-request","text":"New request","lane":"background"}`, "")
	defer sent.Body.Close()
	var work struct {
		Work map[string]any `json:"work"`
	}
	if sent.StatusCode != 201 || json.NewDecoder(sent.Body).Decode(&work) != nil {
		t.Fatal("appearance intake", sent.StatusCode)
	}
	pin, ok := work.Work["botProfilePin"].(map[string]any)
	if !ok || pin["avatar"] != "🐱" || pin["color"] != "#7C5CFF" {
		t.Fatal("frozen profile lost appearance", pin)
	}

	// Retry with the same nonce and same resolved appearance → duplicate, no rewrite
	retry := post(t, client, server.URL, "/v1/bots/bot-meiiie/profile", body, "")
	defer retry.Body.Close()
	var dup struct {
		Profile   map[string]any `json:"profile"`
		Duplicate bool           `json:"duplicate"`
	}
	if retry.StatusCode != http.StatusOK || json.NewDecoder(retry.Body).Decode(&dup) != nil || !dup.Duplicate || dup.Profile["revision"] != float64(1) {
		t.Fatal("appearance duplicate handling", retry.StatusCode, dup)
	}

	// Instructions-only edit keeps appearance
	keep := post(t, client, server.URL, "/v1/bots/bot-meiiie/profile", `{"instructions":"Answer fully.","includeHistory":false,"expectedRevision":1,"clientNonce":"profile-appearance-instructions-only"}`, "")
	defer keep.Body.Close()
	var kept struct {
		Profile map[string]any `json:"profile"`
	}
	if keep.StatusCode != http.StatusOK || json.NewDecoder(keep.Body).Decode(&kept) != nil || kept.Profile["avatar"] != "🐱" || kept.Profile["color"] != "#7C5CFF" || kept.Profile["revision"] != float64(2) {
		t.Fatal("instructions-only edit cleared appearance", keep.StatusCode, kept)
	}

	for name, invalid := range map[string]string{
		"profile-bad-color":       `{"instructions":"ok","includeHistory":false,"color":"7C5CFF","expectedRevision":2,"clientNonce":"profile-bad-color"}`,
		"profile-bad-avatar":      `{"instructions":"ok","includeHistory":false,"avatar":"0123456789abcdef0","expectedRevision":2,"clientNonce":"profile-bad-avatar"}`,
		"profile-unknown-field":   `{"instructions":"ok","includeHistory":false,"avatarz":"🐱","expectedRevision":2,"clientNonce":"profile-unknown-field"}`,
		"profile-color-num":       `{"instructions":"ok","includeHistory":false,"color":7,"expectedRevision":2,"clientNonce":"profile-color-num"}`,
		"profile-stale":           `{"instructions":"ok","includeHistory":false,"avatar":"🐯","expectedRevision":0,"clientNonce":"profile-stale-appearance"}`,
		"profile-appearance-noop": `{"instructions":"Answer fully.","includeHistory":false,"expectedRevision":2,"clientNonce":"profile-appearance-noop"}`,
	} {
		response := post(t, client, server.URL, "/v1/bots/bot-meiiie/profile", invalid, "")
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest && response.StatusCode != http.StatusConflict {
			t.Fatal(name, "appearance validation gap", response.StatusCode)
		}
	}
}
