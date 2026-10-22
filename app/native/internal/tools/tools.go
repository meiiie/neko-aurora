// Package tools gives bots read-only, server-side data tools: a live gold
// price feed, web search and an allowlisted web fetch. Every result carries
// its sources so answers can cite them; tools never send or post anything.
// Fetches are HTTPS-only to allowlisted public hosts, refuse private/loopback
// addresses (SSRF), cap body size and time out quickly.
package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	At    string `json:"at,omitempty"`
}

type Result struct {
	Name    string
	Arg     string
	Text    string // compact data for the model
	Summary string // one line for the UI card
	Sources []Source
}

var (
	ErrDenied  = errors.New("host_not_allowed")
	ErrUnknown = errors.New("unknown_tool")
)

// DefaultAllow lists public hosts web_fetch may read (subdomains included).
var DefaultAllow = []string{"sjc.com.vn", "giavang.com.vn", "doji.vn", "pnj.com.vn", "btmc.vn", "kitco.com", "gold.org", "reuters.com", "vnexpress.net", "cafef.vn", "vietnamnet.vn", "tuoitre.vn", "thanhnien.vn", "24h.com.vn", "vneconomy.vn", "sbv.gov.vn", "investing.com", "gold-api.com", "finance.yahoo.com", "wikipedia.org", "duckduckgo.com"}

type Kit struct {
	Allow  []string
	Client *http.Client
	Now    func() time.Time
	// Base URLs, overridable in tests.
	GoldAPI, Yahoo, SJC, Search string
}

func blockPrivate(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip != nil && fakeIPEgress && fakeIPNet.Contains(ip) {
		return nil // sandbox egress tunnel answers DNS with fake IPs in 198.18/15
	}
	if ip == nil || !isPublicAddress(ip) {
		return ErrDenied
	}
	return nil
}

// AURORA_EGRESS_FAKEIP=1 is for hosts whose egress proxy resolves every name
// to 198.18.0.0/15 (fake-IP tunnels). Only that range is relaxed, and the
// hostname allowlist still applies.
var (
	fakeIPEgress    = os.Getenv("AURORA_EGRESS_FAKEIP") == "1"
	_, fakeIPNet, _ = net.ParseCIDR("198.18.0.0/15")
)

// Short reports a tool error without echoing URLs or response bodies.
func Short(err error) string {
	var ne net.Error
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrDenied):
		return "địa chỉ bị chặn"
	case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout():
		return "hết thời gian chờ"
	case strings.HasPrefix(err.Error(), "http_"):
		return "máy chủ nguồn trả " + strings.TrimPrefix(err.Error(), "http_")
	case strings.Contains(err.Error(), "unavailable") || strings.Contains(err.Error(), "no_results"):
		return "nguồn không trả dữ liệu"
	}
	return "lỗi mạng"
}

func New() *Kit {
	allow := append([]string(nil), DefaultAllow...)
	if v := os.Getenv("AURORA_FETCH_ALLOW"); v != "" {
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(strings.ToLower(h)); h != "" {
				allow = append(allow, h)
			}
		}
	}
	k := &Kit{Allow: allow, Now: time.Now, GoldAPI: "https://api.gold-api.com", Yahoo: "https://query1.finance.yahoo.com", SJC: "https://sjc.com.vn", Search: "https://html.duckduckgo.com"}
	d := &net.Dialer{Timeout: 8 * time.Second, Control: blockPrivate}
	k.Client = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{DialContext: d.DialContext, TLSHandshakeTimeout: 8 * time.Second, MaxResponseHeaderBytes: 64 << 10}}
	k.Client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || !k.Allowed(r.URL) {
			return ErrDenied
		}
		return nil
	}
	return k
}

func (k *Kit) Allowed(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, a := range k.Allow {
		if h == a || strings.HasSuffix(h, "."+a) {
			return true
		}
	}
	return false
}

