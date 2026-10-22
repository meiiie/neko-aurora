package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestContextAccessRequiresExplicitDecisionAndKeepsSuspendedObligation(t *testing.T) {
	server, client := setup(t)
	r := post(t, client, server.URL, "/v1/messages", `{"botId":"bot-meiiie","clientNonce":"api-permission-fixture","text":"fixture request","lane":"background"}`, "")
	var accepted struct {
		Work ledger.Work `json:"work"`
	}
	if json.NewDecoder(r.Body).Decode(&accepted) != nil || r.StatusCode != 201 {
		t.Fatal("fixture intake failed")
	}
	r.Body.Close()
	id := accepted.Work.RequestID
	missing := fmt.Sprintf(`{"requestId":%q,"expectedRevision":0,"clientNonce":"api-permission-revoke"}`, id)
	r = post(t, client, server.URL, "/v1/context-access", missing, "")
	r.Body.Close()
	if r.StatusCode != 400 {
		t.Fatal("missing permission decision defaulted to revoke")
	}
	body := fmt.Sprintf(`{"requestId":%q,"expectedRevision":0,"granted":false,"clientNonce":"api-permission-revoke"}`, id)
	r = post(t, http.DefaultClient, server.URL, "/v1/context-access", body, "")
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatal("permission changed without local session")
	}
	r = post(t, client, server.URL, "/v1/context-access", body, "https://untrusted.example")
	r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal("cross origin changed permission")
	}
	r = post(t, client, server.URL, "/v1/context-access", body, "")
	var revoked struct {
		Work ledger.Work `json:"work"`
	}
	if json.NewDecoder(r.Body).Decode(&revoked) != nil || r.StatusCode != 200 || revoked.Work.Status != "suspended" || revoked.Work.ContextAccess.Granted {
		t.Fatal("revocation lost queued obligation")
	}
	r.Body.Close()
	r = post(t, client, server.URL, "/v1/context-refresh", fmt.Sprintf(`{"requestId":%q,"expectedEpoch":%d}`, id, revoked.Work.Epoch), "")
	r.Body.Close()
	if r.StatusCode != 409 {
		t.Fatal("refresh while own input denied")
	}
	r = post(t, client, server.URL, "/v1/context-access", fmt.Sprintf(`{"requestId":%q,"expectedRevision":1,"granted":true,"clientNonce":"api-permission-regrant"}`, id), "")
	r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal("explicit regrant failed")
	}
	r = post(t, client, server.URL, "/v1/context-refresh", fmt.Sprintf(`{"requestId":%q,"expectedEpoch":%d}`, id, revoked.Work.Epoch), "")
	var fresh ledger.Work
	if json.NewDecoder(r.Body).Decode(&fresh) != nil || r.StatusCode != 200 || fresh.RequestID != id || fresh.Status != "waiting_host" || fresh.ContextRevision != 1 {
		t.Fatal("fresh context changed obligation identity")
	}
	r.Body.Close()
}
