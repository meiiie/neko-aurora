// Package computer gives each Neko Aurora bot its own persistent computer:
// a private home directory plus a sandbox that runs shell commands in it.
//
// Backends are pluggable (Sandbox). The box/dev backend is bubblewrap
// (bwrap.go). Production hosts without user namespaces (Railway, Fly) plug a
// container backend in behind the same interface (container.go documents
// the Docker/gVisor/Firecracker/E2B options).
//
// Security model: the host never trusts anything inside a home. File access
// from the host goes through os.Root, so symlinks planted by sandboxed code
// cannot reach outside the home. Command output is untrusted data. The
// sandbox environment is built from scratch (no host env, so no vault or
// model keys) and has no view of the host home, /workspace or the state dir.
package computer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Limits bound one command run.
type Limits struct {
	Timeout     time.Duration
	CPUSeconds  int   // RLIMIT_CPU
	MemoryBytes int64 // RLIMIT_AS
	FileBytes   int64 // RLIMIT_FSIZE (largest single file a command may write)
	Procs       int   // RLIMIT_NPROC
	OutputBytes int   // stdout+stderr kept per run
	HomeBytes   int64 // disk quota for the whole home
}

func DefaultLimits() Limits {
	return Limits{Timeout: 60 * time.Second, CPUSeconds: 60, MemoryBytes: 1 << 30, FileBytes: 64 << 20, Procs: 4096, OutputBytes: 16 << 10, HomeBytes: 256 << 20}
}

// Spec is one command for a sandbox.
type Spec struct {
	Cmd     string
	Home    string // host path of the bot's home (mounted at /home/bot)
	Network bool
	Limits  Limits
}

