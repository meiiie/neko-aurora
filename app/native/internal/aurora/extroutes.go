package aurora

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"nekoaurora.local/native/internal/computer"
	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/tools"
	"nekoaurora.local/native/internal/vault"
)

type modelOpt struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Provider string `json:"provider"`
}

func (s *Service) modelMenu() []modelOpt {
	def := DefaultModel
	if a := httpAI(s.AI); a != nil {
		def = a.Model
	}
	out := []modelOpt{}
	for _, m := range defaultModels(def) {
		out = append(out, modelOpt{"glm:" + m, m, "GLM (mặc định)"})
	}
	s.ext.mu.Lock()
	defer s.ext.mu.Unlock()
	for _, p := range s.ext.d.Providers {
		for _, m := range p.Models {
			out = append(out, modelOpt{p.ID + ":" + m, m, p.Name})
		}
	}
	return out
}

func validModel(menu []modelOpt, id string) bool {
	if id == "" {
		return true
	}
	for _, m := range menu {
		if m.ID == id {
			return true
		}
	}
	return false
}

func (s *Service) providersView() []map[string]any {
	s.ext.mu.Lock()
	defer s.ext.mu.Unlock()
	out := []map[string]any{}
	for _, p := range s.ext.d.Providers {
		out = append(out, map[string]any{"id": p.ID, "name": p.Name, "base": p.Base, "models": p.Models, "hasKey": s.V != nil && s.V.Has(providerKey(p.ID)), "keyName": providerKey(p.ID)})
	}
	return out
}

