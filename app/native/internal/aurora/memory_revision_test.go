package aurora

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type memoryRevisionView struct {
	Items        []memItem `json:"items"`
	Team         []memItem `json:"team"`
	Revision     string    `json:"revision"`
	TeamRevision string    `json:"teamRevision"`
	WorkspaceID  string    `json:"workspaceId"`
}

func memoryRevisionRead(t *testing.T, s *Service, bot string) memoryRevisionView {
	t.Helper()
	r := memoryPersistenceRequest(s, "GET", "/app/api/agents/"+bot+"/memory?workspaceId="+url.QueryEscape(s.workspaceID), "")
	var out memoryRevisionView
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &out) != nil {
		t.Fatalf("snapshot HTTP%d", r.Code)
	}
	return out
}

func TestMemoryRevisionShiftedIndex(t *testing.T) {
	for _, method := range []string{"POST", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			s, l := memoryPersistenceService(t)
			bot := l.Snapshot().Bots[0].ID
			for _, text := range []string{"First fact", "Target fact", "Last fact"} {
				s.Mem.Remember(bot, text)
			}
			rev := memoryRevision(s.Mem.List(bot))
			s.Mem.Forget(bot, 0)
			before, journal := s.Mem.List(bot), s.Mem.Journal(bot, 100)
			r := memoryPersistenceRequest(s, method, "/app/api/agents/"+bot+"/memory/1?workspaceId="+s.workspaceID+"&revision="+rev, `{"text":"Edited target"}`)
			if r.Code != 409 || !reflect.DeepEqual(before, s.Mem.List(bot)) || !reflect.DeepEqual(journal, s.Mem.Journal(bot, 100)) {
				t.Fatalf("stale positional mutation HTTP%d changed=%v", r.Code, !reflect.DeepEqual(before, s.Mem.List(bot)))
			}
		})
	}
}

func TestMemoryRevisionSnapshotAndLegacy(t *testing.T) {
	s, l := memoryPersistenceService(t)
	bot := l.Snapshot().Bots[0].ID
	view := memoryRevisionRead(t, s, bot)
	preview, _ := s.Mem.previewImport(bot, "Imported fact")
	if view.WorkspaceID != s.workspaceID || view.Items == nil || view.Team == nil || view.Revision != preview.Revision || len(view.TeamRevision) != 64 {
		t.Fatal("missing or inconsistent empty snapshot")
	}
	path := "/app/api/agents/" + bot + "/memory"
	for _, method := range []string{"GET", "POST"} {
		r := memoryPersistenceRequest(s, method, path, `{"text":"Legacy fact"}`)
		if r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("legacy %s HTTP%d cache=%q", method, r.Code, r.Header().Get("Cache-Control"))
		}
	}
	view = memoryRevisionRead(t, s, bot)
	if view.Revision != memoryRevision(view.Items) {
		t.Fatal("revision does not match returned facts")
	}
}

func TestMemoryRevisionWorkspaceAndMalformedQuery(t *testing.T) {
	s, l := memoryPersistenceService(t)
	other, _ := memoryPersistenceService(t)
	bot := l.Snapshot().Bots[0].ID
	s.Mem.Remember(bot, "Original fact")
	s.Mem.Remember(teamKey, "Shared fact")
	before, team, journal := s.Mem.List(bot), s.Mem.List(teamKey), s.Mem.Journal(bot, 100)
	for _, c := range []struct{ method, suffix, body string }{
		{"GET", "", ""}, {"POST", "", `{"text":"New fact"}`}, {"POST", "/0", `{"text":"Edited fact"}`}, {"DELETE", "/0", ""},
		{"POST", "/ask", `{"q":"What do you know?"}`}, {"POST", "/summarize", `{}`}, {"POST", "/summarize", `{"apply":["Summary fact"]}`},
		{"POST", "/import/preview", `{"text":"Imported fact"}`}, {"POST", "/import", `{"revision":"fixture","items":["Imported fact"]}`},
	} {
		t.Run(c.method+c.suffix+c.body, func(t *testing.T) {
			for _, q := range []struct {
				query string
				code  int
			}{
				{"workspaceId=" + other.workspaceID, 409}, {"workspaceId=", 400}, {"workspaceId=" + s.workspaceID + "&workspaceId=" + s.workspaceID, 400},
				{"workspaceId=%zz", 400}, {"workspaceId=" + s.workspaceID + ";ignored=x", 400}, {"revision=", 400}, {"revision=bad", 400},
				{"revision=" + strings.Repeat("a", 64) + "&revision=" + strings.Repeat("b", 64), 400}, {"scope=team&scope=team", 400},
			} {
				r := memoryPersistenceRequest(s, c.method, "/app/api/agents/"+bot+"/memory"+c.suffix+"?"+q.query, c.body)
				if r.Code != q.code || r.Header().Get("Cache-Control") != "no-store" {
					t.Errorf("query rejected incorrectly: HTTP%d want%d", r.Code, q.code)
				}
			}
		})
	}
	if !reflect.DeepEqual(before, s.Mem.List(bot)) || !reflect.DeepEqual(team, s.Mem.List(teamKey)) || !reflect.DeepEqual(journal, s.Mem.Journal(bot, 100)) {
		t.Fatal("rejected requests changed memory")
	}
}

