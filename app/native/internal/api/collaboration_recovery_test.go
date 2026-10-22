package api

import (
	"bytes"
	"encoding/json"
	"nekoaurora.local/native/internal/ledger"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGroupServiceRecoveryAPIRequiresExplicitScopedIntent(t *testing.T) {
	s, e := ledger.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	server, e := New(s, "127.0.0.1:8123")
	if e != nil {
		t.Fatal(e)
	}
	c, _, e := s.SaveChannel(ledger.ChannelInput{Name: "Recovery", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "group-recovery-api-channel"})
	if e != nil {
		t.Fatal(e)
	}
	g, _, e := s.CreateCollaboration(ledger.CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Original finite goal", MaxStages: 3, ClientNonce: "group-recovery-api-create"})
	if e != nil {
		t.Fatal(e)
	}
	post := func(body any, auth bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "http://"+server.host+"/v1/collaborations/"+g.ID+"/recover", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		}
		out := httptest.NewRecorder()
		server.ServeHTTP(out, r)
		return out
	}
	body := map[string]any{"version": 2, "requestId": "req-00000000000000000000000000000000", "expectedRevision": g.Revision, "expectedEpoch": uint64(1), "expectedTaskRevision": uint64(0), "clientNonce": "group-recovery-api-owner"}
	before := s.Snapshot().Sequence
	if out := post(body, false); out.Code != 401 {
		t.Fatal("unauthenticated", out.Code)
	}
	if out := post(body, true); out.Code != 409 {
		t.Fatal("unproved group recovery boundary", out.Code, out.Body.String())
	}
	body["runtimeStopped"] = true
	if out := post(body, true); out.Code != 400 {
		t.Fatal("renderer proof accepted", out.Code)
	}
	delete(body, "runtimeStopped")
	delete(body, "expectedTaskRevision")
	if out := post(body, true); out.Code != 400 {
		t.Fatal("missing CAS accepted", out.Code)
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("rejected request mutated state")
	}
}