func (s *Service) extRoutes() {
	s.mcpAuthorizationRoutes()
	m := s.mux
	m.HandleFunc("GET /app/api/models", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"models": s.modelMenu(), "providers": s.providersView(), "vision": s.visionModel(), "browser": browserBin() != ""})
	})
	m.HandleFunc("POST /app/api/providers", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     string   `json:"id"`
			Name   string   `json:"name"`
			Base   string   `json:"base"`
			Models []string `json:"models"`
			Key    string   `json:"key"` // write-only: goes to the vault, never returned
		}
		if decodeStrict(r, &in) != nil || !providerID.MatchString(in.ID) || in.ID == "glm" {
			fail(w, ErrInvalid)
			return
		}
		in.Base = strings.TrimRight(strings.TrimSpace(in.Base), "/")
		if !ownerURL(in.Base) {
			writeJSON(w, 400, map[string]string{"error": "base_url"})
			return
		}
		models := []string{}
		for _, x := range in.Models {
			if x = clean(x, 80); x != "" && !strings.ContainsAny(x, " \n") && len(models) < 12 {
				models = append(models, x)
			}
		}
		if len(models) == 0 {
			fail(w, ErrInvalid)
			return
		}
		if in.Key != "" {
			if s.V == nil || s.L.ReadOnly() {
				fail(w, ledger.ErrConflict)
				return
			}
			if _, err := s.V.Set(vault.Meta{Name: providerKey(in.ID), Label: "Khoá " + clean(in.Name, 40), Kind: "api_key"}, in.Key); err != nil {
				vaultErr(w, err)
				return
			}
		}
		p := provider{ID: in.ID, Name: first(clean(in.Name, 40), in.ID), Base: in.Base, Models: models}
		s.ext.mu.Lock()
		found := false
		for i := range s.ext.d.Providers {
			if s.ext.d.Providers[i].ID == p.ID {
				s.ext.d.Providers[i], found = p, true
			}
		}
		if !found {
			if len(s.ext.d.Providers) >= 12 {
				s.ext.mu.Unlock()
				fail(w, ErrInvalid)
				return
			}
			s.ext.d.Providers = append(s.ext.d.Providers, p)
		}
		s.ext.saveLocked()
		s.ext.mu.Unlock()
		s.record("", "", "secret", "Đã lưu nhà cung cấp mô hình "+p.Name)
		writeJSON(w, 200, map[string]any{"providers": s.providersView(), "models": s.modelMenu()})
	})
	m.HandleFunc("POST /app/api/providers/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		s.ext.mu.Lock()
		keep := s.ext.d.Providers[:0]
		for _, p := range s.ext.d.Providers {
			if p.ID != id {
				keep = append(keep, p)
			}
		}
		s.ext.d.Providers = keep
		for _, b := range s.ext.d.Bots {
			if strings.HasPrefix(b.Model, id+":") {
				b.Model = ""
			}
		}
		if strings.HasPrefix(s.ext.d.Vision, id+":") {
			s.ext.d.Vision = ""
		}
		s.ext.saveLocked()
		s.ext.mu.Unlock()
		if s.V != nil {
			_ = s.V.Delete(providerKey(id))
		}
		writeJSON(w, 200, map[string]any{"providers": s.providersView()})
	})
	m.HandleFunc("POST /app/api/vision", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model string `json:"model"`
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		p, mdl, ok := strings.Cut(in.Model, ":")
		if in.Model != "" && (!ok || mdl == "" || p != "glm" && !strings.HasPrefix(s.modelProviderOK(p), "ok")) {
			fail(w, ErrInvalid)
			return
		}
		s.ext.mu.Lock()
		s.ext.d.Vision = clean(in.Model, 120)
		s.ext.saveLocked()
		s.ext.mu.Unlock()
		writeJSON(w, 200, map[string]string{"vision": s.visionModel()})
	})
	m.HandleFunc("GET /app/api/agents/{id}/ext", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, ok := s.view().bot(id); !ok {
			fail(w, ErrNotFound)
			return
		}
		be := s.ext.bot(id)
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		srv := []map[string]any{}
		for _, x := range be.MCP {
			ts, err := s.mcpTools(ctx, id, x)
			names := []map[string]any{}
			for _, t := range ts {
				names = append(names, map[string]any{"name": t.Name})
			}
			e := ""
			if err != nil {
				e = tools.Short(err)
				if strings.HasPrefix(err.Error(), "MCP") || strings.Contains(err.Error(), "máy chủ MCP") {
					e = clean(err.Error(), 120)
				}
			}
			state, retryAt := "ready", ""
			if err != nil {
				state = "unavailable"
				var failure *mcpFailure
				if errors.As(err, &failure) {
					state = failure.State
					if !failure.RetryAt.IsZero() {
						retryAt = failure.RetryAt.UTC().Format(time.RFC3339)
					}
				}
			}
			srv = append(srv, map[string]any{"name": x.Name, "url": x.URL, "header": x.Header, "secret": x.Secret, "tools": names, "error": s.redact(e), "state": state, "retryAt": retryAt})
		}
		writeJSON(w, 200, map[string]any{"workspaceId": s.workspaceID, "mcpRevision": be.MCPRevision, "model": be.Model, "thinking": be.Thinking, "folder": be.Folder, "skills": be.Skills, "mcp": srv, "off": be.Off, "menu": s.modelMenu(), "allSkills": s.ext.skills()})
	})
	m.HandleFunc("POST /app/api/agents/{id}/mcp/reconnect", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name        string `json:"name"`
			WorkspaceID string `json:"workspaceId"`
			MCPRevision uint64 `json:"mcpRevision"`
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		id := r.PathValue("id")
		unlock, err := s.mcpState().lock(r.Context(), "configuration\x00"+id)
		if err != nil {
			fail(w, err)
			return
		}
		defer unlock()
		if in.WorkspaceID != s.workspaceID || in.MCPRevision != s.ext.bot(id).MCPRevision {
			writeJSON(w, 409, map[string]string{"error": "mcp_configuration_changed"})
			return
		}
		srv, ok := s.mcpServerOf(id, in.Name)
		if !ok {
			fail(w, ErrNotFound)
			return
		}
		cache := s.mcpState()
		cache.mu.Lock()
		failure := cache.failures[mcpKey(id, srv)]
		limited := failure != nil && failure.State == "rate_limited" && time.Now().Before(failure.RetryAt)
		cache.mu.Unlock()
		if limited {
			writeJSON(w, 429, map[string]string{"error": "rate_limited", "retryAt": failure.RetryAt.UTC().Format(time.RFC3339)})
			return
		}
		if err := s.mcpDrop(r.Context(), id, srv); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	m.HandleFunc("POST /app/api/agents/{id}/ext", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, ok := s.view().bot(id); !ok {
			fail(w, ErrNotFound)
			return
		}
		var in struct {
			WorkspaceID string           `json:"workspaceId"`
			MCPRevision uint64           `json:"mcpRevision"`
			Model       *string          `json:"model"`
			Thinking    *bool            `json:"thinking"`
			Folder      *string          `json:"folder"`
			Skills      *[]string        `json:"skills"`
			MCP         *[]mcpServer     `json:"mcp"`
			Off         *map[string]bool `json:"off"`
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		unlock, err := s.mcpState().lock(r.Context(), "configuration\x00"+id)
		if err != nil {
			fail(w, err)
			return
		}
		defer unlock()
		be := s.ext.bot(id)
		if (in.WorkspaceID != "" && in.WorkspaceID != s.workspaceID) || (in.MCP != nil && (in.WorkspaceID != s.workspaceID || in.MCPRevision != be.MCPRevision || be.MCPRevision >= 9007199254740991)) {
			writeJSON(w, 409, map[string]string{"error": "mcp_configuration_changed"})
			return
		}
		if in.Model != nil {
			if !validModel(s.modelMenu(), *in.Model) {
				fail(w, ErrInvalid)
				return
			}
			be.Model = *in.Model
		}
		if in.Thinking != nil {
			be.Thinking = *in.Thinking
		}
		if in.Folder != nil {
			f := strings.Trim(strings.TrimSpace(strings.TrimPrefix(*in.Folder, "/home/bot")), "/")
			if !validFolder(f) {
				writeJSON(w, 400, map[string]string{"error": "folder"})
				return
			}
			be.Folder = f
		}
		if in.Skills != nil {
			be.Skills = nil
			for _, x := range *in.Skills {
				if _, ok := s.ext.skill(x); ok && len(be.Skills) < 20 {
					be.Skills = append(be.Skills, x)
				}
			}
		}
		if in.Off != nil {
			be.Off = map[string]bool{}
			for k, v := range *in.Off {
				if capLabel[k] != "" && v {
					be.Off[k] = true
				}
			}
		}
		var dropKeys []string
		if in.MCP != nil {
			if len(*in.MCP) > 6 {
				fail(w, ErrInvalid)
				return
			}
			next := []mcpServer{}
			for _, x := range *in.MCP {
				x.Name, x.URL, x.Header, x.Secret = clean(x.Name, 32), strings.TrimSpace(x.URL), clean(x.Header, 64), strings.ToUpper(strings.TrimSpace(x.Secret))
				duplicate := false
				for _, existing := range next {
					if strings.EqualFold(existing.Name, x.Name) {
						duplicate = true
						break
					}
				}
				if x.Name == "" || duplicate || strings.ContainsAny(x.Name, "/ ") || !ownerURL(x.URL) || (x.Secret != "" && !vault.ValidName(x.Secret)) {
					writeJSON(w, 400, map[string]string{"error": "mcp_server"})
					return
				}
				next = append(next, x)
			}
			locked := map[string]bool{}
			for _, x := range be.MCP {
				key := mcpKey(id, x)
				if locked[key] {
					continue
				}
				release, err := s.mcpState().lock(r.Context(), key)
				if err != nil {
					fail(w, err)
					return
				}
				defer release()
				locked[key] = true
				dropKeys = append(dropKeys, key)
			}
			be.MCP = next
			be.MCPRevision++
		}
		if err := s.ext.setBot(id, be); err != nil {
			writeJSON(w, 500, map[string]string{"error": "settings_save_failed"})
			return
		}
		if len(dropKeys) > 0 {
			cache := s.mcpState()
			cache.mu.Lock()
			for _, key := range dropKeys {
				cache.dropLocked(key)
			}
			cache.mu.Unlock()
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	m.HandleFunc("GET /app/api/skills", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"skills": s.ext.skills()})
	})
	m.HandleFunc("POST /app/api/skills", func(w http.ResponseWriter, r *http.Request) {
		var in skill
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		in.Name, in.Desc, in.Body = clean(in.Name, 60), clean(in.Desc, 200), clean(in.Body, 6000)
		if in.Name == "" || in.Body == "" {
			fail(w, ErrInvalid)
			return
		}
		s.ext.mu.Lock()
		if in.ID == "" {
			in.ID = newID("sk-")
		}
		for _, b := range builtinSkills {
			if b.ID == in.ID {
				s.ext.mu.Unlock()
				fail(w, ErrConflict)
				return
			}
		}
		done := false
		for i := range s.ext.d.Skills {
			if s.ext.d.Skills[i].ID == in.ID {
				s.ext.d.Skills[i], done = in, true
			}
		}
		if !done {
			if len(s.ext.d.Skills) >= 40 {
				s.ext.mu.Unlock()
				fail(w, ErrInvalid)
				return
			}
			s.ext.d.Skills = append(s.ext.d.Skills, in)
		}
		s.ext.saveLocked()
		s.ext.mu.Unlock()
		writeJSON(w, 200, map[string]any{"skills": s.ext.skills()})
	})
	m.HandleFunc("POST /app/api/skills/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		s.ext.mu.Lock()
		keep := s.ext.d.Skills[:0]
		for _, x := range s.ext.d.Skills {
			if x.ID != id {
				keep = append(keep, x)
			}
		}
		s.ext.d.Skills = keep
		s.ext.saveLocked()
		s.ext.mu.Unlock()
		writeJSON(w, 200, map[string]any{"skills": s.ext.skills()})
	})
	m.HandleFunc("POST /app/api/forms/{task}/done", func(w http.ResponseWriter, r *http.Request) {
		question, err := s.L.Get(r.PathValue("task"))
		if err == nil && s.hasFormReceipt(question.RequestID, threadOf(question)) {
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		f, durable := s.formForWork(question)
		if f == nil {
			fail(w, ErrNotFound)
			return
		}
		if durable {
			fail(w, ErrConflict)
			return
		}
		accepted := false
		for _, work := range s.L.Snapshot().Work {
			if formAnswered(work.ClientNonce) == r.PathValue("task") {
				accepted = true
				break
			}
		}
		if !accepted {
			fail(w, ErrConflict)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	m.HandleFunc("POST /app/api/agents/{id}/memory/ask", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Q     string `json:"q"`
			Scope string `json:"scope"`
		}
		if decodeStrict(r, &in) != nil || strings.TrimSpace(in.Q) == "" {
			fail(w, ErrInvalid)
			return
		}
		id := r.PathValue("id")
		items := append(s.memList(id, ""), s.Mem.List(teamKey)...)
		if len(items) == 0 {
			writeJSON(w, 200, map[string]string{"answer": "Bot chưa nhớ điều gì."})
			return
		}
		if !s.admitUtility(w) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		out, err := s.stream(ctx, id, "memory", []ChatMsg{{"system", memAskPrompt}, {"user", "TRÍ NHỚ:\n" + memText(items) + "\nCÂU HỎI: " + clean(in.Q, 500)}}, func(string) {})
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": "ai_failed"})
			return
		}
		writeJSON(w, 200, map[string]string{"answer": strings.TrimSpace(out)})
	})
	m.HandleFunc("POST /app/api/agents/{id}/memory/summarize", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Scope string   `json:"scope"`
			Apply []string `json:"apply"` // the reviewed list to save (owner confirmed)
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		id := r.PathValue("id")
		key := id
		if in.Scope == "team" {
			key = teamKey
		}
		if in.Apply != nil {
			list := parseList(mustJSON(in.Apply))
			if len(list) == 0 {
				fail(w, ErrInvalid)
				return
			}
			s.Mem.Replace(key, list, "owner")
			writeJSON(w, 200, map[string]any{"items": s.Mem.List(key)})
			return
		}
		items := s.Mem.List(key)
		if len(items) < 2 {
			writeJSON(w, 200, map[string]any{"proposal": []string{}})
			return
		}
		if !s.admitUtility(w) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		out, err := s.stream(ctx, id, "memory", []ChatMsg{{"system", memSumPrompt}, {"user", memText(items)}}, func(string) {})
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": "ai_failed"})
			return
		}
		writeJSON(w, 200, map[string]any{"proposal": parseList(out), "before": len(items)})
	})
}

