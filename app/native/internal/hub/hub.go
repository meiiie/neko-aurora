// Package hub is Neko Aurora's hosted multi-user mode (AURORA_MODE=hosted).
//
// Every account gets its own workspace directory (users/<uid>/) holding a
// separate ledger, Aurora service, vault (encrypted with a key derived from
// AURORA_SECRET, never stored on disk) and per-bot computers. Requests are
// routed to the signed-in user's service only; nothing is shared between
// workspaces except the server's z.ai key, which is metered per user plan.
//
// Sign-in: email one-time code + magic link (baseline), Google and Apple
// OAuth when their env vars are set. Sessions are HMAC-signed cookies.
// Single-owner mode (the default) does not use this package.
package hub

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nekoaurora.local/native/internal/aurora"
	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/web"
)

type Config struct {
	Dir        string // state root; users live under Dir/users
	Secret     []byte // AURORA_SECRET (>= 32 bytes)
	PublicURL  string // https://host (links in mail, OAuth redirects, Origin check)
	DeployURL  string // Railway template URL for the "Deploy" button
	Plans      map[string]int
	DefaultPln string
	MaxUsers   int
	Mail       Mailer
	Google     *OAuth
	Apple      *OAuth
	Now        func() time.Time
}

// ConfigFromEnv reads hosted-mode settings; it fails closed without a secret.
func ConfigFromEnv(dir string) (Config, error) {
	sec := os.Getenv("AURORA_SECRET")
	if len(sec) < 32 {
		return Config{}, errors.New("AURORA_SECRET must be at least 32 characters")
	}
	pub := strings.TrimRight(os.Getenv("AURORA_PUBLIC_URL"), "/")
	if pub == "" && os.Getenv("RAILWAY_PUBLIC_DOMAIN") != "" {
		pub = "https://" + os.Getenv("RAILWAY_PUBLIC_DOMAIN")
	}
	c := Config{Dir: dir, Secret: []byte(sec), PublicURL: pub, DeployURL: envOr("AURORA_DEPLOY_URL", DefaultDeployURL),
		Plans:      map[string]int{"free": envInt("AURORA_PLAN_FREE_DAILY", 50), "pro": envInt("AURORA_PLAN_PRO_DAILY", 1000)},
		DefaultPln: envOr("AURORA_DEFAULT_PLAN", "free"), MaxUsers: envInt("AURORA_MAX_USERS", 500), Now: time.Now}
	c.Mail = MailerFromEnv(filepath.Join(dir, "hub", "dev-mail"))
	c.Google = googleFromEnv(pub)
	c.Apple = appleFromEnv(pub)
	return c, nil
}

// DefaultDeployURL is the one-click Railway deploy link (template repo).
const DefaultDeployURL = "https://railway.com/new/template?template=https%3A%2F%2Fgithub.com%2Fmeiiie%2Fneko-aurora"

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envInt(k string, d int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n >= 0 {
		return n
	}
	return d
}

type tenant struct {
	store *ledger.Store
	svc   *aurora.Service
	stop  context.CancelFunc
}

type Hub struct {
	cfg     Config
	ctx     context.Context
	users   *userStore
	mu      sync.Mutex
	tenants map[string]*tenant
	codes   *codeStore
	mux     *http.ServeMux
}

