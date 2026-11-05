package aurora

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/tools"
)

// This host supplies facts and effects. Only Core decides the next model/tool step.
type auroraTurnHost struct {
	s         *Service
	w         ledger.Work
	tc        *toolCtx
	state     *coreAgentState
	offered   map[string]ledger.Work
	draft     strings.Builder
	pending   string
	lastDelta time.Time
	newStep   bool
	usageOpen bool
	usageIn   int
	usageOut  int
}

type coreSnapshot struct {
	Version  int           `json:"version"`
	Messages []coreMessage `json:"messages"`
	Accepted []string      `json:"acceptedInputs"`
}

type coreTurnKey struct{}

func (s *Service) updateWork(w ledger.Work, change func(*ledger.DirectRun)) error {
	if w.Direct == nil || w.Direct.Engine != "core" {
		return s.update(w.RequestID, change)
	}
	if _, err := s.L.DirectUpdateAttempt(w.RequestID, w.Epoch, w.AttemptID, change); err != nil {
		return err
	}
	s.sync()
	return nil
}

type coreMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	History    bool            `json:"_neko_host_history"`
	Inflight   bool            `json:"_neko_inflight"`
	Segment    string          `json:"_neko_host_segment"`
	At         string          `json:"_neko_host_segment_at"`
	Input      string          `json:"_neko_host_input"`
	AfterInput *string         `json:"_neko_host_after_input"`
	Calls      []struct {
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
	Operations []struct {
		ID      string `json:"operationId"`
		Call    string `json:"toolCallId"`
		Version int    `json:"version"`
	} `json:"_neko_tool_operations"`
}

func decodeCoreSnapshot(raw json.RawMessage) (coreSnapshot, error) {
	var cp coreSnapshot
	if len(raw) > coreFrameLimit || json.Unmarshal(raw, &cp) != nil || cp.Version != 1 || len(cp.Messages) == 0 || len(cp.Messages) > 4096 || len(cp.Accepted) > ledger.MaxSteered {
		return cp, errors.New("core_checkpoint_invalid")
	}
	seen := map[string]bool{}
	for _, id := range cp.Accepted {
		if !coreStateID.MatchString(id) || seen[id] {
			return cp, errors.New("core_checkpoint_invalid")
		}
		seen[id] = true
	}
	for _, m := range cp.Messages {
		if m.Role != "system" && m.Role != "user" && m.Role != "assistant" && m.Role != "tool" {
			return cp, errors.New("core_checkpoint_invalid")
		}
	}
	return cp, nil
}

func strictCoreJSON(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errors.New("core_protocol_invalid")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("core_protocol_invalid")
	}
	return nil
}

func (h *auroraTurnHost) current() (ledger.Work, error) {
	w, err := h.s.L.Get(h.w.RequestID)
	if err != nil || w.Status != "running" || w.DesiredState != "running" || w.Epoch != h.w.Epoch || w.AttemptID != h.w.AttemptID || w.Direct == nil || w.Direct.Engine != "core" {
		return w, errors.New("core_attempt_expired")
	}
	if h.s.ext != nil && h.s.ext.bot(h.w.BotID).Folder != h.state.Folder {
		return w, errors.New("core_workspace_changed")
	}
	return w, nil
}

func (h *auroraTurnHost) Request(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := h.current(); err != nil {
		return nil, err
	}
	switch method {
	case "checkpoint":
		return nil, h.checkpoint(raw)
	case "inputs":
		if string(raw) != "{}" {
			return nil, errors.New("core_protocol_invalid")
		}
		return h.inputs(), nil
	case "tool":
		return h.execute(ctx, raw)
	case "completion":
		var in struct {
			Answer string `json:"answer"`
		}
		if err := strictCoreJSON(raw, &in); err != nil {
			return nil, err
		}
		return h.verify(ctx, in.Answer), nil
	default:
		return nil, errors.New("core_method_unavailable")
	}
}

func (h *auroraTurnHost) inputs() []map[string]string {
	out := []map[string]string{}
	if h.w.PeerMessageID != "" {
		return out
	}
	cur, _ := h.current()
	for _, q := range h.s.inbox(h.w) {
		if len(cur.Direct.Steered)+len(out) >= ledger.MaxSteered {
			break
		}
		h.offered[q.RequestID] = q
		out = append(out, map[string]string{"id": q.RequestID, "text": h.redact(q.Text)})
	}
	return out
}

