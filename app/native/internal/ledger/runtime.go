package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"nekobot.local/native/internal/protocol"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"
)

type CoreBinding struct {
	NativeProcessID uint32          `json:"nativeProcessId,omitempty"`
	SessionID       string          `json:"sessionId"`
	Task            json.RawMessage `json:"task"`
	Execution       json.RawMessage `json:"execution"`
	CoreSHA256      string          `json:"coreSha256"`
}
type Artifact struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
	Bytes    int    `json:"bytes"`
}
type Operation struct {
	Key      string          `json:"key"`
	Digest   string          `json:"digest"`
	Status   string          `json:"status"`
	Text     string          `json:"text"`
	Artifact Artifact        `json:"artifact"`
	Result   json.RawMessage `json:"result,omitempty"`
}

func copyWork(w Work) Work {
	if w.RuntimeAttemptBinding != nil {
		binding := *w.RuntimeAttemptBinding
		binding.Task = append(json.RawMessage(nil), binding.Task...)
		binding.Execution = append(json.RawMessage(nil), binding.Execution...)
		w.RuntimeAttemptBinding = &binding
	}
	if w.RuntimeStopReceipt != nil {
		receipt := *w.RuntimeStopReceipt
		w.RuntimeStopReceipt = &receipt
	}
	if w.ContextAccess != nil {
		access := *w.ContextAccess
		w.ContextAccess = &access
	}
	w.ContextAttempts = append([]ContextAttempt(nil), w.ContextAttempts...)
	for i := range w.ContextAttempts {
		if w.ContextAttempts[i].Core != nil {
			core := *w.ContextAttempts[i].Core
			core.Task = append(json.RawMessage(nil), core.Task...)
			core.Execution = append(json.RawMessage(nil), core.Execution...)
			w.ContextAttempts[i].Core = &core
		}
	}
	if w.Context != nil {
		pin := *w.Context
		pin.Sources = append([]ContextSource{}, pin.Sources...)
		for i := range pin.Sources {
			pin.Sources[i].Artifacts = append([]Artifact(nil), pin.Sources[i].Artifacts...)
		}
		w.Context = &pin
	}
	if w.Core != nil {
		core := *w.Core
		core.Task = append(json.RawMessage(nil), core.Task...)
		core.Execution = append(json.RawMessage(nil), core.Execution...)
		w.Core = &core
	}
	w.Artifacts = append([]Artifact(nil), w.Artifacts...)
	w.Operations = append([]Operation(nil), w.Operations...)
	for i := range w.Operations {
		w.Operations[i].Result = append(json.RawMessage(nil), w.Operations[i].Result...)
	}
	return w
}
func (s *Store) Get(id string) (Work, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	if !ok {
		return Work{}, ErrNotFound
	}
	return copyWork(w), nil
}
func (s *Store) Runtime(label string) { s.mu.Lock(); defer s.mu.Unlock(); s.runtime = label }
func (s *Store) update(id string, epoch uint64, typ string, change func(*Work) error) (Work, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	if !ok {
		return Work{}, ErrNotFound
	}
	if w.Epoch != epoch {
		return Work{}, ErrConflict
	}
	if (typ == "work.claimed" || typ == "work.admitted" || typ == "work.text" || typ == "work.completed") && !s.contextAllowed(w) {
		return Work{}, ErrConflict
	}
	w = copyWork(w)
	if err := change(&w); err != nil {
		return Work{}, err
	}
	e := Event{Type: typ, Work: w}
	if typ == "work.completed" {
		e.CompletionVersion = 1
	}
	if typ == "work.context_refreshed" {
		e.ContextRefreshVersion = 1
	}
	if !validRuntimeReplay(s, e) {
		return Work{}, ErrConflict
	}
	if err := s.commit(e); err != nil {
		return Work{}, err
	}
	return w, nil
}
func (s *Store) Claim(id string, epoch uint64) (Work, error) {
	attempt, err := newID("attempt-")
	if err != nil {
		return Work{}, err
	}
	return s.update(id, epoch, "work.claimed", func(w *Work) error {
		if w.Status != "waiting_host" || w.DesiredState != "running" {
			return ErrConflict
		}
		if w.Core == nil && !boundedRuntimePrompt(*w) {
			return ErrInvalid
		}
		w.Status = "starting"
		w.AttemptID = attempt
		w.RuntimeStopped = false
		w.RuntimeStopReceipt = nil
		w.RuntimeAttemptBinding = nil
		w.Failure = ""
		return nil
	})
}
func (s *Store) Admit(id string, epoch uint64, b CoreBinding) error {
	_, err := s.update(id, epoch, "work.admitted", func(w *Work) error {
		if w.Status != "starting" || len(b.SessionID) != 32 || !protocol.CoreRuntimeAllowed(b.CoreSHA256) || len(b.Task) == 0 || len(b.Execution) == 0 || (w.RuntimeAttemptBinding != nil && !reflect.DeepEqual(*w.RuntimeAttemptBinding, b)) {
			return ErrInvalid
		}
		w.Status = "running"
		w.Core = &b
		return nil
	})
	return err
}
func (s *Store) AppendText(id string, epoch uint64, text string) error {
	_, err := s.update(id, epoch, "work.text", func(w *Work) error {
		if w.Status != "running" || !utf8.ValidString(text) || len(w.Reply)+len(text) > 65536 {
			return ErrInvalid
		}
		w.Reply += text
		return nil
	})
	return err
}
func (s *Store) Finish(id string, epoch uint64, status, failure string) error {
	typ := "work." + status
	_, err := s.update(id, epoch, typ, func(w *Work) error {
		if status == "completed" {
			for _, op := range w.Operations {
				if op.Status != "completed" {
					return ErrConflict
				}
			}
		}
		w.Status = status
		w.Failure = failure
		return nil
	})
	return err
}
func (s *Store) RecoverInterrupted() error {
	if err := s.reconcileArtifacts(); err != nil {
		return err
	}
	s.mu.Lock()
	err := s.suspendInvalidContext()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	for _, w := range s.Snapshot().Work {
		if w.Status == "waiting_host" && w.Core == nil && (w.AttemptID == "" || w.RuntimeStopped) && !boundedRuntimePrompt(w) {
			if _, err := s.update(w.RequestID, w.Epoch, "work.prompt_blocked", func(current *Work) error {
				current.Status = "suspended"
				current.DesiredState = "suspended"
				current.Epoch++
				current.Failure = "context_prompt_oversized"
				return nil
			}); err != nil {
				return err
			}
		}
		if w.Status == "starting" || w.Status == "running" {
			if err := s.Finish(w.RequestID, w.Epoch, "recovery_required", "transport_interrupted"); err != nil {
				return err
			}
		}
		if w.Status == "cancelling" {
			if err := s.Finish(w.RequestID, w.Epoch, "cancelled", "outcome_not_rollback"); err != nil {
				return err
			}
		}
		if w.Status == "suspending" && w.RuntimeStopped {
			if err := s.Finish(w.RequestID, w.Epoch, "suspended", "context_permission_changed"); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Store) Requeue(id string) (Work, error) {
	w, err := s.Get(id)
	if err != nil {
		return Work{}, err
	}
	return s.update(id, w.Epoch, "work.requeued", func(w *Work) error {
		if w.Status != "recovery_required" || !s.contextAllowed(*w) {
			return ErrConflict
		}
		w.Status = "waiting_host"
		w.Epoch++
		w.Failure = ""
		return nil
	})
}
func validRuntimeReplay(s *Store, e Event) bool {
	old, ok := s.work[e.Work.RequestID]
	if !ok {
		return false
	}
	w := e.Work
	if (e.Type == "work.claimed" || e.Type == "work.admitted" || e.Type == "work.text" || e.Type == "work.operation_started" || e.Type == "work.completed" || e.Type == "work.requeued") && !s.contextAllowed(old) {
		return false
	}
	if w.OwnerUserID != old.OwnerUserID || w.SpaceID != old.SpaceID || w.BotID != old.BotID || w.Text != old.Text || w.ClientNonce != old.ClientNonce || w.SourceMessageID != old.SourceMessageID || w.ConversationID != old.ConversationID || w.CreatedSequence != old.CreatedSequence || w.CreatedAt != old.CreatedAt || w.BotConfigRevision != old.BotConfigRevision {
		return false
	}
	expected := copyWork(old)
	switch e.Type {
	case "work.claimed":
		if old.Status != "waiting_host" || old.DesiredState != "running" || w.Status != "starting" || !strings.HasPrefix(w.AttemptID, "attempt-") {
			return false
		}
		expected.Status = w.Status
		expected.AttemptID = w.AttemptID
		expected.RuntimeStopped = false
		expected.RuntimeStopReceipt = nil
		expected.RuntimeAttemptBinding = nil
		expected.Failure = ""
	case "work.admitted":
		if old.Status != "starting" || w.Status != "running" || w.Core == nil || (old.RuntimeAttemptBinding != nil && !reflect.DeepEqual(*old.RuntimeAttemptBinding, *w.Core)) {
			return false
		}
		expected.Status = w.Status
		expected.Core = w.Core
	case "work.text":
		if old.Status != "running" || w.Status != "running" || !strings.HasPrefix(w.Reply, old.Reply) || len(w.Reply) > 65536 {
			return false
		}
		expected.Reply = w.Reply
	case "work.operation_started":
		if old.Status != "running" || len(w.Operations) != len(old.Operations)+1 || len(w.Operations) > 8 || !reflect.DeepEqual(append([]Operation(nil), w.Operations[:len(old.Operations)]...), old.Operations) {
			return false
		}
		added := w.Operations[len(w.Operations)-1]
		if added.Status != "started" || len(added.Key) != 65 || len(added.Digest) != 64 || added.Result != nil || !artifactMatches(added.Artifact, []byte(added.Text)) {
			return false
		}
		expected.Operations = w.Operations
	case "work.artifact":
		if !reconcilable(old.Status) || len(w.Artifacts) != len(old.Artifacts)+1 || len(w.Operations) != len(old.Operations) || !reflect.DeepEqual(append([]Artifact(nil), w.Artifacts[:len(old.Artifacts)]...), old.Artifacts) {
			return false
		}
		changes := 0
		for i, op := range w.Operations {
			previous := old.Operations[i]
			if reflect.DeepEqual(previous, op) {
				continue
			}
			if previous.Status != "started" || op.Status != "completed" || op.Artifact != w.Artifacts[len(w.Artifacts)-1] || len(op.Result) == 0 {
				return false
			}
			previous.Status = op.Status
			previous.Result = op.Result
			if !reflect.DeepEqual(previous, op) {
				return false
			}
			changes++
		}
		if changes != 1 {
			return false
		}
		expected.Artifacts = w.Artifacts
		expected.Operations = w.Operations
	case "work.operation_unknown":
		if !reconcilable(old.Status) || len(w.Operations) != len(old.Operations) {
			return false
		}
		changes := 0
		for i, op := range w.Operations {
			previous := old.Operations[i]
			if reflect.DeepEqual(previous, op) {
				continue
			}
			if previous.Status != "started" || op.Status != "unknown" {
				return false
			}
			previous.Status = op.Status
			if !reflect.DeepEqual(previous, op) {
				return false
			}
			changes++
		}
		if changes != 1 {
			return false
		}
		expected.Operations = w.Operations
	case "work.completed":
		if old.Status != "running" || w.Status != "completed" {
			return false
		}
		if e.CompletionVersion == 1 {
			for _, op := range old.Operations {
				if op.Status != "completed" {
					return false
				}
			}
		}
		expected.Status = w.Status
		expected.Failure = w.Failure
	case "work.recovery_required":
		if (old.Status != "running" && old.Status != "starting") || w.Status != "recovery_required" {
			return false
		}
		expected.Status = w.Status
		expected.Failure = w.Failure
	case "work.requeued":
		if old.Status != "recovery_required" || w.Status != "waiting_host" || w.Epoch != old.Epoch+1 {
			return false
		}
		expected.Status = w.Status
		expected.Epoch = w.Epoch
		expected.Failure = ""
	case "work.cancelling":
		if (old.Status != "running" && old.Status != "starting" && old.Status != "suspending") || w.Status != "cancelling" || w.DesiredState != "cancelled" || w.Epoch != old.Epoch+1 {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
	case "work.cancelled":
		if (old.Status != "cancelling" && old.Status != "recovery_required" && old.Status != "suspended" && old.Status != "suspending") || w.Status != "cancelled" {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Failure = w.Failure
	case "work.source_access":
		if w.ContextAccess == nil || w.ContextAccess.Revision != accessRevision(old)+1 || !validAccessNonce(w.ContextAccess.Nonce) || accessRevision(old) == ^uint64(0) {
			return false
		}
		expected.ContextAccess = w.ContextAccess
	case "work.suspending", "work.suspended":
		if !validSuspension(s, old, w, e.Type) {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Failure = w.Failure
	case "work.runtime_bound":
		if old.AttemptID == "" || old.RuntimeStopped || old.RuntimeAttemptBinding != nil || w.RuntimeAttemptBinding == nil || !validAttemptBinding(old, *w.RuntimeAttemptBinding) {
			return false
		}
		expected.RuntimeAttemptBinding = w.RuntimeAttemptBinding
	case "work.runtime_stopped":
		if w.RuntimeStopped != true || old.RuntimeStopped || old.AttemptID == "" || w.RuntimeStopReceipt == nil || !validStoppedReceipt(old, *w.RuntimeStopReceipt) {
			return false
		}
		expected.RuntimeStopped = true
		expected.RuntimeStopReceipt = w.RuntimeStopReceipt
	case "work.prompt_blocked":
		if old.Status != "waiting_host" || old.Core != nil || (old.AttemptID != "" && !old.RuntimeStopped) || boundedRuntimePrompt(old) || w.Status != "suspended" || w.DesiredState != "suspended" || w.Epoch != old.Epoch+1 || w.Failure != "context_prompt_oversized" {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Failure = w.Failure
	case "work.context_refreshed":
		if !s.validContextRefresh(old, w, e.ContextRefreshVersion) {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Context = w.Context
		expected.Core = nil
		expected.Reply = ""
		expected.ContextRevision = w.ContextRevision
		expected.ContextAttempts = w.ContextAttempts
		expected.Failure = ""
	default:
		return false
	}
	return reflect.DeepEqual(expected, w)
}
func toolError(label string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"isError": true, "content": []any{map[string]string{"type": "text", "text": label}}})
	return b
}
func (s *Store) ExecuteTool(id string, epoch uint64, frame json.RawMessage) (json.RawMessage, error) {
	canonical, err := protocol.Canonical(frame)
	if err != nil {
		return toolError("Invalid tool request"), nil
	}
	var call struct {
		Name      string `json:"name"`
		Arguments struct {
			Name      string `json:"name"`
			Arguments struct {
				Filename string `json:"filename"`
				Text     string `json:"text"`
			} `json:"arguments"`
		} `json:"arguments"`
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(canonical)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&call) != nil || call.Name != "execute_tool" || call.Arguments.Name != "artifact.write_text" {
		return toolError("Action denied by host"), nil
	}
	filename, text := call.Arguments.Arguments.Filename, call.Arguments.Arguments.Text
	if len(filename) == 0 || len(filename) > 80 || filepath.Base(filename) != filename || strings.ContainsAny(filename, "/\\:\x00\r\n") || (!strings.HasSuffix(filename, ".md") && !strings.HasSuffix(filename, ".txt")) || len(text) == 0 || len(text) > 24000 || !utf8.ValidString(text) {
		return toolError("Invalid report filename or content"), nil
	}
	var op struct {
		Version         int    `json:"version"`
		OperationID     string `json:"operationId"`
		SessionID       string `json:"sessionId"`
		TaskID          string `json:"taskId"`
		Root            string `json:"root"`
		ActivationEpoch uint64 `json:"activationEpoch"`
		ActivationID    string `json:"activationId"`
	}
	opDecoder := json.NewDecoder(strings.NewReader(string(call.Meta["neko.operation"])))
	opDecoder.DisallowUnknownFields()
	if len(call.Meta) != 1 || opDecoder.Decode(&op) != nil || op.Version != 1 || !lowerHex(op.OperationID, 32) {
		return toolError("Missing canonical operation"), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	if !ok || w.Epoch != epoch || w.Status != "running" || !s.contextAllowed(w) || w.Core == nil || w.Core.SessionID != op.SessionID {
		return nil, ErrConflict
	}
	w = copyWork(w)
	var task struct {
		ID              string `json:"id"`
		Root            string `json:"root"`
		ActivationEpoch uint64 `json:"activationEpoch"`
		ActivationID    string `json:"activationId"`
	}
	if json.Unmarshal(w.Core.Task, &task) != nil || op.TaskID != task.ID || op.Root != task.Root || op.ActivationEpoch != task.ActivationEpoch || op.ActivationID != task.ActivationID {
		return nil, ErrConflict
	}
	h := sha256.Sum256(canonical)
	digest := hex.EncodeToString(h[:])
	key := op.SessionID + ":" + op.OperationID
	for _, previous := range w.Operations {
		if previous.Key == key {
			if previous.Digest != digest {
				return nil, ErrConflict
			}
			if previous.Status == "completed" {
				return previous.Result, nil
			}
			return toolError("Operation outcome requires reconciliation; not repeated"), nil
		}
	}
	for _, previous := range w.Operations {
		if previous.Status != "completed" {
			return toolError("An earlier effect has an unresolved outcome; new mutations are blocked"), nil
		}
	}
	if len(w.Operations) >= 8 {
		return toolError("Task artifact limit reached"), nil
	}
	contentHash := sha256.Sum256([]byte(text))
	identityHash := sha256.Sum256([]byte(id + ":" + key))
	artifact := Artifact{ID: hex.EncodeToString(identityHash[:]), Filename: filename, SHA256: hex.EncodeToString(contentHash[:]), Bytes: len(text)}
	operation := Operation{Key: key, Digest: digest, Status: "started", Text: text, Artifact: artifact}
	w.Operations = append(w.Operations, operation)
	if err = s.commit(Event{Type: "work.operation_started", Work: w}); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.dir, "artifacts")
	if os.MkdirAll(dir, 0700) != nil {
		return nil, ErrUnavailable
	}
	path := filepath.Join(dir, artifact.ID)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, ErrUnavailable
	}
	_, err = file.WriteString(text)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return nil, ErrUnavailable
	}
	result := artifactResult(artifact)
	w = copyWork(w)
	w.Operations[len(w.Operations)-1].Status = "completed"
	w.Operations[len(w.Operations)-1].Result = result
	w.Artifacts = append(w.Artifacts, artifact)
	if err = s.commit(Event{Type: "work.artifact", Work: w}); err != nil {
		return nil, err
	}
	return result, nil
}
func (s *Store) ReadArtifact(id string) (Artifact, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.work {
		for _, a := range w.Artifacts {
			if a.ID == id {
				b, err := os.ReadFile(filepath.Join(s.dir, "artifacts", a.ID))
				if err != nil {
					return Artifact{}, nil, ErrUnavailable
				}
				if !artifactMatches(a, b) {
					return Artifact{}, nil, ErrCorrupt
				}
				return a, b, nil
			}
		}
	}
	return Artifact{}, nil, ErrNotFound
}
func lowerHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func artifactMatches(a Artifact, b []byte) bool {
	h := sha256.Sum256(b)
	return lowerHex(a.ID, 64) && len(b) == a.Bytes && hex.EncodeToString(h[:]) == a.SHA256
}
func artifactResult(a Artifact) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"content": []any{map[string]string{"type": "text", "text": fmt.Sprintf("Saved artifact %s (%d bytes), SHA256 %s", a.Filename, a.Bytes, a.SHA256)}}})
	return b
}
func (s *Store) reconcileArtifacts() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, original := range s.work {
		w := copyWork(original)
		for i, operation := range w.Operations {
			if operation.Status != "started" {
				continue
			}
			if !reconcilable(w.Status) {
				return ErrCorrupt
			}
			b, err := os.ReadFile(filepath.Join(s.dir, "artifacts", operation.Artifact.ID))
			typ := "work.operation_unknown"
			if err == nil && artifactMatches(operation.Artifact, b) {
				w.Operations[i].Status = "completed"
				w.Operations[i].Result = artifactResult(operation.Artifact)
				w.Artifacts = append(w.Artifacts, operation.Artifact)
				typ = "work.artifact"
			} else {
				w.Operations[i].Status = "unknown"
			}
			if err = s.commit(Event{Type: typ, Work: w}); err != nil {
				return err
			}
			w = copyWork(s.work[id])
		}
	}
	return nil
}

func reconcilable(status string) bool {
	return status == "waiting_host" || status == "starting" || status == "running" || status == "cancelling" || status == "cancelled" || status == "recovery_required" || status == "completed" || status == "suspending" || status == "suspended"
}
