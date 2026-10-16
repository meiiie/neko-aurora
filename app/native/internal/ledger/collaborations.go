package ledger

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/executors"
)

type CollaborationInput struct {
	CoreProfileSelection       *CoreProfileSelection `json:"coreProfileSelection,omitempty"`
	ExpectedContinuityRevision *uint64               `json:"expectedContinuityRevision,omitempty"`
	PluginSelection            *[]PluginSelection    `json:"pluginSelection,omitempty"`
	ChannelID                  string                `json:"channelId"`
	ExpectedChannelRevision    uint64                `json:"expectedChannelRevision"`
	Participants               []string              `json:"participants"`
	CoordinatorBotID           string                `json:"coordinatorBotId"`
	Goal                       string                `json:"goal"`
	MaxStages                  int                   `json:"maxStages"`
	ClientNonce                string                `json:"clientNonce"`
	AccountID                  string                `json:"accountId,omitempty"`
	AccountRevision            uint64                `json:"accountRevision,omitempty"`
	ProjectID                  string                `json:"projectId,omitempty"`
	ProjectRevision            uint64                `json:"projectRevision,omitempty"`
	FileIDs                    []string              `json:"fileIds,omitempty"`
}

type Collaboration struct {
	CoreProfileGroup       *GroupCoreProfile       `json:"coreProfileGroup,omitempty"`
	PublishedReportContext *PublishedReportContext `json:"publishedReportContext,omitempty"`
	ExecutorPolicyVersion  int                     `json:"executorPolicyVersion,omitempty"`
	ExecutorPin            *executors.InstallPin   `json:"executorPin,omitempty"`
	ToolPolicyVersion      int                     `json:"toolPolicyVersion,omitempty"`
	PluginPins             []PluginPin             `json:"pluginPins,omitempty"`
	Version                int                     `json:"version"`
	ID                     string                  `json:"id"`
	OwnerUserID            string                  `json:"ownerUserId"`
	SpaceID                string                  `json:"spaceId"`
	Revision               uint64                  `json:"revision"`
	CreatedAt              string                  `json:"createdAt"`
	DeadlineAt             string                  `json:"deadlineAt"`
	BudgetClosedAt         string                  `json:"budgetClosedAt,omitempty"`
	BudgetClosedReason     string                  `json:"budgetClosedReason,omitempty"`
	Status                 string                  `json:"status"`
	Reason                 string                  `json:"reason,omitempty"`
	RequestIDs             []string                `json:"requestIds"`
	Account                *connections.Pin        `json:"accountPin,omitempty"`
	Resources              *ResourcePin            `json:"resources,omitempty"`
	CollaborationInput
}

type CollaborationBinding struct {
	ID    string `json:"id"`
	Stage int    `json:"stage"`
}

type CollaborationCancelInput struct {
	ExpectedRevision uint64 `json:"expectedRevision"`
	ClientNonce      string `json:"clientNonce"`
}

func copyCollaboration(g Collaboration) Collaboration {
	g.CoreProfileGroup = copyGroupCoreProfile(g.CoreProfileGroup)
	if g.CoreProfileSelection != nil {
		p := *g.CoreProfileSelection
		g.CoreProfileSelection = &p
	}
	g.PublishedReportContext = copyPublishedReportContext(g.PublishedReportContext)
	if g.ExecutorPin != nil {
		p := *g.ExecutorPin
		g.ExecutorPin = &p
	}
	g.PluginSelection = copySelection(g.PluginSelection)
	g.PluginPins = append([]PluginPin(nil), g.PluginPins...)
	if g.ExpectedContinuityRevision != nil {
		r := *g.ExpectedContinuityRevision
		g.ExpectedContinuityRevision = &r
	}
	g.Participants = append([]string{}, g.Participants...)
	g.FileIDs = append([]string(nil), g.FileIDs...)
	g.RequestIDs = append([]string{}, g.RequestIDs...)
	g.Resources = copyResources(g.Resources)
	if g.Account != nil {
		p := *g.Account
		g.Account = &p
	}
	return g
}

