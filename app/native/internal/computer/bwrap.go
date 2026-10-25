package computer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Bwrap runs commands with bubblewrap (user namespaces, no root needed):
// fresh mount/pid/ipc/uts/net namespaces, read-only /usr, a private /tmp,
// the bot's home bound at /home/bot and nothing else from the host.
// Resource caps come from prlimit (CPU, address space, file size, procs) and
// a wall-clock timeout that kills the whole sandbox (pid namespace).
type Bwrap struct {
	Path    string // bwrap binary
	Prlimit string

	once     sync.Once
	probeErr error
}

func NewBwrap() *Bwrap {
	b := &Bwrap{Path: "bwrap", Prlimit: "prlimit"}
	if p, err := exec.LookPath("bwrap"); err == nil {
		b.Path = p
	}
	if p, err := exec.LookPath("prlimit"); err == nil {
		b.Prlimit = p
	}
	return b
}

func (b *Bwrap) Name() string { return "bubblewrap" }

// probe checks once that this host lets bwrap create namespaces. Some
// container hosts (Railway's default runtime) forbid it: every command
// would fail with "Creating new namespace failed", so report the computer
// as unavailable up front instead (field report 2026-10-23).
func (b *Bwrap) probe() error {
	b.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, b.Path, "--unshare-user", "--unshare-pid", "--ro-bind", "/", "/", "--proc", "/proc", "--dev", "/dev", "/bin/true").CombinedOutput()
		if err != nil {
			b.probeErr = fmt.Errorf("%w: host does not allow sandbox namespaces (%s)", ErrUnavailable, strings.TrimSpace(string(out)))
		}
	})
	return b.probeErr
}

// sandboxEnv is the complete environment inside the sandbox. It is built
// from constants, never from os.Environ, so no host secret can leak in.
var sandboxEnv = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/home/bot", "USER=bot", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "TERM=dumb", "PYTHONDONTWRITEBYTECODE=1", "MPLBACKEND=Agg", "TMPDIR=/tmp", "PIP_NO_CACHE_DIR=1", "PIP_ROOT_USER_ACTION=ignore", "PIP_DISABLE_PIP_VERSION_CHECK=1"}

// strictShell gives every command set -e and (where the shell has it)
// pipefail, so "python3 render.py; ls -la out" or "build | tail" report the
// failing part's exit code instead of the last command's 0 (live run 5:
// the video render was OOM-killed and the card still said exit 0).
const strictShell = "(set -o pipefail) 2>/dev/null && set -o pipefail; set -e\n"

func (b *Bwrap) args(s Spec) []string {
	l := s.Limits
	a := []string{
		"--cpu=" + strconv.Itoa(l.CPUSeconds),
		"--as=" + strconv.FormatInt(l.MemoryBytes, 10),
		"--fsize=" + strconv.FormatInt(l.FileBytes, 10),
		"--nproc=" + strconv.Itoa(l.Procs),
		"--core=0",
		"--", b.Path,
		"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
		"--die-with-parent", "--new-session", "--clearenv",
		"--hostname", "neko-computer",
		"--ro-bind", "/usr", "/usr",
		"--symlink", "usr/bin", "/bin", "--symlink", "usr/sbin", "/sbin",
		"--symlink", "usr/lib", "/lib", "--symlink", "usr/lib64", "/lib64",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
		"--dir", "/etc",
		"--ro-bind-try", "/etc/alternatives", "/etc/alternatives",
		"--ro-bind-try", "/etc/ssl", "/etc/ssl",
		"--ro-bind-try", "/etc/ca-certificates", "/etc/ca-certificates",
		"--ro-bind-try", "/etc/mime.types", "/etc/mime.types",
		"--ro-bind-try", "/etc/ld.so.cache", "/etc/ld.so.cache",
		"--bind", s.Home, "/home/bot",
		"--chdir", "/home/bot",
	}
	if s.Network {
		a = append(a, "--share-net", "--ro-bind-try", "/etc/resolv.conf", "/etc/resolv.conf", "--ro-bind-try", "/etc/hosts", "/etc/hosts")
	} else {
		a = append(a, "--unshare-net")
	}
	for _, e := range sandboxEnv {
		k, v, _ := bytesCut(e)
		a = append(a, "--setenv", k, v)
	}
	return append(a, "/bin/sh", "-c", strictShell+s.Cmd)
}

func bytesCut(e string) (string, string, bool) {
	for i := 0; i < len(e); i++ {
		if e[i] == '=' {
			return e[:i], e[i+1:], true
		}
	}
	return e, "", false
}

type capWriter struct {
	buf   bytes.Buffer
	max   int
	trunc bool
}

func (c *capWriter) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		if len(p) > room {
			c.buf.Write(p[:room])
			c.trunc = true
		} else {
			c.buf.Write(p)
		}
	} else if len(p) > 0 {
		c.trunc = true
	}
	return len(p), nil
}
