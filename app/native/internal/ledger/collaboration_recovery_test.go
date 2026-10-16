package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/protocol"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func configuredRecoveryGroup(t *testing.T, configure ...func(*Store, *CollaborationInput)) (*Store, *executors.Registry, Collaboration) {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "journal"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	r, e := executors.Open(filepath.Join(t.TempDir(), "registry"), executors.Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: "97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb", RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	p := r.Snapshot()
	if e = r.MarkReady(p.Pin, p.BootID); e != nil {
		t.Fatal(e)
	}
	s.ConfigureExecutor(r)
	s.ConfigureContextAdmission(true)
	c := sharedChannel(t, s, "Recovery group", "group-recovery-channel")
	in := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Complete the original finite goal", MaxStages: 3, ClientNonce: "group-recovery-create"}
	for _, f := range configure {
		f(s, &in)
	}
	g, _, e := s.CreateCollaboration(in)
	if e != nil {
		t.Fatal(e)
	}
	return s, r, g
}
func admittedRecoveryStage(t *testing.T, s *Store, w Work) (Work, CoreBinding) {
	t.Helper()
	w, e := s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	b := executorBinding(w, uint32(400+w.Collaboration.Stage))
	b.NativeProcessGeneration = "134362178434013773"
	b.Task, _ = protocol.Canonical(b.Task)
	b.Execution, _ = protocol.Canonical(b.Execution)
	b.ContextAdmission = &ContextAdmission{Version: 1, SessionID: b.SessionID, ContextPinSHA256: contextHash(w), ManifestSHA256: w.RuntimeContextSHA256, SourceIDs: []string{strings.Repeat("d", 64)}, Task: b.Task, Execution: b.Execution}
	if e = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, b); e != nil {
		t.Fatal(e)
	}
	if e = s.Admit(w.RequestID, w.Epoch, b); e != nil {
		t.Fatal(e)
	}
	if e = s.AppendText(w.RequestID, w.Epoch, "Original stage saved output"); e != nil {
		t.Fatal(e)
	}
	w, _ = s.Get(w.RequestID)
	return w, b
}
func groupRecoveryFixture(t *testing.T, stage int, configure ...func(*Store, *CollaborationInput)) (*Store, *executors.Registry, Collaboration, Work, CoreBinding) {
	t.Helper()
	s, r, g := configuredRecoveryGroup(t, configure...)
	var w Work
	var b CoreBinding
	for i := 1; i <= stage; i++ {
		g, w = advanceGroup(t, s, g.ID)
		w, b = admittedRecoveryStage(t, s, w)
		if i < stage {
			if e := s.Finish(w.RequestID, w.Epoch, "completed", ""); e != nil {
				t.Fatal(e)
			}
			serviceStopped(t, s, w, b)
		}
	}
	p := r.Snapshot()
	if e := r.MarkUnavailable(p.Pin, p.BootID, "host_disconnected"); e != nil {
		t.Fatal(e)
	}
	if e := s.ReconcileExecutors(); e != nil {
		t.Fatal(e)
	}
	w = serviceStopped(t, s, w, b)
	if e := s.AdvanceCollaborations(); e != nil {
		t.Fatal(e)
	}
	g = groupState(t, s, g.ID)
	if g.Status != "suspended" || g.Reason != "task_suspended" {
		t.Fatal(g)
	}
	p = r.Snapshot()
	if e := r.MarkReady(p.Pin, p.BootID); e != nil {
		t.Fatal(e)
	}
	return s, r, g, w, b
}
func groupRecoverInput(g Collaboration, w Work, nonce string) CollaborationRecoveryInput {
	return CollaborationRecoveryInput{Version: 2, RequestID: w.RequestID, ExpectedRevision: g.Revision, ExpectedEpoch: w.Epoch, ExpectedTaskRevision: w.TaskRevision, ClientNonce: nonce}
}
func TestGroupServiceRecoveryPreservesEveryOriginalStageAndBudget(t *testing.T) {
	for stage := 1; stage <= 3; stage++ {
		t.Run(string(rune('0'+stage)), func(t *testing.T) {
			s, _, g, w, _ := groupRecoveryFixture(t, stage)
			before := s.Snapshot().Sequence
			in := groupRecoverInput(g, w, "group-recover-original-stage")
			out, dup, e := s.RecoverCollaboration(g.ID, in)
			if e != nil || dup {
				t.Fatal(e)
			}
			if s.Snapshot().Sequence != before+1 || out.Collaboration.Status != "running" || out.Work.Status != "waiting_host" {
				t.Fatal("not atomic", out)
			}
			same := out.Collaboration
			same.Status, same.Reason, same.Revision = g.Status, g.Reason, g.Revision
			if !reflect.DeepEqual(same, g) {
				t.Fatal("group identity/budget/pins drift")
			}
			work := out.Work
			work.Status, work.DesiredState, work.Failure = w.Status, w.DesiredState, w.Failure
			work.Epoch, work.TaskRevision = w.Epoch, w.TaskRevision
			if !reflect.DeepEqual(work, w) {
				t.Fatal("stage context/nonce/source/result/effects mutated")
			}
			again, dup, e := s.RecoverCollaboration(g.ID, in)
			if e != nil || !dup || !reflect.DeepEqual(out, again) || s.Snapshot().Sequence != before+1 {
				t.Fatal("lost ACK", e)
			}
			receipt, e := s.CommandReceipt("collaboration_recover", in.ClientNonce, g.ID)
			if e != nil || !receipt.Found || receipt.Work.Context != nil || receipt.Work.RuntimeContext != nil || !reflect.DeepEqual(receipt.Input, in) {
				t.Fatal("receipt", e)
			}
			claimed, e := s.Claim(w.RequestID, out.Work.Epoch)
			if e != nil {
				t.Fatal(e)
			}
			if claimed.AttemptID == w.AttemptID || claimed.CurrentAttemptLease.ID == w.CurrentAttemptLease.ID || claimed.CurrentAttemptLease.Fence <= w.CurrentAttemptLease.Fence || !reflect.DeepEqual(claimed.Core, w.Core) || !reflect.DeepEqual(claimed.Context, w.Context) || claimed.RuntimeContextSHA256 != w.RuntimeContextSHA256 {
				t.Fatal("not exact context/new lease")
			}
			if e = s.AppendText(w.RequestID, w.Epoch, "late prior output"); !errors.Is(e, ErrConflict) {
				t.Fatal("old epoch published", e)
			}
			if e = s.AdvanceCollaborations(); e != nil {
				t.Fatal(e)
			}
			if len(groupState(t, s, g.ID).RequestIDs) != stage {
				t.Fatal("new hidden stage")
			}
			dir := s.dir
			s.Close()
			for i := 0; i < 3; i++ {
				re, e := Open(dir)
				if e != nil {
					t.Fatal(e)
				}
				got, _ := re.Get(w.RequestID)
				if !reflect.DeepEqual(got, claimed) {
					t.Fatal("stage replay drift")
				}
				if !reflect.DeepEqual(groupState(t, re, g.ID), out.Collaboration) {
					t.Fatal("group replay drift")
				}
				rpt, e := re.CommandReceipt("collaboration_recover", in.ClientNonce, g.ID)
				if e != nil || !rpt.Found {
					t.Fatal("receipt replay", e)
				}
				re.Close()
			}
		})
	}
}
func TestGroupServiceRecoveryRejectsUnsafeAndStaleIntentsWithoutRecoveryIntent(t *testing.T) {
	cases := []string{"missing-stop", "generation", "context0", "wrong-binding", "cancel", "pause", "started-effect", "unknown-effect", "expired", "backwards", "wrong-stage", "wrong-group", "epoch", "revision", "task-revision", "source-revoke", "audience-revoke", "plugin-revoke", "install-revoke", "prior-stop", "prior-effect", "group-authority-changed"}
	for _, kind := range cases {
		t.Run(kind, func(t *testing.T) {
			stage := 1
			if kind == "prior-stop" || kind == "prior-effect" {
				stage = 2
			}
			s, r, g, w, _ := groupRecoveryFixture(t, stage)
			in := groupRecoverInput(g, w, "group-negative-"+kind)
			switch kind {
			case "missing-stop":
				x := s.work[w.RequestID]
				x.RuntimeStopped = false
				x.RuntimeStopReceipt = nil
				s.work[w.RequestID] = x
			case "generation":
				x := s.work[w.RequestID]
				x.RuntimeAttemptBinding.NativeProcessGeneration = ""
				s.work[w.RequestID] = x
			case "context0":
				x := s.work[w.RequestID]
				x.ContextProtocolVersion = 0
				s.work[w.RequestID] = x
			case "wrong-binding":
				x := s.work[w.RequestID]
				x.Core.NativeProcessID++
				s.work[w.RequestID] = x
			case "cancel":
				_, _, e := s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: g.Revision, ClientNonce: "group-negative-cancel-intent"})
				if e != nil {
					t.Fatal(e)
				}
			case "pause":
				x := s.work[w.RequestID]
				x.Status, x.DesiredState = "paused", "paused"
				s.work[w.RequestID] = x
			case "started-effect", "unknown-effect":
				x := s.work[w.RequestID]
				x.Operations = []Operation{{Key: "fixture-uncertain-effect", Status: strings.TrimSuffix(kind, "-effect")}}
				s.work[w.RequestID] = x
			case "expired":
				deadline, _ := time.Parse(time.RFC3339Nano, g.DeadlineAt)
				s.now = func() time.Time { return deadline }
			case "backwards":
				created, _ := time.Parse(time.RFC3339Nano, g.CreatedAt)
				s.now = func() time.Time { return created.Add(-time.Nanosecond) }
			case "wrong-stage":
				x := s.work[w.RequestID]
				x.Collaboration.Stage++
				s.work[w.RequestID] = x
			case "wrong-group":
				x := s.work[w.RequestID]
				x.Collaboration.ID = "collab-" + strings.Repeat("f", 32)
				s.work[w.RequestID] = x
			case "epoch":
				in.ExpectedEpoch++
			case "revision":
				in.ExpectedRevision++
			case "task-revision":
				in.ExpectedTaskRevision++
			case "source-revoke":
				_, _, e := s.SetSourceAccess(AccessInput{RequestID: w.RequestID, ExpectedRevision: 0, Granted: false, ClientNonce: "group-source-revoke"})
				if e != nil {
					t.Fatal(e)
				}
			case "audience-revoke":
				c, _ := s.channel(g.ChannelID)
				_, _, e := s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, BotIDs: []string{"bot-meiiie"}, ExpectedRevision: c.Revision, ClientNonce: "group-channel-revoke"})
				if e != nil {
					t.Fatal(e)
				}
			case "plugin-revoke":
				p, ok := s.plugin("neko-artifacts")
				if !ok {
					t.Fatal("plugin fixture name")
				}
				_, _, e := s.ControlPlugin(PluginInput{Version: 1, ID: p.ID, Action: "disable", ExpectedRevision: p.Revision, ClientNonce: "group-plugin-revoke"})
				if e != nil {
					t.Fatal(e)
				}
			case "install-revoke":
				p := r.Snapshot()
				if _, _, e := r.Revoke(p.Pin, "group-install-revoke"); e != nil {
					t.Fatal(e)
				}
			case "prior-stop":
				x := s.work[g.RequestIDs[0]]
				x.RuntimeStopped = false
				x.RuntimeStopReceipt = nil
				s.work[x.RequestID] = x
			case "prior-effect":
				x := s.work[g.RequestIDs[0]]
				x.Operations = []Operation{{Key: "fixture-prior-unknown", Status: "unknown"}}
				s.work[x.RequestID] = x
			case "group-authority-changed":
				g.Reason = "authority_changed"
				s.collaborations[g.ID] = g
			}
			before := s.Snapshot().Sequence
			if _, _, e := s.RecoverCollaboration(g.ID, in); !errors.Is(e, ErrConflict) {
				t.Fatal("unsafe recovery accepted", e)
			}
			if kind == "expired" || kind == "backwards" {
				closed := groupState(t, s, g.ID)
				got, _ := s.Get(w.RequestID)
				if s.Snapshot().Sequence != before+1 || s.events[len(s.events)-1].Type != "collaboration.budget_closed" || closed.BudgetClosedAt == "" || closed.Revision != g.Revision+1 || !reflect.DeepEqual(got, w) {
					t.Fatal("expiry denial must write only irreversible observation, never recovery/work mutation")
				}
			} else if s.Snapshot().Sequence != before {
				t.Fatal("rejected non-time intent changed journal")
			}
		})
	}
}
func TestGroupServiceRecoveryDeadlineFencesClaimPromptOutputEffectsAfterIntent(t *testing.T) {
	s, _, g, w, _ := groupRecoveryFixture(t, 1)
	out, _, e := s.RecoverCollaboration(g.ID, groupRecoverInput(g, w, "group-deadline-recover"))
	if e != nil {
		t.Fatal(e)
	}
	deadline, _ := time.Parse(time.RFC3339Nano, g.DeadlineAt)
	s.now = func() time.Time { return deadline }
	if _, e = s.Claim(w.RequestID, out.Work.Epoch); !errors.Is(e, ErrConflict) {
		t.Fatal("expired group claimed", e)
	}
	if s.RuntimeAllowed(w.RequestID, out.Work.Epoch) {
		t.Fatal("expired prompt allowed")
	}
	if e = s.AdvanceCollaborations(); e != nil {
		t.Fatal(e)
	}
	closed := groupState(t, s, g.ID)
	if closed.Status != "needs_owner" || closed.Reason != "stage_budget_expired" || closed.DeadlineAt != g.DeadlineAt {
		t.Fatal(closed)
	}
}
func TestGroupServiceRecoveryRacesCancelAndNonceIsolation(t *testing.T) {
	for i := 0; i < 12; i++ {
		s, _, g, w, _ := groupRecoveryFixture(t, 1)
		in := groupRecoverInput(g, w, "group-race-recover")
		var wg sync.WaitGroup
		wg.Add(2)
		var recoverErr, cancelErr error
		go func() { defer wg.Done(); _, _, recoverErr = s.RecoverCollaboration(g.ID, in) }()
		go func() {
			defer wg.Done()
			_, _, cancelErr = s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: g.Revision, ClientNonce: "group-race-cancel"})
		}()
		wg.Wait()
		if (recoverErr == nil) == (cancelErr == nil) {
			t.Fatal("CAS admits both or neither", recoverErr, cancelErr)
		}
	}
	s, _, g, w, _ := groupRecoveryFixture(t, 1)
	in := groupRecoverInput(g, w, "group-same-nonce")
	if _, _, e := s.RecoverCollaboration(g.ID, in); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: g.Revision, ClientNonce: in.ClientNonce}); !errors.Is(e, ErrConflict) {
		t.Fatal("recover nonce became cancel", e)
	}
	other, _, e := s.Accept(input("group-independent-task", "foreground"))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.ControlTask(controlInput(other, "pause", in.ClientNonce)); !errors.Is(e, ErrConflict) {
		t.Fatal("group nonce became task command", e)
	}
	if _, e = s.CommandReceipt("collaboration_cancel", in.ClientNonce, g.ID); !errors.Is(e, ErrConflict) {
		t.Fatal("receipt kind confusion", e)
	}
	if _, e = s.CommandReceipt("collaboration_recover", in.ClientNonce, "collab-"+strings.Repeat("f", 32)); !errors.Is(e, ErrConflict) {
		t.Fatal("receipt group confusion", e)
	}
	in.ExpectedRevision++
	if _, _, e = s.RecoverCollaboration(g.ID, in); !errors.Is(e, ErrConflict) {
		t.Fatal("nonce changed payload", e)
	}
}
func TestGroupServiceRecoveryRejectsMixedOrMutatedReplay(t *testing.T) {
	s, _, g, w, _ := groupRecoveryFixture(t, 1)
	in := groupRecoverInput(g, w, "group-replay-recover")
	if _, _, e := s.RecoverCollaboration(g.ID, in); e != nil {
		t.Fatal(e)
	}
	e := s.events[len(s.events)-1]
	s.collaborations[g.ID] = g
	s.work[w.RequestID] = w
	delete(s.collaborationActions, in.ClientNonce)
	if !validReplay(s, e) {
		t.Fatal("positive replay")
	}
	for _, kind := range []string{"deadline", "new-stage", "new-context", "nonce", "task-control", "missing-input", "time-expired", "claim-version"} {
		t.Run(kind, func(t *testing.T) {
			bad := e
			group := copyCollaboration(*e.Collaboration)
			bad.Collaboration = &group
			bad.Work = copyWork(e.Work)
			c := *e.CollaborationRecovery
			bad.CollaborationRecovery = &c
			switch kind {
			case "deadline":
				group.DeadlineAt = time.Now().Add(time.Hour).Format(time.RFC3339Nano)
			case "new-stage":
				group.RequestIDs = append(group.RequestIDs, "req-"+strings.Repeat("f", 32))
			case "new-context":
				bad.Work.RuntimeContextSHA256 = strings.Repeat("f", 64)
			case "nonce":
				bad.Nonce = "another-owner-nonce"
			case "task-control":
				bad.TaskControl = &TaskControlEvent{}
			case "missing-input":
				bad.CollaborationRecovery = nil
			case "time-expired":
				c.IssuedAt = g.DeadlineAt
			case "claim-version":
				bad.ExecutorClaimVersion = 1
			}
			if validReplay(s, bad) {
				t.Fatal("forged recovery replay accepted")
			}
		})
	}
}

