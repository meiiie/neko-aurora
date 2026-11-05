package aurora

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/computer"
)

func TestDisabledComputerHasInspectableStatus(t *testing.T) {
	t.Setenv("AURORA_COMPUTER", "off")
	s, _ := newSvc(t)
	bot := firstBot(s)
	r := httptest.NewRecorder()
	s.ServeHTTP(r, httptest.NewRequest("GET", "/app/api/agents/"+bot+"/computer", nil))
	if r.Code != 200 {
		t.Fatalf("status should describe setup, not fail to fetch: HTTP %d %s", r.Code, r.Body.String())
	}
	var status struct {
		Available, FileAccess, CanWrite bool
		SetupCode                       string
	}
	if err := json.Unmarshal(r.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Available || status.FileAccess || status.CanWrite || status.SetupCode != "disabled" {
		t.Fatalf("incorrect disabled status: %+v", status)
	}
	r = httptest.NewRecorder()
	s.ServeHTTP(r, httptest.NewRequest("GET", "/app/api/agents/missing/computer", nil))
	if r.Code != 404 {
		t.Fatalf("unknown bot: HTTP %d", r.Code)
	}
}

func TestComputerPromptReportsDisabledCapabilities(t *testing.T) {
	s := &Service{}
	if note := s.computerNote("bot-meiiie"); !strings.Contains(note, "KHÔNG có workspace") {
		t.Fatalf("disabled tools advertised without guidance: %q", note)
	}
}

func TestComputerFileAccessDoesNotDependOnShell(t *testing.T) {
	m, err := computer.New(t.TempDir(), &computer.Container{Runtime: "unconfigured"})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := m.Status("bot-test")
	if err != nil || empty.Files == nil || empty.Log == nil || len(empty.Files) != 0 || len(empty.Log) != 0 {
		t.Fatalf("fresh workspace must expose empty collections: %+v %v", empty, err)
	}
	if err := m.WriteFile("bot-test", "site/index.html", "<!doctype html><title>Aurora</title>"); err != nil {
		t.Fatal(err)
	}
	st, err := m.Status("bot-test")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(st)
	var caps struct{ Available, FileAccess, CanWrite bool }
	_ = json.Unmarshal(encoded, &caps)
	if caps.Available || !caps.FileAccess || !caps.CanWrite {
		t.Fatalf("files must remain usable without a shell: %s", encoded)
	}
	if err := m.SetPolicy("bot-test", computer.Policy{Preset: "readonly"}); err != nil {
		t.Fatal(err)
	}
	st, _ = m.Status("bot-test")
	encoded, _ = json.Marshal(st)
	_ = json.Unmarshal(encoded, &caps)
	if caps.CanWrite || !caps.FileAccess {
		t.Fatalf("read-only policy must be reflected: %s", encoded)
	}
}