func (k *Kit) get(ctx context.Context, method, raw, body string) ([]byte, error) {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, raw, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; NekoAurora/1.0; +https://github.com/meiiie/neko-aurora)")
	req.Header.Set("Accept-Language", "vi,en;q=0.8")
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := k.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http_%d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// Run executes one tool by name.
func (k *Kit) Run(ctx context.Context, name, arg string) (Result, error) {
	switch name {
	case "gold_price":
		return k.Gold(ctx)
	case "web_search":
		return k.WebSearch(ctx, arg)
	case "web_fetch":
		return k.Fetch(ctx, arg)
	}
	return Result{}, ErrUnknown
}

var goldRe = regexp.MustCompile(`(?i)\b(giá vàng|vàng sjc|xau|gold|sjc|vàng miếng|vàng nhẫn)\b`)

// Auto returns tools the server should prefetch for a request (one model
// round trip instead of two). Only clear data intents trigger it.
func Auto(text string) []string {
	if goldRe.MatchString(text) {
		return []string{"gold_price"}
	}
	return nil
}

// AutoURLs returns owner-typed https URLs on the allowlist (max 2) to read
// before the model answers.
func (k *Kit) AutoURLs(text string) []string {
	var out []string
	for _, raw := range RequestedURLs(text) {
		if u, err := url.Parse(raw); err == nil && k.Allowed(u) && len(out) < 2 {
			out = append(out, u.String())
		}
	}
	return out
}

var (
	tagRe   = regexp.MustCompile(`(?s)<(script|style|noscript|svg|head)[^>]*>.*?</(script|style|noscript|svg|head)>|<[^>]+>`)
	spaceRe = regexp.MustCompile(`[ \t\r\f\v]+`)
	nlRe    = regexp.MustCompile(`\n\s*\n+`)
	titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

func htmlText(b string) string {
	b = tagRe.ReplaceAllString(b, "\n")
	b = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(b)
	b = spaceRe.ReplaceAllString(b, " ")
	return strings.TrimSpace(nlRe.ReplaceAllString(b, "\n"))
}

// inlineText flattens an HTML fragment (title, snippet) to one line.
func inlineText(b string) string {
	b = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(b, "")
	return strings.Join(strings.Fields(htmlText(b)), " ")
}

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n] + "…"
}

func (k *Kit) Fetch(ctx context.Context, raw string) (Result, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !k.Allowed(u) {
		return Result{Name: "web_fetch", Arg: raw}, ErrDenied
	}
	b, err := k.get(ctx, "GET", u.String(), "")
	if err != nil {
		return Result{Name: "web_fetch", Arg: raw}, err
	}
	title := u.Host
	if m := titleRe.FindSubmatch(b); m != nil {
		title = cut(inlineText(string(m[1])), 120)
	}
	at := k.Now().Format(time.RFC3339)
	return Result{Name: "web_fetch", Arg: u.String(), Text: cut(htmlText(string(b)), 6000), Summary: "Đọc " + u.Host, Sources: []Source{{Title: title, URL: u.String(), At: at}}}, nil
}

var ddgRe = regexp.MustCompile(`(?s)class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>.*?class="result__snippet"[^>]*>(.*?)</a>`)

func (k *Kit) WebSearch(ctx context.Context, q string) (Result, error) {
	q = cut(strings.TrimSpace(q), 200)
	r := Result{Name: "web_search", Arg: q}
	if q == "" {
		return r, errors.New("empty_query")
	}
	b, err := k.get(ctx, "GET", k.Search+"/html/?kl=vn-vi&q="+url.QueryEscape(q), "")
	if err != nil {
		return r, err
	}
	var sb strings.Builder
	for _, m := range ddgRe.FindAllStringSubmatch(string(b), 8) {
		link := m[1]
		if pu, err := url.Parse(strings.ReplaceAll(link, "&amp;", "&")); err == nil {
			if v := pu.Query().Get("uddg"); v != "" {
				link = v
			}
		}
		if !strings.HasPrefix(link, "https://") || len(r.Sources) >= 6 {
			continue
		}
		t := cut(inlineText(m[2]), 120)
		r.Sources = append(r.Sources, Source{Title: t, URL: link, At: k.Now().Format(time.RFC3339)})
		fmt.Fprintf(&sb, "[%d] %s — %s\n%s\n", len(r.Sources), t, link, cut(inlineText(m[3]), 300))
	}
	if len(r.Sources) == 0 {
		return r, errors.New("no_results")
	}
	r.Text, r.Summary = sb.String(), fmt.Sprintf("Tìm “%s” · %d kết quả", cut(q, 40), len(r.Sources))
	return r, nil
}
