package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/privatefs"
)

func TestQuickSetupReadsActualCoreCatalog(t *testing.T) {
	if findCoreBun() == "" {
		t.Skip("Bun unavailable; Core catalog integration not exercised")
	}
	root, err := filepath.Abs("../../../../core")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEKO_CORE_DIR", root)
	t.Setenv("AURORA_RUNTIME", "core")
	q := openQuickSetup(context.Background(), t.TempDir())
	if q == nil {
		t.Fatal("actual Core catalog unavailable")
	}
	p, ok := q.provider("zai")
	if !ok || p.Auth != "api_key" || len(p.Models) == 0 || p.KeyEnv != "ZAI_API_KEY" {
		t.Fatal("Core GLM preset missing")
	}
	p, ok = q.provider("chatgpt")
	if !ok || p.Auth != "chatgpt_oauth" || q.ChatGPT {
		t.Fatal("Core OAuth preset missing or personal home read")
	}
}

func TestQuickBunUsesExecutableBehindNpmShim(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows npm layout")
	}
	dir := t.TempDir()
	t.Setenv("NEKO_BUN_PATH", "")
	t.Setenv("BUN_INSTALL", "")
	t.Setenv("USERPROFILE", dir)
	t.Setenv("PATH", dir)
	if err := os.WriteFile(filepath.Join(dir, "bun.cmd"), []byte("fixture shim"), 0600); err != nil {
		t.Fatal(err)
	}
	if findCoreBun() != "" {
		t.Fatal("shell shim selected")
	}
	exe := filepath.Join(dir, "node_modules", "bun", "bin", "bun.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("fixture executable path only"), 0600); err != nil {
		t.Fatal(err)
	}
	if findCoreBun() != exe {
		t.Fatal("npm Bun executable not found")
	}
	t.Setenv("NEKO_BUN_PATH", "relative/bun.exe")
	if findCoreBun() != "" {
		t.Fatal("relative override accepted")
	}
}

func TestQuickModelReadinessAndMetadata(t *testing.T) {
	empty := &Service{}
	if empty.modelConfigured("") {
		t.Fatal("unconfigured service reported ready")
	}
	s, srv := quickFixture(t)
	quickPost(t, srv, "", map[string]any{"workspaceId": s.workspaceID, "provider": "zai", "model": "glm-fixture", "key": "fixture-status", "expectedSelection": ""}, 200)
	if !s.modelConfigured(firstBot(s)) {
		t.Fatal("default not used by bot utilities")
	}
	view := s.aiView()
	if view["provider"] != "GLM Coding Plan" || view["runtime"] != "neko-core-cli" || view["model"] != "glm-fixture" {
		t.Fatal("default metadata does not describe selected Core route")
	}
	if err := s.V.Delete(providerKey("core-zai")); err != nil {
		t.Fatal(err)
	}
	if s.modelConfigured(firstBot(s)) {
		t.Fatal("deleted key silently fell back to server provider")
	}
}