func (h *auroraTurnHost) redact(text string) string {
	if h.s.V != nil {
		return h.s.V.Redact(text)
	}
	return text
}

func (h *auroraTurnHost) checkpoint(raw json.RawMessage) error {
	cp, err := decodeCoreSnapshot(raw)
	if err != nil {
		return err
	}
	cur, err := h.current()
	if err != nil {
		return err
	}
	known := map[string]bool{}
	changed := false
	for _, id := range cur.Direct.Steered {
		known[id] = true
	}
	for _, id := range cp.Accepted {
		if !known[id] {
			q, ok := h.offered[id]
			found := false
			for _, m := range cp.Messages {
				var text string
				if m.Role == "user" && m.Input == id && json.Unmarshal(m.Content, &text) == nil && text == h.redact(q.Text) {
					found = true
				}
			}
			if !ok || !found {
				return errors.New("core_input_receipt_invalid")
			}
			cur.Direct.Steered = append(cur.Direct.Steered, id)
			changed = true
			known[id] = true
		}
	}
	segments := map[string]bool{}
	segmentAdded := false
	afterInput := ""
	for _, s := range cur.Direct.Segments {
		segments[s.ID] = true
	}
	for i, m := range cp.Messages {
		if m.Role == "user" && m.Input != "" {
			if !known[m.Input] {
				return errors.New("core_input_receipt_invalid")
			}
			afterInput = m.Input
		}
		if m.AfterInput != nil && *m.AfterInput != "" && !known[*m.AfterInput] {
			return errors.New("core_input_receipt_invalid")
		}
		// The latest answer remains the live/final bubble. Earlier completed
		// prose becomes a stable message, including pre-tool progress.
		if m.Role != "assistant" || m.History || m.Inflight || m.Segment == "" || segments[m.Segment] || (i == len(cp.Messages)-1 && len(m.Calls) == 0) {
			continue
		}
		var text string
		if json.Unmarshal(m.Content, &text) != nil {
			continue
		}
		text = h.redact(visibleText(text))
		if strings.TrimSpace(text) == "" {
			continue
		}
		if !coreStateID.MatchString(m.Segment) || len(text) > 60000 {
			return errors.New("core_segment_invalid")
		}
		if _, err := time.Parse(time.RFC3339Nano, m.At); err != nil {
			return errors.New("core_segment_invalid")
		}
		anchor := afterInput
		if m.AfterInput != nil {
			anchor = *m.AfterInput
		}
		cur.Direct.Segments = append(cur.Direct.Segments, ledger.DirectSegment{ID: m.Segment, Text: text, At: m.At, AfterInputID: anchor})
		changed = true
		segmentAdded = true
		segments[m.Segment] = true
	}
	// Persist canonical input acceptance BEFORE acknowledging it in the queue.
	h.state.Snapshot = append(json.RawMessage(nil), raw...)
	h.state.Presentation = cur.Direct
	if err := h.state.save(); err != nil {
		return err
	}
	if changed {
		if err := h.s.updateWork(h.w, func(d *ledger.DirectRun) {
			d.Steered, d.Segments = cur.Direct.Steered, cur.Direct.Segments
		}); err != nil {
			return err
		}
	}
	if segmentAdded && !h.newStep {
		h.flush()
		h.s.endDraft(h.w)
		h.draft.Reset()
	}
	if h.newStep {
		// Move prose to the journal before retiring its live draft. A new
		// step event alone is not evidence that the previous prose was saved.
		h.s.endDraft(h.w)
		h.draft.Reset()
		h.newStep = false
		h.usageOpen = true
		h.usageIn = 0
		for _, m := range cp.Messages {
			var text string
			if json.Unmarshal(m.Content, &text) == nil {
				h.usageIn += len([]rune(text))
			}
		}
	}
	for _, id := range cp.Accepted {
		if q, ok := h.offered[id]; ok {
			h.tc.ownerURLs = append(h.tc.ownerURLs, tools.RequestedURLs(stripRef(q.Text))...)
			delete(h.offered, id)
		}
	}
	return nil
}

type coreToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Operation string          `json:"operationId"`
}

