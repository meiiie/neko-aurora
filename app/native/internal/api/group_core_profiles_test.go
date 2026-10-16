package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"nekobot.local/native/internal/ledger"
)

func TestGroupCoreProfileHTTPClosedSchema(t *testing.T) {
	s, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server, err := New(s, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := s.SaveChannel(ledger.ChannelInput{Name: "Group HTTP boundary", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "group-profile-api-channel"})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"channelId": c.ID, "expectedChannelRevision": c.Revision, "participants": []string{"bot-meiiie", "bot-davin"}, "coordinatorBotId": "bot-meiiie", "goal": "A bounded group contract", "maxStages": 3, "clientNonce": "group-profile-api-create", "coreProfileSelection": map[string]any{"consentId": "core-consent-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "expectedConsentRevision": 1}}
	request := func(body any) int {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8123/v1/collaborations", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w.Code
	}
	before := s.Snapshot().Sequence
	code := request(body)
	if _, exists := reflect.TypeOf(ledger.CollaborationInput{}).FieldByName("CoreProfileSelection"); !exists {
		if code != http.StatusBadRequest {
			t.Fatal("baseline accepted unknown future selection", code)
		}
		t.Log("baseline API rejects missing additive selection schema with 400; injected store REDs are not a historical API permission defect")
	} else if code != http.StatusForbidden && code != http.StatusConflict {
		t.Fatal("known selection did not fail closed without authority", code)
	}
	body["callerProof"] = "untrusted-fixture"
	if code = request(body); code != http.StatusBadRequest || s.Snapshot().Sequence != before {
		t.Fatal("unknown proof field or denied intake mutated journal", code)
	}
}
