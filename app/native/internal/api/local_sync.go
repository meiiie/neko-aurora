package api

import (
	"errors"
	"net/http"

	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/syncstream"
)

func (s *Server) localSync(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	for key, values := range query {
		if key != "botId" && key != "channelId" && key != "projectId" && key != "cursor" || len(values) != 1 {
			sendError(w, ledger.ErrInvalid)
			return
		}
	}
	if query.Get("botId") == "" {
		sendError(w, ledger.ErrInvalid)
		return
	}
	envelope, err := s.store.LocalSync(ledger.LocalSyncScope{BotID: query.Get("botId"), ChannelID: query.Get("channelId"), ProjectID: query.Get("projectId")}, query.Get("cursor"))
	if err != nil {
		code := http.StatusConflict
		switch {
		case errors.Is(err, syncstream.ErrInvalid):
			code = http.StatusBadRequest
		case errors.Is(err, syncstream.ErrLimit):
			code = http.StatusRequestEntityTooLarge
		case errors.Is(err, ledger.ErrNotFound):
			code = http.StatusNotFound
		case errors.Is(err, ledger.ErrUnavailable):
			code = http.StatusServiceUnavailable
		}
		send(w, code, map[string]string{"error": err.Error()})
		return
	}
	raw, err := syncstream.Encode(envelope)
	if err != nil {
		send(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "sync_limit"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Neko-Sync-Mode", "local-two-client-simulation")
	w.WriteHeader(http.StatusOK)
	w.Write(raw)
}