func TestMemoryRevisionMutationsAndIsolation(t *testing.T) {
	for _, scope := range []string{"private", "team"} {
		for _, action := range []string{"add", "edit", "delete", "summary"} {
			t.Run(scope+"/"+action, func(t *testing.T) {
				s, l := memoryPersistenceService(t)
				bot := l.Snapshot().Bots[0].ID
				s.Mem.Remember(bot, "Private fact")
				s.Mem.Remember(teamKey, "Shared fact")
				view := memoryRevisionRead(t, s, bot)
				key, rev, query, body := bot, view.Revision, "", `{"text":"Changed fact"}`
				if scope == "team" {
					key, rev, query = teamKey, view.TeamRevision, "&scope=team"
					body = `{"text":"Changed fact","team":true}`
				}
				other := teamKey
				if key == teamKey {
					other = bot
				}
				otherBefore := s.Mem.List(other)
				method, path := "POST", "/app/api/agents/"+bot+"/memory"
				switch action {
				case "edit":
					path += "/0"
				case "delete":
					method = "DELETE"
					path += "/0"
				case "summary":
					path += "/summarize"
					body = mustJSON(map[string]any{"scope": map[bool]string{true: "team", false: ""}[key == teamKey], "apply": []string{"Summary fact"}})
				}
				path += "?workspaceId=" + s.workspaceID + "&revision=" + rev + query
				r := memoryPersistenceRequest(s, method, path, body)
				if r.Code != 200 {
					t.Fatalf("current revision HTTP%d", r.Code)
				}
				after, journal := s.Mem.List(key), s.Mem.Journal(bot, 100)
				retry := memoryPersistenceRequest(s, method, path, body)
				if retry.Code != 409 || !reflect.DeepEqual(after, s.Mem.List(key)) || !reflect.DeepEqual(journal, s.Mem.Journal(bot, 100)) || !reflect.DeepEqual(otherBefore, s.Mem.List(other)) {
					t.Fatalf("replay/isolation HTTP%d", retry.Code)
				}
			})
		}
	}
}

func TestMemoryRevisionConcurrentAndRollback(t *testing.T) {
	s, l := memoryPersistenceService(t)
	bot := l.Snapshot().Bots[0].ID
	s.Mem.Remember(bot, "Original fact")
	rev := memoryRevisionRead(t, s, bot).Revision
	path := "/app/api/agents/" + bot + "/memory/0?workspaceId=" + s.workspaceID + "&revision=" + rev
	blockMemoryWrite(t, s.Mem, "write")
	if r := memoryPersistenceRequest(s, "POST", path, `{"text":"Failed edit"}`); r.Code != 500 {
		t.Fatalf("storage HTTP%d", r.Code)
	}
	if memoryRevisionRead(t, s, bot).Revision != rev {
		t.Fatal("failed write changed revision")
	}
	if err := os.Remove(s.Mem.path + ".tmp"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	codes := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes <- memoryPersistenceRequest(s, "POST", path, fmt.Sprintf(`{"text":"Competing edit %d"}`, i)).Code
		}(i)
	}
	wg.Wait()
	close(codes)
	ok, conflict := 0, 0
	for code := range codes {
		switch code {
		case 200:
			ok++
		case 409:
			conflict++
		default:
			t.Errorf("unexpected HTTP%d", code)
		}
	}
	if ok != 1 || conflict != 7 || len(s.Mem.Journal(bot, 100)) != 2 {
		t.Fatalf("accepted=%d conflicts=%d", ok, conflict)
	}
}

