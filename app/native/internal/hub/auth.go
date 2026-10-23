package hub

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie = "neko_aurora_session"
	sessionTTL    = 30 * 24 * time.Hour
	codeTTL       = 10 * time.Minute
)

func (h *Hub) sign(parts ...string) string {
	m := hmac.New(sha256.New, h.cfg.Secret)
	m.Write([]byte(strings.Join(parts, "|")))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// session value: uid.gen.exp.sig
func (h *Hub) setSession(w http.ResponseWriter, u User) {
	exp := strconv.FormatInt(h.cfg.Now().Add(sessionTTL).Unix(), 10)
	gen := strconv.Itoa(u.Gen)
	v := u.ID + "." + gen + "." + exp + "." + h.sign("session", u.ID, gen, exp)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: v, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(h.cfg.PublicURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second)})
}

func (h *Hub) user(r *http.Request) (User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return User{}, false
	}
	p := strings.Split(c.Value, ".")
	if len(p) != 4 || subtle.ConstantTimeCompare([]byte(p[3]), []byte(h.sign("session", p[0], p[1], p[2]))) != 1 {
		return User{}, false
	}
	exp, _ := strconv.ParseInt(p[2], 10, 64)
	if h.cfg.Now().Unix() > exp {
		return User{}, false
	}
	u, ok := h.users.get(p[0])
	if !ok || strconv.Itoa(u.Gen) != p[1] {
		return User{}, false
	}
	return u, true
}

func (h *Hub) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- email one-time code + magic link ----

type pending struct {
	codeHash, linkHash string
	exp                time.Time
	tries              int
}

type codeStore struct {
	mu   sync.Mutex
	m    map[string]*pending // email → pending
	sent map[string][]time.Time
}

func newCodes() *codeStore {
	return &codeStore{m: map[string]*pending{}, sent: map[string][]time.Time{}}
}

func hash(s string) string { x := sha256.Sum256([]byte(s)); return hex.EncodeToString(x[:]) }

func validEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || len(s) > 254 || !strings.Contains(s[strings.LastIndexByte(s, '@'):], ".") {
		return "", false
	}
	return s, true
}

func sixDigits() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n.Int64())
}

func (h *Hub) authRoutes() {
	m := h.mux
	m.HandleFunc("GET /app/auth/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"hosted": true, "email": h.cfg.Mail != nil && h.cfg.Mail.Ready(), "google": h.cfg.Google != nil, "apple": h.cfg.Apple != nil, "deployUrl": h.cfg.DeployURL})
	})
	m.HandleFunc("POST /app/auth/email", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email string `json:"email"`
		}
		_ = decode(r, &in)
		email, ok := validEmail(in.Email)
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "invalid_email"})
			return
		}
		if h.cfg.Mail == nil || !h.cfg.Mail.Ready() {
			writeJSON(w, 503, map[string]string{"error": "email_signin_not_configured"})
			return
		}
		now := h.cfg.Now()
		h.codes.mu.Lock()
		recent := h.codes.sent[email][:0]
		for _, t := range h.codes.sent[email] {
			if now.Sub(t) < time.Hour {
				recent = append(recent, t)
			}
		}
		if len(recent) >= 5 || len(recent) > 0 && now.Sub(recent[len(recent)-1]) < 30*time.Second {
			h.codes.sent[email] = recent
			h.codes.mu.Unlock()
			writeJSON(w, 429, map[string]string{"error": "too_many_requests"})
			return
		}
		code, link := sixDigits(), randHex(24)
		h.codes.m[email] = &pending{codeHash: hash(email + code), linkHash: hash(link), exp: now.Add(codeTTL)}
		h.codes.sent[email] = append(recent, now)
		h.codes.mu.Unlock()
		u := h.cfg.PublicURL + "/app/auth/magic?e=" + url.QueryEscape(email) + "&t=" + link
		body := fmt.Sprintf("Mã đăng nhập Neko Aurora của bạn: %s\n\nHoặc mở liên kết này trên thiết bị đang đăng nhập:\n%s\n\nMã hết hạn sau 10 phút. Nếu không phải bạn, hãy bỏ qua thư này.\n\n—\nYour Neko Aurora sign-in code: %s (expires in 10 minutes).", code, u, code)
		if err := h.cfg.Mail.Send(email, "Mã đăng nhập Neko Aurora: "+code, body); err != nil {
			writeJSON(w, 502, map[string]string{"error": "email_send_failed"})
			return
		}
		writeJSON(w, 200, map[string]bool{"sent": true})
	})
	m.HandleFunc("POST /app/auth/verify", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Email, Code string }
		_ = decode(r, &in)
		email, ok := validEmail(in.Email)
		if !ok || !h.consume(email, func(p *pending) bool {
			return subtle.ConstantTimeCompare([]byte(p.codeHash), []byte(hash(email+strings.TrimSpace(in.Code)))) == 1
		}) {
			time.Sleep(300 * time.Millisecond)
			writeJSON(w, 401, map[string]string{"error": "invalid_code"})
			return
		}
		h.finish(w, r, email, "email", "", "", false)
	})
	m.HandleFunc("GET /app/auth/magic", func(w http.ResponseWriter, r *http.Request) {
		email, ok := validEmail(r.URL.Query().Get("e"))
		t := r.URL.Query().Get("t")
		if !ok || !h.consume(email, func(p *pending) bool { return subtle.ConstantTimeCompare([]byte(p.linkHash), []byte(hash(t))) == 1 }) {
			http.Redirect(w, r, "/app/signin.html#expired", http.StatusFound)
			return
		}
		h.finish(w, r, email, "email", "", "", true)
	})
	h.oauthRoutes()
}

// consume checks a pending code once; 5 wrong tries burn it.
func (h *Hub) consume(email string, ok func(*pending) bool) bool {
	h.codes.mu.Lock()
	defer h.codes.mu.Unlock()
	p := h.codes.m[email]
	if p == nil || h.cfg.Now().After(p.exp) {
		delete(h.codes.m, email)
		return false
	}
	if !ok(p) {
		if p.tries++; p.tries >= 5 {
			delete(h.codes.m, email)
		}
		return false
	}
	delete(h.codes.m, email)
	return true
}

func (h *Hub) finish(w http.ResponseWriter, r *http.Request, email, provider, subject, name string, redirect bool) {
	u, err := h.users.signIn(email, provider, subject, name, h.cfg.DefaultPln, h.cfg.MaxUsers, h.cfg.Now())
	if err != nil {
		if redirect {
			http.Redirect(w, r, "/app/signin.html#full", http.StatusFound)
			return
		}
		writeJSON(w, 403, map[string]string{"error": "signups_closed"})
		return
	}
	h.setSession(w, u)
	next := "/app/"
	if !u.Onboarded {
		next = "/app/welcome.html"
	}
	if redirect {
		http.Redirect(w, r, next, http.StatusFound)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "next": next})
}
