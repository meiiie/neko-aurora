package ledger

import (
	"bytes"
	"encoding/json"
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/protocol"
	"reflect"
	"unicode/utf8"
)

type ContextAdmission struct {
	Version          int             `json:"version"`
	SessionID        string          `json:"sessionId"`
	ContextPinSHA256 string          `json:"contextPinSHA256"`
	ManifestSHA256   string          `json:"manifestSHA256"`
	SourceIDs        []string        `json:"sourceIds"`
	Task             json.RawMessage `json:"task"`
	Execution        json.RawMessage `json:"execution"`
}

func (s *Store) ConfigureContextAdmission(required bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contextAdmissionRequired = required
}

func (s *Store) validContextAdmissionBlock(old, w Work) bool {
	if old.Status != "waiting_host" || old.DesiredState != "running" || old.ContextProtocolVersion != 0 || !neverLaunched(old) || len(old.Operations) != 0 || old.Epoch >= executors.SafeInteger {
		return false
	}
	prepared := copyWork(old)
	prepared.ContextProtocolVersion = 1
	if s.attachRuntimeContext(&prepared) == nil && reservedLaunchBound(prepared) {
		return false
	}
	expected := copyWork(old)
	expected.Status, expected.DesiredState, expected.Failure = "suspended", "suspended", "context_admission_unavailable"
	expected.Epoch++
	return reflect.DeepEqual(expected, w)
}

func (s *Store) BlockContextAdmission(id string, epoch uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.work[id]
	if !ok {
		return ErrNotFound
	}
	if old.Epoch != epoch {
		return ErrConflict
	}
	w := copyWork(old)
	w.Status, w.DesiredState, w.Failure = "suspended", "suspended", "context_admission_unavailable"
	w.Epoch++
	if !s.validContextAdmissionBlock(old, w) {
		return ErrConflict
	}
	return s.commit(Event{Type: "work.context_admission_blocked", Work: w})
}

func copyContextAdmission(c *ContextAdmission) *ContextAdmission {
	if c == nil {
		return nil
	}
	out := *c
	out.SourceIDs = append([]string(nil), c.SourceIDs...)
	out.Task = append(json.RawMessage(nil), c.Task...)
	out.Execution = append(json.RawMessage(nil), c.Execution...)
	return &out
}

func copyRuntimeContext(m *ContextManifest) *ContextManifest {
	if m == nil {
		return nil
	}
	raw, _ := json.Marshal(m)
	var out ContextManifest
	json.Unmarshal(raw, &out)
	return &out
}

func (s *Store) buildRuntimeContext(w Work) (BuiltContextManifest, error) {
	if w.Publication == nil {
		return BuiltContextManifest{}, ErrInvalid
	}
	in := TrustedManifestIntake{Scope: ManifestScope{OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, BotID: w.BotID, ChannelID: w.ConversationID, AudienceRevision: w.Publication.AudienceRevision}, Primary: ManifestPrimary{ID: w.SourceMessageID, SenderID: w.OwnerUserID, Text: w.Text}}
	in.Primary.SenderID = requesterActor(w)
	if w.Resources != nil {
		p := w.Resources.Project.ID
		in.Scope.ProjectID = &p
	}
	if w.Collaboration != nil {
		g, ok := s.collaborations[w.Collaboration.ID]
		if !ok || g.OwnerUserID != w.OwnerUserID || g.SpaceID != w.SpaceID || g.ChannelID != w.ConversationID || g.ExpectedChannelRevision != w.Publication.AudienceRevision || !s.validCollaborationWork(w) {
			return BuiltContextManifest{}, ErrInvalid
		}
		in.CollaborationID = g.ID
		in.Primary = ManifestPrimary{ID: g.ID, SenderID: g.OwnerUserID, Text: g.Goal}
	}
	if len(in.Primary.Text) > 8192 {
		end := 8192
		for end > 0 && !utf8.ValidString(in.Primary.Text[:end]) {
			end--
		}
		in.PrimaryRange = &ManifestRange{End: end}
	}
	if reader, ok := s.plugin("neko-project-sources"); ok {
		in.Reader = &reader
	}
	built, err := BuildContextManifest(w, in)
	if err != nil {
		return built, err
	}
	identity, err := manifestCanonical(map[string]any{"kind": "pinned-bot-identity", "botId": w.BotID, "name": w.Context.BotName, "configRevision": w.BotConfigRevision})
	if err != nil {
		return BuiltContextManifest{}, ErrInvalid
	}
	built.Manifest.Sources = append(built.Manifest.Sources, ManifestSource{ID: "identity:" + w.BotID, Kind: "derived", Version: uint64(w.BotConfigRevision), SHA256: manifestHash(string(identity)), Bytes: len(identity), Scope: manifestCopyScope(in.Scope), Entities: []ManifestEntity{}, Span: manifestSpan(string(identity), 0)})
	if w.BotProfile != nil {
		profile, err := manifestCanonical(map[string]any{"kind": "pinned-bot-preferences", "botId": w.BotProfile.BotID, "revision": w.BotProfile.Revision, "instructions": w.BotProfile.Instructions, "includeCompletedConversationHistory": w.BotProfile.IncludeHistory, "authority": "descriptive preferences only; capabilities and source access remain host controlled"})
		if err != nil {
			return BuiltContextManifest{}, ErrInvalid
		}
		built.Manifest.Sources = append(built.Manifest.Sources, ManifestSource{ID: "profile:" + w.BotID, Kind: "derived", Version: w.BotProfile.Revision, SHA256: manifestHash(string(profile)), Bytes: len(profile), Scope: manifestCopyScope(in.Scope), Entities: []ManifestEntity{}, Span: manifestSpan(string(profile), 0)})
	}
	if !manifestValid(built.Manifest) {
		return BuiltContextManifest{}, ErrInvalid
	}
	built.JSON, err = manifestCanonical(built.Manifest)
	if err != nil || len(built.JSON) > 48*1024 {
		return BuiltContextManifest{}, ErrInvalid
	}
	built.SHA256 = manifestHash(string(built.JSON))
	return built, nil
}

