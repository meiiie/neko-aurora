package runtime

import (
	"encoding/json"
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/ledger"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutorEnvironmentNeverEnablesUnconfiguredOrImportsProvider(t *testing.T) {
	t.Setenv("NEKO_EXECUTOR_TEST_UNCONFIGURED", "1")
	t.Setenv("NEKO_API_KEY", "fixture-only")
	for _, value := range hostEnvironment() {
		if strings.HasPrefix(value, "NEKO_") {
			t.Fatal("untrusted runtime env propagated")
		}
	}
}
func TestExecutorStateRootDigestIsConfiguredRootNotTaskDirectory(t *testing.T) {
	root := t.TempDir()
	tasks := filepath.Join(root, "tasks")
	os.MkdirAll(tasks, 0700)
	a, e := installRootHash(root)
	if e != nil {
		t.Fatal(e)
	}
	b, e := installRootHash(tasks)
	if e != nil || a == b {
		t.Fatal("wrong root scope")
	}
	again, e := installRootHash(filepath.Join(root, "tasks", ".."))
	if e != nil || again != a {
		t.Fatal("noncanonical root")
	}
}
func TestExecutorUnavailableManagerDoesNotCorruptOrClaimQueue(t *testing.T) {
	state := t.TempDir()
	s, e := ledger.Open(state)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	w, _, e := s.Accept(ledger.Input{BotID: "bot-meiiie", ClientNonce: "executor-manager-fixture", Text: "retained", Lane: "foreground"})
	if e != nil {
		t.Fatal(e)
	}
	m, e := New(s, Config{HostExecutable: filepath.Join(state, "missing-host"), CoreExecutable: filepath.Join(state, "missing-core")}, state)
	if e != nil {
		t.Fatal(e)
	}
	if m.executor != nil {
		t.Fatal("missing binaries became live executor")
	}
	got, e := s.Get(w.RequestID)
	if e != nil || got.Status != "waiting_host" || got.AttemptID != "" {
		t.Fatal("obligation lost", e)
	}
	if _, e = s.Claim(w.RequestID, w.Epoch); e == nil {
		t.Fatal("silent fallback")
	}
	if s.Snapshot().Runtime != "executor_unavailable" {
		t.Fatal(s.Snapshot().Runtime)
	}
	for _, capability := range s.Snapshot().Capabilities {
		if capability == "local_executor_lease_v1" {
			t.Fatal("unavailable registry capability")
		}
	}
}
func TestExecutorReceiptStrictUnknownCaseAndMissingFields(t *testing.T) {
	pin := executors.InstallPin{Version: 1, ExecutorID: "executor-" + strings.Repeat("a", 32), Generation: 1, Descriptor: executors.Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: strings.Repeat("2", 64), RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)}}
	l := executors.AttemptLease{Version: 1, ID: "executor-lease-" + strings.Repeat("b", 32), ExecutorID: pin.ExecutorID, Generation: 1, BootID: "executor-boot-" + strings.Repeat("c", 32), TaskScope: executors.TaskScope{RequestID: "req-" + strings.Repeat("d", 32), AttemptID: "attempt-" + strings.Repeat("e", 32), WorkEpoch: 1, AuthoritySHA256: strings.Repeat("f", 64)}, Fence: 1, IssuedAtMs: 1, ExpiresAtMs: 2}
	proof, _ := executors.ProofFor(l)
	raw, _ := json.Marshal(map[string]any{"executorProof": proof})
	if e := strictProof(raw); e != nil {
		t.Fatal(e)
	}
	var obj map[string]map[string]any
	json.Unmarshal(raw, &obj)
	for _, key := range []string{"unknown", "Generation"} {
		bad := map[string]map[string]any{"executorProof": {}}
		for k, v := range obj["executorProof"] {
			bad["executorProof"][k] = v
		}
		bad["executorProof"][key] = 1
		b, _ := json.Marshal(bad)
		if strictProof(b) == nil {
			t.Fatal(key)
		}
	}
}
