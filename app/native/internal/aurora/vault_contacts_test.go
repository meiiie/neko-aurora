package aurora

import (
	"encoding/json"
	"nekoaurora.local/native/internal/ledger"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVaultContactsOwnerAPI(t *testing.T) {
	s, _ := newSvc(t)
	it, err := s.V.SaveItem("", "identity", "Fixture contact", map[string]string{"name": "Fixture Owner", "email": "first@example.test", "phone": "+84 900 111 222"})
	if err != nil {
		t.Fatal(err)
	}
	w := hintRequest(t, s, "POST", "/app/api/vault/"+it.ID+"/contacts", map[string]any{"workspaceId": s.workspaceID, "revision": "0", "action": "add", "kind": "email", "value": "second@example.test"})
	if w.Code != 200 {
		t.Fatalf("add contact HTTP%d", w.Code)
	}
	var got struct {
		WorkspaceID string `json:"workspaceId"`
		Item        struct {
			Contacts struct {
				Revision string
				Emails   []struct{ ID, Hint string }
			}
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || got.WorkspaceID != s.workspaceID || len(got.Item.Contacts.Emails) != 2 || got.Item.Contacts.Revision == "0" {
		t.Fatal("contact metadata missing")
	}
	for _, method := range []string{"list", "mutation"} {
		response := w
		if method == "list" {
			response = hintRequest(t, s, "GET", "/app/api/vault?workspaceId="+s.workspaceID, nil)
		}
		if strings.Contains(response.Body.String(), "first@example.test") || strings.Contains(response.Body.String(), "second@example.test") {
			t.Fatal("public metadata exposed full contact values")
		}
	}
	w = hintRequest(t, s, "POST", "/app/api/vault/"+it.ID+"/contacts", map[string]any{"workspaceId": s.workspaceID, "revision": got.Item.Contacts.Revision, "action": "primary", "kind": "email", "entryId": got.Item.Contacts.Emails[1].ID})
	if w.Code != 200 {
		t.Fatal("choose primary failed")
	}
	w = hintRequest(t, s, "POST", "/app/api/vault/"+it.ID+"/reveal", map[string]string{"workspaceId": s.workspaceID})
	var reveal struct {
		Fields   map[string]string
		Contacts struct{ Emails []struct{ ID, Value string } }
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &reveal) != nil || reveal.Fields["email"] != "second@example.test" || len(reveal.Contacts.Emails) != 2 || reveal.Contacts.Emails[0].Value != "second@example.test" {
		t.Fatal("explicit reveal/primary mismatch")
	}
	if len(s.V.Uses(30)) != 3 {
		t.Fatal("contact mutation/reveal audit missing")
	}
}

func TestVaultContactsRejectInvalidAuthorityAndBodies(t *testing.T) {
	for _, mode := range []string{"wrong-workspace", "missing-workspace", "stale", "unknown-action", "wrong-type", "read-only", "unknown-property", "trailing-json", "missing-revision", "null-revision"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := newSvc(t)
			typ, fields := "identity", map[string]string{"email": "first@example.test"}
			if mode == "wrong-type" {
				typ = "login"
				fields = map[string]string{"password": "fixture-contact-private"}
			}
			it, err := s.V.SaveItem("", typ, "Fixture", fields)
			if err != nil {
				t.Fatal(err)
			}
			body := map[string]any{"workspaceId": s.workspaceID, "revision": "0", "kind": "email", "action": "add", "value": "second@example.test"}
			want := 400
			switch mode {
			case "wrong-workspace":
				body["workspaceId"] = "other"
				want = 409
			case "missing-workspace":
				delete(body, "workspaceId")
			case "stale":
				body["revision"] = "old"
				want = 409
			case "unknown-action":
				body["action"] = "replace-all"
			case "missing-revision":
				delete(body, "revision")
			case "null-revision":
				body["revision"] = nil
			case "unknown-property":
				body["extra"] = "rejected"
			case "read-only":
				dir := t.TempDir()
				l, err := ledger.Open(dir)
				if err != nil {
					t.Fatal(err)
				}
				l.Close()
				ro, err := ledger.OpenReadOnly(dir)
				if err != nil {
					t.Fatal(err)
				}
				defer ro.Close()
				s.L = ro
				want = 409
			}
			data, _ := json.Marshal(body)
			if mode == "trailing-json" {
				data = append(data, []byte(" {}")...)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("POST", "/app/api/vault/"+it.ID+"/contacts", strings.NewReader(string(data))))
			if w.Code != want {
				t.Fatalf("HTTP%d want%d", w.Code, want)
			}
			got, _ := s.V.Fields(it.ID)
			if len(got) != 1 || len(s.V.Uses(30)) != 0 {
				t.Fatal("rejected request changed state/audit")
			}
			if typ == "identity" && got["email"] != "first@example.test" {
				t.Fatal("rejected request replaced contact")
			}
		})
	}
}
