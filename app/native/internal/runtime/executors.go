package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"nekoaurora.local/native/internal/executors"
	"nekoaurora.local/native/internal/protocol"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"
)

const executorCapabilities = `["executor_lease_v1","native_pid_session_stop_v1"]`

func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func installRootHash(path string) (string, error) {
	absolute, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	resolved, e := filepath.EvalSymlinks(absolute)
	if e != nil {
		return "", e
	}
	if goruntime.GOOS == "windows" {
		resolved = strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(resolved, `\\?\`), `\`, "/"))
	}
	return hashBytes([]byte(strings.TrimRight(resolved, "/"))), nil
}
func configuredDescriptor(c Config, state string) (executors.Descriptor, error) {
	var d executors.Descriptor
	host, e := os.ReadFile(c.HostExecutable)
	if e != nil {
		return d, e
	}
	core, e := os.ReadFile(c.CoreExecutable)
	if e != nil {
		return d, e
	}
	root, e := installRootHash(state)
	if e != nil {
		return d, e
	}
	d = executors.Descriptor{HostSHA256: hashBytes(host), CoreSHA256: hashBytes(core), RootSHA256: root, CapabilitiesSHA256: hashBytes([]byte(executorCapabilities))}
	return d, nil
}
func hostEnvironment() []string {
	env := []string{}
	for _, key := range []string{"SystemRoot", "WINDIR", "TEMP", "TMP", "USERPROFILE"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

type probeOutput struct {
	bytes.Buffer
	overflow bool
}

func (w *probeOutput) Write(p []byte) (int, error) {
	n := len(p)
	if w.Len()+n > 1024 {
		w.overflow = true
		return n, nil
	}
	return w.Buffer.Write(p)
}
func (m *Manager) probeExecutor() error {
	if m.executor == nil {
		return executors.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.config.HostExecutable, "--executor-probe", "--runtime", m.config.CoreExecutable, "--supervisor", m.config.Supervisor, "--install-state-root", m.state)
	hideWindow(cmd)
	cmd.Env = hostEnvironment()
	var output probeOutput
	cmd.Stdout = &output
	var diagnostic diagnostics
	cmd.Stderr = &diagnostic
	if e := cmd.Run(); e != nil || output.overflow {
		return executors.ErrUnavailable
	}
	canonical, e := protocol.Canonical(bytes.TrimSpace(output.Bytes()))
	if e != nil {
		return e
	}
	var d executors.Descriptor
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&d) != nil {
		return executors.ErrUnavailable
	}
	rendered, _ := json.Marshal(d)
	exact, _ := protocol.Canonical(rendered)
	if !bytes.Equal(canonical, exact) || d != m.descriptor {
		return errors.New("executor_descriptor_mismatch")
	}
	snap := m.executor.Snapshot()
	if snap.Pin.Descriptor != d {
		return executors.ErrUnavailable
	}
	if snap.Ready {
		return m.executor.Heartbeat(snap.Pin, snap.BootID)
	}
	return m.executor.MarkReady(snap.Pin, snap.BootID)
}
func (m *Manager) healthExecutor() {
	if time.Since(m.lastProbe) < 5*time.Second {
		return
	}
	m.lastProbe = time.Now()
	if e := m.probeExecutor(); e != nil {
		if m.executor != nil {
			snap := m.executor.Snapshot()
			_ = m.executor.MarkUnavailable(snap.Pin, snap.BootID, "host_unavailable")
		}
		m.store.Runtime("executor_unavailable")
		return
	}
	if m.config.CoreProfiles != nil {
		m.store.Runtime("core_profile_bridge_ready")
	} else {
		m.store.Runtime("core_fixture_ready")
	}
}

func strictProof(data []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return executors.ErrInvalid
	}
	proof, ok := fields["executorProof"]
	if !ok {
		return nil
	}
	_, e := executors.ParseRuntimeProof(proof)
	return e
}
func decodeStrictProofFrame(data []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	return strictProof(data)
}
