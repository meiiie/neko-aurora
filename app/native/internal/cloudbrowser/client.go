// Package cloudbrowser connects Aurora to its private browser worker. It never
// starts a local browser or exposes a debugging/profile transport to callers.
package cloudbrowser

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"nekoaurora.local/native/internal/tools"
)

var (
	ErrNotConfigured  = errors.New("cloud_browser_not_configured")
	ErrConfiguration  = errors.New("cloud_browser_invalid_configuration")
	ErrInvalid        = errors.New("cloud_browser_invalid_request")
	ErrUnavailable    = errors.New("cloud_browser_unavailable")
	ErrUnknownOutcome = errors.New("cloud_browser_unknown_outcome")
	ErrConflict       = errors.New("cloud_browser_conflict")
	ErrProtocol       = errors.New("cloud_browser_invalid_response")
	ErrCleanupPending = errors.New("cloud_browser_cleanup_pending")
	identifier        = regexp.MustCompile(`^[A-Za-z0-9_-]{8,120}$`)
	profileID         = regexp.MustCompile(`^bot-[A-Za-z0-9_-]{1,60}$`)
)

const NamespaceFile = "cloud-browser-namespace"

type Config struct {
	LocalDocker bool
	URL         string
	Key         []byte
	StateDir    string
	HTTP        *http.Client
	Now         func() time.Time
}

type Client struct {
	lifecycle   sync.Mutex
	deleting    bool
	purgeDone   bool
	localDocker bool
	endpoint    string
	key         []byte
	namespace   string
	http        *http.Client
	now         func() time.Time
	stateDir    string
}

type Command struct {
	Namespace      string  `json:"namespace"`
	Profile        string  `json:"profile"`
	Kind           string  `json:"kind"`
	Actor          string  `json:"actor"`
	ActorID        string  `json:"actorId,omitempty"`
	WorkID         string  `json:"workId,omitempty"`
	WorkEpoch      uint64  `json:"workEpoch,omitempty"`
	Attempt        string  `json:"attempt,omitempty"`
	Nonce          string  `json:"nonce,omitempty"`
	Epoch          uint64  `json:"epoch,omitempty"`
	ObservationID  string  `json:"observationId,omitempty"`
	FrameID        string  `json:"frameId,omitempty"`
	TargetID       string  `json:"targetId,omitempty"`
	Text           string  `json:"text,omitempty"`
	Key            string  `json:"key,omitempty"`
	X              float64 `json:"x,omitempty"`
	Y              float64 `json:"y,omitempty"`
	DeltaY         float64 `json:"deltaY,omitempty"`
	URL            string  `json:"url,omitempty"`
	Mode           string  `json:"mode,omitempty"`
	ExpectedOrigin string  `json:"expectedOrigin,omitempty"`
	Sealed         string  `json:"sealed,omitempty"`
}

func (cmd Command) MarshalJSON() ([]byte, error) {
	type wire Command
	var text *string
	var x, y *float64
	if cmd.Text != "" || cmd.Kind == "fill" {
		text = &cmd.Text
	}
	if cmd.X != 0 || (cmd.Kind == "click" && cmd.TargetID == "") {
		x = &cmd.X
	}
	if cmd.Y != 0 || (cmd.Kind == "click" && cmd.TargetID == "") {
		y = &cmd.Y
	}
	return json.Marshal(struct {
		wire
		Text *string  `json:"text,omitempty"`
		X    *float64 `json:"x,omitempty"`
		Y    *float64 `json:"y,omitempty"`
	}{wire(cmd), text, x, y})
}

