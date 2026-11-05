package computer

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDockerAvailabilityAvoidsRepeatedProbesDuringActiveWorkAndRecoversSoon(t *testing.T) {
	d := NewDocker()
	d.Path = filepath.Join(t.TempDir(), "missing-docker")
	d.checked = time.Now().Add(-10 * time.Second)
	if err := d.Available(); err != nil {
		t.Fatal("recent successful readiness should be reused during active work", err)
	}
	old := &SetupError{Code: "previous-failure"}
	d.checkErr = old
	d.checked = time.Now().Add(-4 * time.Second)
	if err := d.Available(); err == nil || err == old {
		t.Fatal("failed readiness must be probed again promptly")
	}
	d.checkErr = nil
	d.checked = time.Now().Add(-31 * time.Second)
	if err := d.Available(); err == nil {
		t.Fatal("stale success must not conceal a missing engine")
	}
}
