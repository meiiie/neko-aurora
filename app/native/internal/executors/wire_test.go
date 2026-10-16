package executors

import (
	"encoding/json"
	"strings"
	"testing"
)

func descriptor() Descriptor {
	return Descriptor{strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), strings.Repeat("d", 64)}
}
func fixturePin() InstallPin {
	return InstallPin{Version: 1, ExecutorID: "executor-" + strings.Repeat("a", 32), Generation: 1, Descriptor: descriptor()}
}
func fixtureScope() TaskScope {
	return TaskScope{RequestID: "req-" + strings.Repeat("a", 32), AttemptID: "attempt-" + strings.Repeat("b", 32), WorkEpoch: 1, AuthoritySHA256: strings.Repeat("e", 64)}
}
func fixtureLease() AttemptLease {
	return AttemptLease{Version: 1, ID: "executor-lease-" + strings.Repeat("c", 32), ExecutorID: fixturePin().ExecutorID, Generation: 1, BootID: "executor-boot-" + strings.Repeat("d", 32), TaskScope: fixtureScope(), Fence: 1, IssuedAtMs: 1791734400000, ExpiresAtMs: 1791735000000}
}

func TestStrictExecutorWireAndProofRoundtrip(t *testing.T) {
	pin := fixturePin()
	lease := fixtureLease()
	if !ValidInstallPin(pin) || !ValidLease(lease) {
		t.Fatal("valid exact public descriptors rejected")
	}
	p, _ := json.Marshal(pin)
	if got, err := ParseInstallPin(p); err != nil || got != pin {
		t.Fatal("install pin roundtrip", err)
	}
	b, _ := json.Marshal(lease)
	if len(b) > 1024 {
		t.Fatal("launch metadata budget exceeded")
	}
	if got, err := ParseLease(b); err != nil || got != lease {
		t.Fatal("lease roundtrip", err)
	}
	proof, err := ProofFor(lease)
	if err != nil || !MatchRuntimeProof(lease, proof) {
		t.Fatal("exact proof not bound", err)
	}
	b, _ = json.Marshal(proof)
	if got, err := ParseRuntimeProof(b); err != nil || got != proof {
		t.Fatal("proof roundtrip", err)
	}
}
func TestStrictExecutorWireRejectsUnknownDuplicateAndNumericAmbiguity(t *testing.T) {
	raw, _ := json.Marshal(fixtureLease())
	base := string(raw)
	for name, input := range map[string]string{"unknown": strings.TrimSuffix(base, "}") + `,"secret":"fixture-canary"}`, "duplicate": strings.TrimSuffix(base, "}") + `,"generation":1}`, "trailing": base + `{}`, "unsafe": strings.Replace(base, `"workEpoch":1`, `"workEpoch":9007199254740992`, 1), "float": strings.Replace(base, `"workEpoch":1`, `"workEpoch":1.5`, 1), "wrong-version": strings.Replace(base, `"version":1`, `"version":2`, 1), "wrong-executor": strings.Replace(base, `executor-aaaaaaaa`, `device-aaaaaaaa`, 1), "long-ttl": strings.Replace(base, `1791735000000`, `1791735000001`, 1)} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseLease([]byte(input)); err == nil {
				t.Fatal("ambiguous or unbounded lease accepted")
			}
		})
	}
	for _, parse := range []func([]byte) error{func(b []byte) error { _, e := ParseInstallPin(b); return e }, func(b []byte) error { _, e := ParseRuntimeProof(b); return e }} {
		if parse([]byte(`{"version":1,"version":1}`)) == nil {
			t.Fatal("duplicate identity accepted")
		}
	}
}
func TestRuntimeProofRequiresEveryExactAttemptField(t *testing.T) {
	lease := fixtureLease()
	proof, err := ProofFor(lease)
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*RuntimeProof){func(p *RuntimeProof) { p.ExecutorID = "executor-" + strings.Repeat("f", 32) }, func(p *RuntimeProof) { p.Generation++ }, func(p *RuntimeProof) { p.BootID = "executor-boot-" + strings.Repeat("f", 32) }, func(p *RuntimeProof) { p.LeaseID = "executor-lease-" + strings.Repeat("f", 32) }, func(p *RuntimeProof) { p.RequestID = "req-" + strings.Repeat("f", 32) }, func(p *RuntimeProof) { p.AttemptID = "attempt-" + strings.Repeat("f", 32) }, func(p *RuntimeProof) { p.WorkEpoch++ }, func(p *RuntimeProof) { p.Fence++ }, func(p *RuntimeProof) { p.AuthoritySHA256 = strings.Repeat("f", 64) }} {
		changed := proof
		edit(&changed)
		if MatchRuntimeProof(lease, changed) {
			t.Fatal("cross-attempt proof accepted")
		}
	}
}