type memoryRevisionModel struct {
	during func()
	calls  int
}

func (m *memoryRevisionModel) Configured() bool  { return true }
func (m *memoryRevisionModel) Runtime() string   { return "direct-http" }
func (m *memoryRevisionModel) ModelName() string { return "memory-fixture" }
func (m *memoryRevisionModel) Stream(_ context.Context, _ []ChatMsg, on func(string)) (string, error) {
	m.calls++
	m.during()
	out := `["Reviewed summary"]`
	on(out)
	return out, nil
}

func TestMemoryRevisionSummaryUsesModelSourceSnapshot(t *testing.T) {
	s, l := memoryPersistenceService(t)
	bot := l.Snapshot().Bots[0].ID
	s.Mem.Remember(bot, "First fact")
	s.Mem.Remember(bot, "Second fact")
	before := memoryRevisionRead(t, s, bot).Revision
	model := &memoryRevisionModel{during: func() { s.Mem.Remember(bot, "Added during model call") }}
	s.AI = model
	path := "/app/api/agents/" + bot + "/memory/summarize?workspaceId=" + s.workspaceID
	r := memoryPersistenceRequest(s, "POST", path, `{}`)
	var proposal struct {
		Revision string   `json:"revision"`
		Proposal []string `json:"proposal"`
	}
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &proposal) != nil || proposal.Revision != before || len(proposal.Proposal) != 1 {
		t.Fatal("proposal is not bound to source snapshot")
	}
	r = memoryPersistenceRequest(s, "POST", path+"&revision="+proposal.Revision, mustJSON(map[string]any{"apply": proposal.Proposal}))
	if r.Code != 409 || len(s.Mem.List(bot)) != 3 || model.calls != 1 {
		t.Fatalf("stale proposal applied or model replayed: HTTP%d calls%d", r.Code, model.calls)
	}
}

func TestMemoryRevisionOtherScopeDoesNotConflict(t *testing.T) {
	s, l := memoryPersistenceService(t)
	bot := l.Snapshot().Bots[0].ID
	s.Mem.Remember(bot, "Private fact")
	s.Mem.Remember(teamKey, "Shared fact")
	view := memoryRevisionRead(t, s, bot)
	s.Mem.Remember(bot, "Another private fact")
	path := "/app/api/agents/" + bot + "/memory/0?workspaceId=" + s.workspaceID + "&revision=" + view.TeamRevision
	if r := memoryPersistenceRequest(s, "POST", path, `{"text":"Shared update","team":true}`); r.Code != 200 {
		t.Fatalf("private update invalidated team revision: HTTP%d", r.Code)
	}
	if len(s.Mem.List(bot)) != 2 || s.Mem.List(teamKey)[0].Text != "Shared update" {
		t.Fatal("scope isolation failed")
	}
}

func TestMemoryRevisionMalformedPositionAndEmptySummary(t *testing.T) {
	s, l := memoryPersistenceService(t)
	bot := l.Snapshot().Bots[0].ID
	path := "/app/api/agents/" + bot + "/memory"
	r := memoryPersistenceRequest(s, "POST", path+"/summarize", `{}`)
	var proposal struct {
		Revision string `json:"revision"`
	}
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &proposal) != nil || proposal.Revision != memoryRevisionRead(t, s, bot).Revision {
		t.Fatal("empty proposal revision missing")
	}
	s.Mem.Remember(bot, "Original fact")
	for _, method := range []string{"POST", "DELETE"} {
		for _, pos := range []string{"0junk", "0%20ignored", "-1", "999999999999999999999999999"} {
			r := memoryPersistenceRequest(s, method, path+"/"+pos, `{"text":"Wrong edit"}`)
			if r.Code != 404 || s.Mem.List(bot)[0].Text != "Original fact" {
				t.Errorf("bad index accepted: %s HTTP%d", pos, r.Code)
			}
		}
	}
}