func quickFixture(t *testing.T) (*Service, *httptest.Server) {
	t.Helper()
	t.Setenv("AURORA_RUNTIME", "http")
	s, _ := newSvc(t)
	exe, _ := os.Executable()
	s.quick = &quickSetup{Core: &Core{Bun: exe, Entry: "aurora-quick-fixture", Home: t.TempDir()}, Providers: []quickProvider{
		{ID: "zai", Label: "GLM Coding Plan", Auth: "api_key", Model: "glm-fixture", Models: []string{"glm-fixture"}, KeyEnv: "ZAI_API_KEY"},
		{ID: "zai-openai", Label: "GLM API", Auth: "api_key", Model: "glm-api-fixture", Models: []string{"glm-api-fixture"}, KeyEnv: "ZAI_API_KEY"},
		{ID: "chatgpt", Label: "ChatGPT", Auth: "chatgpt_oauth", Model: "gpt-fixture", Models: []string{"gpt-fixture"}},
	}}
	if err := privatefs.ProtectDirectory(s.quick.Core.Home); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return s, srv
}
func quickPost(t *testing.T, srv *httptest.Server, path string, body any, code int) []byte {
	t.Helper()
	b, _ := json.Marshal(body)
	r := post(t, srv.URL+"/app/api/quick-setup"+path, string(b))
	defer r.Body.Close()
	out, _ := io.ReadAll(r.Body)
	if r.StatusCode != code {
		t.Fatalf("status %d, wanted %d", r.StatusCode, code)
	}
	return out
}
func TestQuickSetupSavesVaultAndSelectsOnlyTarget(t *testing.T) {
	s, srv := quickFixture(t)
	bot := firstBot(s)
	in := map[string]any{"workspaceId": s.workspaceID, "botId": bot, "provider": "zai", "model": "glm-fixture", "key": "fixture-quick-private", "expectedSelection": ""}
	out := quickPost(t, srv, "", in, 200)
	if strings.Contains(string(out), "fixture-quick-private") || !s.V.Has(providerKey("core-zai")) || s.ext.bot(bot).Model != "core-zai:glm-fixture" || s.quickSelection("") != "" {
		t.Fatal("key isolation or target selection failed")
	}
	if len(s.L.Snapshot().Work) != 0 {
		t.Fatal("setup must not submit work")
	}
	c := s.quickModel(s.ext.bot(bot).Model)
	if c == nil || c.Profile != "zai" || c.KeyEnv != "ZAI_API_KEY" || !c.Quick {
		t.Fatal("Core route not selected")
	}
	answer, err := c.Stream(context.Background(), []ChatMsg{{"user", "fixture"}}, func(string) {})
	if err != nil || answer != "Core fixture zai" {
		t.Fatalf("Core command failed: %v", err)
	}
	fresh := openExt(filepath.Dir(s.ext.path))
	if fresh.bot(bot).Model != s.ext.bot(bot).Model {
		t.Fatal("selection lost after reload")
	}
	in["key"] = "fixture-replacement"
	in["provider"] = "zai-openai"
	in["model"] = "glm-api-fixture"
	quickPost(t, srv, "", in, 409)
	if s.V.Has(providerKey("core-zai-openai")) {
		t.Fatal("stale selection wrote key")
	}
	in["expectedSelection"] = s.ext.bot(bot).Model
	in["workspaceId"] = "other-workspace"
	quickPost(t, srv, "", in, 409)
	if s.V.Has(providerKey("core-zai-openai")) {
		t.Fatal("foreign workspace wrote key")
	}
}
func TestQuickSetupRejectsInvalidAndSupportsDefault(t *testing.T) {
	s, srv := quickFixture(t)
	in := map[string]any{"workspaceId": s.workspaceID, "provider": "zai", "model": "glm-fixture", "expectedSelection": ""}
	quickPost(t, srv, "", in, 400)
	in["key"] = "fixture-private"
	in["model"] = "not-listed"
	quickPost(t, srv, "", in, 400)
	in["model"] = "glm-fixture"
	in["botId"] = "missing-bot"
	quickPost(t, srv, "", in, 400)
	delete(in, "botId")
	quickPost(t, srv, "", in, 200)
	if s.quickSelection("") != "core-zai:glm-fixture" {
		t.Fatal("default not persisted")
	}
	out, used, err := s.streamExt(context.Background(), firstBot(s), "chat", []ChatMsg{{"user", "fixture"}}, func(string) {})
	if !used || err != nil || out != "Core fixture zai" {
		t.Fatal("default Core route not used")
	}
	in["provider"] = "chatgpt"
	in["model"] = "gpt-fixture"
	in["expectedSelection"] = s.quickSelection("")
	quickPost(t, srv, "", in, 400)
	delete(in, "key")
	quickPost(t, srv, "", in, 400)
	s.quick.mu.Lock()
	s.quick.ChatGPT = true
	s.quick.mu.Unlock()
	quickPost(t, srv, "", in, 200)
	c := s.quickModel(s.quickSelection(""))
	if c == nil || c.key != "" || !c.SignedIn || c.Profile != "chatgpt" {
		t.Fatal("ChatGPT must not use API credentials")
	}
	out, err = c.Stream(context.Background(), []ChatMsg{{"user", "fixture"}}, func(string) {})
	if err != nil || out != "Core fixture chatgpt" {
		t.Fatal("ChatGPT Core route failed")
	}
	s.quick.mu.Lock()
	s.quick.ChatGPT = false
	s.quick.mu.Unlock()
	_, used, err = s.streamExt(context.Background(), firstBot(s), "chat", nil, func(string) {})
	if !used || !errors.Is(err, ErrNoAI) {
		t.Fatal("missing OAuth must fail closed, no API fallback")
	}
}
func quickLoginState(t *testing.T, srv *httptest.Server, id, want string) quickLogin {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r, e := http.Get(srv.URL + "/app/api/quick-setup/login/" + id)
		if e != nil {
			t.Fatal(e)
		}
		var x quickLogin
		if r.Header.Get("Cache-Control") != "no-store" {
			r.Body.Close()
			t.Fatal("OAuth status must not be cached")
		}
		json.NewDecoder(r.Body).Decode(&x)
		r.Body.Close()
		if x.State == want {
			return x
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("login did not reach %s", want)
	return quickLogin{}
}
func TestQuickLoginLinkCancelAndNoLateSuccess(t *testing.T) {
	s, srv := quickFixture(t)
	done := make(chan struct{})
	s.quick.runLogin = func(ctx context.Context, emit func(quickLogin)) error {
		emit(quickLogin{State: "waiting", URL: "https://auth.openai.com/oauth/authorize?state=fixture"})
		<-ctx.Done()
		emit(quickLogin{State: "connected"})
		close(done)
		return nil
	}
	in := map[string]string{"workspaceId": s.workspaceID}
	var attempt quickLogin
	json.Unmarshal(quickPost(t, srv, "/login", in, 202), &attempt)
	waiting := quickLoginState(t, srv, attempt.ID, "waiting")
	if waiting.URL == "" {
		t.Fatal("missing safe URL")
	}
	var repeated quickLogin
	json.Unmarshal(quickPost(t, srv, "/login", in, 200), &repeated)
	if repeated.ID != attempt.ID {
		t.Fatal("duplicate login process")
	}
	quickPost(t, srv, "/login/"+attempt.ID+"/cancel", map[string]string{"workspaceId": "other"}, 409)
	quickPost(t, srv, "/login/"+attempt.ID+"/cancel", in, 200)
	<-done
	cancelled := quickLoginState(t, srv, attempt.ID, "cancelled")
	if cancelled.URL != "" {
		t.Fatal("cancelled URL retained")
	}
	s.quick.mu.Lock()
	ready := s.quick.ChatGPT
	s.quick.mu.Unlock()
	if ready {
		t.Fatal("late success accepted after cancel")
	}
}
func TestQuickLoginCompletionAndFailure(t *testing.T) {
	for _, ok := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "success"}[ok], func(t *testing.T) {
			s, srv := quickFixture(t)
			s.quick.runLogin = func(ctx context.Context, emit func(quickLogin)) error {
				emit(quickLogin{State: "waiting", URL: "https://untrusted.test/oauth/authorize"})
				if ok {
					emit(quickLogin{State: "connected"})
					return nil
				}
				return errors.New("fixture-private-error")
			}
			var x quickLogin
			json.Unmarshal(quickPost(t, srv, "/login", map[string]string{"workspaceId": s.workspaceID}, 202), &x)
			end := quickLoginState(t, srv, x.ID, map[bool]string{true: "connected", false: "failed"}[ok])
			if end.URL != "" {
				t.Fatal("untrusted/finished link exposed")
			}
			s.quick.mu.Lock()
			ready := s.quick.ChatGPT
			s.quick.mu.Unlock()
			if ready != ok {
				t.Fatal("incorrect credential readiness")
			}
			if s.quickSelection("") != "" || len(s.L.Snapshot().Work) != 0 {
				t.Fatal("sign-in alone activated model or sent work")
			}
		})
	}
}
func TestQuickSetupUnavailableAndURLBoundary(t *testing.T) {
	s, srv := quickFixture(t)
	s.quick = nil
	quickPost(t, srv, "", map[string]string{"workspaceId": s.workspaceID}, 503)
	for _, raw := range []string{"https://auth.openai.com.evil.test/oauth/authorize", "https://user@auth.openai.com/oauth/authorize", "http://auth.openai.com/oauth/authorize", "https://auth.openai.com/other"} {
		if validQuickLoginURL(raw) {
			t.Fatal("untrusted auth URL accepted")
		}
	}
}
