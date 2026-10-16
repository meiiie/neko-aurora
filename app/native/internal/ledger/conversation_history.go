package ledger

import (
	"reflect"
	"sort"

	"nekobot.local/native/internal/historyexport"
	"nekobot.local/native/internal/retentionplan"
)

type RetentionPreference struct {
	ConversationID string `json:"conversationId"`
	Revision       uint64 `json:"revision"`
	Days           int    `json:"days"`
}
type RetentionInput struct {
	Days             int    `json:"days"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	ClientNonce      string `json:"clientNonce"`
}
type RetentionPreview struct {
	retentionplan.Preview
	ConversationID string `json:"conversationId"`
	AtSequence     uint64 `json:"atSequence"`
}

func (s *Store) validRetentionEvent(e Event) bool {
	if e.RetentionPreference == nil || e.RetentionInput == nil {
		return false
	}
	p, in := *e.RetentionPreference, *e.RetentionInput
	expected := Event{Sequence: e.Sequence, Type: "retention.preference_saved", RetentionPreference: &p, RetentionInput: &in, Nonce: in.ClientNonce}
	if !reflect.DeepEqual(e, expected) || !validAccessNonce(in.ClientNonce) || len(s.retentionNonces) >= 512 || in.Days < 0 || in.Days > 3650 || in.ExpectedRevision >= 1<<53-1 {
		return false
	}
	if _, ok := s.retentionNonces[in.ClientNonce]; ok {
		return false
	}
	if _, ok := s.conversationNonces[in.ClientNonce]; ok {
		return false
	}
	if _, ok := s.conversation(p.ConversationID); !ok {
		return false
	}
	old := s.retentionPreferences[p.ConversationID]
	return old.Revision == in.ExpectedRevision && p.Revision == old.Revision+1 && p.Days == in.Days
}
func (s *Store) SetRetentionPreference(id string, in RetentionInput) (RetentionPreference, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return RetentionPreference{}, false, ErrUnavailable
	}
	if !validAccessNonce(in.ClientNonce) || in.Days < 0 || in.Days > 3650 || in.ExpectedRevision >= 1<<53-1 {
		return RetentionPreference{}, false, ErrInvalid
	}
	if e, ok := s.retentionNonces[in.ClientNonce]; ok {
		if e.RetentionPreference.ConversationID != id || *e.RetentionInput != in {
			return RetentionPreference{}, false, ErrConflict
		}
		return s.retentionPreferences[id], true, nil
	}
	if _, ok := s.conversationNonces[in.ClientNonce]; ok {
		return RetentionPreference{}, false, ErrConflict
	}
	if _, ok := s.conversation(id); !ok {
		return RetentionPreference{}, false, ErrNotFound
	}
	old := s.retentionPreferences[id]
	if old.Revision != in.ExpectedRevision {
		return RetentionPreference{}, false, ErrConflict
	}
	if len(s.retentionNonces) >= 512 {
		return RetentionPreference{}, false, ErrUnavailable
	}
	p := RetentionPreference{ConversationID: id, Revision: old.Revision + 1, Days: in.Days}
	e := Event{Type: "retention.preference_saved", RetentionPreference: &p, RetentionInput: &in, Nonce: in.ClientNonce}
	if !s.validRetentionEvent(e) {
		return RetentionPreference{}, false, ErrInvalid
	}
	if err := s.commit(e); err != nil {
		return RetentionPreference{}, false, err
	}
	return p, false, nil
}
func (s *Store) ExportConversation(id, format string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return nil, ErrUnavailable
	}
	c, ok := s.conversation(id)
	if !ok {
		return nil, ErrNotFound
	}
	h := historyexport.History{Version: 1, ConversationID: id, Title: c.Title, BotID: c.BotID, Archived: c.Archived, AtSequence: uint64(len(s.events)), Messages: []historyexport.Message{}}
	for _, w := range s.sortedWorkLocked() {
		if w.ConversationID != id || w.OwnerUserID != "local-owner" || w.SpaceID != "local-space" || !s.publicationAllowed(w) || w.Collaboration != nil {
			continue
		}
		m := historyexport.Message{RequestID: w.RequestID, BotID: w.BotID, CreatedAt: w.CreatedAt, Text: w.Text, Reply: w.Reply, Status: w.Status, Files: []historyexport.File{}}
		if s.contextAllowed(w) && s.resourceAllowed(w) {
			for _, f := range w.Artifacts {
				m.Files = append(m.Files, historyexport.File{ID: f.ID, Name: f.Filename, SHA256: f.SHA256, Bytes: f.Bytes, DownloadPath: "/v1/channels/" + id + "/artifacts/" + f.ID})
			}
		}
		h.Messages = append(h.Messages, m)
	}
	if sensitiveHistory(h) {
		return nil, ErrInvalid
	}
	raw, err := historyexport.Encode(h, format)
	if err != nil || hasSecret(raw) {
		return nil, ErrInvalid
	}
	return raw, nil
}
func (s *Store) PreviewRetention(id string, days int) (RetentionPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return RetentionPreview{}, ErrUnavailable
	}
	if _, ok := s.conversation(id); !ok {
		return RetentionPreview{}, ErrNotFound
	}
	referenced := map[string]bool{}
	for _, w := range s.work {
		if w.Context != nil {
			for _, src := range w.Context.Sources {
				referenced[src.RequestID] = true
			}
		}
		if w.TaskLink != nil {
			referenced[w.TaskLink.ParentRequestID] = true
			referenced[w.RequestID] = true
		}
	}
	remember := func(w Work) {
		if w.Context != nil {
			for _, source := range w.Context.Sources {
				referenced[source.RequestID] = true
			}
		}
		if w.TaskLink != nil {
			referenced[w.TaskLink.ParentRequestID] = true
			referenced[w.RequestID] = true
		}
	}
	for _, event := range s.events {
		remember(event.Work)
		if event.TaskControl != nil && event.TaskControl.Related != nil {
			remember(*event.TaskControl.Related)
		}
	}
	items := []retentionplan.Item{}
	for _, w := range s.sortedWorkLocked() {
		if w.ConversationID != id {
			continue
		}
		reasons := []string{}
		if referenced[w.RequestID] {
			reasons = append(reasons, "immutable_context_or_task_link")
		}
		if !operationsComplete(w) {
			reasons = append(reasons, "unresolved_effect")
		}
		if w.Core != nil && !w.RuntimeStopped {
			reasons = append(reasons, "native_stop_unproven")
		}
		if w.Collaboration != nil {
			reasons = append(reasons, "collaboration_stage")
		}
		if len(w.Artifacts) > 0 {
			reasons = append(reasons, "result_file_binding")
		}
		if w.Resources != nil {
			reasons = append(reasons, "project_source_pin")
		}
		items = append(items, retentionplan.Item{ID: w.RequestID, CreatedAt: w.CreatedAt, Status: w.Status, ProtectedReasons: reasons, TextBytes: len(w.Text) + len(w.Reply)})
	}
	p, err := retentionplan.Plan(items, days, s.clock())
	if err != nil {
		return RetentionPreview{}, ErrInvalid
	}
	return RetentionPreview{Preview: p, ConversationID: id, AtSequence: uint64(len(s.events))}, nil
}
func (s *Store) retentionPreferencesLocked() []RetentionPreference {
	out := []RetentionPreference{}
	for _, p := range s.retentionPreferences {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ConversationID < out[j].ConversationID })
	return out
}