type Result struct {
	Cmd       string `json:"cmd"`
	ExitCode  int    `json:"exitCode"`
	Output    string `json:"output"` // combined stdout+stderr, capped; untrusted
	Truncated bool   `json:"truncated,omitempty"`
	TimedOut  bool   `json:"timedOut,omitempty"`
	Ms        int    `json:"ms"`
	At        string `json:"at"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

// Sandbox runs one command against a home directory.
type Sandbox interface {
	Name() string
	Available() error
	Exec(ctx context.Context, s Spec) (Result, error)
}

// Policy is the per-bot setting.
type Policy struct {
	AutoReadOnly bool `json:"autoReadOnly"` // run read-only commands without asking
	Network      bool `json:"network"`      // sandbox may reach the internet
}

type meta struct {
	Policy Policy   `json:"policy"`
	Log    []Result `json:"log"`
}

const maxLog = 30

var (
	ErrUnavailable = errors.New("computer_unavailable")
	ErrPath        = errors.New("invalid_path")
	ErrQuota       = errors.New("home_quota_exceeded")
	ErrTooLarge    = errors.New("file_too_large")
	botIDRe        = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// Manager owns every bot's computer under dir (state/computers).
type Manager struct {
	dir    string
	sb     Sandbox
	Limits Limits
	mu     sync.Mutex
	busy   map[string]*sync.Mutex
}

func New(dir string, sb Sandbox) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Manager{dir: dir, sb: sb, Limits: DefaultLimits(), busy: map[string]*sync.Mutex{}}, nil
}

func (m *Manager) Backend() string { return m.sb.Name() }

func (m *Manager) botDir(bot string) (string, error) {
	if !botIDRe.MatchString(bot) {
		return "", ErrPath
	}
	return filepath.Join(m.dir, bot), nil
}

// home returns (and creates) the bot's home. Metadata lives beside it, never
// inside, so sandboxed code cannot rewrite its own policy or log.
func (m *Manager) home(bot string) (string, error) {
	d, err := m.botDir(bot)
	if err != nil {
		return "", err
	}
	h := filepath.Join(d, "home")
	return h, os.MkdirAll(h, 0o700)
}

func (m *Manager) lock(bot string) func() {
	m.mu.Lock()
	l := m.busy[bot]
	if l == nil {
		l = &sync.Mutex{}
		m.busy[bot] = l
	}
	m.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func (m *Manager) load(bot string) meta {
	var x meta
	d, err := m.botDir(bot)
	if err != nil {
		return x
	}
	if b, err := os.ReadFile(filepath.Join(d, "meta.json")); err == nil {
		_ = json.Unmarshal(b, &x)
	}
	return x
}

func (m *Manager) save(bot string, x meta) error {
	d, err := m.botDir(bot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(x)
	tmp := filepath.Join(d, "meta.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(d, "meta.json"))
}

func (m *Manager) Policy(bot string) Policy { return m.load(bot).Policy }

func (m *Manager) SetPolicy(bot string, p Policy) error {
	unlock := m.lock(bot)
	defer unlock()
	x := m.load(bot)
	x.Policy = p
	return m.save(bot, x)
}

// Run executes cmd in the bot's sandbox and appends it to the command log.
// Callers decide approval first (NeedsApproval).
func (m *Manager) Run(ctx context.Context, bot, cmd string) (Result, error) {
	if strings.TrimSpace(cmd) == "" || len(cmd) > 8000 {
		return Result{}, ErrPath
	}
	if err := m.sb.Available(); err != nil {
		return Result{}, err
	}
	h, err := m.home(bot)
	if err != nil {
		return Result{}, err
	}
	unlock := m.lock(bot)
	defer unlock()
	if used, _ := dirSize(h); used > m.Limits.HomeBytes {
		return Result{}, ErrQuota
	}
	x := m.load(bot)
	r, err := m.sb.Exec(ctx, Spec{Cmd: cmd, Home: h, Network: x.Policy.Network, Limits: m.Limits})
	if err != nil {
		return r, err
	}
	r.ReadOnly = ReadOnly(cmd)
	if used, _ := dirSize(h); used > m.Limits.HomeBytes {
		r.Output += fmt.Sprintf("\n[cảnh báo: thư mục nhà vượt hạn mức %d MB; lệnh sau sẽ bị từ chối tới khi dọn bớt]", m.Limits.HomeBytes>>20)
	}
	x.Log = append(x.Log, r)
	if len(x.Log) > maxLog {
		x.Log = x.Log[len(x.Log)-maxLog:]
	}
	_ = m.save(bot, x)
	return r, nil
}

// NeedsApproval: anything that may change state needs the owner, unless the
// bot is set to auto-run commands that are clearly read-only.
func (m *Manager) NeedsApproval(bot, cmd string) bool {
	return !(m.Policy(bot).AutoReadOnly && ReadOnly(cmd))
}

// cleanRel maps a bot-visible path ("/home/bot/x", "~/x", "x") to a path
// relative to the home root.
func cleanRel(p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/home/bot")
	p = strings.TrimPrefix(p, "~")
	p = strings.TrimLeft(p, "/")
	if p == "" {
		return ".", nil
	}
	c := path.Clean(p)
	if c == ".." || strings.HasPrefix(c, "../") || strings.ContainsRune(c, 0) {
		return "", ErrPath
	}
	return c, nil
}

func (m *Manager) root(bot string) (*os.Root, error) {
	h, err := m.home(bot)
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(h)
}

const maxReadBytes = 64 << 10
const maxWriteBytes = 1 << 20

// ReadFile returns up to 64 KB of a file in the bot's home.
func (m *Manager) ReadFile(bot, p string) (string, bool, error) {
	rel, err := cleanRel(p)
	if err != nil {
		return "", false, err
	}
	r, err := m.root(bot)
	if err != nil {
		return "", false, err
	}
	defer r.Close()
	f, err := r.Open(rel)
	if err != nil {
		return "", false, ErrPath
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return "", false, ErrPath
	}
	b := make([]byte, maxReadBytes+1)
	n, _ := f.Read(b)
	for n < len(b) {
		k, err := f.Read(b[n:])
		n += k
		if err != nil || k == 0 {
			break
		}
	}
	return strings.ToValidUTF8(string(b[:min(n, maxReadBytes)]), "?"), n > maxReadBytes, nil
}

// WriteFile writes a file inside the home (parents created).
func (m *Manager) WriteFile(bot, p, content string) error {
	if len(content) > maxWriteBytes {
		return ErrTooLarge
	}
	rel, err := cleanRel(p)
	if err != nil || rel == "." {
		return ErrPath
	}
	unlock := m.lock(bot)
	defer unlock()
	r, err := m.root(bot)
	if err != nil {
		return err
	}
	defer r.Close()
	if dir := path.Dir(rel); dir != "." {
		if err := r.MkdirAll(dir, 0o700); err != nil {
			return ErrPath
		}
	}
	h, _ := m.home(bot)
	if used, _ := dirSize(h); used+int64(len(content)) > m.Limits.HomeBytes {
		return ErrQuota
	}
	if err := r.WriteFile(rel, []byte(content), 0o600); err != nil {
		return ErrPath
	}
	return nil
}

type Entry struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir,omitempty"`
	Size int64  `json:"size"`
	Mod  string `json:"mod"`
}

