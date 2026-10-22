package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"nekoaurora.local/native/internal/protocol"
)

const finiteCoreProfilePurpose = "finite_collaboration_v1"

type CoreProfileGroupIntent struct {
	Version                    int                `json:"version"`
	ChannelID                  string             `json:"channelId"`
	ExpectedChannelRevision    uint64             `json:"expectedChannelRevision"`
	Participants               []string           `json:"participants"`
	CoordinatorBotID           string             `json:"coordinatorBotId"`
	Goal                       string             `json:"goal"`
	MaxStages                  int                `json:"maxStages"`
	ClientNonce                string             `json:"clientNonce"`
	ProjectID                  string             `json:"projectId,omitempty"`
	ProjectRevision            uint64             `json:"projectRevision,omitempty"`
	FileIDs                    []string           `json:"fileIds,omitempty"`
	PluginSelection            *[]PluginSelection `json:"pluginSelection,omitempty"`
	ExpectedContinuityRevision *uint64            `json:"expectedContinuityRevision,omitempty"`
}

type CoreProfileGroupBinding struct {
	Version         int    `json:"version"`
	CollaborationID string `json:"collaborationId"`
	Stage           int    `json:"stage"`
	IntentSHA256    string `json:"intentSha256"`
}

type GroupCoreProfileQuota struct {
	Requests     int `json:"requests"`
	Steps        int `json:"steps"`
	OutputTokens int `json:"outputTokens"`
}

type GroupCoreProfileStageAllocation struct {
	Stage  int               `json:"stage"`
	BotID  string            `json:"botId"`
	Limits CoreProfileLimits `json:"limits"`
}

type GroupCoreProfilePlan struct {
	Version          int                               `json:"version"`
	DeadlineAtUnixMS int64                             `json:"deadlineAtUnixMs"`
	StageAllocations []GroupCoreProfileStageAllocation `json:"stageAllocations"`
	InitialReserved  GroupCoreProfileQuota             `json:"initialReserved"`
	RecoveryPool     GroupCoreProfileQuota             `json:"recoveryPool"`
}

type GroupCoreProfileRecoveryReservation struct {
	ClientNonce    string            `json:"clientNonce"`
	RequestID      string            `json:"requestId"`
	Stage          int               `json:"stage"`
	ExpectedEpoch  uint64            `json:"expectedEpoch"`
	IssuedAtUnixMS int64             `json:"issuedAtUnixMs"`
	Limits         CoreProfileLimits `json:"limits"`
}

type GroupCoreProfileBudget struct {
	Version              int                                   `json:"version"`
	RecoveryReserved     GroupCoreProfileQuota                 `json:"recoveryReserved"`
	RecoveryReservations []GroupCoreProfileRecoveryReservation `json:"recoveryReservations"`
}

type GroupCoreProfile struct {
	Version      int                    `json:"version"`
	Purpose      string                 `json:"purpose"`
	IntentSHA256 string                 `json:"intentSha256"`
	OriginalPin  CoreProfilePin         `json:"originalPin"`
	Plan         GroupCoreProfilePlan   `json:"plan"`
	Budget       GroupCoreProfileBudget `json:"budget"`
}

func copyGroupProfileIntent(p *CoreProfileGroupIntent) *CoreProfileGroupIntent {
	if p == nil {
		return nil
	}
	value := *p
	value.Participants = append([]string{}, p.Participants...)
	value.FileIDs = append([]string(nil), p.FileIDs...)
	value.PluginSelection = copySelection(p.PluginSelection)
	if p.ExpectedContinuityRevision != nil {
		r := *p.ExpectedContinuityRevision
		value.ExpectedContinuityRevision = &r
	}
	return &value
}

func copyGroupCoreProfile(p *GroupCoreProfile) *GroupCoreProfile {
	if p == nil {
		return nil
	}
	value := *p
	value.Plan.StageAllocations = append([]GroupCoreProfileStageAllocation{}, p.Plan.StageAllocations...)
	value.Budget.RecoveryReservations = append([]GroupCoreProfileRecoveryReservation{}, p.Budget.RecoveryReservations...)
	return &value
}

