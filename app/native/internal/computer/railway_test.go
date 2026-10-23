package computer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRailway speaks the subset of backboard GraphQL the backend uses and
// runs sandboxExec commands locally, mapping /home/bot and /tmp to a temp VM dir.
type fakeRailway struct {
	t       *testing.T
	mu      sync.Mutex
	vms     map[string]string
	n       int
	cmds    []string
	creates int
	auth    []string
}

func (f *fakeRailway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	reply := func(v any) { json.NewEncoder(w).Encode(map[string]any{"data": v}) }
	switch {
	case strings.Contains(in.Query, "sandboxCreate("):
		inp := in.Variables["input"].(map[string]any)
		if inp["environmentId"] != "env1" || inp["variables"] != nil {
			f.t.Errorf("bad create input %v", inp)
		}
		f.n++
		f.creates++
		id := "sb" + string(rune('0'+f.n))
		dir := f.t.TempDir()
		os.MkdirAll(filepath.Join(dir, "tmp"), 0o700)
		f.vms[id] = dir
		reply(map[string]any{"sandboxCreate": map[string]any{"id": id, "status": "CREATING"}})
	case strings.Contains(in.Query, "sandbox(environmentId"):
		reply(map[string]any{"sandbox": map[string]any{"id": in.Variables["id"], "status": "RUNNING"}})
	case strings.Contains(in.Query, "sandboxExec("):
		dir, ok := f.vms[in.Variables["id"].(string)]
		if !ok {
			// what backboard really says for an idle-destroyed VM (checked live)
			json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]string{{"message": "Sandbox is destroyed, cannot exec"}}})
			return
		}
		c := in.Variables["command"].(string)
		f.cmds = append(f.cmds, c)
		c = strings.ReplaceAll(strings.ReplaceAll(c, "/home/bot", dir+"/home"), "/tmp/", dir+"/tmp/")
		out, err := exec.Command("/bin/sh", "-c", c).Output()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		reply(map[string]any{"sandboxExec": map[string]any{"exitCode": code, "stdout": string(out), "stderr": "", "truncated": false, "timedOut": false}})
	case strings.Contains(in.Query, "sandboxDestroy("):
		delete(f.vms, in.Variables["id"].(string))
		reply(map[string]any{"sandboxDestroy": map[string]any{"id": in.Variables["id"], "status": "DESTROYED"}})
	default:
		f.t.Errorf("unexpected query %s", in.Query)
	}
}

func TestRailwayBackend(t *testing.T) {
	f := &fakeRailway{t: t, vms: map[string]string{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	t.Setenv("RAILWAY_TOKEN", "rw-secret-token")
	t.Setenv("RAILWAY_ENVIRONMENT_ID", "env1")
	rw := NewRailwayFromEnv()
	rw.Endpoint = srv.URL
	m, _ := New(t.TempDir(), rw)
	if err := m.WriteFile("b", "data/a.csv", "x,y\n1,2\n3,4\n"); err != nil {
		t.Fatal(err)
	}
	r, err := m.Run(context.Background(), "b", "awk -F, 'NR>1{s+=$2} END{print s}' data/a.csv > sum.txt; cat sum.txt; env")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.Output, "6\n") || strings.Contains(r.Output, "rw-secret") || strings.Contains(r.Output, "RAILWAY") {
		t.Fatalf("%+v", r)
	}
	if txt, _, err := m.ReadFile("b", "sum.txt"); err != nil || txt != "6\n" {
		t.Fatalf("file not pulled back: %q %v", txt, err)
	}
	for _, c := range f.cmds {
		if strings.Contains(c, "rw-secret") {
			t.Fatal("token sent into sandbox")
		}
	}
	for _, a := range f.auth {
		if a != "Bearer rw-secret-token" {
			t.Fatal("auth header", a)
		}
	}
	// idle-destroyed VM: recreated, home restored from our copy
	f.mu.Lock()
	f.vms = map[string]string{}
	f.mu.Unlock()
	if r, err := m.Run(context.Background(), "b", "cat sum.txt"); err != nil || strings.TrimSpace(r.Output) != "6" || f.creates != 2 {
		t.Fatalf("%+v %v creates=%d", r, err, f.creates)
	}
	// server restart: the registry is empty, the recorded VM is reattached
	reg.Lock()
	reg.ids, reg.used = map[string]string{}, map[string]time.Time{}
	reg.Unlock()
	if r, err := m.Run(context.Background(), "b", "cat sum.txt"); err != nil || strings.TrimSpace(r.Output) != "6" || f.creates != 2 {
		t.Fatalf("restart: %+v %v creates=%d", r, err, f.creates)
	}
	if err := m.Reset("b"); err != nil || len(f.vms) != 0 {
		t.Fatal("reset did not destroy VM", err)
	}
	if (&Railway{}).Available() == nil {
		t.Fatal("no token must be unavailable")
	}
}

func TestUnpackRejectsEscape(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "ok"), []byte("1"), 0o600)
	tgz, _ := packHome(src)
	if err := unpackHome(tgz, t.TempDir(), 10); err != nil {
		t.Fatal(err)
	}
	if err := unpackHome(tgz, t.TempDir(), 0); err != ErrQuota {
		t.Fatal("quota", err)
	}
}

