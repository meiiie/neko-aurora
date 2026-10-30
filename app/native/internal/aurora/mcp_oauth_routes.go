package aurora

import (
	"errors"
	"io"
	"net/http"
)

func (s *Service) mcpOAuthCredential(bot string, srv mcpServer) (string, bool, error) {
	if s.oauth == nil || s.ext == nil {
		return "", false, nil
	}
	record, err := s.oauth.read(mcpOAuthRecordID(s.workspaceID, bot, srv.Name))
	if err != nil {
		return "", false, err
	}
	if record == nil || record.Phase == "disconnected" {
		return "", false, nil
	}
	b := record.Binding
	if record.Phase != "connected" || b.Workspace != s.workspaceID || b.Bot != bot || b.Server != srv || b.ConfigRevision != s.ext.bot(bot).MCPRevision || srv.Header != "" || srv.Secret != "" || !mcpDiscoveryURL(srv.URL, true) || record.ExpiresAt != 0 && record.ExpiresAt <= s.oauth.now().Unix() || !mcpOAuthBearer(record.secrets["access"]) {
		return "", false, errMCPOAuthChanged
	}
	return "Bearer " + record.secrets["access"], true, nil
}

func mcpOAuthReply(w http.ResponseWriter, status *mcpOAuthStatus, err error) {
	if err == nil {
		writeJSON(w, 200, status)
		return
	}
	code, message := 500, "mcp_oauth_storage"
	switch {
	case errors.Is(err, errMCPOAuthInvalid):
		code, message = 400, "mcp_oauth_invalid"
	case errors.Is(err, errMCPOAuthChanged):
		code, message = 409, "mcp_oauth_changed"
	case errors.Is(err, errMCPOAuthUnknown):
		code, message = 409, "mcp_oauth_outcome_unknown"
	case errors.Is(err, errMCPOAuthDisabled):
		code, message = 403, "mcp_oauth_disabled"
	}
	writeJSON(w, code, map[string]string{"error": message})
}

func (s *Service) mcpOAuthRoutes() {
	bind := func(path string, action func(http.ResponseWriter, *http.Request, []byte)) {
		s.mux.HandleFunc("POST /app/api/"+path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			origins := r.Header.Values("Origin")
			if len(origins) != 1 {
				mcpOAuthReply(w, nil, errMCPOAuthDisabled)
				return
			}
			if _, err := s.oauth.redirect(origins[0]); err != nil {
				mcpOAuthReply(w, nil, err)
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
			if err != nil || len(body) > 64<<10 {
				mcpOAuthReply(w, nil, errMCPOAuthInvalid)
				return
			}
			action(w, r, body)
		})
	}
	bind("agents/{id}/mcp/oauth/start", func(w http.ResponseWriter, r *http.Request, body []byte) {
		var in mcpOAuthStart
		if err := mcpOAuthUniqueJSON(body, &in, true); err != nil {
			mcpOAuthReply(w, nil, err)
			return
		}
		out, err := s.mcpOAuthStart(r.Context(), r.PathValue("id"), r.Header.Get("Origin"), in)
		mcpOAuthReply(w, out, err)
	})
	bind("agents/{id}/mcp/oauth/refresh", func(w http.ResponseWriter, r *http.Request, body []byte) {
		var in mcpOAuthRefresh
		if err := mcpOAuthUniqueJSON(body, &in, true); err != nil {
			mcpOAuthReply(w, nil, err)
			return
		}
		out, err := s.mcpOAuthRefresh(r.Context(), r.PathValue("id"), r.Header.Get("Origin"), in)
		mcpOAuthReply(w, out, err)
	})
	for _, operation := range []string{"status", "disconnect"} {
		bind("agents/{id}/mcp/oauth/"+operation, func(w http.ResponseWriter, r *http.Request, body []byte) {
			var in mcpOAuthIdentity
			if err := mcpOAuthUniqueJSON(body, &in, true); err != nil {
				mcpOAuthReply(w, nil, err)
				return
			}
			method := s.mcpOAuthStatus
			if operation == "disconnect" {
				method = s.mcpOAuthDisconnect
			}
			out, err := method(r.Context(), r.PathValue("id"), r.Header.Get("Origin"), in)
			mcpOAuthReply(w, out, err)
		})
	}
	bind("mcp/oauth/callback", func(w http.ResponseWriter, r *http.Request, body []byte) {
		var in mcpOAuthCallback
		if err := mcpOAuthUniqueJSON(body, &in, true); err != nil {
			mcpOAuthReply(w, nil, err)
			return
		}
		out, err := s.mcpOAuthFinish(r.Context(), r.Header.Get("Origin"), in)
		mcpOAuthReply(w, out, err)
	})
}