func New(ctx context.Context, cfg Config) (*Hub, error) {
	if len(cfg.Secret) < 32 {
		return nil, errors.New("secret too short")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Plans == nil {
		cfg.Plans = map[string]int{"free": 50, "pro": 1000}
	}
	if cfg.DefaultPln == "" {
		cfg.DefaultPln = "free"
	}
	if cfg.MaxUsers == 0 {
		cfg.MaxUsers = 500
	}
	us, err := openUsers(filepath.Join(cfg.Dir, "hub", "users.json"))
	if err != nil {
		return nil, err
	}
	h := &Hub{cfg: cfg, ctx: ctx, users: us, tenants: map[string]*tenant{}, codes: newCodes(), mux: http.NewServeMux()}
	h.routes()
	return h, nil
}

func (h *Hub) userDir(uid string) string { return filepath.Join(h.cfg.Dir, "users", uid) }

// vaultKey derives the per-user vault key; it exists only in memory.
func (h *Hub) vaultKey(uid string) []byte {
	m := hmac.New(sha256.New, h.cfg.Secret)
	m.Write([]byte("neko-aurora:vault:v1:" + uid))
	return m.Sum(nil)
}

func (h *Hub) tenant(uid string) (*tenant, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t := h.tenants[uid]; t != nil {
		return t, nil
	}
	dir := h.userDir(uid)
	st, err := ledger.Open(dir)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(h.ctx)
	svc, err := aurora.New(ctx, st, filepath.Join(dir, "aurora"), aurora.Options{RunTurns: true, VaultKey: h.vaultKey(uid), NoBriefLoop: true,
		OwnerName: func() string { u, _ := h.users.get(uid); return u.Name }})
	if err != nil {
		cancel()
		st.Close()
		return nil, err
	}
	t := &tenant{store: st, svc: svc, stop: cancel}
	svc.OnDeleteAccount = func() error {
		go func() { time.Sleep(300 * time.Millisecond); h.deleteUser(uid) }()
		return nil
	}
	h.tenants[uid] = t
	return t, nil
}

func (h *Hub) close(uid string) {
	h.mu.Lock()
	t := h.tenants[uid]
	delete(h.tenants, uid)
	h.mu.Unlock()
	if t != nil {
		t.stop()
		time.Sleep(50 * time.Millisecond)
		t.store.Close()
	}
}

// deleteUser erases a workspace and its account (App Store 5.1.1(v)).
func (h *Hub) deleteUser(uid string) {
	h.close(uid)
	if err := os.RemoveAll(h.userDir(uid)); err != nil {
		log.Printf("hub: delete workspace failed: %v", err)
	}
	h.users.remove(uid)
}

// Close stops every tenant (shutdown, tests).
func (h *Hub) Close() {
	h.mu.Lock()
	ids := make([]string, 0, len(h.tenants))
	for id := range h.tenants {
		ids = append(ids, id)
	}
	h.mu.Unlock()
	for _, id := range ids {
		h.close(id)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(v)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const csp = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; font-src 'self'; manifest-src 'self'; worker-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self' https://appleid.apple.com"

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(h.cfg.PublicURL, "https://") {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	p := r.URL.Path
	switch {
	case p == "/healthz":
		writeJSON(w, 200, map[string]any{"ok": true, "mode": "hosted"})
		return
	case p == "/":
		http.Redirect(w, r, "/app/", http.StatusFound)
		return
	case r.Method == http.MethodGet && web.IsPublicAsset(p):
		w.Header().Set("Content-Security-Policy", csp)
		web.Serve(w, r)
		return
	}
	// CSRF: same-origin JSON only for state changes (Apple's form_post
	// callback is the one cross-site POST and carries a signed state).
	if r.Method != http.MethodGet && p != "/app/auth/apple/callback" {
		if o := r.Header.Get("Origin"); o != "" && h.cfg.PublicURL != "" && o != h.cfg.PublicURL {
			http.Error(w, "invalid_origin", http.StatusForbidden)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "json_required", http.StatusUnsupportedMediaType)
			return
		}
	}
	h.mux.ServeHTTP(w, r)
}

func (h *Hub) routes() {
	m := h.mux
	h.authRoutes()
	m.HandleFunc("GET /app/auth/me", func(w http.ResponseWriter, r *http.Request) {
		u, ok := h.user(r)
		if !ok {
			writeJSON(w, 401, map[string]string{"error": "signin_required"})
			return
		}
		writeJSON(w, 200, h.me(u))
	})
	m.HandleFunc("POST /app/api/onboard", func(w http.ResponseWriter, r *http.Request) {
		u, ok := h.user(r)
		if !ok {
			writeJSON(w, 401, map[string]string{"error": "signin_required"})
			return
		}
		var in struct {
			Name string `json:"name"`
			Goal string `json:"goal"`
		}
		_ = decode(r, &in)
		if err := h.onboard(u, in.Name, in.Goal); err != nil {
			writeJSON(w, 500, map[string]string{"error": "onboard_failed"})
			return
		}
		u, _ = h.users.get(u.ID)
		writeJSON(w, 200, h.me(u))
	})
	m.HandleFunc("/app/api/", h.api)
	m.HandleFunc("POST /app/login", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 400, map[string]string{"error": "use_signin"}) // owner-code login is single-owner only
	})
	m.HandleFunc("POST /app/logout", h.logout)
}

func (h *Hub) me(u User) map[string]any {
	used := h.users.usedToday(u.ID, h.cfg.Now())
	return map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "provider": u.Provider, "plan": u.Plan,
		"quota": map[string]int{"daily": h.cfg.Plans[u.Plan], "used": used}, "onboarded": u.Onboarded}
}

// metered is any request that spends model tokens.
func metered(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	p := r.URL.Path
	return strings.HasSuffix(p, "/messages") || p == "/app/api/brief"
}

