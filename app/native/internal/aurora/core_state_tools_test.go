package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func stateToolCall(t *testing.T, h *auroraTurnHost, op, name string, args any) json.RawMessage {
	t.Helper()
	input := map[string]any{"args": args}
	call := map[string]any{"name": name, "arguments": input}
	message := map[string]any{"role": "assistant", "content": "", "tool_calls": []any{
		map[string]any{"id": "state-call", "type": "function", "function": map[string]any{"name": "mcp__aurora__execute", "arguments": string(rawCore(t, call))}},
	}, "_neko_tool_operations": []any{map[string]any{"version": 1, "operationId": op, "toolCallId": "state-call"}}}
	if err := h.checkpoint(rawCore(t, map[string]any{"version": 1, "messages": []any{message}, "acceptedInputs": []string{}})); err != nil {
		t.Fatal(err)
	}
	return rawCore(t, map[string]any{"name": name, "arguments": input, "operationId": op})
}

func TestCoreHostStateToolsPersistAndReplayWithoutDuplicateEffects(t *testing.T) {
	for _, name := range []string{"memory_remember", "schedule_create"} {
		t.Run(name, func(t *testing.T) {
			h, c := coreHostFixture(t)
			var args any = map[string]any{"text": "Owner prefers short answers.", "scope": "team"}
			if name == "schedule_create" {
				args = map[string]any{"text": "Fixture task", "repeat": "interval", "intervalMinutes": 60}
			}
			req := stateToolCall(t, h, strings.Repeat("d", 32), name, args)
			a, err := h.execute(context.Background(), req)
			if err != nil || !strings.Contains(a.Text, `"saved":true`) {
				t.Fatal("missing persisted receipt", err, a.Text)
			}
			h.state, err = openCoreAgentState(h.s.coreStateDir, h.w, c)
			if err != nil {
				t.Fatal(err)
			}
			b, err := h.execute(context.Background(), req)
			if err != nil || a.Text != b.Text {
				t.Fatal("replay lost receipt", err)
			}
			if name == "memory_remember" {
				if len(h.s.Mem.List(teamKey)) != 1 || len(h.s.Mem.List(h.w.BotID)) != 0 {
					t.Fatal("wrong memory scope or duplicate")
				}
				if len(openMemory(filepath.Dir(h.s.Mem.path)).List(teamKey)) != 1 {
					t.Fatal("memory did not survive reopen")
				}
			} else {
				items := h.s.sched.list(h.w.BotID)
				if len(items) != 1 || items[0].ID != "s-core-"+strings.Repeat("d", 32) || items[0].IntervalAnchor == "" {
					t.Fatal("wrong schedule or duplicate")
				}
				stored, err := readSchedules(h.s.sched.path)
				if err != nil || len(stored) != 1 {
					t.Fatal("schedule not durable", err)
				}
			}
		})
	}
}

func TestCoreHostStateToolsRefusePeerAndUnboundCalls(t *testing.T) {
	h, _ := coreHostFixture(t)
	call := callsOf([]toolReq{{Name: "memory_remember", Args: rawCore(t, map[string]string{"text": "Do not save this"})}})[0]
	if _, err := h.s.runCoreStateTool(context.Background(), h.w, call); !errors.Is(err, ledger.ErrForbidden) {
		t.Fatal("unbound mutation admitted", err)
	}
	ctx := context.WithValue(context.WithValue(context.Background(), coreTurnKey{}, true), coreOperationKey{}, strings.Repeat("e", 32))
	peer := h.w
	peer.PeerMessageID = "untrusted-peer"
	// Unknown peer provenance is fail-closed, just as it is for network tools.
	if _, err := h.s.runCoreStateTool(ctx, peer, call); !errors.Is(err, ledger.ErrForbidden) {
		t.Fatal("peer changed owner memory", err)
	}
	if len(h.s.Mem.List(h.w.BotID)) != 0 {
		t.Fatal("forbidden call mutated memory")
	}
}