type Target struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Protected bool   `json:"protected"`
}
type Frame struct {
	ID     string `json:"id"`
	MIME   string `json:"mime"`
	Data   string `json:"data"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type Receipt struct {
	Nonce  string `json:"nonce"`
	Status string `json:"status"`
	Code   string `json:"code"`
}
type Response struct {
	ProfileID     string   `json:"profileId"`
	Status        string   `json:"status"`
	Control       string   `json:"control"`
	Epoch         uint64   `json:"epoch"`
	BootID        string   `json:"bootId,omitempty"`
	AuthState     string   `json:"authState"`
	Protected     *bool    `json:"protected"`
	Origin        string   `json:"origin,omitempty"`
	Title         string   `json:"title,omitempty"`
	ObservationID string   `json:"observationId,omitempty"`
	Text          string   `json:"text,omitempty"`
	Targets       []Target `json:"targets,omitempty"`
	Frame         *Frame   `json:"frame,omitempty"`
	Receipt       *Receipt `json:"receipt,omitempty"`
	LastSavedAt   string   `json:"lastSavedAt,omitempty"`
	Error         string   `json:"error,omitempty"`
}
type Credentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	TOTP     string `json:"totp,omitempty"`
}

func New(cfg Config) (*Client, error) {
	if cfg.URL == "" && len(cfg.Key) == 0 && !cfg.LocalDocker {
		return nil, ErrNotConfigured
	}
	u, err := url.Parse(cfg.URL)
	if err != nil || len(cfg.Key) < 32 || cfg.StateDir == "" || u == nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, ErrConfiguration
	}
	host := strings.ToLower(u.Hostname())
	internal := (strings.HasSuffix(host, ".railway.internal") && host != ".railway.internal") ||
		(strings.HasSuffix(host, ".aurora.internal") && host != ".aurora.internal")
	if cfg.LocalDocker {
		port, e := strconv.Atoi(u.Port())
		if u.Scheme != "http" || host != "127.0.0.1" || e != nil || port < 1024 || port > 65535 || strconv.Itoa(port) != u.Port() {
			return nil, ErrConfiguration
		}
	} else if net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || (u.Scheme != "https" && !(u.Scheme == "http" && internal)) || (u.Scheme == "https" && strings.HasSuffix(host, ".internal")) {
		return nil, ErrConfiguration
	}
	client := cfg.HTTP
	if client == nil {
		client = tools.SafeClient(45 * time.Second)
		if internal {
			client = &http.Client{Timeout: 45 * time.Second, Transport: &http.Transport{Proxy: nil}}
		}
		if cfg.LocalDocker {
			pinned := net.JoinHostPort("127.0.0.1", u.Port())
			dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
			client = &http.Client{Timeout: 45 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if addr != pinned || (network != "tcp" && network != "tcp4") {
					return nil, ErrConfiguration
				}
				return dialer.DialContext(ctx, "tcp4", pinned)
			}}}
		}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if copyClient.Timeout == 0 {
		copyClient.Timeout = 45 * time.Second
	}
	raw, err := stableNamespace(cfg.StateDir)
	if err != nil {
		return nil, ErrConfiguration
	}
	m := hmac.New(sha256.New, cfg.Key)
	m.Write([]byte("aurora-cloud-namespace-v1\n" + raw))
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	c := &Client{localDocker: cfg.LocalDocker, endpoint: strings.TrimRight(cfg.URL, "/") + "/v1/command", key: append([]byte(nil), cfg.Key...), namespace: hex.EncodeToString(m.Sum(nil)), http: &copyClient, now: now, stateDir: cfg.StateDir}
	for _, name := range []string{UsedMarkerFile, PurgeMarkerFile} {
		marked, exists, e := readMarker(cfg.StateDir, name)
		if e != nil || (exists && marked != c.namespace) {
			return nil, ErrConfiguration
		}
		if name == PurgeMarkerFile && exists {
			c.deleting = true
		}
	}
	return c, nil
}

func (c *Client) LocalDocker() bool { return c != nil && c.localDocker }

func stableNamespace(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, NamespaceFile)
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		var raw [32]byte
		if _, err = rand.Read(raw[:]); err != nil {
			return "", err
		}
		b = []byte(hex.EncodeToString(raw[:]))
		f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(e) {
			b, err = os.ReadFile(p)
		} else if e != nil {
			return "", e
		} else {
			_, err = f.Write(b)
			if e = f.Close(); err == nil {
				err = e
			}
		}
	}
	if err != nil || len(b) != 64 {
		return "", ErrConfiguration
	}
	if _, err = hex.DecodeString(string(b)); err != nil {
		return "", ErrConfiguration
	}
	return string(b), nil
}

func (c *Client) Namespace() string {
	if c == nil {
		return ""
	}
	return c.namespace
}
func ValidNonce(s string) bool   { return identifier.MatchString(s) }
func ValidProfile(s string) bool { return profileID.MatchString(s) }
func Mutation(kind string) bool  { return kind != "status" && kind != "observe" && kind != "frame" }

func (c *Client) Do(ctx context.Context, cmd Command) (out Response, err error) {
	defer func() {
		if Mutation(cmd.Kind) && errors.Is(err, ErrProtocol) {
			err = ErrUnknownOutcome
		}
	}()
	if c == nil {
		return out, ErrNotConfigured
	}
	c.lifecycle.Lock()
	locked := true
	defer func() {
		if locked {
			c.lifecycle.Unlock()
		}
	}()
	if c.deleting {
		return out, ErrCleanupPending
	}
	if !ValidProfile(cmd.Profile) || (cmd.Actor != "owner" && cmd.Actor != "agent") || cmd.ActorID == "" || len(cmd.ActorID) > 160 {
		return out, ErrInvalid
	}
	switch cmd.Kind {
	case "status", "open", "observe", "frame", "control", "navigate", "click", "fill", "press", "scroll", "close", "vault_fill":
	default:
		return out, ErrInvalid
	}
	if cmd.Actor == "agent" && (cmd.Kind == "frame" || cmd.WorkID == "" || cmd.WorkEpoch == 0 || cmd.Attempt == "") {
		return out, ErrInvalid
	}
	if Mutation(cmd.Kind) && !ValidNonce(cmd.Nonce) {
		return out, ErrInvalid
	}
	cmd.Namespace = c.namespace
	body, err := json.Marshal(cmd)
	if err != nil || len(body) > 1<<20 {
		return out, ErrInvalid
	}
	ts := strconv.FormatInt(c.now().Unix(), 10)
	digest := sha256.Sum256(body)
	m := hmac.New(sha256.New, c.key)
	m.Write([]byte(ts + "\n" + hex.EncodeToString(digest[:])))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return out, ErrConfiguration
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Aurora-Timestamp", ts)
	req.Header.Set("X-Aurora-Signature", hex.EncodeToString(m.Sum(nil)))
	if cmd.Kind == "open" && cmd.Actor == "owner" {
		if err := writeMarker(c.stateDir, UsedMarkerFile, c.namespace); err != nil {
			return out, ErrCleanupPending
		}
	}
	locked = false
	c.lifecycle.Unlock()
	res, err := c.http.Do(req)
	if err != nil {
		if Mutation(cmd.Kind) {
			return out, ErrUnknownOutcome
		}
		return out, ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		if res.StatusCode == 409 {
			return out, ErrConflict
		}
		if res.StatusCode == 400 || res.StatusCode == 403 {
			return out, ErrInvalid
		}
		if Mutation(cmd.Kind) && res.StatusCode >= 500 {
			return out, ErrUnknownOutcome
		}
		return out, ErrUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil || len(b) >= 2<<20 {
		return out, ErrProtocol
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF || len(out.Text) > 16<<10 || len(out.Targets) > 256 {
		return Response{}, ErrProtocol
	}
	if out.ProfileID != cmd.Profile || out.Epoch == 0 || out.Protected == nil {
		return Response{}, ErrProtocol
	}
	switch out.Status {
	case "stopped", "ready", "starting", "recovery_required", "unavailable":
	default:
		return Response{}, ErrProtocol
	}
	switch out.Control {
	case "paused", "owner", "agent":
	default:
		return Response{}, ErrProtocol
	}
	switch out.AuthState {
	case "login_required", "reauth_required", "authenticated", "unknown":
	default:
		return Response{}, ErrProtocol
	}
	if out.Receipt != nil {
		switch out.Receipt.Status {
		case "dispatched", "rejected", "unknown":
		default:
			return Response{}, ErrProtocol
		}
	}
	if out.Frame != nil {
		if out.Frame.MIME != "image/png" && out.Frame.MIME != "image/jpeg" {
			return Response{}, ErrProtocol
		}
		decoded, e := base64.StdEncoding.DecodeString(out.Frame.Data)
		if e != nil || len(decoded) > 1<<20 {
			return Response{}, ErrProtocol
		}
	}
	if cmd.Actor == "agent" {
		out.Frame = nil
		if out.Status != "ready" || out.Control != "agent" || *out.Protected || out.AuthState == "login_required" || out.AuthState == "reauth_required" {
			out.Text = ""
			out.Targets = nil
			out.Title = ""
		} else {
			for i := range out.Targets {
				if out.Targets[i].Protected {
					out.Targets[i].Name = ""
				}
			}
		}
	}
	return out, nil
}

func (c *Client) Seal(cmd Command, fields Credentials) (string, error) {
	if c == nil {
		return "", ErrNotConfigured
	}
	if !ValidProfile(cmd.Profile) || !ValidNonce(cmd.Nonce) || cmd.ObservationID == "" || cmd.ExpectedOrigin == "" {
		return "", ErrInvalid
	}
	b, err := json.Marshal(fields)
	if err != nil || len(b) > 32<<10 {
		return "", ErrInvalid
	}
	m := hmac.New(sha256.New, c.key)
	m.Write([]byte("aurora-cloud-vault-v1\n" + c.namespace + "\n" + cmd.Profile))
	blk, _ := aes.NewCipher(m.Sum(nil))
	a, _ := cipher.NewGCM(blk)
	iv := make([]byte, a.NonceSize())
	if _, err = rand.Read(iv); err != nil {
		return "", ErrUnavailable
	}
	aad := []byte(c.namespace + "\n" + cmd.Profile + "\n" + cmd.Nonce + "\n" + cmd.ExpectedOrigin + "\n" + cmd.ObservationID)
	return base64.StdEncoding.EncodeToString(a.Seal(iv, iv, b, aad)), nil
}

func (c *Client) SignProposal(body []byte) string {
	if c == nil {
		return ""
	}
	m := hmac.New(sha256.New, c.key)
	m.Write([]byte("aurora-cloud-proposal-v1\n" + c.namespace + "\n"))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}
func (c *Client) VerifyProposal(body []byte, signature string) bool {
	return c != nil && signature != "" && hmac.Equal([]byte(c.SignProposal(body)), []byte(signature))
}

// PrepareVault retains the exact encrypted payload under its nonce. A transport
// failure or duplicate approval must not produce a different IV/payload.
func (c *Client) PrepareVault(cmd Command, fields Credentials, binding string) (Command, error) {
	if c == nil {
		return Command{}, ErrNotConfigured
	}
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	if c.deleting {
		return Command{}, ErrCleanupPending
	}
	if cmd.Kind != "vault_fill" || !ValidProfile(cmd.Profile) || !ValidNonce(cmd.Nonce) {
		return Command{}, ErrInvalid
	}
	raw, _ := json.Marshal(cmd)
	digest := sha256.Sum256(append(raw, []byte("\n"+binding)...))
	key := sha256.Sum256([]byte(cmd.Profile + "\n" + cmd.Nonce))
	dir := filepath.Join(c.stateDir, "cloud-browser-pending")
	if os.MkdirAll(dir, 0o700) != nil {
		return Command{}, ErrUnavailable
	}
	p := filepath.Join(dir, hex.EncodeToString(key[:])+".json")
	type pending struct {
		Binding string  `json:"binding"`
		Command Command `json:"command"`
	}
	read := func() (Command, error) {
		b, e := os.ReadFile(p)
		var old pending
		if e != nil || len(b) > 64<<10 || json.Unmarshal(b, &old) != nil {
			return Command{}, ErrUnavailable
		}
		if old.Binding != hex.EncodeToString(digest[:]) {
			return Command{}, ErrConflict
		}
		return old.Command, nil
	}
	if _, err := os.Stat(p); err == nil {
		return read()
	}
	sealed, err := c.Seal(cmd, fields)
	if err != nil {
		return Command{}, err
	}
	cmd.Sealed = sealed
	b, _ := json.Marshal(pending{hex.EncodeToString(digest[:]), cmd})
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return read()
	}
	if err != nil {
		return Command{}, ErrUnavailable
	}
	_, err = f.Write(b)
	if e := f.Sync(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return Command{}, ErrUnavailable
	}
	return cmd, nil
}