func normalizeCollaboration(in CollaborationInput) CollaborationInput {
	if in.CoreProfileSelection != nil {
		p := *in.CoreProfileSelection
		in.CoreProfileSelection = &p
	}
	in.PluginSelection = copySelection(in.PluginSelection)
	if in.ExpectedContinuityRevision != nil {
		r := *in.ExpectedContinuityRevision
		in.ExpectedContinuityRevision = &r
	}
	in.Participants = append([]string{}, in.Participants...)
	sort.Strings(in.Participants)
	in.FileIDs = append([]string(nil), in.FileIDs...)
	sort.Strings(in.FileIDs)
	return in
}

func (s *Store) validCollaborationInput(in CollaborationInput) bool {
	if !validChannelID(in.ChannelID) || !validAccessNonce(in.ClientNonce) || !botText(in.Goal, 2000) || strings.TrimSpace(in.Goal) == "" || in.MaxStages != 3 || len(in.Participants) != 2 || in.Participants[0] == in.Participants[1] || !sort.StringsAreSorted(in.Participants) || !contains(in.Participants, in.CoordinatorBotID) {
		return false
	}
	c, ok := s.channel(in.ChannelID)
	return ok && c.Revision == in.ExpectedChannelRevision && member(c, in.Participants[0]) && member(c, in.Participants[1])
}

func collaborationScope(in CollaborationInput) SubmissionScope {
	rev := in.ExpectedChannelRevision
	return SubmissionScope{ExpectedContinuityRevision: in.ExpectedContinuityRevision, ChannelID: in.ChannelID, ExpectedChannelRevision: &rev, AccountID: in.AccountID, AccountRevision: in.AccountRevision, ProjectID: in.ProjectID, ProjectRevision: in.ProjectRevision, FileIDs: in.FileIDs}
}

func (s *Store) CreateCollaboration(in CollaborationInput) (Collaboration, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.identity != nil {
		return Collaboration{}, false, ErrForbidden
	}
	in = normalizeCollaboration(in)
	if s.closed || s.poisoned {
		return Collaboration{}, false, ErrUnavailable
	}
	if id, ok := s.collaborationNonces[in.ClientNonce]; ok {
		g := s.collaborations[id]
		if !reflect.DeepEqual(g.CollaborationInput, in) {
			return Collaboration{}, false, ErrConflict
		}
		return copyCollaboration(g), true, nil
	}
	if s.lifecycleNonceUsed(in.ClientNonce) {
		return Collaboration{}, false, ErrConflict
	}
	if _, ok := s.taskControlNonces[in.ClientNonce]; ok {
		return Collaboration{}, false, ErrConflict
	}
	if _, ok := s.collaborationActions[in.ClientNonce]; ok {
		return Collaboration{}, false, ErrConflict
	}
	if s.archivedProject(in.ProjectID) {
		return Collaboration{}, false, ErrConflict
	}
	if !s.validCollaborationInput(in) {
		if c, ok := s.channel(in.ChannelID); ok && (c.Revision != in.ExpectedChannelRevision || (len(in.Participants) == 2 && (!member(c, in.Participants[0]) || !member(c, in.Participants[1])))) {
			return Collaboration{}, false, ErrConflict
		}
		return Collaboration{}, false, ErrInvalid
	}
	for _, id := range in.Participants {
		if s.archivedBot(id) {
			return Collaboration{}, false, ErrConflict
		}
	}
	if in.CoreProfileSelection != nil {
		if err := s.verifyGroupProfileSelection(in); err != nil {
			return Collaboration{}, false, err
		}
		if id, ok := s.collaborationNonces[in.ClientNonce]; ok {
			g := s.collaborations[id]
			if !reflect.DeepEqual(g.CollaborationInput, in) {
				return Collaboration{}, false, ErrConflict
			}
			return copyCollaboration(g), true, nil
		}
		if s.profileNonceUsed(in.ClientNonce) {
			return Collaboration{}, false, ErrConflict
		}
	}
	if len(s.collaborations) >= 128 {
		return Collaboration{}, false, ErrInvalid
	}
	id, err := newID("collab-")
	if err != nil {
		return Collaboration{}, false, err
	}
	now := s.clock()
	g := Collaboration{Version: 1, ID: id, OwnerUserID: "local-owner", SpaceID: "local-space", Revision: 1, CreatedAt: now.Format(time.RFC3339Nano), DeadlineAt: now.Add(5 * time.Minute).Format(time.RFC3339Nano), Status: "running", RequestIDs: []string{}, CollaborationInput: in}
	if err := s.pinGroupCoreProfile(&g); err != nil {
		return Collaboration{}, false, err
	}
	w := Work{BotID: in.CoordinatorBotID, OwnerUserID: g.OwnerUserID, SpaceID: g.SpaceID, ConversationID: in.ChannelID, Text: in.Goal}
	g.Resources, err = s.pinResources(w, collaborationScope(in))
	if err != nil {
		return Collaboration{}, false, err
	}
	w.Resources = g.Resources
	if err = s.pinNewExecutor(&w); err != nil {
		return Collaboration{}, false, err
	}
	g.ExecutorPolicyVersion = w.ExecutorPolicyVersion
	g.ExecutorPin = w.ExecutorPin
	s.pinBotProfile(&w)
	g.PublishedReportContext = s.captureGroupPublishedReports(w, uint64(len(s.events)))
	g.ToolPolicyVersion = 2
	w.ToolPolicyVersion = 2
	w.Context = &ContextPin{Version: 5, PublishedReports: copyPublishedReports(g.PublishedReportContext.Pins)}
	g.PluginPins, err = s.selectPlugins(w, in.PluginSelection)
	if err != nil {
		return Collaboration{}, false, err
	}
	w.PluginPins = g.PluginPins
	if !pinnedTool(w, "report.read_artifact") {
		stripPublishedFiles(g.PublishedReportContext.Pins)
	}
	commit := func(pin *connections.Pin) error {
		g.Account = pin
		return s.withInstallPin(g.ExecutorPolicyVersion, g.ExecutorPin, func() error { return s.commit(Event{Type: "collaboration.created", Collaboration: &g}) })
	}
	if in.AccountID == "" {
		if in.AccountRevision != 0 {
			return Collaboration{}, false, ErrInvalid
		}
		err = commit(nil)
	} else if s.accounts == nil {
		return Collaboration{}, false, ErrConflict
	} else {
		err = s.accounts.WithSelection(in.AccountID, in.AccountRevision, func(pin connections.Pin) error { return commit(&pin) })
		if errors.Is(err, connections.ErrConflict) {
			err = ErrConflict
		}
	}
	return copyCollaboration(g), false, err
}

