package hub

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OAuth is one OpenID Connect provider (Google, Apple). Enabled only when
// its env vars are set; ID tokens are verified (RS256 against the provider
// JWKS, iss, aud, exp, nonce, verified email) before any account is touched.
type OAuth struct {
	Name, ClientID      string
	Secret              func() (string, error)
	AuthURL, TokenURL   string
	JWKSURL             string
	Issuers             []string
	Scope, ResponseMode string
	Redirect            string
	HTTP                *http.Client
	mu                  sync.Mutex
	keys                map[string]*rsa.PublicKey
	keysAt              time.Time
}

func googleFromEnv(pub string) *OAuth {
	id, sec := os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET")
	if id == "" || sec == "" || pub == "" {
		return nil
	}
	return &OAuth{Name: "google", ClientID: id, Secret: func() (string, error) { return sec, nil },
		AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token",
		JWKSURL: "https://www.googleapis.com/oauth2/v3/certs", Issuers: []string{"https://accounts.google.com", "accounts.google.com"},
		Scope: "openid email profile", Redirect: pub + "/app/auth/google/callback"}
}

// Apple: APPLE_CLIENT_ID (Services ID), APPLE_TEAM_ID, APPLE_KEY_ID and
// APPLE_PRIVATE_KEY (the .p8 PEM). The client secret is a short-lived ES256 JWT.
func appleFromEnv(pub string) *OAuth {
	id, team, kid, p8 := os.Getenv("APPLE_CLIENT_ID"), os.Getenv("APPLE_TEAM_ID"), os.Getenv("APPLE_KEY_ID"), os.Getenv("APPLE_PRIVATE_KEY")
	if id == "" || team == "" || kid == "" || p8 == "" || pub == "" {
		return nil
	}
	key, err := parseP8(strings.ReplaceAll(p8, `\n`, "\n"))
	if err != nil {
		return nil
	}
	return &OAuth{Name: "apple", ClientID: id, Secret: func() (string, error) { return appleSecret(key, team, kid, id, time.Now()) },
		AuthURL: "https://appleid.apple.com/auth/authorize", TokenURL: "https://appleid.apple.com/auth/token",
		JWKSURL: "https://appleid.apple.com/auth/keys", Issuers: []string{"https://appleid.apple.com"},
		Scope: "name email", ResponseMode: "form_post", Redirect: pub + "/app/auth/apple/callback"}
}

func parseP8(p string) (*ecdsa.PrivateKey, error) {
	b, _ := pem.Decode([]byte(p))
	if b == nil {
		return nil, errors.New("bad pem")
	}
	k, err := x509.ParsePKCS8PrivateKey(b.Bytes)
	if err != nil {
		return nil, err
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("not ec")
	}
	return ec, nil
}

func b64(v []byte) string { return base64.RawURLEncoding.EncodeToString(v) }

