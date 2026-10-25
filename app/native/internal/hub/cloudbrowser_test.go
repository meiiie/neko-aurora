package hub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nekoaurora.local/native/internal/cloudbrowser"
)

type hostedCloudTransport struct {
	mu       sync.Mutex
	commands []cloudbrowser.Command
	purged   []string
	fail     bool
}

func (f *hostedCloudTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/v1/purge" {
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		f.purged = append(f.purged, in["namespace"])
		if f.fail {
			return nil, errors.New("fixture worker unavailable")
		}
		body, _ := json.Marshal(map[string]string{"namespace": in["namespace"], "nonce": in["nonce"], "status": "purged"})
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}
	var cmd cloudbrowser.Command
	json.NewDecoder(r.Body).Decode(&cmd)
	f.commands = append(f.commands, cmd)
	body, _ := json.Marshal(map[string]any{"profileId": cmd.Profile, "status": "ready", "control": "paused", "epoch": 2, "authState": "unknown", "protected": true})
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func TestHostedCloudPilotDefaultsOffAndMatchesExactNormalizedEmails(t *testing.T) {
	t.Setenv("AURORA_CLOUD_BROWSER_URL", "https://fixture.example.test")
	t.Setenv("AURORA_CLOUD_BROWSER_KEY", strings.Repeat("fixture!", 4))
	t.Setenv("AURORA_CLOUD_BROWSER_DOCKER", "")
	h, srv, fm := newHub(t, func(c *Config) { c.CloudBrowserEmails = map[string]bool{" ALLOWED@EXAMPLE.COM ": true} })
	a, b := newClient(t, srv), newClient(t, srv)
	signIn(t, a, fm, "allowed@example.com")
	signIn(t, b, fm, "allowed+other@example.com")
	_, ma := a.do("GET", "/app/auth/me", nil)
	_, mb := b.do("GET", "/app/auth/me", nil)
	ta, err := h.tenant(ma["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := h.tenant(mb["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if ta.svc.Cloud == nil || tb.svc.Cloud != nil {
		t.Fatal("hosted pilot did not require exact normalized email")
	}
	if status, out := b.do("GET", "/app/api/agents/bot-davin/cloud", nil); status != 200 || out["configured"] != false {
		t.Fatal("unlisted hosted owner reached browser capability")
	}
	if _, err = os.Stat(filepath.Join(h.userDir(mb["id"].(string)), "aurora", cloudbrowser.NamespaceFile)); !os.IsNotExist(err) {
		t.Fatal("unlisted user created browser namespace")
	}
	h2, srv2, fm2 := newHub(t, nil)
	u := newClient(t, srv2)
	signIn(t, u, fm2, "allowed@example.com")
	_, me := u.do("GET", "/app/auth/me", nil)
	tenant, err := h2.tenant(me["id"].(string))
	if err != nil || tenant.svc.Cloud != nil {
		t.Fatal("absent pilot list enabled hosted cloud")
	}
}

func TestHostedCloudEmailListIsAnExactNormalizedSet(t *testing.T) {
	allowed := cloudBrowserEmails(" ALLOWED@EXAMPLE.COM, second@example.com, not-an-email, , ")
	if len(allowed) != 2 || !allowed["allowed@example.com"] || !allowed["second@example.com"] || allowed["allowed+other@example.com"] {
		t.Fatal("pilot list used prefix/wildcard or failed normalization")
	}
	if len(cloudBrowserEmails("")) != 0 {
		t.Fatal("absent hosted pilot list enabled accounts")
	}
}

func TestHostedCloudNamespacesAndConfirmedDeletionStayTenantScoped(t *testing.T) {
	t.Setenv("AURORA_CLOUD_BROWSER_URL", "https://fixture.example.test")
	t.Setenv("AURORA_CLOUD_BROWSER_KEY", strings.Repeat("fixture!", 4))
	t.Setenv("AURORA_CLOUD_BROWSER_DOCKER", "")
	h, srv, fm := newHub(t, func(c *Config) { c.CloudBrowserEmails = map[string]bool{"a@example.com": true, "b@example.com": true} })
	a, b := newClient(t, srv), newClient(t, srv)
	signIn(t, a, fm, "a@example.com")
	signIn(t, b, fm, "b@example.com")
	_, ma := a.do("GET", "/app/auth/me", nil)
	_, mb := b.do("GET", "/app/auth/me", nil)
	ua, ub := ma["id"].(string), mb["id"].(string)
	ta, err := h.tenant(ua)
	if err != nil {
		t.Fatal(err)
	}
	tb, err := h.tenant(ub)
	if err != nil {
		t.Fatal(err)
	}
	f := &hostedCloudTransport{}
	for uid, tenant := range map[string]*tenant{ua: ta, ub: tb} {
		c, err := cloudbrowser.New(cloudbrowser.Config{URL: "https://fixture.example.test", Key: []byte(strings.Repeat("fixture!", 4)), StateDir: filepath.Join(h.userDir(uid), "aurora"), HTTP: &http.Client{Transport: f}})
		if err != nil {
			t.Fatal(err)
		}
		tenant.svc.Cloud = c
	}
	for _, owner := range []*client{a, b} {
		if status, out := owner.do("POST", "/app/api/agents/bot-davin/cloud/open", map[string]string{"clientNonce": "fixture-hosted-open"}); status != 200 {
			t.Fatal(status, out)
		}
	}
	if len(f.commands) != 2 || f.commands[0].Namespace == f.commands[1].Namespace || f.commands[0].ActorID != ua || f.commands[1].ActorID != ub {
		t.Fatal("hosted cookie/tenant/namespace boundary lost")
	}
	namespaceA, namespaceB := ta.svc.Cloud.Namespace(), tb.svc.Cloud.Namespace()
	if status, _ := a.do("POST", "/app/api/account/delete", map[string]string{"confirm": "no"}); status != 400 || len(f.purged) != 0 {
		t.Fatal("unconfirmed hosted deletion purged")
	}
	f.fail = true
	if status, _ := a.do("POST", "/app/api/account/delete", map[string]string{"confirm": "DELETE"}); status != 503 {
		t.Fatal("failed purge reported deletion accepted")
	}
	if _, ok := h.users.get(ua); !ok {
		t.Fatal("failed cleanup removed account")
	}
	if _, err = os.Stat(filepath.Join(h.userDir(ua), "aurora", cloudbrowser.NamespaceFile)); err != nil {
		t.Fatal("failed cleanup removed locator")
	}
	f.fail = false
	if status, out := a.do("POST", "/app/api/account/delete", map[string]string{"confirm": "DELETE"}); status != 202 {
		t.Fatal(status, out)
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if _, ok := h.users.get(ua); !ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, ok := h.users.get(ua); ok {
		t.Fatal("successful cleanup did not remove account")
	}
	if _, ok := h.users.get(ub); !ok || tb.svc.Cloud.Namespace() != namespaceB {
		t.Fatal("deletion changed another tenant")
	}
	for _, ns := range f.purged {
		if ns != namespaceA {
			t.Fatal("purged wrong tenant namespace")
		}
	}
	if len(f.purged) != 2 {
		t.Fatal("host deletion repeated a completed purge unnecessarily")
	}
	if _, err = h.tenant(ua); err == nil {
		t.Fatal("a request already authenticated before deletion could reopen the removed tenant")
	}
	if _, err = tb.svc.Cloud.Do(context.Background(), cloudbrowser.Command{Profile: "bot-davin", Kind: "status", Actor: "owner", ActorID: ub}); err != nil {
		t.Fatal("other tenant became unavailable")
	}
}