func stageBot(g Collaboration, stage int) string {
	if stage == 2 {
		for _, id := range g.Participants {
			if id != g.CoordinatorBotID {
				return id
			}
		}
	}
	return g.CoordinatorBotID
}
func stageNonce(g Collaboration, stage int) string { return g.ID + fmt.Sprintf("-stage-%d", stage) }
func stageText(g Collaboration, stage int) string {
	role := "Propose a plan using the goal and allowed sources. No file writes or external effects; the final coordinator has sole file authority."
	if stage == 2 {
		role = "Review the coordinator's prior result from the pinned shared-channel context. Identify evidence gaps and tradeoffs; do not invent agreement. No file writes or external effects; the final coordinator has sole file authority."
	}
	if stage == 3 {
		role = "Return the final conclusion using both prior stage results. Separate unresolved owner choices from evidence. Save a file only if the owner explicitly requested one."
	}
	return fmt.Sprintf("Finite collaboration %s, stage %d of 3. %s\nGoal from owner:\n%s", g.ID, stage, role, g.Goal)
}
func stageWriteAllowed(w Work) bool { return w.Collaboration == nil || w.Collaboration.Stage == 3 }

func (s *Store) collaborationAllowed(w Work) bool {
	if w.Collaboration == nil {
		return true
	}
	g, ok := s.collaborations[w.Collaboration.ID]
	if !ok || (w.Status != "completed" && g.Status != "running") || w.Collaboration.Stage < 1 || w.Collaboration.Stage > len(g.RequestIDs) {
		return false
	}
	return g.RequestIDs[w.Collaboration.Stage-1] == w.RequestID
}

