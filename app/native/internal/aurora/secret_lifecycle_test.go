package aurora

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/vault"
)

func TestSecretWorkspaceLifecycle(t *testing.T) {
	for _, action := range []string{"list", "save", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, _ := newSvc(t)
			if _, err := s.V.Set(vault.Meta{Name: "FIXTURE_SECRET", Kind: "secret"}, "fixture-original"); err != nil {
				t.Fatal(err)
			}
			method, path, body := "POST", "/app/api/secrets", `{"workspaceId":"other","name":"FIXTURE_SECRET","value":"fixture-replacement"}`
			if action == "delete" {
				path, body = "/app/api/secrets/FIXTURE_SECRET/delete", `{"workspaceId":"other"}`
			}
			if action == "list" {
				method, path, body = "GET", "/app/api/secrets?workspaceId=other", ""
			}
			r := httptest.NewRecorder()
			s.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
			if r.Code != 409 {
				t.Fatalf("wrong workspace HTTP%d", r.Code)
			}
			if v, ok := s.V.Get("FIXTURE_SECRET"); !ok || v != "fixture-original" {
				t.Fatal("wrong workspace changed credential")
			}
			if strings.Contains(r.Body.String(), "fixture-original") {
				t.Fatal("response exposed credential")
			}
		})
	}
}

func TestSecretScopedAndLegacyLifecycle(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "scoped"}[scoped], func(t *testing.T) {
			s, _ := newSvc(t)
			body := map[string]any{"name": "FIXTURE_SECRET", "value": "fixture-private", "label": "Fixture"}
			query, deletion := "", ""
			if scoped {
				body["workspaceId"] = s.workspaceID
				query = "?workspaceId=" + s.workspaceID
				deletion = `{"workspaceId":"` + s.workspaceID + `"}`
			}
			b, _ := json.Marshal(body)
			for _, req := range []struct{ method, path, body string }{
				{"POST", "/app/api/secrets", string(b)}, {"GET", "/app/api/secrets" + query, ""}, {"POST", "/app/api/secrets/FIXTURE_SECRET/delete", deletion},
			} {
				r := httptest.NewRecorder()
				s.ServeHTTP(r, httptest.NewRequest(req.method, req.path, strings.NewReader(req.body)))
				if r.Code != 200 {
					t.Fatalf("%s HTTP%d", req.path, r.Code)
				}
				if r.Header().Get("Cache-Control") != "no-store" || strings.Contains(r.Body.String(), "fixture-private") {
					t.Fatal("credential metadata cache/exposure contract failed")
				}
				if req.method == "GET" {
					var out struct{ WorkspaceID string }
					if json.Unmarshal(r.Body.Bytes(), &out) != nil || out.WorkspaceID != s.workspaceID {
						t.Fatal("list missing workspace marker")
					}
				}
			}
			if s.V.Has("FIXTURE_SECRET") {
				t.Fatal("delete did not persist")
			}
		})
	}
}

func TestSecretReadOnlyLifecycle(t *testing.T) {
	for _, action := range []string{"save", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, _ := newSvc(t)
			if _, err := s.V.Set(vault.Meta{Name: "FIXTURE_SECRET", Kind: "secret"}, "fixture-original"); err != nil {
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
			path, body := "/app/api/secrets", `{"name":"FIXTURE_SECRET","value":"fixture-replacement"}`
			if action == "delete" {
				path, body = "/app/api/secrets/FIXTURE_SECRET/delete", `{}`
			}
			r := httptest.NewRecorder()
			s.ServeHTTP(r, httptest.NewRequest("POST", path, strings.NewReader(body)))
			if r.Code != 409 {
				t.Fatalf("read-only mutation HTTP%d", r.Code)
			}
			if v, ok := s.V.Get("FIXTURE_SECRET"); !ok || v != "fixture-original" {
				t.Fatal("read-only mutation changed credential")
			}
		})
	}
}

func TestSecretInvalidLifecycleRequests(t *testing.T) {
	for name, query := range map[string]string{"empty": "workspaceId=", "duplicate": "workspaceId=a&workspaceId=b", "malformed": "workspaceId=%zz", "separator": "workspaceId=a;b"} {
		t.Run("query/"+name, func(t *testing.T) {
			s, _ := newSvc(t)
			r := httptest.NewRecorder()
			s.ServeHTTP(r, httptest.NewRequest("GET", "/app/api/secrets?"+query, nil))
			if r.Code != 400 {
				t.Fatalf("invalid query HTTP%d", r.Code)
			}
		})
	}
	for _, action := range []string{"save", "delete"} {
		for name, body := range map[string]string{"unknown": `{"extra":true}`, "trailing": `{"name":"FIXTURE_SECRET","value":"fixture-replacement"} {}`, "truncated": `{`, "wrong-type": `{"workspaceId":123}`, "oversized": `{"workspaceId":"` + strings.Repeat("x", 17<<10) + `"}`} {
			t.Run(action+"/"+name, func(t *testing.T) {
				s, _ := newSvc(t)
				if _, err := s.V.Set(vault.Meta{Name: "FIXTURE_SECRET", Kind: "secret"}, "fixture-original"); err != nil {
					t.Fatal(err)
				}
				path := "/app/api/secrets"
				if action == "delete" {
					path += "/FIXTURE_SECRET/delete"
					if name == "trailing" {
						body = `{} {}`
					}
				}
				r := httptest.NewRecorder()
				s.ServeHTTP(r, httptest.NewRequest("POST", path, strings.NewReader(body)))
				if r.Code != 400 {
					t.Fatalf("invalid request HTTP%d", r.Code)
				}
				if v, ok := s.V.Get("FIXTURE_SECRET"); !ok || v != "fixture-original" {
					t.Fatal("invalid request changed credential")
				}
			})
		}
	}
}
