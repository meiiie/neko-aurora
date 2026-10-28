package aurora

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"nekoaurora.local/native/internal/ledger"
)

type memoryImportPreview struct {
	Items      []string `json:"items"`
	Revision   string   `json:"revision"`
	Remaining  int      `json:"remaining"`
	Duplicates int      `json:"duplicates"`
	Rejected   int      `json:"rejected"`
}

func memoryRevision(items []memItem) string {
	b, _ := json.Marshal(items)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (ms *memStore) safeImportFact(text string) bool {
	return text != "" && utf8.RuneCountInString(text) <= 300 && utf8.ValidString(text) &&
		!strings.ContainsAny(text, "\r\n\x00") && !looksSecret(text) && (ms.mask == nil || ms.mask(text) == text)
}

func (ms *memStore) previewImport(bot, text string) (memoryImportPreview, error) {
	if ms.loadErr != nil {
		return memoryImportPreview{}, ms.loadErr
	}
	if len(text) > 32768 || !utf8.ValidString(text) {
		return memoryImportPreview{}, ErrInvalid
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) > 100 {
		return memoryImportPreview{}, ErrInvalid
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	old := ms.m[bot]
	out := memoryImportPreview{Items: []string{}, Revision: memoryRevision(old), Remaining: max(0, maxMemPerBot-len(old))}
	seen := map[string]bool{}
	for _, x := range old {
		seen[strings.ToLower(x.Text)] = true
	}
	for _, line := range lines {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		if !ms.safeImportFact(text) {
			out.Rejected++
			continue
		}
		key := strings.ToLower(text)
		if seen[key] {
			out.Duplicates++
			continue
		}
		seen[key] = true
		out.Items = append(out.Items, text)
	}
	return out, nil
}

func (ms *memStore) applyImport(bot, revision string, items []string) error {
	if ms.loadErr != nil {
		return ms.loadErr
	}
	if len(items) == 0 || len(items) > maxMemPerBot {
		return ErrInvalid
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	old := ms.m[bot]
	if revision != memoryRevision(old) {
		return ledger.ErrConflict
	}
	if len(old)+len(items) > maxMemPerBot {
		return ErrInvalid
	}
	next := append([]memItem(nil), old...)
	seen := map[string]bool{}
	for _, x := range old {
		seen[strings.ToLower(x.Text)] = true
	}
	for _, raw := range items {
		text := strings.TrimSpace(raw)
		key := strings.ToLower(text)
		if !ms.safeImportFact(text) || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		next = append(next, memItem{Text: text, At: time.Now().UTC().Format(time.RFC3339)})
	}
	changes := make([]memChange, 0, len(items))
	for _, x := range next[len(old):] {
		changes = append(changes, memChange{Bot: bot, Op: "add", After: x.Text, By: "owner-import"})
	}
	return ms.commitMemoryLocked(bot, next, changes)
}

func (s *Service) memoryImportRoutes() {
	validBot := func(w http.ResponseWriter, r *http.Request) bool {
		if s.L.ReadOnly() {
			fail(w, ledger.ErrConflict)
			return false
		}
		for _, b := range s.L.Snapshot().Bots {
			if b.ID == r.PathValue("id") {
				return true
			}
		}
		fail(w, ErrNotFound)
		return false
	}
	s.mux.HandleFunc("POST /app/api/agents/{id}/memory/import/preview", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text string `json:"text"`
		}
		if !validBot(w, r) {
			return
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		out, err := s.Mem.previewImport(r.PathValue("id"), in.Text)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	s.mux.HandleFunc("POST /app/api/agents/{id}/memory/import", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Revision string   `json:"revision"`
			Items    []string `json:"items"`
		}
		if !validBot(w, r) {
			return
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		if err := s.Mem.applyImport(r.PathValue("id"), in.Revision, in.Items); err != nil {
			if errors.Is(err, ledger.ErrConflict) {
				fail(w, err)
			} else {
				failMemoryWrite(w, err)
			}
			return
		}
		writeJSON(w, 200, map[string]any{"added": len(in.Items)})
	})
}
