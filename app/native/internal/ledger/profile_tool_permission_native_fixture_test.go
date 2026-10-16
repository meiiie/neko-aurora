//go:build neko_native_fixture

package ledger

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"nekobot.local/native/internal/executors"
)

func TestProfileToolPermissionRegistryDiskNoMutation(t *testing.T) {
	for _, kind := range []string{"valid", "heartbeat", "lease", "rewind"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _, w := permissionGroupStage(t, 3)
			registry := s.executor
			dir := reflect.ValueOf(registry).Elem().FieldByName("dir").String()
			instant := time.UnixMilli(int64(w.CurrentAttemptLease.IssuedAtMs))
			switch kind {
			case "valid":
				instant = instant.Add(time.Millisecond)
			case "heartbeat":
				instant = instant.Add(executors.HeartbeatMilliseconds * time.Millisecond)
			case "lease":
				instant = time.UnixMilli(int64(w.CurrentAttemptLease.ExpiresAtMs))
			case "rewind":
				instant = instant.Add(-time.Second)
			}
			if err := executors.FixtureSetClock(registry, func() time.Time { return instant }); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "executor-v1.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			journalBefore := permissionJournal(t, s)
			result := permissionInvoke(t, s, w, permissionFixtureFrame(t, w, permissionWriteArguments()))
			if result["allowed"] != (kind == "valid") {
				t.Fatal("registry current-time authority result", kind)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || !bytes.Equal(journalBefore, permissionJournal(t, s)) {
				t.Fatal("preflight changed registry or journal bytes", kind)
			}
		})
	}
}
