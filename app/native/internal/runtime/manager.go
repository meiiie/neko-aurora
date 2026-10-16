package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/protocol"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	HostExecutable  string `json:"hostExecutable"`
	CoreExecutable  string `json:"coreExecutable"`
	Supervisor      string `json:"supervisor"`
	FixtureEndpoint string `json:"fixtureEndpoint"`
}
type diagnostics struct{ bytes []byte }

func (d *diagnostics) Write(p []byte) (int, error) {
	n := len(p)
	limit := 512 - len(d.bytes)
	if limit > 0 {
		if len(p) > limit {
			p = p[:limit]
		}
		d.bytes = append(d.bytes, p...)
	}
	return n, nil
}
func (d *diagnostics) Code() string {
	fields := strings.Fields(string(d.bytes))
	if len(fields) == 0 {
		return ""
	}
	code := fields[0]
	if len(code) > 64 {
		return ""
	}
	for _, c := range code {
		if !(c >= 'a' && c <= 'z' || c == '_') {
			return ""
		}
	}
	for _, prefix := range []string{"core_", "host_", "runtime_", "admission_", "resume_", "receipt_", "operation_", "session_", "supervisor_", "task_", "frame_", "mcp_"} {
		if strings.HasPrefix(code, prefix) {
			return code
		}
	}
	return ""
}

func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	canonical, err := protocol.Canonical(b)
	if err != nil {
		return c, err
	}
	d := json.NewDecoder(strings.NewReader(string(canonical)))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, err
	}
	if !strings.HasPrefix(c.FixtureEndpoint, "http://127.0.0.1:") || !strings.HasSuffix(c.FixtureEndpoint, "/v1") {
		return c, errors.New("fixture_endpoint_invalid")
	}
	for _, path := range []string{c.HostExecutable, c.CoreExecutable, c.Supervisor} {
		if !filepath.IsAbs(path) {
			return c, errors.New("absolute_launch_path_required")
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return c, errors.New("launch_file_missing")
		}
	}
	return c, nil
}

type Manager struct {
	store  *ledger.Store
	config Config
	tasks  string
	mu     sync.Mutex
	active map[string]bool
	wg     sync.WaitGroup
}

