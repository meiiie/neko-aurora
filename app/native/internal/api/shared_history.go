package api

import (
	"fmt"
	"nekobot.local/native/internal/ledger"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) sharedHistory(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	q, err := url.ParseQuery(r.URL.RawQuery)
	if len(parts) != 5 || err != nil || r.URL.ForceQuery {
		sendError(w, ledger.ErrInvalid)
		return
	}
	for k, v := range q {
		if (k != "format" && k != "projectId") || len(v) != 1 || v[0] == "" {
			sendError(w, ledger.ErrInvalid)
			return
		}
	}
	format := q.Get("format")
	if format != "json" && format != "text" {
		sendError(w, ledger.ErrInvalid)
		return
	}
	raw, err := s.store.ExportSharedHistory(parts[3], q.Get("projectId"), format)
	if err != nil {
		sendError(w, err)
		return
	}
	ext, mime := "json", "application/json; charset=utf-8"
	if format == "text" {
		ext, mime = "txt", "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"neko-public-%s.%s\"", parts[3], ext))
	w.WriteHeader(200)
	w.Write(raw)
}
