package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func pluginChange(t *testing.T, s *Store, id, action, nonce string, revision uint64) PluginRecord {
	t.Helper()
	p, dup, err := s.ControlPlugin(PluginInput{Version: 1, ID: id, Action: action, ExpectedRevision: revision, ClientNonce: nonce})
	if err != nil || dup {
		t.Fatalf("plugin %s: duplicate=%v err=%v", action, dup, err)
	}
	return p
}

func TestPluginRunningFenceStopReceiptAndCrashGap(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.Accept(input("plugin-running-001", "background"))
	if err != nil {
		t.Fatal(err)
	}
	w = admitGroup(t, s, w)
	// The operation first produced its durable artifact before policy revocation.
	result, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("retained durable bytes"))
	if err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	in := PluginInput{Version: 1, ID: "neko-artifacts", Action: "disable", ExpectedRevision: 0, ClientNonce: "plugin-crash-gap"}
	p, err := s.pluginMutation(in)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.commit(Event{Type: "plugin.lifecycle", Plugin: &PluginEvent{Version: 1, Input: in, Plugin: p}}); err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Sequence
	for _, call := range []func() error{
		func() error { return s.AppendText(w.RequestID, w.Epoch, "late canary") },
		func() error { return s.Finish(w.RequestID, w.Epoch, "completed", "") },
		func() error {
			_, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("retained durable bytes"))
			return err
		},
	} {
		if err = call(); !errors.Is(err, ErrConflict) {
			t.Fatal("commit-before-reconcile fence failed", err)
		}
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("denied action committed")
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
		current, _ := s.Get(w.RequestID)
		if current.Status != "suspending" || current.Epoch != w.Epoch+1 || !reflect.DeepEqual(current.Operations[0].Result, result) || current.Operations[0].Status != "completed" {
			t.Fatal("restart lost policy/effect", current)
		}
		if i == 2 {
			if err = s.MarkRuntimeStopped(current.RequestID, current.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: current.Core.SessionID}); err != nil {
				t.Fatal(err)
			}
			if err = s.RecoverInterrupted(); err != nil {
				t.Fatal(err)
			}
			current, _ = s.Get(w.RequestID)
			if current.Status != "suspended" {
				t.Fatal(current.Status)
			}
			if _, err = s.RefreshContext(RefreshInput{RequestID: current.RequestID, ExpectedEpoch: current.Epoch}); !errors.Is(err, ErrConflict) {
				t.Fatal("refresh rebound revoked own tool pin", err)
			}
		}
		s.Close()
	}
}

func TestPluginExplicitEmptyBrokerAndBuiltinSchemaImmutable(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, _, err := s.AcceptProduct(input("plugin-no-tools-001", "background"), SubmissionScope{PluginSelection: &[]PluginSelection{}})
	if err != nil {
		t.Fatal(err)
	}
	w = admitGroup(t, s, w)
	result, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("denied write"))
	if err != nil || !strings.Contains(string(result), "outside the immutable") {
		t.Fatal("tool-free write allowed", err, string(result))
	}
	after, _ := s.Get(w.RequestID)
	if len(after.Operations) != 0 || len(after.Artifacts) != 0 {
		t.Fatal("denied effect started")
	}
	p, _ := s.plugin("neko-artifacts")
	manifest := p.PluginManifest
	manifest.Version = "99.0.0"
	before := s.Snapshot().Sequence
	if _, _, err = s.ControlPlugin(PluginInput{Version: 1, ID: p.ID, Action: "update", ExpectedRevision: 0, ClientNonce: "plugin-schema-spoof", Manifest: &manifest}); !errors.Is(err, ErrInvalid) || before != s.Snapshot().Sequence {
		t.Fatal("client replaced trusted manifest", err)
	}
	updated := pluginChange(t, s, p.ID, "update", "plugin-trusted-update", 0)
	if updated.Version != p.Version || updated.Publisher != p.Publisher || !reflect.DeepEqual(updated.Tools, p.Tools) || updated.Revision != 1 {
		t.Fatal("server schema changed", updated)
	}
}

