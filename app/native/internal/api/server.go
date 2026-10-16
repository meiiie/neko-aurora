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
	"net/url"
	"strconv"
	"strings"
	"time"

	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/identity"
	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/protocol"
	"nekobot.local/native/internal/web"
)

type Server struct {
	store       *ledger.Store
	host        string
	session     string
	historyKey  [32]byte
	connections *connections.Store
	fixture     *identity.Authority
	desktop     *desktopLaunchReceipt
}

func (s *Server) EnableConnections(c *connections.Store) {
	s.connections = c
	s.store.EnableAccounts(c)
}

func New(store *ledger.Store, host string) (*Server, error) {
	var secret [64]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, err
	}
	var historyKey [32]byte
	copy(historyKey[:], secret[32:])
	return &Server{store: store, host: host, session: hex.EncodeToString(secret[:32]), historyKey: historyKey}, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if r.Host != s.host {
		http.Error(w, "invalid_host", http.StatusForbidden)
		return
	}
	if r.URL.Path == "/v1/local-instance" {
		s.desktopInstance(w, r)
		return
	}
	if r.Method == http.MethodGet && web.IsPublicAsset(r.URL.Path) {
		if r.URL.Path == "/" && s.fixture == nil {
			http.SetCookie(w, &http.Cookie{Name: "neko_local", Value: s.session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
		web.Serve(w, r)
		return
	}
	var err error
	var principal identity.VerifiedPrincipal
	if s.fixture != nil {
		if r.URL.Path == "/v1/fixture-sessions" && r.Method == http.MethodPost {
			s.fixtureBootstrap(w, r)
			return
		}
		cookie, e := r.Cookie("neko_fixture")
		if e != nil {
			http.Error(w, "fixture_session_required", http.StatusUnauthorized)
			return
		}
		principal, err = s.fixture.Resolve(cookie.Value)
		if err != nil {
			http.Error(w, "fixture_session_invalid", http.StatusUnauthorized)
			return
		}
		binding := identity.Binding(principal)
		w.Header().Set("X-Neko-Session-Binding", binding)
		expected := r.Header.Get("X-Neko-Session-Binding")
		if r.URL.Path != "/v1/session" && (r.Method != http.MethodGet && expected != binding || r.Method == http.MethodGet && expected != "" && expected != binding) {
			sendError(w, ledger.ErrConflict)
			return
		}
	} else {
		cookie, e := r.Cookie("neko_local")
		if e != nil || cookie.Value != s.session {
			http.Error(w, "local_session_required", http.StatusUnauthorized)
			return
		}
	}
	if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+s.host {
		http.Error(w, "invalid_origin", http.StatusForbidden)
		return
	}
	if s.store.ReadOnly() && r.Method != http.MethodGet {
		http.Error(w, "restored_state_read_only", http.StatusForbidden)
		return
	}
	if r.Method != "GET" && r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "json_required", http.StatusUnsupportedMediaType)
		return
	}
	if r.URL.Path == "/v1/session" && r.Method == http.MethodGet {
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			sendError(w, ledger.ErrInvalid)
			return
		}
		if s.fixture != nil {
			send(w, 200, identity.Status(principal))
		} else {
			send(w, 200, identity.SessionStatus{Version: 1, Mode: "local_owner", Authenticated: true, ActorID: "local-owner", SpaceID: "local-space", CacheNamespace: "local-owner", SessionBinding: "local-owner"})
		}
		return
	}
	if s.fixture != nil && s.fixtureRequest(w, r, principal) {
		return
	}
	switch {
	case coreProfileRoute(r.URL.Path):
		s.coreProfiles(w, r)
	case r.URL.Path == "/v1/core-discovery" && r.Method == http.MethodGet:
		s.coreDiscovery(w, r, principal)
	case r.URL.Path == "/v1/report-delivery-receipts" || r.URL.Path == "/v1/report-deliveries" || strings.HasPrefix(r.URL.Path, "/v1/report-deliveries/"):
		s.reportDeliveries(w, r)
	case s.connections != nil && r.Method == "GET" && r.URL.Path == "/v1/account-receipts":
		query, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || len(query) != 1 || len(query["clientNonce"]) != 1 || r.URL.ForceQuery || !s.store.PublicInputAllowed([]byte(query.Get("clientNonce"))) {
			sendConnectionError(w, connections.ErrInvalid)
			return
		}
		receipt, e := s.connections.AccountReceipt(query["clientNonce"][0])
		if e != nil {
			sendConnectionError(w, e)
			return
		}
		send(w, 200, receipt)
	case r.Method == "POST" && (r.URL.Path == "/v1/secure-challenges" || strings.HasPrefix(r.URL.Path, "/v1/secure-challenges/")):
		s.secureChallenge(w, r)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/channels/") && strings.HasSuffix(r.URL.Path, "/export"):
		s.sharedHistory(w, r)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/tasks/") && strings.HasSuffix(r.URL.Path, "/sources"):
		s.sourceExplanation(w, r)
	case r.URL.Path == "/v1/conversations" || strings.HasPrefix(r.URL.Path, "/v1/conversations/"):
		s.conversations(w, r)
	case r.Method == "GET" && r.URL.Path == "/v1/local-sync":
		s.localSync(w, r)
	case r.Method == "GET" && r.URL.Path == "/v1/adapters/capabilities":
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			sendError(w, ledger.ErrInvalid)
			return
		}
		capabilities, err := s.store.AdapterCapabilities()
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, capabilities)
	case r.Method == "GET" && r.URL.Path == "/v1/adapters/installation-catalog":
		if r.URL.RawQuery != "" {
			sendError(w, ledger.ErrInvalid)
			return
		}
		catalog, err := s.store.AdapterInstallationCatalog()
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, catalog)
	case r.Method == "GET" && r.URL.Path == "/v1/adapters/catalog":
		if r.URL.RawQuery != "" {
			sendError(w, ledger.ErrInvalid)
			return
		}
		catalog, err := s.store.AdapterCatalog()
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, catalog)
	case r.Method == "GET" && r.URL.Path == "/v1/command-receipts":
		query, err := strictReceiptQuery(r)
		if err != nil {
			sendError(w, err)
			return
		}
		receipt, err := s.store.CommandReceipt(query["kind"], query["clientNonce"], query["collaborationId"])
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, receipt)
	case r.Method == "GET" && r.URL.Path == "/v1/upload-receipts":
		query, err := strictUploadReceiptQuery(r)
		if err != nil {
			sendError(w, err)
			return
		}
		receipt, err := s.store.UploadReceipt(query["projectId"], query["channelId"], query["clientNonce"])
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, receipt)
	case r.Method == "GET" && r.URL.Path == "/v1/plugin-catalog":
		snapshot := s.store.Snapshot()
		send(w, http.StatusOK, map[string]any{"plugins": snapshot.Plugins, "sequence": snapshot.Sequence, "execution": "trusted built-ins only; external manifests disabled"})
	case r.Method == "POST" && r.URL.Path == "/v1/plugin-lifecycle":
		var in struct {
			Version          int                    `json:"version"`
			ID               string                 `json:"id"`
			Action           string                 `json:"action"`
			ExpectedRevision *uint64                `json:"expectedRevision"`
			ClientNonce      string                 `json:"clientNonce"`
			Manifest         *ledger.PluginManifest `json:"manifest,omitempty"`
		}
		if decode(w, r, &in) != nil || in.ExpectedRevision == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		plugin, duplicate, e := s.store.ControlPlugin(ledger.PluginInput{Version: in.Version, ID: in.ID, Action: in.Action, ExpectedRevision: *in.ExpectedRevision, ClientNonce: in.ClientNonce, Manifest: in.Manifest})
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]any{"plugin": plugin, "duplicate": duplicate})
	case (r.Method == "POST" || r.Method == "GET") && strings.HasPrefix(r.URL.Path, "/v1/projects/") && strings.HasSuffix(r.URL.Path, "/continuity"):
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 5 || parts[4] != "continuity" {
			http.NotFound(w, r)
			return
		}
		if r.Method == "GET" {
			view, e := s.store.GetProjectContinuity(parts[3], r.URL.Query().Get("channelId"))
			if e != nil {
				sendError(w, e)
				return
			}
			send(w, http.StatusOK, view)
			return
		}
		var input struct {
			ledger.ProjectContinuityInput
			ExpectedProjectRevision *uint64 `json:"expectedProjectRevision"`
			ExpectedChannelRevision *uint64 `json:"expectedChannelRevision"`
			ExpectedRevision        *uint64 `json:"expectedRevision"`
		}
		if decode(w, r, &input) != nil || input.ExpectedProjectRevision == nil || input.ExpectedChannelRevision == nil || input.ExpectedRevision == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		in := input.ProjectContinuityInput
		in.ProjectID = parts[3]
		in.ExpectedProjectRevision = *input.ExpectedProjectRevision
		in.ExpectedChannelRevision = *input.ExpectedChannelRevision
		in.ExpectedRevision = *input.ExpectedRevision
		c, duplicate, e := s.store.SaveProjectContinuity(in)
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]any{"continuity": c, "duplicate": duplicate})
	case r.Method == "POST" && r.URL.Path == "/v1/collaborations":
		var in ledger.CollaborationInput
		if decode(w, r, &in) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		g, dup, e := s.store.CreateCollaboration(in)
		if e != nil {
			sendError(w, e)
			return
		}
		status := http.StatusCreated
		if dup {
			status = http.StatusOK
		}
		send(w, status, map[string]any{"collaboration": s.store.PublicCollaboration(g), "duplicate": dup})
	case r.Method == "GET" && r.URL.Path == "/v1/collaborations":
		send(w, http.StatusOK, map[string]any{"collaborations": s.store.PublicSnapshot().Collaborations})
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/collaborations/") && strings.HasSuffix(r.URL.Path, "/cancel"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/collaborations/"), "/cancel")
		var in ledger.CollaborationCancelInput
		if decode(w, r, &in) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		g, dup, e := s.store.CancelCollaboration(id, in)
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]any{"collaboration": s.store.PublicCollaboration(g), "duplicate": dup})
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/collaborations/") && strings.HasSuffix(r.URL.Path, "/recover"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/collaborations/"), "/recover")
		var input struct {
			Version              int     `json:"version"`
			RequestID            string  `json:"requestId"`
			ExpectedRevision     *uint64 `json:"expectedRevision"`
			ExpectedEpoch        *uint64 `json:"expectedEpoch"`
			ExpectedTaskRevision *uint64 `json:"expectedTaskRevision"`
			ClientNonce          string  `json:"clientNonce"`
		}
		if decode(w, r, &input) != nil || input.ExpectedRevision == nil || input.ExpectedEpoch == nil || input.ExpectedTaskRevision == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		result, dup, e := s.store.RecoverCollaboration(id, ledger.CollaborationRecoveryInput{Version: input.Version, RequestID: input.RequestID, ExpectedRevision: *input.ExpectedRevision, ExpectedEpoch: *input.ExpectedEpoch, ExpectedTaskRevision: *input.ExpectedTaskRevision, ClientNonce: input.ClientNonce})
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]any{"collaboration": s.store.PublicCollaboration(result.Collaboration), "work": s.store.PublicWork(result.Work), "duplicate": dup})
	case r.Method == "POST" && r.URL.Path == "/v1/task-controls":
		var input struct {
			Version          int     `json:"version"`
			RequestID        string  `json:"requestId"`
			ExpectedEpoch    *uint64 `json:"expectedEpoch"`
			ExpectedRevision *uint64 `json:"expectedRevision"`
			Action           string  `json:"action"`
			ClientNonce      string  `json:"clientNonce"`
			Text             string  `json:"text,omitempty"`
		}
		if decode(w, r, &input) != nil || input.ExpectedEpoch == nil || input.ExpectedRevision == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		result, duplicate, e := s.store.ControlTask(ledger.TaskControlInput{Version: input.Version, RequestID: input.RequestID, ExpectedEpoch: *input.ExpectedEpoch, ExpectedRevision: *input.ExpectedRevision, Action: input.Action, ClientNonce: input.ClientNonce, Text: input.Text})
		if e != nil {
			sendError(w, e)
			return
		}
		result = s.store.PublicControl(result)
		send(w, http.StatusOK, map[string]any{"work": result.Work, "related": result.Related, "duplicate": duplicate})
	case r.Method == "POST" && r.URL.Path == "/v1/adapters/sessions":
		var input ledger.AdapterSessionInput
		if decode(w, r, &input) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		session, duplicate, e := s.store.RegisterAdapter(input)
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]any{"session": session, "duplicate": duplicate})
	case strings.HasPrefix(r.URL.Path, "/v1/adapters/sessions/"):
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 6 {
			http.NotFound(w, r)
			return
		}
		if r.Method == "POST" && parts[5] == "messages" {
			var input ledger.AdapterMessageInput
			if decode(w, r, &input) != nil {
				sendError(w, ledger.ErrInvalid)
				return
			}
			work, duplicate, e := s.store.AcceptAdapter(parts[4], input)
			if e != nil {
				sendError(w, e)
				return
			}
			status := http.StatusCreated
			if duplicate {
				status = http.StatusOK
			}
			send(w, status, map[string]any{"work": s.store.PublicWork(work), "duplicate": duplicate})
			return
		}
		if r.Method == "GET" && parts[5] == "replies" {
			after := uint64(0)
			var e error
			if raw := r.URL.Query().Get("after"); raw != "" {
				after, e = strconv.ParseUint(raw, 10, 64)
				if e != nil {
					sendError(w, ledger.ErrInvalid)
					return
				}
			}
			page, e := s.store.AdapterReplies(parts[4], after)
			if e != nil {
				sendError(w, e)
				return
			}
			send(w, http.StatusOK, page)
			return
		}
		if r.Method == "GET" && parts[5] == "history" {
			s.adapterHistory(w, r, parts[4])
			return
		}
		http.NotFound(w, r)
	case r.Method == "POST" && r.URL.Path == "/v1/projects":
		var input ledger.ProjectInput
		if decode(w, r, &input) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		project, duplicate, e := s.store.SaveProject(input)
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]any{"project": project, "duplicate": duplicate})
	case r.Method == "POST" && r.URL.Path == "/v1/uploads":
		var input ledger.UploadInput
		if decodeLimit(w, r, &input, 1500000) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		file, duplicate, e := s.store.UploadFile(input)
		input.Content = nil
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]any{"file": file, "duplicate": duplicate})
	case r.Method == "POST" && r.URL.Path == "/v1/file-access":
		var input struct {
			FileID           string  `json:"fileId"`
			ExpectedRevision *uint64 `json:"expectedRevision"`
			Granted          *bool   `json:"granted"`
			ClientNonce      string  `json:"clientNonce"`
		}
		if decode(w, r, &input) != nil || input.ExpectedRevision == nil || input.Granted == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		if e := s.store.SetFileAccess(ledger.FileAccessInput{FileID: input.FileID, ExpectedRevision: *input.ExpectedRevision, Granted: *input.Granted, ClientNonce: input.ClientNonce}); e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]bool{"updated": true})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/projects/"):
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) == 7 && parts[4] == "files" && parts[6] == "text" {
			query := r.URL.Query()
			if len(query) != 3 || len(query["channelId"]) != 1 || len(query["projectRevision"]) != 1 || len(query["accessRevision"]) != 1 {
				sendError(w, ledger.ErrInvalid)
				return
			}
			pr, e1 := strconv.ParseUint(query.Get("projectRevision"), 10, 64)
			ar, e2 := strconv.ParseUint(query.Get("accessRevision"), 10, 64)
			if e1 != nil || e2 != nil {
				sendError(w, ledger.ErrInvalid)
				return
			}
			preview, err := s.store.PreviewProjectFile(parts[3], query.Get("channelId"), parts[5], pr, ar)
			if err != nil {
				sendError(w, err)
				return
			}
			send(w, http.StatusOK, preview)
			return
		}
		if len(parts) != 6 || parts[4] != "files" {
			http.NotFound(w, r)
			return
		}
		file, content, e := s.store.ReadProjectFile(parts[3], r.URL.Query().Get("channelId"), parts[5])
		if e != nil {
			sendError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename}))
		w.Header().Set("X-File-SHA256", file.SHA256)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	case r.Method == "POST" && r.URL.Path == "/v1/channels":
		var input ledger.ChannelInput
		if decode(w, r, &input) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		channel, duplicate, e := s.store.SaveChannel(input)
		if e != nil {
			sendError(w, e)
			return
		}
		send(w, http.StatusOK, map[string]any{"channel": channel, "duplicate": duplicate})
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
		send(w, http.StatusOK, map[string]any{"work": s.store.PublicWork(work), "duplicate": duplicate})
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
		send(w, http.StatusOK, s.store.PublicWork(work))
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/projects/") && strings.HasSuffix(r.URL.Path, "/state"):
		parts := strings.Split(r.URL.Path, "/")
		var in struct {
			Action           string  `json:"action"`
			ExpectedRevision *uint64 `json:"expectedRevision"`
			ClientNonce      string  `json:"clientNonce"`
		}
		if len(parts) != 5 || r.URL.RawQuery != "" || decode(w, r, &in) != nil || in.ExpectedRevision == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		state, duplicate, err := s.store.ControlProject(ledger.ProjectControlInput{ProjectID: parts[3], Action: in.Action, ExpectedRevision: *in.ExpectedRevision, ClientNonce: in.ClientNonce})
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, map[string]any{"projectState": state, "duplicate": duplicate})
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/bots/") && strings.HasSuffix(r.URL.Path, "/state"):
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 5 || r.URL.RawQuery != "" {
			sendError(w, ledger.ErrInvalid)
			return
		}
		var in struct {
			Action                 string  `json:"action"`
			ExpectedRevision       *uint64 `json:"expectedRevision"`
			ClientNonce            string  `json:"clientNonce"`
			DefaultAccountID       *string `json:"defaultAccountId"`
			DefaultAccountRevision *uint64 `json:"defaultAccountRevision"`
			DefaultModel           *string `json:"defaultModel"`
		}
		if decode(w, r, &in) != nil || in.ExpectedRevision == nil || (in.Action == "defaults" && (in.DefaultAccountID == nil || in.DefaultAccountRevision == nil || in.DefaultModel == nil)) || (in.Action != "defaults" && (in.DefaultAccountID != nil || in.DefaultAccountRevision != nil || in.DefaultModel != nil)) {
			sendError(w, ledger.ErrInvalid)
			return
		}
		input := ledger.BotControlInput{BotID: parts[3], Action: in.Action, ExpectedRevision: *in.ExpectedRevision, ClientNonce: in.ClientNonce}
		if in.Action == "defaults" {
			input.DefaultAccountID, input.DefaultAccountRevision, input.DefaultModel = *in.DefaultAccountID, *in.DefaultAccountRevision, *in.DefaultModel
		}
		var state ledger.BotState
		var duplicate bool
		var err error
		if s.fixture != nil {
			state, duplicate, err = s.store.ControlBotFor(principal, input)
		} else {
			state, duplicate, err = s.store.ControlBot(input)
		}
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, map[string]any{"botState": state, "duplicate": duplicate})
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/bots/") && strings.HasSuffix(r.URL.Path, "/profile"):
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 5 || r.URL.RawQuery != "" {
			sendError(w, ledger.ErrInvalid)
			return
		}
		var in struct {
			Instructions     *string `json:"instructions"`
			IncludeHistory   *bool   `json:"includeHistory"`
			ExpectedRevision *uint64 `json:"expectedRevision"`
			ClientNonce      string  `json:"clientNonce"`
		}
		if decode(w, r, &in) != nil || in.Instructions == nil || in.IncludeHistory == nil || in.ExpectedRevision == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		input := ledger.BotProfileInput{Instructions: *in.Instructions, IncludeHistory: *in.IncludeHistory, ExpectedRevision: *in.ExpectedRevision, ClientNonce: in.ClientNonce}
		var profile ledger.BotProfile
		var duplicate bool
		var err error
		if s.fixture != nil {
			profile, duplicate, err = s.store.SetBotProfileFor(principal, parts[3], input)
		} else {
			profile, duplicate, err = s.store.SetBotProfile(parts[3], input)
		}
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, map[string]any{"profile": profile, "duplicate": duplicate})
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/bots/") && strings.HasSuffix(r.URL.Path, "/name"):
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 5 || r.URL.RawQuery != "" {
			sendError(w, ledger.ErrInvalid)
			return
		}
		var in struct {
			Name                    string  `json:"name"`
			ExpectedDisplayRevision *uint64 `json:"expectedDisplayRevision"`
			ClientNonce             string  `json:"clientNonce"`
		}
		if decode(w, r, &in) != nil || in.ExpectedDisplayRevision == nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		input := ledger.BotNameInput{Name: in.Name, ExpectedDisplayRevision: *in.ExpectedDisplayRevision, ClientNonce: in.ClientNonce}
		var bot ledger.Bot
		var duplicate bool
		var err error
		if s.fixture != nil {
			bot, duplicate, err = s.store.RenameBotFor(principal, parts[3], input)
		} else {
			bot, duplicate, err = s.store.RenameBot(parts[3], input)
		}
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, map[string]any{"bot": bot, "duplicate": duplicate})
	case r.Method == "POST" && r.URL.Path == "/v1/bots":
		var input ledger.BotInput
		if decode(w, r, &input) != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		var bot ledger.Bot
		var duplicate bool
		var err error
		if s.fixture != nil {
			bot, duplicate, err = s.store.CreateBotFor(principal, input)
		} else {
			bot, duplicate, err = s.store.CreateBot(input)
		}
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
		public, _ := json.Marshal(map[string]any{"id": input.ID, "name": input.Name, "provider": input.Provider, "endpoint": input.Endpoint, "model": input.Model, "clientNonce": input.ClientNonce})
		if !s.store.PublicInputAllowed(public) {
			input.Secret = ""
			sendConnectionError(w, connections.ErrInvalid)
			return
		}
		account, duplicate, e := s.connections.SaveAccount(input)
		input.Secret = ""
		if e != nil {
			sendConnectionError(w, e)
			return
		}
		if e = s.store.ReconcileAccounts(); e != nil {
			sendError(w, e)
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
		if e := s.store.ReconcileAccounts(); e != nil {
			sendError(w, e)
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
		send(w, http.StatusOK, s.store.PublicSnapshot())
	case r.Method == "GET" && r.URL.Path == "/v1/events":
		s.stream(w, r)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/channels/"):
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 6 || parts[4] != "artifacts" {
			http.NotFound(w, r)
			return
		}
		artifact, content, e := s.store.ReadChannelArtifact(parts[3], parts[5])
		if e != nil {
			sendError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": artifact.Filename}))
		w.Header().Set("X-Artifact-SHA256", artifact.SHA256)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
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
		send(w, http.StatusOK, s.store.PublicWork(work))
	case r.Method == "POST" && r.URL.Path == "/v1/messages":
		var input struct {
			ledger.Input
			ledger.SubmissionScope
		}
		if err = decode(w, r, &input); err != nil {
			sendError(w, ledger.ErrInvalid)
			return
		}
		var work ledger.Work
		var duplicate bool
		work, duplicate, err = s.store.AcceptProduct(input.Input, input.SubmissionScope)
		if err != nil {
			sendError(w, err)
			return
		}
		status := http.StatusCreated
		if duplicate {
			status = http.StatusOK
		}
		send(w, status, map[string]any{"work": s.store.PublicWork(work), "duplicate": duplicate})
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
		send(w, http.StatusOK, s.store.PublicWork(work))
	default:
		http.NotFound(w, r)
	}
}
func strictReceiptQuery(r *http.Request) (map[string]string, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, ledger.ErrInvalid
	}
	out := map[string]string{}
	for key, value := range values {
		if (key != "kind" && key != "clientNonce" && key != "collaborationId") || len(value) != 1 || value[0] == "" {
			return nil, ledger.ErrInvalid
		}
		out[key] = value[0]
	}
	return out, nil
}
func strictUploadReceiptQuery(r *http.Request) (map[string]string, error) {
	if len(r.URL.RawQuery) > 1024 {
		return nil, ledger.ErrInvalid
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(values) != 3 {
		return nil, ledger.ErrInvalid
	}
	out := map[string]string{}
	for key, value := range values {
		if (key != "projectId" && key != "channelId" && key != "clientNonce") || len(value) != 1 || value[0] == "" {
			return nil, ledger.ErrInvalid
		}
		out[key] = value[0]
	}
	return out, nil
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
	return decodeLimit(w, r, out, 65536)
}
func decodeLimit(w http.ResponseWriter, r *http.Request, out any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
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
	case errors.Is(err, identity.ErrUnauthenticated):
		code = http.StatusUnauthorized
		label = "fixture_session_invalid"
	case errors.Is(err, ledger.ErrForbidden):
		code = http.StatusForbidden
		label = "principal_forbidden"
	case errors.Is(err, ledger.ErrUnsupported):
		code = http.StatusNotImplemented
		label = "contract_unsupported"
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