func copyCoreProfileConsentInput(in CoreProfileConsentInput) CoreProfileConsentInput {
	in.BotIDs = append([]string{}, in.BotIDs...)
	in.GroupIntent = copyGroupProfileIntent(in.GroupIntent)
	return in
}

func groupProfileIntent(in CollaborationInput) CoreProfileGroupIntent {
	in = normalizeCollaboration(in)
	return CoreProfileGroupIntent{Version: 1, ChannelID: in.ChannelID, ExpectedChannelRevision: in.ExpectedChannelRevision, Participants: in.Participants, CoordinatorBotID: in.CoordinatorBotID, Goal: in.Goal, MaxStages: in.MaxStages, ClientNonce: in.ClientNonce, ProjectID: in.ProjectID, ProjectRevision: in.ProjectRevision, FileIDs: in.FileIDs, PluginSelection: in.PluginSelection, ExpectedContinuityRevision: in.ExpectedContinuityRevision}
}

func groupIntentInput(in CoreProfileGroupIntent) CollaborationInput {
	return CollaborationInput{ChannelID: in.ChannelID, ExpectedChannelRevision: in.ExpectedChannelRevision, Participants: append([]string{}, in.Participants...), CoordinatorBotID: in.CoordinatorBotID, Goal: in.Goal, MaxStages: in.MaxStages, ClientNonce: in.ClientNonce, ProjectID: in.ProjectID, ProjectRevision: in.ProjectRevision, FileIDs: append([]string(nil), in.FileIDs...), PluginSelection: copySelection(in.PluginSelection), ExpectedContinuityRevision: in.ExpectedContinuityRevision}
}

