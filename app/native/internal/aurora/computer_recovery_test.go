package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/computer"
	"nekoaurora.local/native/internal/ledger"
)

type recoverySandboxFixture struct {
	recovered, executed, destroyed int
	recoverErr, destroyErr         error
}

func (*recoverySandboxFixture) Name() string     { return "recovery-fixture" }
func (*recoverySandboxFixture) Available() error { return nil }
func (f *recoverySandboxFixture) Exec(context.Context, computer.Spec) (computer.Result, error) {
	f.executed++
	return computer.Result{}, nil
}
func (f *recoverySandboxFixture) Recover(context.Context, computer.Spec) error {
	f.recovered++
	return f.recoverErr
}
func (f *recoverySandboxFixture) Destroy(context.Context, string) error {
	f.destroyed++
	return f.destroyErr
}
func (*recoverySandboxFixture) RecoveryStatus(string) *computer.RecoveryStatus {
	return &computer.RecoveryStatus{State: "files", CanRecover: true, Message: "fixture files pending"}
}

func TestComputerRecoveryAPIScopeAndNoReplay(t *testing.T) {
	dir := t.TempDir()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
	if err != nil {
		t.Fatal(err)
	}
	f := &recoverySandboxFixture{}
	s.PC, err = computer.New(filepath.Join(dir, "computers"), f)
	if err != nil {
		t.Fatal(err)
	}
	bot := l.Snapshot().Bots[0].ID
	post := func(action string, body map[string]any) int {
		t.Helper()
		raw, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest("POST", "/app/api/agents/"+bot+"/computer/"+action, strings.NewReader(string(raw))))
		return rec.Code
	}
	for _, action := range []string{"recover", "reset"} {
		for _, scope := range []string{"", "different"} {
			if code := post(action, map[string]any{"workspaceId": scope}); code != 409 {
				t.Fatal("scope not rejected", action, code)
			}
		}
		if code := post(action, map[string]any{"workspaceId": s.workspaceID, "command": "do not run"}); code != 400 {
			t.Fatal("unknown field accepted", action, code)
		}
	}
	if f.recovered != 0 || f.destroyed != 0 || f.executed != 0 {
		t.Fatal("invalid request had side effects", f)
	}
	if code := post("recover", map[string]any{"workspaceId": s.workspaceID}); code != 200 || f.recovered != 1 || f.executed != 0 {
		t.Fatal("recovery did not use file-only path", code, f)
	}
	f.recoverErr = computer.ErrRecoveryConflict
	if code := post("recover", map[string]any{"workspaceId": s.workspaceID}); code != 409 {
		t.Fatal("local edit conflict", code)
	}
	f.recoverErr = computer.ErrRecoveryPending
	if code := post("recover", map[string]any{"workspaceId": s.workspaceID}); code != 409 {
		t.Fatal("unknown command outcome", code)
	}
	f.destroyErr = errors.New("fixture destruction unconfirmed")
	if err = s.PC.WriteFile(bot, "keep.txt", "owner copy"); err != nil {
		t.Fatal(err)
	}
	if code := post("reset", map[string]any{"workspaceId": s.workspaceID}); code != 500 {
		t.Fatal("failed reset acknowledged", code)
	}
	if txt, _, err := s.PC.ReadFile(bot, "keep.txt"); err != nil || txt != "owner copy" {
		t.Fatal("failed reset discarded local data", err)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/app/api/agents/"+bot+"/computer", nil))
	var state computer.Status
	if err = json.Unmarshal(rec.Body.Bytes(), &state); err != nil || state.Available || state.Recovery == nil || !state.Recovery.CanRecover {
		t.Fatal("pending status unavailable", err, state)
	}
}
