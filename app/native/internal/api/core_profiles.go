package api

import (
	"nekoaurora.local/native/internal/ledger"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) EnableCoreProfiles(service ledger.CoreProfileService) {
	s.store.ConfigureCoreProfiles(service)
}
func (s *Server) coreProfiles(w http.ResponseWriter, r *http.Request) {
	if s.fixture != nil {
		sendError(w, ledger.ErrForbidden)
		return
	}
	if r.URL.Path == "/v1/core-profiles" && r.Method == http.MethodGet {
		q, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || len(q) != 2 || len(q["sourceId"]) != 1 || len(q["inspectionConsentId"]) != 1 || r.URL.ForceQuery {
			sendError(w, ledger.ErrInvalid)
			return
		}
		out, e := s.store.CoreProfiles(q.Get("sourceId"), q.Get("inspectionConsentId"))
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, 200, out)
		return
	}
	if r.URL.Path == "/v1/core-profile-receipts" && r.Method == http.MethodGet {
		q, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || len(q) != 2 || len(q["kind"]) != 1 || len(q["clientNonce"]) != 1 || r.URL.ForceQuery {
			sendError(w, ledger.ErrInvalid)
			return
		}
		out, e := s.store.CoreProfileReceipt(q.Get("kind"), q.Get("clientNonce"))
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, 200, out)
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		sendError(w, ledger.ErrInvalid)
		return
	}
	switch {
	case r.URL.Path == "/v1/core-profile-sources" && r.Method == http.MethodGet:
		sources, e := s.store.CoreProfileSources()
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, 200, map[string]any{"version": 1, "sources": sources})
	case r.URL.Path == "/v1/core-profile-inspection-consents" && r.Method == http.MethodPost:
		var in ledger.CoreProfileInspectionInput
		if decode(w, r, &in) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		out, dup, e := s.store.InspectCoreProfileConsent(in)
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, 200, map[string]any{"inspectionConsent": out, "duplicate": dup})
	case r.URL.Path == "/v1/core-profile-consents" && r.Method == http.MethodPost:
		var in ledger.CoreProfileConsentInput
		if decode(w, r, &in) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		out, dup, e := s.store.ApproveCoreProfile(in)
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, 200, map[string]any{"consent": out, "duplicate": dup})
	case r.URL.Path == "/v1/core-profile-consents/control" && r.Method == http.MethodPost:
		var in ledger.CoreProfileConsentControlInput
		if decode(w, r, &in) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		out, dup, e := s.store.ControlCoreProfile(in)
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, 200, map[string]any{"consent": out, "duplicate": dup})
	default:
		http.NotFound(w, r)
	}
}
func coreProfileRoute(path string) bool {
	return strings.HasPrefix(path, "/v1/core-profile-") || path == "/v1/core-profiles"
}
