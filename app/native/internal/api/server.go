package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/protocol"
	"nekobot.local/native/internal/web"
)

type Server struct {
	store       *ledger.Store
	host        string
	session     string
	connections *connections.Store
}

func (s *Server) EnableConnections(c *connections.Store) { s.connections = c }

func New(store *ledger.Store, host string) (*Server, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, err
	}
	return &Server{store: store, host: host, session: hex.EncodeToString(secret[:])}, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if r.Host != s.host {
		http.Error(w, "invalid_host", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "/app.js" || r.URL.Path == "/connections.js" || r.URL.Path == "/style.css") {
		if r.URL.Path == "/" {
			http.SetCookie(w, &http.Cookie{Name: "neko_local", Value: s.session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
		web.Serve(w, r)
		return
	}
	cookie, err := r.Cookie("neko_local")
	if err != nil || cookie.Value != s.session {
		http.Error(w, "local_session_required", http.StatusUnauthorized)
		return
	}
	if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+s.host {
		http.Error(w, "invalid_origin", http.StatusForbidden)
		return
	}
	if r.Method != "GET" && r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "json_required", http.StatusUnsupportedMediaType)
		return
	}
	switch {
	case r.Method == "POST" && r.URL.Path == "/v1/context-access":
		var input struct {
			RequestID        string  `json:"requestId"`
			ExpectedRevision *uint64 `json:"expectedRevision"`
			Granted          *bool   `json:"granted"`
			ClientNonce      string  `json:"clientNonce"`
		}
		if decode(w, r, &input) != nil || input.ExpectedRevision == nil || input.Granted == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		work, duplicate, e := s.store.SetSourceAccess(ledger.AccessInput{RequestID: input.RequestID, ExpectedRevision: *input.ExpectedRevision, Granted: *input.Granted, ClientNonce: input.ClientNonce})
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]any{"work": work, "duplicate": duplicate})
	case r.Method == "POST" && r.URL.Path == "/v1/context-refresh":
		var input ledger.RefreshInput
		if decode(w, r, &input) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		work, e := s.store.RefreshContext(input)
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, work)
	case r.Method == "POST" && r.URL.Path == "/v1/bots":
		var input ledger.BotInput
		if decode(w, r, &input) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		bot, duplicate, err := s.store.CreateBot(input)
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusCreated, map[string]any{"bot": bot, "duplicate": duplicate})
	case s.connections != nil && r.Method == "GET" && r.URL.Path == "/v1/connections":
		send(w, http.StatusOK, s.connections.Snapshot())
	case s.connections != nil && r.Method == "POST" && r.URL.Path == "/v1/accounts":
		var input connections.AccountInput
		if decode(w, r, &input) != nil {
			sendConnectionError(w, connections.ErrInvalid)
			return
		}
		account, duplicate, e := s.connections.SaveAccount(input)
		input.Secret = ""
		if e != nil {
			sendConnectionError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]any{"account": account, "duplicate": duplicate})
	case s.connections != nil && r.Method == "POST" && r.URL.Path == "/v1/accounts/remove":
		var input struct {
			ID               string `json:"id"`
			ExpectedRevision uint64 `json:"expectedRevision"`
		}
		if decode(w, r, &input) != nil {
			sendConnectionError(w, connections.ErrInvalid)
			return
		}
		if e := s.connections.RemoveAccount(input.ID, input.ExpectedRevision); e != nil {
			sendConnectionError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]bool{"removedLocally": true})
	case s.connections != nil && r.Method == "POST" && r.URL.Path == "/v1/plugins":
		var input connections.PluginInput
		if decode(w, r, &input) != nil {
			sendConnectionError(w, connections.ErrInvalid)
			return
		}
		plugin, e := s.connections.AddPlugin(input)
		if e != nil {
			sendConnectionError(w, e)
			return
		}
		send(w, http.StatusCreated, plugin)
	case r.Method == "GET" && r.URL.Path == "/v1/snapshot":
		send(w, http.StatusOK, s.store.Snapshot())
	case r.Method == "GET" && r.URL.Path == "/v1/events":
		s.stream(w, r)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/artifacts/"):
		artifact, content, e := s.store.ReadArtifact(strings.TrimPrefix(r.URL.Path, "/v1/artifacts/"))
		if e != nil {
			sendError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": artifact.Filename}))
		w.Header().Set("X-Artifact-SHA256", artifact.SHA256)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	case r.Method == "POST" && r.URL.Path == "/v1/recover":
		var input struct {
			RequestID string `json:"requestId"`
		}
		if decode(w, r, &input) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		work, e := s.store.Requeue(input.RequestID)
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, work)
	case r.Method == "POST" && r.URL.Path == "/v1/messages":
		var input ledger.Input
		if err = decode(w, r, &input); err != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		work, duplicate, err := s.store.Accept(input)
		if err != nil {
			sendError(w, err)
			return
		}
		status := http.StatusCreated
		if duplicate {
			status = http.StatusOK
		}
		send(w, status, map[string]any{"work": work, "duplicate": duplicate})
	case r.Method == "POST" && r.URL.Path == "/v1/cancel":
		var input struct {
			RequestID string `json:"requestId"`
		}
		if err = decode(w, r, &input); err != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		work, err := s.store.Cancel(input.RequestID)
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, work)
	default:
		http.NotFound(w, r)
	}
}
func sendConnectionError(w http.ResponseWriter, err error) {
	code := http.StatusServiceUnavailable
	label := "secure_connection_unavailable"
	if errors.Is(err, connections.ErrInvalid) {
		code = http.StatusBadRequest
		label = "invalid_connection"
	}
	if errors.Is(err, connections.ErrConflict) {
		code = http.StatusConflict
		label = "connection_revision_conflict"
	}
	send(w, code, map[string]string{"error": label})
}
func decode(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	frame, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	canonical, err := protocol.Canonical(frame)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(canonical)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return ledger.ErrInvalid
	}
	return nil
}
func send(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func sendError(w http.ResponseWriter, err error) {
	code := http.StatusServiceUnavailable
	label := "store_unavailable"
	switch {
	case errors.Is(err, ledger.ErrInvalid):
		code = http.StatusBadRequest
		label = "invalid_message"
	case errors.Is(err, ledger.ErrConflict):
		code = http.StatusConflict
		label = "nonce_or_cursor_conflict"
	case errors.Is(err, ledger.ErrNotFound):
		code = http.StatusNotFound
		label = "not_found"
	}
	send(w, code, map[string]string{"error": label})
}
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("after")
	if r.Header.Get("Last-Event-ID") != "" {
		cursor = r.Header.Get("Last-Event-ID")
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
	events, err := s.store.Events(after)
	if err != nil {
		sendError(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream_unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	emit := func(events []ledger.Event) bool {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
		for _, e := range events {
			b, _ := json.Marshal(e)
			if _, err := fmt.Fprintf(w, "id: %d\nevent: work\ndata: %s\n\n", e.Sequence, b); err != nil {
				return false
			}
			after = e.Sequence
		}
		flusher.Flush()
		return true
	}
	if !emit(events) {
		return
	}
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			events, err = s.store.Events(after)
			if err != nil {
				return
			}
			if len(events) > 0 && !emit(events) {
				return
			}
		case <-heartbeat.C:
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err = fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
