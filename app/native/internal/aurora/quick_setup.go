package aurora

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/privatefs"
	"nekoaurora.local/native/internal/vault"
)

type quickProvider struct {
	ID           string             `json:"id"`
	Label        string             `json:"label"`
	Auth         string             `json:"auth"`
	Model        string             `json:"model"`
	Models       []string           `json:"models"`
	KeyEnv       string             `json:"keyEnv,omitempty"`
	ModelSource  string             `json:"modelSource,omitempty"`
	ModelOptions []quickModelOption `json:"modelOptions,omitempty"`
	CatalogError string             `json:"catalogError,omitempty"`
	CheckedAt    string             `json:"checkedAt,omitempty"`
}
type quickLogin struct {
	ID    string `json:"id"`
	State string `json:"state"`
	URL   string `json:"url,omitempty"`
}
type quickSetup struct {
	mu           sync.Mutex
	Core         *Core
	Providers    []quickProvider
	ChatGPT      bool
	login        quickLogin
	cancel       context.CancelFunc
	runLogin     func(context.Context, func(quickLogin)) error
	runModels    func(context.Context) (quickModelCatalog, error)
	modelsMu     sync.Mutex
	models       *quickModelCatalog
	modelsError  string
	runAPIModels func(context.Context, quickProvider, string) (quickModelCatalog, error)
	apiModels    map[string]quickAPICatalog
}

func openQuickSetup(parent context.Context, dir string) *quickSetup {
	c := CoreFromEnv(dir)
	if c == nil {
		return nil
	}
	if err := os.MkdirAll(c.Home, 0o700); err != nil {
		return nil
	}
	if privatefs.ProtectDirectory(c.Home) != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cmd := c.setupCommand(ctx, "catalog")
	var out limited
	cmd.Stdout = &out
	if cmd.Run() != nil {
		return nil
	}
	var data struct {
		Providers []quickProvider `json:"providers"`
		ChatGPT   bool            `json:"chatgpt"`
	}
	if json.Unmarshal([]byte(out.String()), &data) != nil || len(data.Providers) == 0 {
		return nil
	}
	q := &quickSetup{Core: c, Providers: data.Providers, ChatGPT: data.ChatGPT}
	q.runModels = c.chatGPTModels
	q.runLogin = func(ctx context.Context, emit func(quickLogin)) error {
		if err := privatefs.CheckDirectory(c.Home); err != nil {
			return err
		}
		cmd := c.setupCommand(ctx, "login")
		pipe, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		if err = cmd.Start(); err != nil {
			return err
		}
		scan := bufio.NewScanner(pipe)
		scan.Buffer(make([]byte, 4096), 16384)
		for scan.Scan() {
			var e quickLogin
			if json.Unmarshal(scan.Bytes(), &e) == nil {
				emit(e)
			}
		}
		if scan.Err() != nil {
			_ = cmd.Process.Kill()
		}
		return cmd.Wait()
	}
	return q
}

func (c *Core) setupCommand(ctx context.Context, action string) *exec.Cmd {
	args := append(c.bunFlags(), filepath.Join(filepath.Dir(c.Entry), "aurora-setup.ts"), action)
	cmd := exec.CommandContext(ctx, c.Bun, args...)
	cmd.Dir, cmd.Env, cmd.Stderr = c.Home, c.environment(), io.Discard
	hideCoreWindow(cmd)
	return cmd
}

func (q *quickSetup) provider(id string) (quickProvider, bool) {
	if q != nil {
		for _, p := range q.providers() {
			if p.ID == id {
				return p, true
			}
		}
	}
	return quickProvider{}, false
}
func (s *Service) quickReady(p quickProvider) bool {
	if p.Auth == "chatgpt_oauth" {
		s.quick.mu.Lock()
		defer s.quick.mu.Unlock()
		return s.quick.ChatGPT
	}
	return s.V != nil && s.V.Has(providerKey("core-"+p.ID))
}
func (s *Service) quickModel(selection string) *Core {
	id, model, ok := strings.Cut(strings.TrimPrefix(selection, "core-"), ":")
	p, found := s.quickProvider(id)
	if !ok || !found || !s.quickReady(p) {
		return nil
	}
	valid := false
	for _, m := range p.Models {
		valid = valid || m == model
	}
	if !valid && !coreModelID.MatchString(model) {
		return nil
	}
	c := *s.quick.Core
	c.Profile, c.Model, c.KeyEnv, c.Quick = p.ID, model, p.KeyEnv, true
	c.key, c.SignedIn = "", p.Auth == "chatgpt_oauth"
	if !c.SignedIn {
		c.key, _ = s.V.Get(providerKey("core-" + p.ID))
	}
	return &c
}
func (s *Service) quickSelection(bot string) string {
	if s.ext == nil {
		return ""
	}
	if bot != "" {
		return s.ext.bot(bot).Model
	}
	s.ext.mu.Lock()
	defer s.ext.mu.Unlock()
	return s.ext.d.DefaultModel
}

