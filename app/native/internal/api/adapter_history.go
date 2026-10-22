package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

var adapterHistoryToken = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

func (s *Server) adapterHistory(w http.ResponseWriter, r *http.Request, id string) {
	if r.URL.RawPath != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.Body != nil && r.Body != http.NoBody {
		send(w, http.StatusBadRequest, map[string]string{"error": "adapter_history_cursor_invalid"})
		return
	}
	cursor := ""
	if r.URL.RawQuery != "" {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query) != 1 || len(query["cursor"]) != 1 {
			send(w, http.StatusBadRequest, map[string]string{"error": "adapter_history_cursor_invalid"})
			return
		}
		cursor = query.Get("cursor")
		if !adapterHistoryToken.MatchString(cursor) || r.URL.RawQuery != "cursor="+cursor {
			send(w, http.StatusBadRequest, map[string]string{"error": "adapter_history_cursor_invalid"})
			return
		}
	}
	page, err := s.store.AdapterHistoryPage(id, cursor, s.historyKey, time.Now())
	if err != nil {
		sendAdapterHistoryError(w, err)
		return
	}
	raw, err := json.Marshal(page)
	if err != nil {
		sendError(w, ledger.ErrUnavailable)
		return
	}
	if len(raw) > 900000 {
		send(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "adapter_history_budget_exceeded"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func sendAdapterHistoryError(w http.ResponseWriter, err error) {
	status := 0
	switch err.Error() {
	case "adapter_history_cursor_invalid":
		status = http.StatusBadRequest
	case "adapter_history_binding_mismatch":
		status = http.StatusForbidden
	case "adapter_history_scope_changed", "adapter_history_authority_changed":
		status = http.StatusConflict
	case "adapter_history_snapshot_expired":
		status = http.StatusGone
	case "adapter_history_budget_exceeded":
		status = http.StatusRequestEntityTooLarge
	}
	if status != 0 {
		send(w, status, map[string]string{"error": err.Error()})
		return
	}
	sendError(w, err)
}
