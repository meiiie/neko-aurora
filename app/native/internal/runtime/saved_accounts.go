package runtime

import (
	"context"
	"os"
	"time"

	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/providerexec"
)

// savedAccountSession owns only the short-lived authority witness. It does not
// prepare credentials, dispatch a host, admit Core, or reset a task's budget.
// The Rust/Core execution bootstrap must be wired before Run can select it.
type savedAccountSession struct {
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	retireErr error // published by closing done
}

func (s *savedAccountSession) Context() context.Context { return s.ctx }

// Close waits until retirement has been attempted, including on cancellation.
// A retirement failure is surfaced; the last issued proof still expires within
// five seconds. Callers must also stop their host, not rely on that expiry.
func (s *savedAccountSession) Close() error {
	s.cancel()
	<-s.done
	return s.retireErr
}

// startSavedAccountSession accepts private transport configuration only from
// trusted runtime code. Browser Work copies cannot choose witness authority.
// It is intentionally not a runtime activation switch.
func (m *Manager) startSavedAccountSession(ctx context.Context, work ledger.Work, path, revisionKey string) (*savedAccountSession, error) {
	if ctx.Err() != nil {
		return nil, providerexec.ErrSavedAuthority
	}
	current, e := m.store.Get(work.RequestID)
	if e != nil || current.AttemptID != work.AttemptID || current.Epoch != work.Epoch || current.CurrentAttemptLease == nil {
		return nil, providerexec.ErrSavedAuthority
	}
	binding, e := m.store.SavedAccountTaskBinding(current.RequestID, current.AttemptID, current.Epoch)
	if e != nil {
		return nil, providerexec.ErrSavedAuthority
	}
	lease := current.CurrentAttemptLease
	sessionCtx, cancel := context.WithCancel(ctx)
	controller, e := providerexec.OpenWitness(providerexec.WitnessConfig{Path: path, Binding: binding, ServiceGeneration: lease.BootID, AuthoritySHA256: lease.AuthoritySHA256, RevisionKey: revisionKey, CurrentTask: func(b providerexec.SavedTaskBinding) bool {
		return sessionCtx.Err() == nil && m.store.CurrentSavedAccountTask(b)
	}})
	if e != nil {
		cancel()
		return nil, providerexec.ErrSavedAuthority
	}
	// Reserve this path atomically across sessions before issuing anything.
	// The empty owner directory is retained as a restart tombstone, including
	// on failure. Never remove another owner's reservation or adopt its proof.
	// OpenWitness above has already validated the private path ancestry.
	if e = os.Mkdir(path+".owner", 0700); e != nil {
		cancel()
		return nil, providerexec.ErrSavedAuthority
	}
	if _, e = os.Lstat(path); !os.IsNotExist(e) {
		cancel()
		return nil, providerexec.ErrSavedAuthority
	}
	if _, e = controller.Issue(); e != nil {
		cancel()
		_ = controller.Retire()
		return nil, providerexec.ErrSavedAuthority
	}
	s := &savedAccountSession{ctx: sessionCtx, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer cancel()
		defer func() { s.retireErr = controller.Retire() }()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-sessionCtx.Done():
				return
			case <-tick.C:
				if _, e := controller.Issue(); e != nil {
					return
				}
			}
		}
	}()
	return s, nil
}