func TestPluginDispatchLinearizesAndNoNewEffectsAfterDisable(t *testing.T) {
	for i := 0; i < 12; i++ {
		s, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		w, _, err := s.Accept(input("plugin-race-001", "background"))
		if err != nil {
			t.Fatal(err)
		}
		w, err = s.Claim(w.RequestID, w.Epoch)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		var dispatched bool
		var dispatchErr, disableErr error
		go func() {
			defer wg.Done()
			dispatchErr = s.Dispatch(w.RequestID, w.Epoch, func() error { dispatched = true; return nil })
		}()
		go func() {
			defer wg.Done()
			_, _, disableErr = s.ControlPlugin(PluginInput{Version: 1, ID: "neko-artifacts", Action: "disable", ExpectedRevision: 0, ClientNonce: "plugin-race-disable"})
		}()
		wg.Wait()
		if disableErr != nil || dispatchErr != nil && !errors.Is(dispatchErr, ErrConflict) {
			t.Fatal(disableErr, dispatchErr)
		}
		if dispatchErr == nil && !dispatched || dispatchErr != nil && dispatched {
			t.Fatal("dispatch not linearized")
		}
		current, _ := s.Get(w.RequestID)
		if s.RuntimeAllowed(current.RequestID, current.Epoch) {
			t.Fatal("runtime retained revoked authority")
		}
		s.Close()
	}
}

func TestPluginCollaborationPinFrozenThroughAllStages(t *testing.T) {
	s, g, _, dir := collaborationFixture(t)
	if g.ToolPolicyVersion != 2 || len(g.PluginPins) != 1 {
		t.Fatal("group missing policy")
	}
	g, w := advanceGroup(t, s, g.ID)
	if !reflect.DeepEqual(w.PluginPins, g.PluginPins) {
		t.Fatal("stage changed group pin")
	}
	w = admitGroup(t, s, w)
	if err := s.AppendText(w.RequestID, w.Epoch, "plan"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	pluginChange(t, s, "neko-artifacts", "disable", "plugin-group-disable", 0)
	pluginChange(t, s, "neko-artifacts", "enable", "plugin-group-enable", 1)
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	g = groupState(t, s, g.ID)
	if g.Status != "suspended" || len(g.RequestIDs) != 1 {
		t.Fatal("next stage rebound old group", g)
	}
	s.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	g = groupState(t, s, g.ID)
	if g.Status != "suspended" || len(g.RequestIDs) != 1 {
		t.Fatal("restart revived group", g)
	}
}

func TestPluginLegacyRevisionZeroNeverRevived(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.Accept(input("plugin-legacy-001", "background"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	// A typed historical v0 journal fixture has no modern policy or context fields.
	path := filepath.Join(dir, "transactions.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rec record
	if err = json.Unmarshal(bytes.TrimSpace(raw), &rec); err != nil {
		t.Fatal(err)
	}
	rec.Event.Work.ToolPolicyVersion = 0
	rec.Event.Work.PluginSelection = nil
	rec.Event.Work.PluginPins = nil
	pin := rec.Event.Work.Context
	pin.ToolPolicyVersion = 0
	pin.PluginPins = nil
	pin.SHA256 = ""
	b, _ := json.Marshal(pin)
	h := sha256.Sum256(b)
	pin.SHA256 = hex.EncodeToString(h[:])
	rec.Digest, _ = digestRecord(rec)
	b, _ = json.Marshal(rec)
	if err = os.WriteFile(path, append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	if !s.pluginsAllowed(w) {
		t.Fatal("legacy v0 refused before lifecycle")
	}
	pluginChange(t, s, "neko-artifacts", "disable", "plugin-legacy-disable", 0)
	pluginChange(t, s, "neko-artifacts", "enable", "plugin-legacy-enable", 1)
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, _ = s.Get(w.RequestID)
	if s.pluginsAllowed(w) || s.contextAllowed(w) || w.ToolPolicyVersion != 0 {
		t.Fatal("legacy policy upgraded or revived")
	}
}

func TestPluginSourceRevocationCoversReadAndInlineContext(t *testing.T) {
	s, c, p := productFixture(t)
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "plugin-source-upload", Filename: "canary.md", Content: []byte("source plugin canary")})
	if err != nil {
		t.Fatal(err)
	}
	scope := SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}}
	explicit := scope
	explicit.PluginSelection = &[]PluginSelection{}
	before := s.Snapshot().Sequence
	if _, _, err = s.AcceptProduct(input("plugin-source-empty", "background"), explicit); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("inline source bypasses tool-free policy", err)
	}
	w, _, err := s.AcceptProduct(input("plugin-source-work", "background"), scope)
	if err != nil {
		t.Fatal(err)
	}
	w = admitGroup(t, s, w)
	var frame map[string]any
	_ = json.Unmarshal(invocation("unused"), &frame)
	frame["arguments"] = map[string]any{"name": "project.read_file", "arguments": map[string]any{"fileId": f.ID, "offset": 0, "bytes": 4000}}
	raw, _ := json.Marshal(frame)
	result, err := s.ExecuteTool(w.RequestID, w.Epoch, raw)
	if err != nil || !bytes.Contains(result, []byte("source plugin canary")) {
		t.Fatal("trusted bounded source read failed", err)
	}
	pluginChange(t, s, "neko-project-sources", "disable", "plugin-source-disable", 0)
	if _, err = s.ExecuteTool(w.RequestID, w.Epoch, raw); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked plugin reads source", err)
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "source plugin canary"); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked canary output", err)
	}
	before = s.Snapshot().Sequence
	if _, _, err = s.AcceptProduct(input("plugin-source-disabled", "background"), scope); !errors.Is(err, ErrConflict) || before != s.Snapshot().Sequence {
		t.Fatal("disabled read plugin bypassed through fresh inline excerpt", err)
	}
}

