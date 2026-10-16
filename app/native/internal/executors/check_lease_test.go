package executors

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCheckLeaseReadOnlyTimeAndIdentity(t *testing.T) {
	for _, kind := range []string{"valid", "heartbeat", "lease", "rewind", "identity", "fence"} {
		t.Run(kind, func(t *testing.T) {
			r, dir, now := openFixture(t)
			snapshot := readyFixture(t, r)
			lease := issueFixture(t, r, snapshot.Pin, fixtureScope())
			if kind == "lease" {
				state := cloneState(r.state)
				state.ReadyUntilMs = lease.ExpiresAtMs + 1
				if err := r.commit(state); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "heartbeat":
				*now = time.UnixMilli(int64(r.state.ReadyUntilMs))
			case "lease":
				*now = time.UnixMilli(int64(lease.ExpiresAtMs))
			case "rewind":
				*now = time.UnixMilli(int64(r.state.ObservedAtMs) - 1)
			case "identity":
				raw, _ := os.ReadFile(filepath.Join(dir, stateName))
				if err := os.WriteFile(filepath.Join(dir, stateName), append(raw, '\n'), 0600); err != nil {
					t.Fatal(err)
				}
			case "fence":
				lease.Fence++
			}
			before, err := os.ReadFile(filepath.Join(dir, stateName))
			if err != nil {
				t.Fatal(err)
			}
			stateBefore := cloneState(r.state)
			poisoned := r.poisoned
			method := reflect.ValueOf(r).MethodByName("CheckLease")
			if !method.IsValid() {
				t.Fatal("missing pure exact-lease check")
			}
			values := method.Call([]reflect.Value{reflect.ValueOf(snapshot.Pin), reflect.ValueOf(lease)})
			if len(values) != 1 {
				t.Fatal("check lease result schema")
			}
			passed := values[0].IsNil()
			if passed != (kind == "valid") {
				t.Fatal("exact current lease/time/identity result", kind)
			}
			after, _ := os.ReadFile(filepath.Join(dir, stateName))
			if !bytes.Equal(before, after) || !reflect.DeepEqual(stateBefore, r.state) || poisoned != r.poisoned {
				t.Fatal("read-only check changed registry bytes/state")
			}
		})
	}
}
