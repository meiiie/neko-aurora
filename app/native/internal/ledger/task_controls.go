package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"time"
)

var ErrUnsupported = errors.New("contract_unsupported")

type TaskControlInput struct {
	Version          int    `json:"version"`
	RequestID        string `json:"requestId"`
	ExpectedEpoch    uint64 `json:"expectedEpoch"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	Action           string `json:"action"`
	ClientNonce      string `json:"clientNonce"`
	Text             string `json:"text,omitempty"`
}
type TaskLink struct {
	Version             int    `json:"version"`
	Kind                string `json:"kind"`
	ParentRequestID     string `json:"parentRequestId"`
	ParentMessageID     string `json:"parentMessageId"`
	ParentEpoch         uint64 `json:"parentEpoch"`
	ParentPinSHA256     string `json:"parentPinSha256"`
	RequirementRevision uint64 `json:"requirementRevision"`
	CommandNonce        string `json:"commandNonce"`
}
type TaskControlEvent struct {
	Version int              `json:"version"`
	Input   TaskControlInput `json:"input"`
	Related *Work            `json:"related,omitempty"`
}
type TaskControlResult struct {
	Work    Work  `json:"work"`
	Related *Work `json:"related,omitempty"`
}

func neverLaunched(w Work) bool {
	return w.Core == nil && w.AttemptID == "" && w.RuntimeAttemptBinding == nil
}
func stoppedTask(w Work) bool {
	return neverLaunched(w) || (w.RuntimeStopped && w.RuntimeStopReceipt != nil && validStoppedReceipt(w, *w.RuntimeStopReceipt))
}
func controlActive(w Work) bool {
	return w.Status == "waiting_host" || w.Status == "starting" || w.Status == "running" || w.Status == "recovery_required" || w.Status == "suspended" || w.Status == "suspending" || w.Status == "paused" || w.Status == "waiting_control_stop"
}
func controlCancelled(w Work) Work {
	w = copyWork(w)
	w.Status = "cancelled"
	if !stoppedTask(w) {
		w.Status = "cancelling"
	}
	w.DesiredState = "cancelled"
	w.Epoch++
	return w
}
func validControlInput(in TaskControlInput) bool {
	if (in.Version != 1 && !(in.Version == 2 && in.Action == "recover")) || (in.Version == 1 && in.Action == "recover") || in.RequestID == "" || in.ExpectedEpoch == 0 || !validAccessNonce(in.ClientNonce) {
		return false
	}
	switch in.Action {
	case "pause", "resume", "cancel", "recover":
		return in.Text == ""
	case "supplement", "replace":
		return strings.TrimSpace(in.Text) != "" && len(in.Text) <= 32768 && !hasSecret([]byte(in.Text))
	default:
		return false
	}
}
func controlMutation(old Work, in TaskControlInput) (Work, error) {
	if old.Epoch != in.ExpectedEpoch || old.TaskRevision != in.ExpectedRevision || old.Epoch == ^uint64(0) || old.TaskRevision == ^uint64(0) || !controlActive(old) {
		return Work{}, ErrConflict
	}
	w := copyWork(old)
	w.TaskRevision++
	switch in.Action {
	case "recover":
		if !serviceRecoveryReady(old) {
			return Work{}, ErrConflict
		}
		w.Status, w.DesiredState, w.Failure = "waiting_host", "running", ""
		w.Epoch++
	case "pause":
		if old.Status != "waiting_host" || !neverLaunched(old) {
			return Work{}, ErrUnsupported
		}
		w.Status = "paused"
		w.DesiredState = "paused"
		w.Epoch++
		w.Failure = ""
	case "resume":
		if old.Status != "paused" || old.DesiredState != "paused" || !neverLaunched(old) || !operationsComplete(old) {
			return Work{}, ErrConflict
		}
		w.Status = "waiting_host"
		w.DesiredState = "running"
		w.Epoch++
		w.Failure = ""
	case "cancel", "replace":
		w = controlCancelled(w)
	case "supplement":
		if old.Status == "waiting_control_stop" {
			return Work{}, ErrConflict
		}
	}
	return w, nil
}
func (s *Store) controlResult(e Event) TaskControlResult {
	out := TaskControlResult{Work: copyWork(s.work[e.Work.RequestID])}
	if e.TaskControl.Related != nil {
		related := copyWork(s.work[e.TaskControl.Related.RequestID])
		out.Related = &related
	}
	return out
}
func (s *Store) ControlTask(in TaskControlInput) (TaskControlResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.identity != nil {
		return TaskControlResult{}, false, ErrForbidden
	}
	if s.closed || s.poisoned {
		return TaskControlResult{}, false, ErrUnavailable
	}
	if !validControlInput(in) {
		return TaskControlResult{}, false, ErrInvalid
	}
	if e, ok := s.taskControlNonces[in.ClientNonce]; ok {
		if !reflect.DeepEqual(e.TaskControl.Input, in) {
			return TaskControlResult{}, false, ErrConflict
		}
		return s.controlResult(e), true, nil
	}
	if s.lifecycleNonceUsed(in.ClientNonce) {
		return TaskControlResult{}, false, ErrConflict
	}
	if _, ok := s.collaborationActions[in.ClientNonce]; ok {
		return TaskControlResult{}, false, ErrConflict
	}
	if _, ok := s.collaborationNonces[in.ClientNonce]; ok {
		return TaskControlResult{}, false, ErrConflict
	}
	old, ok := s.work[in.RequestID]
	if !ok {
		return TaskControlResult{}, false, ErrNotFound
	}
	if hasCollaborationMarker(old) {
		return TaskControlResult{}, false, ErrUnsupported
	}
	if old.Adapter != nil && (in.Action == "replace" || in.Action == "supplement") {
		return TaskControlResult{}, false, ErrUnsupported
	}
	w, err := controlMutation(old, in)
	if err != nil {
		return TaskControlResult{}, false, err
	}
	if len(s.taskControlNonces) >= 2048 {
		return TaskControlResult{}, false, ErrInvalid
	}
	event := Event{Type: "task.control", Work: w, TaskControl: &TaskControlEvent{Version: in.Version, Input: in}}
	if in.Action == "resume" || in.Action == "recover" || in.Action == "supplement" || in.Action == "replace" {
		if !s.contextAllowed(old) {
			return TaskControlResult{}, false, ErrConflict
		}
		release, err := s.controlAccountGuard(old)
		if err != nil {
			return TaskControlResult{}, false, err
		}
		defer release()
		if in.Action != "resume" && in.Action != "recover" {
			related, err := s.newControlWork(old, in, w.TaskRevision)
			if err != nil {
				return TaskControlResult{}, false, err
			}
			event.TaskControl.Related = &related
		}
	}
	event.Sequence = uint64(len(s.events) + 1)
	if !s.validTaskControlEvent(event) {
		return TaskControlResult{}, false, ErrConflict
	}
	if in.Action == "recover" {
		err = s.withInstallPin(old.ExecutorPolicyVersion, old.ExecutorPin, func() error { return s.commit(event) })
	} else {
		err = s.commit(event)
	}
	if err != nil {
		return TaskControlResult{}, false, err
	}
	return s.controlResult(event), false, nil
}
func (s *Store) controlAccountGuard(w Work) (func(), error) {
	if w.Account == nil {
		return func() {}, nil
	}
	if s.accounts == nil || w.AccountExpiredAt != "" {
		return nil, ErrConflict
	}
	release, err := s.accounts.AcquirePin(*w.Account)
	if err != nil {
		return nil, ErrConflict
	}
	return release, nil
}
func (s *Store) newControlWork(parent Work, in TaskControlInput, revision uint64) (Work, error) {
	req, err := newID("req-")
	if err != nil {
		return Work{}, err
	}
	msg, err := newID("msg-")
	if err != nil {
		return Work{}, err
	}
	w := Work{RequestID: req, SourceMessageID: msg, OwnerUserID: parent.OwnerUserID, SpaceID: parent.SpaceID, BotID: parent.BotID, ConversationID: parent.ConversationID, ClientNonce: controlWorkNonce(parent.RequestID, in.ClientNonce), Text: in.Text, Lane: parent.Lane, Status: "waiting_host", DesiredState: "running", Epoch: 1, BotConfigRevision: parent.BotConfigRevision, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), CreatedSequence: uint64(len(s.events) + 1), Account: parent.Account}
	w.UseBotDefaults = parent.UseBotDefaults
	w.BotState = copyBotState(parent.BotState)
	if parent.ExpectedBotStateRevision != nil {
		r := *parent.ExpectedBotStateRevision
		w.ExpectedBotStateRevision = &r
	}
	if len(w.ClientNonce) > 128 {
		return Work{}, ErrInvalid
	}
	if err = s.validInput(Input{w.BotID, w.ClientNonce, w.Text, w.Lane}); err != nil {
		return Work{}, err
	}
	if target, _, err := s.resolveRoute(w.Text); err != nil {
		return Work{}, err
	} else if target != "" && target != w.BotID {
		return Work{}, ErrConflict
	}
	c, ok := s.channel(w.ConversationID)
	if !ok || parent.Publication == nil || c.Revision != parent.Publication.AudienceRevision {
		return Work{}, ErrConflict
	}
	w.Publication = &Publication{Version: 1, ActorID: w.OwnerUserID, Platform: "nekobot-local", SourceChannelID: w.ConversationID, SourceMessageID: w.SourceMessageID, ReplyChannelID: w.ConversationID, ReplyThreadID: "thread-" + w.SourceMessageID, AudienceRevision: c.Revision}
	scope := SubmissionScope{ChannelID: w.ConversationID}
	if parent.Resources != nil {
		scope.ProjectID = parent.Resources.Project.ID
		scope.ProjectRevision = parent.Resources.Project.Revision
		scope.FileIDs = parent.Resources.RequestedFileIDs
	}
	w.Resources, err = s.pinResources(w, scope)
	if err != nil {
		return Work{}, err
	}
	w.ExecutorPolicyVersion = parent.ExecutorPolicyVersion
	if parent.ExecutorPin != nil {
		pin := *parent.ExecutorPin
		w.ExecutorPin = &pin
	}
	w.ToolPolicyVersion = parent.ToolPolicyVersion
	w.PluginSelection = copySelection(parent.PluginSelection)
	w.PluginPins = append([]PluginPin(nil), parent.PluginPins...)
	s.pinBotProfile(&w)
	w.Context = s.captureContext(w)
	w.ContextProtocolVersion = parent.ContextProtocolVersion
	if err = s.attachRuntimeContext(&w); err != nil {
		return Work{}, err
	}
	if !boundedRuntimePrompt(w) {
		return Work{}, ErrInvalid
	}
	w.TaskLink = &TaskLink{Version: 1, Kind: in.Action, ParentRequestID: parent.RequestID, ParentMessageID: parent.SourceMessageID, ParentEpoch: parent.Epoch, ParentPinSHA256: contextHash(parent), RequirementRevision: revision, CommandNonce: in.ClientNonce}
	if in.Action == "replace" {
		w.Status = "waiting_control_stop"
	}
	return w, nil
}
func (s *Store) validTaskControlEvent(e Event) bool {
	if s.lifecycleNonceUsed(e.Nonce) {
		return false
	}
	c := e.TaskControl
	if c != nil && s.lifecycleNonceUsed(c.Input.ClientNonce) {
		return false
	}
	if e.Type != "task.control" || c == nil || c.Version != c.Input.Version || !validControlInput(c.Input) || e.Collaboration != nil || e.Bot != nil || e.Channel != nil || e.Project != nil || e.Upload != nil || e.AdapterSession != nil || e.Nonce != "" || e.ResourceRevision != 0 || e.CompletionVersion != 0 || e.ContextRefreshVersion != 0 {
		return false
	}
	if _, ok := s.taskControlNonces[c.Input.ClientNonce]; ok || len(s.taskControlNonces) >= 2048 {
		return false
	}
	if e, ok := s.collaborationActions[c.Input.ClientNonce]; ok && e.Type == "collaboration.recovered" {
		return false
	}
	old, ok := s.work[c.Input.RequestID]
	if !ok || hasCollaborationMarker(old) || old.Adapter != nil && (c.Input.Action == "replace" || c.Input.Action == "supplement") {
		return false
	}
	expected, err := controlMutation(old, c.Input)
	if err != nil || !reflect.DeepEqual(expected, e.Work) {
		return false
	}
	linked := c.Input.Action == "supplement" || c.Input.Action == "replace"
	if !linked {
		return c.Related == nil && ((c.Input.Action != "resume" && c.Input.Action != "recover") || s.contextAllowed(old))
	}
	if !s.contextAllowed(old) || c.Related == nil {
		return false
	}
	related := copyWork(*c.Related)
	if related.UseBotDefaults != old.UseBotDefaults || !reflect.DeepEqual(related.BotState, old.BotState) || !reflect.DeepEqual(related.ExpectedBotStateRevision, old.ExpectedBotStateRevision) {
		return false
	}
	if related.ExecutorPolicyVersion != old.ExecutorPolicyVersion || !reflect.DeepEqual(related.ExecutorPin, old.ExecutorPin) || related.CurrentAttemptLease != nil || related.Collaboration != nil || related.ToolPolicyVersion != old.ToolPolicyVersion || !reflect.DeepEqual(related.PluginSelection, old.PluginSelection) || !reflect.DeepEqual(related.PluginPins, old.PluginPins) {
		return false
	}
	link := related.TaskLink
	if link == nil || link.Version != 1 || link.Kind != c.Input.Action || link.ParentRequestID != old.RequestID || link.ParentMessageID != old.SourceMessageID || link.ParentEpoch != old.Epoch || link.ParentPinSHA256 != contextHash(old) || link.RequirementRevision != e.Work.TaskRevision || link.CommandNonce != c.Input.ClientNonce || related.Text != c.Input.Text || related.BotID != old.BotID || related.ConversationID != old.ConversationID || related.Lane != old.Lane || related.ClientNonce != controlWorkNonce(old.RequestID, c.Input.ClientNonce) || !reflect.DeepEqual(related.Account, old.Account) || related.Adapter != nil || related.TaskRevision != 0 {
		return false
	}
	scope := SubmissionScope{}
	if old.Resources != nil {
		scope.ProjectID = old.Resources.Project.ID
		scope.ProjectRevision = old.Resources.Project.Revision
		scope.FileIDs = old.Resources.RequestedFileIDs
	}
	if !resourceSelectionEqual(related.Resources, scope) {
		return false
	}
	wanted := "waiting_host"
	if c.Input.Action == "replace" {
		wanted = "waiting_control_stop"
	}
	if related.Status != wanted {
		return false
	}
	related.Status = "waiting_host"
	related.TaskLink = nil
	return validReplayWithArchive(s, Event{Sequence: e.Sequence, Type: "work.accepted", Work: related}, true)
}
func (s *Store) ReconcileTaskControls() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, current := range s.sortedWorkLocked() {
		if current.Status != "waiting_control_stop" || current.DesiredState != "running" || current.TaskLink == nil || current.TaskLink.Kind != "replace" {
			continue
		}
		parent, ok := s.work[current.TaskLink.ParentRequestID]
		if !ok || parent.DesiredState != "cancelled" || parent.Status != "cancelled" || !stoppedTask(parent) || !operationsComplete(parent) || !s.contextAllowed(current) {
			continue
		}
		release, err := s.controlAccountGuard(current)
		if err != nil {
			continue
		}
		w := copyWork(current)
		w.Status = "waiting_host"
		w.Epoch++
		e := Event{Type: "work.control_released", Work: w}
		if !s.validControlRelease(e) {
			release()
			return ErrConflict
		}
		err = s.commit(e)
		release()
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) validControlRelease(e Event) bool {
	if e.Type != "work.control_released" || e.TaskControl != nil || e.Collaboration != nil || e.Bot != nil || e.Channel != nil || e.Project != nil || e.Upload != nil || e.AdapterSession != nil || e.Nonce != "" || e.ResourceRevision != 0 || e.CompletionVersion != 0 || e.ContextRefreshVersion != 0 {
		return false
	}
	old, ok := s.work[e.Work.RequestID]
	if !ok || old.Status != "waiting_control_stop" || old.DesiredState != "running" || old.TaskLink == nil || old.TaskLink.Kind != "replace" || !s.contextAllowed(old) {
		return false
	}
	parent, ok := s.work[old.TaskLink.ParentRequestID]
	if !ok || parent.Status != "cancelled" || parent.DesiredState != "cancelled" || !stoppedTask(parent) || !operationsComplete(parent) || old.Epoch == ^uint64(0) {
		return false
	}
	old.Status = "waiting_host"
	old.Epoch++
	return reflect.DeepEqual(old, e.Work)
}

func copyTaskControl(c *TaskControlEvent) *TaskControlEvent {
	if c == nil {
		return nil
	}
	copy := *c
	if copy.Related != nil {
		w := copyWork(*copy.Related)
		copy.Related = &w
	}
	return &copy
}

func hasCollaborationMarker(w Work) bool {
	return w.Collaboration != nil
}
func controlWorkNonce(requestID, nonce string) string {
	sum := sha256.Sum256([]byte(requestID + "\x00" + nonce))
	return "control-" + hex.EncodeToString(sum[:])
}
