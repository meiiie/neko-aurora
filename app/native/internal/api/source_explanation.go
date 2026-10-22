package api

import (
	"nekoaurora.local/native/internal/ledger"
	"net/http"
	"net/url"
	"strings"
)

func scopeQuery(r *http.Request, format bool) (channel, project, value string, err error) {
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil || r.URL.ForceQuery {
		err = ledger.ErrInvalid
		return
	}
	for k, v := range q {
		if len(v) != 1 || v[0] == "" || (k != "channelId" && k != "projectId" && !(format && k == "format")) {
			err = ledger.ErrInvalid
			return
		}
	}
	channel, project, value = q.Get("channelId"), q.Get("projectId"), q.Get("format")
	if channel == "" {
		err = ledger.ErrInvalid
	}
	return
}
func (s *Server) sourceExplanation(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 5 {
		sendError(w, ledger.ErrInvalid)
		return
	}
	channel, project, _, err := scopeQuery(r, false)
	if err != nil {
		sendError(w, err)
		return
	}
	out, err := s.store.ExplainSources(parts[3], channel, project)
	if err != nil {
		sendError(w, err)
		return
	}
	send(w, 200, out)
}