func TestGroupServiceRecoveryRunningDeadlineDeniesLateOutputEffectAndCompletion(t *testing.T) {
	s, _, g := configuredRecoveryGroup(t)
	var w Work
	var b CoreBinding
	for stage := 1; stage <= 3; stage++ {
		_, w = advanceGroup(t, s, g.ID)
		w, b = admittedRecoveryStage(t, s, w)
		if stage < 3 {
			if e := s.Finish(w.RequestID, w.Epoch, "completed", ""); e != nil {
				t.Fatal(e)
			}
			serviceStopped(t, s, w, b)
		}
	}
	deadline, _ := time.Parse(time.RFC3339Nano, g.DeadlineAt)
	s.now = func() time.Time { return deadline }
	before := s.Snapshot().Sequence
	if s.RuntimeAllowed(w.RequestID, w.Epoch) {
		t.Fatal("expired stage prompt allowed")
	}
	if e := s.AppendText(w.RequestID, w.Epoch, "late"); !errors.Is(e, ErrConflict) {
		t.Fatal("expired output", e)
	}
	var frame map[string]json.RawMessage
	json.Unmarshal(invocation("late artifact"), &frame)
	var meta map[string]json.RawMessage
	json.Unmarshal(frame["_meta"], &meta)
	meta["neko.contextAdmission"], _ = json.Marshal(w.Core.ContextAdmission)
	frame["_meta"], _ = json.Marshal(meta)
	raw, _ := json.Marshal(frame)
	if _, e := s.ExecuteTool(w.RequestID, w.Epoch, raw); !errors.Is(e, ErrConflict) {
		t.Fatal("expired effect", e)
	}
	if e := s.Finish(w.RequestID, w.Epoch, "completed", ""); !errors.Is(e, ErrConflict) {
		t.Fatal("expired completion", e)
	}
	got, _ := s.Get(w.RequestID)
	if s.Snapshot().Sequence != before+1 || s.events[len(s.events)-1].Type != "collaboration.budget_closed" || !reflect.DeepEqual(got, w) {
		t.Fatal("expired stage must record only observed budget closure, never output/effect/completion")
	}
}
func TestGroupServiceRecoveryOtherCommandNonceCannotBecomeRecovery(t *testing.T) {
	s, _, g, w, _ := groupRecoveryFixture(t, 1)
	other, _, e := s.Accept(input("group-control-nonce-source", "foreground"))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.ControlTask(controlInput(other, "pause", "group-task-first-nonce")); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.RecoverCollaboration(g.ID, groupRecoverInput(g, w, "group-task-first-nonce")); !errors.Is(e, ErrConflict) {
		t.Fatal("task nonce became recover", e)
	}
	if _, _, e = s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: g.Revision, ClientNonce: "group-cancel-first-nonce"}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.RecoverCollaboration(g.ID, groupRecoverInput(g, w, "group-cancel-first-nonce")); !errors.Is(e, ErrConflict) {
		t.Fatal("cancel nonce became recover", e)
	}
}
func TestGroupServiceRecoveryRetainsProofAcrossCancelAndReplay(t *testing.T) {
	s, _, g, w, _ := groupRecoveryFixture(t, 1)
	in := groupRecoverInput(g, w, "group-recover-then-cancel")
	out, _, e := s.RecoverCollaboration(g.ID, in)
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: out.Collaboration.Revision, ClientNonce: "group-cancel-after-recover"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.AdvanceCollaborations(); e != nil {
		t.Fatal(e)
	}
	got, _ := s.Get(w.RequestID)
	if got.Status != "cancelled" || !reflect.DeepEqual(got.RuntimeStopReceipt, w.RuntimeStopReceipt) || !reflect.DeepEqual(got.Core, w.Core) {
		t.Fatal("cancel lost proof")
	}
	again, dup, e := s.RecoverCollaboration(g.ID, in)
	if e != nil || !dup || again.Collaboration.Status != "cancelled" || again.Work.DesiredState != "cancelled" {
		t.Fatal("retry revived group", e)
	}
	dir := s.dir
	s.Close()
	for i := 0; i < 3; i++ {
		re, e := Open(dir)
		if e != nil {
			t.Fatal(e)
		}
		got, _ = re.Get(w.RequestID)
		if got.Status != "cancelled" || !reflect.DeepEqual(got.RuntimeStopReceipt, w.RuntimeStopReceipt) {
			t.Fatal("cancel replay")
		}
		re.Close()
	}
}

