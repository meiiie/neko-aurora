package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nekobot.local/native/internal/ledger"
)

func TestUploadReceiptReadOnlyOriginalIdentityAndCurrentPublicState(t *testing.T) {
	dir := t.TempDir()
	store, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	server, err := New(store, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	channel, _, err := store.SaveChannel(ledger.ChannelInput{Name: "Original upload channel", BotIDs: []string{"bot-meiiie"}, ClientNonce: "receipt-channel-first"})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := store.SaveChannel(ledger.ChannelInput{Name: "Other channel", BotIDs: []string{"bot-davin"}, ClientNonce: "receipt-channel-other"})
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := store.SaveProject(ledger.ProjectInput{Name: "Original project", ChannelIDs: []string{channel.ID}, ClientNonce: "receipt-project-first"})
	if err != nil {
		t.Fatal(err)
	}
	input := ledger.UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel.ID, Filename: "source.md", Content: []byte("Private original excerpt must not enter receipt"), ClientNonce: "receipt-upload-original"}
	file, _, err := store.UploadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	query := "/v1/upload-receipts?projectId=" + project.ID + "&channelId=" + channel.ID + "&clientNonce=" + input.ClientNonce
	get := func(path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8123"+path, nil)
		if auth {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	if w := get(query, false); w.Code != 401 {
		t.Fatal("unauthenticated receipt", w.Code)
	}
	before := store.Snapshot().Sequence
	w := get(query, true)
	if w.Code != 200 {
		t.Fatalf("accepted upload requires read-only receipt, got %d", w.Code)
	}
	var receipt struct {
		Version     int    `json:"version"`
		Found       bool   `json:"found"`
		ClientNonce string `json:"clientNonce"`
		Input       struct {
			ProjectID       string `json:"projectId"`
			ProjectRevision uint64 `json:"projectRevision"`
			ChannelID       string `json:"channelId"`
			Filename        string `json:"filename"`
			SHA256          string `json:"sha256"`
			Bytes           int    `json:"bytes"`
			ClientNonce     string `json:"clientNonce"`
		} `json:"input"`
		File ledger.UploadedFile `json:"file"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Found || receipt.Version != 1 || receipt.ClientNonce != input.ClientNonce || receipt.Input.ClientNonce != input.ClientNonce || receipt.File.ID != file.ID || receipt.Input.ProjectID != input.ProjectID || receipt.Input.ProjectRevision != input.ProjectRevision || receipt.Input.ChannelID != input.ChannelID || receipt.Input.Filename != input.Filename || receipt.Input.SHA256 != file.SHA256 || receipt.Input.Bytes != len(input.Content) {
		t.Fatal("original identity changed")
	}
	if receipt.File.Excerpt != "" || strings.Contains(w.Body.String(), string(input.Content)) || strings.Contains(w.Body.String(), "contentBase64") {
		t.Fatal("receipt leaked upload content")
	}
	if store.Snapshot().Sequence != before {
		t.Fatal("read receipt mutated journal")
	}
	for _, path := range []string{query + "&unknown=x", query + "&clientNonce=" + input.ClientNonce, strings.Replace(query, "clientNonce="+input.ClientNonce, "clientNonce=bad", 1), strings.Replace(query, "channelId="+channel.ID, "channelId=bad", 1)} {
		if got := get(path, true).Code; got != 400 {
			t.Fatal("noncanonical query", got)
		}
	}
	if got := get(strings.Replace(query, channel.ID, other.ID, 1), true).Code; got != 409 {
		t.Fatal("receipt crossed original channel", got)
	}
	if got := get(strings.Replace(query, input.ClientNonce, "receipt-project-first", 1), true).Code; got != 409 {
		t.Fatal("receipt accepted another operation nonce", got)
	}
	missing := get(strings.Replace(query, input.ClientNonce, "receipt-upload-not-found", 1), true)
	if missing.Code != 200 || !strings.Contains(missing.Body.String(), `"found":false`) || strings.Contains(missing.Body.String(), `"input"`) || strings.Contains(missing.Body.String(), `"file"`) {
		t.Fatal("missing receipt is not metadata-only unknown", missing.Code)
	}
	if err = store.SetFileAccess(ledger.FileAccessInput{FileID: file.ID, ExpectedRevision: file.AccessRevision, Granted: false, ClientNonce: "receipt-upload-revoke"}); err != nil {
		t.Fatal(err)
	}
	updated, _, err := store.SaveProject(ledger.ProjectInput{ID: project.ID, Name: project.Name, ExpectedRevision: project.Revision, ChannelIDs: []string{other.ID}, ClientNonce: "receipt-project-change"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision == input.ProjectRevision {
		t.Fatal("project revision fixture")
	}
	before = store.Snapshot().Sequence
	w = get(query, true)
	if w.Code != 200 {
		t.Fatal("historical original receipt cannot be reconciled", w.Code)
	}
	if err = json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.File.Granted || receipt.File.AccessRevision != file.AccessRevision+1 || receipt.Input.ProjectRevision != input.ProjectRevision {
		t.Fatal("receipt revived or repinned authority")
	}
	if store.Snapshot().Sequence != before {
		t.Fatal("receipt after revoke changed sequence")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if got := get(query, true).Code; got != 503 {
		t.Fatal("receipt accepted closed store", got)
	}
	store, err = ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	server, err = New(store, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	w = get(query, true)
	if w.Code != 200 {
		t.Fatal("receipt missing after restart", w.Code)
	}
	if err = json.Unmarshal(w.Body.Bytes(), &receipt); err != nil || receipt.File.Granted || receipt.Input.ProjectRevision != input.ProjectRevision {
		t.Fatal("restart receipt identity/authority", err)
	}
}
