package aurora

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestScheduleOverviewWorkspaceBoundary(t *testing.T) {
	s, _ := scheduleEditorFixture(t)
	for _, tc := range []struct {
		name, query string
		status      int
	}{
		{"scoped", "workspaceId=" + url.QueryEscape(s.workspaceID), 200},
		{"legacy-profile", "", 200},
		{"foreign-workspace", "workspaceId=another-workspace", 409},
		{"empty-workspace", "workspaceId=", 409},
		{"duplicate-workspace", "workspaceId=" + url.QueryEscape(s.workspaceID) + "&workspaceId=another", 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("GET", "/app/api/schedules?"+tc.query, nil))
			if w.Code != tc.status {
				t.Fatalf("status %d, expected %d", w.Code, tc.status)
			}
			var body struct {
				WorkspaceID string     `json:"workspaceId"`
				Schedules   []schedule `json:"schedules"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tc.status == 200 && (body.WorkspaceID != s.workspaceID || len(body.Schedules) != 1) {
				t.Fatal("overview lost workspace identity or schedules")
			}
			if tc.status != 200 && (len(body.Schedules) != 0 || body.WorkspaceID != "") {
				t.Fatal("foreign request disclosed workspace data")
			}
		})
	}
}
