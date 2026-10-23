package computer

// Railway Sandboxes backend (production on Railway; docs:
// https://docs.railway.com/sandboxes). Each bot gets an isolated Linux VM
// created with the sandboxCreate GraphQL mutation and driven with
// sandboxExec (the SDK's execHttp: one HTTPS request, 16 000 bytes per
// stream, timeoutSec ≤ 600). Sandboxes are destroyed after the plan's idle
// timeout (Free: 5 min), so the bot's home stays authoritative on our
// state volume: before a command the home is pushed into the VM as a
// base64 tarball, after it the home is pulled back in chunks. That keeps
// the Manager contract (os.Root file access, quota, reset) identical to the
// bwrap backend. The SDK's files WebSocket API is the faster upgrade path.
//
// Config: RAILWAY_TOKEN (or RAILWAY_API_TOKEN) and RAILWAY_ENVIRONMENT_ID.
// The token is only sent to backboard as a Bearer header; it is never put
// into the sandbox (sandboxCreate gets no variables), so commands cannot
// read it. Limitations vs bwrap: Railway sandboxes always have outbound
// internet (networkIsolation ISOLATED only removes the private network), so
// Policy.Network=false is best effort; memory/CPU come from the VM size.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const railwayEndpoint = "https://backboard.railway.com/graphql/v2"

type Railway struct {
	Endpoint      string
	Token         string
	EnvironmentID string
	CPU, MemoryGB float64
	IdleMinutes   int
	Max           int    // live VMs across all users (0 = 3)
	Checkpoint    string // named checkpoint to boot from ("" = stock image)
	HTTP          *http.Client
	Relay         *Relay // HTTPS home sync (nil = base64 chunks through exec)
}

// One registry for the whole process (every tenant's Railway value shares
// it) so the total number of live VMs is capped: cost control approved by
// Darren 2026-10-23. At the cap the least recently used VM is destroyed;
// its home is safe on our volume, so the bot just boots a fresh one later.
var reg = struct {
	sync.Mutex
	ids    map[string]string    // home path → sandbox id
	used   map[string]time.Time // home path → last exec
	synced map[string]string    // home path → "<sandbox id>:<sha of home>" after the last pull
}{ids: map[string]string{}, used: map[string]time.Time{}, synced: map[string]string{}}

func NewRailwayFromEnv() *Railway {
	tok := os.Getenv("RAILWAY_TOKEN")
	if tok == "" {
		tok = os.Getenv("RAILWAY_API_TOKEN")
	}
	return &Railway{Endpoint: railwayEndpoint, Token: tok, EnvironmentID: os.Getenv("RAILWAY_ENVIRONMENT_ID"),
		CPU: envFloat("AURORA_SANDBOX_CPU", 0.5), MemoryGB: envFloat("AURORA_SANDBOX_MEMORY_GB", 0.5),
		IdleMinutes: int(envFloat("AURORA_SANDBOX_IDLE_MIN", 3)), Max: int(envFloat("AURORA_SANDBOX_MAX", 3)),
		Checkpoint: os.Getenv("AURORA_SANDBOX_CHECKPOINT"), Relay: DefaultRelay()}
}

var (
	relayOnce sync.Once
	relay     *Relay
)

// DefaultRelay is the process relay for home syncs, set when the server
// knows its public URL (AURORA_PUBLIC_URL, or Railway's own domain).
func DefaultRelay() *Relay {
	relayOnce.Do(func() {
		base := os.Getenv("AURORA_PUBLIC_URL")
		if base == "" && os.Getenv("RAILWAY_PUBLIC_DOMAIN") != "" {
			base = "https://" + os.Getenv("RAILWAY_PUBLIC_DOMAIN")
		}
		if base != "" && os.Getenv("AURORA_SANDBOX_RELAY") != "off" {
			relay = NewRelay(base)
		}
	})
	return relay
}

func envFloat(k string, d float64) float64 {
	var f float64
	if _, err := fmt.Sscan(os.Getenv(k), &f); err == nil && f > 0 {
		return f
	}
	return d
}