func (h *auroraTurnHost) execute(ctx context.Context, raw json.RawMessage) (coreToolReply, error) {
	var in coreToolCall
	if err := strictCoreJSON(raw, &in); err != nil {
		return coreToolReply{}, err
	}
	if !coreOperationID.MatchString(in.Operation) {
		return coreToolReply{}, errors.New("core_operation_invalid")
	}
	var req toolReq
	if err := strictCoreJSON(in.Arguments, &req); err != nil {
		return coreToolReply{Text: "Invalid tool arguments; use the advertised fields."}, nil
	}
	if req.Name != "" {
		return coreToolReply{}, errors.New("core_operation_invalid")
	}
	req.Name = in.Name
	if !validTool(req) {
		return coreToolReply{Text: "Tool unavailable or required arguments missing."}, nil
	}
	canonical, _ := json.Marshal(req)
	digest := coreDigest(canonical)
	if prior, ok := h.state.Effects[in.Operation]; ok {
		if prior.Digest != digest {
			return coreToolReply{}, errors.New("core_operation_conflict")
		}
		if prior.Status != "completed" {
			return coreToolReply{}, errors.New("core_effect_unknown")
		}
		return prior.Reply, nil
	}
	// Bind the runtime ID to the exact tool call in the durable Core trajectory.
	cp, err := decodeCoreSnapshot(h.state.Snapshot)
	if err != nil {
		return coreToolReply{}, err
	}
	bound := false
	for _, m := range cp.Messages {
		for _, op := range m.Operations {
			if op.ID != in.Operation || op.Version != 1 {
				continue
			}
			for _, call := range m.Calls {
				if call.ID != op.Call || call.Function.Name != "mcp__aurora__execute" {
					continue
				}
				var recorded struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}
				var args toolReq
				if strictCoreJSON([]byte(call.Function.Arguments), &recorded) != nil || strictCoreJSON(recorded.Arguments, &args) != nil || args.Name != "" {
					continue
				}
				args.Name = recorded.Name
				data, _ := json.Marshal(args)
				bound = coreDigest(data) == digest
			}
		}
	}
	if !bound || len(h.state.Effects) >= 128 {
		return coreToolReply{}, errors.New("core_operation_not_checkpointed")
	}
	if h.state.Pending != "" {
		return coreToolReply{}, errors.New("core_waiting_for_owner")
	}
	h.state.Effects[in.Operation] = coreEffect{Digest: digest, Status: "started"}
	if err := h.state.save(); err != nil {
		return coreToolReply{}, err
	}
	before := len(h.tc.blocks)
	beforeRuns := h.s.pcSnap(h.w.BotID).runs
	execution := context.WithValue(context.WithValue(ctx, coreTurnKey{}, true), coreOperationKey{}, in.Operation)
	h.s.runTools(execution, h.w, h.tc, callsOf([]toolReq{req}))
	cur, err := h.current()
	if err != nil {
		return coreToolReply{}, err
	}
	text := strings.Join(h.tc.blocks[before:], "\n\n")
	if text == "" {
		text = "No new execution result was recorded. Do not claim this operation succeeded."
	}
	for _, sk := range h.tc.workSkills {
		text += "\nAurora playbook " + sk.ID + ":\n" + sk.Body
	}
	reply := coreToolReply{Text: h.redact(text)}
	if h.tc.pendingCmd != "" {
		h.state.Pending = h.tc.pendingCmd
		d := h.s.parked(h.tc, computerAskText)
		reply.Waiting = &coreCompletion{Status: "waiting", Revision: coreDigest([]byte(h.tc.pendingCmd)), Summary: d.Text, Evidence: []string{"operation_not_executed"}}
	}
	if req.Name == "computer_run" && h.tc.pendingCmd == "" {
		st := h.s.pcSnap(h.w.BotID)
		actual := folderCall(h.state.Folder, callsOf([]toolReq{req})[0]).arg
		if st.ran && st.runs > beforeRuns && st.lastCmd == actual {
			h.state.LastExit = &st.lastExit
		}
	}
	if len(cur.Direct.Tools) > 0 && cur.Direct.Tools[len(cur.Direct.Tools)-1].Status == "done" && (req.Name == "computer_write_file" || req.Name == "computer_edit_file") {
		call := folderCall(h.s.ext.bot(h.w.BotID).Folder, callsOf([]toolReq{req})[0])
		var actual struct {
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(call.arg), &actual) == nil && actual.Path != "" {
			found := false
			for _, p := range h.state.Written {
				if p == actual.Path {
					found = true
				}
			}
			if !found {
				h.state.Written = append(h.state.Written, actual.Path)
			}
		}
	}
	h.state.Effects[in.Operation] = coreEffect{Digest: digest, Status: "completed", Reply: reply}
	h.state.Presentation = cur.Direct
	if err := h.state.save(); err != nil {
		return coreToolReply{}, err
	}
	return reply, nil
}

