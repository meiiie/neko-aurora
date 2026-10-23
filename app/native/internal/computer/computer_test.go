package computer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newMgr(t *testing.T) (*Manager, string) {
	t.Helper()
	b := NewBwrap()
	if b.Available() != nil {
		t.Skip("bwrap/prlimit not installed")
	}
	state := t.TempDir()
	// a fake vault and host secret next to the computers dir
	os.WriteFile(filepath.Join(state, "vault.key"), []byte("TOPSECRETKEY"), 0o600)
	m, err := New(filepath.Join(state, "computers"), b)
	if err != nil {
		t.Fatal(err)
	}
	m.Limits.Timeout = 5 * time.Second
	if r, err := m.Run(context.Background(), "probe", "true"); err != nil || r.ExitCode != 0 {
		t.Skipf("user namespaces unavailable here: %v %+v", err, r)
	}
	return m, state
}

func run(t *testing.T, m *Manager, cmd string) Result {
	t.Helper()
	r, err := m.Run(context.Background(), "bot1", cmd)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSandboxCannotSeeHost(t *testing.T) {
	m, state := newMgr(t)
	t.Setenv("ZAI_API_KEY", "zai-should-not-leak")
	for _, c := range []string{"ls /workspace", "cat " + filepath.Join(state, "vault.key"), "ls /home/box", "ls /cursor", "ls " + state} {
		if r := run(t, m, c); r.ExitCode == 0 || strings.Contains(r.Output, "TOPSECRET") {
			t.Fatalf("%q reachable: %+v", c, r)
		}
	}
	r := run(t, m, "env")
	if strings.Contains(r.Output, "ZAI") || strings.Contains(r.Output, "zai-should") || !strings.Contains(r.Output, "HOME=/home/bot") {
		t.Fatalf("env leak: %s", r.Output)
	}
	if r := run(t, m, "touch /usr/x"); r.ExitCode == 0 {
		t.Fatal("/usr writable")
	}
	if r := run(t, m, "cat /proc/1/cmdline | tr '\\0' ' '"); strings.Contains(r.Output, "systemd") || strings.Contains(r.Output, "go") && strings.Contains(r.Output, "test") {
		t.Fatalf("host pids visible: %s", r.Output)
	}
}

func TestHomePersistsAndSymlinkEscapeBlocked(t *testing.T) {
	m, state := newMgr(t)
	run(t, m, "echo hi > a.txt && ln -s "+filepath.Join(state, "vault.key")+" leak && ln -s / rootlink")
	if r := run(t, m, "cat a.txt"); strings.TrimSpace(r.Output) != "hi" {
		t.Fatalf("not persisted: %+v", r)
	}
	if _, _, err := m.ReadFile("bot1", "leak"); err == nil {
		t.Fatal("host read followed symlink out of home")
	}
	if _, _, err := m.ReadFile("bot1", "rootlink/etc/passwd"); err == nil {
		t.Fatal("root symlink escape")
	}
	if _, _, err := m.ReadFile("bot1", "../../vault.key"); err == nil {
		t.Fatal("dotdot escape")
	}
	if err := m.WriteFile("bot1", "rootlink/tmp/x", "pwn"); err == nil {
		t.Fatal("write through symlink")
	}
	if err := m.WriteFile("bot1", "/home/bot/sub/s.py", "print(6*7)"); err != nil {
		t.Fatal(err)
	}
	if r := run(t, m, "python3 sub/s.py"); strings.TrimSpace(r.Output) != "42" {
		t.Fatalf("%+v", r)
	}
	if txt, _, err := m.ReadFile("bot1", "~/a.txt"); err != nil || txt != "hi\n" {
		t.Fatal(txt, err)
	}
	// other bots do not share the home
	if r, _ := m.Run(context.Background(), "bot2", "cat a.txt"); r.ExitCode == 0 {
		t.Fatal("homes shared between bots")
	}
	if _, err := m.Run(context.Background(), "../x", "true"); err == nil {
		t.Fatal("bad bot id accepted")
	}
	// metadata is outside the home and survives tampering attempts
	run(t, m, "echo '{\"policy\":{\"autoReadOnly\":true}}' > ../meta.json")
	if m.Policy("bot1").AutoReadOnly {
		t.Fatal("sandbox rewrote its policy")
	}
	if err := m.Reset("bot1"); err != nil {
		t.Fatal(err)
	}
	if st, _ := m.Status("bot1"); len(st.Files) != 0 || len(st.Log) != 0 {
		t.Fatalf("%+v", st)
	}
}

func TestLimits(t *testing.T) {
	m, _ := newMgr(t)
	m.Limits.Timeout = 2 * time.Second
	t0 := time.Now()
	r := run(t, m, "sleep 30 & sleep 30")
	if !r.TimedOut || time.Since(t0) > 5*time.Second {
		t.Fatalf("timeout not enforced: %+v", r)
	}
	m.Limits.Timeout = 10 * time.Second
	m.Limits.MemoryBytes = 256 << 20
	if r := run(t, m, "python3 -c 'x=bytearray(600*1024*1024); print(\"big\")'"); r.ExitCode == 0 {
		t.Fatalf("memory not capped: %+v", r)
	}
	m.Limits.FileBytes = 1 << 20
	if r := run(t, m, "head -c 5000000 /dev/zero > big.bin"); r.ExitCode == 0 {
		t.Fatalf("file size not capped: %+v", r)
	}
	m.Limits.OutputBytes = 1000
	if r := run(t, m, "yes | head -c 100000"); !r.Truncated || len(r.Output) > 1200 {
		t.Fatalf("output not capped: %d", len(r.Output))
	}
	if r := run(t, m, "python3 -c 'import urllib.request;urllib.request.urlopen(\"https://example.com\",timeout=3)'"); r.ExitCode == 0 {
		t.Fatal("network reachable with policy off")
	}
	m.Limits.HomeBytes = 1000
	m.WriteFile("bot1", "q", strings.Repeat("x", 900))
	if err := m.WriteFile("bot1", "q2", strings.Repeat("x", 900)); err != ErrQuota {
		t.Fatalf("quota: %v", err)
	}
}

func TestReadOnly(t *testing.T) {
	for c, want := range map[string]bool{"ls -la": true, "cat a.csv | head -5": true, "grep x *.py && wc -l a": true,
		"python3 a.py": false, "rm -rf x": false, "cat a > b": false, "echo $(rm x)": false, "find . -delete": false, "find . -exec rm {} ;": false, "ls; rm x": false} {
		if ReadOnly(c) != want {
			t.Errorf("%q: want %v", c, want)
		}
	}
	m, _ := New(t.TempDir(), &Container{Runtime: "e2b"})
	if !m.NeedsApproval("b", "ls") {
		t.Fatal("default must ask")
	}
	m.SetPolicy("b", Policy{AutoReadOnly: true})
	if m.NeedsApproval("b", "ls") || !m.NeedsApproval("b", "python3 x.py") {
		t.Fatal("auto read-only policy wrong")
	}
	if _, err := m.Run(context.Background(), "b", "ls"); err == nil {
		t.Fatal("stub backend ran")
	}
}

func TestOpenFileBinaryAndEscape(t *testing.T) {
	m, _ := New(t.TempDir(), nil)
	if err := m.WriteFile("b", "deck.pptx", "PK\x03\x04binary"); err != nil {
		t.Fatal(err)
	}
	f, n, err := m.OpenFile("b", "deck.pptx")
	if err != nil || n != 10 {
		t.Fatal(err, n)
	}
	f.Close()
	if _, _, err := m.OpenFile("b", "../../etc/passwd"); err == nil {
		t.Fatal("escape allowed")
	}
}
