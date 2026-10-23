package hub

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeMail struct {
	mu   sync.Mutex
	last map[string]string
}

func (f *fakeMail) Ready() bool { return true }
func (f *fakeMail) Send(to, subject, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last[to] = body
	return nil
}

var aiBodies struct {
	sync.Mutex
	l []string
}

func fakeAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		aiBodies.Lock()
		aiBodies.l = append(aiBodies.l, string(b))
		aiBodies.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Chào bạn, mình là Aurora.\"}}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ZAI_API_KEY", "server-key")
	t.Setenv("AURORA_AI_BASE_URL", srv.URL)
	t.Setenv("AURORA_COMPUTER", "off")
}

func newHub(t *testing.T, mut func(*Config)) (*Hub, *httptest.Server, *fakeMail) {
	fakeAI(t)
	fm := &fakeMail{last: map[string]string{}}
	cfg := Config{Dir: t.TempDir(), Secret: []byte(strings.Repeat("s", 40)), Mail: fm, Plans: map[string]int{"free": 3, "pro": 100}, DefaultPln: "free", MaxUsers: 10}
	if mut != nil {
		mut(&cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); cancel() })
	return h, srv, fm
}

type client struct {
	t   *testing.T
	c   *http.Client
	url string
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	j, _ := cookiejar.New(nil)
	return &client{t, &http.Client{Jar: j, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, srv.URL}
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, c.url+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	b, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]any{"raw": string(b)}
	}
	return res.StatusCode, out
}

var codeRe = regexp.MustCompile(`: (\d{6})\n`)

func signIn(t *testing.T, c *client, fm *fakeMail, email string) {
	t.Helper()
	if s, out := c.do("POST", "/app/auth/email", map[string]string{"email": email}); s != 200 {
		t.Fatal(s, out)
	}
	fm.mu.Lock()
	code := codeRe.FindStringSubmatch(fm.last[email])[1]
	fm.mu.Unlock()
	if s, _ := c.do("POST", "/app/auth/verify", map[string]string{"email": email, "code": "000000x"}); s != 401 {
		t.Fatal("wrong code accepted")
	}
	s, out := c.do("POST", "/app/auth/verify", map[string]string{"email": email, "code": code})
	if s != 200 || out["next"] == nil {
		t.Fatal(s, out)
	}
	if s, _ := c.do("POST", "/app/auth/verify", map[string]string{"email": email, "code": code}); s != 401 {
		t.Fatal("code reused")
	}
}

func bots(c *client) []string {
	_, st := c.do("GET", "/app/api/state", nil)
	var names []string
	for _, a := range st["agents"].([]any) {
		names = append(names, a.(map[string]any)["name"].(string))
	}
	return names
}