func (s *Service) configuredModel(bot string) Model {
	selection := s.quickSelection(bot)
	if selection == "" {
		selection = s.quickSelection("")
	}
	if strings.HasPrefix(selection, "core-") {
		c := s.quickModel(selection)
		if c == nil {
			return nil
		}
		return c
	}
	return s.AI
}
func (s *Service) modelConfigured(bot string) bool {
	m := s.configuredModel(bot)
	return m != nil && m.Configured()
}
func (s *Service) aiView() map[string]any {
	return s.aiViewFor("")
}

func (s *Service) aiViewFor(bot string) map[string]any {
	selection := s.quickSelection(bot)
	inherited := bot != "" && selection == ""
	if selection == "" {
		selection = s.quickSelection("")
	}
	view := map[string]any{"configured": false, "selection": selection, "inherited": inherited}
	if selection != "" {
		id, model, _ := strings.Cut(selection, ":")
		view["model"], view["provider"] = model, strings.TrimPrefix(id, "core-")
		if strings.HasPrefix(id, "core-") {
			view["runtime"] = "neko-core-cli"
			if p, ok := s.quickProvider(strings.TrimPrefix(id, "core-")); ok {
				view["provider"], view["configured"] = p.Label, s.quickReady(p)
				view["modelSource"] = p.ModelSource
				if p.ModelSource == "account" {
					view["issue"] = chatGPTSelectionIssue(quickModelCatalog{Models: p.ModelOptions}, model)
					for _, m := range p.ModelOptions {
						if m.ID == model {
							view["modelLabel"], view["transport"] = m.Label, m.Transport
						}
					}
				}
			}
		} else {
			view["runtime"] = "direct-http"
			for _, m := range s.modelMenu() {
				if m.ID == selection {
					view["provider"] = m.Provider
					break
				}
			}
			_, ai, ok := s.callFor(selection)
			view["configured"] = ok && ai.Configured()
		}
		return view
	}
	if s.AI != nil {
		view["configured"], view["model"] = s.AI.Configured(), s.AI.ModelName()
		view["provider"], view["runtime"] = "z.ai", s.AI.Runtime()
	}
	return view
}
func (s *Service) quickScope(bot string) bool {
	if bot == "" {
		return true
	}
	_, ok := s.view().bot(bot)
	return ok
}
func (s *Service) quickView(bot string) map[string]any {
	rows := []map[string]any{}
	if s.quick != nil {
		for _, p := range s.quickProviders() {
			rows = append(rows, map[string]any{"id": p.ID, "label": p.Label, "auth": p.Auth, "model": p.Model, "models": p.Models, "configured": s.quickReady(p), "modelSource": p.ModelSource, "modelOptions": p.ModelOptions, "catalogError": p.CatalogError, "checkedAt": p.CheckedAt})
		}
	}
	return map[string]any{"workspaceId": s.workspaceID, "available": s.quick != nil, "providers": rows, "selection": s.quickSelection(bot), "effective": s.aiViewFor(bot)}
}

