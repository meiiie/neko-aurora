package duet

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

// Model is what an agent turn runs on. Two implementations:
//   - Core: Neko Core (core/, TypeScript on Bun) via its one-shot CLI
//     `neko run --profile zai --no-tools`, i.e. Core's own GLM Coding Plan
//     provider (glm-5.3). This is the default whenever Bun and core/ exist.
//   - AI (ai.go): the same z.ai provider over HTTP, used when Core is not
//     installed (e.g. a slim container) or for tests with a fake endpoint.
type Model interface {
	Stream(ctx context.Context, msgs []ChatMsg, onDelta func(string)) (string, error)
	Configured() bool
	Runtime() string
	ModelName() string
}

func (a *AI) Runtime() string   { return "direct-http" }
func (a *AI) ModelName() string { return a.Model }

type Core struct {
	Bun, Entry, Home, Model, Profile string
	key                              string
}

// CoreFromEnv locates Bun and core/bin/neko.ts. NEKO_CORE_DIR overrides the
// search; DUET_RUNTIME=http disables Core.
func CoreFromEnv(stateDir string) *Core {
	if os.Getenv("DUET_RUNTIME") == "http" {
		return nil
	}
	bun, err := exec.LookPath("bun")
	if err != nil {
		return nil
	}
	var dirs []string
	if v := os.Getenv("NEKO_CORE_DIR"); v != "" {
		dirs = append(dirs, v)
	} else {
		starts := []string{}
		if wd, err := os.Getwd(); err == nil {
			starts = append(starts, wd)
		}
		if exe, err := os.Executable(); err == nil {
			starts = append(starts, filepath.Dir(exe))
		}
		for _, s := range starts {
			for d := s; ; d = filepath.Dir(d) {
				dirs = append(dirs, filepath.Join(d, "core"))
				if filepath.Dir(d) == d {
					break
				}
			}
		}
	}
	for _, d := range dirs {
		entry := filepath.Join(d, "bin", "neko.ts")
		if st, err := os.Stat(entry); err == nil && st.Mode().IsRegular() {
			if _, err := os.Stat(filepath.Join(d, "node_modules")); err != nil {
				continue // dependencies not installed (bun install)
			}
			model := os.Getenv("DUET_AI_MODEL")
			if model == "" {
				model = DefaultModel
			}
			return &Core{Bun: bun, Entry: entry, Home: filepath.Join(stateDir, "core-home"), Model: model, Profile: "zai", key: os.Getenv("ZAI_API_KEY")}
		}
	}
	return nil
}

func (c *Core) Configured() bool  { return c != nil && c.key != "" }
func (c *Core) Runtime() string   { return "neko-core-cli" }
func (c *Core) ModelName() string { return c.Model }

// prompt flattens the chat into one instruction for `neko run`.
func corePrompt(msgs []ChatMsg) string {
	var sb strings.Builder
	var last string
	hist := []ChatMsg{}
	for i, m := range msgs {
		switch {
		case m.Role == "system":
			sb.WriteString(m.Content + "\n\n")
		case i == len(msgs)-1 && m.Role == "user":
			last = m.Content
		default:
			hist = append(hist, m)
		}
	}
	if len(hist) > 0 {
		sb.WriteString("Lịch sử hội thoại (cũ → mới):\n")
		for _, m := range hist {
			who := "Darren"
			if m.Role == "assistant" {
				who = "Bạn"
			}
			sb.WriteString(who + ": " + m.Content + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Yêu cầu hiện tại của Darren:\n" + last + "\n\nChỉ trả lời nội dung cho Darren (không gọi công cụ, không nói về mã nguồn).")
	p := sb.String()
	if len(p) > 100_000 { // argv limit; keep the newest context
		p = p[len(p)-100_000:]
	}
	return p
}

var ErrCore = errors.New("core_failed")

// coreStats starts the usage summary `neko run` prints after the reply.
const coreStats = "\n[session cumulative:"

func (c *Core) Stream(ctx context.Context, msgs []ChatMsg, onDelta func(string)) (string, error) {
	if !c.Configured() {
		return "", ErrNoAI
	}
	if err := os.MkdirAll(c.Home, 0o700); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, c.Bun, c.Entry, "--profile", c.Profile, "--no-tools", "run", corePrompt(msgs))
	cmd.Dir = c.Home
	cmd.Env = []string{"HOME=" + c.Home, "PATH=" + os.Getenv("PATH"), "ZAI_API_KEY=" + c.key, "NEKO_MODEL=" + c.Model, "NO_COLOR=1", "TERM=dumb", "LANG=C.UTF-8"}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr limited
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	var full strings.Builder
	r := bufio.NewReader(out)
	buf := make([]byte, 2048)
	emitted, done := 0, false
	for {
		n, rerr := r.Read(buf)
		if n > 0 && !done {
			full.WriteString(string(buf[:n]))
			// Core appends a usage summary after the reply; never stream it.
			text := full.String()
			safe := len(text) - len(coreStats)
			if i := strings.Index(text, coreStats); i >= 0 {
				safe, done = i, true
			}
			for safe > emitted && !utf8.RuneStart(text[safe-1]) && safe < len(text) {
				safe--
			}
			if safe > emitted {
				onDelta(strings.ToValidUTF8(text[emitted:safe], ""))
				emitted = safe
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				err = rerr
			}
			break
		}
	}
	werr := cmd.Wait()
	reply := full.String()
	if i := strings.Index(reply, coreStats); i >= 0 {
		reply = reply[:i]
	}
	reply = strings.ToValidUTF8(reply, "")
	if !done && len(reply) > emitted {
		onDelta(reply[emitted:])
	}
	if ctx.Err() != nil {
		return reply, ctx.Err()
	}
	if werr != nil {
		line := strings.TrimSpace(strings.SplitN(stderr.String(), "\n", 2)[0])
		return reply, fmt.Errorf("%w: %s", ErrCore, scrub(clean(line, 160), c.key))
	}
	return reply, err
}

type limited struct {
	mu sync.Mutex
	b  []byte
}

func (l *limited) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := 4096 - len(l.b); room > 0 {
		if len(p) > room {
			l.b = append(l.b, p[:room]...)
		} else {
			l.b = append(l.b, p...)
		}
	}
	return len(p), nil
}
func (l *limited) String() string { l.mu.Lock(); defer l.mu.Unlock(); return string(l.b) }