func (s *Service) admitUtility(w http.ResponseWriter) bool {
	if s.utilityAdmission != nil && !s.utilityAdmission() {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "quota_exceeded"})
		return false
	}
	return true
}

func (s *Service) modelProviderOK(p string) string {
	s.ext.mu.Lock()
	defer s.ext.mu.Unlock()
	for _, x := range s.ext.d.Providers {
		if x.ID == p {
			return "ok"
		}
	}
	return ""
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

var uploadName = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

// uploadRoutes: the owner sends a photo/file into the bot's computer
// (uploads/<name>); images are then read by the vision fallback model.
func (s *Service) uploadRoutes() {
	s.mux.HandleFunc("POST /app/api/agents/{id}/computer/upload", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		// JSON body (the host's CSRF gate accepts only same-origin JSON):
		// {"name":"…","data":"<base64>"}
		var in struct {
			Name        string `json:"name"`
			Data        string `json:"data"`
			WorkspaceID string `json:"workspaceId"`
			Deduplicate bool   `json:"deduplicate"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1500<<10)).Decode(&in); err != nil {
			writeJSON(w, 413, map[string]string{"error": "too_large"})
			return
		}
		if (in.Deduplicate || in.WorkspaceID != "") && in.WorkspaceID != s.workspaceID {
			writeJSON(w, 409, map[string]string{"error": "workspace_changed"})
			return
		}
		if _, ok := s.view().bot(id); !ok || s.PC == nil {
			fail(w, ErrNotFound)
			return
		}
		b, err := base64.StdEncoding.DecodeString(in.Data)
		if err != nil || len(b) == 0 || len(b) > 1<<20 {
			writeJSON(w, 400, map[string]string{"error": "bad_image"})
			return
		}
		name := strings.Trim(uploadName.ReplaceAllString(in.Name, "-"), "-.")
		if in.Deduplicate {
			// Keep the full digest and a stable name within imageAuto's path bound.
			if name == "" || len(name) > 12 {
				name = "image.jpg"
			}
			hash := sha256.Sum256(b)
			p := "uploads/i-" + hex.EncodeToString(hash[:]) + "-" + name
			if err := s.PC.CreateFileOnce(id, p, b); err != nil {
				status, code := 400, "write_failed"
				if errors.Is(err, computer.ErrFileConflict) {
					status, code = 409, "upload_conflict"
				}
				if errors.Is(err, computer.ErrQuota) {
					status, code = 413, "home_quota_exceeded"
				}
				writeJSON(w, status, map[string]string{"error": code})
				return
			}
			writeJSON(w, 200, map[string]string{"path": p})
			return
		}
		if name == "" || len(name) > 60 {
			name = "anh-" + time.Now().Format("150405") + ".jpg"
		}
		p := "uploads/" + newID("img-") + "-" + name
		if err := s.PC.WriteFile(id, p, string(b)); err != nil {
			writeJSON(w, 400, map[string]string{"error": "write_failed"})
			return
		}
		writeJSON(w, 200, map[string]string{"path": p})
	})
}

var imgRef = regexp.MustCompile(`\[Ảnh: (uploads/[^\]\s]{1,90})\]`)

// imageAuto describes images the owner attached to this message.
func imageAuto(text string) []toolCall {
	var out []toolCall
	for _, m := range imgRef.FindAllStringSubmatch(text, 3) {
		b, _ := json.Marshal(toolReq{Name: "image_describe", Path: m[1]})
		out = append(out, toolCall{"image_describe", string(b)})
	}
	return out
}

var browserAsk = regexp.MustCompile(`(?i)trình duyệt|browser`)

// browserAuto: the owner explicitly asked to open a URL in the browser.
func browserAuto(text string) []toolCall {
	if !browserAsk.MatchString(text) {
		return nil
	}
	var out []toolCall
	for _, u := range tools.RequestedURLs(text) {
		if len(out) < 2 {
			b, _ := json.Marshal(toolReq{Name: "browser_open", URL: u})
			out = append(out, toolCall{"browser_open", string(b)})
		}
	}
	return out
}
