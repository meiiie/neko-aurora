package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"nekobot.local/native/internal/protocol"
)

func permissionGroupStage(t *testing.T, stage int) (*Store, *profileServiceFixture, Collaboration, Work) {
	t.Helper()
	s, f, g := profileGroup(t)
	var w Work
	for index := 1; index <= stage; index++ {
		g, w = advanceGroup(t, s, g.ID)
		var b CoreBinding
		w, b = admittedRecoveryStage(t, s, w)
		if index < stage {
			if err := s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
				t.Fatal(err)
			}
			serviceStopped(t, s, w, b)
		}
	}
	return s, f, g, w
}

func permissionFixtureFrame(t *testing.T, w Work, arguments any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"version": 1, "sessionId": w.Core.SessionID, "toolCallId": "fixture-permission-call", "name": "mcp__nekobot__execute_tool", "arguments": arguments, "task": w.Core.Task, "execution": w.Core.Execution, "contextAdmission": w.Core.ContextAdmission})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func permissionWriteArguments() map[string]any {
	return map[string]any{"name": "artifact.write_text", "arguments": map[string]any{"filename": "permission-final.md", "text": "# Scoped permission fixture\n"}}
}

func permissionInvoke(t *testing.T, s *Store, w Work, frame json.RawMessage) map[string]any {
	t.Helper()
	method := reflect.ValueOf(s).MethodByName("PreflightCoreProfileTool")
	if !method.IsValid() {
		t.Fatal("missing additive effect-free Core profile tool preflight")
	}
	values := method.Call([]reflect.Value{reflect.ValueOf(w.RequestID), reflect.ValueOf(w.Epoch), reflect.ValueOf(frame)})
	if len(values) != 1 {
		t.Fatal("preflight result schema")
	}
	raw, ok := values[0].Interface().(json.RawMessage)
	if !ok {
		t.Fatal("preflight result is not a closed JSON decision")
	}
	var result map[string]any
	if json.Unmarshal(raw, &result) != nil || len(result) != 3 || result["version"] != float64(1) {
		t.Fatal("preflight decision schema")
	}
	if _, ok = result["allowed"].(bool); !ok {
		t.Fatal("preflight decision is not boolean")
	}
	if _, ok = result["requestSha256"].(string); !ok {
		t.Fatal("preflight decision hash schema")
	}
	return result
}

func permissionJournal(t *testing.T, s *Store) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(s.dir, "transactions.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestProfileToolPermissionFinalWriterPreflightHasNoEffects(t *testing.T) {
	s, _, g, w := permissionGroupStage(t, 3)
	arguments := permissionWriteArguments()
	frame := permissionFixtureFrame(t, w, arguments)
	before := permissionJournal(t, s)
	groupBefore := groupState(t, s, g.ID)
	envelope, _ := json.Marshal(map[string]any{"name": "execute_tool", "arguments": arguments})
	canonical, err := protocol.Canonical(envelope)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(canonical)
	for count := 0; count < 3; count++ {
		result := permissionInvoke(t, s, w, frame)
		if result["allowed"] != true || result["requestSha256"] != hex.EncodeToString(hash[:]) {
			t.Fatal("exact final writer denied or unbound")
		}
	}
	current, _ := s.Get(w.RequestID)
	if !bytes.Equal(before, permissionJournal(t, s)) || !reflect.DeepEqual(groupBefore, groupState(t, s, g.ID)) || len(current.Operations) != 0 || len(current.Artifacts) != 0 {
		t.Fatal("preflight created effect/reservation/journal record")
	}
	if _, err = os.Stat(filepath.Join(s.dir, "artifacts")); !os.IsNotExist(err) {
		t.Fatal("preflight created artifact storage")
	}
}

func TestProfileToolPermissionEarlyStageWriteDenied(t *testing.T) {
	for _, stage := range []int{1, 2} {
		t.Run(string(rune('0'+stage)), func(t *testing.T) {
			s, _, _, w := permissionGroupStage(t, stage)
			before := permissionJournal(t, s)
			if permissionInvoke(t, s, w, permissionFixtureFrame(t, w, permissionWriteArguments()))["allowed"] != false {
				t.Fatal("early participant acquired final writer permission")
			}
			if !bytes.Equal(before, permissionJournal(t, s)) {
				t.Fatal("denied preflight mutated journal")
			}
		})
	}
}

