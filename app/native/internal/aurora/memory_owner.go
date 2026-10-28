package aurora

import (
	"encoding/hex"
	"net/http"
	"net/url"

	"nekoaurora.local/native/internal/ledger"
)

func validMemoryRevision(revision string) bool {
	if len(revision) != 64 {
		return false
	}
	_, err := hex.DecodeString(revision)
	return err == nil
}

// Omitted query fields preserve legacy clients. The browser always supplies
// workspaceId and a revision for list mutations; neither is an authorization grant.
func (s *Service) memoryRequest(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	w.Header().Set("Cache-Control", "no-store")
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		fail(w, ErrInvalid)
		return nil, false
	}
	for _, key := range []string{"workspaceId", "revision", "scope"} {
		if values, ok := q[key]; ok && (len(values) != 1 || values[0] == "") {
			fail(w, ErrInvalid)
			return nil, false
		}
	}
	if scope, ok := q["workspaceId"]; ok && scope[0] != s.workspaceID {
		fail(w, ledger.ErrConflict)
		return nil, false
	}
	if revision, ok := q["revision"]; ok && !validMemoryRevision(revision[0]) {
		fail(w, ErrInvalid)
		return nil, false
	}
	if scope := q.Get("scope"); scope != "" && scope != "team" {
		fail(w, ErrInvalid)
		return nil, false
	}
	if s.Mem.loadErr != nil {
		writeJSON(w, 500, map[string]string{"error": "memory_store_unavailable"})
		return nil, false
	}
	return q["revision"], true
}

// Caller holds ms.mu, including across the mutation and its disk write.
func (ms *memStore) checkRevisionLocked(bot string, revision []string) error {
	if len(revision) == 0 {
		return nil
	}
	if len(revision) != 1 || !validMemoryRevision(revision[0]) {
		return ErrInvalid
	}
	if revision[0] != memoryRevision(ms.m[bot]) {
		return ledger.ErrConflict
	}
	return nil
}

func (ms *memStore) snapshot(bot string) (items, team []memItem, revision, teamRevision string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return append([]memItem{}, ms.m[bot]...), append([]memItem{}, ms.m[teamKey]...), memoryRevision(ms.m[bot]), memoryRevision(ms.m[teamKey])
}
