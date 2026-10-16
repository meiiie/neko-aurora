package ledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"nekobot.local/native/internal/protocol"
)

type coreProfileToolPermissionFrame struct {
	Version          int                            `json:"version"`
	SessionID        string                         `json:"sessionId"`
	ToolCallID       string                         `json:"toolCallId"`
	Name             string                         `json:"name"`
	Arguments        coreProfilePermissionArguments `json:"arguments"`
	Task             json.RawMessage                `json:"task"`
	Execution        json.RawMessage                `json:"execution"`
	ContextAdmission *ContextAdmission              `json:"contextAdmission"`
}

type coreProfilePermissionArguments struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type coreProfileToolPermissionDecision struct {
	Version       int    `json:"version"`
	Allowed       bool   `json:"allowed"`
	RequestSHA256 string `json:"requestSha256"`
}

func profilePermissionID(value string) bool {
	return len(value) > 0 && len(value) <= 128 && utf8.ValidString(value) && !strings.ContainsRune(value, '\uFFFD') && strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) < 0
}

func profilePermissionRequestHash(arguments coreProfilePermissionArguments) string {
	raw, err := json.Marshal(struct {
		Name      string                         `json:"name"`
		Arguments coreProfilePermissionArguments `json:"arguments"`
	}{"execute_tool", arguments})
	if err != nil {
		return ""
	}
	canonical, err := protocol.Canonical(raw)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func permissionReadSpan(w Work, id string, offset, size int) bool {
	if w.RuntimeContext == nil || offset < 0 || size < 1 || size > 4000 {
		return false
	}
	for _, source := range w.RuntimeContext.Sources {
		if source.Kind == "retrieved" && source.ID == id && source.Span.Offset == offset && source.Span.End > offset && source.Span.End <= offset+size && len([]byte(source.Span.Text)) <= size {
			return true
		}
	}
	return false
}

func permissionExactFields(raw json.RawMessage, names ...string) bool {
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil || len(values) != len(names) {
		return false
	}
	for _, name := range names {
		value, ok := values[name]
		if !ok || bytes.Equal(value, []byte("null")) {
			return false
		}
	}
	return true
}

func (s *Store) profilePermissionArgumentsAllowed(w Work, a coreProfilePermissionArguments) bool {
	if !pinnedTool(w, a.Name) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(a.Arguments))
	decoder.DisallowUnknownFields()
	switch a.Name {
	case "artifact.write_text":
		if !permissionExactFields(a.Arguments, "filename", "text") {
			return false
		}
		var in struct {
			Filename string `json:"filename"`
			Text     string `json:"text"`
		}
		if decoder.Decode(&in) != nil || !stageWriteAllowed(w) || len(w.Operations) >= 8 || !operationsComplete(w) {
			return false
		}
		return len(in.Filename) > 0 && len(in.Filename) <= 80 && filepath.Base(in.Filename) == in.Filename && !strings.ContainsAny(in.Filename, "/\\:\x00\r\n") && (strings.HasSuffix(in.Filename, ".md") || strings.HasSuffix(in.Filename, ".txt")) && len(in.Text) > 0 && len(in.Text) <= 24000 && utf8.ValidString(in.Text) && !hasSecret([]byte(in.Text))
	case "project.read_file":
		if !permissionExactFields(a.Arguments, "fileId", "offset", "bytes") {
			return false
		}
		var in struct {
			FileID string `json:"fileId"`
			Offset int    `json:"offset"`
			Bytes  int    `json:"bytes"`
		}
		if decoder.Decode(&in) != nil || !resourceID(in.FileID, "file-") || w.Resources == nil || !permissionReadSpan(w, in.FileID, in.Offset, in.Bytes) {
			return false
		}
		for _, file := range w.Resources.Files {
			if file.ID == in.FileID {
				return true
			}
		}
	case "report.read_artifact":
		if !permissionExactFields(a.Arguments, "deliveryId", "artifactId", "offset", "bytes") {
			return false
		}
		var in struct {
			DeliveryID string `json:"deliveryId"`
			ArtifactID string `json:"artifactId"`
			Offset     int    `json:"offset"`
			Bytes      int    `json:"bytes"`
		}
		if decoder.Decode(&in) != nil || !resourceID(in.DeliveryID, "report-delivery-") || !lowerHex(in.ArtifactID, 64) || !hasPublishedArtifacts(w) || !permissionReadSpan(w, in.ArtifactID, in.Offset, in.Bytes) {
			return false
		}
		for _, pin := range w.Context.PublishedReports {
			if pin.DeliveryID == in.DeliveryID && s.publishedReportAllowed(w, pin, w.Context.AtSequence) {
				for _, artifact := range pin.Artifacts {
					if artifact.ID == in.ArtifactID {
						return true
					}
				}
			}
		}
	}
	return false
}

