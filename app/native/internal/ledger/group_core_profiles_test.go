package ledger

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/executors"
)

func groupProfileObject(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("typed object did not serialize")
	}
	return out
}

func groupProfileDecode(t *testing.T, value any, target any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil || json.Unmarshal(raw, target) != nil {
		t.Fatal("fixture schema", err)
	}
}

func groupProfileSetup(t *testing.T) (*Store, *profileServiceFixture, *executors.Registry, Channel, CoreProfileConsent, CoreProfileConsentInput, CollaborationInput) {
	t.Helper()
	s, f, c, _, ordinary, consent := profileFixture(t)
	r, err := executors.Open(filepath.Join(t.TempDir(), "registry"), executors.Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: f.source.RuntimeSHA256, RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	state := r.Snapshot()
	if err = r.MarkReady(state.Pin, state.BootID); err != nil {
		t.Fatal(err)
	}
	s.ConfigureExecutor(r)
	s.ConfigureContextAdmission(true)
	intent := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-davin", "bot-meiiie"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare the two bounded proposals and return the final conclusion", MaxStages: 3, ClientNonce: "group-profile-original-intent"}
	body := groupProfileObject(t, consent)
	body["botIds"] = []string{"bot-davin", "bot-meiiie"}
	body["maxInputBytes"], body["maxOutputTokens"], body["maxRequests"], body["maxSteps"], body["maxSeconds"], body["ttlSeconds"] = 64000, 4096, 6, 6, 300, 600
	body["clientNonce"] = "group-profile-consent-intent"
	body["purpose"] = "finite_collaboration_v1"
	group := groupProfileObject(t, intent)
	group["version"] = 1
	body["groupIntent"] = group
	groupProfileDecode(t, body, &consent)
	return s, f, r, c, ordinary, consent, intent
}

func groupProfileApprove(t *testing.T, s *Store, input CoreProfileConsentInput) CoreProfileConsent {
	t.Helper()
	c, duplicate, err := s.ApproveCoreProfile(input)
	if err != nil || duplicate {
		t.Fatal("group execution consent", err)
	}
	return c
}

func groupProfileSelect(t *testing.T, intent CollaborationInput, consent CoreProfileConsent) CollaborationInput {
	t.Helper()
	body := groupProfileObject(t, intent)
	body["coreProfileSelection"] = CoreProfileSelection{ConsentID: consent.ID, ExpectedConsentRevision: consent.Revision}
	var selected CollaborationInput
	groupProfileDecode(t, body, &selected)
	return selected
}

func TestGroupCoreProfileExplicitPurposeConsentAndReceipt(t *testing.T) {
	s, _, _, _, _, input, _ := groupProfileSetup(t)
	c := groupProfileApprove(t, s, input)
	public := groupProfileObject(t, c)
	if c.Version != 2 || public["purpose"] != "finite_collaboration_v1" || public["groupIntent"] == nil || public["groupIntentSha256"] == nil {
		t.Fatal("explicit group purpose/intent was discarded")
	}
	r, err := s.CoreProfileReceipt("group_execution", input.ClientNonce)
	if err != nil || !r.Found || !r.Active || r.Consent == nil || r.Consent.Version != 2 || !reflect.DeepEqual(r.Input, input) {
		t.Fatal("group receipt did not preserve exact explicit intent", err)
	}
	if _, err = s.CoreProfileReceipt("execution", input.ClientNonce); !errors.Is(err, ErrConflict) {
		t.Fatal("ordinary receipt accepted group purpose", err)
	}
}

func TestGroupCoreProfileOrdinaryConsentCannotAuthorizeGroup(t *testing.T) {
	s, _, _, _, ordinary, _, intent := groupProfileSetup(t)
	before := s.Snapshot().Sequence
	if _, _, err := s.CreateCollaboration(groupProfileSelect(t, intent, ordinary)); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("ordinary one-bot consent was widened into a group", err)
	}
}

func TestGroupCoreProfileGroupConsentCannotAuthorizeOrdinaryWork(t *testing.T) {
	s, _, _, c, _, input, _ := groupProfileSetup(t)
	consent := groupProfileApprove(t, s, input)
	rev := c.Revision
	before := s.Snapshot().Sequence
	if _, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "group-profile-wrong-purpose", Text: "An unrelated ordinary request", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &rev, CoreProfileSelection: &CoreProfileSelection{consent.ID, consent.Revision}}); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("group-purpose consent authorized an ordinary request", err)
	}
}