func New(store *ledger.Store, c Config, state string) (*Manager, error) {
	tasks, err := filepath.Abs(filepath.Join(state, "tasks"))
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(tasks, 0700); err != nil {
		return nil, err
	}
	if err = store.RecoverInterrupted(); err != nil {
		return nil, err
	}
	store.Runtime("core_fixture_ready")
	return &Manager{store: store, config: c, tasks: tasks, active: map[string]bool{}}, nil
}
func (m *Manager) Run(ctx context.Context) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	defer m.wg.Wait()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			for _, w := range m.store.Snapshot().Work {
				if w.Status != "waiting_host" || w.DesiredState != "running" {
					continue
				}
				slot := w.BotID + ":" + w.Lane
				m.mu.Lock()
				busy := m.active[slot]
				background := 0
				for key := range m.active {
					if strings.HasSuffix(key, ":background") {
						background++
					}
				}
				busy = busy || len(m.active) >= 4 || (w.Lane == "background" && background >= 3)
				if !busy {
					m.active[slot] = true
				}
				m.mu.Unlock()
				if busy {
					continue
				}
				claimed, err := m.store.Claim(w.RequestID, w.Epoch)
				if err != nil {
					m.mu.Lock()
					delete(m.active, slot)
					m.mu.Unlock()
					continue
				}
				m.wg.Add(1)
				go func(w ledger.Work, slot string) {
					defer m.wg.Done()
					defer func() { m.mu.Lock(); delete(m.active, slot); m.mu.Unlock() }()
					m.execute(ctx, w)
				}(claimed, slot)
			}
		}
	}
}
func (m *Manager) execute(ctx context.Context, w ledger.Work) {
	cmd := exec.Command(m.config.HostExecutable, "--runtime", m.config.CoreExecutable, "--supervisor", m.config.Supervisor, "--state-root", m.tasks, "--fixture-endpoint", m.config.FixtureEndpoint)
	hideWindow(cmd)
	cmd.Env = []string{}
	for _, key := range []string{"SystemRoot", "WINDIR", "TEMP", "TMP", "USERPROFILE"} {
		if value := os.Getenv(key); value != "" {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		m.failed(w, "host_pipe_failed")
		return
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		m.failed(w, "host_pipe_failed")
		return
	}
	var diagnostic diagnostics
	cmd.Stderr = &diagnostic
	if err = cmd.Start(); err != nil {
		input.Close()
		m.failed(w, "host_launch_failed")
		return
	}
	defer input.Close()
	spec := map[string]any{"requestId": w.RequestID, "text": m.prompt(w), "botId": w.BotID}
	if w.Core != nil {
		spec["resume"] = w.Core
	}
	encoder := json.NewEncoder(input)
	if encoder.Encode(spec) != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		m.failed(w, "host_transport_failed")
		return
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				input.Close()
				return
			case <-tick.C:
				current, e := m.store.Get(w.RequestID)
				if e != nil || current.Epoch != w.Epoch || current.DesiredState != "running" || !m.store.RuntimeAllowed(w.RequestID, w.Epoch) {
					input.Close()
					return
				}
			}
		}
	}()
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 65536)
	var sequence uint64
	completed := false
	rejected := false
	for scanner.Scan() {
		var event struct {
			Sequence uint64          `json:"sequence"`
			Kind     string          `json:"kind"`
			Data     json.RawMessage `json:"data"`
		}
		canonical, e := protocol.Canonical(scanner.Bytes())
		if e != nil {
			break
		}
		d := json.NewDecoder(strings.NewReader(string(canonical)))
		d.DisallowUnknownFields()
		if d.Decode(&event) != nil || event.Sequence != sequence+1 {
			break
		}
		sequence = event.Sequence
		if event.Kind == "runtime_stopped" {
			var receipt ledger.StoppedReceipt
			decoder := json.NewDecoder(strings.NewReader(string(event.Data)))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&receipt) == nil {
				_ = m.store.MarkRuntimeStopped(w.RequestID, w.AttemptID, receipt)
			}
			break
		}
		if rejected {
			continue
		}
		var result json.RawMessage = json.RawMessage(`{}`)
		switch event.Kind {
		case "admitted":
			var b ledger.CoreBinding
			if json.Unmarshal(event.Data, &b) != nil {
				e = ledger.ErrInvalid
			} else {
				if b.NativeProcessID != 0 {
					e = m.store.BindRuntimeAttempt(w.RequestID, w.AttemptID, b)
				}
				if e == nil {
					e = m.store.Admit(w.RequestID, w.Epoch, b)
				}
			}
		case "text":
			var v struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(event.Data, &v) != nil {
				e = ledger.ErrInvalid
			} else {
				e = m.store.AppendText(w.RequestID, w.Epoch, v.Text)
			}
		case "tool":
			result, e = m.store.ExecuteTool(w.RequestID, w.Epoch, event.Data)
		case "completed":
			e = m.store.Finish(w.RequestID, w.Epoch, "completed", "")
			completed = e == nil
		default:
			e = ledger.ErrInvalid
		}
		if encoder.Encode(map[string]any{"sequence": sequence, "ok": e == nil, "result": result}) != nil || e != nil {
			input.Close()
			rejected = true
		}
	}
	input.Close()
	waitErr := cmd.Wait()
	if !completed {
		code := "core_transport_interrupted"
		if diagnostic.Code() != "" {
			code = diagnostic.Code()
		}
		if waitErr == nil {
			code = "core_protocol_incomplete"
		}
		m.failed(w, code)
	}
}
func (m *Manager) failed(w ledger.Work, code string) {
	current, err := m.store.Get(w.RequestID)
	if err != nil {
		return
	}
	if current.Status == "cancelling" {
		_ = m.store.Finish(current.RequestID, current.Epoch, "cancelled", "outcome_not_rollback")
		return
	}
	if current.Status == "suspending" {
		if current.RuntimeStopped {
			_ = m.store.Finish(current.RequestID, current.Epoch, "suspended", "context_permission_changed")
		}
		return
	}
	if current.Epoch == w.Epoch && (current.Status == "starting" || current.Status == "running") {
		_ = m.store.Finish(w.RequestID, w.Epoch, "recovery_required", code)
	}
}
func (m *Manager) prompt(w ledger.Work) string {
	if w.Core != nil {
		return "Continue the existing request from its durable Core task. Use recorded results; do not repeat completed effects. Preserve denied or unknown outcomes."
	}
	return w.RuntimePrompt()
}
