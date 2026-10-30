package aurora

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestPageAPICreatesVersionedOwnerDocument(t *testing.T) {
	s, _ := newSvc(t)
	r := httptest.NewRecorder()
	body := mustJSON(map[string]any{"workspaceId": s.workspaceID, "clientNonce": "page-create-fixture", "title": "Tài liệu Aurora", "text": "# Xin chào\nBản đầu tiên.", "baseRevision": 0})
	s.ServeHTTP(r, httptest.NewRequest("POST", "/app/api/pages/save", strings.NewReader(body)))
	if r.Code != 200 {
		t.Fatalf("page save returned %d instead of a version receipt", r.Code)
	}
}

func pageRequest(s *Service, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRecorder()
	s.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
	return r
}

func TestPageAPIWorkspaceConflictReceiptAndExactVersionExport(t *testing.T) {
	s, _ := newSvc(t)
	scope := "?workspaceId=" + url.QueryEscape(s.workspaceID)
	body := map[string]any{"workspaceId": s.workspaceID, "clientNonce": "page-api-create", "title": "Tài liệu 🐈", "text": "# Bản gốc\n<script>fixture</script>\n"}
	first := pageRequest(s, "POST", "/app/api/pages/save", mustJSON(body))
	var saved struct {
		WorkspaceID string              `json:"workspaceId"`
		Page        ledger.PageRevision `json:"page"`
	}
	if first.Code != 200 || json.Unmarshal(first.Body.Bytes(), &saved) != nil || saved.WorkspaceID != s.workspaceID || saved.Page.Revision != 1 {
		t.Fatal("create receipt", first.Code)
	}
	p := saved.Page
	body["id"], body["baseRevision"], body["text"], body["clientNonce"] = p.ID, 1, "Bản mới", "page-api-edit"
	if r := pageRequest(s, "POST", "/app/api/pages/save", mustJSON(body)); r.Code != 200 {
		t.Fatal("update", r.Code)
	}
	if r := pageRequest(s, "POST", "/app/api/pages/save", mustJSON(body)); r.Code != 200 || !strings.Contains(r.Body.String(), `"duplicate":true`) {
		t.Fatal("duplicate receipt", r.Code)
	}
	body["clientNonce"] = "page-api-stale"
	if r := pageRequest(s, "POST", "/app/api/pages/save", mustJSON(body)); r.Code != 409 {
		t.Fatal("stale write", r.Code)
	}
	for _, endpoint := range []string{"/pages", "/pages/" + p.ID, "/pages/" + p.ID + "/history", "/page-receipts/page-api-edit"} {
		r := pageRequest(s, "GET", "/app/api"+endpoint+scope, "")
		if r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" || r.Header().Get("X-Aurora-Workspace") != s.workspaceID {
			t.Fatal("scoped read", endpoint, r.Code)
		}
		if r := pageRequest(s, "GET", "/app/api"+endpoint+"?workspaceId=wrong", ""); r.Code != 409 {
			t.Fatal("cross-workspace read", endpoint, r.Code)
		}
	}
	for _, revision := range []string{"", "&revision=0", "&revision=01", "&revision=-1", "&revision=1&revision=2"} {
		if r := pageRequest(s, "GET", "/app/api/pages/"+p.ID+"/export"+scope+revision, ""); r.Code != 400 {
			t.Fatal("ambiguous export version", revision, r.Code)
		}
	}
	export := pageRequest(s, "GET", "/app/api/pages/"+p.ID+"/export"+scope+"&revision=1", "")
	if export.Code != 200 || export.Body.String() != p.Text || export.Header().Get("X-Aurora-Page") != p.ID || export.Header().Get("X-Aurora-Revision") != "1" || export.Header().Get("X-Aurora-SHA256") != p.SHA256 || !strings.HasPrefix(export.Header().Get("Content-Disposition"), "attachment;") || export.Header().Get("Content-Length") != fmt.Sprint(len(p.Text)) {
		t.Fatal("export did not bind exact original bytes", export.Code)
	}
	if r := pageRequest(s, "GET", "/app/api/pages"+scope, ""); strings.Contains(r.Body.String(), "Bản mới") {
		t.Fatal("list returned full page contents")
	}
	if r := pageRequest(s, "GET", "/app/api/page-receipts/page-api-unknown"+scope, ""); r.Code != 200 || !strings.Contains(r.Body.String(), `"found":false`) {
		t.Fatal("absent receipt was not explicit")
	}
}

func TestPageAPIStrictBoundedInputDoesNotMutate(t *testing.T) {
	s, l := newSvc(t)
	valid := mustJSON(map[string]any{"workspaceId": s.workspaceID, "clientNonce": "page-invalid-fixture", "title": "Fixture", "text": "unchanged"})
	before := l.Snapshot().Sequence
	for _, body := range []string{
		"{", valid + "{}", strings.TrimSuffix(valid, "}") + `,"futureField":true}`,
		strings.Replace(valid, "unchanged", string([]byte{255}), 1),
		strings.Replace(valid, "unchanged", strings.Repeat("x", ledger.MaxPageTextBytes+1), 1),
		strings.Replace(valid, "unchanged", strings.Repeat("x", 512<<10), 1),
	} {
		if r := pageRequest(s, "POST", "/app/api/pages/save", body); r.Code != 400 {
			t.Fatal("invalid body", r.Code)
		}
	}
	if r := pageRequest(s, "POST", "/app/api/pages/save", strings.Replace(valid, s.workspaceID, "wrong", 1)); r.Code != 409 {
		t.Fatal("wrong write workspace", r.Code)
	}
	for _, query := range []string{"", "?workspaceId=", "?workspaceId=wrong", "?workspaceId=" + s.workspaceID + "&workspaceId=" + s.workspaceID, "?workspaceId=" + s.workspaceID + "&=ignored", "?workspaceId=%XX"} {
		if r := pageRequest(s, "GET", "/app/api/pages"+query, ""); r.Code < 400 {
			t.Fatal("invalid read scope accepted", query)
		}
	}
	if l.Snapshot().Sequence != before || len(l.Pages()) != 0 {
		t.Fatal("invalid requests changed journal")
	}
}