func (h *auroraTurnHost) verify(ctx context.Context, answer string) coreCompletion {
	issues, evidence := []string{}, []string{}
	d := ParseDirectives(answer)
	h.s.networkFilterDirectives(h.w, &d)
	if d.Memory != "" || len(d.Schedules) > 0 || len(d.Handoffs) > 0 || len(d.Tools) > 0 {
		return coreCompletion{Status: "incomplete", Revision: "unexecuted-directive", Summary: "Use the advertised Aurora tools for this change. Text directives have not executed any action; do not claim they succeeded.", Evidence: []string{}}
	}
	if d.Approval != nil || d.Secret != nil || questionsOf(d) != nil {
		return coreCompletion{Status: "waiting", Revision: coreDigest([]byte(answer)), Summary: d.Text, Evidence: []string{"awaiting_owner_input"}}
	}
	// A failed persistent change is not a completed memory/schedule operation.
	// A later successful correction for that same tool replaces the earlier failure.
	if current, err := h.current(); err == nil {
		stateTools := map[string]string{}
		for _, tool := range current.Direct.Tools {
			if isCoreStateTool(tool.Name) {
				stateTools[tool.Name] = tool.Status
			}
		}
		for name, status := range stateTools {
			if status != "done" {
				issues = append(issues, "state_change_not_saved:"+name)
			}
		}
		sort.Strings(issues)
	}
	if h.state.LastExit != nil && *h.state.LastExit != 0 {
		issues = append(issues, "last_command_failed")
	}
	paths := map[string]bool{}
	for _, p := range h.state.Written {
		paths[p] = true
	}
	for _, g := range dlLinkRe.FindAllStringSubmatch(answer, -1) {
		p, err := url.QueryUnescape(g[4])
		if err != nil || g[3] != h.w.BotID {
			issues = append(issues, "invalid_artifact_link")
			continue
		}
		paths[p] = true
	}
	ordered := make([]string, 0, len(paths))
	for p := range paths {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)
	for _, p := range ordered {
		if h.s.PC == nil {
			issues = append(issues, "computer_unavailable")
			break
		}
		f, err := h.s.PC.OpenDownload(ctx, h.w.BotID, p)
		if err != nil {
			issues = append(issues, "artifact_unavailable:"+p)
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(f, 4<<20+1))
		f.Close()
		if readErr != nil || len(data) == 0 || len(data) > 4<<20 {
			issues = append(issues, "artifact_not_checked:"+p)
			continue
		}
		evidence = append(evidence, "file:"+p+":sha256:"+coreDigest(data))
	}
	// Only verify actual effects and concrete artifact claims. Mentioning a
	// computer (including "do not check the file") does not require an artifact.
	if h.s.PC != nil && len(h.state.Written) > 0 {
		_, bad := h.s.verifyClaims(h.w.BotID, answer, true)
		for _, p := range bad {
			issues = append(issues, "unverified_claim:"+p)
		}
	}
	// Fingerprint only measured state, never timestamps or read counts.
	data, _ := json.Marshal([][]string{evidence, issues})
	v := coreCompletion{Status: "ready", Revision: coreDigest(data), Evidence: evidence, Summary: "The response is ready. Artifact evidence covers readable bytes, not visual or interaction quality."}
	if len(issues) > 0 {
		v.Status = "incomplete"
		v.Summary = "Chưa thể xác nhận bàn giao: " + clean(strings.Join(issues, ", "), 1800)
	}
	return v
}

func (h *auroraTurnHost) flush() {
	if h.pending == "" {
		return
	}
	h.s.publish("delta", map[string]any{"type": TEXT_MESSAGE_CONTENT, "agentId": threadOf(h.w), "from": h.w.BotID, "id": h.w.RequestID, "messageId": h.w.RequestID, "delta": h.pending, "text": h.draft.String()})
	h.pending, h.lastDelta = "", time.Now()
}

