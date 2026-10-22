package api

import (
	"mime"
	"net/http"
	"net/url"
	"strings"

	"nekoaurora.local/native/internal/ledger"
)

func (s *Server) reportDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/report-delivery-receipts" && r.Method == http.MethodGet {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || r.URL.ForceQuery || len(query) != 1 || len(query["clientNonce"]) != 1 || !s.store.PublicInputAllowed([]byte(query.Get("clientNonce"))) {
			sendError(w, ledger.ErrInvalid)
			return
		}
		receipt, err := s.store.ReportDeliveryReceipt(query.Get("clientNonce"))
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, receipt)
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		sendError(w, ledger.ErrInvalid)
		return
	}
	if r.URL.Path == "/v1/report-deliveries" && r.Method == http.MethodPost {
		var in ledger.ReportDeliveryInput
		if err := decode(w, r, &in); err != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		if !s.store.PublicInputAllowed([]byte(in.Text + in.ClientNonce)) {
			sendError(w, ledger.ErrInvalid)
			return
		}
		delivery, duplicate, err := s.store.DeliverReport(in)
		if err != nil {
			sendError(w, err)
			return
		}
		status := http.StatusCreated
		if duplicate {
			status = http.StatusOK
		}
		send(w, status, map[string]any{"delivery": delivery, "duplicate": duplicate})
		return
	}
	parts := strings.Split(r.URL.Path, "/")
	if r.Method == http.MethodGet && len(parts) == 6 && parts[1] == "v1" && parts[2] == "report-deliveries" && parts[4] == "artifacts" {
		artifact, content, err := s.store.ReadReportDeliveryArtifact(parts[3], parts[5])
		if err != nil {
			sendError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": artifact.Filename}))
		w.Header().Set("X-Artifact-SHA256", artifact.SHA256)
		w.Write(content)
		return
	}
	sendError(w, ledger.ErrNotFound)
}
