package executors

import (
	"crypto/sha256"
	"path/filepath"
)

func (r *Registry) CheckLease(pin InstallPin, lease AttemptLease) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !ValidLease(lease) {
		return ErrInvalid
	}
	if err := r.active(pin, lease.BootID); err != nil {
		return err
	}
	if r.lock == nil {
		return ErrUnavailable
	}
	marker, err := readMarker(r.lock)
	if err != nil || marker != r.state.Pin.ExecutorID {
		return ErrUnavailable
	}
	raw, err := readSingle(filepath.Join(r.dir, stateName), maxStateBytes)
	if err != nil || sha256.Sum256(raw) != r.diskSHA {
		return ErrUnavailable
	}
	instant, err := r.now()
	if err != nil || instant < r.state.ObservedAtMs || instant < lease.IssuedAtMs {
		return ErrUnavailable
	}
	if instant >= lease.ExpiresAtMs {
		return ErrExpired
	}
	if !r.state.Ready || instant >= r.state.ReadyUntilMs {
		return ErrUnavailable
	}
	if lease.Fence > r.state.Fence {
		return ErrConflict
	}
	return r.allowed(pin, lease)
}