func (h *auroraTurnHost) flushUsage() {
	if !h.usageOpen {
		return
	}
	h.s.usage.add(h.w.BotID, "chat", h.usageIn, h.usageOut)
	h.usageOpen, h.usageIn, h.usageOut = false, 0, 0
}

func (h *auroraTurnHost) Event(kind string, raw json.RawMessage) error {
	if _, err := h.current(); err != nil {
		return err
	}
	switch kind {
	case "delta":
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return errors.New("core_protocol_invalid")
		}
		h.draft.WriteString(text)
		h.usageOut += len([]rune(text))
		h.pending += text
		if time.Since(h.lastDelta) >= 60*time.Millisecond {
			h.flush()
		}
	case "step":
		h.flush()
		h.flushUsage()
		h.newStep = true
	case "final":
		h.flush()
	case "host_input", "host_completion", "provider_attempt", "usage", "max_steps":
	default:
		return errors.New("core_event_invalid")
	}
	return nil
}

func (s *Service) runCoreTurn(ctx context.Context, w ledger.Work, core *Core) (*Approval, *questionSet, error) {
	state, err := openCoreAgentState(s.coreStateDir, w, core, s.ext.bot(w.BotID).Folder)
	if err != nil {
		return nil, nil, err
	}
	h := &auroraTurnHost{s: s, w: w, tc: &toolCtx{}, state: state, offered: map[string]ledger.Work{}}
	defer h.flushUsage()
	if state.Presentation != nil {
		if err := s.updateWork(w, func(d *ledger.DirectRun) {
			d.Segments, d.Steered, d.Tools, d.Sources = state.Presentation.Segments, state.Presentation.Steered, state.Presentation.Tools, state.Presentation.Sources
		}); err != nil {
			return nil, nil, err
		}
		h.tc.sources = state.Presentation.Sources
	}
	for _, effect := range state.Effects {
		if effect.Status == "started" {
			return nil, nil, errors.New("core_effect_unknown")
		}
	}
	if err := state.reconcileSnapshot(); err != nil {
		return nil, nil, err
	}
	if state.Pending != "" {
		h.tc.pendingCmd = state.Pending
		d := s.parked(h.tc, computerAskText)
		return d.Approval, nil, s.appendText(w.RequestID, d.Text)
	}
	// Exclude by request identity before personalization or compaction. Matching
	// text afterward can duplicate the current input or remove an older repeat.
	history := s.personalize(s.historyFor(w.BotID, w.RequestID, false))
	history = append(history, ChatMsg{Role: "system", Content: coreRuntimeNote(s.botName(w.BotID), core) + coreStateToolsPrompt})
	input := w.Text
	if s.networkPeerWork(w) {
		input = s.networkPeerInput(w)
	}
	for i := range history {
		history[i].Content = h.redact(history[i].Content)
	}
	var result coreAgentResult
	if state.Result != nil {
		result = *state.Result
	} else {
		result, err = core.Agent(ctx, coreAgentStart{Version: 1, TaskID: w.RequestID, History: history, Instruction: h.redact(input), MaxSteps: 40, ContextWindow: 131072, Snapshot: state.Snapshot}, h)
		h.flush()
		if err != nil {
			return nil, nil, err
		}
		state.Result = &result
		if err := state.save(); err != nil {
			return nil, nil, err
		}
	}
	if err := s.updateWork(w, func(d *ledger.DirectRun) {
		d.Completion = &ledger.DirectCompletion{Status: result.Completion.Status, Revision: result.Completion.Revision, Summary: result.Completion.Summary, Evidence: result.Completion.Evidence}
	}); err != nil {
		return nil, nil, err
	}
	d := ParseDirectives(result.Text)
	s.networkFilterDirectives(w, &d)
	if d.Secret != nil {
		if err := s.updateWork(w, func(run *ledger.DirectRun) { secret := *d.Secret; run.Secret = &secret }); err != nil {
			return nil, nil, err
		}
	}
	if state.Pending != "" {
		d = s.parked(h.tc, computerAskText)
	}
	if result.Completion.Status == "incomplete" {
		d.Text = result.Completion.Summary
	}
	if err := s.appendText(w.RequestID, d.Text); err != nil {
		return nil, nil, err
	}
	return d.Approval, questionsOf(d), nil
}

var _ coreAgentHost = (*auroraTurnHost)(nil)