func TestProfileToolPermissionClosedFrameAndScope(t *testing.T) {
	s, _, _, w := permissionGroupStage(t, 3)
	good := permissionFixtureFrame(t, w, permissionWriteArguments())
	cases := map[string]func(map[string]any){
		"foreign_tool":     func(v map[string]any) { v["name"] = "mcp__other__execute_tool" },
		"unknown_field":    func(v map[string]any) { v["grant"] = true },
		"missing_args":     func(v map[string]any) { delete(v, "arguments") },
		"foreign_session":  func(v map[string]any) { v["sessionId"] = strings.Repeat("f", 32) },
		"missing_context":  func(v map[string]any) { delete(v, "contextAdmission") },
		"foreign_task":     func(v map[string]any) { v["task"].(map[string]any)["activationId"] = strings.Repeat("f", 32) },
		"arbitrary_action": func(v map[string]any) { v["arguments"].(map[string]any)["name"] = "bash" },
		"schema_widening": func(v map[string]any) {
			v["arguments"].(map[string]any)["arguments"].(map[string]any)["overwrite"] = true
		},
		"path_escape": func(v map[string]any) {
			v["arguments"].(map[string]any)["arguments"].(map[string]any)["filename"] = "../outside.md"
		},
		"unpinned_file": func(v map[string]any) {
			v["arguments"] = map[string]any{"name": "project.read_file", "arguments": map[string]any{"fileId": "file-" + strings.Repeat("1", 32), "offset": 0, "bytes": 10}}
		},
		"unpinned_report": func(v map[string]any) {
			v["arguments"] = map[string]any{"name": "report.read_artifact", "arguments": map[string]any{"deliveryId": "report-delivery-" + strings.Repeat("1", 32), "artifactId": strings.Repeat("1", 64), "offset": 0, "bytes": 10}}
		},
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			json.Unmarshal(good, &value)
			alter(value)
			frame, _ := json.Marshal(value)
			before := permissionJournal(t, s)
			if permissionInvoke(t, s, w, frame)["allowed"] != false {
				t.Fatal("foreign or widened frame approved")
			}
			if !bytes.Equal(before, permissionJournal(t, s)) {
				t.Fatal("malformed preflight mutated journal")
			}
		})
	}
}

func TestProfileToolPermissionExpiryIsReadOnly(t *testing.T) {
	s, _, g, w := permissionGroupStage(t, 3)
	s.now = func() time.Time { return time.UnixMilli(g.CoreProfileGroup.Plan.DeadlineAtUnixMS) }
	before := permissionJournal(t, s)
	if permissionInvoke(t, s, w, permissionFixtureFrame(t, w, permissionWriteArguments()))["allowed"] != false {
		t.Fatal("expired permission approved")
	}
	if !bytes.Equal(before, permissionJournal(t, s)) || groupState(t, s, g.ID).BudgetClosedAt != "" {
		t.Fatal("read-only preflight closed durable budget")
	}
}

func TestProfileToolPermissionFreshAuthorityAfterVerify(t *testing.T) {
	s, f, g, w := permissionGroupStage(t, 3)
	f.mu.Lock()
	f.verify = func(CoreProfilePin) error {
		s.mu.Lock()
		changed := s.collaborations[g.ID]
		changed.Status = "cancelling"
		s.collaborations[g.ID] = changed
		s.mu.Unlock()
		return nil
	}
	f.mu.Unlock()
	before := permissionJournal(t, s)
	if permissionInvoke(t, s, w, permissionFixtureFrame(t, w, permissionWriteArguments()))["allowed"] != false {
		t.Fatal("off-lock group change missed")
	}
	if !bytes.Equal(before, permissionJournal(t, s)) {
		t.Fatal("preflight mutated journal while verifying")
	}
}

func TestProfileToolPermissionRevokeBetweenPreflightAndExecute(t *testing.T) {
	s, _, g, w := permissionGroupStage(t, 3)
	args := permissionWriteArguments()
	if permissionInvoke(t, s, w, permissionFixtureFrame(t, w, args))["allowed"] != true {
		t.Fatal("fixture preflight denied")
	}
	c := g.CoreProfileGroup.OriginalPin
	if _, _, err := s.ControlCoreProfile(CoreProfileConsentControlInput{ConsentID: c.ConsentID, ExpectedRevision: c.ConsentRevision, Action: "revoke", ClientNonce: "permission-revoke-after-preflight"}); err != nil {
		t.Fatal(err)
	}
	var task map[string]any
	json.Unmarshal(w.Core.Task, &task)
	operation := map[string]any{"version": 1, "operationId": strings.Repeat("e", 32), "sessionId": w.Core.SessionID, "taskId": task["id"], "root": task["root"], "activationEpoch": task["activationEpoch"], "activationId": task["activationId"]}
	frame, _ := json.Marshal(map[string]any{"name": "execute_tool", "arguments": args, "_meta": map[string]any{"neko.operation": operation, "neko.contextAdmission": w.Core.ContextAdmission}})
	if _, err := s.ExecuteTool(w.RequestID, w.Epoch, frame); err == nil {
		t.Fatal("approved preflight replaced fresh execution authority")
	}
	current, _ := s.Get(w.RequestID)
	if len(current.Artifacts) != 0 || len(current.Operations) != 0 {
		t.Fatal("effect after revoke")
	}
}

