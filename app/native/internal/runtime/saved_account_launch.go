package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/protocol"
	"nekobot.local/native/internal/providerexec"
)

// Trusted runtime dependencies only. No JSON-config or browser activation path.
// Existing directories must be prepared by the trusted host lifecycle first.
type savedAccountLaunchPaths struct{ ConnectionsRoot, PrivateRoot string }
type savedAccountWitnessMetadata struct {
	Path              string `json:"path"`
	ServiceGeneration string `json:"serviceGeneration"`
	AuthoritySHA256   string `json:"authoritySHA256"`
}
type savedAccountLaunchEnvelope struct {
	Version int                         `json:"version"`
	Purpose string                      `json:"purpose"`
	Launch  json.RawMessage             `json:"launch"`
	Request json.RawMessage             `json:"request"`
	Witness savedAccountWitnessMetadata `json:"witness"`
	Root    string                      `json:"root"`
}

// All fields are private: accidental JSON serialization emits no authorization.
// A transport may borrow its frame only during dispatch; it must not retain/log it.
type savedAccountLaunch struct {
	mu          sync.Mutex
	used        bool
	frame       []byte
	manager     *Manager
	catalog     *connections.Store
	selection   connections.ConsumerSelection
	binding     providerexec.SavedTaskBinding
	session     *savedAccountSession
	witnessPath string
}

func (l *savedAccountLaunch) Close() error {
	l.mu.Lock()
	l.used = true
	clear(l.frame)
	l.frame = nil
	l.mu.Unlock()
	return l.session.Close()
}
func (l *savedAccountLaunch) current() bool {
	return l.session.Context().Err() == nil && l.manager.store.CurrentSavedAccountTask(l.binding) && l.catalog.ConsumerCurrent(l.selection)
}

// dispatch is single-use and remains disconnected from Manager.Run. The future
// native transport must perform its own live checks through host teardown.
func (l *savedAccountLaunch) dispatch(transport func(context.Context, []byte) error) (err error) {
	l.mu.Lock()
	if l.used {
		l.mu.Unlock()
		return providerexec.ErrSavedAuthority
	}
	l.used = true
	frame := l.frame
	l.frame = nil
	l.mu.Unlock()
	success := false
	defer func() {
		clear(frame)
		if !success {
			err = errors.Join(providerexec.ErrSavedAuthority, l.session.Close())
		}
	}()
	if transport == nil || !l.current() {
		return providerexec.ErrSavedAuthority
	}
	if transport(l.session.Context(), frame) != nil || !l.current() {
		return providerexec.ErrSavedAuthority
	}
	success = true
	return nil
}
func canonicalLaunchDirectory(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 4096 {
		return false
	}
	for _, c := range path {
		if c < 32 || c == 127 {
			return false
		}
	}
	actual, e := filepath.EvalSymlinks(path)
	if e != nil || actual != path {
		return false
	}
	st, e := os.Stat(path)
	return e == nil && st.IsDir()
}
func (m *Manager) prepareSavedAccountLaunch(ctx context.Context, supplied ledger.Work, catalog *connections.Store, paths savedAccountLaunchPaths) (*savedAccountLaunch, error) {
	deny := providerexec.ErrSavedAuthority
	if ctx.Err() != nil || catalog == nil {
		return nil, deny
	}
	current, e := m.store.Get(supplied.RequestID)
	if e != nil || current.AttemptID != supplied.AttemptID || current.Epoch != supplied.Epoch || current.Account == nil || current.CurrentAttemptLease == nil || current.Core != nil {
		return nil, deny
	}
	binding, e := m.store.SavedAccountTaskBinding(current.RequestID, current.AttemptID, current.Epoch)
	if e != nil {
		return nil, deny
	}
	selection, e := catalog.SelectConsumer(*current.Account)
	if e != nil {
		return nil, deny
	}
	root := filepath.Join(m.tasks, current.RequestID)
	home := filepath.Join(root, ".core-home")
	private := filepath.Join(paths.PrivateRoot, current.RequestID, current.AttemptID)
	if m.tasks != filepath.Join(m.state, "tasks") || paths.ConnectionsRoot != filepath.Join(m.state, "connections") || paths.PrivateRoot != filepath.Join(m.state, "saved-accounts") {
		return nil, deny
	}
	for _, dir := range []string{m.state, m.tasks, root, home, paths.ConnectionsRoot, paths.PrivateRoot, private} {
		if !canonicalLaunchDirectory(dir) {
			return nil, deny
		}
	}
	witness := filepath.Join(private, "witness.json")
	capsule := filepath.Join(private, "capsule.json")
	if _, e := os.Lstat(capsule); !os.IsNotExist(e) {
		return nil, deny
	}
	var key [32]byte
	if _, e = rand.Read(key[:]); e != nil {
		return nil, deny
	}
	defer clear(key[:])
	revisionKey := hex.EncodeToString(key[:])
	request, e := catalog.PrivateSavedTaskFrame(selection, binding, paths.ConnectionsRoot, capsule, home, revisionKey)
	if e != nil {
		return nil, deny
	}
	defer clear(request)
	launch, e := ledger.MarshalRuntimeLaunch(current, nil)
	if e != nil {
		return nil, deny
	}
	lease := current.CurrentAttemptLease
	raw, e := json.Marshal(savedAccountLaunchEnvelope{1, "saved_account_launch_v1", launch, request, savedAccountWitnessMetadata{witness, lease.BootID, lease.AuthoritySHA256}, root})
	if e != nil {
		return nil, deny
	}
	defer clear(raw)
	frame, e := protocol.Canonical(raw)
	if e != nil || len(raw) > 64000 || len(frame) > 64000 {
		clear(frame)
		return nil, deny
	}
	session, e := m.startSavedAccountSession(ctx, current, witness, revisionKey)
	if e != nil {
		clear(frame)
		return nil, deny
	}
	out := &savedAccountLaunch{frame: frame, manager: m, catalog: catalog, selection: selection, binding: binding, session: session, witnessPath: witness}
	if !out.current() {
		return nil, errors.Join(deny, out.Close())
	}
	return out, nil
}
