package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nekobot.local/native/internal/ledger"
)

func TestProjectContinuityAPIAuthScopeCASProvenanceAndPublicProjection(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := New(store, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body any, auth bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://127.0.0.1:8123"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	channel, _, err := store.SaveChannel(ledger.ChannelInput{Name: "Continuity", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "api-continuity-channel"})
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := store.SaveProject(ledger.ProjectInput{Name: "Continuity project", ChannelIDs: []string{channel.ID}, ClientNonce: "api-continuity-project"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/v1/projects/" + project.ID + "/continuity"
	body := map[string]any{"channelId": channel.ID, "expectedProjectRevision": 1, "expectedChannelRevision": 1, "expectedRevision": 0, "clientNonce": "api-continuity-first", "instructions": "Owner continuity private sentence", "decisions": []any{map[string]any{"id": "owner-choice", "text": "Keep separate identities", "status": "confirmed", "effectiveDate": "2026-10-11", "referenceIds": []string{"dated-reference"}}}, "references": []any{map[string]any{"id": "dated-reference", "title": "Same name explicit identity", "entityId": "person-a", "sourceDate": "2026-10-01", "version": "original", "url": "https://example.invalid/source"}}}
	before := store.Snapshot().Sequence
	if w := request("POST", path, body, false); w.Code != 401 {
		t.Fatal("unauthenticated write", w.Code)
	}
	if w := request("GET", path+"?channelId="+channel.ID, nil, false); w.Code != 401 {
		t.Fatal("unauthenticated read", w.Code)
	}
	for _, key := range []string{"expectedRevision", "expectedProjectRevision", "expectedChannelRevision"} {
		value := body[key]
		delete(body, key)
		w := request("POST", path, body, true)
		body[key] = value
		if w.Code != 400 {
			t.Fatal("missing explicit CAS", key, w.Code)
		}
	}
	body["authorType"] = "ai"
	w := request("POST", path, body, true)
	delete(body, "authorType")
	if w.Code != 400 {
		t.Fatal("AI authority accepted", w.Code)
	}
	body["ownerUserId"] = "forged"
	w = request("POST", path, body, true)
	delete(body, "ownerUserId")
	if w.Code != 400 {
		t.Fatal("forged actor accepted", w.Code)
	}
	body["projectId"] = project.ID
	w = request("POST", path, body, true)
	delete(body, "projectId")
	if w.Code != 400 {
		t.Fatal("path scope override accepted", w.Code)
	}
	if before != store.Snapshot().Sequence {
		t.Fatal("invalid write committed")
	}
	w = request("POST", path, body, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var receipt struct {
		Continuity ledger.ProjectContinuity
		Duplicate  bool
	}
	if err = json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Duplicate || receipt.Continuity.AuthorType != "owner" || receipt.Continuity.OwnerUserID != "local-owner" || receipt.Continuity.Revision != 1 {
		t.Fatal("server provenance")
	}
	original := receipt.Continuity.SHA256
	w = request("POST", path, body, true)
	if w.Code != 200 {
		t.Fatal("lost ACK", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &receipt)
	if !receipt.Duplicate || receipt.Continuity.SHA256 != original {
		t.Fatal("lost ACK rebound")
	}
	w = request("GET", path+"?channelId="+channel.ID, nil, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), body["instructions"].(string)) {
		t.Fatal("scoped full record", w.Code)
	}
	for _, query := range []string{"", "?channelId=conversation-bot-davin", "?channelId=unknown"} {
		if w = request("GET", path+query, nil, true); w.Code != 404 {
			t.Fatal("unscoped getter", query, w.Code)
		}
	}
	body["clientNonce"] = "api-continuity-stale"
	w = request("POST", path, body, true)
	if w.Code != 409 {
		t.Fatal("stale record revision", w.Code)
	}
	message := map[string]any{"botId": "bot-meiiie", "channelId": channel.ID, "clientNonce": "api-continuity-message", "text": "Continue", "lane": "foreground", "projectId": project.ID, "projectRevision": 1, "expectedContinuityRevision": 0}
	w = request("POST", "/v1/messages", message, true)
	if w.Code != 409 {
		t.Fatal("stale intake metadata", w.Code)
	}
	message["expectedContinuityRevision"] = 1
	w = request("POST", "/v1/messages", message, true)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), body["instructions"].(string)) || strings.Contains(w.Body.String(), "person-a") {
		t.Fatal("message ACK disclosed metadata")
	}
	var accepted struct{ Work ledger.Work }
	_ = json.Unmarshal(w.Body.Bytes(), &accepted)
	control := map[string]any{"version": 1, "requestId": accepted.Work.RequestID, "expectedEpoch": accepted.Work.Epoch, "expectedRevision": 0, "action": "supplement", "clientNonce": "continuity-control-projection", "text": "A bounded supplement"}
	w = request("POST", "/v1/task-controls", control, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), body["instructions"].(string)) || strings.Contains(w.Body.String(), "person-a") {
		t.Fatal("control ACK disclosed metadata", w.Code, w.Body.String())
	}
	group := map[string]any{"channelId": channel.ID, "expectedChannelRevision": 1, "participants": []string{"bot-meiiie", "bot-davin"}, "coordinatorBotId": "bot-meiiie", "goal": "Compare owner references", "maxStages": 3, "clientNonce": "continuity-api-group", "projectId": project.ID, "projectRevision": 1, "expectedContinuityRevision": 1}
	w = request("POST", "/v1/collaborations", group, true)
	if w.Code != 201 || strings.Contains(w.Body.String(), body["instructions"].(string)) || strings.Contains(w.Body.String(), "person-a") {
		t.Fatal("group ACK disclosed metadata", w.Code, w.Body.String())
	}
	var groupReceipt struct{ Collaboration ledger.Collaboration }
	_ = json.Unmarshal(w.Body.Bytes(), &groupReceipt)
	w = request("POST", "/v1/collaborations/"+groupReceipt.Collaboration.ID+"/cancel", map[string]any{"expectedRevision": 1, "clientNonce": "continuity-api-group-cancel"}, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), body["instructions"].(string)) {
		t.Fatal("cancel group ACK disclosed metadata", w.Code)
	}
	w = request("GET", "/v1/collaborations", nil, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), body["instructions"].(string)) {
		t.Fatal("group list disclosed metadata", w.Code)
	}
	w = request("GET", "/v1/snapshot", nil, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), body["instructions"].(string)) || strings.Contains(w.Body.String(), "person-a") || !strings.Contains(w.Body.String(), "local_continuity_intake_cas_v1") {
		t.Fatal("public snapshot", w.Code)
	}
	events, _ := store.Events(0)
	published, _ := json.Marshal(events)
	if strings.Contains(string(published), body["instructions"].(string)) || strings.Contains(string(published), "Same name explicit identity") {
		t.Fatal("public event metadata leak")
	}
	body["expectedRevision"] = 1
	body["expectedChannelRevision"] = 0
	w = request("POST", path, body, true)
	if w.Code != 409 {
		t.Fatal("stale audience", w.Code)
	}
}