func (h *Hub) api(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(r)
	if !ok {
		writeJSON(w, 401, map[string]string{"error": "signin_required"})
		return
	}
	if r.URL.Path == "/app/api/plan" {
		writeJSON(w, 200, h.me(u))
		return
	}
	if metered(r) {
		limit := h.cfg.Plans[u.Plan]
		if !h.users.spend(u.ID, h.cfg.Now(), limit) {
			writeJSON(w, 429, map[string]any{"error": "quota_exceeded", "plan": u.Plan, "daily": limit})
			return
		}
	}
	t, err := h.tenant(u.ID)
	if err != nil {
		log.Printf("hub: workspace unavailable: %v", err)
		writeJSON(w, 503, map[string]string{"error": "workspace_unavailable"})
		return
	}
	t.svc.ServeHTTP(w, r)
}

// call drives a tenant's own API in-process (onboarding).
func call(svc http.Handler, method, path string, body any) (int, []byte) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	rec := &recorder{h: http.Header{}, code: 200}
	svc.ServeHTTP(rec, req)
	return rec.code, rec.body
}

type recorder struct {
	h    http.Header
	code int
	body []byte
}

func (r *recorder) Header() http.Header         { return r.h }
func (r *recorder) WriteHeader(c int)           { r.code = c }
func (r *recorder) Write(b []byte) (int, error) { r.body = append(r.body, b...); return len(b), nil }

var goals = map[string][]struct{ Name, Desc string }{
	"work":     {{"Thư ký", "Soạn email, lên lịch, nhắc việc; luôn xin duyệt trước khi gửi"}, {"Phân tích", "Phân tích dữ liệu và file CSV bằng Python trên máy tính riêng"}},
	"research": {{"Nghiên cứu", "Tìm và đọc nguồn trên web, tóm tắt có trích dẫn"}, {"Phân tích", "Phân tích dữ liệu và file CSV bằng Python trên máy tính riêng"}},
	"study":    {{"Gia sư", "Giải thích kiến thức từng bước, ra bài tập và chấm"}, {"Nghiên cứu", "Tìm và đọc nguồn trên web, tóm tắt có trích dẫn"}},
	"money":    {{"Tài chính", "Theo dõi giá vàng, tỷ giá, ngân sách cá nhân; chỉ dùng số liệu có nguồn"}, {"Phân tích", "Phân tích dữ liệu và file CSV bằng Python trên máy tính riêng"}},
}

// onboard is the one-button setup: the main bot becomes "Aurora", the demo
// bot is archived, two helpers matching the goal are created, and Aurora
// greets the user and offers next steps. Idempotent.
func (h *Hub) onboard(u User, name, goal string) error {
	if u.Onboarded {
		return nil
	}
	t, err := h.tenant(u.ID)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 40 {
		name = string([]rune(name)[:40])
	}
	helpers, ok := goals[goal]
	if !ok {
		goal, helpers = "work", goals["work"]
	}
	if c, _ := call(t.svc, "POST", "/app/api/agents/bot-meiiie", map[string]any{"name": "Aurora", "desc": "Trợ lý chính: hiểu bạn cần gì, điều phối các bot khác, giúp bạn thiết lập Neko Aurora"}); c != 200 {
		return fmt.Errorf("rename main bot: %d", c)
	}
	call(t.svc, "POST", "/app/api/agents/bot-davin", map[string]any{"archived": true})
	call(t.svc, "POST", "/app/api/agents/bot-meiiie", map[string]any{"pin": true})
	for i, b := range helpers {
		call(t.svc, "POST", "/app/api/agents", map[string]any{"name": b.Name, "desc": b.Desc, "clientNonce": fmt.Sprintf("onboard-%s-%d", u.ID, i)})
	}
	who := "bạn"
	if name != "" {
		who = name
	}
	msg := fmt.Sprintf("Chào Aurora! Mình là %s, mình muốn dùng Neko Aurora cho %s. Giới thiệu ngắn bạn cùng %s và %s giúp được gì, rồi gợi ý 3 việc đầu tiên mình giao được ngay nhé.", who, map[string]string{"work": "công việc hằng ngày", "research": "nghiên cứu, tìm hiểu", "study": "học tập", "money": "tài chính cá nhân"}[goal], helpers[0].Name, helpers[1].Name)
	if c, _ := call(t.svc, "POST", "/app/api/agents/bot-meiiie/messages", map[string]any{"text": msg, "clientNonce": "onboard-" + u.ID}); c >= 300 {
		return fmt.Errorf("welcome: %d", c)
	}
	return h.users.update(u.ID, func(x *User) {
		x.Onboarded = true
		if name != "" {
			x.Name = name
		}
	})
}