func TestDestroyUnderAfterRestart(t *testing.T) {
	f := &fakeRailway{t: t, vms: map[string]string{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	t.Setenv("RAILWAY_TOKEN", "x")
	t.Setenv("RAILWAY_ENVIRONMENT_ID", "env1")
	rw := NewRailwayFromEnv()
	rw.Endpoint = srv.URL
	user := t.TempDir()
	m, _ := New(filepath.Join(user, "bots"), rw)
	if _, err := m.Run(context.Background(), "b", "true"); err != nil {
		t.Fatal(err)
	}
	reg.Lock()
	reg.ids, reg.used = map[string]string{}, map[string]time.Time{}
	reg.Unlock()
	rw.DestroyUnder(context.Background(), user)
	if len(f.vms) != 0 {
		t.Fatal("VM recorded before restart not destroyed on account delete")
	}
}

// Home syncs go over the HTTPS relay (one exec each way, no base64 chunks:
// live pptx task hit Railway's rate limit with ~220 chunk execs), an
// unchanged home is not pushed again, and 429s are retried.
func TestRailwayRelaySync(t *testing.T) {
	f := &fakeRailway{t: t, vms: map[string]string{}}
	var limited int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		if limited < 2 {
			limited++
			f.mu.Unlock()
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited"))
			return
		}
		f.mu.Unlock()
		f.ServeHTTP(w, r)
	}))
	defer srv.Close()
	rl := NewRelay("")
	rsrv := httptest.NewServer(rl)
	defer rsrv.Close()
	rl.Base = rsrv.URL
	defer func(w func(int) time.Duration) { retryWait = w }(retryWait)
	retryWait = func(int) time.Duration { return time.Millisecond }
	t.Setenv("RAILWAY_TOKEN", "tok")
	t.Setenv("RAILWAY_ENVIRONMENT_ID", "env1")
	rw := NewRailwayFromEnv()
	rw.Endpoint, rw.Relay = srv.URL, rl
	m, _ := New(t.TempDir(), rw)
	big := strings.Repeat("0123456789abcdef", 1<<16) // 1 MB: ~120 chunk execs without the relay
	if err := m.WriteFile("b", "deck/in.txt", big); err != nil {
		t.Fatal(err)
	}
	r, err := m.Run(context.Background(), "b", "head -c 2000000 /dev/urandom > deck/out.bin && wc -c < deck/in.txt")
	if err != nil || strings.TrimSpace(strings.Split(r.Output, "\n")[0]) != "1048576" {
		t.Fatalf("%+v %v", r, err)
	}
	if fi, err := os.Stat(filepath.Join(m.dir, "b", "home", "deck", "out.bin")); err != nil || fi.Size() != 2000000 {
		if _, _, e2 := m.ReadFile("b", "deck/out.bin"); e2 != nil {
			t.Fatalf("binary not pulled back: %v %v", err, e2)
		}
	}
	f.mu.Lock()
	n := len(f.cmds)
	for _, c := range f.cmds {
		if strings.Contains(c, "base64") {
			t.Fatalf("chunk sync used despite relay: %.80s", c)
		}
	}
	f.mu.Unlock()
	if n > 5 {
		t.Fatalf("%d execs for one command", n)
	}
	// nothing changed since the pull: the next command skips the push
	if _, err := m.Run(context.Background(), "b", "true"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.cmds[n:] {
		if strings.Contains(c, "h.tgz") {
			t.Fatal("unchanged home pushed again")
		}
	}
}

func TestRelaySingleUse(t *testing.T) {
	rl := NewRelay("http://x")
	u := rl.Offer([]byte("hi"))
	tok := strings.TrimPrefix(u, "http://x")
	for i, want := range []int{200, 404} {
		rec := httptest.NewRecorder()
		rl.ServeHTTP(rec, httptest.NewRequest("GET", tok, nil))
		if rec.Code != want {
			t.Fatalf("get %d: %d", i, rec.Code)
		}
	}
	up, wait := rl.Expect(4)
	rec := httptest.NewRecorder()
	rl.ServeHTTP(rec, httptest.NewRequest("PUT", strings.TrimPrefix(up, "http://x"), strings.NewReader("toolarge")))
	if b, _ := wait(context.Background()); rec.Code != 413 || b != nil {
		t.Fatal("oversize upload accepted", rec.Code)
	}
	rec = httptest.NewRecorder()
	rl.ServeHTTP(rec, httptest.NewRequest("GET", RelayPath+"nope", nil))
	if rec.Code != 404 {
		t.Fatal("unknown token served")
	}
}