func TestGroupServiceRecoveryOriginalAccountProjectFileAndReaderRights(t *testing.T) {
	for _, kind := range []string{"positive", "account-removed", "account-rotated", "project-revised", "file-revoked", "file-regranted", "reader-revoked", "reader-reenabled", "account-expired"} {
		t.Run(kind, func(t *testing.T) {
			var catalog *connections.Store
			var account connections.Account
			var accountInput connections.AccountInput
			var project Project
			var file UploadedFile
			s, _, g, w, _ := groupRecoveryFixture(t, 2, func(s *Store, in *CollaborationInput) {
				var e error
				catalog, e = connections.Open(filepath.Join(t.TempDir(), "accounts"), authorityVault{})
				if e != nil {
					t.Fatal(e)
				}
				accountInput = connections.AccountInput{Name: "Fixture account", Provider: "openai-compatible", Endpoint: "https://example.invalid/v1", Model: "fixture-model", Secret: "fixture-only-account-canary", ClientNonce: "group-account-create"}
				account, _, e = catalog.SaveAccount(accountInput)
				if e != nil {
					t.Fatal(e)
				}
				s.EnableAccounts(catalog)
				in.AccountID, in.AccountRevision = account.ID, account.Revision
				project, _, e = s.SaveProject(ProjectInput{Name: "Pinned group project", Goal: "Original project goal", ChannelIDs: []string{in.ChannelID}, ClientNonce: "group-project-create"})
				if e != nil {
					t.Fatal(e)
				}
				file, _, e = s.UploadFile(UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: in.ChannelID, Filename: "source.md", Content: []byte("Original source span"), ClientNonce: "group-file-upload"})
				if e != nil {
					t.Fatal(e)
				}
				in.ProjectID, in.ProjectRevision, in.FileIDs = project.ID, project.Revision, []string{file.ID}
			})
			switch kind {
			case "account-removed":
				if e := catalog.RemoveAccount(account.ID, account.Revision); e != nil {
					t.Fatal(e)
				}
			case "account-rotated":
				accountInput.ID, accountInput.ExpectedRevision, accountInput.ClientNonce = account.ID, account.Revision, "group-account-rotate"
				accountInput.Secret = "fixture-only-rotated-account-canary"
				if _, _, e := catalog.SaveAccount(accountInput); e != nil {
					t.Fatal(e)
				}
			case "project-revised":
				if _, _, e := s.SaveProject(ProjectInput{ID: project.ID, ExpectedRevision: project.Revision, Name: project.Name, Goal: "Changed project goal", ChannelIDs: project.ChannelIDs, ClientNonce: "group-project-revise"}); e != nil {
					t.Fatal(e)
				}
			case "file-revoked", "file-regranted":
				if e := s.SetFileAccess(FileAccessInput{FileID: file.ID, ExpectedRevision: file.AccessRevision, Granted: false, ClientNonce: "group-file-revoke"}); e != nil {
					t.Fatal(e)
				}
				if kind == "file-regranted" {
					if e := s.SetFileAccess(FileAccessInput{FileID: file.ID, ExpectedRevision: file.AccessRevision + 1, Granted: true, ClientNonce: "group-file-regrant"}); e != nil {
						t.Fatal(e)
					}
				}
			case "reader-revoked", "reader-reenabled":
				reader, _ := s.plugin("neko-project-sources")
				reader = pluginChange(t, s, reader.ID, "disable", "group-reader-disable", reader.Revision)
				if kind == "reader-reenabled" {
					pluginChange(t, s, reader.ID, "enable", "group-reader-reenable", reader.Revision)
				}
			case "account-expired":
				x := s.work[w.RequestID]
				x.AccountExpiredAt = time.Now().UTC().Format(time.RFC3339Nano)
				s.work[w.RequestID] = x
			}
			before := s.Snapshot().Sequence
			out, _, e := s.RecoverCollaboration(g.ID, groupRecoverInput(g, w, "group-resource-recover"))
			if kind == "positive" {
				if e != nil || !reflect.DeepEqual(out.Work.Resources, w.Resources) || !reflect.DeepEqual(out.Work.Account, w.Account) {
					t.Fatal("original scoped pins", e)
				}
				receipt, e := s.CommandReceipt("collaboration_recover", "group-resource-recover", g.ID)
				if e != nil || receipt.Work.Resources.Files[0].Excerpt != "" || receipt.Collaboration.Resources.Files[0].Excerpt != "" || receipt.Work.Context != nil {
					t.Fatal("private receipt payload", e)
				}
			} else if !errors.Is(e, ErrConflict) || s.Snapshot().Sequence != before {
				t.Fatal("revoked original authority resumed", e)
			}
		})
	}
}
func TestGroupServiceRecoveryLateStopNoAutoCancelAndExplicitCancelWaitsProof(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint(cancel), func(t *testing.T) {
			s, r, g := configuredRecoveryGroup(t)
			_, w := advanceGroup(t, s, g.ID)
			w, b := admittedRecoveryStage(t, s, w)
			p := r.Snapshot()
			if e := r.MarkUnavailable(p.Pin, p.BootID, "host_disconnected"); e != nil {
				t.Fatal(e)
			}
			if e := s.ReconcileExecutors(); e != nil {
				t.Fatal(e)
			}
			if e := s.AdvanceCollaborations(); e != nil {
				t.Fatal(e)
			}
			g = groupState(t, s, g.ID)
			for i := 0; i < 4; i++ {
				if e := s.AdvanceCollaborations(); e != nil {
					t.Fatal(e)
				}
			}
			pending, _ := s.Get(w.RequestID)
			if pending.Status != "suspending" || pending.DesiredState != "suspended" || pending.RuntimeStopped {
				t.Fatal("manager turned late-proof suspension into cancel", pending.Status, pending.DesiredState)
			}
			if _, _, e := s.RecoverCollaboration(g.ID, groupRecoverInput(g, pending, "group-recover-before-proof")); !errors.Is(e, ErrConflict) {
				t.Fatal("missing late proof recovered", e)
			}
			if cancel {
				if _, _, e := s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: g.Revision, ClientNonce: "group-cancel-before-late-proof"}); e != nil {
					t.Fatal(e)
				}
				if e := s.AdvanceCollaborations(); e != nil {
					t.Fatal(e)
				}
				if groupState(t, s, g.ID).Status != "cancelling" {
					t.Fatal("cancel fabricated stopped fact")
				}
			}
			later := serviceStopped(t, s, w, b)
			if e := s.AdvanceCollaborations(); e != nil {
				t.Fatal(e)
			}
			if cancel {
				later, _ = s.Get(w.RequestID)
				if later.Status != "cancelled" || later.DesiredState != "cancelled" || !stoppedTask(later) || groupState(t, s, g.ID).Status != "cancelled" {
					t.Fatal("late valid proof cannot settle explicit cancellation", later.Status, groupState(t, s, g.ID).Status)
				}
			} else {
				if later.Status != "suspended" || groupState(t, s, g.ID).Status != "suspended" {
					t.Fatal("late stop auto resumed")
				}
				p = r.Snapshot()
				r.MarkReady(p.Pin, p.BootID)
				if _, _, e := s.RecoverCollaboration(g.ID, groupRecoverInput(g, later, "group-recover-after-late-proof")); e != nil {
					t.Fatal(e)
				}
			}
			dir := s.dir
			s.Close()
			for i := 0; i < 3; i++ {
				re, e := Open(dir)
				if e != nil {
					t.Fatal(e)
				}
				got, _ := re.Get(w.RequestID)
				if !reflect.DeepEqual(got.RuntimeStopReceipt, later.RuntimeStopReceipt) || !reflect.DeepEqual(got.Core, later.Core) {
					t.Fatal("late native proof replay drift")
				}
				re.Close()
			}
		})
	}
}
func TestGroupServiceRecoveryHistoricalCrossKindNonceReceiptsRemainIdempotent(t *testing.T) {
	s, g, _, dir := collaborationFixture(t)
	w, _, e := s.Accept(input("legacy-group-task-input", "foreground"))
	if e != nil {
		t.Fatal(e)
	}
	in := controlInput(w, "pause", "legacy-shared-command-nonce")
	if _, _, e = s.ControlTask(in); e != nil {
		t.Fatal(e)
	}
	// This accepted baseline wire history used separate command nonce indexes.
	old := copyCollaboration(g)
	old.Revision++
	old.Status, old.Reason = "cancelling", "owner_cancelled"
	event := Event{Type: "collaboration.cancelling", Collaboration: &old, Nonce: in.ClientNonce}
	if !validReplay(s, event) {
		t.Fatal("baseline wire event unsupported")
	}
	if e = s.commit(event); e != nil {
		t.Fatal(e)
	}
	if e = s.AdvanceCollaborations(); e != nil {
		t.Fatal(e)
	}
	before := s.Snapshot().Sequence
	if _, dup, e := s.ControlTask(in); e != nil || !dup {
		t.Fatal("old task retry lost its committed receipt", e)
	}
	if _, dup, e := s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: g.Revision, ClientNonce: in.ClientNonce}); e != nil || !dup {
		t.Fatal("old group retry lost its committed receipt", e)
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("legacy receipt retry mutated intent")
	}
	s.Close()
	re, e := Open(dir)
	if e != nil {
		t.Fatal("legacy journal poisoned", e)
	}
	defer re.Close()
	if receipt, e := re.CommandReceipt("task_control", in.ClientNonce, ""); e != nil || !receipt.Found {
		t.Fatal(e)
	}
	if receipt, e := re.CommandReceipt("collaboration_cancel", in.ClientNonce, g.ID); e != nil || !receipt.Found {
		t.Fatal(e)
	}
}