func TestCoreHostStateToolStorageFailureIsNotSuccess(t *testing.T) {
	for _, name := range []string{"memory_remember", "schedule_create"} {
		t.Run(name, func(t *testing.T) {
			h, _ := coreHostFixture(t)
			var args any = map[string]any{"text": "A fact"}
			if name == "memory_remember" {
				if err := os.Mkdir(h.s.Mem.path+".tmp", 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				h.s.sched.loadErr = errScheduleStorage
				args = map[string]any{"text": "A task", "repeat": "daily", "at": "08:00"}
			}
			req := stateToolCall(t, h, strings.Repeat("f", 32), name, args)
			result, err := h.execute(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(result.Text, `"saved":true`) {
				t.Fatal("storage failure claimed success")
			}
			w, _ := h.current()
			if len(w.Direct.Tools) != 1 || w.Direct.Tools[0].Status != "failed" {
				t.Fatal("missing failed tool receipt")
			}
			if verdict := h.verify(context.Background(), "Saved."); verdict.Status != "incomplete" || !strings.Contains(verdict.Summary, "state_change_not_saved") {
				t.Fatal("failed state write became ready", verdict)
			}
			if len(h.s.Mem.List(h.w.BotID)) != 0 || len(h.s.sched.list(h.w.BotID)) != 0 {
				t.Fatal("failed write changed state")
			}
		})
	}
}

func TestCoreHostStateToolsValidateFieldsAndScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		args any
	}{
		{"memory_remember", map[string]any{"text": "A fact", "scope": "another-bot"}},
		{"memory_remember", map[string]any{"text": "A fact", "scope": "group"}},
		{"memory_remember", map[string]any{"text": "A fact", "ownerOverride": true}},
		{"schedule_create", map[string]any{"text": "A task", "repeat": "random", "at": "08:00"}},
		{"schedule_create", map[string]any{"text": "A task", "repeat": "interval", "intervalMinutes": 1}},
		{"schedule_create", map[string]any{"text": "A task", "repeat": "daily", "at": "08:00", "bot": "another-bot"}},
	} {
		h, _ := coreHostFixture(t)
		req := stateToolCall(t, h, strings.Repeat("c", 32), tc.name, tc.args)
		if result, err := h.execute(context.Background(), req); err != nil || strings.Contains(result.Text, `"saved":true`) {
			t.Fatal("invalid state mutation admitted", tc.name, err)
		}
		if len(h.s.Mem.List(h.w.BotID)) != 0 || len(h.s.sched.list(h.w.BotID)) != 0 {
			t.Fatal("invalid write changed state")
		}
	}
}

func TestCoreHostTextDirectivesCannotPretendToExecute(t *testing.T) {
	h, _ := coreHostFixture(t)
	for _, text := range []string{`MEMORY: {"text":"A fact"}`, `SCHEDULE: {"text":"A task","at":"08:00","repeat":"daily"}`, `HANDOFF: {"to":"Davin","text":"A task"}`} {
		if got := h.verify(context.Background(), "Done.\n"+text); got.Status != "incomplete" || got.Revision != "unexecuted-directive" {
			t.Fatal("unexecuted directive became success", got)
		}
	}
	if len(h.s.Mem.List(h.w.BotID)) != 0 || len(h.s.sched.list(h.w.BotID)) != 0 {
		t.Fatal("verification executed an effect")
	}
}

func TestCoreHostCachedSecretCardIsDeliveredAsWaiting(t *testing.T) {
	h, c := coreHostFixture(t)
	text := "Please enter the required credential.\nSECRET: {\"name\":\"SERVICE_TOKEN\",\"label\":\"Service token\",\"desc\":\"Needed for this task\"}"
	v := h.verify(context.Background(), text)
	if v.Status != "waiting" {
		t.Fatal("credential request marked complete", v)
	}
	h.state.Result = &coreAgentResult{Text: text, Completion: v}
	if err := h.state.save(); err != nil {
		t.Fatal(err)
	}
	ap, qs, err := h.s.runCoreTurn(context.Background(), h.w, c)
	if err != nil {
		t.Fatal(err)
	}
	h.s.finishWork(h.w, ap, qs)
	w, _ := h.s.L.Get(h.w.RequestID)
	if w.Direct.Secret == nil || w.Direct.Secret.Name != "SERVICE_TOKEN" || w.Direct.Completion.Status != "waiting" {
		t.Fatal("secret card was dropped")
	}
}

func TestCoreHostRuntimeIdentityUsesPinnedModel(t *testing.T) {
	note := coreRuntimeNote("Meiiie", &Core{Model: "gpt-6.1-sol", Profile: "chatgpt"})
	for _, want := range []string{`"botName":"Meiiie"`, `"modelId":"gpt-6.1-sol"`, `"providerProfile":"chatgpt"`} {
		if !strings.Contains(note, want) {
			t.Fatal("missing runtime identity", want)
		}
	}
}