func appleSecret(k *ecdsa.PrivateKey, team, kid, client string, now time.Time) (string, error) {
	hd, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": kid})
	cl, _ := json.Marshal(map[string]any{"iss": team, "iat": now.Unix(), "exp": now.Add(10 * time.Minute).Unix(), "aud": "https://appleid.apple.com", "sub": client})
	in := b64(hd) + "." + b64(cl)
	d := sha256.Sum256([]byte(in))
	r, s, err := ecdsa.Sign(rand.Reader, k, d[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return in + "." + b64(sig), nil
}

func (o *OAuth) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (o *OAuth) jwk(kid string) (*rsa.PublicKey, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if k := o.keys[kid]; k != nil && time.Since(o.keysAt) < time.Hour {
		return k, nil
	}
	res, err := o.client().Get(o.JWKSURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var set struct {
		Keys []struct{ Kid, Kty, N, E string } `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&set); err != nil {
		return nil, err
	}
	o.keys, o.keysAt = map[string]*rsa.PublicKey{}, time.Now()
	for _, k := range set.Keys {
		n, e1 := base64.RawURLEncoding.DecodeString(k.N)
		e, e2 := base64.RawURLEncoding.DecodeString(k.E)
		if k.Kty != "RSA" || e1 != nil || e2 != nil {
			continue
		}
		o.keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if k := o.keys[kid]; k != nil {
		return k, nil
	}
	return nil, errors.New("unknown key")
}

type idClaims struct {
	Iss, Sub, Email, Nonce, Name string
	Aud                          any
	Exp                          int64
	Verified                     any `json:"email_verified"`
}

// verify checks an RS256 ID token and returns its claims.
func (o *OAuth) verify(tok, nonce string, now time.Time) (idClaims, error) {
	var c idClaims
	p := strings.Split(tok, ".")
	if len(p) != 3 {
		return c, errors.New("malformed")
	}
	var hd struct{ Alg, Kid string }
	hb, _ := base64.RawURLEncoding.DecodeString(p[0])
	if json.Unmarshal(hb, &hd) != nil || hd.Alg != "RS256" {
		return c, errors.New("alg")
	}
	k, err := o.jwk(hd.Kid)
	if err != nil {
		return c, err
	}
	sig, _ := base64.RawURLEncoding.DecodeString(p[2])
	d := sha256.Sum256([]byte(p[0] + "." + p[1]))
	if rsa.VerifyPKCS1v15(k, crypto.SHA256, d[:], sig) != nil {
		return c, errors.New("signature")
	}
	cb, _ := base64.RawURLEncoding.DecodeString(p[1])
	if json.Unmarshal(cb, &c) != nil {
		return c, errors.New("claims")
	}
	issOK := false
	for _, i := range o.Issuers {
		issOK = issOK || c.Iss == i
	}
	audOK := c.Aud == o.ClientID
	if l, ok := c.Aud.([]any); ok {
		for _, a := range l {
			audOK = audOK || a == o.ClientID
		}
	}
	verified := c.Verified == true || c.Verified == "true"
	switch {
	case !issOK:
		return c, errors.New("iss")
	case !audOK:
		return c, errors.New("aud")
	case now.Unix() > c.Exp:
		return c, errors.New("expired")
	case subtle.ConstantTimeCompare([]byte(c.Nonce), []byte(nonce)) != 1:
		return c, errors.New("nonce")
	case c.Sub == "":
		return c, errors.New("sub")
	case c.Email != "" && !verified:
		return c, errors.New("email not verified")
	}
	return c, nil
}

func (o *OAuth) exchange(code string) (string, error) {
	sec, err := o.Secret()
	if err != nil {
		return "", err
	}
	f := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {o.Redirect}, "client_id": {o.ClientID}, "client_secret": {sec}}
	res, err := o.client().PostForm(o.TokenURL, f)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil || out.IDToken == "" {
		return "", errors.New("token exchange failed")
	}
	return out.IDToken, nil
}

// state = nonce.exp.sig (signed, so no cookie is needed for Apple's
// cross-site form_post); the nonce is also bound into the ID token.
func (h *Hub) oauthState(provider string) (state, nonce string) {
	nonce = randHex(16)
	exp := strconv.FormatInt(h.cfg.Now().Add(10*time.Minute).Unix(), 10)
	return nonce + "." + exp + "." + h.sign("oauth", provider, nonce, exp), nonce
}

func (h *Hub) checkState(provider, s string) (string, bool) {
	p := strings.Split(s, ".")
	if len(p) != 3 || subtle.ConstantTimeCompare([]byte(p[2]), []byte(h.sign("oauth", provider, p[0], p[1]))) != 1 {
		return "", false
	}
	exp, _ := strconv.ParseInt(p[1], 10, 64)
	return p[0], h.cfg.Now().Unix() <= exp
}

func (h *Hub) oauthRoutes() {
	for _, o := range []*OAuth{h.cfg.Google, h.cfg.Apple} {
		if o == nil {
			continue
		}
		o := o
		h.mux.HandleFunc("GET /app/auth/"+o.Name+"/start", func(w http.ResponseWriter, r *http.Request) {
			st, nonce := h.oauthState(o.Name)
			q := url.Values{"client_id": {o.ClientID}, "redirect_uri": {o.Redirect}, "response_type": {"code"}, "scope": {o.Scope}, "state": {st}, "nonce": {nonce}}
			if o.ResponseMode != "" {
				q.Set("response_mode", o.ResponseMode)
			}
			http.Redirect(w, r, o.AuthURL+"?"+q.Encode(), http.StatusFound)
		})
		cb := func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			nonce, ok := h.checkState(o.Name, r.Form.Get("state"))
			code := r.Form.Get("code")
			if !ok || code == "" {
				http.Redirect(w, r, "/app/signin.html#failed", http.StatusFound)
				return
			}
			tok, err := o.exchange(code)
			var c idClaims
			if err == nil {
				c, err = o.verify(tok, nonce, h.cfg.Now())
			}
			if err != nil {
				http.Redirect(w, r, "/app/signin.html#failed", http.StatusFound)
				return
			}
			name := c.Name
			if u := r.Form.Get("user"); u != "" && name == "" { // Apple sends the name once, as JSON
				var x struct {
					Name struct{ FirstName, LastName string } `json:"name"`
				}
				if json.Unmarshal([]byte(u), &x) == nil {
					name = strings.TrimSpace(x.Name.FirstName + " " + x.Name.LastName)
				}
			}
			h.finish(w, r, c.Email, o.Name, c.Sub, name, true)
		}
		h.mux.HandleFunc("GET /app/auth/"+o.Name+"/callback", cb)
		h.mux.HandleFunc("POST /app/auth/"+o.Name+"/callback", cb)
	}
}
