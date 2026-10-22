package api

import (
	"nekoaurora.local/native/internal/identity"
	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/runtime"
	"net/http"
)

func (s *Server) coreDiscovery(w http.ResponseWriter, r *http.Request, p identity.VerifiedPrincipal) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		sendError(w, ledger.ErrInvalid)
		return
	}
	if s.fixture != nil && p.ActorID != "local-owner" {
		fixtureError(w, ledger.ErrForbidden)
		return
	}
	result := runtime.DiscoverCore(r.Context())
	if s.fixture != nil && !s.store.SessionCurrent(p) {
		fixtureError(w, identity.ErrUnauthenticated)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	send(w, http.StatusOK, result)
}
