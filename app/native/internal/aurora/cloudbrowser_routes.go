package aurora

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"nekoaurora.local/native/internal/cloudbrowser"
	"nekoaurora.local/native/internal/identity"
	"nekoaurora.local/native/internal/ledger"
)

func cloudFail(w http.ResponseWriter, err error) {
	code := http.StatusBadGateway
	switch {
	case errors.Is(err, errCloudIdentity):
		code = 401
	case errors.Is(err, ledger.ErrForbidden):
		code = 403
	case errors.Is(err, ErrInvalid), errors.Is(err, ledger.ErrInvalid), errors.Is(err, cloudbrowser.ErrInvalid):
		code = 400
	case errors.Is(err, ErrNotFound):
		code = 404
	case errors.Is(err, cloudbrowser.ErrConflict):
		code = 409
	case errors.Is(err, ledger.ErrReadOnly):
		code = 423
	case errors.Is(err, cloudbrowser.ErrNotConfigured):
		code = 503
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func cloudDecode(r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16<<10))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}

func (s *Service) cloudRoutes() {
	auth := func(fn func(http.ResponseWriter, *http.Request, identity.RequestIdentity)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			a, err := s.cloudOwner(r)
			if err != nil {
				cloudFail(w, err)
				return
			}
			if r.URL.RawQuery != "" {
				cloudFail(w, ErrInvalid)
				return
			}
			bot := r.PathValue("id")
			if _, ok := s.view().bot(bot); !ok || s.view().archived[bot] {
				cloudFail(w, ErrNotFound)
				return
			}
			if r.Method != "GET" {
				typ, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if e != nil || typ != "application/json" {
					cloudFail(w, ErrInvalid)
					return
				}
			}
			fn(w, r, a)
		}
	}
	base := "/app/api/agents/{id}/cloud"
	s.mux.HandleFunc("GET "+base, auth(func(w http.ResponseWriter, r *http.Request, a identity.RequestIdentity) {
		if s.Cloud == nil {
			writeJSON(w, 200, map[string]any{"configured": false, "status": "unavailable", "error": "cloud_browser_not_configured"})
			return
		}
		out, err := s.Cloud.Do(r.Context(), cloudbrowser.Command{Profile: r.PathValue("id"), Kind: "status", Actor: "owner", ActorID: a.ActorID})
		if err != nil {
			cloudFail(w, err)
			return
		}
		writeJSON(w, 200, struct {
			cloudbrowser.Response
			Configured  bool `json:"configured"`
			LocalDocker bool `json:"localDocker"`
		}{out, true, s.Cloud.LocalDocker()})
	}))
	for _, kind := range []string{"open", "observe", "frame", "control", "action", "close", "vault"} {
		s.mux.HandleFunc("POST "+base+"/"+kind, auth(func(w http.ResponseWriter, r *http.Request, a identity.RequestIdentity) {
			cmd := cloudbrowser.Command{Profile: r.PathValue("id"), Kind: kind, Actor: "owner", ActorID: a.ActorID}
			var err error
			switch kind {
			case "observe", "frame":
				var in struct{}
				err = cloudDecode(r, &in)
			case "open":
				var in struct {
					Nonce string `json:"clientNonce"`
				}
				err = cloudDecode(r, &in)
				cmd.Nonce = in.Nonce
			case "close":
				var in struct {
					Epoch uint64 `json:"epoch"`
					Nonce string `json:"clientNonce"`
				}
				err = cloudDecode(r, &in)
				cmd.Epoch, cmd.Nonce = in.Epoch, in.Nonce
				if cmd.Epoch == 0 {
					err = ErrInvalid
				}
			case "control":
				var in struct {
					Mode  string `json:"mode"`
					Epoch uint64 `json:"epoch"`
					Nonce string `json:"clientNonce"`
				}
				err = cloudDecode(r, &in)
				cmd.Mode, cmd.Epoch, cmd.Nonce = in.Mode, in.Epoch, in.Nonce
				if cmd.Epoch == 0 || (cmd.Mode != "paused" && cmd.Mode != "owner" && cmd.Mode != "agent") {
					err = ErrInvalid
				}
			case "action":
				var in struct {
					Kind          string  `json:"kind"`
					Epoch         uint64  `json:"epoch"`
					ObservationID string  `json:"observationId"`
					FrameID       string  `json:"frameId"`
					TargetID      string  `json:"targetId"`
					Text          string  `json:"text"`
					Key           string  `json:"key"`
					X             float64 `json:"x"`
					Y             float64 `json:"y"`
					DeltaY        float64 `json:"deltaY"`
					URL           string  `json:"url"`
					Nonce         string  `json:"clientNonce"`
				}
				err = cloudDecode(r, &in)
				cmd.Kind, cmd.Epoch, cmd.ObservationID, cmd.FrameID, cmd.TargetID, cmd.Text, cmd.Key, cmd.X, cmd.Y, cmd.DeltaY, cmd.URL, cmd.Nonce = in.Kind, in.Epoch, in.ObservationID, in.FrameID, in.TargetID, in.Text, in.Key, in.X, in.Y, in.DeltaY, in.URL, in.Nonce
				if !cloudActionKind(cmd.Kind) || cmd.Epoch == 0 {
					err = ErrInvalid
				}
			case "vault":
				var in struct {
					ItemID         string `json:"itemId"`
					Epoch          uint64 `json:"epoch"`
					ObservationID  string `json:"observationId"`
					ExpectedOrigin string `json:"expectedOrigin"`
					Nonce          string `json:"clientNonce"`
					Confirmed      bool   `json:"confirmed"`
				}
				err = cloudDecode(r, &in)
				cmd.Kind, cmd.Epoch, cmd.ObservationID, cmd.ExpectedOrigin, cmd.Nonce = "vault_fill", in.Epoch, in.ObservationID, in.ExpectedOrigin, in.Nonce
				if !in.Confirmed || cmd.Epoch == 0 || cmd.ObservationID == "" || !cloudbrowser.ValidNonce(cmd.Nonce) {
					err = ErrInvalid
				}
				if err == nil && s.Cloud == nil {
					err = cloudbrowser.ErrNotConfigured
				}
				if err == nil {
					_, _, _, err = s.cloudItem(in.ItemID, in.ExpectedOrigin)
				}
				if err == nil {
					var current cloudbrowser.Response
					current, err = s.Cloud.Do(r.Context(), cloudbrowser.Command{Profile: cmd.Profile, Kind: "status", Actor: "owner", ActorID: a.ActorID})
					if err == nil && (current.Epoch != cmd.Epoch || current.ObservationID != cmd.ObservationID || current.Origin != cmd.ExpectedOrigin) {
						err = cloudbrowser.ErrConflict
					}
					if err == nil {
						cmd, err = s.cloudVaultCommand(cmd, in.ItemID, "")
					}
				}
			}
			if err != nil {
				cloudFail(w, err)
				return
			}
			ctx, cancel := cloudTimeout(r.Context())
			defer cancel()
			out, err := s.Cloud.Do(ctx, cmd)
			if err != nil {
				cloudFail(w, err)
				return
			}
			writeJSON(w, 200, out)
		}))
	}
}