func TestSignupIsolationQuotaOnboarding(t *testing.T) {
	h, srv, fm := newHub(t, nil)
	a, b := newClient(t, srv), newClient(t, srv)
	if s, _ := a.do("GET", "/app/api/state", nil); s != 401 {
		t.Fatal("api without session")
	}
	signIn(t, a, fm, "an@example.com")
	signIn(t, b, fm, "binh@example.com")
	// A creates a bot; B never sees it
	if s, out := a.do("POST", "/app/api/agents", map[string]any{"name": "Riêng của An", "clientNonce": "x1"}); s != 201 {
		t.Fatal(s, out)
	}
	if n := strings.Join(bots(b), ","); strings.Contains(n, "Riêng của An") {
		t.Fatal("workspace leak", n)
	}
	if !strings.Contains(strings.Join(bots(a), ","), "Riêng của An") {
		t.Fatal("bot missing")
	}
	// vault is encrypted with a derived key: no key file on disk
	_, me := a.do("GET", "/app/auth/me", nil)
	uid := me["id"].(string)
	if s, out := a.do("POST", "/app/api/secrets", map[string]any{"name": "TEST_TOKEN", "value": "v-123-very-secret"}); s >= 300 {
		t.Fatal("secret save", s, out)
	}
	filepath.Walk(h.userDir(uid), func(p string, i os.FileInfo, err error) error {
		if err == nil && i.Name() == "vault.key" {
			t.Fatal("vault.key written in hosted mode")
		}
		if err == nil && !i.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "v-123-very-secret") {
				t.Fatal("plaintext secret on disk: " + p)
			}
		}
		return nil
	})
	// onboarding: Aurora + helpers, demo bot archived, welcome turn queued
	if s, out := b.do("POST", "/app/api/onboard", map[string]string{"name": "Bình", "goal": "research"}); s != 200 || out["onboarded"] != true {
		t.Fatal(s, out)
	}
	n := strings.Join(bots(b), ",")
	if !strings.Contains(n, "Aurora") || !strings.Contains(n, "Nghiên cứu") || strings.Contains(n, "Davin") || strings.Contains(n, "Meiiie") {
		t.Fatal(n)
	}
	for i := 0; i < 100; i++ {
		aiBodies.Lock()
		n := len(aiBodies.l)
		aiBodies.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	aiBodies.Lock()
	all := strings.Join(aiBodies.l, "")
	aiBodies.Unlock()
	if !strings.Contains(all, "Bình") || strings.Contains(all, "Darren") {
		t.Fatal("prompt not personalised for the workspace owner")
	}
	// quota: free = 3 metered sends per day
	codes := []int{}
	for i := 0; i < 4; i++ {
		s, _ := a.do("POST", "/app/api/agents/bot-meiiie/messages", map[string]any{"text": "chào", "clientNonce": fmt.Sprint("q", i)})
		codes = append(codes, s)
	}
	if codes[3] != 429 || codes[0] >= 300 {
		t.Fatal(codes)
	}
	_, p := a.do("GET", "/app/api/plan", nil)
	if q := p["quota"].(map[string]any); q["used"].(float64) != 3 || q["daily"].(float64) != 3 {
		t.Fatal(p)
	}
	// tampered session
	u, _ := url.Parse(srv.URL)
	for _, ck := range a.c.Jar.Cookies(u) {
		ck.Value = strings.Replace(ck.Value, uid, "u0000000000000000000", 1)
		a.c.Jar.SetCookies(u, []*http.Cookie{ck})
	}
	if s, _ := a.do("GET", "/app/api/state", nil); s != 401 {
		t.Fatal("tampered session accepted")
	}
	// CSRF: cross-origin and non-JSON writes refused
	req, _ := http.NewRequest("POST", srv.URL+"/app/auth/email", strings.NewReader("email=x@y.z"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != 415 {
		t.Fatal("form post accepted", r.StatusCode)
	}
	// account deletion wipes the workspace
	if s, _ := b.do("POST", "/app/api/account/delete", map[string]string{"confirm": "XOÁ"}); s != 202 {
		t.Fatal(s)
	}
	_, mb := b.do("GET", "/app/auth/me", nil)
	_ = mb
	time.Sleep(600 * time.Millisecond)
	if s, _ := b.do("GET", "/app/api/state", nil); s != 401 {
		t.Fatal("deleted account still signed in")
	}
}

func TestMagicLinkAndLimits(t *testing.T) {
	_, srv, fm := newHub(t, func(c *Config) { c.MaxUsers = 1 })
	a := newClient(t, srv)
	a.do("POST", "/app/auth/email", map[string]string{"email": "an@example.com"})
	if s, _ := a.do("POST", "/app/auth/email", map[string]string{"email": "an@example.com"}); s != 429 {
		t.Fatal("resend not throttled")
	}
	link := regexp.MustCompile(`https?://\S*/app/auth/magic\?\S+`).FindString(fm.last["an@example.com"])
	if link == "" {
		link = regexp.MustCompile(`/app/auth/magic\?\S+`).FindString(fm.last["an@example.com"])
	}
	req, _ := http.NewRequest("GET", srv.URL+link[strings.Index(link, "/app/auth/magic"):], nil)
	res, _ := a.c.Do(req)
	if res.StatusCode != 302 || res.Header.Get("Location") != "/app/welcome.html" {
		t.Fatal(res.StatusCode, res.Header.Get("Location"))
	}
	if s, _ := a.do("GET", "/app/auth/me", nil); s != 200 {
		t.Fatal("magic link did not sign in")
	}
	// second account over MaxUsers
	b := newClient(t, srv)
	b.do("POST", "/app/auth/email", map[string]string{"email": "b@example.com"})
	code := codeRe.FindStringSubmatch(fm.last["b@example.com"])[1]
	if s, _ := b.do("POST", "/app/auth/verify", map[string]string{"email": "b@example.com", "code": code}); s != 403 {
		t.Fatal("user cap not enforced", s)
	}
	if s, out := b.do("GET", "/app/auth/config", nil); s != 200 || out["google"] != false || out["email"] != true {
		t.Fatal(out)
	}
}

func TestGoogleOAuthVerifiesIDToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	var nonce string
	b64 := base64.RawURLEncoding.EncodeToString
	mk := func(claims map[string]any) string {
		hd := b64([]byte(`{"alg":"RS256","kid":"k1"}`))
		cb, _ := json.Marshal(claims)
		in := hd + "." + b64(cb)
		d := sha256.Sum256([]byte(in))
		sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, d[:])
		return in + "." + b64(sig)
	}
	var claims map[string]any
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/certs":
			json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kid": "k1", "kty": "RSA", "n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes())}}})
		case "/token":
			r.ParseForm()
			if r.Form.Get("client_secret") != "gsecret" || r.Form.Get("code") != "c1" {
				w.WriteHeader(400)
				return
			}
			claims["nonce"] = nonce
			json.NewEncoder(w).Encode(map[string]string{"id_token": mk(claims)})
		}
	}))
	defer idp.Close()
	var srvURL string
	h, srv, _ := newHub(t, func(c *Config) {
		c.Google = &OAuth{Name: "google", ClientID: "cid", Secret: func() (string, error) { return "gsecret", nil }, AuthURL: "https://accounts.example/auth", TokenURL: "", JWKSURL: "", Issuers: []string{"https://accounts.google.com"}, Scope: "openid email"}
	})
	srvURL = srv.URL
	h.cfg.Google.TokenURL, h.cfg.Google.JWKSURL, h.cfg.Google.Redirect = idp.URL+"/token", idp.URL+"/certs", srvURL+"/app/auth/google/callback"
	c := newClient(t, srv)
	run := func(cl map[string]any) string {
		claims = cl
		res, _ := c.c.Get(srv.URL + "/app/auth/google/start")
		loc, _ := url.Parse(res.Header.Get("Location"))
		nonce = loc.Query().Get("nonce")
		res, _ = c.c.Get(srv.URL + "/app/auth/google/callback?code=c1&state=" + url.QueryEscape(loc.Query().Get("state")))
		return res.Header.Get("Location")
	}
	exp := time.Now().Add(time.Hour).Unix()
	if l := run(map[string]any{"iss": "https://accounts.google.com", "aud": "other", "sub": "g1", "email": "g@example.com", "email_verified": true, "exp": exp}); l != "/app/signin.html#failed" {
		t.Fatal("wrong aud accepted", l)
	}
	if l := run(map[string]any{"iss": "https://accounts.google.com", "aud": "cid", "sub": "g1", "email": "g@example.com", "email_verified": false, "exp": exp}); l != "/app/signin.html#failed" {
		t.Fatal("unverified email accepted", l)
	}
	if l := run(map[string]any{"iss": "https://accounts.google.com", "aud": "cid", "sub": "g1", "email": "g@example.com", "email_verified": true, "exp": exp}); l != "/app/welcome.html" {
		t.Fatal("valid token refused", l)
	}
	if s, me := c.do("GET", "/app/auth/me", nil); s != 200 || me["email"] != "g@example.com" {
		t.Fatal(me)
	}
	res, _ := c.c.Get(srv.URL + "/app/auth/google/callback?code=c1&state=forged.1.x")
	if res.Header.Get("Location") != "/app/signin.html#failed" {
		t.Fatal("forged state")
	}
}

func TestAppleClientSecret(t *testing.T) {
	// ES256 secret shape (3 parts, 64-byte signature)
	k, _ := parseP8(testP8)
	s, err := appleSecret(k, "TEAM", "KID", "com.example", time.Now())
	if err != nil || len(strings.Split(s, ".")) != 3 {
		t.Fatal(s, err)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(strings.Split(s, ".")[2])
	if len(sig) != 64 {
		t.Fatal(len(sig))
	}
}