func groupProfileIntentHash(in CoreProfileGroupIntent) string {
	raw, err := json.Marshal(struct {
		Version int                    `json:"version"`
		Purpose string                 `json:"purpose"`
		Intent  CoreProfileGroupIntent `json:"intent"`
	}{1, finiteCoreProfilePurpose, in})
	if err != nil {
		return ""
	}
	raw, err = protocol.Canonical(raw)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func groupProfileLimits(l CoreProfileLimits) (int, int, bool) {
	if !validProfileLimits(l) || l.MaxRequests < 4 || l.MaxSteps < 4 {
		return 0, 0, false
	}
	u := min(l.MaxRequests-4, l.MaxSteps-4, 2)
	q := l.MaxOutputTokens / (4 + u)
	return u, q, q > 0
}

func (s *Store) validGroupConsentPurpose(c CoreProfileConsent) bool {
	if c.Version == 1 {
		return c.Purpose == "" && c.GroupIntent == nil && c.GroupIntentSHA256 == ""
	}
	i := c.GroupIntent
	if c.Version != 2 || c.Purpose != finiteCoreProfilePurpose || i == nil || i.Version != 1 || i.ClientNonce == "" || !sort.StringsAreSorted(i.Participants) || !sort.StringsAreSorted(i.FileIDs) || !reflect.DeepEqual(groupProfileIntent(groupIntentInput(*i)), *i) || c.GroupIntentSHA256 != groupProfileIntentHash(*i) || !lowerHex(c.GroupIntentSHA256, 64) || !s.validCollaborationInput(groupIntentInput(*i)) || c.ChannelID != i.ChannelID || c.AudienceRevision != i.ExpectedChannelRevision || c.ProjectID != i.ProjectID || c.ProjectRevision != i.ProjectRevision || !reflect.DeepEqual(c.BotIDs, i.Participants) {
		return false
	}
	_, _, ok := groupProfileLimits(c.Limits)
	return ok
}

func (s *Store) validGroupConsentInput(in CoreProfileConsentInput) bool {
	if in.Purpose == "" {
		return in.GroupIntent == nil
	}
	if in.Purpose != finiteCoreProfilePurpose || in.GroupIntent == nil || in.ClientNonce == in.GroupIntent.ClientNonce {
		return false
	}
	c := CoreProfileConsent{Version: 2, Purpose: in.Purpose, GroupIntent: copyGroupProfileIntent(in.GroupIntent), GroupIntentSHA256: groupProfileIntentHash(*in.GroupIntent), ChannelID: in.ChannelID, AudienceRevision: in.ExpectedChannelRevision, BotIDs: append([]string{}, in.BotIDs...), ProjectID: in.ProjectID, ProjectRevision: in.ExpectedProjectRevision, Limits: in.CoreProfileLimits}
	return s.validGroupConsentPurpose(c)
}

func groupProfilePlan(g Collaboration, p CoreProfilePin) (GroupCoreProfilePlan, bool) {
	u, q, ok := groupProfileLimits(p.Limits)
	created, err := time.Parse(time.RFC3339Nano, g.CreatedAt)
	if !ok || err != nil || created.UnixMilli() <= 0 || created.UnixMilli() >= 1<<53-int64(p.Limits.MaxSeconds)*1000 {
		return GroupCoreProfilePlan{}, false
	}
	deadline := min(created.UnixMilli()+300000, created.UnixMilli()+int64(p.Limits.MaxSeconds)*1000, p.ExpiresAtUnixMS)
	if p.CredentialExpiresAtUnixMS > 0 {
		deadline = min(deadline, p.CredentialExpiresAtUnixMS)
	}
	if deadline <= created.UnixMilli() || deadline >= 1<<53 {
		return GroupCoreProfilePlan{}, false
	}
	plan := GroupCoreProfilePlan{Version: 1, DeadlineAtUnixMS: deadline, StageAllocations: []GroupCoreProfileStageAllocation{}, InitialReserved: GroupCoreProfileQuota{4, 4, 4 * q}, RecoveryPool: GroupCoreProfileQuota{u, u, u * q}}
	for i, weight := range []int{1, 1, 2} {
		limits := p.Limits
		limits.MaxRequests, limits.MaxSteps, limits.MaxOutputTokens = weight, weight, weight*q
		plan.StageAllocations = append(plan.StageAllocations, GroupCoreProfileStageAllocation{Stage: i + 1, BotID: stageBot(g, i+1), Limits: limits})
	}
	return plan, true
}

func (s *Store) groupProfileOriginalConsent(g Collaboration) (CoreProfileConsent, bool) {
	if g.CoreProfileGroup == nil || g.CoreProfileSelection == nil {
		return CoreProfileConsent{}, false
	}
	p := g.CoreProfileGroup.OriginalPin
	c, ok := s.coreProfileConsents[p.ConsentID]
	if !ok || c.Version != 2 || p.ConsentRevision != 1 || g.CoreProfileSelection.ConsentID != c.ID || g.CoreProfileSelection.ExpectedConsentRevision != p.ConsentRevision {
		return CoreProfileConsent{}, false
	}
	c = copyCoreConsent(c)
	c.Status, c.Revision = "approved", p.ConsentRevision
	return c, profileConsentPin(c) == p && c.Purpose == finiteCoreProfilePurpose && c.GroupIntentSHA256 == g.CoreProfileGroup.IntentSHA256 && c.GroupIntent != nil && reflect.DeepEqual(*c.GroupIntent, groupProfileIntent(g.CollaborationInput))
}

func (s *Store) validGroupProfile(g Collaboration) bool {
	if g.CoreProfileSelection == nil {
		return g.Version == 1 && g.CoreProfileGroup == nil
	}
	p := g.CoreProfileGroup
	if g.Version != 2 || p == nil || p.Version != 1 || p.Purpose != finiteCoreProfilePurpose || g.Account != nil || g.AccountID != "" || g.AccountRevision != 0 || p.IntentSHA256 != groupProfileIntentHash(groupProfileIntent(g.CollaborationInput)) {
		return false
	}
	c, ok := s.groupProfileOriginalConsent(g)
	created, e := time.Parse(time.RFC3339Nano, g.CreatedAt)
	plan, valid := groupProfilePlan(g, p.OriginalPin)
	if !ok || e != nil || created.UnixMilli() < c.CreatedAtUnixMS || created.UnixMilli() >= c.ExpiresAtUnixMS || !valid || !reflect.DeepEqual(plan, p.Plan) || g.DeadlineAt != time.UnixMilli(plan.DeadlineAtUnixMS).UTC().Format(time.RFC3339Nano) || p.Budget.Version != 1 || p.Budget.RecoveryReservations == nil || len(p.Budget.RecoveryReservations) > 2 {
		return false
	}
	total := GroupCoreProfileQuota{}
	nonces := map[string]bool{}
	for _, r := range p.Budget.RecoveryReservations {
		if r.Stage < 1 || r.Stage > 3 || !validAccessNonce(r.ClientNonce) || nonces[r.ClientNonce] || !resourceID(r.RequestID, "req-") || r.ExpectedEpoch == 0 || r.ExpectedEpoch >= 1<<53 || r.IssuedAtUnixMS < created.UnixMilli() || r.IssuedAtUnixMS >= plan.DeadlineAtUnixMS || r.Limits != plan.StageAllocations[r.Stage-1].Limits {
			return false
		}
		nonces[r.ClientNonce] = true
		total.Requests += r.Limits.MaxRequests
		total.Steps += r.Limits.MaxSteps
		total.OutputTokens += r.Limits.MaxOutputTokens
	}
	return p.Budget.RecoveryReserved == total && total.Requests <= plan.RecoveryPool.Requests && total.Steps <= plan.RecoveryPool.Steps && total.OutputTokens <= plan.RecoveryPool.OutputTokens
}

func (s *Store) groupProfileCurrent(g Collaboration) bool {
	if g.CoreProfileGroup == nil {
		return g.CoreProfileSelection == nil
	}
	if !s.validGroupProfile(g) || !groupBudgetAt(g, s.clock()) {
		return false
	}
	p := g.CoreProfileGroup.OriginalPin
	c, ok := s.coreProfileConsents[p.ConsentID]
	return ok && c.Revision == p.ConsentRevision && s.profileConsentActive(c) && profileNow(s) < g.CoreProfileGroup.Plan.DeadlineAtUnixMS && s.validGroupConsentPurpose(c)
}

func (s *Store) verifyGroupProfileSelection(in CollaborationInput) error {
	selection := in.CoreProfileSelection
	if selection == nil {
		return nil
	}
	if !s.coreProfilesEnabled() || !s.contextAdmissionRequired || !s.executorRequired || s.executor == nil {
		return ErrForbidden
	}
	c, ok := s.coreProfileConsents[selection.ConsentID]
	if !ok || c.Version != 2 || selection.ExpectedConsentRevision != c.Revision || !s.validGroupConsentPurpose(c) || !s.profileConsentActive(c) || in.AccountID != "" || in.AccountRevision != 0 || c.GroupIntentSHA256 != groupProfileIntentHash(groupProfileIntent(in)) {
		return ErrConflict
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	service := s.coreProfiles
	s.mu.Unlock()
	err := service.Verify(ctx, profileConsentPin(c))
	s.mu.Lock()
	current, ok := s.coreProfileConsents[c.ID]
	if err != nil || !ok || !reflect.DeepEqual(current, c) || !s.profileConsentActive(current) || !s.validGroupConsentPurpose(current) || s.closed || s.poisoned {
		return ErrConflict
	}
	return nil
}

func (s *Store) pinGroupCoreProfile(g *Collaboration) error {
	if g.CoreProfileSelection == nil {
		return nil
	}
	c, ok := s.coreProfileConsents[g.CoreProfileSelection.ConsentID]
	if !ok || c.Version != 2 || c.Revision != g.CoreProfileSelection.ExpectedConsentRevision || !s.profileConsentActive(c) || !s.validGroupConsentPurpose(c) || c.GroupIntentSHA256 != groupProfileIntentHash(groupProfileIntent(g.CollaborationInput)) {
		return ErrConflict
	}
	p := profileConsentPin(c)
	plan, ok := groupProfilePlan(*g, p)
	if !ok {
		return ErrInvalid
	}
	g.Version = 2
	g.DeadlineAt = time.UnixMilli(plan.DeadlineAtUnixMS).UTC().Format(time.RFC3339Nano)
	g.CoreProfileGroup = &GroupCoreProfile{Version: 1, Purpose: finiteCoreProfilePurpose, IntentSHA256: c.GroupIntentSHA256, OriginalPin: p, Plan: plan, Budget: GroupCoreProfileBudget{Version: 1, RecoveryReservations: []GroupCoreProfileRecoveryReservation{}}}
	if !s.validGroupProfile(*g) {
		return ErrInvalid
	}
	return nil
}

func groupStageCoreProfile(g Collaboration, w Work) (CoreProfilePin, CoreProfileGroupBinding, bool) {
	if g.CoreProfileGroup == nil || w.Collaboration == nil || w.Collaboration.ID != g.ID || w.Collaboration.Stage < 1 || w.Collaboration.Stage > 3 {
		return CoreProfilePin{}, CoreProfileGroupBinding{}, false
	}
	stage := w.Collaboration.Stage
	if len(g.CoreProfileGroup.Plan.StageAllocations) != 3 {
		return CoreProfilePin{}, CoreProfileGroupBinding{}, false
	}
	p := g.CoreProfileGroup.OriginalPin
	p.BotID, p.ClientNonce, p.ContextSHA256, p.ResourcesSHA256 = stageBot(g, stage), stageNonce(g, stage), contextHash(w), profileHash(w.Resources)
	p.Limits = g.CoreProfileGroup.Plan.StageAllocations[stage-1].Limits
	p.ExpiresAtUnixMS = g.CoreProfileGroup.Plan.DeadlineAtUnixMS
	return p, CoreProfileGroupBinding{Version: 1, CollaborationID: g.ID, Stage: stage, IntentSHA256: g.CoreProfileGroup.IntentSHA256}, true
}

func (s *Store) pinGroupStageCoreProfile(g Collaboration, w *Work) error {
	if g.CoreProfileGroup == nil {
		return nil
	}
	p, binding, ok := groupStageCoreProfile(g, *w)
	if !ok || !s.groupProfileCurrent(g) {
		return ErrConflict
	}
	w.CoreProfilePin, w.CoreProfileGroupBinding = &p, &binding
	selection := *g.CoreProfileSelection
	w.CoreProfileSelection = &selection
	if !s.validGroupProfileWork(*w, g) || !profileInputBound(*w) {
		return ErrInvalid
	}
	return nil
}

func (s *Store) validGroupProfileWork(w Work, g Collaboration) bool {
	if g.CoreProfileGroup == nil {
		return w.CoreProfilePin == nil && w.CoreProfileSelection == nil && w.CoreProfileGroupBinding == nil
	}
	p, binding, ok := groupStageCoreProfile(g, w)
	return ok && s.validGroupProfile(g) && w.CoreProfilePin != nil && *w.CoreProfilePin == p && w.CoreProfileGroupBinding != nil && *w.CoreProfileGroupBinding == binding && reflect.DeepEqual(w.CoreProfileSelection, g.CoreProfileSelection) && w.Account == nil && w.Adapter == nil && w.TaskLink == nil && w.RequesterPin == nil && !w.UseBotDefaults && w.BotID == p.BotID && w.ClientNonce == p.ClientNonce && w.ConversationID == p.ChannelID && w.OwnerUserID == p.ActorScope.ActorID && w.SpaceID == p.ActorScope.SpaceID && w.Publication != nil && w.Publication.AudienceRevision == p.AudienceRevision && reflect.DeepEqual(w.Resources, g.Resources) && profileInputBound(w)
}

func (s *Store) groupProfilePendingStage(w Work) bool {
	if w.CoreProfileGroupBinding == nil || w.Collaboration == nil || w.Status != "waiting_host" || !neverLaunched(w) {
		return false
	}
	g, ok := s.collaborations[w.Collaboration.ID]
	return ok && g.Status == "running" && w.Collaboration.Stage == len(g.RequestIDs)+1 && s.validGroupProfileWork(w, g) && s.groupProfileCurrent(g)
}

func (s *Store) groupWorkProfileCurrent(w Work) bool {
	if w.CoreProfileGroupBinding == nil || w.Collaboration == nil {
		return false
	}
	g, ok := s.collaborations[w.Collaboration.ID]
	if !ok || g.Status != "running" || w.Collaboration.Stage < 1 || w.Collaboration.Stage > len(g.RequestIDs) || g.RequestIDs[w.Collaboration.Stage-1] != w.RequestID || !s.validGroupProfileWork(w, g) {
		return false
	}
	g, err := s.observeCollaborationBudgetLocked(g, s.clock())
	return err == nil && s.groupProfileCurrent(g)
}

func (s *Store) validGroupProfileIntake(w Work) bool {
	if w.CoreProfileGroupBinding == nil || w.Collaboration == nil || w.CoreProfilePin == nil {
		return false
	}
	g, ok := s.collaborations[w.Collaboration.ID]
	c, exists := s.coreProfileConsents[w.CoreProfilePin.ConsentID]
	created, err := time.Parse(time.RFC3339Nano, w.CreatedAt)
	groupCreated, groupErr := time.Parse(time.RFC3339Nano, g.CreatedAt)
	return ok && exists && g.CoreProfileGroup != nil && err == nil && groupErr == nil && c.Status == "approved" && c.Revision == w.CoreProfilePin.ConsentRevision && !s.coreProfileRetired[c.ID] && !created.Before(groupCreated) && created.UnixMilli() < g.CoreProfileGroup.Plan.DeadlineAtUnixMS && s.validGroupProfileWork(w, g)
}

func (s *Store) groupProfileRecordAuthority(w Work) bool {
	if w.CoreProfileGroupBinding == nil {
		return true
	}
	if w.Collaboration == nil || w.CoreProfilePin == nil {
		return false
	}
	g, ok := s.collaborations[w.Collaboration.ID]
	c, exists := s.coreProfileConsents[w.CoreProfilePin.ConsentID]
	return ok && exists && g.Status == "running" && g.BudgetClosedAt == "" && c.Status == "approved" && c.Revision == w.CoreProfilePin.ConsentRevision && !s.coreProfileRetired[c.ID] && s.validGroupProfileWork(w, g)
}

func (s *Store) groupProfileGuard(g Collaboration) error {
	if g.CoreProfileGroup == nil {
		return nil
	}
	if !s.groupProfileCurrent(g) {
		return ErrForbidden
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	service := s.coreProfiles
	s.mu.Unlock()
	err := service.Verify(ctx, g.CoreProfileGroup.OriginalPin)
	s.mu.Lock()
	current, exists := s.collaborations[g.ID]
	if !exists || !reflect.DeepEqual(current, g) {
		return ErrConflict
	}
	if err != nil || !s.groupProfileCurrent(current) || s.closed || s.poisoned {
		return ErrForbidden
	}
	return nil
}

func reserveGroupProfileRecovery(g *Collaboration, w Work, in CollaborationRecoveryInput, at time.Time) bool {
	if g.CoreProfileGroup == nil {
		return true
	}
	if w.Collaboration == nil || w.Collaboration.Stage < 1 || w.Collaboration.Stage > 3 || len(g.CoreProfileGroup.Plan.StageAllocations) != 3 {
		return false
	}
	stage := w.Collaboration.Stage
	limits := g.CoreProfileGroup.Plan.StageAllocations[stage-1].Limits
	p := copyGroupCoreProfile(g.CoreProfileGroup)
	budget := &p.Budget
	budget.RecoveryReserved.Requests += limits.MaxRequests
	budget.RecoveryReserved.Steps += limits.MaxSteps
	budget.RecoveryReserved.OutputTokens += limits.MaxOutputTokens
	if budget.RecoveryReserved.Requests > p.Plan.RecoveryPool.Requests || budget.RecoveryReserved.Steps > p.Plan.RecoveryPool.Steps || budget.RecoveryReserved.OutputTokens > p.Plan.RecoveryPool.OutputTokens || len(budget.RecoveryReservations) >= 2 {
		return false
	}
	budget.RecoveryReservations = append(budget.RecoveryReservations, GroupCoreProfileRecoveryReservation{ClientNonce: in.ClientNonce, RequestID: w.RequestID, Stage: stage, ExpectedEpoch: in.ExpectedEpoch, IssuedAtUnixMS: at.UnixMilli(), Limits: limits})
	g.CoreProfileGroup = p
	return true
}
