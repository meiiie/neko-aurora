package executors

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestReadInstallationDoesNotObserveExpiryOrMutateLease(t *testing.T) {
	r, dir, now := openFixture(t)
	s := readyFixture(t, r)
	issueFixture(t, r, s.Pin, fixtureScope())
	before, _ := os.ReadFile(filepath.Join(dir, stateName))
	state := cloneState(r.state)
	*now = now.Add(2 * time.Hour)
	for i := 0; i < 3; i++ {
		pin, e := r.ReadInstallation()
		if e != nil || pin != s.Pin {
			t.Fatal("identity unavailable", e)
		}
	}
	after, _ := os.ReadFile(filepath.Join(dir, stateName))
	if !bytes.Equal(before, after) || !reflect.DeepEqual(state, r.state) {
		t.Fatal("readonly metadata mutated registry")
	}
}
func TestReadInstallationRevokedClosedAndCorruptFailClosed(t *testing.T) {
	for _, mode := range []string{"revoked", "closed", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			r, dir, _ := openFixture(t)
			pin, e := r.ReadInstallation()
			if e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "revoked":
				_, _, e = r.Revoke(pin, "readonly-installation-revoke")
			case "closed":
				e = r.Close()
			case "corrupt":
				e = os.WriteFile(filepath.Join(dir, stateName), []byte("{}"), 0600)
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = r.ReadInstallation(); e == nil {
				t.Fatal("invalid installation exposed")
			}
		})
	}
}
