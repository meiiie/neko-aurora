//go:build neko_admission_http_fixture

package runtime

import (
	"encoding/json"
	"nekoaurora.local/native/internal/executors"
	"nekoaurora.local/native/internal/ledger"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdmissionHTTPFixtureConfigDeniesUnscopedAndMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	marker := filepath.Join(dir, "admission-observation.json")
	valid := map[string]any{"version": 1, "clientNonce": "owned-http-config-negative", "stage": "pre-bind", "observationPath": marker}
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"version", "version", 2}, {"foreign-nonce", "clientNonce", "ordinary-client-nonce"}, {"wrong-stage", "stage", "after-all-effects"}, {"relative-marker", "observationPath", "admission-observation.json"}, {"outside-config-dir", "observationPath", filepath.Join(dir, "other", "admission-observation.json")}, {"marker-filename", "observationPath", filepath.Join(dir, "other.json")}, {"unknown-field", "bootId", "untrusted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := map[string]any{}
			for k, v := range valid {
				copy[k] = v
			}
			copy[tc.key] = tc.value
			raw, _ := json.Marshal(copy)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, ok := readAdmissionHTTPFixtureConfig(path); ok {
				t.Fatal("invalid fixture configuration admitted")
			}
		})
	}
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", 4097)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readAdmissionHTTPFixtureConfig(path); ok {
		t.Fatal("unbounded input admitted")
	}
	raw, _ := json.Marshal(valid)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readAdmissionHTTPFixtureConfig(path); !ok {
		t.Fatal("valid owned fixture denied")
	}
}

func TestAdmissionHTTPFixtureWrongTargetAndReceiptCannotActivate(t *testing.T) {
	proof := executors.RuntimeProof{Version: 1, ExecutorID: "executor-" + strings.Repeat("a", 32), Generation: 1, BootID: "executor-boot-" + strings.Repeat("b", 32), LeaseID: "executor-lease-" + strings.Repeat("c", 32), TaskScope: executors.TaskScope{RequestID: "req-" + strings.Repeat("d", 32), AttemptID: "attempt-" + strings.Repeat("e", 32), WorkEpoch: 1, AuthoritySHA256: strings.Repeat("f", 64)}, Fence: 1}
	lease := executors.AttemptLease{Version: 1, ID: proof.LeaseID, ExecutorID: proof.ExecutorID, Generation: proof.Generation, BootID: proof.BootID, TaskScope: proof.TaskScope, Fence: proof.Fence, IssuedAtMs: 1, ExpiresAtMs: 60000}
	w := ledger.Work{ClientNonce: "owned-http-target-negative", ExecutorPolicyVersion: 1, ExecutorPin: &executors.InstallPin{Descriptor: executors.Descriptor{CoreSHA256: strings.Repeat("1", 64)}}, CurrentAttemptLease: &lease, ContextProtocolVersion: 1, Context: &ledger.ContextPin{SHA256: strings.Repeat("2", 64)}, RuntimeContextSHA256: strings.Repeat("3", 64)}
	b := ledger.CoreBinding{NativeProcessID: 1, NativeProcessGeneration: "123", SessionID: strings.Repeat("4", 32), CoreSHA256: strings.Repeat("1", 64), ExecutorProof: &proof, ContextAdmission: &ledger.ContextAdmission{Version: 1, SessionID: strings.Repeat("4", 32), ContextPinSHA256: strings.Repeat("2", 64), ManifestSHA256: strings.Repeat("3", 64)}}
	c := admissionHTTPFixtureConfig{ClientNonce: w.ClientNonce, Stage: "pre-bind"}
	if !admissionHTTPFixtureMatches(c, "pre-bind", w, b) {
		t.Fatal("matching typed fixture scope denied")
	}
	for _, kind := range []string{"nonce", "stage", "generation", "receipt", "lease"} {
		t.Run(kind, func(t *testing.T) {
			bad := b
			badW := w
			stage := "pre-bind"
			switch kind {
			case "nonce":
				badW.ClientNonce = "owned-http-other-request"
			case "stage":
				stage = "post-durable-pre-ack"
			case "generation":
				bad.NativeProcessGeneration = "0123"
			case "receipt":
				r := *bad.ContextAdmission
				r.ManifestSHA256 = strings.Repeat("0", 64)
				bad.ContextAdmission = &r
			case "lease":
				p := *bad.ExecutorProof
				p.Fence++
				bad.ExecutorProof = &p
			}
			if admissionHTTPFixtureMatches(c, stage, badW, bad) {
				t.Fatal("wrong target/receipt activated fixture")
			}
		})
	}
}