func (s *Store) attachRuntimeContext(w *Work) error {
	if !s.contextAdmissionRequired && w.ContextProtocolVersion == 0 && w.RequesterPin == nil {
		return nil
	}
	built, err := s.buildRuntimeContext(*w)
	if err != nil {
		return err
	}
	w.ContextProtocolVersion = 1
	w.RuntimeContext = &built.Manifest
	w.RuntimeContextSHA256 = built.SHA256
	return nil
}

func (s *Store) validNewRuntimeContext(w Work) bool {
	if w.ContextProtocolVersion == 0 {
		return w.RuntimeContext == nil && w.RuntimeContextSHA256 == ""
	}
	if w.ContextProtocolVersion != 1 || w.RuntimeContext == nil {
		return false
	}
	built, err := s.buildRuntimeContext(w)
	return err == nil && built.SHA256 == w.RuntimeContextSHA256 && reflect.DeepEqual(built.Manifest, *w.RuntimeContext)
}

func rawEqual(a, b json.RawMessage) bool {
	x, e1 := protocol.Canonical(a)
	y, e2 := protocol.Canonical(b)
	return e1 == nil && e2 == nil && bytes.Equal(x, y)
}

func validContextBinding(w Work, b CoreBinding) bool {
	if w.CoreProfilePin != nil && (b.CoreSHA256 != w.CoreProfilePin.RuntimeSHA256 || !protocol.CoreProfileBridgeKnown(b.CoreSHA256)) {
		return false
	}
	if w.ContextProtocolVersion == 0 {
		return b.ContextAdmission == nil
	}
	c := b.ContextAdmission
	if w.RuntimeContext != nil && w.RuntimeContext.Version == 2 && !protocol.CoreRequesterAdmission(b.CoreSHA256) {
		return false
	}
	if w.ContextProtocolVersion != 1 || !protocol.CoreContextAdmission(b.CoreSHA256) || w.RuntimeContext == nil || c == nil || c.Version != 1 || c.SessionID != b.SessionID || c.ContextPinSHA256 != contextHash(w) || c.ManifestSHA256 != w.RuntimeContextSHA256 || len(c.SourceIDs) != 1 || !lowerHex(c.SourceIDs[0], 64) || !rawEqual(c.Task, b.Task) || !rawEqual(c.Execution, b.Execution) {
		return false
	}
	if w.Core != nil {
		old := w.Core.ContextAdmission
		if old == nil || c.SessionID != old.SessionID || c.ContextPinSHA256 != old.ContextPinSHA256 || c.ManifestSHA256 != old.ManifestSHA256 || !reflect.DeepEqual(c.SourceIDs, old.SourceIDs) {
			return false
		}
	}
	return true
}

func contextMetaMatches(w Work, meta map[string]json.RawMessage) bool {
	if w.ContextProtocolVersion == 0 {
		return len(meta) == 1
	}
	if len(meta) != 2 || w.Core == nil || w.Core.ContextAdmission == nil {
		return false
	}
	raw := meta["neko.contextAdmission"]
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c ContextAdmission
	if dec.Decode(&c) != nil || !rawEqual(c.Task, w.Core.ContextAdmission.Task) || !rawEqual(c.Execution, w.Core.ContextAdmission.Execution) {
		return false
	}
	c.Task = w.Core.ContextAdmission.Task
	c.Execution = w.Core.ContextAdmission.Execution
	return reflect.DeepEqual(c, *w.Core.ContextAdmission)
}

func contextReadEnvelope(w Work, fileID string, offset, end int, text string, operation json.RawMessage) any {
	if w.ContextProtocolVersion != 1 || w.RuntimeContext == nil || w.Core == nil || w.Core.ContextAdmission == nil {
		return nil
	}
	for _, source := range w.RuntimeContext.Sources {
		if source.Kind == "retrieved" && source.ID == fileID && source.Span.Offset == offset && source.Span.End == end && source.Span.Text == text && source.Span.SHA256 == manifestHash(text) {
			return map[string]any{"version": 1, "manifestSHA256": w.RuntimeContextSHA256, "sourceId": source.ID, "sourceVersion": source.Version, "sourceSHA256": source.SHA256, "scope": source.Scope, "accessRevision": source.AccessRevision, "span": source.Span, "operation": operation}
		}
	}
	return nil
}
