package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"nekobot.local/native/internal/identity"
	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/syncstream"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Server) EnableFixtureIdentity(a *identity.Authority) {
	s.fixture = a
	s.store.ConfigureIdentity(a)
}
func (s *Server) fixtureBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+s.host {
		http.Error(w, "invalid_origin", 403)
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.Header.Get("Content-Type") != "application/json" {
		sendError(w, ledger.ErrInvalid)
		return
	}
	var in struct {
		Ticket string `json:"ticket"`
	}
	if decode(w, r, &in) != nil || len(in.Ticket) != 64 {
		sendError(w, ledger.ErrInvalid)
		return
	}
	token, p, err := s.fixture.Exchange(in.Ticket)
	if err != nil {
		fixtureError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "neko_fixture", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int((p.ExpiresAtUnixMs - time.Now().UnixMilli()) / 1000)})
	w.Header().Set("X-Neko-Session-Binding", identity.Binding(p))
	send(w, 200, identity.Status(p))
}
func fixtureError(w http.ResponseWriter, err error) {
	if errors.Is(err, identity.ErrUnauthenticated) {
		http.Error(w, "fixture_session_invalid", 401)
		return
	}
	if errors.Is(err, identity.ErrConflict) {
		send(w, 409, map[string]string{"error": err.Error()})
		return
	}
	if errors.Is(err, ledger.ErrForbidden) {
		send(w, 403, map[string]string{"error": err.Error()})
		return
	}
	if errors.Is(err, identity.ErrInvalid) {
		send(w, 400, map[string]string{"error": err.Error()})
		return
	}
	sendError(w, err)
}
func (s *Server) fixtureRequest(w http.ResponseWriter, r *http.Request, p identity.VerifiedPrincipal) bool {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/report-deliveries":
		var in ledger.ReportDeliveryInput
		if decode(w, r, &in) != nil {
			sendError(w, ledger.ErrInvalid)
			return true
		}
		out, duplicate, err := s.store.DeliverReportFor(p, in)
		if err != nil {
			fixtureError(w, err)
			return true
		}
		status := 201
		if duplicate {
			status = 200
		}
		send(w, status, map[string]any{"delivery": out, "duplicate": duplicate})
		return true
	case r.Method == http.MethodGet && r.URL.Path == "/v1/snapshot":
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			sendError(w, ledger.ErrInvalid)
			return true
		}
		err := s.store.WriteScopedSnapshot(p, func(out ledger.Snapshot) error { send(w, 200, out); return nil })
		if err != nil {
			fixtureError(w, err)
		}
		return true
	case r.Method == http.MethodGet && r.URL.Path == "/v1/events":
		s.fixtureStream(w, r, p)
		return true
	case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
		var in struct {
			ledger.Input
			ledger.SubmissionScope
		}
		if decode(w, r, &in) != nil {
			sendError(w, ledger.ErrInvalid)
			return true
		}
		work, duplicate, err := s.store.AcceptFor(p, in.Input, in.SubmissionScope)
		if err != nil {
			fixtureError(w, err)
			return true
		}
		view, err := s.store.ScopedPublicWork(p, work.RequestID)
		if err != nil {
			fixtureError(w, err)
			return true
		}
		status := 201
		if duplicate {
			status = 200
		}
		send(w, status, map[string]any{"work": view, "duplicate": duplicate})
		return true
	case r.Method == http.MethodPost && r.URL.Path == "/v1/channels":
		var in ledger.ChannelInput
		if decode(w, r, &in) != nil {
			sendError(w, ledger.ErrInvalid)
			return true
		}
		channel, duplicate, err := s.store.SaveChannelFor(p, in)
		if err != nil {
			fixtureError(w, err)
			return true
		}
		send(w, 200, map[string]any{"channel": channel, "duplicate": duplicate})
		return true
	case r.Method == http.MethodPost && r.URL.Path == "/v1/fixture-devices/control":
		var in struct {
			DeviceID         string  `json:"deviceId"`
			Action           string  `json:"action"`
			ExpectedRevision *uint64 `json:"expectedRevision"`
		}
		if decode(w, r, &in) != nil || in.ExpectedRevision == nil {
			sendError(w, ledger.ErrInvalid)
			return true
		}
		out, err := s.fixture.ControlDeviceFor(p, in.DeviceID, in.Action, *in.ExpectedRevision)
		if err != nil {
			fixtureError(w, err)
			return true
		}
		if err = s.store.ReconcilePrincipals(); err != nil {
			fixtureError(w, err)
			return true
		}
		send(w, 200, out)
		return true
	case r.Method == http.MethodGet && r.URL.Path == "/v1/local-sync":
		q, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || r.URL.ForceQuery {
			sendError(w, ledger.ErrInvalid)
			return true
		}
		for k, v := range q {
			if k != "botId" && k != "channelId" && k != "projectId" && k != "cursor" || len(v) != 1 {
				sendError(w, ledger.ErrInvalid)
				return true
			}
		}
		if q.Get("botId") == "" || p.ActorID != "local-owner" && q.Get("channelId") == "" {
			sendError(w, ledger.ErrInvalid)
			return true
		}
		out, err := s.store.ScopedLocalSync(p, ledger.LocalSyncScope{BotID: q.Get("botId"), ChannelID: q.Get("channelId"), ProjectID: q.Get("projectId")}, q.Get("cursor"))
		if err != nil {
			if errors.Is(err, syncstream.ErrScope) || errors.Is(err, syncstream.ErrInvalid) {
				send(w, 409, map[string]string{"error": err.Error()})
			} else {
				fixtureError(w, err)
			}
			return true
		}
		raw, err := syncstream.Encode(out)
		if err != nil {
			send(w, 413, map[string]string{"error": "sync_limit"})
			return true
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Neko-Sync-Mode", "local-two-client-simulation")
		w.Write(raw)
		return true
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/channels/"):
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) == 5 && parts[4] == "export" {
			q, e := url.ParseQuery(r.URL.RawQuery)
			if e != nil || r.URL.ForceQuery {
				sendError(w, ledger.ErrInvalid)
				return true
			}
			for k, v := range q {
				if k != "format" && k != "projectId" || len(v) != 1 {
					sendError(w, ledger.ErrInvalid)
					return true
				}
			}
			raw, e := s.store.ScopedExportHistory(p, parts[3], q.Get("projectId"), q.Get("format"))
			if e != nil {
				fixtureError(w, e)
				return true
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if q.Get("format") == "text" {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			}
			w.Write(raw)
			return true
		}
		if len(parts) == 6 && parts[4] == "artifacts" && r.URL.RawQuery == "" && !r.URL.ForceQuery {
			a, raw, e := s.store.ScopedReadArtifact(p, parts[3], parts[5])
			if e != nil {
				fixtureError(w, e)
				return true
			}
			writeFixtureArtifact(w, a, raw)
			return true
		}
		sendError(w, ledger.ErrNotFound)
		return true
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/report-deliveries/"):
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 6 || parts[4] != "artifacts" || r.URL.RawQuery != "" || r.URL.ForceQuery {
			sendError(w, ledger.ErrInvalid)
			return true
		}
		a, raw, e := s.store.ScopedReadReportArtifact(p, parts[3], parts[5])
		if e != nil {
			fixtureError(w, e)
			return true
		}
		writeFixtureArtifact(w, a, raw)
		return true
	}
	if p.ActorID != "local-owner" {
		fixtureError(w, ledger.ErrForbidden)
		return true
	}
	if r.Method != http.MethodGet {
		safe := r.URL.Path == "/v1/bots" || strings.HasPrefix(r.URL.Path, "/v1/bots/") && (strings.HasSuffix(r.URL.Path, "/profile") || strings.HasSuffix(r.URL.Path, "/name") || strings.HasSuffix(r.URL.Path, "/state"))
		if !safe {
			fixtureError(w, ledger.ErrForbidden)
			return true
		}
	}
	return false
}
func writeFixtureArtifact(w http.ResponseWriter, a ledger.Artifact, raw []byte) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename}))
	w.Header().Set("X-Artifact-SHA256", a.SHA256)
	w.Write(raw)
}
func (s *Server) fixtureStream(w http.ResponseWriter, r *http.Request, p identity.VerifiedPrincipal) {
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e == nil && len(q["sessionBinding"]) == 0 {
		sendError(w, ledger.ErrConflict)
		return
	}
	if e != nil || r.URL.ForceQuery || len(q) > 2 || len(q["after"]) > 1 || len(q["sessionBinding"]) != 1 {
		sendError(w, ledger.ErrInvalid)
		return
	}
	for k := range q {
		if k != "after" && k != "sessionBinding" {
			sendError(w, ledger.ErrInvalid)
			return
		}
	}
	if q.Get("sessionBinding") != identity.Binding(p) {
		sendError(w, ledger.ErrConflict)
		return
	}
	cursor := q.Get("after")
	if r.Header.Get("Last-Event-ID") != "" {
		parts := strings.Split(r.Header.Get("Last-Event-ID"), ":")
		if len(parts) != 2 || parts[0] != identity.Binding(p) || parts[1] == "" {
			sendError(w, ledger.ErrConflict)
			return
		}
		cursor = parts[1]
	}
	var after uint64
	var err error
	if cursor != "" {
		after, err = strconv.ParseUint(cursor, 10, 64)
		if err != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
	}
	events, err := s.store.ScopedEvents(p, after)
	if err != nil {
		fixtureError(w, err)
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream_unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	emit := func(events []ledger.ScopedEvent) bool {
		for _, entry := range events {
			err := s.store.WriteScopedEvent(p, entry.Sequence, func(event ledger.ScopedEvent) error {
				_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
				raw, _ := json.Marshal(event)
				_, err := fmt.Fprintf(w, "id: %s:%d\nevent: work\ndata: %s\n\n", identity.Binding(p), event.Sequence, raw)
				return err
			})
			if err != nil {
				return false
			}
			after = entry.Sequence
		}
		f.Flush()
		return true
	}
	if !emit(events) {
		return
	}
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			events, err = s.store.ScopedEvents(p, after)
			if err != nil {
				return
			}
			if len(events) > 0 && !emit(events) {
				return
			}
		}
	}
}