func TestGroupServiceRecoveryReadyLossPreservesQueuedOriginalObligation(t *testing.T) {
	s, r, g, w, _ := groupRecoveryFixture(t, 1)
	p := r.Snapshot()
	if e := r.MarkUnavailable(p.Pin, p.BootID, "host_disconnected"); e != nil {
		t.Fatal(e)
	}
	out, _, e := s.RecoverCollaboration(g.ID, groupRecoverInput(g, w, "group-unavailable-recover"))
	if e != nil {
		t.Fatal("same-pin availability erased intent", e)
	}
	sequence := s.Snapshot().Sequence
	if _, e = s.Claim(w.RequestID, out.Work.Epoch); !errors.Is(e, ErrConflict) || s.Snapshot().Sequence != sequence {
		t.Fatal("unready dispatch", e)
	}
	got, _ := s.Get(w.RequestID)
	if got.Status != "waiting_host" || !reflect.DeepEqual(got.Core, w.Core) || !reflect.DeepEqual(got.ExecutorPin, w.ExecutorPin) || len(groupState(t, s, g.ID).RequestIDs) != 1 {
		t.Fatal("queued obligation changed")
	}
	p = r.Snapshot()
	if e = r.MarkReady(p.Pin, p.BootID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Claim(w.RequestID, out.Work.Epoch); e != nil {
		t.Fatal("same-pin readiness cannot resume", e)
	}
}