func TestGroupCoreProfileOriginalIntentNonceAndStagePins(t *testing.T) {
	s, _, _, _, _, input, intent := groupProfileSetup(t)
	consent := groupProfileApprove(t, s, input)
	selected := groupProfileSelect(t, intent, consent)
	changed := selected
	changed.Goal = "A different original goal"
	before := s.Snapshot().Sequence
	if _, _, err := s.CreateCollaboration(changed); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("consent was rebound to changed goal", err)
	}
	g, duplicate, err := s.CreateCollaboration(selected)
	if err != nil || duplicate || g.Version != 2 || groupProfileObject(t, g)["coreProfileGroup"] == nil {
		t.Fatal("explicit profile group did not pin the original grant", err)
	}
	repeated, duplicate, err := s.CreateCollaboration(selected)
	if err != nil || !duplicate || repeated.ID != g.ID {
		t.Fatal("original nonce created another group", err)
	}
	g, w := advanceGroup(t, s, g.ID)
	if w.CoreProfilePin == nil || w.CoreProfileSelection == nil || w.CoreProfilePin.ConsentID != consent.ID || w.CoreProfilePin.BotID != stageBot(g, 1) || w.CoreProfilePin.ClientNonce != stageNonce(g, 1) || w.CoreProfilePin.ContextSHA256 != contextHash(w) || groupProfileObject(t, w)["coreProfileGroupBinding"] == nil {
		t.Fatal("stage admission lacked exact group/profile/context binding")
	}
	if w.CoreProfilePin.Limits.MaxRequests != 1 || w.CoreProfilePin.Limits.MaxSteps != 1 || w.CoreProfilePin.Limits.MaxOutputTokens != 682 {
		t.Fatal("stage one reset the entire group budget")
	}
}

func TestGroupCoreProfileBudgetAllocationAndReplay(t *testing.T) {
	s, _, _, _, _, input, intent := groupProfileSetup(t)
	consent := groupProfileApprove(t, s, input)
	g, _, err := s.CreateCollaboration(groupProfileSelect(t, intent, consent))
	if err != nil {
		t.Fatal(err)
	}
	group, ok := groupProfileObject(t, g)["coreProfileGroup"].(map[string]any)
	if !ok {
		t.Fatal("group has no immutable allocation plan")
	}
	plan := group["plan"].(map[string]any)
	initial := plan["initialReserved"].(map[string]any)
	pool := plan["recoveryPool"].(map[string]any)
	if initial["requests"] != float64(4) || initial["steps"] != float64(4) || initial["outputTokens"] != float64(2728) || pool["requests"] != float64(2) || pool["steps"] != float64(2) || pool["outputTokens"] != float64(1364) {
		t.Fatal("original total budget was not split conservatively")
	}
	dir := s.dir
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("new typed consent/group did not replay", err)
	}
	defer reopened.Close()
	got := groupState(t, reopened, g.ID)
	if !reflect.DeepEqual(got, g) {
		t.Fatal("restart repinned or reset the group plan")
	}
}

func TestGroupCoreProfileStageCandidateComposition(t *testing.T) {
	s, _, _, _, _, input, intent := groupProfileSetup(t)
	consent := groupProfileApprove(t, s, input)
	g, _, err := s.CreateCollaboration(groupProfileSelect(t, intent, consent))
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w := Work{RequestID: "req-" + strings.Repeat("c", 32), SourceMessageID: "msg-" + strings.Repeat("d", 32), OwnerUserID: g.OwnerUserID, SpaceID: g.SpaceID, BotID: stageBot(g, 1), ConversationID: g.ChannelID, ClientNonce: stageNonce(g, 1), Text: stageText(g, 1), Lane: "background", Status: "waiting_host", DesiredState: "running", Epoch: 1, BotConfigRevision: 1, CreatedAt: s.clock().UTC().Format(time.RFC3339Nano), CreatedSequence: uint64(len(s.events) + 1), Resources: copyResources(g.Resources), Collaboration: &CollaborationBinding{ID: g.ID, Stage: 1}, Publication: &Publication{Version: 1, ActorID: g.OwnerUserID, Platform: "nekobot-local", SourceChannelID: g.ChannelID, SourceMessageID: "msg-" + strings.Repeat("d", 32), ReplyChannelID: g.ChannelID, ReplyThreadID: "thread-msg-" + strings.Repeat("d", 32), AudienceRevision: g.ExpectedChannelRevision}, ToolPolicyVersion: g.ToolPolicyVersion, PluginSelection: copySelection(g.PluginSelection), PluginPins: append([]PluginPin(nil), g.PluginPins...), ExecutorPolicyVersion: g.ExecutorPolicyVersion, ExecutorPin: g.ExecutorPin}
	s.pinBotProfile(&w)
	w.Context = s.captureContext(w)
	if err = s.pinGroupStageCoreProfile(g, &w); err != nil {
		t.Fatal("pin", err)
	}
	if err = s.attachRuntimeContext(&w); err != nil {
		t.Fatal("context", err)
	}
	checks := map[string]bool{"group": s.validGroupProfile(g), "groupWork": s.validGroupProfileWork(w, g), "collaboration": s.validCollaborationWork(w), "profile": s.validCoreProfileIntake(w), "manifest": s.validNewRuntimeContext(w), "plugins": s.validPluginWork(w), "executor": validExecutorWork(w), "launch": boundedRuntimePrompt(w), "replay": validReplay(s, Event{Sequence: w.CreatedSequence, Type: "work.accepted", Work: w})}
	for name, pass := range checks {
		if !pass {
			t.Errorf("stage composition %s failed", name)
		}
	}
}