func (s *Store) validCollaborationWork(w Work) bool {
	if w.Collaboration == nil {
		return true
	}
	if w.UseBotDefaults || w.ExpectedBotStateRevision != nil || w.BotState != nil {
		return false
	}
	g, ok := s.collaborations[w.Collaboration.ID]
	stage := w.Collaboration.Stage
	if !ok || w.ExecutorPolicyVersion != g.ExecutorPolicyVersion || !reflect.DeepEqual(w.ExecutorPin, g.ExecutorPin) || w.ToolPolicyVersion != g.ToolPolicyVersion || !reflect.DeepEqual(w.PluginSelection, g.PluginSelection) || !reflect.DeepEqual(w.PluginPins, g.PluginPins) || g.Status != "running" || g.BudgetClosedAt != "" || g.BudgetClosedReason != "" || stage != len(g.RequestIDs)+1 || stage > 3 || w.BotID != stageBot(g, stage) || w.ClientNonce != stageNonce(g, stage) || w.Text != stageText(g, stage) || w.Lane != "background" || w.ConversationID != g.ChannelID || w.SourceBotID != "" || w.RoutedAlias != "" || w.Publication == nil || w.Publication.AudienceRevision != g.ExpectedChannelRevision || !reflect.DeepEqual(w.Account, g.Account) || !reflect.DeepEqual(w.Resources, g.Resources) {
		return false
	}
	for _, id := range g.RequestIDs {
		prior, ok := s.work[id]
		if !ok || prior.Status != "completed" || strings.TrimSpace(prior.Reply) == "" || !operationsComplete(prior) || !s.contextAllowed(prior) {
			return false
		}
		found := false
		if w.Context != nil {
			for _, source := range w.Context.Sources {
				found = found || source.RequestID == id
			}
		}
		if !found {
			return false
		}
	}
	return s.validGroupProfileWork(w, g)
}

