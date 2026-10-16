//go:build neko_native_fixture

package executors

import "time"

// FixtureSetClock is compiled only for explicit owned native validation builds.
// The supplied clock must be bounded and must not reenter the registry.
func FixtureSetClock(r *Registry, now func() time.Time) error {
	if r == nil || now == nil {
		return ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.poisoned {
		return ErrUnavailable
	}
	r.clock = now
	return nil
}