func (r *Railway) Name() string { return "railway-sandbox" }

func (r *Railway) Available() error {
	if r.Token == "" || r.EnvironmentID == "" {
		return fmt.Errorf("%w: RAILWAY_TOKEN and RAILWAY_ENVIRONMENT_ID not set", ErrUnavailable)
	}
	return nil
}

var errSandboxGone = errors.New("railway sandbox not found")

func (r *Railway) gql(ctx context.Context, query string, vars map[string]any, out any) error {
	b, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	c := r.HTTP
	if c == nil {
		c = &http.Client{Timeout: 11 * time.Minute}
	}
	var res *http.Response
	for try := 0; ; try++ {
		req, err := http.NewRequestWithContext(ctx, "POST", r.Endpoint, bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+r.Token)
		res, err = c.Do(req)
		if err != nil {
			return fmt.Errorf("%w: railway: %v", ErrUnavailable, err)
		}
		if res.StatusCode != http.StatusTooManyRequests || try >= 4 {
			break
		}
		// rate limited (live pptx task: pull failed with http 429): back off
		res.Body.Close()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryWait(try)):
		}
	}
	defer res.Body.Close()
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&env); err != nil {
		return fmt.Errorf("railway: http %d", res.StatusCode)
	}
	if len(env.Errors) > 0 {
		m := env.Errors[0].Message
		if l := strings.ToLower(m); strings.Contains(l, "not found") || strings.Contains(l, "destroyed") || strings.Contains(l, "not running") {
			// idle timeout already took the VM ("Sandbox is destroyed, cannot exec")
			return errSandboxGone
		}
		return fmt.Errorf("railway: %s", m) // never echoes the token
	}
	return json.Unmarshal(env.Data, out)
}

const (
	qCreate  = `mutation RailwaySandboxCreate($input: SandboxCreateInput!) { sandboxCreate(input: $input) { id status } }`
	qGet     = `query RailwaySandbox($environmentId: String!, $id: String!) { sandbox(environmentId: $environmentId, id: $id) { id status } }`
	qExec    = `mutation RailwaySandboxExec($environmentId: String!, $id: String!, $command: String!, $timeoutSec: Int) { sandboxExec(environmentId: $environmentId, id: $id, command: $command, timeoutSec: $timeoutSec) { exitCode stdout stderr truncated timedOut } }`
	qDestroy = `mutation RailwaySandboxDestroy($id: String!, $environmentId: String!) { sandboxDestroy(id: $id, environmentId: $environmentId) { id status } }`
)

type execOut struct {
	ExitCode  *int   `json:"exitCode"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timedOut"`
}

