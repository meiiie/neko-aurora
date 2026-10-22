package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"nekoaurora.local/native/internal/executors"
	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/protocol"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	HostExecutable  string        `json:"hostExecutable"`
	CoreExecutable  string        `json:"coreExecutable"`
	Supervisor      string        `json:"supervisor"`
	FixtureEndpoint string        `json:"fixtureEndpoint"`
	CoreProfiles    *CoreProfiles `json:"-"`
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
	for _, prefix := range []string{"executor_", "context_", "core_", "host_", "runtime_", "admission_", "resume_", "receipt_", "operation_", "session_", "supervisor_", "task_", "frame_", "mcp_"} {
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
	state             string
	executor          *executors.Registry
	descriptor        executors.Descriptor
	lastProbe         time.Time
	store             *ledger.Store
	config            Config
	tasks             string
	mu                sync.Mutex
	active            map[string]bool
	stopIssues        map[string]string
	lastStopReconcile time.Time
	wg                sync.WaitGroup
}

func New(store *ledger.Store, c Config, state string) (*Manager, error) {
	if store.ReadOnly() {
		return nil, ledger.ErrReadOnly
	}
	absolute, e := filepath.Abs(state)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(absolute, 0700); e != nil {
		return nil, e
	}
	tasks := filepath.Join(absolute, "tasks")
	if e = os.MkdirAll(tasks, 0700); e != nil {
		return nil, e
	}
	m := &Manager{store: store, config: c, state: absolute, tasks: tasks, active: map[string]bool{}, stopIssues: map[string]string{}}
	store.ConfigureExecutor(nil)
	d, e := configuredDescriptor(c, absolute)
	if e == nil {
		m.descriptor = d
		m.executor, e = executors.Open(filepath.Join(absolute, "executors"), d)
	}
	store.ConfigureExecutor(m.executor)
	store.ConfigureContextAdmission(protocol.CoreContextAdmission(m.descriptor.CoreSHA256))
	if e = store.RecoverInterrupted(); e != nil {
		if m.executor != nil {
			m.executor.Close()
		}
		return nil, e
	}
	m.reconcileNativeStops()
	if e = store.ReconcileExecutors(); e != nil {
		if m.executor != nil {
			m.executor.Close()
		}
		return nil, e
	}
	m.healthExecutor()
	return m, nil
}
func (m *Manager) Run(ctx context.Context) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	defer func() {
		m.wg.Wait()
		if m.executor != nil {
			snap := m.executor.Snapshot()
			_ = m.executor.MarkUnavailable(snap.Pin, snap.BootID, "shutdown")
			_ = m.executor.Close()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			m.healthExecutor()
			if err := m.store.ReconcilePrincipals(); err != nil {
				continue
			}
			if err := m.store.ReconcileCoreProfiles(); err != nil {
				continue
			}
			if err := m.store.ReconcileExecutors(); err != nil {
				continue
			}
			m.reconcileNativeStops()
			if err := m.store.ReconcileAccounts(); err != nil {
				continue
			}
			if err := m.store.ReconcileTaskControls(); err != nil {
				continue
			}
			if err := m.store.AdvanceCollaborations(); err != nil {
				continue
			}
			for _, w := range m.store.Snapshot().Work {
				if w.Status != "waiting_host" || w.DesiredState != "running" {
					continue
				}
				if w.CoreProfilePin == nil && m.config.FixtureEndpoint == "" {
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
					if errors.Is(err, ledger.ErrInvalid) {
						if e := m.store.BlockContextAdmission(w.RequestID, w.Epoch); e != nil {
							_ = m.store.BlockRuntimeEnvelope(w.RequestID, w.Epoch)
						}
					}
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
	mode, selection := "--fixture-endpoint", m.config.FixtureEndpoint
	privateEnvironment := []string{}
	if w.CoreProfilePin != nil {
		if m.config.CoreProfiles == nil || m.config.CoreProfiles.Verify(ctx, *w.CoreProfilePin) != nil {
			m.failed(w, "core_profile_unavailable")
			return
		}
		var err error
		privateEnvironment, err = m.config.CoreProfiles.launchEnvironment(*w.CoreProfilePin)
		if err != nil {
			m.failed(w, "core_profile_unavailable")
			return
		}
		mode, selection = "--core-profile", w.CoreProfilePin.ProfileID
	}
	cmd := exec.Command(m.config.HostExecutable, "--runtime", m.config.CoreExecutable, "--supervisor", m.config.Supervisor, "--state-root", m.tasks, mode, selection, "--install-state-root", m.state)
	hideWindow(cmd)
	cmd.Env = append(hostEnvironment(), privateEnvironment...)
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
	specBytes, err := ledger.MarshalRuntimeLaunch(w, nil)
	if err != nil {
		input.Close()
		m.failed(w, "frame_bound")
		return
	}
	var spec json.RawMessage = specBytes
	encoder := json.NewEncoder(hostFrameWriter{input})
	startAttempted := false
	if err = m.store.Dispatch(w.RequestID, w.Epoch, func() error {
		bytes, e := os.ReadFile(m.config.HostExecutable)
		if e != nil || w.ExecutorPin == nil || hashBytes(bytes) != w.ExecutorPin.HostSHA256 {
			return executors.ErrUnavailable
		}
		startAttempted = true
		return cmd.Start()
	}); err != nil {
		input.Close()
		if startAttempted && cmd.Process == nil && w.CurrentAttemptLease != nil {
			proof, _ := executors.ProofFor(*w.CurrentAttemptLease)
			_ = m.store.MarkRuntimeStopped(w.RequestID, w.AttemptID, ledger.StoppedReceipt{Version: 2, Kind: "never_launched", ExecutorProof: &proof})
		}
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		m.failed(w, "host_dispatch_failed")
		return
	}
	defer input.Close()
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
	startupTimeout := time.AfterFunc(5*time.Second, func() { _ = cmd.Process.Kill() })
	err = encoder.Encode(spec)
	startupTimeout.Stop()
	if err != nil {
		input.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		m.failed(w, "host_dispatch_failed")
		return
	}
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
			if decoder.Decode(&receipt) == nil && strictProof(event.Data) == nil {
				_ = m.store.MarkRuntimeStopped(w.RequestID, w.AttemptID, receipt)
			}
			break
		}
		if rejected {
			continue
		}
		var result json.RawMessage = json.RawMessage(`{}`)
		switch event.Kind {
		case "launch_ready", "prompt_ready":
			proof, decodeErr := executors.ParseRuntimeProof(event.Data)
			if decodeErr != nil || w.CurrentAttemptLease == nil || !executors.MatchRuntimeProof(*w.CurrentAttemptLease, proof) || !m.store.RuntimeAllowed(w.RequestID, w.Epoch) {
				e = ledger.ErrConflict
			}
		case "admitted":
			var b ledger.CoreBinding
			if decodeStrictProofFrame(event.Data, &b) != nil || (w.ExecutorPolicyVersion == 1 && !ledger.ValidProcessGeneration(b.NativeProcessGeneration)) {
				e = ledger.ErrInvalid
			} else {
				admissionHTTPFixtureObserve("pre-bind", w, b, sequence)
				if b.NativeProcessID != 0 {
					e = m.store.BindRuntimeAttempt(w.RequestID, w.AttemptID, b)
				}
				if e == nil {
					e = m.store.Admit(w.RequestID, w.Epoch, b)
				}
				if e == nil {
					admissionHTTPFixtureObserve("post-durable-pre-ack", w, b, sequence)
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
		case "profile_tool_permission":
			result = m.store.PreflightCoreProfileTool(w.RequestID, w.Epoch, event.Data)
		case "tool":
			result, e = m.store.ExecuteTool(w.RequestID, w.Epoch, event.Data)
		case "capabilities":
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
		if current.Failure == "executor_lease_unavailable" {
			_ = m.store.ReconcileExecutors()
			return
		}
		if current.RuntimeStopped {
			reason := current.Failure
			if reason == "" {
				reason = "context_permission_changed"
			}
			_ = m.store.Finish(current.RequestID, current.Epoch, "suspended", reason)
		}
		return
	}
	if current.Epoch == w.Epoch && (current.Status == "starting" || current.Status == "running") {
		_ = m.store.Finish(w.RequestID, w.Epoch, "recovery_required", code)
	}
}