func (s *Store) validCollaborationEvent(e Event) bool {
	if s.lifecycleNonceUsed(e.Nonce) {
		return false
	}
	if e.CompletionVersion != 0 || e.ContextRefreshVersion != 0 || e.Collaboration == nil || e.TaskControl != nil || e.AdapterSession != nil || e.Bot != nil || e.Channel != nil || e.Project != nil || e.Upload != nil || e.ResourceRevision != 0 || !reflect.DeepEqual(e.Work, Work{}) {
		return false
	}
	g := *e.Collaboration
	if e.Type == "collaboration.created" {
		if s.lifecycleNonceUsed(g.ClientNonce) {
			return false
		}
		if s.archivedProject(g.ProjectID) {
			return false
		}
		for _, id := range g.Participants {
			if s.archivedBot(id) {
				return false
			}
		}
		if e.Nonce != "" || !resourceID(g.ID, "collab-") || (g.Version != 1 && g.Version != 2) || g.OwnerUserID != "local-owner" || g.SpaceID != "local-space" || g.Revision != 1 || g.Status != "running" || g.Reason != "" || g.BudgetClosedAt != "" || g.BudgetClosedReason != "" || len(g.RequestIDs) != 0 || !s.validCollaborationInput(g.CollaborationInput) {
			return false
		}
		created, e1 := time.Parse(time.RFC3339Nano, g.CreatedAt)
		deadline, e2 := time.Parse(time.RFC3339Nano, g.DeadlineAt)
		if e1 != nil || e2 != nil || !s.validGroupProfile(g) || g.Version == 1 && deadline.Sub(created) != 5*time.Minute || g.Version == 2 && (s.coreProfileRetired[g.CoreProfileSelection.ConsentID] || len(g.CoreProfileGroup.Budget.RecoveryReservations) != 0 || s.coreProfileConsents[g.CoreProfileSelection.ConsentID].Status != "approved" || s.coreProfileConsents[g.CoreProfileSelection.ConsentID].Revision != g.CoreProfileSelection.ExpectedConsentRevision) {
			return false
		}
		if _, ok := s.collaborations[g.ID]; ok {
			return false
		}
		if _, ok := s.collaborationNonces[g.ClientNonce]; ok {
			return false
		}
		if g.Account == nil {
			if g.AccountID != "" || g.AccountRevision != 0 {
				return false
			}
		} else if !connections.ValidPin(*g.Account) || g.Account.AccountID != g.AccountID || g.Account.Revision != g.AccountRevision {
			return false
		}
		pin, err := s.pinResources(Work{BotID: g.CoordinatorBotID, OwnerUserID: g.OwnerUserID, SpaceID: g.SpaceID, ConversationID: g.ChannelID, Text: g.Goal}, collaborationScope(g.CollaborationInput))
		candidate := Work{ExecutorPolicyVersion: g.ExecutorPolicyVersion, ExecutorPin: g.ExecutorPin, Resources: g.Resources, ToolPolicyVersion: g.ToolPolicyVersion, PluginSelection: g.PluginSelection, PluginPins: g.PluginPins}
		if g.PublishedReportContext != nil {
			if g.PublishedReportContext.Version != 1 && g.PublishedReportContext.Version != 2 && g.PublishedReportContext.Version != 3 {
				return false
			}
			probe := Work{BotID: g.CoordinatorBotID, OwnerUserID: g.OwnerUserID, SpaceID: g.SpaceID, ConversationID: g.ChannelID}
			s.pinBotProfile(&probe)
			if e.Sequence == 0 {
				return false
			}
			expected := s.capturePublishedReportsVersion(probe, e.Sequence-1, g.PublishedReportContext.Version)
			probe.ToolPolicyVersion = g.ToolPolicyVersion
			probe.PluginPins = g.PluginPins
			if !pinnedTool(probe, "report.read_artifact") {
				stripPublishedFiles(expected.Pins)
			}
			if g.ToolPolicyVersion != 2 || !reflect.DeepEqual(g.PublishedReportContext, expected) {
				return false
			}
			candidate.Context = &ContextPin{Version: 5, PublishedReports: copyPublishedReports(s.capturePublishedReportsVersion(probe, e.Sequence-1, g.PublishedReportContext.Version).Pins)}
		} else if g.ToolPolicyVersion == 2 {
			return false
		}
		return validExecutorWork(candidate) && err == nil && reflect.DeepEqual(pin, g.Resources) && s.validPluginWork(candidate)
	}
	old, ok := s.collaborations[g.ID]
	if !ok || old.Revision == ^uint64(0) || g.Revision != old.Revision+1 {
		return false
	}
	expected := copyCollaboration(old)
	expected.Revision = g.Revision
	switch e.Type {
	case "collaboration.budget_closed":
		if e.Nonce != "" || e.Plugin != nil || e.ProjectContinuity != nil || e.CollaborationRecovery != nil || e.ContextClaimVersion != 0 || e.ExecutorClaimVersion != 0 || old.BudgetClosedAt != "" || old.BudgetClosedReason != "" || (old.Status != "running" && old.Status != "suspended" && old.Status != "needs_owner" && old.Status != "failed" && old.Status != "cancelling") {
			return false
		}
		at, err := time.Parse(time.RFC3339Nano, g.BudgetClosedAt)
		if err != nil || at.UTC().Format(time.RFC3339Nano) != g.BudgetClosedAt || collaborationBudgetClosureReason(old, at) == "" || collaborationBudgetClosureReason(old, at) != g.BudgetClosedReason {
			return false
		}
		expected.BudgetClosedAt, expected.BudgetClosedReason = g.BudgetClosedAt, g.BudgetClosedReason
	case "collaboration.stage":
		if e.Nonce != "" || old.Status != "running" || len(g.RequestIDs) != len(old.RequestIDs)+1 || len(g.RequestIDs) > 3 || !reflect.DeepEqual(old.RequestIDs, g.RequestIDs[:len(old.RequestIDs)]) {
			return false
		}
		w, ok := s.work[g.RequestIDs[len(g.RequestIDs)-1]]
		if !ok || !s.validCollaborationWork(w) {
			return false
		}
		expected.RequestIDs = g.RequestIDs
	case "collaboration.stopped":
		if e.Nonce != "" || old.Status != "running" || !validCollaborationStop(g.Status, g.Reason) {
			return false
		}
		if g.Status == "completed" {
			if len(old.RequestIDs) != 3 {
				return false
			}
			for _, id := range old.RequestIDs {
				w := s.work[id]
				if w.Status != "completed" || strings.TrimSpace(w.Reply) == "" || !operationsComplete(w) {
					return false
				}
			}
		}
		expected.Status = g.Status
		expected.Reason = g.Reason
	case "collaboration.cancelling":
		if !validAccessNonce(e.Nonce) || (old.Status != "running" && old.Status != "suspended" && old.Status != "needs_owner" && old.Status != "failed") || g.Status != "cancelling" || g.Reason != "owner_cancelled" {
			return false
		}

		if _, ok := s.collaborationActions[e.Nonce]; ok {
			return false
		}
		expected.Status = g.Status
		expected.Reason = g.Reason
	case "collaboration.cancelled":
		if e.Nonce != "" || old.Status != "cancelling" || g.Status != "cancelled" || g.Reason != "owner_cancelled" {
			return false
		}
		for _, w := range s.work {
			if w.Collaboration != nil && w.Collaboration.ID == g.ID && !collaborationTaskSettled(w) {
				return false
			}
		}
		expected.Status = g.Status
	default:
		return false
	}
	return reflect.DeepEqual(expected, g)
}
func validCollaborationStop(status, reason string) bool {
	switch status {
	case "completed":
		return reason == "stage_budget_completed"
	case "suspended":
		return reason == "authority_changed" || reason == "task_suspended"
	case "needs_owner":
		return reason == "stage_context_unavailable" || reason == "outcome_unresolved" || reason == "empty_result" || reason == "stage_budget_expired"
	case "failed":
		return reason == "stage_accept_failed" || reason == "task_cancelled"
	}
	return false
}
func collaborationTaskSettled(w Work) bool {
	return w.Status == "completed" || ((w.Status == "cancelled" || w.Status == "suspended") && (w.RuntimeStopped || w.AttemptID == ""))
}

