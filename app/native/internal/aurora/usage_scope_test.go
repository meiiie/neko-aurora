package aurora

import (
	"encoding/json"
	"testing"
)

func TestUsageWorkspaceScope(t *testing.T) {
	s, _ := memoryPersistenceService(t)
	for _, c := range []struct {
		query string
		code  int
	}{
		{"", 200}, {"?range=1&workspaceId=" + s.workspaceID, 200}, {"?range=7&workspaceId=" + s.workspaceID, 200}, {"?range=30&workspaceId=" + s.workspaceID, 200}, {"?range=365&workspaceId=" + s.workspaceID, 200},
		{"?workspaceId=other-workspace", 409}, {"?workspaceId=", 400}, {"?workspaceId=" + s.workspaceID + "&workspaceId=" + s.workspaceID, 400}, {"?workspaceId=%zz", 400}, {"?range=1&range=30", 400}, {"?range=bad", 400},
	} {
		t.Run(c.query, func(t *testing.T) {
			r := memoryPersistenceRequest(s, "GET", "/app/api/usage"+c.query, "")
			if r.Code != c.code || r.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("HTTP%d cache=%q", r.Code, r.Header().Get("Cache-Control"))
			}
			if r.Code == 200 {
				var body struct {
					WorkspaceID string `json:"workspaceId"`
					Estimated   bool   `json:"estimated"`
				}
				if json.Unmarshal(r.Body.Bytes(), &body) != nil || body.WorkspaceID != s.workspaceID || !body.Estimated {
					t.Fatal("usage response missing scope or estimate marker")
				}
			}
		})
	}
}