func (r *Railway) create(ctx context.Context) (string, error) {
	in := map[string]any{"environmentId": r.EnvironmentID, "idleTimeoutMinutes": r.IdleMinutes, "networkIsolation": "ISOLATED",
		"resources": map[string]any{"cpu": r.CPU, "memoryGB": r.MemoryGB}}
	if r.Checkpoint != "" { // boot from the prepared base (python3 installed)
		in["template"] = map[string]any{"name": r.Checkpoint}
	}
	var out struct {
		SandboxCreate struct{ ID, Status string } `json:"sandboxCreate"`
	}
	if err := r.gql(ctx, qCreate, map[string]any{"input": in}, &out); err != nil {
		return "", err
	}
	id := out.SandboxCreate.ID
	for i := 0; out.SandboxCreate.Status != "RUNNING"; i++ { // create resolves CREATING → RUNNING
		if i > 120 || out.SandboxCreate.Status == "FAILED" || out.SandboxCreate.Status == "DESTROYED" {
			return "", fmt.Errorf("%w: sandbox %s", ErrUnavailable, strings.ToLower(out.SandboxCreate.Status))
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		var g struct {
			Sandbox struct{ ID, Status string } `json:"sandbox"`
		}
		if err := r.gql(ctx, qGet, map[string]any{"environmentId": r.EnvironmentID, "id": id}, &g); err != nil {
			return "", err
		}
		out.SandboxCreate.Status = g.Sandbox.Status
	}
	return id, nil
}

func (r *Railway) sandbox(ctx context.Context, home string, fresh bool) (string, error) {
	reg.Lock()
	id := reg.ids[home]
	if id == "" && !fresh { // after a restart: reattach to the VM we recorded
		if b, err := os.ReadFile(idFile(home)); err == nil {
			id = strings.TrimSpace(string(b))
			if id != "" {
				reg.ids[home] = id
			}
		}
	}
	reg.used[home] = time.Now()
	var evict []string
	if id == "" || fresh {
		delete(reg.ids, home)
		max := r.Max
		if max <= 0 {
			max = 3
		}
		for len(reg.ids) >= max { // make room: drop the least recently used VM
			var lru string
			for h := range reg.ids {
				if lru == "" || reg.used[h].Before(reg.used[lru]) {
					lru = h
				}
			}
			evict = append(evict, reg.ids[lru])
			delete(reg.ids, lru)
			_ = os.Remove(idFile(lru))
		}
	}
	reg.Unlock()
	for _, e := range evict {
		r.destroyID(ctx, e)
	}
	if id != "" && !fresh {
		return id, nil
	}
	id, err := r.create(ctx)
	if err == nil {
		reg.Lock()
		reg.ids[home] = id
		reg.Unlock()
		_ = os.WriteFile(idFile(home), []byte(id), 0o600)
	}
	return id, err
}

// idFile records a home's VM id next to (never inside) the home, so a
// restarted server reattaches instead of leaking the VM.
func idFile(home string) string { return filepath.Clean(home) + ".sandbox-id" }

func (r *Railway) destroyID(ctx context.Context, id string) {
	var out map[string]any
	if err := r.gql(ctx, qDestroy, map[string]any{"id": id, "environmentId": r.EnvironmentID}, &out); err != nil && !errors.Is(err, errSandboxGone) {
		log.Printf("railway: destroy sandbox failed: %v", err)
	}
}

// DestroyUnder tears down every VM whose home lies under dir (account
// deletion removes the user's workspace directory).
func (r *Railway) DestroyUnder(ctx context.Context, dir string) {
	dir = filepath.Clean(dir) + string(filepath.Separator)
	reg.Lock()
	var ids []string
	for h, id := range reg.ids {
		if strings.HasPrefix(filepath.Clean(h)+string(filepath.Separator), dir) {
			ids = append(ids, id)
			delete(reg.ids, h)
			delete(reg.used, h)
		}
	}
	reg.Unlock()
	// VMs recorded before a restart are only on disk
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".sandbox-id") {
			if b, err := os.ReadFile(p); err == nil && len(b) > 0 && !slices.Contains(ids, strings.TrimSpace(string(b))) {
				ids = append(ids, strings.TrimSpace(string(b)))
			}
		}
		return nil
	})
	for _, id := range ids {
		r.destroyID(ctx, id)
	}
}

func (r *Railway) exec(ctx context.Context, id, cmd string, timeout int) (execOut, error) {
	var out struct {
		SandboxExec execOut `json:"sandboxExec"`
	}
	err := r.gql(ctx, qExec, map[string]any{"environmentId": r.EnvironmentID, "id": id, "command": cmd, "timeoutSec": timeout}, &out)
	return out.SandboxExec, err
}

// Destroy tears down a bot's VM (Reset / account deletion).
func (r *Railway) Destroy(ctx context.Context, home string) error {
	reg.Lock()
	id := reg.ids[home]
	delete(reg.ids, home)
	delete(reg.used, home)
	reg.Unlock()
	if b, err := os.ReadFile(idFile(home)); err == nil && id == "" {
		id = strings.TrimSpace(string(b))
	}
	_ = os.Remove(idFile(home))
	if id != "" {
		r.destroyID(ctx, id)
	}
	return nil
}

var retryWait = func(try int) time.Duration { return time.Duration(2<<try) * time.Second }

