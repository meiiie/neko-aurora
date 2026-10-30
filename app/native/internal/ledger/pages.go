package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
)

const MaxPageTextBytes = 64 << 10
const MaxPages = 64

var ErrPageLimit = errors.New("page_limit_reached")

type PageRevision struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Title    string `json:"title"`
	Text     string `json:"text"`
	SHA256   string `json:"sha256"`
	Archived bool   `json:"archived"`
	SavedAt  string `json:"savedAt"`
}

type PageSummary struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Title    string `json:"title"`
	Bytes    int    `json:"bytes"`
	SHA256   string `json:"sha256"`
	Archived bool   `json:"archived"`
	SavedAt  string `json:"savedAt"`
}

type PageInput struct {
	ID           string `json:"id"`
	BaseRevision uint64 `json:"baseRevision"`
	Title        string `json:"title"`
	Text         string `json:"text"`
	Archived     bool   `json:"archived"`
	ClientNonce  string `json:"clientNonce"`
}

type PageChange struct {
	Input PageInput    `json:"input"`
	Page  PageRevision `json:"page"`
}

func pageSummary(p PageRevision) PageSummary {
	return PageSummary{p.ID, p.Revision, p.Title, len(p.Text), p.SHA256, p.Archived, p.SavedAt}
}

func validPageInput(in PageInput) bool {
	if !validAccessNonce(in.ClientNonce) || !directText(in.Title, 512) || in.Title == "" || strings.TrimSpace(in.Title) != in.Title || strings.IndexFunc(in.Title, unicode.IsControl) >= 0 || !directText(in.Text, MaxPageTextBytes) || strings.ContainsRune(in.Text, 0) || in.BaseRevision >= 1<<53-1 {
		return false
	}
	return in.ID == "" && in.BaseRevision == 0 && !in.Archived || resourceID(in.ID, "page-") && in.BaseRevision > 0
}

func (s *Store) validPageChange(change PageChange) bool {
	in, p := change.Input, change.Page
	if !validPageInput(in) || !resourceID(p.ID, "page-") || p.Revision != in.BaseRevision+1 || p.Title != in.Title || p.Text != in.Text || p.Archived != in.Archived {
		return false
	}
	if _, used := s.pageNonces[in.ClientNonce]; used {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, p.SavedAt); err != nil {
		return false
	}
	hash := sha256.Sum256([]byte(p.Text))
	if p.SHA256 != hex.EncodeToString(hash[:]) {
		return false
	}
	history := s.pages[p.ID]
	if in.ID == "" {
		return len(history) == 0 && len(s.pages) < MaxPages
	}
	return in.ID == p.ID && len(history) > 0 && history[len(history)-1].Revision == in.BaseRevision
}

func (s *Store) SavePage(in PageInput) (PageRevision, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readOnly {
		return PageRevision{}, false, ErrReadOnly
	}
	if s.closed || s.poisoned {
		return PageRevision{}, false, ErrUnavailable
	}
	if !validPageInput(in) {
		return PageRevision{}, false, ErrInvalid
	}
	if previous, ok := s.pageNonces[in.ClientNonce]; ok {
		if previous.Input != in {
			return PageRevision{}, false, ErrConflict
		}
		return previous.Page, true, nil
	}
	id := in.ID
	if id == "" {
		if len(s.pages) >= MaxPages {
			return PageRevision{}, false, ErrPageLimit
		}
		var err error
		id, err = newID("page-")
		if err != nil {
			return PageRevision{}, false, ErrUnavailable
		}
	} else {
		history := s.pages[id]
		if len(history) == 0 {
			return PageRevision{}, false, ErrNotFound
		}
		if history[len(history)-1].Revision != in.BaseRevision {
			return PageRevision{}, false, ErrConflict
		}
	}
	hash := sha256.Sum256([]byte(in.Text))
	p := PageRevision{id, in.BaseRevision + 1, in.Title, in.Text, hex.EncodeToString(hash[:]), in.Archived, time.Now().UTC().Format(time.RFC3339Nano)}
	change := PageChange{Input: in, Page: p}
	e := Event{Type: "aurora.page", Aurora: &AuroraRecord{Page: &change}}
	if !s.validAuroraEvent(e) {
		return PageRevision{}, false, ErrInvalid
	}
	if err := s.commit(e); err != nil {
		return PageRevision{}, false, err
	}
	return p, false, nil
}

func (s *Store) Pages() []PageSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []PageSummary{}
	for _, history := range s.pages {
		out = append(out, pageSummary(history[len(history)-1]))
	}
	sort.Slice(out, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339Nano, out[i].SavedAt)
		right, _ := time.Parse(time.RFC3339Nano, out[j].SavedAt)
		if left.Equal(right) {
			return out[i].ID < out[j].ID
		}
		return left.After(right)
	})
	return out
}

func (s *Store) Page(id string, revision uint64) (PageRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := s.pages[id]
	if len(history) == 0 || revision > uint64(len(history)) {
		return PageRevision{}, ErrNotFound
	}
	if revision == 0 {
		revision = uint64(len(history))
	}
	return history[revision-1], nil
}

func (s *Store) PageHistory(id string, before uint64) ([]PageSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := s.pages[id]
	if len(history) == 0 {
		return nil, ErrNotFound
	}
	if before == 0 || before > uint64(len(history))+1 {
		before = uint64(len(history)) + 1
	}
	out := []PageSummary{}
	for i := int(before) - 2; i >= 0 && len(out) < 50; i-- {
		out = append(out, pageSummary(history[i]))
	}
	return out, nil
}

func (s *Store) PageReceipt(nonce string) (PageRevision, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	change, found := s.pageNonces[nonce]
	return change.Page, found
}
