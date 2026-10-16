package ledger

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func historicalUnstagedAttempt(t *testing.T, dir, digest string) (*Store, Work, CoreBinding) {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	w, _, err := s.Accept(input("historical-runtime-provenance", "background"))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{NativeProcessID: 401, SessionID: strings.Repeat("a", 32), CoreSHA256: digest, Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"authorityId":"fixture-authority"}`)}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
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
	b.NativeProcessID = 402
	b.Task = json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":2,"activationId":"dddddddddddddddddddddddddddddddd"}`)
	if w.RuntimeAttemptBinding != nil || !validAttemptBinding(w, b) {
		t.Fatal("same-runtime historical continuation fixture invalid")
	}
	return s, w, b
}

func TestUnstagedAdmissionPreservesHistoricalRuntimeProvenance(t *testing.T) {
	for name, digest := range map[string]string{
		"v1": "4b3fb6202551912f86b5dd5faffccf75d854174c2ddd6b9fb331470a2b2e0b36",
		"v2": "2a618df35470fb54d6058c44275999f00a95f55c2435f3aadafe19cc6b791904",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s, w, b := historicalUnstagedAttempt(t, dir, digest)
			sequence := s.Snapshot().Sequence
			changed := b
			changed.CoreSHA256 = "fa48d43f59257dcd171441f09a8ac53d3702485ddf7f80ac13a0796d53ac6310"
			if err := s.Admit(w.RequestID, w.Epoch, changed); !errors.Is(err, ErrConflict) {
				t.Fatalf("unstaged runtime rewrite accepted: %v", err)
			}
			got, _ := s.Get(w.RequestID)
			if got.Core.CoreSHA256 != digest || got.Status != "starting" || s.Snapshot().Sequence != sequence {
				t.Fatal("rejected runtime rewrite mutated durable state")
			}
			if err := s.Admit(w.RequestID, w.Epoch, b); err != nil {
				t.Fatal("legitimate proof-free historical continuation rejected", err)
			}
			s.Close()
			for i := 0; i < 3; i++ {
				var err error
				s, err = Open(dir)
				if err != nil {
					t.Fatal(err)
				}
				got, _ = s.Get(w.RequestID)
				if got.Core.CoreSHA256 != digest || got.Core.TaskRecovery != nil || got.Status != "running" {
					t.Fatal("historical proof-free receipt changed on replay")
				}
				s.Close()
			}
		})
	}
}

func TestHashValidUnstagedRuntimeRewriteCannotReplay(t *testing.T) {
	dir := t.TempDir()
	s, w, b := historicalUnstagedAttempt(t, dir, "2a618df35470fb54d6058c44275999f00a95f55c2435f3aadafe19cc6b791904")
	b.CoreSHA256 = "fa48d43f59257dcd171441f09a8ac53d3702485ddf7f80ac13a0796d53ac6310"
	w.Status = "running"
	w.Core = &b
	if err := s.commit(Event{Type: "work.admitted", Work: w}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if opened, err := Open(dir); err == nil {
		opened.Close()
		t.Fatal("hash-valid runtime provenance transplant replayed")
	} else if !errors.Is(err, ErrCorrupt) {
		t.Fatal("replay failed for an unrelated reason", err)
	}
}