func TestPluginPromptBoundAndMixedJournalPayload(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	before := s.Snapshot().Sequence
	if _, _, err = s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "plugin-large-prompt", Lane: "foreground", Text: strings.Repeat("\\", 32000)}); !errors.Is(err, ErrInvalid) || s.Snapshot().Sequence != before {
		t.Fatal("encoded prompt bound after plugin pin", err)
	}
	in := PluginInput{Version: 1, ID: "neko-artifacts", Action: "disable", ClientNonce: "plugin-mixed-event"}
	p, err := s.pluginMutation(in)
	if err != nil {
		t.Fatal(err)
	}
	e := Event{Type: "plugin.lifecycle", Plugin: &PluginEvent{Version: 1, Input: in, Plugin: p}, TaskControl: &TaskControlEvent{Version: 1}}
	if validReplay(s, e) {
		t.Fatal("mixed plugin/control payload accepted")
	}
	e.Type = "collaboration.created"
	if validReplay(s, e) {
		t.Fatal("plugin authority smuggled into group event")
	}
}

func TestPluginRevocationBetweenClaimAndAdmissionKeepsOnlyStopProof(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, _, err := s.Accept(input("plugin-admission-race", "background"))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	binding := CoreBinding{NativeProcessID: 401, SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1,"authorityId":"fixture-authority"}`), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"}
	pluginChange(t, s, "neko-artifacts", "remove", "plugin-admission-remove", 0)
	if err = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, binding); err != nil {
		t.Fatal("lost current process binding", err)
	}
	if err = s.Admit(w.RequestID, w.Epoch, binding); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked task admitted", err)
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 402, SessionID: binding.SessionID}); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong PID proof", err)
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: binding.SessionID}); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "suspended" || current.Core != nil || current.RuntimeStopReceipt.NativeProcessID != 401 || current.PluginPins[0] != w.PluginPins[0] {
		t.Fatal("authority changed while recording stop proof", current)
	}
}
func TestPluginLifecycleCASNonceReplayAndExternalFailClosed(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest := &PluginManifest{Name: "Untrusted local descriptor", Version: "1.0", License: "unknown", Tools: []PluginTool{{Name: "artifact.write_text", Description: "spoof", InputSchema: json.RawMessage(`{"type":"object"}`)}}}
	in := PluginInput{Version: 1, ID: "external-local", Action: "import", ClientNonce: "plugin-import-001", Manifest: manifest}
	p, dup, err := s.ControlPlugin(in)
	if err != nil || dup || p.Status != "manifest_only_disabled" {
		t.Fatalf("external import: %v %+v", err, p)
	}
	if _, _, err = s.ControlPlugin(PluginInput{Version: 1, ID: p.ID, Action: "enable", ExpectedRevision: 1, ClientNonce: "plugin-enable-001"}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("external execution enabled", err)
	}
	if _, _, err = s.AcceptProduct(input("external-selection-001", "background"), SubmissionScope{PluginSelection: &[]PluginSelection{{ID: p.ID, ExpectedRevision: 1}}}); !errors.Is(err, ErrConflict) {
		t.Fatal("external manifest became authority", err)
	}
	same, dup, err := s.ControlPlugin(in)
	if err != nil || !dup || same.Revision != 1 {
		t.Fatal("lost ACK replay failed", err)
	}
	changed := in
	changed.Action = "remove"
	if _, _, err = s.ControlPlugin(changed); !errors.Is(err, ErrConflict) {
		t.Fatal("nonce body changed", err)
	}
	p = pluginChange(t, s, p.ID, "remove", "plugin-remove-001", 1)
	if p.Status != "removed" || p.Revision != 2 {
		t.Fatal(p)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, dup, err = s.ControlPlugin(in)
	if err != nil || !dup || p.Status != "removed" || p.Revision != 2 {
		t.Fatal("replay restored removed authority", err, p)
	}
	if _, _, err = s.ControlPlugin(PluginInput{Version: 1, ID: p.ID, Action: "import", ClientNonce: "plugin-reimport-001", Manifest: manifest}); !errors.Is(err, ErrConflict) {
		t.Fatal("tombstone reused", err)
	}
}
func TestPluginRevocationFencesDispatchOutputsEffectsAndAncestors(t *testing.T) {
	dir := t.TempDir()
	s, w := running(t, dir)
	if w.ToolPolicyVersion != 1 || len(w.PluginPins) != 1 || w.Context.ToolPolicyVersion != 1 {
		t.Fatal("new intake missing immutable plugin policy")
	}
	if _, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("fixture canary")); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendText(w.RequestID, w.Epoch, "fixture canary summary"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	child, _, err := s.AcceptProduct(input("plugin-child-001", "background"), SubmissionScope{PluginSelection: &[]PluginSelection{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(child.Context.Sources) != 1 {
		t.Fatal("missing canary ancestry")
	}
	independent, _, err := s.AcceptProduct(Input{BotID: "bot-davin", ClientNonce: "plugin-other-001", Text: "independent", Lane: "foreground"}, SubmissionScope{PluginSelection: &[]PluginSelection{}})
	if err != nil {
		t.Fatal(err)
	}
	p := pluginChange(t, s, "neko-artifacts", "disable", "plugin-disable-001", 0)
	childNow, _ := s.Get(child.RequestID)
	if childNow.Status != "suspended" || s.RuntimeAllowed(child.RequestID, childNow.Epoch) {
		t.Fatal("ancestor output still authorized", childNow)
	}
	if _, err = s.Claim(child.RequestID, child.Epoch); !errors.Is(err, ErrConflict) {
		t.Fatal("stale dispatch allowed", err)
	}
	if _, err = s.Claim(independent.RequestID, independent.Epoch); err != nil {
		t.Fatal("unrelated tool-free work fenced", err)
	}
	pluginChange(t, s, p.ID, "enable", "plugin-reenable-001", p.Revision)
	if s.contextAllowed(w) || s.contextAllowed(childNow) {
		t.Fatal("re-enable revived old pins/ancestry")
	}
	fresh, err := s.RefreshContext(RefreshInput{RequestID: childNow.RequestID, ExpectedEpoch: childNow.Epoch})
	if err != nil || len(fresh.Context.Sources) != 0 {
		t.Fatal("explicit fresh context retained revoked ancestry", err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	again, _ := s.Get(w.RequestID)
	if s.contextAllowed(again) {
		t.Fatal("restart revived pins")
	}
	if _, err = s.ExecuteTool(w.RequestID, w.Epoch, invocation("fixture canary")); !errors.Is(err, ErrConflict) {
		t.Fatal("cached effect receipt leaked after revocation", err)
	}
}
func TestPluginPinPreservedByTaskControlsAndNoImplicitRebind(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	selection := &[]PluginSelection{}
	w, _, err := s.AcceptProduct(input("plugin-control-001", "background"), SubmissionScope{PluginSelection: selection})
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := s.ControlTask(TaskControlInput{Version: 1, RequestID: w.RequestID, ExpectedEpoch: w.Epoch, Action: "supplement", ClientNonce: "plugin-supplement-001", Text: "continue separately"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Related.ToolPolicyVersion != w.ToolPolicyVersion || !reflect.DeepEqual(result.Related.PluginPins, w.PluginPins) || result.Related.PluginSelection == nil {
		t.Fatal("supplement rebound tools")
	}
	if strings.Contains(string(actionCapabilities(*result.Related)), "artifact.write_text") {
		t.Fatal("explicit empty gained a tool")
	}
	selectionWithTool := &[]PluginSelection{{ID: "neko-artifacts", ExpectedRevision: 0}}
	original, _, err := s.AcceptProduct(input("plugin-pinned-001", "background"), SubmissionScope{PluginSelection: selectionWithTool})
	if err != nil {
		t.Fatal(err)
	}
	pluginChange(t, s, "neko-artifacts", "disable", "plugin-disable-control", 0)
	pluginChange(t, s, "neko-artifacts", "enable", "plugin-enable-control", 1)
	if _, _, err = s.ControlTask(TaskControlInput{Version: 1, RequestID: original.RequestID, ExpectedEpoch: original.Epoch, Action: "replace", ClientNonce: "plugin-replace-001", Text: "new"}); !errors.Is(err, ErrConflict) {
		t.Fatal("replacement silently rebound tools", err)
	}
	if _, _, err = s.AcceptProduct(input("plugin-pinned-001", "background"), SubmissionScope{PluginSelection: &[]PluginSelection{{ID: "neko-artifacts", ExpectedRevision: 2}}}); !errors.Is(err, ErrConflict) {
		t.Fatal("retry changed pin", err)
	}
}
