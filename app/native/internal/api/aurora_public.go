package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"nekoaurora.local/native/internal/web"
)

// auroraPublic exposes ONLY the /app/ surface on an extra HTTPS host (e.g. a
// tunnel) behind an owner passcode. The loopback session, /v1/* and the
// desktop UI are never reachable through this host.
type auroraPublic struct {
	host    string
	code    string
	session string
	mu      sync.Mutex
	fails   int
	lockTil time.Time
}

// EnableAuroraPublic allows host to reach /app/ with owner login by code.
func (s *Server) EnableAuroraPublic(host, code string) {
	m := hmac.New(sha256.New, []byte(code))
	m.Write([]byte("neko-aurora-owner-session-v1"))
	s.auroraPub = &auroraPublic{host: strings.ToLower(host), code: code, session: hex.EncodeToString(m.Sum(nil))}
}

const auroraCookie = "neko_aurora_owner"

func (p *auroraPublic) authed(r *http.Request) bool {
	c, err := r.Cookie(auroraCookie)
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(p.session)) == 1
}

func (s *Server) servePublicAurora(w http.ResponseWriter, r *http.Request) {
	p := s.auroraPub
	w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	path := r.URL.Path
	if path == "/" {
		http.Redirect(w, r, "/app/", http.StatusFound)
		return
	}
	if !strings.HasPrefix(path, "/app/") {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet && web.IsPublicAsset(path) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; font-src 'self'; manifest-src 'self'; worker-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		web.Serve(w, r)
		return
	}
	if o := r.Header.Get("Origin"); o != "" && o != "https://"+p.host {
		http.Error(w, "invalid_origin", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet && r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "json_required", http.StatusUnsupportedMediaType)
		return
	}
	if path == "/app/login" && r.Method == http.MethodPost {
		p.login(w, r)
		return
	}
	if !p.authed(r) {
		http.Error(w, "owner_login_required", http.StatusUnauthorized)
		return
	}
	if s.aurora != nil && strings.HasPrefix(path, "/app/api/") {
		s.aurora.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

func (p *auroraPublic) login(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	if time.Now().Before(p.lockTil) {
		p.mu.Unlock()
		http.Error(w, "too_many_attempts", http.StatusTooManyRequests)
		return
	}
	p.mu.Unlock()
	var in struct {
		Code string `json:"code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	_ = json.NewDecoder(r.Body).Decode(&in)
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(in.Code)), []byte(p.code)) != 1 {
		p.mu.Lock()
		p.fails++
		if p.fails >= 5 {
			p.lockTil, p.fails = time.Now().Add(5*time.Minute), 0
		}
		p.mu.Unlock()
		time.Sleep(400 * time.Millisecond)
		http.Error(w, "invalid_code", http.StatusUnauthorized)
		return
	}
	p.mu.Lock()
	p.fails = 0
	p.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: auroraCookie, Value: p.session, Path: "/app/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 30 * 24 * 3600})
	send(w, 200, map[string]bool{"ok": true})
}