func (s *Store) collaborationAuthority(g Collaboration) bool {
	if !s.groupProfileCurrent(g) {
		return false
	}
	if g.PublishedReportContext != nil {
		for _, bot := range g.Participants {
			probe := Work{BotID: bot, OwnerUserID: g.OwnerUserID, SpaceID: g.SpaceID, ConversationID: g.ChannelID, ToolPolicyVersion: g.ToolPolicyVersion, PluginPins: g.PluginPins}
			for _, p := range g.PublishedReportContext.Pins {
				if !s.publishedReportAllowed(probe, p, g.PublishedReportContext.AtSequence) {
					return false
				}
			}
		}
	}
	if s.executorRequired || g.ExecutorPolicyVersion != 0 {
		if s.executor == nil || g.ExecutorPin == nil || g.ExecutorPolicyVersion != 1 || *g.ExecutorPin != s.executor.Snapshot().Pin {
			return false
		}
	}
	c, ok := s.channel(g.ChannelID)
	if !ok || c.Revision != g.ExpectedChannelRevision || !member(c, g.Participants[0]) || !member(c, g.Participants[1]) {
		return false
	}
	w := Work{Account: g.Account, Resources: g.Resources, ConversationID: g.ChannelID}
	w.ToolPolicyVersion = g.ToolPolicyVersion
	w.PluginPins = g.PluginPins
	if !s.resourceAllowed(w) || !s.pluginsAllowed(w) {
		return false
	}
	release, err := s.accountGuard(w, false)
	if err != nil {
		return false
	}
	release()
	return true
}
func (s *Store) stageWorkLocked(g Collaboration) (Work, error) {
	release, err := s.accountGuard(Work{Account: g.Account}, false)
	if err != nil {
		return Work{}, err
	}
	defer release()
	stage := len(g.RequestIDs) + 1
	in := Input{BotID: stageBot(g, stage), ClientNonce: stageNonce(g, stage), Text: stageText(g, stage), Lane: "background"}
	if id, ok := s.nonces[inputNonceKey(in, g.ChannelID)]; ok {
		w := s.work[id]
		if !s.validCollaborationWork(w) {
			return Work{}, ErrConflict
		}
		return copyWork(w), nil
	}
	req, err := newID("req-")
	if err != nil {
		return Work{}, err
	}
	msg, err := newID("msg-")
	if err != nil {
		return Work{}, err
	}
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	if g.CoreProfileGroup != nil {
		createdAt = s.clock().UTC().Format(time.RFC3339Nano)
	}
	w := Work{RequestID: req, SourceMessageID: msg, OwnerUserID: g.OwnerUserID, SpaceID: g.SpaceID, BotID: in.BotID, ConversationID: g.ChannelID, ClientNonce: in.ClientNonce, Text: in.Text, Lane: in.Lane, Status: "waiting_host", DesiredState: "running", Epoch: 1, BotConfigRevision: 1, CreatedAt: createdAt, CreatedSequence: uint64(len(s.events) + 1), Account: g.Account, Resources: copyResources(g.Resources), Collaboration: &CollaborationBinding{ID: g.ID, Stage: stage}}
	w.Publication = &Publication{Version: 1, ActorID: g.OwnerUserID, Platform: "nekobot-local", SourceChannelID: g.ChannelID, SourceMessageID: msg, ReplyChannelID: g.ChannelID, ReplyThreadID: "thread-" + msg, AudienceRevision: g.ExpectedChannelRevision}
	w.ToolPolicyVersion = g.ToolPolicyVersion
	w.PluginSelection = copySelection(g.PluginSelection)
	w.PluginPins = append([]PluginPin(nil), g.PluginPins...)
	w.ExecutorPolicyVersion = g.ExecutorPolicyVersion
	if g.ExecutorPin != nil {
		pin := *g.ExecutorPin
		w.ExecutorPin = &pin
	}
	s.pinBotProfile(&w)
	w.Context = s.captureContext(w)
	if err = s.pinGroupStageCoreProfile(g, &w); err != nil {
		return Work{}, err
	}
	if err = s.attachRuntimeContext(&w); err != nil {
		return Work{}, err
	}
	if !s.validCollaborationWork(w) || !boundedRuntimePrompt(w) {
		return Work{}, ErrInvalid
	}
	if err = s.withInstallPin(w.ExecutorPolicyVersion, w.ExecutorPin, func() error { return s.commit(Event{Type: "work.accepted", Work: w}) }); err != nil {
		return Work{}, err
	}
	return copyWork(w), nil
}
func (s *Store) stopCollaboration(g Collaboration, status, reason string) error {
	g = copyCollaboration(g)
	g.Revision++
	g.Status = status
	g.Reason = reason
	return s.commit(Event{Type: "collaboration.stopped", Collaboration: &g})
}

