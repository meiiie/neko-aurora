package aurora

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/privatefs"
	"nekoaurora.local/native/internal/tools"
)

func coreHostFixture(t *testing.T) (*auroraTurnHost, *Core) {
	t.Helper()
	t.Setenv("AURORA_COMPUTER", "files")
	s, closeService := historyService(t, t.TempDir())
	t.Cleanup(closeService)
	s.Tools = tools.New()
	w, _, err := s.L.Accept(ledger.Input{BotID: firstBot(s), ClientNonce: "core-fixture", Text: "Create a landing page for Neko Aurora"})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.L.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "neko-core-cli", Model: "glm-fixture", Engine: "core"})
	if err != nil {
		t.Fatal(err)
	}
	c := &Core{Model: "glm-fixture", Profile: "zai-openai"}
	state, err := openCoreAgentState(s.coreStateDir, w, c)
	if err != nil {
		t.Fatal(err)
	}
	return &auroraTurnHost{s: s, w: w, tc: &toolCtx{}, state: state, offered: map[string]ledger.Work{}}, c
}

func rawCore(t *testing.T, value any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func coreCallSnapshot(t *testing.T, op, content string) (json.RawMessage, json.RawMessage) {
	t.Helper()
	args := map[string]any{"path": "index.html", "content": content, "append": true}
	call := map[string]any{"name": "computer_write_file", "arguments": args}
	request := rawCore(t, map[string]any{"name": "computer_write_file", "arguments": args, "operationId": op})
	message := map[string]any{"role": "assistant", "content": "I am creating the page.", "_neko_host_segment": "segment-one", "_neko_host_segment_at": time.Now().UTC().Format(time.RFC3339Nano),
		"tool_calls":            []any{map[string]any{"id": "call-1", "type": "function", "function": map[string]any{"name": "mcp__aurora__execute", "arguments": string(rawCore(t, call))}}},
		"_neko_tool_operations": []any{map[string]any{"version": 1, "operationId": op, "toolCallId": "call-1"}}}
	return rawCore(t, map[string]any{"version": 1, "messages": []any{message}, "acceptedInputs": []string{}}), request
}

func TestCoreHostDurableEffectReplayAndBinding(t *testing.T) {
	h, c := coreHostFixture(t)
	op := strings.Repeat("a", 32)
	snapshot, request := coreCallSnapshot(t, op, "<!doctype html><h1>Aurora</h1>")
	if _, err := h.execute(context.Background(), request); err == nil {
		t.Fatal("effect ran without checkpoint")
	}
	if err := h.checkpoint(snapshot); err != nil {
		t.Fatal(err)
	}
	a, err := h.execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	h.state, err = openCoreAgentState(h.s.coreStateDir, h.w, c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.execute(context.Background(), request)
	if err != nil || a.Text != b.Text {
		t.Fatalf("receipt replay %v", err)
	}
	text, _, err := h.s.PC.ReadFile(h.w.BotID, "index.html")
	if err != nil || strings.Count(text, "<!doctype html>") != 1 {
		t.Fatal("append executed twice")
	}
	_, changed := coreCallSnapshot(t, op, "different")
	if _, err := h.execute(context.Background(), changed); err == nil {
		t.Fatal("operation allowed different arguments")
	}
	h.state.Effects[op] = coreEffect{Digest: h.state.Effects[op].Digest, Status: "started"}
	if _, err := h.execute(context.Background(), request); err == nil || err.Error() != "core_effect_unknown" {
		t.Fatal("unknown effect replayed", err)
	}
	c.Model = "other-model"
	if _, err := openCoreAgentState(h.s.coreStateDir, h.w, c); err == nil {
		t.Fatal("checkpoint accepted a different model")
	}
}

func TestCoreHostInputReceiptAndStableSegments(t *testing.T) {
	h, _ := coreHostFixture(t)
	q, _, err := h.s.L.Accept(ledger.Input{BotID: h.w.BotID, ClientNonce: "follow-up", Text: "Also write a professional email"})
	if err != nil {
		t.Fatal(err)
	}
	in := h.inputs()
	if len(in) != 1 || in[0]["id"] != q.RequestID {
		t.Fatal("input not offered")
	}
	cur, _ := h.current()
	if len(cur.Direct.Steered) != 0 {
		t.Fatal("reading inbox acknowledged input")
	}
	msg := []any{map[string]any{"role": "assistant", "content": "First complete answer", "_neko_host_segment": "first", "_neko_host_segment_at": time.Now().UTC().Format(time.RFC3339Nano)},
		map[string]any{"role": "user", "content": q.Text, "_neko_host_input": q.RequestID}}
	raw := rawCore(t, map[string]any{"version": 1, "messages": msg, "acceptedInputs": []string{q.RequestID}})
	for i := 0; i < 2; i++ {
		if err := h.checkpoint(raw); err != nil {
			t.Fatal(err)
		}
	}
	cur, _ = h.current()
	if len(cur.Direct.Steered) != 1 || len(cur.Direct.Segments) != 1 {
		t.Fatal("input or prose duplicated")
	}
	if len(h.inputs()) != 0 {
		t.Fatal("accepted input redelivered")
	}
	view := workMessages(cur, true)
	if len(view) != 2 || !view[1].Segment || view[1].Interrupted || view[1].Text != "First complete answer" {
		t.Fatal("segment projected as interrupted or missing")
	}
	if _, err := h.s.L.DirectResume(h.w.RequestID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Request(context.Background(), "inputs", []byte("{}")); err == nil {
		t.Fatal("stale attempt still admitted")
	}
}

func TestCoreHostCheckpointFailureDoesNotAcknowledge(t *testing.T) {
	h, _ := coreHostFixture(t)
	q, _, _ := h.s.L.Accept(ledger.Input{BotID: h.w.BotID, ClientNonce: "follow-up", Text: "Keep this request"})
	h.inputs()
	h.state.file = t.TempDir() // atomic rename onto directory must fail
	raw := rawCore(t, map[string]any{"version": 1, "messages": []any{map[string]any{"role": "user", "content": q.Text, "_neko_host_input": q.RequestID}}, "acceptedInputs": []string{q.RequestID}})
	if h.checkpoint(raw) == nil {
		t.Fatal("checkpoint unexpectedly saved")
	}
	cur, _ := h.current()
	if len(cur.Direct.Steered) != 0 {
		t.Fatal("lost unpersisted owner input")
	}
}

func TestCoreHostSegmentsInterleaveWithAcceptedInputs(t *testing.T) {
	h, _ := coreHostFixture(t)
	q1, _, err := h.s.L.Accept(ledger.Input{BotID: h.w.BotID, ClientNonce: "anchor-first", Text: "First addition"})
	if err != nil {
		t.Fatal(err)
	}
	q2, _, err := h.s.L.Accept(ledger.Input{BotID: h.w.BotID, ClientNonce: "anchor-second", Text: "Second addition"})
	if err != nil {
		t.Fatal(err)
	}
	h.inputs()
	segment := func(id, text string) any {
		return map[string]any{"role": "assistant", "content": text, "_neko_host_segment": id, "_neko_host_segment_at": time.Now().UTC().Format(time.RFC3339Nano)}
	}
	msg := []any{segment("before", "Before additions"),
		map[string]any{"role": "user", "content": q1.Text, "_neko_host_input": q1.RequestID}, segment("first-answer", "Answer to first addition"),
		map[string]any{"role": "user", "content": q2.Text, "_neko_host_input": q2.RequestID}, segment("second-answer", "Answer to second addition"),
		map[string]any{"role": "tool", "content": "A completed observation"}}
	h.draft.WriteString("Before additions")
	if err := h.checkpoint(rawCore(t, map[string]any{"version": 1, "messages": msg, "acceptedInputs": []string{q1.RequestID, q2.RequestID}})); err != nil {
		t.Fatal(err)
	}
	if h.draft.Len() != 0 {
		t.Fatal("provider-managed tool kept the retired draft")
	}
	cur, _ := h.current()
	if cur.Direct.Segments[1].AfterInputID != q1.RequestID || cur.Direct.Segments[2].AfterInputID != q2.RequestID {
		t.Fatal("lost canonical input anchors")
	}
	var texts []string
	for _, m := range h.s.view().messages(h.w.BotID, 0) {
		texts = append(texts, m.Text)
	}
	expected := []string{h.w.Text, "Before additions", q1.Text, "Answer to first addition", q2.Text, "Answer to second addition"}
	if strings.Join(texts, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("input/segment order differs: %q", texts)
	}
	var history []string
	for _, m := range h.s.historyFor(h.w.BotID, "next-task", false) {
		if m.Role != "system" {
			history = append(history, m.Content)
		}
	}
	if strings.Join(history, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("next-turn history reordered the conversation: %q", history)
	}
}

func TestCoreHostHistoryLeavesCompactionToCore(t *testing.T) {
	h, _ := coreHostFixture(t)
	for i := 0; i < keepFullTurns+stewardBlock+1; i++ {
		w, _, err := h.s.L.Accept(ledger.Input{BotID: h.w.BotID, ClientNonce: "history-compaction-" + itoa(i), Text: "Older owner request " + itoa(i)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.s.L.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "neko-core-cli"}); err != nil {
			t.Fatal(err)
		}
		if _, err = h.s.L.DirectFinish(w.RequestID, w.Epoch, "completed", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	msgs := h.s.historyFor(h.w.BotID, "next-task", false)
	if cutPoint(msgs[1:]) < 0 {
		t.Fatal("fixture did not reach old host compaction threshold")
	}
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "CHECKPOINT CÁC LƯỢT CŨ") {
			t.Fatal("host compacted owner history into policy")
		}
	}
}

func TestCoreHostRecoveryRepairsOnlyMissingObservation(t *testing.T) {
	h, c := coreHostFixture(t)
	snapshot, request := coreCallSnapshot(t, strings.Repeat("b", 32), "<!doctype html><h1>Recovery</h1>")
	if err := h.checkpoint(snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := h.execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	state, err := openCoreAgentState(h.s.coreStateDir, h.w, c)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.reconcileSnapshot(); err != nil {
		t.Fatal(err)
	}
	first := string(state.Snapshot)
	if !strings.Contains(first, `"tool_call_id":"call-1"`) || !strings.Contains(first, "Đã ghi") {
		t.Fatal("receipt missing from recovered trajectory")
	}
	if err := state.reconcileSnapshot(); err != nil || string(state.Snapshot) != first {
		t.Fatal("repeated recovery duplicated observation")
	}
	if err := h.s.ext.setBot(h.w.BotID, botExt{Folder: "different"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Request(context.Background(), "inputs", []byte("{}")); err == nil || err.Error() != "core_workspace_changed" {
		t.Fatal("workspace switched within a turn", err)
	}
}

func TestCoreHostCompactedSegmentAnchorsAndUnknownInputs(t *testing.T) {
	h, _ := coreHostFixture(t)
	q, _, err := h.s.L.Accept(ledger.Input{BotID: h.w.BotID, ClientNonce: "compacted-anchor", Text: "An addition"})
	if err != nil {
		t.Fatal(err)
	}
	h.inputs()
	if err = h.checkpoint(rawCore(t, map[string]any{"version": 1, "acceptedInputs": []string{q.RequestID}, "messages": []any{
		map[string]any{"role": "user", "content": q.Text, "_neko_host_input": q.RequestID},
	}})); err != nil {
		t.Fatal(err)
	}
	compacted := map[string]any{"version": 1, "acceptedInputs": []string{q.RequestID}, "messages": []any{
		map[string]any{"role": "user", "content": "Compacted task summary"},
		map[string]any{"role": "assistant", "content": "Continued after compaction", "_neko_host_segment": "compacted-segment", "_neko_host_segment_at": time.Now().UTC().Format(time.RFC3339Nano), "_neko_host_after_input": q.RequestID},
		map[string]any{"role": "tool", "content": "Completed observation"},
	}}
	if err = h.checkpoint(rawCore(t, compacted)); err != nil {
		t.Fatal(err)
	}
	cur, _ := h.current()
	if len(cur.Direct.Segments) != 1 || cur.Direct.Segments[0].AfterInputID != q.RequestID {
		t.Fatal("compaction lost ordering receipt")
	}
	before := string(h.state.Snapshot)
	for _, marker := range []string{"_neko_host_input", "_neko_host_after_input"} {
		bad := rawCore(t, map[string]any{"version": 1, "acceptedInputs": []string{q.RequestID}, "messages": []any{
			map[string]any{"role": "user", "content": "Unknown receipt", marker: "not-accepted"},
		}})
		if err = h.checkpoint(bad); err == nil || err.Error() != "core_input_receipt_invalid" {
			t.Fatal("unknown input receipt admitted", marker, err)
		}
		if string(h.state.Snapshot) != before {
			t.Fatal("invalid receipt persisted")
		}
	}
}

func TestCoreHostEvidenceChangesOnlyWithArtifact(t *testing.T) {
	h, _ := coreHostFixture(t)
	h.state.Written = []string{"index.html"}
	a := h.verify(context.Background(), "Not ready")
	if a.Status != "incomplete" {
		t.Fatal("missing artifact marked ready")
	}
	if err := h.s.PC.WriteFile(h.w.BotID, "index.html", "<!doctype html><h1>Aurora</h1>"); err != nil {
		t.Fatal(err)
	}
	b := h.verify(context.Background(), "Ready")
	c := h.verify(context.Background(), "A differently worded response")
	if b.Status != "ready" || b.Revision != c.Revision || b.Revision == a.Revision {
		t.Fatal("read or wording changed evidence revision")
	}
	if err := h.s.PC.WriteFile(h.w.BotID, "index.html", "<!doctype html><h1>Changed</h1>"); err != nil {
		t.Fatal(err)
	}
	if d := h.verify(context.Background(), "Ready"); d.Revision == b.Revision {
		t.Fatal("edited artifact reused stale evidence")
	}
}

func TestCoreHostDoesNotInventArtifactRequirementsFromKeywords(t *testing.T) {
	h, _ := coreHostFixture(t)
	h.w.Text = "Which model are you? Do not run commands or check the website again."
	if !computerTask(h.w) {
		t.Fatal("fixture must expose the legacy keyword classifier")
	}
	verdict := h.verify(context.Background(), "I am Meiiie, using gpt-6.1-sol.")
	if verdict.Status != "ready" || len(verdict.Evidence) != 0 {
		t.Fatal("a factual answer was forced into artifact work", verdict)
	}
	code := 0
	h.state.LastExit = &code
	h.w.Text = "Run a calculation and report its output; no file is needed."
	if verdict = h.verify(context.Background(), "The command returned 42."); verdict.Status != "ready" {
		t.Fatal("successful command required an unrelated file", verdict)
	}
	answer := "[Download](/app/api/agents/" + h.w.BotID + "/computer/download?path=missing.html)"
	if verdict = h.verify(context.Background(), answer); verdict.Status != "incomplete" {
		t.Fatal("a concrete missing download was accepted", verdict)
	}
}

func TestCoreHostSavedFinalHandoffDoesNotCallModelAgain(t *testing.T) {
	h, c := coreHostFixture(t)
	h.state.Result = &coreAgentResult{Text: "The saved answer.", Completion: coreCompletion{Status: "incomplete", Revision: "saved-revision", Summary: "A requirement is still missing.", Evidence: []string{}}}
	if err := h.state.save(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.L.DirectResume(h.w.RequestID); err != nil {
		t.Fatal(err)
	}
	w, err := h.s.L.DirectStart(h.w.RequestID, h.w.Epoch, ledger.DirectRun{Runtime: "neko-core-cli", Engine: "core", Model: c.Model})
	if err != nil {
		t.Fatal(err)
	}
	// c deliberately has no executable or credential. A cached handoff must
	// not need either, and may not turn an incomplete task into success.
	ap, qs, err := h.s.runCoreTurn(context.Background(), w, c)
	if err != nil {
		t.Fatal(err)
	}
	h.s.finishWork(w, ap, qs)
	done, _ := h.s.L.Get(w.RequestID)
	if done.Reply != "A requirement is still missing." || taskStatus(done) != TaskIncomplete {
		t.Fatal("cached handoff restarted or lost its incomplete verdict")
	}
}

func TestCoreHostHistoryExcludesCurrentByIdentity(t *testing.T) {
	s, closeService := historyService(t, t.TempDir())
	defer closeService()
	s.owner = func() string { return "Owner" }
	bot := firstBot(s)
	for _, grouped := range []bool{false, true} {
		scope := ledger.SubmissionScope{}
		if grouped {
			bots := s.L.Snapshot().Bots
			ch, _, err := s.L.SaveChannel(ledger.ChannelInput{Name: "Core history", BotIDs: []string{bots[0].ID, bots[1].ID}, ClientNonce: "core-history-group"})
			if err != nil {
				t.Fatal(err)
			}
			scope = ledger.SubmissionScope{ChannelID: ch.ID, ExpectedChannelRevision: &ch.Revision}
		}
		prefix := "core-history-" + map[bool]string{false: "direct", true: "group"}[grouped]
		old, _, err := s.L.AcceptProduct(ledger.Input{BotID: bot, ClientNonce: prefix + "-old", Text: "Darren repeats this message"}, scope)
		if err != nil {
			t.Fatal(err)
		}
		old, err = s.L.DirectStart(old.RequestID, old.Epoch, ledger.DirectRun{Runtime: "direct-http"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.L.DirectFinish(old.RequestID, old.Epoch, "completed", "", nil); err != nil {
			t.Fatal(err)
		}
		cur, _, err := s.L.AcceptProduct(ledger.Input{BotID: bot, ClientNonce: prefix + "-current", Text: old.Text}, scope)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, m := range s.personalize(s.historyFor(bot, cur.RequestID, false)) {
			if m.Role == "user" && m.Content == "Owner repeats this message" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("group=%v: previous repeat lost or current input duplicated: %d", grouped, count)
		}
	}
}

func TestCoreHostFileBytesAreNotSilentlyRewritten(t *testing.T) {
	h, _ := coreHostFixture(t)
	content := strings.Repeat("Giả kim sư tìm hòn đá triết gia để biến thủy ngân thành vàng. ", 3) + "const label = 'đan石';"
	if fixed, _ := fixMixedScripts(content); fixed == content {
		t.Fatal("fixture must expose legacy rewriting")
	}
	arg := string(rawCore(t, map[string]any{"path": "literal.html", "content": content}))
	_, err := h.s.runComputer(context.WithValue(context.Background(), coreTurnKey{}, true), h.w.BotID, h.tc, toolCall{"computer_write_file", arg})
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := h.s.PC.ReadFile(h.w.BotID, "literal.html")
	if err != nil || got != content {
		t.Fatal("host changed bytes supplied by Core", err)
	}
}

func TestCoreHostDecidedApprovalNoLongerShowsWaitingLabel(t *testing.T) {
	h, _ := coreHostFixture(t)
	w := h.w
	w.Direct.Completion = &ledger.DirectCompletion{Status: "waiting", Revision: "approval", Summary: "Waiting"}
	w.Direct.Approval = &ledger.DirectApproval{State: "approved", Kind: "computer", Body: "echo fixture"}
	w.Reply = "Waiting"
	for _, message := range workMessages(w, false) {
		if message.ID == w.RequestID && message.Completion != nil {
			t.Fatal("decided approval still shown as pending")
		}
	}
}

func TestCoreActualAgentCLIWritesAndHandsOff(t *testing.T) {
	for _, profile := range []string{"zai-openai", "chatgpt"} {
		t.Run(profile, func(t *testing.T) { testCoreAgentHandoff(t, profile) })
	}
}

func testCoreAgentHandoff(t *testing.T, profile string) {
	h, c := coreHostFixture(t)
	c.Profile = profile
	bun := findCoreBun()
	if bun == "" {
		t.Skip("Bun unavailable; actual Core bridge not exercised")
	}
	root := t.TempDir()
	home, bin := filepath.Join(root, "home"), filepath.Join(root, "bin")
	for _, dir := range []string{home, bin, filepath.Join(home, ".neko-core")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := privatefs.ProtectDirectory(home); err != nil {
		t.Fatal(err)
	}
	entry, _ := filepath.Abs("../../../../core/bin/aurora-agent.ts")
	setup, _ := filepath.Abs("../../../../core/bin/aurora-setup.ts")
	entryJSON, setupJSON := rawCore(t, entry), rawCore(t, setup)
	botJSON := rawCore(t, h.w.BotID)
	if profile == "chatgpt" {
		creds := rawCore(t, map[string]any{"accessToken": "fixture-chatgpt-access", "refreshToken": "fixture-chatgpt-refresh", "expiresAt": time.Now().Add(time.Hour).UnixMilli()})
		if err := os.WriteFile(filepath.Join(home, ".neko-core", "chatgpt-auth.json"), creds, 0600); err != nil {
			t.Fatal(err)
		}
	}
	bootstrap := `import {pathToFileURL} from 'node:url';
const bot=` + string(botJSON) + `;
let calls=0;
globalThis.fetch=async(input,init)=>{
 const url=String(input);
 if(url==='https://api.z.ai/api/paas/v4/models')return Response.json({data:[{id:'glm-fixture'}]});
 if(url.startsWith('https://chatgpt.com/backend-api/codex/models'))return Response.json({models:[{slug:'glm-fixture',display_name:'Fixture',visibility:'list'}]});
 const chatgpt=url==='https://chatgpt.com/backend-api/codex/responses';
 if(!chatgpt&&url!=='https://api.z.ai/api/paas/v4/chat/completions')throw Error('unexpected fixture network destination');
 const body=JSON.parse(String(init?.body));
 if(body.tools?.length!==1||(chatgpt?body.tools[0].name:body.tools[0].function.name)!=='mcp__aurora__execute')throw Error('local tool escape or tools missing');
 if(++calls>2)throw Error('unexpected repeated model review');
 let delta,finish;
 if(calls===1){
  const args={name:'computer_write_file',arguments:{path:'index.html',content:'<!doctype html><html lang="vi"><meta charset="utf-8"><title>Neko Aurora</title><h1>Neko Aurora</h1><button onclick="this.textContent=\'Ready\'">Try</button></html>'}};
  delta={content:'I am creating the landing page.',tool_calls:[{index:0,id:'file1',type:'function',function:{name:'mcp__aurora__execute',arguments:JSON.stringify(args)}}]};finish='tool_calls';
 }else{
  if(!(chatgpt?body.input.some(m=>m.type==='function_call_output'&&String(m.output).includes('Đã ghi')):body.messages.some(m=>m.role==='tool'&&String(m.content).includes('Đã ghi'))))throw Error('missing actual tool receipt');
  delta={content:'[Download](/app/api/agents/'+bot+'/computer/download?path=index.html)'};finish='stop';
 }
 if(chatgpt){
  const events=[{type:'response.output_text.delta',delta:delta.content}];
  if(delta.tool_calls){const f=delta.tool_calls[0].function;events.push({type:'response.output_item.done',output_index:1,item:{id:'fc-1',type:'function_call',call_id:'file1',name:f.name,arguments:f.arguments}})}
  events.push({type:'response.completed',response:{output:[],usage:{input_tokens:4,output_tokens:5}}});
  return new Response(events.map(e=>'data: '+JSON.stringify(e)+'\n\n').join(''),{headers:{'content-type':'text/event-stream'}});
 }
 return new Response('data: '+JSON.stringify({choices:[{index:0,delta}]})+'\n\ndata: '+JSON.stringify({choices:[{index:0,delta:{},finish_reason:finish}]})+'\n\ndata: [DONE]\n\n',{headers:{'content-type':'text/event-stream'}});
};
await import(pathToFileURL(` + string(entryJSON) + `).href);`
	for name, data := range map[string]string{filepath.Join(bin, "aurora-agent.ts"): bootstrap, filepath.Join(bin, "aurora-setup.ts"): strings.Replace(bootstrap, string(entryJSON), string(setupJSON), 1), filepath.Join(root, "bunfig.neko.toml"): ""} {
		if err := os.WriteFile(name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c.Bun, c.Entry, c.Home, c.Quick, c.KeyEnv, c.key = bun, filepath.Join(bin, "aurora-agent.ts"), home, true, "ZAI_API_KEY", "fixture-only"
	if profile == "chatgpt" {
		c.KeyEnv, c.key, c.SignedIn = "", "", true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ap, qs, err := h.s.runCoreTurn(ctx, h.w, c)
	if err != nil {
		t.Fatal(err)
	}
	h.s.finishWork(h.w, ap, qs)
	w, _ := h.s.L.Get(h.w.RequestID)
	if w.Status != "completed" || w.Direct.Completion == nil || w.Direct.Completion.Status != "ready" || len(w.Direct.Tools) != 1 || len(w.Direct.Segments) != 1 {
		t.Fatalf("handoff mismatch: status=%s tools=%d segments=%d completion=%+v", w.Status, len(w.Direct.Tools), len(w.Direct.Segments), w.Direct.Completion)
	}
	if !strings.Contains(w.Reply, "/computer/download?path=index.html") {
		t.Fatal("download not handed off")
	}
	for _, action := range []string{"preview", "download"} {
		r := httptest.NewRecorder()
		h.s.ServeHTTP(r, httptest.NewRequest("GET", "/app/api/agents/"+w.BotID+"/computer/"+action+"?path=index.html&workspaceId="+url.QueryEscape(h.s.workspaceID), nil))
		if r.Code != 200 || !strings.Contains(r.Body.String(), "<h1>Neko Aurora</h1>") {
			t.Fatalf("%s: status=%d", action, r.Code)
		}
	}
}
