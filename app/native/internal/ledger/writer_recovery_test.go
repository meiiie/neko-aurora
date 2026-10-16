package ledger

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestWriterPriorReceiptRecoveryIsBoundedAndReplays(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.Accept(input("writer-recovery", "background"))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	binding := CoreBinding{NativeProcessID: 401, SessionID: strings.Repeat("a", 32), CoreSHA256: "2a618df35470fb54d6058c44275999f00a95f55c2435f3aadafe19cc6b791904", Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1,"authorityId":"fixture-authority"}`)}
	if err = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, binding); err != nil {
		t.Fatal(err)
	}
	if err = s.Admit(w.RequestID, w.Epoch, binding); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "recovery_required", "transport_interrupted"); err != nil {
		t.Fatal(err)
	}
	w, err = s.Requeue(w.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	fresh := binding
	fresh.NativeProcessID = 402
	fresh.Task = json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":3,"activationId":"dddddddddddddddddddddddddddddddd"}`)
	fresh.TaskRecovery = &TaskRecovery{1, "prior_receipt_retry"}
	cases := map[string]func(*CoreBinding){
		"missing-proof": func(b *CoreBinding) { b.TaskRecovery = nil },
		"unversioned":   func(b *CoreBinding) { b.TaskRecovery = &TaskRecovery{0, "prior_receipt_retry"} },
		"unknown-proof": func(b *CoreBinding) { b.TaskRecovery = &TaskRecovery{1, "unknown"} },
		"other-runtime": func(b *CoreBinding) {
			b.CoreSHA256 = "4b3fb6202551912f86b5dd5faffccf75d854174c2ddd6b9fb331470a2b2e0b36"
		},
		"other-session":   func(b *CoreBinding) { b.SessionID = strings.Repeat("e", 32) },
		"other-authority": func(b *CoreBinding) { b.Execution = json.RawMessage(`{"authorityId":"other"}`) },
		"epoch-one-or-three-step": func(b *CoreBinding) {
			b.Task = json.RawMessage(fmt.Sprintf(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":%d,"activationId":"dddddddddddddddddddddddddddddddd"}`, 4))
		},
		"same-activation": func(b *CoreBinding) {
			b.Task = json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":3,"activationId":"cccccccccccccccccccccccccccccccc"}`)
		},
		"other-task": func(b *CoreBinding) {
			b.Task = json.RawMessage(`{"id":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","root":"/fixture","activationEpoch":3,"activationId":"dddddddddddddddddddddddddddddddd"}`)
		},
		"other-root": func(b *CoreBinding) {
			b.Task = json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/other","activationEpoch":3,"activationId":"dddddddddddddddddddddddddddddddd"}`)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			b := fresh
			change(&b)
			if validAttemptBinding(w, b) {
				t.Fatal("accepted unrelated or unproven writer recovery")
			}
		})
	}
	if err = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, fresh); err != nil {
		t.Fatal(err)
	}
	if err = s.Admit(w.RequestID, w.Epoch, fresh); err != nil {
		t.Fatal(err)
	}
	s.Close()
	for i := 0; i < 3; i++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.RecoverInterrupted(); err != nil {
			t.Fatal(err)
		}
		got, _ := s.Get(w.RequestID)
		if got.Core.TaskRecovery == nil || got.Core.TaskRecovery.Kind != "prior_receipt_retry" {
			t.Fatal("lost durable recovery receipt")
		}
		got.Core.TaskRecovery.Kind = "changed"
		again, _ := s.Get(w.RequestID)
		if again.Core.TaskRecovery.Kind != "prior_receipt_retry" {
			t.Fatal("receipt alias escaped")
		}
		s.Close()
	}
}
func TestWriterRecoveryProofCannotBypassStagedAdmissionOrReplay(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, _, err := s.Accept(input("unbound-writer-recovery", "background"))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{NativeProcessID: 401, SessionID: strings.Repeat("a", 32), CoreSHA256: "2a618df35470fb54d6058c44275999f00a95f55c2435f3aadafe19cc6b791904", Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"authorityId":"fixture-authority"}`), TaskRecovery: &TaskRecovery{1, "prior_receipt_retry"}}
	if s.Admit(w.RequestID, w.Epoch, b) == nil {
		t.Fatal("unbound proof bypassed admission")
	}
	forged := copyWork(w)
	forged.Status = "running"
	forged.Core = &b
	if validRuntimeReplay(s, Event{Type: "work.admitted", Work: forged}) {
		t.Fatal("unbound proof bypassed replay")
	}
}