const chunk = 12000 // base64 bytes per exec (stdout limit is 16 000)

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (r *Railway) Exec(ctx context.Context, s Spec) (Result, error) {
	if err := r.Available(); err != nil {
		return Result{}, err
	}
	t0 := time.Now()
	tgz, err := packHome(s.Home)
	if err != nil {
		return Result{}, err
	}
	id, err := r.sandbox(ctx, s.Home, false)
	if err != nil {
		return Result{}, err
	}
	run := func(id string) (execOut, error) {
		if err := r.push(ctx, id, s.Home, tgz); err != nil {
			return execOut{}, err
		}
		// fresh env: only what the bwrap sandbox gets, never host variables
		envs := strings.Join(sandboxEnv, " ")
		secs := max(1, min(600, int(s.Limits.Timeout/time.Second)))
		return r.exec(ctx, id, "cd /home/bot && env -i "+envs+" timeout -k 5 "+fmt.Sprint(secs)+" /bin/sh -c "+shq(strictShell+s.Cmd)+" 2>&1", secs+15)
	}
	o, err := run(id)
	if errors.Is(err, errSandboxGone) { // idle-destroyed: boot a fresh VM, home comes from our copy
		if id, err = r.sandbox(ctx, s.Home, true); err == nil {
			o, err = run(id)
		}
	}
	if err != nil {
		return Result{}, err
	}
	res := Result{Cmd: s.Cmd, Output: o.Stdout + o.Stderr, Truncated: o.Truncated, TimedOut: o.TimedOut, Ms: int(time.Since(t0) / time.Millisecond), At: time.Now().UTC().Format(time.RFC3339)}
	if len(res.Output) > s.Limits.OutputBytes {
		res.Output, res.Truncated = res.Output[:s.Limits.OutputBytes], true
	}
	if o.ExitCode != nil {
		res.ExitCode = *o.ExitCode
	} else {
		res.ExitCode = 137
	}
	if res.ExitCode == 124 {
		res.TimedOut = true
	}
	if err := r.pull(ctx, id, s); err != nil {
		res.Output += "\n[không đồng bộ được file về: " + err.Error() + "]"
	}
	return res, nil
}

func homeSum(tgz []byte) string { return fmt.Sprintf("%x", sha256.Sum256(tgz)) }

// push replaces the VM's /home/bot with our copy, unless the VM already
// holds exactly that home (same VM, nothing changed since the last pull).
func (r *Railway) push(ctx context.Context, id, home string, tgz []byte) error {
	reg.Lock()
	same := reg.synced[home] == id+":"+homeSum(tgz)
	reg.Unlock()
	if same {
		return nil
	}
	if r.Relay != nil && len(tgz) > 0 {
		u := r.Relay.Offer(tgz)
		o, err := r.exec(ctx, id, "rm -rf /home/bot /tmp/h.tgz && mkdir -p /home/bot && python3 -c "+shq(pyGet)+" "+shq(u)+" && tar xzf /tmp/h.tgz -C /home/bot && rm -f /tmp/h.tgz", 120)
		if err == nil && o.ExitCode != nil && *o.ExitCode == 0 {
			return nil
		}
		if errors.Is(err, errSandboxGone) {
			return err
		}
		log.Printf("railway relay push failed, using chunks: %v %s", err, strings.TrimSpace(o.Stderr))
	}
	b64 := base64.StdEncoding.EncodeToString(tgz)
	if _, err := r.exec(ctx, id, "rm -rf /home/bot /tmp/h.b64 && mkdir -p /home/bot", 30); err != nil {
		return err
	}
	for i := 0; i < len(b64); i += 48000 {
		if _, err := r.exec(ctx, id, "printf %s "+shq(b64[i:min(i+48000, len(b64))])+" >> /tmp/h.b64", 30); err != nil {
			return err
		}
	}
	if len(b64) > 0 {
		if _, err := r.exec(ctx, id, "base64 -d /tmp/h.b64 | tar xzf - -C /home/bot", 60); err != nil {
			return err
		}
	}
	return nil
}

const (
	pyGet = "import sys,urllib.request as u;open('/tmp/h.tgz','wb').write(u.urlopen(sys.argv[1],timeout=120).read())"
	pyPut = "import sys,urllib.request as u;u.urlopen(u.Request(sys.argv[1],data=open('/tmp/o.tgz','rb').read(),method='PUT'),timeout=120)"
)

