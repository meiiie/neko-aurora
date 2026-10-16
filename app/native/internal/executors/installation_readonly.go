package executors

// ReadInstallation is identity metadata, never a readiness or lease assertion.
func (r *Registry) ReadInstallation() (InstallPin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.poisoned || r.state.Revoked || !ValidInstallPin(r.state.Pin) {
		return InstallPin{}, ErrUnavailable
	}
	if r.checkIdentity() != nil {
		return InstallPin{}, ErrUnavailable
	}
	return r.state.Pin, nil
}
