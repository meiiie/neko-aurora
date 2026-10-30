package aurora

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func memoryPersistenceService(t *testing.T) (*Service, *ledger.Store) {
	t.Helper()
	dir := t.TempDir()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s, err := New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
	if err != nil {
		cancel()
		l.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); s.Wait(); l.Close() })
	return s, l
}

func blockMemoryWrite(t *testing.T, m *memStore, stage string) {
	t.Helper()
	path := m.path + ".tmp"
	if stage == "rename" {
		path = filepath.Join(filepath.Dir(m.path), "blocked-destination")
		m.path = path
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryPersistenceRollbackAndJournal(t *testing.T) {
	for _, scope := range []string{"bot-fixture", teamKey} {
		for _, stage := range []string{"write", "rename"} {
			for _, action := range []string{"add", "edit", "delete", "replace"} {
				t.Run(scope+"/"+stage+"/"+action, func(t *testing.T) {
					dir := t.TempDir()
					m := openMemory(dir)
					if !m.RememberBy(scope, "Original fact", "owner") || !m.RememberBy(scope, "Second fact", "owner") {
						t.Fatal("fixture setup")
					}
					before, journal := m.List(scope), m.Journal(scope, 100)
					disk, err := os.ReadFile(m.path)
					if err != nil {
						t.Fatal(err)
					}
					blockMemoryWrite(t, m, stage)
					switch action {
					case "add":
						if m.RememberBy(scope, "New fact", "owner") {
							t.Error("failed add acknowledged")
						}
					case "edit":
						if m.Edit(scope, 0, "Edited fact", "owner") == nil {
							t.Error("failed edit acknowledged")
						}
					case "delete":
						if m.ForgetBy(scope, 0, "owner") {
							t.Error("failed delete acknowledged")
						}
					case "replace":
						m.Replace(scope, []string{"Summary fact"}, "owner")
					}
					if !reflect.DeepEqual(before, m.List(scope)) {
						t.Error("failed write changed in-memory facts")
					}
					if !reflect.DeepEqual(journal, m.Journal(scope, 100)) {
						t.Error("failed write added a false change record")
					}
					after, err := os.ReadFile(filepath.Join(dir, "memory.json"))
					if err != nil || !bytes.Equal(disk, after) || !reflect.DeepEqual(before, openMemory(dir).List(scope)) {
						t.Error("failed write changed persisted facts")
					}
				})
			}
		}
	}
}

func TestMemoryPersistenceFailedEvictionPreservesOldest(t *testing.T) {
	for _, scope := range []string{"bot-fixture", teamKey} {
		t.Run(scope, func(t *testing.T) {
			m := openMemory(t.TempDir())
			limit := maxMemPerBot
			if scope == teamKey {
				limit = maxTeamMem
			}
			for i := 0; i < limit; i++ {
				if !m.Remember(scope, fmt.Sprintf("Fact %d", i)) {
					t.Fatal("fixture setup")
				}
			}
			before, journal := m.List(scope), m.Journal(scope, 100)
			blockMemoryWrite(t, m, "write")
			if m.Remember(scope, "Evicts oldest only after persistence") || !reflect.DeepEqual(before, m.List(scope)) || !reflect.DeepEqual(journal, m.Journal(scope, 100)) {
				t.Fatal("failed eviction changed facts or history")
			}
		})
	}
}

func memoryPersistenceRequest(s *Service, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRecorder()
	s.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
	return r
}

func TestMemoryPersistenceOwnerAPIWriteFailures(t *testing.T) {
	for _, action := range []string{"add", "edit", "delete", "replace", "import"} {
		t.Run(action, func(t *testing.T) {
			s, l := memoryPersistenceService(t)
			bot := l.Snapshot().Bots[0].ID
			if !s.Mem.Remember(bot, "Original fact") {
				t.Fatal("fixture setup")
			}
			before, journal := s.Mem.List(bot), s.Mem.Journal(bot, 100)
			blockMemoryWrite(t, s.Mem, "write")
			method, path, body := "POST", "/app/api/agents/"+bot+"/memory", `{"text":"New fact"}`
			switch action {
			case "edit":
				path += "/0"
			case "delete":
				method, path, body = "DELETE", path+"/0", ""
			case "replace":
				path, body = path+"/summarize", `{"apply":["Summary fact"]}`
			case "import":
				preview, err := s.Mem.previewImport(bot, "Imported fact")
				if err != nil {
					t.Fatal(err)
				}
				path, body = path+"/import", mustJSON(map[string]any{"revision": preview.Revision, "items": preview.Items})
			}
			r := memoryPersistenceRequest(s, method, path, body)
			if r.Code != 500 {
				t.Errorf("write failure HTTP%d, want 500", r.Code)
			}
			var response map[string]string
			if err := json.Unmarshal(r.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(response["error"], filepath.Dir(s.Mem.path)) {
				t.Error("response exposed local storage path")
			}
			if !reflect.DeepEqual(before, s.Mem.List(bot)) || !reflect.DeepEqual(journal, s.Mem.Journal(bot, 100)) {
				t.Error("failed API write changed facts or history")
			}
		})
	}
}

func TestMemoryPersistenceReadOnlyOwnerActions(t *testing.T) {
	s, l := memoryPersistenceService(t)
	bot := l.Snapshot().Bots[0].ID
	s.Mem.Remember(bot, "Original fact")
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
	before, journal := s.Mem.List(bot), s.Mem.Journal(bot, 100)
	for _, c := range []struct{ method, suffix, body string }{
		{"POST", "", `{"text":"New fact"}`},
		{"POST", "/0", `{"text":"Edited fact"}`},
		{"DELETE", "/0", ""},
		{"POST", "/summarize", `{"apply":["Summary fact"]}`},
	} {
		r := memoryPersistenceRequest(s, c.method, "/app/api/agents/"+bot+"/memory"+c.suffix, c.body)
		if r.Code != 409 || !reflect.DeepEqual(before, s.Mem.List(bot)) || !reflect.DeepEqual(journal, s.Mem.Journal(bot, 100)) {
			t.Errorf("read-only mutation %s %s HTTP%d", c.method, c.suffix, r.Code)
		}
	}
}

func TestMemoryPersistenceSummaryRejectsUnsafeOrOversizedList(t *testing.T) {
	s, l := memoryPersistenceService(t)
	bot := l.Snapshot().Bots[0].ID
	s.Mem.Remember(bot, "Original fact")
	s.Mem.mask = func(s string) string { return strings.ReplaceAll(s, "fixture-private-value", "[redacted]") }
	before, journal := s.Mem.List(bot), s.Mem.Journal(bot, 100)
	tooMany := []string{}
	for i := 0; i <= maxMemPerBot; i++ {
		tooMany = append(tooMany, fmt.Sprintf("Fact %d", i))
	}
	for _, list := range [][]string{{"Valid fact", "fixture-private-value"}, {"Valid fact", "password example"}, {"duplicate", "DUPLICATE"}, {strings.Repeat("x", 301)}, tooMany} {
		r := memoryPersistenceRequest(s, "POST", "/app/api/agents/"+bot+"/memory/summarize", mustJSON(map[string]any{"apply": list}))
		if r.Code != 400 || !reflect.DeepEqual(before, s.Mem.List(bot)) || !reflect.DeepEqual(journal, s.Mem.Journal(bot, 100)) {
			t.Errorf("invalid summary changed store or was accepted: HTTP%d", r.Code)
		}
	}
}

func TestMemoryPersistenceSuccessfulRetryAndConcurrentWrites(t *testing.T) {
	s, l := memoryPersistenceService(t)
	bot := l.Snapshot().Bots[0].ID
	before := s.Mem.Journal(bot, 100)
	blockMemoryWrite(t, s.Mem, "write")
	path := "/app/api/agents/" + bot + "/memory"
	if r := memoryPersistenceRequest(s, "POST", path, `{"text":"First fact"}`); r.Code != 500 {
		t.Fatal(r.Code)
	}
	if err := os.Remove(s.Mem.path + ".tmp"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if r := memoryPersistenceRequest(s, "POST", path, `{"text":"First fact"}`); r.Code != 200 {
			t.Fatal(r.Code)
		}
	}
	if len(s.Mem.List(bot)) != 1 || len(s.Mem.Journal(bot, 100)) != len(before)+1 {
		t.Fatal("failed write or duplicate created history")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if !s.Mem.Remember(bot, fmt.Sprintf("Concurrent fact %d", i)) {
				t.Errorf("write %d failed", i)
			}
		}(i)
	}
	wg.Wait()
	if got := s.Mem.List(bot); len(got) != 9 || !reflect.DeepEqual(got, openMemory(filepath.Dir(s.Mem.path)).List(bot)) {
		t.Fatal("same-process concurrent writes lost persisted facts")
	}
}

func TestMemoryPersistenceUnreadableStoreCannotBeOverwritten(t *testing.T) {
	for _, data := range []string{"null", "{broken", `{"bot":[123]}`} {
		t.Run(data, func(t *testing.T) {
			s, l := memoryPersistenceService(t)
			bot := l.Snapshot().Bots[0].ID
			path := s.Mem.path
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			s.Mem = openMemory(filepath.Dir(path))
			read := memoryPersistenceRequest(s, "GET", "/app/api/agents/"+bot+"/memory", "")
			if read.Code != 500 {
				t.Errorf("unreadable store presented as healthy: HTTP%d", read.Code)
			}
			write := memoryPersistenceRequest(s, "POST", "/app/api/agents/"+bot+"/memory", `{"text":"Must not replace corrupt store"}`)
			if write.Code != 500 {
				t.Errorf("unreadable store mutation HTTP%d", write.Code)
			}
			for _, c := range []struct{ method, suffix, body string }{
				{"POST", "/0", `{"text":"Edited fact"}`},
				{"DELETE", "/0", ""},
				{"POST", "/ask", `{"q":"What do you know?"}`},
				{"POST", "/summarize", `{}`},
				{"POST", "/summarize", `{"apply":["Summary fact"]}`},
				{"POST", "/import/preview", `{"text":"Import fact"}`},
				{"POST", "/import", `{"revision":"stale","items":["Import fact"]}`},
			} {
				if r := memoryPersistenceRequest(s, c.method, "/app/api/agents/"+bot+"/memory"+c.suffix, c.body); r.Code != 500 {
					t.Errorf("unreadable store endpoint %s HTTP%d", c.suffix, r.Code)
				}
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != data {
				t.Fatal("unreadable file was overwritten")
			}
		})
	}
}