func (s *Store) AdvanceCollaborations() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.identity != nil {
		return nil
	}
	if s.closed || s.poisoned {
		return ErrUnavailable
	}
	ids := make([]string, 0, len(s.collaborations))
	for id := range s.collaborations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		g := s.collaborations[id]
		now := s.clock()
		var err error
		g, err = s.observeCollaborationBudgetLocked(g, now)
		if err != nil {
			return err
		}
		if g.Status == "cancelling" || g.Status == "suspended" || g.Status == "needs_owner" || g.Status == "failed" {
			for _, w := range s.sortedWorkLocked() {
				if groupServiceStopPending(g, w) {
					continue
				}
				if g.Status == "cancelling" && w.Collaboration != nil && w.Collaboration.ID == id && w.Status == "cancelling" && w.DesiredState == "cancelled" && stoppedTask(w) {
					settled := copyWork(w)
					settled.Status = "cancelled"
					settled.Failure = "outcome_not_rollback"
					if err := s.commit(Event{Type: "work.cancelled", Work: settled}); err != nil {
						return err
					}
					continue
				}
				if w.Collaboration != nil && w.Collaboration.ID == id && !collaborationTaskSettled(w) && w.Status != "cancelling" {
					if _, err := s.cancelLocked(w.RequestID); err != nil {
						return err
					}
				}
			}
			if g.Status == "cancelling" {
				settled := true
				for _, w := range s.work {
					if w.Collaboration != nil && w.Collaboration.ID == id {
						settled = settled && collaborationTaskSettled(w)
					}
				}
				if settled {
					g.Revision++
					g.Status = "cancelled"
					if err := s.commit(Event{Type: "collaboration.cancelled", Collaboration: &g}); err != nil {
						return err
					}
				}
			}
			continue
		}
		if g.Status != "running" {
			continue
		}
		if !groupBudgetAt(g, now) {
			if err := s.stopCollaboration(g, "needs_owner", "stage_budget_expired"); err != nil {
				return err
			}
			continue
		}
		if err := s.groupProfileGuard(g); err != nil {
			if err == ErrConflict {
				continue
			}
			if err = s.stopCollaboration(g, "suspended", "authority_changed"); err != nil {
				return err
			}
			continue
		}
		if !s.collaborationAuthority(g) {
			if err := s.stopCollaboration(g, "suspended", "authority_changed"); err != nil {
				return err
			}
			continue
		}
		if len(g.RequestIDs) > 0 {
			w := s.work[g.RequestIDs[len(g.RequestIDs)-1]]
			status, reason := "", ""
			switch w.Status {
			case "suspending", "suspended":
				status, reason = "suspended", "task_suspended"
			case "cancelled", "cancelling":
				status, reason = "failed", "task_cancelled"
			case "recovery_required":
				status, reason = "needs_owner", "outcome_unresolved"
			case "completed":
				if !operationsComplete(w) {
					status, reason = "needs_owner", "outcome_unresolved"
				} else if strings.TrimSpace(w.Reply) == "" {
					status, reason = "needs_owner", "empty_result"
				} else if len(g.RequestIDs) == 3 {
					status, reason = "completed", "stage_budget_completed"
				}
			default:
				continue
			}
			if status != "" {
				if err := s.stopCollaboration(g, status, reason); err != nil {
					return err
				}
				continue
			}
		}
		w, err := s.stageWorkLocked(g)
		if err != nil {
			if errors.Is(err, ErrUnavailable) {
				return err
			}
			reason := "stage_accept_failed"
			status := "failed"
			if errors.Is(err, ErrInvalid) {
				status, reason = "needs_owner", "stage_context_unavailable"
			} else if errors.Is(err, ErrConflict) && !s.collaborationAuthority(g) {
				status, reason = "suspended", "authority_changed"
			}
			if err = s.stopCollaboration(g, status, reason); err != nil {
				return err
			}
			continue
		}
		g = copyCollaboration(g)
		g.RequestIDs = append(g.RequestIDs, w.RequestID)
		g.Revision++
		if err = s.commit(Event{Type: "collaboration.stage", Collaboration: &g}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CancelCollaboration(id string, in CollaborationCancelInput) (Collaboration, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return Collaboration{}, false, ErrUnavailable
	}
	g, ok := s.collaborations[id]
	if !ok {
		return Collaboration{}, false, ErrNotFound
	}
	if !validAccessNonce(in.ClientNonce) {
		return Collaboration{}, false, ErrInvalid
	}
	if e, ok := s.collaborationActions[in.ClientNonce]; ok {
		if e.Type != "collaboration.cancelling" || e.Collaboration.ID != id || e.Collaboration.Revision != in.ExpectedRevision+1 {
			return Collaboration{}, false, ErrConflict
		}
		return copyCollaboration(g), true, nil
	}
	if s.lifecycleNonceUsed(in.ClientNonce) {
		return Collaboration{}, false, ErrConflict
	}
	if _, ok := s.taskControlNonces[in.ClientNonce]; ok {
		return Collaboration{}, false, ErrConflict
	}
	if _, ok := s.collaborationNonces[in.ClientNonce]; ok {
		return Collaboration{}, false, ErrConflict
	}
	if g.Revision != in.ExpectedRevision || (g.Status != "running" && g.Status != "suspended" && g.Status != "needs_owner" && g.Status != "failed") {
		return Collaboration{}, false, ErrConflict
	}
	g = copyCollaboration(g)
	g.Revision++
	g.Status = "cancelling"
	g.Reason = "owner_cancelled"
	if err := s.commit(Event{Type: "collaboration.cancelling", Collaboration: &g, Nonce: in.ClientNonce}); err != nil {
		return Collaboration{}, false, err
	}
	for _, w := range s.sortedWorkLocked() {
		if w.Collaboration != nil && w.Collaboration.ID == id && !collaborationTaskSettled(w) && w.Status != "cancelling" {
			if _, err := s.cancelLocked(w.RequestID); err != nil {
				return Collaboration{}, false, err
			}
		}
	}
	return copyCollaboration(g), false, nil
}