// pull copies /home/bot back (tar.gz, base64 in chunks) and replaces the
// local home atomically-ish: extract to a temp dir, then swap.
func (r *Railway) pull(ctx context.Context, id string, s Spec) error {
	raw, err := r.fetchHome(ctx, id, s)
	if err != nil {
		return err
	}
	tmp := s.Home + ".pull"
	_ = os.RemoveAll(tmp)
	if err := unpackHome(raw, tmp, s.Limits.HomeBytes); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	old := s.Home + ".old"
	_ = os.RemoveAll(old)
	if err := os.Rename(s.Home, old); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.Home); err != nil {
		_ = os.Rename(old, s.Home)
		return err
	}
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	if tgz, err := packHome(s.Home); err == nil {
		reg.Lock()
		reg.synced[s.Home] = id + ":" + homeSum(tgz)
		reg.Unlock()
	}
	return nil
}

func (r *Railway) fetchHome(ctx context.Context, id string, s Spec) ([]byte, error) {
	if r.Relay != nil {
		u, wait := r.Relay.Expect(s.Limits.HomeBytes * 2)
		o, err := r.exec(ctx, id, "cd /home/bot && tar czf /tmp/o.tgz . && python3 -c "+shq(pyPut)+" "+shq(u)+" && rm -f /tmp/o.tgz", 180)
		if err == nil && o.ExitCode != nil && *o.ExitCode == 0 {
			if b, werr := wait(ctx); werr == nil && b != nil {
				return b, nil
			} else {
				err = werr
			}
		}
		if errors.Is(err, errSandboxGone) {
			return nil, err
		}
		log.Printf("railway relay pull failed, using chunks: %v %s", err, strings.TrimSpace(o.Stderr))
	}
	return r.fetchChunks(ctx, id, s)
}

func (r *Railway) fetchChunks(ctx context.Context, id string, s Spec) ([]byte, error) {
	sz, err := r.exec(ctx, id, "cd /home/bot && tar czf /tmp/o.tgz . && base64 -w0 /tmp/o.tgz > /tmp/o.b64 && wc -c < /tmp/o.b64", 60)
	if err != nil {
		return nil, err
	}
	var n int
	if _, err := fmt.Sscan(strings.TrimSpace(sz.Stdout), &n); err != nil || int64(n) > s.Limits.HomeBytes*2 {
		return nil, errors.New("home too large")
	}
	var b strings.Builder
	for off := 0; off < n; off += chunk {
		o, err := r.exec(ctx, id, fmt.Sprintf("tail -c +%d /tmp/o.b64 | head -c %d", off+1, chunk), 30)
		if err != nil {
			return nil, err
		}
		b.WriteString(o.Stdout)
	}
	return base64.StdEncoding.DecodeString(b.String())
}

func packHome(home string) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "." || !(d.IsDir() || d.Type().IsRegular()) {
			return nil // symlinks and devices never leave or enter the host
		}
		info, _ := d.Info()
		h, _ := tar.FileInfoHeader(info, "")
		h.Name = p
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if d.Type().IsRegular() {
			b, err := root.ReadFile(p)
			if err != nil {
				return err
			}
			_, err = tw.Write(b)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	tw.Close()
	gz.Close()
	return buf.Bytes(), nil
}

// unpackHome extracts regular files and dirs only, inside dst, under a byte cap.
func unpackHome(tgz []byte, dst string, maxBytes int64) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	root, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer root.Close()
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := cleanRel(h.Name)
		if err != nil {
			return err
		}
		if rel == "." {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(rel, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if total += h.Size; total > maxBytes {
				return ErrQuota
			}
			if d := filepath.Dir(rel); d != "." {
				if err := root.MkdirAll(d, 0o700); err != nil {
					return err
				}
			}
			f, err := root.OpenFile(rel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, io.LimitReader(tr, h.Size))
			f.Close()
			if err != nil {
				return err
			}
		}
	}
}