func TestProjectContinuityAPICurrentScopeRemovalAndClear(t *testing.T) {
	s, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server, err := New(s, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	channel, _, err := s.SaveChannel(ledger.ChannelInput{Name: "Team", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "continuity-removal-channel"})
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := s.SaveProject(ledger.ProjectInput{Name: "Scoped", ChannelIDs: []string{channel.ID, "conversation-bot-meiiie"}, ClientNonce: "continuity-removal-project"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, input ledger.ProjectContinuityInput) *httptest.ResponseRecorder {
		b, _ := json.Marshal(input)
		r := httptest.NewRequest(method, "http://127.0.0.1:8123/v1/projects/"+p.ID+"/continuity?channelId="+channel.ID, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	in := ledger.ProjectContinuityInput{ChannelID: channel.ID, ExpectedProjectRevision: 1, ExpectedChannelRevision: 1, ClientNonce: "continuity-clear-api", Instructions: "A prior choice"}
	if w := call("POST", in); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	in.ExpectedRevision = 1
	in.ClientNonce = "continuity-clear-second"
	in.Instructions = ""
	if w := call("POST", in); w.Code != 200 {
		t.Fatal("explicit clear", w.Code, w.Body.String())
	}
	if _, _, err = s.SaveProject(ledger.ProjectInput{ID: p.ID, Name: p.Name, ExpectedRevision: 1, ChannelIDs: []string{"conversation-bot-meiiie"}, ClientNonce: "continuity-remove-audience"}); err != nil {
		t.Fatal(err)
	}
	if w := call("GET", in); w.Code != 404 {
		t.Fatal("removed scope read", w.Code)
	}
	if w := call("POST", in); w.Code != 409 {
		t.Fatal("old nonce disclosed removed scope", w.Code)
	}
}
