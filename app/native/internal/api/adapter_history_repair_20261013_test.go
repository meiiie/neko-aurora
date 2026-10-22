package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestAdapterHistoryRepairSignerIsIndependentAndRetiredOnRestart(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	channel, _, err := store.SaveChannel(ledger.ChannelInput{Name: "signer fixture", BotIDs: []string{"bot-meiiie"}, ClientNonce: "repair-signer-channel"})
	if err != nil {
		t.Fatal(err)
	}
	register := func(source string) ledger.AdapterSession {
		a, _, e := store.RegisterAdapter(ledger.AdapterSessionInput{SourceApp: "wiii", SourceSessionID: source, SourceThreadID: source, ChannelID: channel.ID, ExpectedChannelRevision: channel.Revision, ClientNonce: source})
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	adapter := register("repair-signer-source")
	other := register("repair-other-source")
	for i := 0; i < 129; i++ {
		id := "repair-signer-message-" + strconv.Itoa(i)
		_, _, e := store.AcceptAdapter(adapter.ID, ledger.AdapterMessageInput{Input: ledger.Input{BotID: "bot-meiiie", Text: "synthetic signer request", Lane: "foreground", ClientNonce: id}, SourceMessageID: id})
		if e != nil {
			t.Fatal(e)
		}
	}
	server, err := New(store, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	if server.session == hex.EncodeToString(server.historyKey[:]) || server.historyKey == [32]byte{} {
		t.Fatal("history signer reused authentication credential")
	}
	request := func(s *Server, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8123"+path, nil)
		r.AddCookie(&http.Cookie{Name: "neko_local", Value: s.session})
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	path := "/v1/adapters/sessions/" + adapter.ID + "/history"
	before := store.Snapshot().Sequence
	w := request(server, path)
	if w.Code != 200 {
		t.Fatal("first page unavailable", w.Code, w.Body.String())
	}
	var first struct {
		NextCursor string            `json:"nextCursor"`
		TotalRows  int               `json:"totalRows"`
		Offset     int               `json:"offset"`
		Rows       []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.NextCursor) != 215 || first.TotalRows != 129 || first.Offset != 0 || len(first.Rows) != 128 {
		t.Fatal("first page did not honor bounded snapshot contract")
	}
	w = request(server, "/v1/adapters/sessions/"+other.ID+"/history?cursor="+first.NextCursor)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "adapter_history_binding_mismatch") {
		t.Fatal("cross-session cursor allowed", w.Code)
	}
	w = request(server, path+"?cursor="+first.NextCursor)
	if w.Code != 200 {
		t.Fatal("continuation unavailable", w.Code, w.Body.String())
	}
	var last struct {
		ConfirmationCursor string            `json:"confirmationCursor"`
		Complete           bool              `json:"complete"`
		Offset             int               `json:"offset"`
		Rows               []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &last); err != nil {
		t.Fatal(err)
	}
	if !last.Complete || last.Offset != 128 || len(last.Rows) != 1 || len(last.ConfirmationCursor) != 215 {
		t.Fatal("complete page missing authority confirmation")
	}
	w = request(server, path+"?cursor="+last.ConfirmationCursor)
	if w.Code != 200 {
		t.Fatal("terminal confirmation unavailable", w.Code)
	}
	var confirmation struct {
		Offset int               `json:"offset"`
		Rows   []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &confirmation); err != nil {
		t.Fatal(err)
	}
	if confirmation.Offset != 129 || len(confirmation.Rows) != 0 {
		t.Fatal("terminal confirmation replayed rows")
	}
	restarted, err := New(store, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	if restarted.historyKey == server.historyKey {
		t.Fatal("server restart retained ephemeral signer")
	}
	w = request(restarted, path+"?cursor="+first.NextCursor)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "adapter_history_cursor_invalid") {
		t.Fatal("retired signer survived restart", w.Code)
	}
	if store.Snapshot().Sequence != before {
		t.Fatal("repair cursor verification mutated journal")
	}
}

func TestAdapterHistoryRepairHTTPContractAndReadonlyBoundary(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	channel, _, err := store.SaveChannel(ledger.ChannelInput{Name: "repair fixture", BotIDs: []string{"bot-meiiie"}, ClientNonce: "repair-api-channel"})
	if err != nil {
		t.Fatal(err)
	}
	adapter, _, err := store.RegisterAdapter(ledger.AdapterSessionInput{SourceApp: "wiii", SourceSessionID: "repair-api-source", SourceThreadID: "repair-api-thread", ChannelID: channel.ID, ExpectedChannelRevision: channel.Revision, ClientNonce: "repair-api-register"})
	if err != nil {
		t.Fatal(err)
	}
	work, _, err := store.AcceptAdapter(adapter.ID, ledger.AdapterMessageInput{Input: ledger.Input{BotID: "bot-meiiie", Text: "synthetic repair request", Lane: "foreground", ClientNonce: "repair-api-intake"}, SourceMessageID: "repair-api-message"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(store, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, auth bool, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8123"+path, nil)
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	path := "/v1/adapters/sessions/" + adapter.ID + "/history"
	before := store.Snapshot().Sequence
	if w := request("GET", path, false, ""); w.Code != http.StatusUnauthorized {
		t.Fatal("missing authentication", w.Code)
	}
	if w := request("GET", path, true, "http://foreign.invalid"); w.Code != http.StatusForbidden {
		t.Fatal("foreign origin", w.Code)
	}
	if w := request("POST", path, true, ""); w.Code != http.StatusNotFound {
		t.Fatal("repair must not accept mutation", w.Code)
	}
	w := request("GET", path, true, "")
	if w.Code != http.StatusOK {
		t.Fatalf("bounded authoritative repair is unavailable: %d %s", w.Code, w.Body.String())
	}
	var page struct {
		Version          int    `json:"version"`
		AdapterSessionID string `json:"adapterSessionId"`
		SourceSessionID  string `json:"sourceSessionId"`
		SourceThreadID   string `json:"sourceThreadId"`
		ChannelID        string `json:"channelId"`
		OwnerUserID      string `json:"ownerUserId"`
		SpaceID          string `json:"spaceId"`
		SnapshotCursor   uint64 `json:"snapshotCursor"`
		Complete         bool   `json:"complete"`
		Rows             []struct {
			RequestID       string `json:"requestId"`
			SourceMessageID string `json:"sourceMessageId"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Version != 1 || page.AdapterSessionID != adapter.ID || page.SourceSessionID != adapter.SourceSessionID || page.SourceThreadID != adapter.SourceThreadID || page.ChannelID != adapter.ChannelID || page.OwnerUserID != adapter.OwnerUserID || page.SpaceID != adapter.SpaceID || page.SnapshotCursor != before || !page.Complete || len(page.Rows) != 1 || page.Rows[0].RequestID != work.RequestID || page.Rows[0].SourceMessageID != "repair-api-message" {
		t.Fatal("repair changed original source identity or lost accepted history")
	}
	for _, suffix := range []string{"?", "?cursor=", "?cursor=a&cursor=b", "?after=0", "?cursor=abc&after=0", "?cursor=%zz", "?cursor=a+b", "?cursor=" + strings.Repeat("a", 257)} {
		w := request("GET", path+suffix, true, "")
		if w.Code != http.StatusBadRequest {
			t.Fatal("noncanonical repair query accepted", suffix, w.Code)
		}
	}
	for _, input := range []struct{ path, body string }{
		{path: strings.Replace(path, "/adapters/", "/%61dapters/", 1)},
		{path: path, body: "{}"},
	} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8123"+input.path, strings.NewReader(input.body))
		r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "adapter_history_cursor_invalid") {
			t.Fatal("noncanonical path or GET body allowed", w.Code)
		}
	}
	w = request("GET", "/v1/adapters/capabilities", true, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "local_adapter_history_repair_v1") {
		t.Fatal("repair capability absent")
	}
	if store.Snapshot().Sequence != before {
		t.Fatal("history GET or denied inputs mutated the durable journal")
	}
}
