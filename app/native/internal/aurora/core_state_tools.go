package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/tools"
)

type coreOperationKey struct{}

const coreStateToolsPrompt = `
For this Core-hosted turn, use explicit Aurora tools for persistent changes; MEMORY, SCHEDULE and HANDOFF text directives do not execute actions.
Call mcp__aurora__execute with:
- {"name":"memory_remember","arguments":{"args":{"text":"<one lasting fact the owner explicitly stated or asked to remember>","scope":"bot|group|team"}}}. Default bot; group only in the current group, team only for an owner preference applying to every bot. Never store credentials or inferred facts.
- {"name":"schedule_create","arguments":{"args":{"text":"<the requested future task>","at":"HH:MM","repeat":"once|daily|weekdays","date":"YYYY-MM-DD","timezone":"Asia/Ho_Chi_Minh"}}}. Only when the owner explicitly requests a scheduled action. Use their requested timezone, otherwise Asia/Ho_Chi_Minh. For interval requests use repeat="interval" and intervalMinutes=15..10080, without at/date. The returned record is the actual saved schedule; report its time and timezone accurately.
For permitted group handoffs, use network_list_sessions and network_send_message with exact returned session IDs. Delivery is not proof the other bot finished.
Do not claim a persistent change until its tool succeeds. CHOICES, QUESTIONS, APPROVAL, SECRET and FORM remain presentation directives for owner input. They do not grant execution permission. Do not run a separate model review after completion.`

func isCoreStateTool(name string) bool {
	return name == "memory_remember" || name == "schedule_create"
}

func init() {
	toolLabel["memory_remember"] = "Bộ nhớ"
	toolLabel["schedule_create"] = "Lịch chạy"
}

func (s *Service) runCoreStateTool(ctx context.Context, w ledger.Work, call toolCall) (tools.Result, error) {
	operation, _ := ctx.Value(coreOperationKey{}).(string)
	if !coreOperationID.MatchString(operation) || ctx.Value(coreTurnKey{}) != true || s.networkPeerWork(w) {
		return tools.Result{}, ledger.ErrForbidden
	}
	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}
	var request toolReq
	if strictCoreJSON([]byte(call.arg), &request) != nil {
		return tools.Result{}, ErrInvalid
	}
	var value any
	switch call.name {
	case "memory_remember":
		var in struct {
			Text  string `json:"text"`
			Scope string `json:"scope"`
		}
		if strictCoreJSON(request.Args, &in) != nil || strings.TrimSpace(in.Text) == "" || utf8.RuneCountInString(in.Text) > 300 {
			return tools.Result{}, ErrInvalid
		}
		scope := w.BotID
		switch in.Scope {
		case "", "bot":
		case "team":
			scope = teamKey
		case "group":
			if !isGroupID(w.ConversationID) {
				return tools.Result{}, ErrInvalid
			}
			scope = w.ConversationID
		default:
			return tools.Result{}, ErrInvalid
		}
		added, err := s.Mem.rememberBy(scope, in.Text, "bot")
		if err != nil {
			return tools.Result{}, err
		}
		value = map[string]any{"saved": true, "alreadyPresent": !added, "scope": scope, "text": clean(in.Text, 300)}
	case "schedule_create":
		var in struct {
			Text            string `json:"text"`
			At              string `json:"at"`
			Repeat          string `json:"repeat"`
			Date            string `json:"date"`
			Timezone        string `json:"timezone"`
			IntervalMinutes int    `json:"intervalMinutes"`
		}
		if strictCoreJSON(request.Args, &in) != nil || utf8.RuneCountInString(in.Text) > 1000 {
			return tools.Result{}, ErrInvalid
		}
		if in.Repeat != "once" && in.Repeat != "daily" && in.Repeat != "weekdays" && in.Repeat != "interval" {
			return tools.Result{}, ErrInvalid
		}
		x, ok := (schedule{ID: "s-core-" + operation, Bot: w.BotID, Text: in.Text, At: in.At, Repeat: in.Repeat, Date: in.Date, Timezone: in.Timezone, IntervalMinutes: in.IntervalMinutes}).normalize(s.Now())
		if !ok {
			return tools.Result{}, ErrInvalid
		}
		if err := s.sched.storageError(); err != nil {
			return tools.Result{}, err
		}
		if !s.sched.add(x) {
			// An identical prior schedule is usable evidence, not a second schedule.
			found := false
			for _, existing := range s.sched.list(w.BotID) {
				if existing.ID == x.ID || existing.At == x.At && existing.Date == x.Date && existing.Repeat == x.Repeat && existing.zoneName() == x.zoneName() && existing.IntervalMinutes == x.IntervalMinutes && strings.EqualFold(existing.Text, x.Text) && !existing.Paused {
					x, found = existing, true
					break
				}
			}
			if !found {
				return tools.Result{}, errors.New("schedule_not_saved")
			}
		}
		x.Revision = max(1, x.Revision)
		value = map[string]any{"saved": true, "schedule": x}
	default:
		return tools.Result{}, ErrInvalid
	}
	body, err := json.Marshal(value)
	return tools.Result{Summary: toolLabel[call.name], Text: string(body)}, err
}

func coreRuntimeNote(name string, core *Core) string {
	body, _ := json.Marshal(map[string]string{"botName": name, "modelId": core.Model, "providerProfile": core.Profile, "agentLoop": "Neko Core"})
	return "\nCURRENT AURORA RUNTIME (host-owned facts, pinned for this turn): " + string(body) + "\nYour user-facing identity is the named Aurora bot. If asked which model you use, quote modelId exactly; a transport label such as Codex is not the bot name or a more precise model ID. Do not infer a different model from old messages.\n"
}
