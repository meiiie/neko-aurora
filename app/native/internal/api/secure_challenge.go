package api

import (
	"nekoaurora.local/native/internal/ledger"
	"net/http"
	"strings"
)

func (s *Server) secureChallenge(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		sendError(w, ledger.ErrInvalid)
		return
	}
	if r.URL.Path == "/v1/secure-challenges" {
		var in ledger.ChallengeInput
		if decode(w, r, &in) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		out, dup, err := s.store.CreateSecureChallenge(in)
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, 200, map[string]any{"challenge": out.Challenge, "simulationCode": out.Code, "duplicate": dup})
		return
	}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 5 || parts[4] != "resolve" {
		http.NotFound(w, r)
		return
	}
	var raw struct {
		Action         string `json:"action"`
		Fingerprint    string `json:"fingerprint"`
		ClientNonce    string `json:"clientNonce"`
		SimulationCode string `json:"simulationCode"`
	}
	if decode(w, r, &raw) != nil {
		sendError(w, ledger.ErrInvalid)
		return
	}
	out, dup, err := s.store.ResolveSecureChallenge(ledger.ChallengeInput{Action: raw.Action, ChallengeID: parts[3], Fingerprint: raw.Fingerprint, ClientNonce: raw.ClientNonce}, raw.SimulationCode)
	raw.SimulationCode = ""
	if err != nil {
		sendError(w, err)
		return
	}
	send(w, 200, map[string]any{"challenge": out, "duplicate": dup})
}
