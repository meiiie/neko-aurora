package aurora

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"unicode/utf8"

	"nekoaurora.local/native/internal/ledger"
)

func (s *Service) pageQuery(w http.ResponseWriter, r *http.Request, number string) (uint64, bool) {
	w.Header().Set("Cache-Control", "no-store")
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		fail(w, ErrInvalid)
		return 0, false
	}
	for key, values := range q {
		if len(values) != 1 || key != "workspaceId" && (number == "" || key != number) {
			fail(w, ErrInvalid)
			return 0, false
		}
	}
	if q.Get("workspaceId") == "" || q.Get("workspaceId") != s.workspaceID {
		fail(w, ErrConflict)
		return 0, false
	}
	var value uint64
	if number != "" && q.Has(number) {
		value, err = strconv.ParseUint(q.Get(number), 10, 53)
		if err != nil || strconv.FormatUint(value, 10) != q.Get(number) || value == 0 {
			fail(w, ErrInvalid)
			return 0, false
		}
	}
	w.Header().Set("X-Aurora-Workspace", s.workspaceID)
	return value, true
}

func (s *Service) pageRoutes() {
	s.mux.HandleFunc("POST /app/api/pages/save", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var in struct {
			WorkspaceID string `json:"workspaceId"`
			ledger.PageInput
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
		if err != nil || !utf8.Valid(raw) || r.URL.RawQuery != "" {
			fail(w, ErrInvalid)
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var extra any
		if decoder.Decode(&in) != nil || decoder.Decode(&extra) != io.EOF {
			fail(w, ErrInvalid)
			return
		}
		if in.WorkspaceID == "" || in.WorkspaceID != s.workspaceID {
			fail(w, ErrConflict)
			return
		}
		p, duplicate, err := s.L.SavePage(in.PageInput)
		if err != nil {
			if errors.Is(err, ledger.ErrPageLimit) {
				writeJSON(w, 507, map[string]string{"error": "page_limit_reached"})
			} else {
				fail(w, err)
			}
			return
		}
		writeJSON(w, 200, map[string]any{"workspaceId": s.workspaceID, "page": p, "duplicate": duplicate})
	})
	s.mux.HandleFunc("GET /app/api/pages", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.pageQuery(w, r, ""); !ok {
			return
		}
		writeJSON(w, 200, map[string]any{"workspaceId": s.workspaceID, "pages": s.L.Pages(), "maxPages": ledger.MaxPages, "maxTextBytes": ledger.MaxPageTextBytes})
	})
	s.mux.HandleFunc("GET /app/api/page-receipts/{nonce}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.pageQuery(w, r, ""); !ok {
			return
		}
		p, found := s.L.PageReceipt(r.PathValue("nonce"))
		writeJSON(w, 200, map[string]any{"workspaceId": s.workspaceID, "found": found, "page": p})
	})
	s.mux.HandleFunc("GET /app/api/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		revision, ok := s.pageQuery(w, r, "revision")
		if !ok {
			return
		}
		p, err := s.L.Page(r.PathValue("id"), revision)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"workspaceId": s.workspaceID, "page": p})
	})
	s.mux.HandleFunc("GET /app/api/pages/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		before, ok := s.pageQuery(w, r, "before")
		if !ok {
			return
		}
		history, err := s.L.PageHistory(r.PathValue("id"), before)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"workspaceId": s.workspaceID, "versions": history})
	})
	s.mux.HandleFunc("GET /app/api/pages/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		revision, ok := s.pageQuery(w, r, "revision")
		if !ok {
			return
		}
		if revision == 0 {
			fail(w, ErrInvalid)
			return
		}
		p, err := s.L.Page(r.PathValue("id"), revision)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": p.ID + "-v" + strconv.FormatUint(p.Revision, 10) + ".md"}))
		w.Header().Set("Content-Length", strconv.Itoa(len(p.Text)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("X-Aurora-Page", p.ID)
		w.Header().Set("X-Aurora-Revision", strconv.FormatUint(p.Revision, 10))
		w.Header().Set("X-Aurora-SHA256", p.SHA256)
		_, _ = io.WriteString(w, p.Text)
	})
}
