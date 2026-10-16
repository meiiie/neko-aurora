package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"nekobot.local/native/internal/ledger"
)

func oneConversationQuery(r *http.Request, key string) (string, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.ForceQuery || len(q) != 1 || len(q[key]) != 1 || q.Get(key) == "" {
		return "", ledger.ErrInvalid
	}
	return q.Get(key), nil
}
func (s *Server) conversations(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/conversations"), "/")
	if len(parts) == 1 && parts[0] == "" {
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			sendError(w, ledger.ErrInvalid)
			return
		}
		if r.Method == "GET" {
			send(w, 200, map[string]any{"conversations": s.store.PublicSnapshot().Conversations})
			return
		}
		if r.Method == "POST" {
			var raw struct {
				ledger.ConversationCreateInput
				ExpectedRevision *uint64 `json:"expectedRevision"`
			}
			if decode(w, r, &raw) != nil || raw.ExpectedRevision == nil {
				sendError(w, ledger.ErrInvalid)
				return
			}
			in := raw.ConversationCreateInput
			in.ExpectedRevision = *raw.ExpectedRevision
			c, dup, err := s.store.CreateConversation(in)
			if err != nil {
				sendError(w, err)
				return
			}
			status := 201
			if dup {
				status = 200
			}
			send(w, status, map[string]any{"conversation": c, "duplicate": dup})
			return
		}
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method_not_allowed", 405)
		return
	}
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[1]
	if len(parts) == 2 {
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method_not_allowed", 405)
			return
		}
		var raw struct {
			Archived         *bool   `json:"archived"`
			ExpectedRevision *uint64 `json:"expectedRevision"`
			ClientNonce      string  `json:"clientNonce"`
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery || decode(w, r, &raw) != nil || raw.Archived == nil || raw.ExpectedRevision == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		c, dup, err := s.store.SetConversationArchive(id, ledger.ConversationArchiveInput{Archived: *raw.Archived, ExpectedRevision: *raw.ExpectedRevision, ClientNonce: raw.ClientNonce})
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, 200, map[string]any{"conversation": c, "duplicate": dup})
		return
	}
	switch parts[2] {
	case "export":
		if r.Method != "GET" {
			http.Error(w, "method_not_allowed", 405)
			return
		}
		format, err := oneConversationQuery(r, "format")
		if err != nil {
			sendError(w, err)
			return
		}
		raw, err := s.store.ExportConversation(id, format)
		if err != nil {
			sendError(w, err)
			return
		}
		ext, mime := "json", "application/json; charset=utf-8"
		if format == "text" {
			ext, mime = "txt", "text/plain; charset=utf-8"
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"neko-history-%s.%s\"", id, ext))
		w.WriteHeader(200)
		w.Write(raw)
	case "retention":
		if r.Method != "POST" {
			http.Error(w, "method_not_allowed", 405)
			return
		}
		var raw struct {
			Days             *int    `json:"days"`
			ExpectedRevision *uint64 `json:"expectedRevision"`
			ClientNonce      string  `json:"clientNonce"`
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery || decode(w, r, &raw) != nil || raw.Days == nil || raw.ExpectedRevision == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		p, dup, err := s.store.SetRetentionPreference(id, ledger.RetentionInput{Days: *raw.Days, ExpectedRevision: *raw.ExpectedRevision, ClientNonce: raw.ClientNonce})
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, 200, map[string]any{"preference": p, "duplicate": dup})
	case "retention-preview":
		if r.Method != "GET" {
			http.Error(w, "method_not_allowed", 405)
			return
		}
		value, err := oneConversationQuery(r, "days")
		if err != nil {
			sendError(w, err)
			return
		}
		days, err := strconv.Atoi(value)
		if err != nil || strconv.Itoa(days) != value {
			sendError(w, ledger.ErrInvalid)
			return
		}
		p, err := s.store.PreviewRetention(id, days)
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, 200, p)
	default:
		http.NotFound(w, r)
	}
}
