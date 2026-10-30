package aurora

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestVaultWorkspaceFence(t *testing.T) {
	for _, action := range []string{"list", "save", "reveal", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, _ := newSvc(t)
			it, err := s.V.SaveItem("", "login", "fixture login", map[string]string{"password": "fixture-private-value", "site": "https://fixture.test"})
			if err != nil {
				t.Fatal(err)
			}
			method, path, body := "POST", "/app/api/vault/"+it.ID+"/"+action, `{"workspaceId":"other-workspace"}`
			if action == "list" {
				method, path, body = "GET", "/app/api/vault?workspaceId=other-workspace", ""
			}
			if action == "save" {
				path = "/app/api/vault"
				body = `{"workspaceId":"other-workspace","id":"` + it.ID + `","type":"login","label":"wrong label","fields":{}}`
			}
			r := httptest.NewRecorder()
			s.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
			if r.Code != 409 {
				t.Fatalf("workspace mismatch must fail before access: %s HTTP%d", action, r.Code)
			}
			if strings.Contains(r.Body.String(), "fixture-private-value") {
				t.Fatal("mismatch exposed a value")
			}
			got, ok := s.V.Item(it.ID)
			if !ok || got.Label != it.Label || len(s.V.Uses(30)) != 0 {
				t.Fatal("mismatch changed the item or its audit history")
			}
		})
	}
}

func TestVaultScopedResponsesAndLegacyCaller(t *testing.T) {
	s, _ := newSvc(t)
	for _, scoped := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "scoped"}[scoped], func(t *testing.T) {
			body := map[string]any{"type": "login", "label": "fixture login", "fields": map[string]string{"password": "fixture-private-value"}}
			if scoped {
				body["workspaceId"] = s.workspaceID
			}
			b, _ := json.Marshal(body)
			r := httptest.NewRecorder()
			s.ServeHTTP(r, httptest.NewRequest("POST", "/app/api/vault", strings.NewReader(string(b))))
			if r.Code != 200 {
				t.Fatalf("save HTTP%d", r.Code)
			}
			var item struct{ ID string }
			if json.Unmarshal(r.Body.Bytes(), &item) != nil || item.ID == "" {
				t.Fatal("missing item")
			}
			readBody := `{}`
			query := ""
			if scoped {
				readBody = `{"workspaceId":"` + s.workspaceID + `"}`
				query = "?workspaceId=" + s.workspaceID
			}
			for _, action := range []string{"list", "reveal"} {
				method, path, data := "POST", "/app/api/vault/"+item.ID+"/reveal", readBody
				if action == "list" {
					method, path, data = "GET", "/app/api/vault"+query, ""
				}
				r = httptest.NewRecorder()
				s.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(data)))
				var out map[string]any
				if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &out) != nil || out["workspaceId"] != s.workspaceID {
					t.Fatalf("%s missing workspace binding", action)
				}
				if !strings.Contains(r.Header().Get("Cache-Control"), "no-store") {
					t.Fatal("Vault response is cacheable")
				}
				if action == "list" && strings.Contains(r.Body.String(), "fixture-private-value") {
					t.Fatal("list exposed a value")
				}
			}
			r = httptest.NewRecorder()
			s.ServeHTTP(r, httptest.NewRequest("POST", "/app/api/vault/"+item.ID+"/delete", strings.NewReader(readBody)))
			if r.Code != 200 {
				t.Fatalf("delete HTTP%d", r.Code)
			}
		})
	}
}

func TestVaultDeleteHonorsReadOnlyLedger(t *testing.T) {
	s, _ := newSvc(t)
	it, err := s.V.SaveItem("", "login", "fixture login", map[string]string{"password": "fixture-private-value"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	w, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	ro, err := ledger.OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	s.L = ro
	r := httptest.NewRecorder()
	s.ServeHTTP(r, httptest.NewRequest("POST", "/app/api/vault/"+it.ID+"/delete", strings.NewReader(`{}`)))
	if r.Code != 409 {
		t.Fatalf("read-only delete HTTP%d", r.Code)
	}
	if _, ok := s.V.Item(it.ID); !ok || len(s.V.Uses(30)) != 0 {
		t.Fatal("read-only delete changed the Vault")
	}
}

func TestVaultActionBodies(t *testing.T) {
	for _, action := range []string{"reveal", "delete"} {
		for name, body := range map[string]string{"unknown": `{"extra":true}`, "trailing": `{} {}`, "truncated": `{`, "wrong-type": `{"workspaceId":123}`, "oversized": `{"workspaceId":"` + strings.Repeat("x", 17<<10) + `"}`} {
			t.Run(action+"/"+name, func(t *testing.T) {
				s, _ := newSvc(t)
				it, err := s.V.SaveItem("", "login", "fixture login", map[string]string{"password": "fixture-private-value"})
				if err != nil {
					t.Fatal(err)
				}
				r := httptest.NewRecorder()
				s.ServeHTTP(r, httptest.NewRequest("POST", "/app/api/vault/"+it.ID+"/"+action, strings.NewReader(body)))
				if r.Code != 400 {
					t.Fatalf("invalid request HTTP%d", r.Code)
				}
				if _, ok := s.V.Item(it.ID); !ok || len(s.V.Uses(30)) != 0 || strings.Contains(r.Body.String(), "fixture-private-value") {
					t.Fatal("invalid request accessed Vault")
				}
			})
		}
	}
}

func TestVaultLegacyEmptyActionAndAmbiguousQuery(t *testing.T) {
	s, _ := newSvc(t)
	it, err := s.V.SaveItem("", "login", "fixture login", map[string]string{"password": "fixture-private-value"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRecorder()
	s.ServeHTTP(r, httptest.NewRequest("GET", "/app/api/vault?workspaceId="+s.workspaceID+"&workspaceId=other", nil))
	if r.Code != 400 {
		t.Fatalf("ambiguous query HTTP%d", r.Code)
	}
	for _, action := range []string{"reveal", "delete"} {
		r = httptest.NewRecorder()
		s.ServeHTTP(r, httptest.NewRequest("POST", "/app/api/vault/"+it.ID+"/"+action, nil))
		if r.Code != 200 {
			t.Fatalf("legacy empty %s HTTP%d", action, r.Code)
		}
	}
}