func (s *Store) profilePermissionExecutorCurrent(w Work) bool {
	return s.executorRequired && s.executor != nil && w.ExecutorPolicyVersion == 1 && validExecutorWork(w) && w.ExecutorPin != nil && w.CurrentAttemptLease != nil && w.CurrentAttemptLease.WorkEpoch == w.Epoch && w.CurrentAttemptLease.AuthoritySHA256 == ExecutorAuthorityHash(w) && s.executor.CheckLease(*w.ExecutorPin, *w.CurrentAttemptLease) == nil
}

func (s *Store) profilePermissionCurrent(w Work) bool {
	if s.closed || s.poisoned || s.readOnly || s.coreProfiles == nil || w.Status != "running" || w.DesiredState != "running" || w.CoreProfilePin == nil || w.CoreProfileSelection == nil || w.Core == nil || w.RuntimeStopped || w.RuntimeAttemptBinding == nil || !equalNativeBinding(*w.Core, *w.RuntimeAttemptBinding) || w.Core.NativeProcessID == 0 || !ValidProcessGeneration(w.Core.NativeProcessGeneration) || w.ContextProtocolVersion != 1 || w.Core.ContextAdmission == nil || w.Core.CoreSHA256 != w.CoreProfilePin.RuntimeSHA256 || w.Account != nil || w.RequesterPin != nil || s.identity != nil || !s.contextAllowed(w) || !s.profilePermissionExecutorCurrent(w) {
		return false
	}
	if w.CoreProfileGroupBinding == nil {
		return w.Collaboration == nil && s.coreProfileCurrent(w)
	}
	if w.Collaboration == nil {
		return false
	}
	g, ok := s.collaborations[w.Collaboration.ID]
	stage := w.Collaboration.Stage
	return ok && g.Status == "running" && stage > 0 && stage <= len(g.RequestIDs) && g.RequestIDs[stage-1] == w.RequestID && s.validGroupProfileWork(w, g) && s.groupProfileCurrent(g)
}

func (s *Store) PreflightCoreProfileTool(id string, epoch uint64, frame json.RawMessage) json.RawMessage {
	decision := coreProfileToolPermissionDecision{Version: 1}
	encode := func() json.RawMessage { raw, _ := json.Marshal(decision); return raw }
	canonical, err := protocol.Canonical(frame)
	if err != nil {
		return encode()
	}
	if !permissionExactFields(canonical, "version", "sessionId", "toolCallId", "name", "arguments", "task", "execution", "contextAdmission") {
		return encode()
	}
	var rawFields map[string]json.RawMessage
	if json.Unmarshal(canonical, &rawFields) != nil || !permissionExactFields(rawFields["arguments"], "name", "arguments") {
		return encode()
	}
	var input coreProfileToolPermissionFrame
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.Version != 1 || input.Name != "mcp__nekobot__execute_tool" || !profilePermissionID(input.ToolCallID) || input.ContextAdmission == nil {
		return encode()
	}
	decision.RequestSHA256 = profilePermissionRequestHash(input.Arguments)
	if decision.RequestSHA256 == "" {
		return encode()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	if !ok || w.Epoch != epoch || !s.profilePermissionCurrent(w) || input.SessionID != w.Core.SessionID || !rawEqual(input.Task, w.Core.Task) || !rawEqual(input.Execution, w.Core.Execution) || !reflect.DeepEqual(input.ContextAdmission, w.Core.ContextAdmission) || !s.profilePermissionArgumentsAllowed(w, input.Arguments) {
		return encode()
	}
	old := copyWork(w)
	var groupBefore *Collaboration
	if w.Collaboration != nil {
		value := copyCollaboration(s.collaborations[w.Collaboration.ID])
		groupBefore = &value
	}
	service := s.coreProfiles
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.mu.Unlock()
	err = service.Verify(ctx, *w.CoreProfilePin)
	s.mu.Lock()
	current, exists := s.work[id]
	if err != nil || !exists || !reflect.DeepEqual(current, old) || !s.profilePermissionCurrent(current) || !s.profilePermissionArgumentsAllowed(current, input.Arguments) {
		return encode()
	}
	if groupBefore != nil {
		group, exists := s.collaborations[groupBefore.ID]
		if !exists || !reflect.DeepEqual(group, *groupBefore) {
			return encode()
		}
	}
	decision.Allowed = true
	return encode()
}