func TestProfileToolPermissionExpiryBetweenPreflightAndExecute(t *testing.T) {
	s, _, g, w := permissionGroupStage(t, 3)
	args := permissionWriteArguments()
	if permissionInvoke(t, s, w, permissionFixtureFrame(t, w, args))["allowed"] != true {
		t.Fatal("fixture preflight denied")
	}
	s.now = func() time.Time { return time.UnixMilli(g.CoreProfileGroup.Plan.DeadlineAtUnixMS) }
	var task map[string]any
	json.Unmarshal(w.Core.Task, &task)
	op := map[string]any{"version": 1, "operationId": strings.Repeat("e", 32), "sessionId": w.Core.SessionID, "taskId": task["id"], "root": task["root"], "activationEpoch": task["activationEpoch"], "activationId": task["activationId"]}
	frame, _ := json.Marshal(map[string]any{"name": "execute_tool", "arguments": args, "_meta": map[string]any{"neko.operation": op, "neko.contextAdmission": w.Core.ContextAdmission}})
	if _, err := s.ExecuteTool(w.RequestID, w.Epoch, frame); err == nil {
		t.Fatal("preflight renewed original deadline")
	}
	current, _ := s.Get(w.RequestID)
	if len(current.Operations) != 0 || len(current.Artifacts) != 0 {
		t.Fatal("effect after deadline")
	}
}

func TestProfileToolPermissionNativeEpochAndPinnedPolicy(t *testing.T) {
	changes := map[string]func(*Work){
		"epoch":                     func(w *Work) { w.Epoch++ },
		"stopped":                   func(w *Work) { w.RuntimeStopped = true },
		"missing_native_binding":    func(w *Work) { w.RuntimeAttemptBinding = nil },
		"foreign_native_generation": func(w *Work) { w.Core.NativeProcessGeneration = "1" },
		"missing_context":           func(w *Work) { w.Core.ContextAdmission = nil },
		"missing_lease":             func(w *Work) { w.CurrentAttemptLease = nil },
		"foreign_plugin":            func(w *Work) { w.PluginPins[0].Tool = "bash" },
		"orphan_group":              func(w *Work) { w.CoreProfileGroupBinding = nil },
		"scope0":                    func(w *Work) { w.ExecutorPolicyVersion = 0; w.ExecutorPin = nil; w.CurrentAttemptLease = nil },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			s, _, _, w := permissionGroupStage(t, 3)
			frame := permissionFixtureFrame(t, w, permissionWriteArguments())
			changed := copyWork(w)
			change(&changed)
			s.mu.Lock()
			s.work[w.RequestID] = changed
			if name == "scope0" {
				s.executorRequired = false
			}
			s.mu.Unlock()
			before := permissionJournal(t, s)
			if permissionInvoke(t, s, w, frame)["allowed"] != false {
				t.Fatal("stale native/scope/policy approved")
			}
			if !bytes.Equal(before, permissionJournal(t, s)) {
				t.Fatal("denied preflight journal mutation")
			}
		})
	}
}

func TestProfileToolPermissionOrdinarySelectedProfileScope(t *testing.T) {
	s, _, _, c, ordinary, _, _ := groupProfileSetup(t)
	w := acceptProfile(t, s, c, ordinary, "permission-ordinary-profile")
	var err error
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := executorBinding(w, 501)
	b.NativeProcessGeneration = "134362178434013773"
	b.Task, _ = protocol.Canonical(b.Task)
	b.Execution, _ = protocol.Canonical(b.Execution)
	b.ContextAdmission = &ContextAdmission{Version: 1, SessionID: b.SessionID, ContextPinSHA256: contextHash(w), ManifestSHA256: w.RuntimeContextSHA256, SourceIDs: []string{strings.Repeat("d", 64)}, Task: b.Task, Execution: b.Execution}
	if err = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, b); err != nil {
		t.Fatal(err)
	}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	before := permissionJournal(t, s)
	if permissionInvoke(t, s, w, permissionFixtureFrame(t, w, permissionWriteArguments()))["allowed"] != true {
		t.Fatal("explicit ordinary selected profile scope rejected")
	}
	if !bytes.Equal(before, permissionJournal(t, s)) {
		t.Fatal("ordinary preflight mutated journal")
	}
}

func TestProfileToolPermissionPinnedReadRequiresExplicitOffset(t *testing.T) {
	s, _, _, w := permissionGroupStage(t, 3)
	id := "file-" + strings.Repeat("a", 32)
	w.PluginPins = append(w.PluginPins, PluginPin{Tool: "project.read_file"})
	w.Resources = &ResourcePin{Files: []UploadedFile{{ID: id}}}
	w.RuntimeContext.Sources = append(w.RuntimeContext.Sources, ManifestSource{ID: id, Kind: "retrieved", Span: ManifestSpan{Offset: 0, End: 4, Text: "text"}})
	good := map[string]any{"fileId": id, "offset": 0, "bytes": 4}
	raw, _ := json.Marshal(good)
	if !s.profilePermissionArgumentsAllowed(w, coreProfilePermissionArguments{Name: "project.read_file", Arguments: raw}) {
		t.Fatal("explicit offset0 pinned read rejected")
	}
	delete(good, "offset")
	raw, _ = json.Marshal(good)
	if s.profilePermissionArgumentsAllowed(w, coreProfilePermissionArguments{Name: "project.read_file", Arguments: raw}) {
		t.Fatal("required offset omitted but approved")
	}
	good["offset"] = nil
	raw, _ = json.Marshal(good)
	if s.profilePermissionArgumentsAllowed(w, coreProfilePermissionArguments{Name: "project.read_file", Arguments: raw}) {
		t.Fatal("null offset approved")
	}
}