// List walks the home (or a subdirectory), max 200 entries, never following
// symlinks.
func (m *Manager) List(bot, p string) ([]Entry, error) {
	rel, err := cleanRel(p)
	if err != nil {
		return nil, err
	}
	r, err := m.root(bot)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out := []Entry{}
	err = fs.WalkDir(r.FS(), rel, func(q string, d fs.DirEntry, err error) error {
		if err != nil || q == rel {
			return nil
		}
		if len(out) >= 200 {
			return fs.SkipAll
		}
		info, e := d.Info()
		if e != nil {
			return nil
		}
		out = append(out, Entry{Path: q, Dir: d.IsDir(), Size: info.Size(), Mod: info.ModTime().UTC().Format(time.RFC3339)})
		if d.IsDir() && strings.Count(q, "/") >= 3 {
			return fs.SkipDir
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

type Status struct {
	Backend   string   `json:"backend"`
	Available bool     `json:"available"`
	Error     string   `json:"error,omitempty"`
	Policy    Policy   `json:"policy"`
	UsedBytes int64    `json:"usedBytes"`
	Quota     int64    `json:"quotaBytes"`
	Files     []Entry  `json:"files"`
	Log       []Result `json:"log"`
}

func (m *Manager) Status(bot string) (Status, error) {
	h, err := m.home(bot)
	if err != nil {
		return Status{}, err
	}
	x := m.load(bot)
	st := Status{Backend: m.sb.Name(), Available: true, Policy: x.Policy, Quota: m.Limits.HomeBytes, Log: x.Log}
	if st.Log == nil {
		st.Log = []Result{}
	}
	if err := m.sb.Available(); err != nil {
		st.Available, st.Error = false, err.Error()
	}
	st.UsedBytes, _ = dirSize(h)
	st.Files, _ = m.List(bot, "")
	return st, nil
}

// Reset wipes the home and command log; the policy is kept.
func (m *Manager) Reset(bot string) error {
	h, err := m.home(bot)
	if err != nil {
		return err
	}
	unlock := m.lock(bot)
	defer unlock()
	_ = filepath.WalkDir(h, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0o700) // sandboxed code may have dropped write bits
		}
		return nil
	})
	if err := os.RemoveAll(h); err != nil {
		return err
	}
	if d, ok := m.sb.(interface {
		Destroy(context.Context, string) error
	}); ok {
		_ = d.Destroy(context.Background(), h) // remote VM (Railway) goes too
	}
	x := m.load(bot)
	x.Log = nil
	if err := m.save(bot, x); err != nil {
		return err
	}
	return os.MkdirAll(h, 0o700)
}

func dirSize(root string) (int64, error) {
	var n int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if i, e := d.Info(); e == nil {
				n += i.Size()
			}
		}
		return nil
	})
	return n, err
}

var readOnlyCmds = map[string]bool{"ls": true, "cat": true, "head": true, "tail": true, "wc": true, "grep": true, "pwd": true, "stat": true, "du": true, "df": true, "file": true, "sort": true, "uniq": true, "cut": true, "tree": true, "echo": true, "date": true, "whoami": true, "id": true, "uname": true, "which": true, "md5sum": true, "sha256sum": true, "diff": true, "column": true, "nl": true, "less": true, "true": true, "find": true}

var unsafeShell = regexp.MustCompile("[>`]|\\$\\(|<\\(|(^|\\s)-(exec|execdir|delete|ok|okdir|fprint\\w*|fls)\\b")

// ReadOnly reports a command made only of read-only programs joined by
// pipes/&&/;, with no redirection or substitution. Conservative by design:
// anything else (python, rm, cp, pip…) needs approval.
func ReadOnly(cmd string) bool {
	if unsafeShell.MatchString(cmd) || strings.Contains(cmd, "\n") {
		return false
	}
	for _, seg := range regexp.MustCompile(`\|\||&&|[|;&]`).Split(cmd, -1) {
		f := strings.Fields(seg)
		if len(f) == 0 || !readOnlyCmds[f[0]] {
			return false
		}
	}
	return true
}