func (s *Service) quickSetupRoutes() {
	s.mux.HandleFunc("POST /app/api/quick-setup/models", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WorkspaceID string `json:"workspaceId"`
			BotID       string `json:"botId"`
			Provider    string `json:"provider"`
			Key         string `json:"key"`
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		if in.WorkspaceID != s.workspaceID || s.L.ReadOnly() {
			fail(w, ledger.ErrConflict)
			return
		}
		if !s.quickScope(in.BotID) {
			fail(w, ErrNotFound)
			return
		}
		if s.quick == nil {
			writeJSON(w, 503, map[string]string{"error": "core_setup_unavailable"})
			return
		}
		p, ok := s.quickProvider(first(in.Provider, "chatgpt"))
		in.Key = strings.TrimSpace(in.Key)
		if !ok || !validQuickKey(in.Key) || p.Auth == "chatgpt_oauth" && in.Key != "" {
			fail(w, ErrInvalid)
			return
		}
		if p.Auth == "chatgpt_oauth" {
			if s.quick.runModels == nil {
				fail(w, ErrInvalid)
				return
			}
			_ = s.quick.refreshModels(r.Context())
		} else {
			key := first(in.Key, s.quickKey(p.ID))
			catalog, err := s.loadAPIModels(r.Context(), p, key)
			if err != nil {
				catalog = quickModelCatalog{Error: catalogErrorCode(err)}
			}
			if key != "" && key == s.quickKey(p.ID) {
				s.rememberAPIModels(p, key, catalog)
			}
			view := s.quickView(in.BotID)
			// A candidate key's catalog is for this form only; it must not change another bot's connection.
			for _, row := range view["providers"].([]map[string]any) {
				if row["id"] == p.ID {
					candidate := catalogProvider(quickProvider{ID: p.ID, ModelSource: "account_required", Models: []string{}}, catalog)
					row["models"], row["modelOptions"], row["modelSource"], row["catalogError"], row["checkedAt"] = candidate.Models, candidate.ModelOptions, candidate.ModelSource, candidate.CatalogError, candidate.CheckedAt
				}
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, 200, view)
			return
		}
		s.resync()
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, s.quickView(in.BotID))
	})
	s.mux.HandleFunc("GET /app/api/quick-setup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		bot := r.URL.Query().Get("botId")
		if !s.quickScope(bot) {
			fail(w, ErrNotFound)
			return
		}
		writeJSON(w, 200, s.quickView(bot))
	})
	s.mux.HandleFunc("POST /app/api/quick-setup", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WorkspaceID       string `json:"workspaceId"`
			BotID             string `json:"botId"`
			Provider          string `json:"provider"`
			Model             string `json:"model"`
			Key               string `json:"key"`
			ExpectedSelection string `json:"expectedSelection"`
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		if in.WorkspaceID != s.workspaceID || s.L.ReadOnly() {
			fail(w, ledger.ErrConflict)
			return
		}
		if s.quick == nil {
			writeJSON(w, 503, map[string]string{"error": "core_setup_unavailable"})
			return
		}
		p, ok := s.quickProvider(in.Provider)
		selection := "core-" + p.ID + ":" + in.Model
		if !ok || !coreModelID.MatchString(in.Model) || !s.quickScope(in.BotID) {
			fail(w, ErrInvalid)
			return
		}
		unlock, err := s.mcpState().lock(r.Context(), "configuration\x00"+in.BotID)
		if err != nil {
			fail(w, err)
			return
		}
		defer unlock()
		providerUnlock, err := s.mcpState().lock(r.Context(), "quick-provider\x00"+p.ID)
		if err != nil {
			fail(w, err)
			return
		}
		defer providerUnlock()
		if in.ExpectedSelection != s.quickSelection(in.BotID) {
			fail(w, ledger.ErrConflict)
			return
		}
		in.Key = strings.TrimSpace(in.Key)
		if p.Auth == "chatgpt_oauth" && in.Key != "" || !validQuickKey(in.Key) {
			fail(w, ErrInvalid)
			return
		}
		var catalog quickModelCatalog
		key := first(in.Key, s.quickKey(p.ID))
		if p.Auth == "chatgpt_oauth" {
			if !validModel(s.modelMenu(), selection) {
				fail(w, ErrInvalid)
				return
			}
		} else {
			catalog, err = s.loadAPIModels(r.Context(), p, key)
			if err == nil {
				if issue := chatGPTSelectionIssue(catalog, in.Model); issue != "" {
					err = errors.New(issue)
				}
			}
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": catalogErrorCode(err)})
				return
			}
		}
		if in.Key != "" {
			if s.V == nil {
				fail(w, ledger.ErrConflict)
				return
			}
			if _, err = s.V.Set(vault.Meta{Name: providerKey("core-" + p.ID), Label: p.Label, Kind: "api_key"}, in.Key); err != nil {
				vaultErr(w, err)
				return
			}
		}
		if !s.quickReady(p) {
			writeJSON(w, 400, map[string]string{"error": "provider_not_configured"})
			return
		}
		if in.BotID != "" {
			be := s.ext.bot(in.BotID)
			be.Model = selection
			err = s.ext.setBot(in.BotID, be)
		} else {
			s.ext.mu.Lock()
			old := s.ext.d.DefaultModel
			s.ext.d.DefaultModel = selection
			err = s.ext.saveLocked()
			if err != nil {
				s.ext.d.DefaultModel = old
			}
			s.ext.mu.Unlock()
		}
		if err != nil {
			fail(w, err)
			return
		}
		if p.Auth != "chatgpt_oauth" {
			s.rememberAPIModels(p, key, catalog)
		}
		s.resync()
		writeJSON(w, 200, s.quickView(in.BotID))
	})
	s.mux.HandleFunc("POST /app/api/quick-setup/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WorkspaceID string `json:"workspaceId"`
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		if in.WorkspaceID != s.workspaceID || s.L.ReadOnly() {
			fail(w, ledger.ErrConflict)
			return
		}
		q := s.quick
		if q == nil || q.runLogin == nil {
			writeJSON(w, 503, map[string]string{"error": "core_setup_unavailable"})
			return
		}
		q.mu.Lock()
		defer q.mu.Unlock()
		if q.login.State == "starting" || q.login.State == "waiting" {
			writeJSON(w, 200, q.login)
			return
		}
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			fail(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
		q.cancel = cancel
		q.login = quickLogin{ID: hex.EncodeToString(id), State: "starting"}
		attempt := q.login.ID
		s.bg.Add(1)
		go func() {
			defer s.bg.Done()
			defer cancel()
			connected := false
			err := q.runLogin(ctx, func(e quickLogin) {
				q.mu.Lock()
				defer q.mu.Unlock()
				if q.login.ID != attempt || ctx.Err() != nil {
					return
				}
				if e.State == "waiting" && validQuickLoginURL(e.URL) {
					q.login.State, q.login.URL = "waiting", e.URL
				}
				if e.State == "connected" {
					connected = true
				}
			})
			q.mu.Lock()
			defer q.mu.Unlock()
			if q.login.ID != attempt || q.login.State == "cancelled" {
				return
			}
			q.login.URL = ""
			if err == nil && connected && ctx.Err() == nil {
				q.ChatGPT = true
				q.models, q.modelsError = nil, ""
				q.login.State = "connected"
			} else {
				q.login.State = "failed"
			}
		}()
		writeJSON(w, 202, q.login)
	})
	s.mux.HandleFunc("GET /app/api/quick-setup/login/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		q := s.quick
		if q == nil {
			fail(w, ErrNotFound)
			return
		}
		q.mu.Lock()
		defer q.mu.Unlock()
		if q.login.ID != r.PathValue("id") {
			fail(w, ErrNotFound)
			return
		}
		writeJSON(w, 200, q.login)
	})
	s.mux.HandleFunc("POST /app/api/quick-setup/login/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WorkspaceID string `json:"workspaceId"`
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		if in.WorkspaceID != s.workspaceID {
			fail(w, ledger.ErrConflict)
			return
		}
		q := s.quick
		if q == nil {
			fail(w, ErrNotFound)
			return
		}
		q.mu.Lock()
		defer q.mu.Unlock()
		if q.login.ID != r.PathValue("id") {
			fail(w, ErrNotFound)
			return
		}
		if q.login.State == "starting" || q.login.State == "waiting" {
			q.cancel()
			q.login.State = "cancelled"
			q.login.URL = ""
		}
		writeJSON(w, 200, q.login)
	})
}
func validQuickLoginURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && len(raw) < 12000 && u.Scheme == "https" && u.Host == "auth.openai.com" && u.User == nil && u.Path == "/oauth/authorize" && u.Fragment == ""
}
